package broker

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/basi/docbase-cli/cmd/root"
	internalbroker "github.com/basi/docbase-cli/internal/broker"
	"github.com/basi/docbase-cli/internal/config"
)

const testToken = "s3cret-cmd-token-0123456789"

// shortDir returns a new directory with a short path, for a socket.
func shortDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "bk")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// setFlag sets a flag of the command for the test.
func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()

	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("no flag %q", name)
	}
	original := flag.Value.String()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Set(%s, %s): %v", name, value, err)
	}
	t.Cleanup(func() {
		_ = cmd.Flags().Set(name, original)
		flag.Changed = false
	})
}

// run runs the command and returns what it wrote to stdout and stderr.
func run(t *testing.T, cmd *cobra.Command) (stdout, stderr string, err error) {
	t.Helper()

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	t.Cleanup(func() {
		cmd.SetOut(nil)
		cmd.SetErr(nil)
	})
	err = cmd.RunE(cmd, nil)

	return out.String(), errOut.String(), err
}

// fakeBroker plays a broker on a Unix socket.
func fakeBroker(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	socket := filepath.Join(shortDir(t), "fake.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return socket
}

func TestCommandsAreRegistered(t *testing.T) {
	cmd, _, err := BrokerCmd.Root().Find([]string{"broker", "start"})
	if err != nil || cmd != StartCmd {
		t.Fatalf("Find(broker start) = %v, %v", cmd, err)
	}
	for _, name := range []string{"status", "stop", "serve"} {
		if cmd, _, err := BrokerCmd.Root().Find([]string{"broker", name}); err != nil || cmd.Name() != name {
			t.Errorf("Find(broker %s) = %v, %v", name, cmd, err)
		}
	}
	if !ServeCmd.Hidden {
		t.Error("broker serve is not hidden")
	}
	if ServeCmd.Annotations["docbase/skip-config"] != "true" {
		t.Error("broker serve reads the config file; it has no home directory")
	}
	for _, c := range []*cobra.Command{StartCmd, StatusCmd, StopCmd} {
		if c.Hidden {
			t.Errorf("broker %s is hidden", c.Name())
		}
	}
	if ttl := StartCmd.Flags().Lookup("ttl"); ttl == nil || ttl.DefValue != "1h0m0s" {
		t.Errorf("--ttl = %v, want a default of 1h", ttl)
	}
}

func TestStartAndServeAreLinuxOnly(t *testing.T) {
	original := hostOS
	hostOS = "darwin"
	t.Cleanup(func() { hostOS = original })

	for _, cmd := range []*cobra.Command{StartCmd, ServeCmd} {
		if _, _, err := run(t, cmd); err == nil || err.Error() != "docbase broker is supported on Linux only" {
			t.Errorf("broker %s: got %v, want the Linux-only error", cmd.Name(), err)
		}
	}
}

// Before anything is started, a start must be refused for the reasons below.
func TestStartRefusals(t *testing.T) {
	original := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = original })

	// Nothing may reach the credential store or the API.
	keyring.MockInitWithError(errors.New("the credential store must not be used"))
	t.Cleanup(keyring.MockInit)
	unsetEnv(t, config.EnvBrokerSocket)
	t.Setenv(config.EnvAPIToken, testToken)
	t.Setenv("RUNNER_TEMP", shortDir(t))
	originalValidate := validateToken
	validated := 0
	validateToken = func(team, token string) error {
		validated++
		return nil
	}
	t.Cleanup(func() { validateToken = originalValidate })
	setFlag(t, StartCmd, "team", "example")

	t.Run("a broker is already set", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, "/run/other/broker.sock")
		_, stderr, err := run(t, StartCmd)
		if err == nil || !strings.Contains(err.Error(), "DOCBASE_BROKER_SOCKET is already set (/run/other/broker.sock)") {
			t.Errorf("got %v, want the already-set error", err)
		}
		if stderr != "" {
			t.Errorf("stderr = %q", stderr)
		}
	})

	t.Run("no team", func(t *testing.T) {
		setFlag(t, StartCmd, "team", "")
		if _, _, err := run(t, StartCmd); err == nil || !strings.Contains(err.Error(), "team domain is required") {
			t.Errorf("got %v, want the team error", err)
		}
	})

	t.Run("team that is not a name", func(t *testing.T) {
		setFlag(t, StartCmd, "team", "a/b")
		if _, _, err := run(t, StartCmd); err == nil || !strings.Contains(err.Error(), "team domain") {
			t.Errorf("got %v, want the team error", err)
		}
	})

	for _, ttl := range []string{"30s", "13h", "-1h", "0s"} {
		t.Run("ttl "+ttl, func(t *testing.T) {
			setFlag(t, StartCmd, "ttl", ttl)
			if _, _, err := run(t, StartCmd); err == nil || !strings.Contains(err.Error(), "ttl must be between 1m0s and 12h0m0s") {
				t.Errorf("got %v, want the ttl error", err)
			}
		})
	}

	t.Run("token that cannot be used", func(t *testing.T) {
		t.Setenv(config.EnvAPIToken, "s3cretvalue with space")
		_, _, err := run(t, StartCmd)
		if err == nil || strings.Contains(err.Error(), "s3cretvalue") {
			t.Errorf("got %v, want an error without the token", err)
		}
	})

	t.Run("no token", func(t *testing.T) {
		t.Setenv(config.EnvAPIToken, "")
		_, _, err := run(t, StartCmd)
		if err == nil {
			t.Fatal("got nil, want an error")
		}
		// The credential store failed on purpose; its error must be the one reported.
		if !strings.Contains(err.Error(), "credential store") {
			t.Errorf("got %v, want the credential store error", err)
		}
	})

	t.Run("authentication fails", func(t *testing.T) {
		validateToken = func(team, token string) error {
			return errors.New("API error: unauthorized for token " + token)
		}
		stdout, _, err := run(t, StartCmd)
		if err == nil || !strings.Contains(err.Error(), "failed to authenticate with DocBase") || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("got %v, want the authentication error", err)
		}
		if strings.Contains(err.Error(), testToken) {
			t.Errorf("the error contains the token: %v", err)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	// No socket directory was made by any of these.
	if entries, _ := os.ReadDir(os.Getenv("RUNNER_TEMP")); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

// The token check of start goes through the client of the broker (no proxy, no
// redirect, fixed base), not through the ordinary client of the CLI.
func TestStartChecksTheTokenWithTheClientOfTheBroker(t *testing.T) {
	if reflect.ValueOf(validateToken).Pointer() != reflect.ValueOf(internalbroker.ValidateToken).Pointer() {
		t.Error("validateToken is not broker.ValidateToken")
	}
	if internalbroker.Version != root.Version {
		t.Errorf("the version in the User-Agent of the check is %q, want %q", internalbroker.Version, root.Version)
	}
}

// A broker variable that is set but empty is an error, not a way back to credentials of the run's own.
func TestEmptyBrokerVariable(t *testing.T) {
	original := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = original })
	originalValidate := validateToken
	validated := 0
	validateToken = func(team, token string) error {
		validated++
		return nil
	}
	t.Cleanup(func() { validateToken = originalValidate })
	keyring.MockInitWithError(errors.New("the credential store must not be used"))
	t.Cleanup(keyring.MockInit)
	t.Setenv(config.EnvBrokerSocket, "")
	t.Setenv(config.EnvAPIToken, testToken)
	t.Setenv("RUNNER_TEMP", shortDir(t))
	setFlag(t, StartCmd, "team", "example")

	t.Run("start", func(t *testing.T) {
		stdout, _, err := run(t, StartCmd)
		if err == nil || !strings.Contains(err.Error(), "DOCBASE_BROKER_SOCKET is set but empty") || !strings.Contains(err.Error(), "unset it") {
			t.Errorf("got %v, want the set-but-empty error", err)
		}
		if stdout != "" || validated != 0 {
			t.Errorf("stdout = %q, token checked %d times; want nothing started", stdout, validated)
		}
	})

	for _, cmd := range []*cobra.Command{StatusCmd, StopCmd} {
		t.Run(cmd.Name()+" without --socket", func(t *testing.T) {
			if _, _, err := run(t, cmd); err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
				t.Errorf("got %v, want the set-but-empty error", err)
			}
		})
	}

	t.Run("--socket still works", func(t *testing.T) {
		socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"team":"example","expires_at":"2030-01-02T03:04:05Z"}`)
		})
		setFlag(t, StatusCmd, "socket", socket)
		if stdout, _, err := run(t, StatusCmd); err != nil || !strings.HasPrefix(stdout, "team: example\n") {
			t.Errorf("status --socket = %q, %v", stdout, err)
		}
	})
}

func TestStatusCommand(t *testing.T) {
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"team":"example","expires_at":"2030-01-02T03:04:05Z"}`)
	})

	t.Run("socket from the environment", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, socket)
		stdout, stderr, err := run(t, StatusCmd)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if want := "team: example\nexpires_at: 2030-01-02T03:04:05Z\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing", stderr)
		}
	})

	t.Run("socket from the flag, which wins", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, "/nonexistent/broker.sock")
		setFlag(t, StatusCmd, "socket", socket)
		if stdout, _, err := run(t, StatusCmd); err != nil || !strings.HasPrefix(stdout, "team: example\n") {
			t.Errorf("status = %q, %v", stdout, err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, "/nonexistent/broker.sock")
		stdout, _, err := run(t, StatusCmd)
		if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker") {
			t.Errorf("got %v, want the unreachable error", err)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	t.Run("no socket", func(t *testing.T) {
		unsetEnv(t, config.EnvBrokerSocket)
		if _, _, err := run(t, StatusCmd); err == nil || !strings.Contains(err.Error(), "no broker socket") {
			t.Errorf("got %v, want the no-socket error", err)
		}
	})
}

func TestStopCommand(t *testing.T) {
	stopped := make(chan struct{}, 1)
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/-/stop" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		stopped <- struct{}{}
	})

	t.Run("stops the broker", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, socket)
		stdout, stderr, err := run(t, StopCmd)
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
		select {
		case <-stopped:
		default:
			t.Error("the broker was not asked to stop")
		}
		// The confirmation goes to stderr only.
		if stdout != "" || !strings.Contains(stderr, "Stopped the DocBase broker at "+socket) {
			t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		t.Setenv(config.EnvBrokerSocket, "/nonexistent/broker.sock")
		_, stderr, err := run(t, StopCmd)
		if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker") {
			t.Errorf("got %v, want the unreachable error", err)
		}
		if strings.Contains(stderr, "Stopped") {
			t.Errorf("stderr = %q", stderr)
		}
	})

	t.Run("no socket", func(t *testing.T) {
		unsetEnv(t, config.EnvBrokerSocket)
		if _, _, err := run(t, StopCmd); err == nil || !strings.Contains(err.Error(), "no broker socket") {
			t.Errorf("got %v, want the no-socket error", err)
		}
	})
}

func TestInheritedPipeRefusesOtherFiles(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = file.Close() }()

	// A regular file at the given descriptor is not what start passes.
	if _, err := inheritedPipe(file.Fd(), "status"); err == nil || !strings.Contains(err.Error(), "not a pipe") {
		t.Errorf("inheritedPipe(file) = %v, want the not-a-pipe error", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	f, err := inheritedPipe(w.Fd(), "status")
	if err != nil || f == nil {
		t.Errorf("inheritedPipe(pipe) = %v, %v; want the pipe", f, err)
	}
}

// unsetEnv removes the variable for the test; the original value comes back afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
}
