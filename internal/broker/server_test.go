package broker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestForwardSendsOnlyTheBrokerHeaders(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	// The client tries to supply its own credentials and identity.
	resp, body := b.do(t, http.MethodGet, "/teams/EXAMPLE/posts?q=a+b&per_page=5&page=2", http.Header{
		"X-Docbasetoken":  {"client-supplied-token"},
		"Authorization":   {"Bearer client-token"},
		"Cookie":          {"session=client"},
		"X-Forwarded-For": {"203.0.113.9"},
		"User-Agent":      {"evil-agent/9"},
		"Accept":          {"text/html"},
		"Accept-Encoding": {"gzip"},
		"X-Custom":        {"1"},
	})
	if resp.StatusCode != http.StatusOK || body != "[]" {
		t.Fatalf("got %d %q, want 200 []", resp.StatusCode, body)
	}

	seen := b.upstream.seen()
	if len(seen) != 1 {
		t.Fatalf("the upstream saw %d requests, want 1", len(seen))
	}
	got := seen[0]
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	// The team is the pinned one, and the query is rebuilt.
	if want := "/teams/example/posts?page=2&per_page=5&q=a+b"; got.URI != want {
		t.Errorf("URI = %q, want %q", got.URI, want)
	}

	want := http.Header{
		"X-Docbasetoken": {testToken},
		"X-Api-Version":  {"3"},
		"Accept":         {"application/json"},
		"User-Agent":     {"docbase-cli-broker/1.2.3"},
	}
	if len(got.Header) != len(want) {
		t.Errorf("the upstream saw the headers %v, want exactly %v", got.Header, want)
	}
	for name, values := range want {
		if !slices.Equal(got.Header[name], values) {
			t.Errorf("header %s = %v, want %v", name, got.Header[name], values)
		}
	}
}

func TestForwardFiltersResponseHeaders(t *testing.T) {
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("Set-Cookie", "session=upstream; HttpOnly")
		h.Set("Retry-After", "7")
		h.Set("X-RateLimit-Limit", "100")
		h.Set("X-RateLimit-Remaining", "42")
		h.Set("X-RateLimit-Reset", "1700000000")
		h.Set("Server", "upstream")
		h.Set("X-Request-Id", "abc")
		h.Set("Link", "<https://example.com>; rel=next")
		h.Set("Location", "https://example.com")
		h.Set("Content-Encoding", "identity")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"rate_limit"}`)
	})

	resp, body := b.get(t, "/teams/example/posts")
	if resp.StatusCode != http.StatusTooManyRequests || body != `{"error":"rate_limit"}` {
		t.Fatalf("got %d %q, want the upstream answer", resp.StatusCode, body)
	}

	for name, want := range map[string]string{
		"Content-Type":          "application/json; charset=utf-8",
		"Retry-After":           "7",
		"X-Ratelimit-Limit":     "100",
		"X-Ratelimit-Remaining": "42",
		"X-Ratelimit-Reset":     "1700000000",
	} {
		if got := resp.Header.Get(name); got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"Set-Cookie", "Server", "X-Request-Id", "Link", "Location", "Content-Encoding"} {
		if got := resp.Header.Values(name); len(got) != 0 {
			t.Errorf("header %s = %v, want it dropped", name, got)
		}
	}
}

func TestForwardDoesNotTrustAnOtherContentType(t *testing.T) {
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>bad gateway</html>")
	})

	resp, body := b.get(t, "/teams/example/groups")
	if resp.StatusCode != http.StatusBadGateway || body != "<html>bad gateway</html>" {
		t.Fatalf("got %d %q, want the upstream answer", resp.StatusCode, body)
	}
	// No type is given, and none is guessed from the body.
	if got := resp.Header.Values("Content-Type"); len(got) != 0 {
		t.Errorf("Content-Type = %v, want none", got)
	}
}

func TestForwardRedirectIsAnError(t *testing.T) {
	// Following the redirect would send the token to another address.
	elsewhere := newFakeUpstream(t, nil)
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/teams/example/posts", http.StatusFound)
	})

	for _, path := range []string{"/teams/example/posts", "/teams/example/groups/1"} {
		resp, body := b.get(t, path)
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("GET %s: status = %d, want 502", path, resp.StatusCode)
		}
		if resp.Header.Get("Location") != "" {
			t.Errorf("GET %s: the redirect target was passed on", path)
		}
		if strings.Contains(body, testToken) || strings.Contains(body, elsewhere.URL) {
			t.Errorf("GET %s: unexpected body %q", path, body)
		}
		errorMessage(t, body)
	}
	if got := elsewhere.seen(); len(got) != 0 {
		t.Errorf("the redirect target received %d requests, want none", len(got))
	}
}

func TestForwardRedirectStatuses(t *testing.T) {
	for _, status := range []int{300, 301, 302, 303, 304, 307, 308} {
		b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(status)
		})
		if resp, _ := b.get(t, "/teams/example/tags"); resp.StatusCode != http.StatusBadGateway {
			t.Errorf("upstream %d: status = %d, want 502", status, resp.StatusCode)
		}
	}
}

func TestForwardBodyLimit(t *testing.T) {
	respond := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat("x", 100))
	}

	at := newTestBroker(t, time.Hour, respond, func(s *Server) { s.maxBody = 100 })
	resp, body := at.get(t, "/teams/example/tags")
	if resp.StatusCode != http.StatusOK || len(body) != 100 {
		t.Errorf("a body at the limit: got %d with %d bytes, want 200 with 100", resp.StatusCode, len(body))
	}

	over := newTestBroker(t, time.Hour, respond, func(s *Server) { s.maxBody = 99 })
	resp, body = over.get(t, "/teams/example/tags")
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("a body over the limit: status = %d, want 502", resp.StatusCode)
	}
	if strings.Contains(body, "xxx") {
		t.Errorf("part of the oversized body was passed on: %q", body)
	}
	errorMessage(t, body)
}

func TestForwardUpstreamErrors(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		b := newTestBroker(t, time.Hour, nil)
		b.upstream.Close()

		resp, body := b.get(t, "/teams/example/posts")
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", resp.StatusCode)
		}
		if strings.Contains(body, testToken) {
			t.Errorf("the response contains the token: %q", body)
		}
		errorMessage(t, body)
	})

	t.Run("connection cut", func(t *testing.T) {
		b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		})

		resp, body := b.get(t, "/teams/example/posts")
		if resp.StatusCode != http.StatusBadGateway || strings.Contains(body, testToken) {
			t.Errorf("got %d %q, want 502 without the token", resp.StatusCode, body)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}, func(s *Server) { s.upstream.Timeout = 100 * time.Millisecond })

		resp, body := b.get(t, "/teams/example/posts")
		if resp.StatusCode != http.StatusBadGateway || strings.Contains(body, testToken) {
			t.Errorf("got %d %q, want 502 without the token", resp.StatusCode, body)
		}
		if got := errorMessage(t, body); !strings.Contains(got, "in time") {
			t.Errorf("message = %q, want it to mention the timeout", got)
		}
	})
}

func TestForwardPassesUpstreamStatuses(t *testing.T) {
	for _, status := range []int{200, 400, 401, 403, 404, 422, 429, 500, 503} {
		b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":"from upstream"}`)
		})
		resp, body := b.get(t, "/teams/example/posts/1")
		if resp.StatusCode != status || body != `{"error":"from upstream"}` {
			t.Errorf("upstream %d: got %d %q", status, resp.StatusCode, body)
		}
	}
}

func TestConcurrencyLimit(t *testing.T) {
	entered := make(chan struct{}, maxInflight+1)
	release := make(chan struct{})
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "[]")
	})
	// Registered after the broker, so that it runs before the fake API is closed.
	releaseAll := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseAll)

	var wg sync.WaitGroup
	statuses := make(chan int, maxInflight)
	for range maxInflight {
		wg.Go(func() {
			resp, _, err := b.request(http.MethodGet, "/teams/example/posts", nil)
			if err != nil {
				statuses <- -1
				return
			}
			statuses <- resp.StatusCode
		})
	}
	for range maxInflight {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the upstream did not receive the requests in time")
		}
	}

	// All slots are taken: the next request is refused at once, not queued.
	over := &http.Client{Transport: NewTransport(b.socket), Timeout: 3 * time.Second}
	resp, err := over.Get(BaseURL + "/teams/example/posts")
	if err != nil {
		t.Fatalf("a request over the limit was not answered: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	errorMessage(t, string(body))
	// Management requests are not limited.
	if resp, _ := b.get(t, statusPath); resp.StatusCode != http.StatusOK {
		t.Errorf("status request: status = %d, want 200", resp.StatusCode)
	}

	releaseAll()
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Errorf("a request in flight ended with %d, want 200", status)
		}
	}

	// A slot is free again.
	if resp, _ := b.get(t, "/teams/example/posts"); resp.StatusCode != http.StatusOK {
		t.Errorf("after the others finished: status = %d, want 200", resp.StatusCode)
	}
	if got := len(b.upstream.seen()); got != maxInflight+1 {
		t.Errorf("the upstream saw %d requests, want %d", got, maxInflight+1)
	}
}

func TestPolicyIsEnforcedOnTheWire(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"write", http.MethodPost, "/teams/example/posts", http.StatusMethodNotAllowed},
		{"delete", http.MethodDelete, "/teams/example/posts/1", http.StatusMethodNotAllowed},
		{"other team", http.MethodGet, "/teams/other/posts", http.StatusForbidden},
		{"archive", http.MethodGet, "/teams/example/posts/1/archive", http.StatusForbidden},
		{"unknown endpoint", http.MethodGet, "/users", http.StatusForbidden},
		{"traversal", http.MethodGet, "/teams/example/posts/../groups", http.StatusForbidden},
		{"encoded traversal", http.MethodGet, "/teams/example/posts/%2e%2e/groups", http.StatusBadRequest},
		{"double slash", http.MethodGet, "/teams/example//posts", http.StatusBadRequest},
		{"bad query", http.MethodGet, "/teams/example/posts?per_page=1000", http.StatusBadRequest},
		{"unknown management path", http.MethodGet, "/-/other", http.StatusForbidden},
		{"management path with a query", http.MethodGet, "/-/status?x=1", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := b.do(t, tt.method, tt.path, nil)
			if resp.StatusCode != tt.want {
				t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, resp.StatusCode, tt.want)
			}
			errorMessage(t, body)
			if tt.want == http.StatusMethodNotAllowed && resp.Header.Get("Allow") != http.MethodGet {
				t.Errorf("Allow = %q, want GET", resp.Header.Get("Allow"))
			}
		})
	}
	if got := b.upstream.seen(); len(got) != 0 {
		t.Errorf("the upstream received %d requests for refused ones: %v", len(got), got)
	}
}

func TestManagementEndpoints(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	resp, body := b.get(t, statusPath)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("status: got %d %q, want 200 JSON", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("status body %q: %v", body, err)
	}
	if len(fields) != 2 || fields["team"] != "example" {
		t.Errorf("status body = %v, want the fields team and expires_at", fields)
	}
	expires, err := time.Parse(time.RFC3339, fields["expires_at"])
	if err != nil {
		t.Fatalf("expires_at %q is not RFC 3339: %v", fields["expires_at"], err)
	}
	if until := time.Until(expires); until < 59*time.Minute || until > time.Hour+time.Second {
		t.Errorf("expires_at is in %s, want about an hour", until)
	}
	if strings.Contains(body, testToken) {
		t.Errorf("the status contains the token: %q", body)
	}

	// Wrong methods
	if resp, _ := b.do(t, http.MethodPost, statusPath, nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST %s: status = %d, want 405", statusPath, resp.StatusCode)
	}
	if resp, _ := b.get(t, stopPath); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET %s: status = %d, want 405", stopPath, resp.StatusCode)
	}

	// None of this reaches the upstream, and the broker still runs.
	if got := b.upstream.seen(); len(got) != 0 {
		t.Errorf("the upstream received %v", got)
	}
	select {
	case <-b.stopped:
		t.Fatalf("the broker stopped: %v", b.err)
	default:
	}
}

func TestStopRequest(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	resp, _ := b.do(t, http.MethodPost, stopPath, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	if err := b.waitStopped(t); err != nil {
		t.Errorf("Serve returned %v, want nil", err)
	}
	if _, err := os.Stat(b.socket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket still exists (Stat error: %v)", err)
	}
}

func TestServeStopsWhenTheTTLExpires(t *testing.T) {
	b := newTestBroker(t, 200*time.Millisecond, nil)

	if resp, _ := b.get(t, statusPath); resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	if err := b.waitStopped(t); err != nil {
		t.Errorf("Serve returned %v, want nil", err)
	}
	if _, err := os.Stat(b.socket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket still exists (Stat error: %v)", err)
	}
	if conn, err := net.Dial("unix", b.socket); err == nil {
		_ = conn.Close()
		t.Error("the socket still accepts connections")
	}
}

func TestServeStopsWhenTheContextIsDone(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	b.cancel()
	if err := b.waitStopped(t); err != nil {
		t.Errorf("Serve returned %v, want nil", err)
	}
}

func TestPeerCheck(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)
	uid := b.server.uid

	tests := []struct {
		name string
		uid  uint32
		fail error
		want int
	}{
		{"same user", uid, nil, http.StatusOK},
		{"other user", uid + 1, nil, http.StatusForbidden},
		{"lookup failed", uid, errors.New("no credentials"), http.StatusForbidden},
	}
	if uid != 0 {
		tests = append(tests, struct {
			name string
			uid  uint32
			fail error
			want int
		}{"root", 0, nil, http.StatusForbidden})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A connection is checked when it is made, so use a new one.
			b.peer.set(tt.uid, tt.fail)
			b.client.CloseIdleConnections()

			for _, path := range []string{statusPath, "/teams/example/posts", "/teams/other/posts"} {
				resp, body := b.get(t, path)
				want := tt.want
				if path == "/teams/other/posts" && want == http.StatusOK {
					want = http.StatusForbidden
				}
				if resp.StatusCode != want {
					t.Errorf("GET %s: status = %d, want %d", path, resp.StatusCode, want)
				}
				if want == http.StatusForbidden {
					errorMessage(t, body)
				}
			}
		})
	}

	// Only the requests of the allowed user reached the upstream.
	if got := len(b.upstream.seen()); got != 1 {
		t.Errorf("the upstream saw %d requests, want 1", got)
	}
}

// Without the peer lookup of Serve there is no peer, and a request is refused.
func TestHandlerRefusesRequestsWithoutAPeer(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	rec := httptest.NewRecorder()
	b.server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/teams/example/posts", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if got := b.upstream.seen(); len(got) != 0 {
		t.Errorf("the upstream received %v", got)
	}
}

func TestErrorResponsesNeverContainTheToken(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	rec := httptest.NewRecorder()
	b.server.writeError(rec, http.StatusBadGateway, "failure with "+testToken+" inside")
	if body := rec.Body.String(); strings.Contains(body, testToken) {
		t.Errorf("the error response contains the token: %q", body)
	}
	if got := errorMessage(t, rec.Body.String()); !strings.Contains(got, "[redacted]") {
		t.Errorf("message = %q, want the token replaced", got)
	}
}

func TestUpstreamClientSettings(t *testing.T) {
	// A proxy in the environment must not see the token.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")

	client := newUpstreamClient()
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want 30s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is a %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Error("the upstream transport uses a proxy")
	}
	if !transport.DisableCompression {
		t.Error("the upstream transport asks for compression")
	}

	req, _ := http.NewRequest(http.MethodGet, "https://api.docbase.io/", nil)
	if err := client.CheckRedirect(req, []*http.Request{req}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("CheckRedirect = %v, want http.ErrUseLastResponse", err)
	}

	server, err := NewServer(Config{Team: "example", Token: testToken, Version: "1.0.0", TTL: time.Minute})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server.upstreamBase != "https://api.docbase.io" {
		t.Errorf("upstream base = %q, want https://api.docbase.io", server.upstreamBase)
	}
	if server.userAgent != "docbase-cli-broker/1.0.0" {
		t.Errorf("user agent = %q, want docbase-cli-broker/1.0.0", server.userAgent)
	}
}

func TestNewServerChecksItsConfig(t *testing.T) {
	good := Config{Team: "example", Token: testToken, TTL: time.Minute}
	if _, err := NewServer(good); err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	tests := map[string]func(*Config){
		"no team":       func(c *Config) { c.Team = "" },
		"bad team":      func(c *Config) { c.Team = "a/b" },
		"no token":      func(c *Config) { c.Token = "" },
		"bad token":     func(c *Config) { c.Token = "has space" },
		"zero ttl":      func(c *Config) { c.TTL = 0 },
		"negative ttl":  func(c *Config) { c.TTL = -time.Second },
		"team with dot": func(c *Config) { c.Team = "a.b" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := good
			mutate(&cfg)
			if _, err := NewServer(cfg); err == nil {
				t.Error("NewServer succeeded, want an error")
			}
		})
	}
}

func TestCheckTTL(t *testing.T) {
	for _, ttl := range []time.Duration{MinTTL, 30 * time.Minute, DefaultTTL, MaxTTL} {
		if err := CheckTTL(ttl); err != nil {
			t.Errorf("CheckTTL(%s) = %v, want nil", ttl, err)
		}
	}
	for _, ttl := range []time.Duration{0, -time.Minute, MinTTL - time.Second, MaxTTL + time.Second, 24 * time.Hour} {
		if err := CheckTTL(ttl); err == nil {
			t.Errorf("CheckTTL(%s) = nil, want an error", ttl)
		}
	}
}

// A listener that fails ends Serve with the error.
func TestServeReportsAListenerFailure(t *testing.T) {
	server, err := NewServer(Config{Team: "example", Token: testToken, TTL: time.Hour})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), failingListener{}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve returned nil, want the listener's error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return")
	}
}

type failingListener struct{}

func (failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (failingListener) Close() error              { return nil }
func (failingListener) Addr() net.Addr            { return &net.UnixAddr{Name: "failing", Net: "unix"} }
