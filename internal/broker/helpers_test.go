package broker

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testToken is a distinctive value, so that a leak into a response or an error is easy to spot.
const testToken = "s3cret-broker-token-0123456789"

// shortTempDir returns a new private directory with a short path: the path of a
// Unix socket is limited to about 100 bytes, and temporary directories on macOS are longer.
func shortTempDir(t *testing.T) string {
	t.Helper()

	base := "/tmp"
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "bk")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// recordedRequest is what the fake DocBase API saw.
type recordedRequest struct {
	Method string
	URI    string
	Header http.Header
	Host   string
}

// fakeUpstream is a TLS server that plays the DocBase API.
type fakeUpstream struct {
	*httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	respond  http.HandlerFunc
}

// newFakeUpstream starts a fake API that answers every request with respond
// (default: 200 and an empty JSON array).
func newFakeUpstream(t *testing.T, respond http.HandlerFunc) *fakeUpstream {
	t.Helper()

	f := &fakeUpstream{respond: respond}
	f.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{Method: r.Method, URI: r.URL.RequestURI(), Header: r.Header.Clone(), Host: r.Host})
		f.mu.Unlock()

		if f.respond != nil {
			f.respond(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "[]")
	}))
	t.Cleanup(f.Close)

	return f
}

// seen returns the requests that reached the fake API.
func (f *fakeUpstream) seen() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]recordedRequest(nil), f.requests...)
}

// testBroker is a Server serving on a Unix socket in a temporary directory, with a
// fake DocBase API behind it.
type testBroker struct {
	server   *Server
	upstream *fakeUpstream
	socket   string
	client   *http.Client
	peer     *switchablePeer
	cancel   context.CancelFunc

	stopped chan struct{} // closed when Serve has returned
	err     error         // the result of Serve, valid once stopped is closed
}

// switchablePeer is a peer lookup whose answer a test can change while the broker runs.
type switchablePeer struct {
	mu   sync.Mutex
	uid  uint32
	fail error
}

func (p *switchablePeer) lookup(net.Conn) (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.uid, p.fail
}

func (p *switchablePeer) set(uid uint32, fail error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uid, p.fail = uid, fail
}

// newTestBroker starts a broker for the team "example" that lives for ttl. The
// functions in setup change the server before it starts serving.
func newTestBroker(t *testing.T, ttl time.Duration, respond http.HandlerFunc, setup ...func(*Server)) *testBroker {
	t.Helper()

	upstream := newFakeUpstream(t, respond)
	server, err := NewServer(Config{Team: "example", Token: testToken, Version: "1.2.3", TTL: ttl})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.upstreamBase = upstream.URL
	server.upstream = hardenUpstream(upstream.Client())
	// The tests run on any platform; the real check is tested on Linux.
	peer := &switchablePeer{uid: server.uid}
	server.peerUID = peer.lookup
	for _, change := range setup {
		change(server)
	}

	socket := filepath.Join(shortTempDir(t), "broker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &testBroker{
		server:   server,
		upstream: upstream,
		socket:   socket,
		client:   &http.Client{Transport: NewTransport(socket), Timeout: 10 * time.Second},
		peer:     peer,
		cancel:   cancel,
		stopped:  make(chan struct{}),
	}
	go func() {
		b.err = server.Serve(ctx, listener)
		close(b.stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-b.stopped:
		case <-time.After(10 * time.Second):
			t.Error("the broker did not stop")
		}
	})

	return b
}

// waitStopped waits for the broker to stop on its own and returns the result of Serve.
func (b *testBroker) waitStopped(t *testing.T) error {
	t.Helper()

	select {
	case <-b.stopped:
		return b.err
	case <-time.After(5 * time.Second):
		t.Fatal("the broker did not stop")
		return nil
	}
}

// get sends a GET request for the path to the broker and returns the response with its body.
func (b *testBroker) get(t *testing.T, path string) (*http.Response, string) {
	t.Helper()

	return b.do(t, http.MethodGet, path, nil)
}

func (b *testBroker) do(t *testing.T, method, path string, header http.Header) (*http.Response, string) {
	t.Helper()

	resp, body, err := b.request(method, path, header)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}

	return resp, body
}

// request is like do, but returns the error, so that other goroutines can use it too.
func (b *testBroker) request(method, path string, header http.Header) (*http.Response, string, error) {
	req, err := http.NewRequest(method, BaseURL+path, nil)
	if err != nil {
		return nil, "", err
	}
	maps.Copy(req.Header, header)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	return resp, string(body), nil
}

// errorMessage returns the message of a broker error response.
func errorMessage(t *testing.T, body string) string {
	t.Helper()

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || payload.Error == "" {
		t.Fatalf("%q is not a broker error response (%v)", body, err)
	}

	return payload.Error
}
