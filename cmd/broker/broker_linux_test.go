//go:build linux

package broker

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/basi/docbase-cli/internal/broker"
	"github.com/basi/docbase-cli/internal/config"
)

// proc reads a file of /proc/<pid>.
func proc(t *testing.T, pid int, name string) (string, error) {
	t.Helper()

	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/" + name)

	return string(data), err
}

// TestStartServeStopEndToEnd runs the real thing: 'broker start' starts the test
// binary again as 'broker serve', hands over the token, and the broker serves and stops.
func TestStartServeStopEndToEnd(t *testing.T) {
	t.Setenv("RUNNER_TEMP", shortDir(t))
	unsetEnv(t, config.EnvBrokerSocket)
	t.Setenv(config.EnvAPIToken, testToken)
	// What serve may inherit, and what it must not.
	t.Setenv("SSL_CERT_FILE", "/etc/ssl/e2e-ca.pem")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("DOCBASE_E2E_SECRET", "must-not-reach-serve")
	wantEnv := []string{"SSL_CERT_FILE=/etc/ssl/e2e-ca.pem"}
	for _, key := range []string{"SSL_CERT_DIR", "RUNNER_TRACKING_ID"} {
		if value, ok := os.LookupEnv(key); ok {
			wantEnv = append(wantEnv, key+"="+value)
		}
	}

	var validated string
	originalValidate := validateToken
	validateToken = func(team, token string) error {
		validated = token
		return nil
	}
	t.Cleanup(func() { validateToken = originalValidate })
	setFlag(t, StartCmd, "team", "example")
	setFlag(t, StartCmd, "ttl", "30m")

	stdout, stderr, err := run(t, StartCmd)
	if err != nil {
		t.Fatalf("broker start: %v", err)
	}
	if validated != testToken {
		t.Errorf("the token was checked as %q", validated)
	}

	// Exactly one line on stdout, suitable for $GITHUB_ENV.
	if !strings.HasPrefix(stdout, "DOCBASE_BROKER_SOCKET=/") || strings.Count(stdout, "\n") != 1 || !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout = %q, want one DOCBASE_BROKER_SOCKET=<path> line", stdout)
	}
	socket := strings.TrimSuffix(strings.TrimPrefix(stdout, "DOCBASE_BROKER_SOCKET="), "\n")
	match := regexp.MustCompile(`\(pid (\d+)\)`).FindStringSubmatch(stderr)
	if match == nil {
		t.Fatalf("stderr = %q, want the pid of the broker", stderr)
	}
	pid, _ := strconv.Atoi(match[1])
	if strings.Contains(stderr, testToken) || strings.Contains(stdout, testToken) {
		t.Error("the output contains the token")
	}

	t.Cleanup(func() {
		_ = broker.Stop(socket)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = reap(pid, time.Second)
	})

	// The command line has no secret.
	cmdline, err := proc(t, pid, "cmdline")
	if err != nil {
		t.Fatalf("cmdline: %v", err)
	}
	exe, _ := os.Executable()
	wantArgs := []string{exe, "broker", "serve", "--team", "example", "--socket", socket, "--ttl", "30m0s", ""}
	if got := strings.Split(cmdline, "\x00"); !slices.Equal(got, wantArgs) {
		t.Errorf("command line = %q, want %q", got, wantArgs)
	}

	// The process hardened itself and is in a session of its own.
	status, err := proc(t, pid, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	// no_new_privs is set on every thread where the runtime can do that (not in a binary
	// that uses cgo, where only the thread that hardened the process has it).
	if _, _, errno := syscall.AllThreadsSyscall(syscall.SYS_GETPID, 0, 0, 0); errno != 0 {
		t.Logf("not checking no_new_privs: the binary uses cgo (%v)", errno)
	} else if !regexp.MustCompile(`(?m)^NoNewPrivs:\s+1$`).MatchString(status) {
		t.Errorf("no_new_privs is not set:\n%s", status)
	}
	limits, _ := proc(t, pid, "limits")
	if !regexp.MustCompile(`(?m)^Max core file size\s+0\s+0\s`).MatchString(limits) {
		t.Errorf("core files are not disabled:\n%s", limits)
	}
	stat, err := proc(t, pid, "stat")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	fields := strings.Fields(stat[strings.LastIndex(stat, ")")+2:])
	if len(fields) < 4 || fields[3] != strconv.Itoa(pid) {
		t.Errorf("session = %v, want the process to lead its own session (%d)", fields, pid)
	}

	// Its environment cannot be read by the same user, not even by root without the
	// capability to trace other processes (a container has none). Where it can be read
	// it is the minimum.
	environ, err := proc(t, pid, "environ")
	switch {
	case errors.Is(err, os.ErrPermission):
	case err != nil:
		t.Errorf("environ of the hardened process: %v, want a permission error", err)
	case os.Geteuid() != 0:
		t.Errorf("environ of the hardened process can be read by a user that is not root: %q", environ)
	default:
		got := strings.FieldsFunc(environ, func(r rune) bool { return r == 0 })
		slices.Sort(got)
		slices.Sort(wantEnv)
		if !slices.Equal(got, wantEnv) {
			t.Errorf("environment of serve = %q, want %q", got, wantEnv)
		}
	}

	// The socket and its directory are private.
	dir := filepath.Dir(socket)
	if info, err := os.Lstat(dir); err != nil || info.Mode().Perm() != 0o700 || !info.IsDir() {
		t.Errorf("socket directory: %v, %v; want a directory with mode 0700", info, err)
	}
	if info, err := os.Lstat(socket); err != nil || info.Mode().Perm() != 0o600 || info.Mode().Type() != os.ModeSocket {
		t.Errorf("socket: %v, %v; want a socket with mode 0600", info, err)
	}

	// It serves its team to this user, and refuses what the policy refuses (no request goes upstream).
	t.Setenv(config.EnvBrokerSocket, socket)
	statusOut, _, err := run(t, StatusCmd)
	if err != nil || !strings.HasPrefix(statusOut, "team: example\nexpires_at: ") {
		t.Errorf("broker status = %q, %v", statusOut, err)
	}
	client := &http.Client{Transport: broker.NewTransport(socket), Timeout: 5 * time.Second}
	for path, want := range map[string]int{
		"/teams/other/posts":          http.StatusForbidden,
		"/teams/example/posts/1/edit": http.StatusForbidden,
		"/teams/example/posts?page=0": http.StatusBadRequest,
	} {
		resp, err := client.Get(broker.BaseURL + path)
		if err != nil {
			t.Errorf("GET %s: %v", path, err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("GET %s: status = %d, want %d", path, resp.StatusCode, want)
		}
	}

	// A second broker cannot be started from here while this one is set.
	if _, _, err := run(t, StartCmd); err == nil || !strings.Contains(err.Error(), "already set") {
		t.Errorf("a second start: got %v, want the already-set error", err)
	}

	// Stop it.
	_, stopErr, err := run(t, StopCmd)
	if err != nil || !strings.Contains(stopErr, "Stopped the DocBase broker") {
		t.Fatalf("broker stop = %q, %v", stopErr, err)
	}
	exit, ok := reap(pid, 10*time.Second)
	if !ok {
		t.Fatal("the broker process did not exit after stop")
	}
	if exit != 0 {
		t.Errorf("the broker exited with status %d, want 0", exit)
	}
	for _, path := range []string{socket, dir} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists after stop (Lstat error: %v)", path, err)
		}
	}
	if _, _, err := run(t, StatusCmd); err == nil {
		t.Error("broker status succeeded after stop")
	}
}

// reap waits for the child with the process id to exit and returns its exit status.
func reap(pid int, timeout time.Duration) (status int, exited bool) {
	deadline := time.Now().Add(timeout)
	for {
		var ws syscall.WaitStatus
		wpid, err := syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
		if err != nil && !errors.Is(err, syscall.EINTR) {
			return -1, false
		}
		if wpid == pid {
			return ws.ExitStatus(), true
		}
		if time.Now().After(deadline) {
			return -1, false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
