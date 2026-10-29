package broker_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/basi/docbase-cli/internal/broker"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/internal/credential"
	"github.com/basi/docbase-cli/pkg/docbase"
)

const e2eToken = "s3cret-e2e-token-9876543210"

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
func TestMain(m *testing.M) {
	keyring.MockInit()
	credential.SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}

// e2eBroker is a broker for the team "example" in this process, with a fake DocBase API behind it.
type e2eBroker struct {
	socket string

	mu       sync.Mutex
	requests []*http.Request
}

func (b *e2eBroker) seen() []*http.Request {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]*http.Request(nil), b.requests...)
}

func startE2EBroker(t *testing.T) *e2eBroker {
	t.Helper()

	b := &e2eBroker{}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.requests = append(b.requests, r.Clone(context.Background()))
		b.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/teams/example/groups":
			_, _ = io.WriteString(w, `[{"id":1,"name":"Everyone"},{"id":2,"name":"Dev"}]`)
		case "/teams/example/groups/2":
			_, _ = io.WriteString(w, `{"id":2,"name":"Dev","users":[{"id":9,"name":"Ann"}]}`)
		case "/teams/example/posts":
			_, _ = io.WriteString(w, `{"posts":[{"id":7,"title":"Weekly"}],"meta":{"total":1}}`)
		case "/teams/example/posts/7":
			_, _ = io.WriteString(w, `{"id":7,"title":"Weekly","body":"Hello"}`)
		case "/teams/example/posts/7/comments":
			_, _ = io.WriteString(w, `{"comments":[{"id":3,"body":"Nice"}],"meta":{"total":1}}`)
		case "/teams/example/tags":
			_, _ = io.WriteString(w, `[{"name":"weekly"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not_found"}`)
		}
	}))
	t.Cleanup(upstream.Close)

	server, err := broker.NewServer(broker.Config{Team: "example", Token: e2eToken, Version: "e2e", TTL: time.Hour})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.SetUpstream(upstream.URL, upstream.Client())
	server.SetPeerUID(func(net.Conn) (uint32, error) { return uint32(os.Getuid()), nil })

	// The path of a socket is short, unlike the temporary directories of macOS.
	dir, err := os.MkdirTemp("/tmp", "bk")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	b.socket = filepath.Join(dir, "broker.sock")
	listener, err := net.Listen("unix", b.socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = server.Serve(ctx, listener)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	return b
}

// newCommand returns a command with the global flags that client.Create reads.
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

// brokerMode puts the process into broker mode for the test, with no credentials
// of its own and a credential store that fails when it is touched.
func brokerMode(t *testing.T, socket string) {
	t.Helper()

	t.Setenv(config.EnvBrokerSocket, socket)
	t.Setenv(config.EnvAPIToken, "")
	t.Setenv(config.EnvInsecureStorage, "")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)
}

func TestCreateTalksToTheBroker(t *testing.T) {
	b := startE2EBroker(t)
	brokerMode(t, b.socket)

	api, err := client.Create(newCommand(t, nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	groups, err := api.Group.List(1, 10)
	if err != nil || len(groups.Groups) != 2 || groups.Groups[1].Name != "Dev" {
		t.Fatalf("Group.List = %+v, %v", groups, err)
	}
	group, err := api.Group.Get(2)
	if err != nil || group.Name != "Dev" || len(group.Users) != 1 {
		t.Fatalf("Group.Get = %+v, %v", group, err)
	}
	memos, err := api.Memo.List(1, 20, "tag:weekly 週報")
	if err != nil || len(memos.Memos) != 1 || memos.Memos[0].Title != "Weekly" {
		t.Fatalf("Memo.List = %+v, %v", memos, err)
	}
	memo, err := api.Memo.Get(7)
	if err != nil || memo.Body != "Hello" {
		t.Fatalf("Memo.Get = %+v, %v", memo, err)
	}
	comments, err := api.Comment.List(7, 1, 20)
	if err != nil || len(comments.Comments) != 1 || comments.Comments[0].Body != "Nice" {
		t.Fatalf("Comment.List = %+v, %v", comments, err)
	}
	tags, err := api.Tag.Search("week", 1, 20)
	if err != nil || len(tags.Tags) != 1 || tags.Tags[0].Name != "weekly" {
		t.Fatalf("Tag.Search = %+v, %v", tags, err)
	}
	tagList, err := api.Tag.List(2, 5)
	if err != nil || len(tagList.Tags) != 1 {
		t.Fatalf("Tag.List = %+v, %v", tagList, err)
	}

	// Every request reached the API with the token the broker holds, and nothing else.
	wantURIs := []string{
		"/teams/example/groups?page=1&per_page=10",
		"/teams/example/groups/2",
		"/teams/example/posts?page=1&per_page=20&q=tag%3Aweekly+%E9%80%B1%E5%A0%B1",
		"/teams/example/posts/7",
		"/teams/example/posts/7/comments?page=1&per_page=20",
		"/teams/example/tags?page=1&per_page=20&q=week",
		"/teams/example/tags?page=2&per_page=5",
	}
	seen := b.seen()
	if len(seen) != len(wantURIs) {
		t.Fatalf("the API saw %d requests, want %d", len(seen), len(wantURIs))
	}
	for i, r := range seen {
		if got := r.URL.RequestURI(); got != wantURIs[i] {
			t.Errorf("request %d: %s, want %s", i, got, wantURIs[i])
		}
		if got := r.Header.Get("X-DocBaseToken"); got != e2eToken {
			t.Errorf("request %d: token = %q, want the token of the broker", i, got)
		}
		if got := r.Header.Get("User-Agent"); got != "docbase-cli-broker/e2e" {
			t.Errorf("request %d: User-Agent = %q", i, got)
		}
	}
}

func TestBrokerModeIsReadOnly(t *testing.T) {
	b := startE2EBroker(t)
	brokerMode(t, b.socket)

	api, err := client.Create(newCommand(t, nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	writes := map[string]error{
		"Memo.Create":    errOf(api.Memo.Create(&docbase.CreateMemoRequest{Title: "t", Body: "b"})),
		"Memo.Update":    errOf(api.Memo.Update(7, &docbase.UpdateMemoRequest{Title: "t"})),
		"Memo.Delete":    api.Memo.Delete(7),
		"Memo.Archive":   api.Memo.Archive(7),
		"Memo.Unarchive": api.Memo.Unarchive(7),
		"Comment.Create": errOf(api.Comment.Create(7, &docbase.CreateCommentRequest{Body: "b"})),
		"Comment.Delete": api.Comment.Delete(3),
	}
	for name, err := range writes {
		if err == nil || !strings.Contains(err.Error(), "only GET requests are allowed") {
			t.Errorf("%s = %v, want the read-only error", name, err)
		}
	}

	// A read that the policy does not list (what 'docbase api get' would send)
	for _, path := range []string{"/users", "/posts/7/archive", "/comments/3", "/posts/7/../groups"} {
		resp, err := api.Get(path, nil)
		if err != nil {
			t.Fatalf("Get(%s): %v", path, err)
		}
		if resp.StatusCode() != http.StatusForbidden {
			t.Errorf("Get(%s): status = %d, want 403", path, resp.StatusCode())
		}
	}
	if resp, err := api.Get("/posts", map[string]string{"per_page": "1000"}); err != nil || resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("a page size of 1000: %v, %v; want 400", resp, err)
	}

	if got := b.seen(); len(got) != 0 {
		t.Errorf("the API saw %d requests for refused ones", len(got))
	}
}

func TestCreateInBrokerModeRefusesATokenOfItsOwn(t *testing.T) {
	b := startE2EBroker(t)
	missing := filepath.Join(filepath.Dir(b.socket), "missing.sock")

	tests := []struct {
		name  string
		flags map[string]string
		env   string
	}{
		{"flag", map[string]string{"token": "from-flag"}, ""},
		{"environment", nil, "from-env"},
		{"both", map[string]string{"token": "from-flag"}, "from-env"},
	}
	for _, tt := range tests {
		for socketName, socket := range map[string]string{"running broker": b.socket, "missing socket": missing} {
			t.Run(tt.name+", "+socketName, func(t *testing.T) {
				brokerMode(t, socket)
				t.Setenv(config.EnvAPIToken, tt.env)

				api, err := client.Create(newCommand(t, tt.flags))
				if err == nil || !strings.Contains(err.Error(), "DOCBASE_BROKER_SOCKET cannot be combined with --token or DOCBASE_API_TOKEN") {
					t.Errorf("Create = %v, %v; want the combination error", api, err)
				}
				// It fails before anything is sent anywhere, so no token can reach a socket.
				if got := b.seen(); len(got) != 0 {
					t.Errorf("the API saw %d requests", len(got))
				}
			})
		}
	}
}

func TestCreateInBrokerModeDoesNotFallBack(t *testing.T) {
	missing := filepath.Join(shortDir(t), "missing.sock")
	brokerMode(t, missing)
	// With these a direct client could be made; the broker mode must not do that.
	t.Setenv("HOME", shortDir(t))

	api, err := client.Create(newCommand(t, map[string]string{"team": "example"}))
	if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker at "+missing) {
		t.Errorf("Create = %v, %v; want the unreachable error", api, err)
	}

	// A path that is not a socket
	notSocket := filepath.Join(shortDir(t), "file")
	if err := os.WriteFile(notSocket, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	brokerMode(t, notSocket)
	if api, err := client.Create(newCommand(t, nil)); err == nil {
		t.Errorf("Create = %v, want an error for a path that is not a socket", api)
	}
}

func TestCreateInBrokerModeChecksTheTeam(t *testing.T) {
	b := startE2EBroker(t)
	brokerMode(t, b.socket)

	for _, team := range []string{"", "example", "EXAMPLE", "Example"} {
		if _, err := client.Create(newCommand(t, map[string]string{"team": team})); err != nil {
			t.Errorf("--team %q: %v", team, err)
		}
	}
	for _, team := range []string{"other", "example2", "exampl"} {
		_, err := client.Create(newCommand(t, map[string]string{"team": team}))
		if err == nil || !strings.Contains(err.Error(), "does not match the team of the DocBase broker") {
			t.Errorf("--team %q: got %v, want the mismatch error", team, err)
		}
	}

	// The configured team does not matter: the broker decides.
	api, err := client.Create(newCommand(t, nil))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := api.Group.List(1, 1); err != nil {
		t.Errorf("Group.List: %v", err)
	}
}

func TestStatusThroughTheBrokerPackage(t *testing.T) {
	b := startE2EBroker(t)

	info, err := broker.Status(b.socket)
	if err != nil || info.Team != "example" {
		t.Fatalf("Status = %+v, %v", info, err)
	}
	if err := broker.Stop(b.socket); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestWithoutBrokerModeTheTokenIsResolvedAsBefore(t *testing.T) {
	unsetEnv(t, config.EnvBrokerSocket)
	t.Setenv(config.EnvAPIToken, "token-from-env")
	keyring.MockInitWithError(errors.New("the credential store is not needed"))
	t.Cleanup(keyring.MockInit)

	api, err := client.Create(newCommand(t, map[string]string{"team": "example"}))
	if err != nil || api == nil {
		t.Fatalf("Create = %v, %v", api, err)
	}
	if _, err := client.Create(newCommand(t, nil)); err == nil {
		t.Error("Create without a team succeeded")
	}
}

func errOf[T any](_ T, err error) error { return err }

func shortDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "bk")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// unsetEnv removes the variable for the test; the original value comes back afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
}
