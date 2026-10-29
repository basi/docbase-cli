package auth

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"

	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/internal/credential"
)

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
func TestMain(m *testing.M) {
	keyring.MockInit()
	credential.SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}

// brokerEnv puts the test into broker mode with a credential store that fails when it is touched,
// and returns the path of a config file that must stay untouched.
func brokerEnv(t *testing.T, socket string) string {
	t.Helper()

	color.NoColor = true
	t.Setenv(config.EnvBrokerSocket, socket)
	t.Setenv(config.EnvAPIToken, "")
	t.Setenv(config.EnvInsecureStorage, "")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)

	viper.Reset()
	t.Cleanup(viper.Reset)
	configFile := filepath.Join(t.TempDir(), "config.json")
	viper.SetConfigFile(configFile)

	return configFile
}

// fakeBroker plays a broker for the team "example"; the API check at /teams/example/groups
// answers with groupsStatus.
func fakeBroker(t *testing.T, groupsStatus int) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "bk")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "fake.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/-/status":
			_, _ = io.WriteString(w, `{"team":"example","expires_at":"2030-01-02T03:04:05Z"}`)
		case "/teams/example/groups":
			w.WriteHeader(groupsStatus)
			if groupsStatus == http.StatusOK {
				_, _ = io.WriteString(w, `[{"id":1,"name":"Everyone"}]`)
			} else {
				_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return socket
}

func runStatus(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()

	var out, errOut bytes.Buffer
	StatusCmd.SetOut(&out)
	StatusCmd.SetErr(&errOut)
	t.Cleanup(func() {
		StatusCmd.SetOut(nil)
		StatusCmd.SetErr(nil)
	})
	err = StatusCmd.RunE(StatusCmd, nil)

	return out.String(), errOut.String(), err
}

// setRootFlag sets a global flag as the command line would.
func setRootFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()

	flag := cmd.Flag(name)
	if flag == nil {
		t.Fatalf("no flag %q", name)
	}
	if err := flag.Value.Set(value); err != nil {
		t.Fatalf("Set(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = flag.Value.Set("") })
}

func TestLoginAndLogoutAreRefusedInBrokerMode(t *testing.T) {
	configFile := brokerEnv(t, "/nonexistent/broker.sock")

	for name, cmd := range map[string]*cobra.Command{"auth login": LoginCmd, "auth logout": LogoutCmd} {
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), name+" is not available while DOCBASE_BROKER_SOCKET is set") {
			t.Errorf("%s: got %v, want the broker mode error", name, err)
		}
	}
	// Login with a token on the command line is refused too, and nothing is saved.
	if err := LoginCmd.Flags().Set("team", "example"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := LoginCmd.Flags().Set("token", testToken); err != nil {
		t.Fatalf("Set: %v", err)
	}
	t.Cleanup(func() {
		_ = LoginCmd.Flags().Set("team", "")
		_ = LoginCmd.Flags().Set("token", "")
	})
	if err := LoginCmd.RunE(LoginCmd, nil); err == nil {
		t.Error("login with a token succeeded in broker mode")
	}
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the config file was written (Stat error: %v)", err)
	}
}

const testToken = "s3cret-auth-token-0123456789"

func TestStatusInBrokerMode(t *testing.T) {
	t.Run("logged in through the broker", func(t *testing.T) {
		brokerEnv(t, fakeBroker(t, http.StatusOK))

		stdout, stderr, err := runStatus(t)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if want := "Logged in to DocBase as team: example\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing", stderr)
		}
	})

	t.Run("the broker's token is rejected", func(t *testing.T) {
		brokerEnv(t, fakeBroker(t, http.StatusUnauthorized))

		stdout, _, err := runStatus(t)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if want := "Logged in as team: example, but authentication failed: API error: unauthorized\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("broker unreachable", func(t *testing.T) {
		brokerEnv(t, "/nonexistent/broker.sock")

		stdout, stderr, err := runStatus(t)
		if err != nil {
			t.Fatalf("status: %v, want exit 0", err)
		}
		if want := "Not logged in to DocBase\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !strings.Contains(stderr, "cannot reach the DocBase broker") {
			t.Errorf("stderr = %q, want the reason", stderr)
		}
	})

	t.Run("a token of its own", func(t *testing.T) {
		brokerEnv(t, fakeBroker(t, http.StatusOK))
		t.Setenv(config.EnvAPIToken, testToken)

		stdout, stderr, err := runStatus(t)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if want := "Not logged in to DocBase\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !strings.Contains(stderr, "cannot be combined") || strings.Contains(stderr, testToken) {
			t.Errorf("stderr = %q, want the combination error without the token", stderr)
		}
	})

	t.Run("another team on the command line", func(t *testing.T) {
		brokerEnv(t, fakeBroker(t, http.StatusOK))
		setRootFlag(t, StatusCmd, "team", "other")

		stdout, stderr, err := runStatus(t)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if want := "Not logged in to DocBase\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !strings.Contains(stderr, "does not match") {
			t.Errorf("stderr = %q, want the mismatch error", stderr)
		}
	})

	t.Run("the same team on the command line", func(t *testing.T) {
		brokerEnv(t, fakeBroker(t, http.StatusOK))
		setRootFlag(t, StatusCmd, "team", "EXAMPLE")

		stdout, _, err := runStatus(t)
		if err != nil || stdout != "Logged in to DocBase as team: example\n" {
			t.Errorf("status = %q, %v", stdout, err)
		}
	})
}

// A broker variable that is set but empty is an error, not a way back to direct access.
func TestEmptyBrokerVariable(t *testing.T) {
	configFile := brokerEnv(t, "")
	// What a direct login would use is there.
	t.Setenv(config.EnvAPIToken, testToken)
	setRootFlag(t, StatusCmd, "team", "example")

	for name, cmd := range map[string]*cobra.Command{"auth login": LoginCmd, "auth logout": LogoutCmd} {
		err := cmd.RunE(cmd, nil)
		if err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
			t.Errorf("%s: got %v, want the set-but-empty error", name, err)
		}
	}
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the config file was written (Stat error: %v)", err)
	}

	stdout, stderr, err := runStatus(t)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if want := "Not logged in to DocBase\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if !strings.Contains(stderr, "DOCBASE_BROKER_SOCKET is set but empty") || strings.Contains(stderr, testToken) {
		t.Errorf("stderr = %q, want the set-but-empty error", stderr)
	}
}
