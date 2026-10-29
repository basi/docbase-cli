package client

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
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

func newCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("team", "", "")
	cmd.Flags().String("token", "", "")
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Set(%s): %v", name, err)
		}
	}

	return cmd
}

// brokerMode puts the test into broker mode, with a credential store that fails when it is touched.
func brokerMode(t *testing.T, socket string) {
	t.Helper()

	t.Setenv(config.EnvBrokerSocket, socket)
	t.Setenv(config.EnvAPIToken, "")
	t.Setenv(config.EnvInsecureStorage, "")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)
}

// fakeBroker plays a broker for the team "example"; it records the headers it receives.
func fakeBroker(t *testing.T) (socket string, headers func() []http.Header) {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "bk")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket = filepath.Join(dir, "fake.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	var mu sync.Mutex
	var seen []http.Header
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/-/status" {
			_, _ = io.WriteString(w, `{"team":"example","expires_at":"2030-01-02T03:04:05Z"}`)
			return
		}
		_, _ = io.WriteString(w, `[{"id":1,"name":"Everyone"}]`)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return socket, func() []http.Header {
		mu.Lock()
		defer mu.Unlock()
		return append([]http.Header(nil), seen...)
	}
}

func TestCreateInBrokerMode(t *testing.T) {
	socket, headers := fakeBroker(t)
	brokerMode(t, socket)

	api, err := Create(newCommand(t, nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	groups, err := api.Group.List(1, 10)
	if err != nil || len(groups.Groups) != 1 {
		t.Fatalf("Group.List = %+v, %v", groups, err)
	}

	// Only the broker is asked: the status, then the API call. No credential travels to it.
	seen := headers()
	if len(seen) != 2 {
		t.Fatalf("the broker saw %d requests, want 2", len(seen))
	}
	for i, header := range seen {
		for _, name := range []string{"X-Docbasetoken", "Authorization", "Cookie"} {
			if values := header.Values(name); len(values) != 0 {
				t.Errorf("request %d carries %s: %v", i, name, values)
			}
		}
	}
}

func TestCreateInBrokerModeTeamFlag(t *testing.T) {
	socket, _ := fakeBroker(t)
	brokerMode(t, socket)

	for _, team := range []string{"example", "EXAMPLE"} {
		if _, err := Create(newCommand(t, map[string]string{"team": team})); err != nil {
			t.Errorf("--team %s: %v", team, err)
		}
	}
	_, err := Create(newCommand(t, map[string]string{"team": "other"}))
	if err == nil || !strings.Contains(err.Error(), `--team "other" does not match the team of the DocBase broker ("example")`) {
		t.Errorf("--team other: got %v, want the mismatch error", err)
	}
}

func TestCreateInBrokerModeRefusesATokenOfItsOwn(t *testing.T) {
	socket, headers := fakeBroker(t)

	tests := []struct {
		name  string
		flags map[string]string
		env   string
	}{
		{"flag", map[string]string{"token": "from-flag"}, ""},
		{"environment", nil, "from-env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brokerMode(t, socket)
			t.Setenv(config.EnvAPIToken, tt.env)

			_, err := Create(newCommand(t, tt.flags))
			if err == nil || !strings.Contains(err.Error(), "DOCBASE_BROKER_SOCKET cannot be combined with --token or DOCBASE_API_TOKEN") {
				t.Errorf("got %v, want the combination error", err)
			}
		})
	}
	// The check comes before the broker is asked anything.
	if got := headers(); len(got) != 0 {
		t.Errorf("the broker saw %d requests, want none", len(got))
	}
}

func TestCreateInBrokerModeDoesNotFallBack(t *testing.T) {
	brokerMode(t, "/nonexistent/broker.sock")

	_, err := Create(newCommand(t, map[string]string{"team": "example"}))
	if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker at /nonexistent/broker.sock") {
		t.Errorf("got %v, want the unreachable error", err)
	}
}

// A broker variable that is set but empty must not lead to a client with credentials of its own.
func TestCreateWithAnEmptyBrokerVariable(t *testing.T) {
	// Everything a direct client needs is there, and the credential store would answer.
	t.Setenv(config.EnvBrokerSocket, "")
	t.Setenv(config.EnvAPIToken, "token-from-env")
	t.Setenv(config.EnvInsecureStorage, "")
	if err := keyring.Set("docbase-cli", "example", "token-from-keyring"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	t.Cleanup(keyring.MockInit)

	for _, flags := range []map[string]string{nil, {"team": "example"}} {
		api, err := Create(newCommand(t, flags))
		if err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
			t.Errorf("Create(%v) = %v, %v; want the set-but-empty error", flags, api, err)
		}
		if api != nil {
			t.Errorf("Create(%v) returned a client", flags)
		}
	}

	// The same for the entry that the status command uses.
	api, team, err := CreateForBroker(newCommand(t, nil), "")
	if err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" || api != nil || team != "" {
		t.Errorf("CreateForBroker with no socket = %v, %q, %v; want the set-but-empty error", api, team, err)
	}
}

func TestCreateOutsideBrokerMode(t *testing.T) {
	unsetEnv(t, config.EnvBrokerSocket)
	t.Setenv(config.EnvAPIToken, "token-from-env")
	t.Setenv(config.EnvInsecureStorage, "")

	if api, err := Create(newCommand(t, map[string]string{"team": "example"})); err != nil || api == nil {
		t.Errorf("Create = %v, %v; want a client", api, err)
	}
	if _, err := Create(newCommand(t, nil)); err == nil || !strings.Contains(err.Error(), "team domain is required") {
		t.Errorf("Create without a team: got %v, want the team error", err)
	}

	// Without a token anywhere, the credential store is asked, as before.
	t.Setenv(config.EnvAPIToken, "")
	_, err := Create(newCommand(t, map[string]string{"team": "example"}))
	if !errors.Is(err, config.ErrNoToken) {
		t.Errorf("got %v, want ErrNoToken", err)
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
