package broker

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// dialBroker opens a connection to the broker. The connection is closed when the test ends.
func dialBroker(t *testing.T, b *testBroker) net.Conn {
	t.Helper()

	conn, err := net.Dial("unix", b.socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// statusRequest is a request for the status of the broker.
const statusRequest = "GET /-/status HTTP/1.1\r\nHost: broker\r\n\r\n"

func TestHTTPServerLimits(t *testing.T) {
	server, err := NewServer(Config{Team: "example", Token: testToken, TTL: time.Hour})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv := server.httpServer()

	for name, got := range map[string]time.Duration{
		"ReadHeaderTimeout": srv.ReadHeaderTimeout,
		"ReadTimeout":       srv.ReadTimeout,
	} {
		if got != 10*time.Second {
			t.Errorf("%s = %s, want 10s", name, got)
		}
	}
	if srv.WriteTimeout != time.Minute || srv.IdleTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %s, IdleTimeout = %s; want 1m and 30s", srv.WriteTimeout, srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 16<<10 {
		t.Errorf("MaxHeaderBytes = %d, want 16384", srv.MaxHeaderBytes)
	}
	if srv.ConnContext == nil {
		t.Error("ConnContext is not set: the peer would not be looked up")
	}
	if server.maxConns != 16 {
		t.Errorf("maxConns = %d, want 16", server.maxConns)
	}
}

// A request that announces a body it never sends must be answered, and its
// connection closed, without waiting for the body. net/http would wait for it,
// before the answer and again after it, until the read timeout (10 seconds).
func TestAnnouncedBodyDoesNotHoldTheConnection(t *testing.T) {
	tests := []struct {
		name    string
		request string
		want    int
	}{
		{"write", "POST /teams/example/posts HTTP/1.1\r\nHost: x\r\nContent-Length: 100\r\n\r\n", http.StatusMethodNotAllowed},
		{"read", "GET /teams/example/posts HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\n", http.StatusBadRequest},
		{"read with a large body", "GET /teams/example/posts HTTP/1.1\r\nHost: x\r\nContent-Length: 100000\r\n\r\n", http.StatusBadRequest},
		{"chunked", "GET /teams/example/posts HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n", http.StatusBadRequest},
		{"another team", "GET /teams/other/posts HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\n", http.StatusBadRequest},
		{"status", "GET /-/status HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\n", http.StatusOK},
	}
	b := newTestBroker(t, time.Hour, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := dialBroker(t, b)

			started := time.Now()
			_ = conn.SetDeadline(started.Add(3 * time.Second))
			if _, err := io.WriteString(conn, tt.request); err != nil {
				t.Fatalf("write: %v", err)
			}
			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, nil)
			if err != nil {
				t.Fatalf("no answer to a request with an unsent body: %v", err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.want)
			}
			if !resp.Close {
				t.Error("the answer does not announce that the connection is closed")
			}

			// The server closes the connection: nothing more can be read.
			if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
				t.Errorf("after the answer, reading gave %v, want the end of the connection", err)
			}
			elapsed := time.Since(started)
			t.Logf("answered and closed after %s", elapsed)
			if elapsed > 2*time.Second {
				t.Errorf("it took %s", elapsed)
			}
			if got := b.upstream.seen(); len(got) != 0 {
				t.Errorf("the upstream saw %v", got)
			}
		})
	}

	t.Run("refused peer", func(t *testing.T) {
		b.peer.set(b.server.uid+1, nil)
		t.Cleanup(func() { b.peer.set(b.server.uid, nil) })
		conn := dialBroker(t, b)

		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.WriteString(conn, "POST /teams/example/posts HTTP/1.1\r\nHost: x\r\nContent-Length: 100\r\n\r\n")
		reader := bufio.NewReader(conn)
		resp, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("no answer: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusForbidden || !resp.Close {
			t.Errorf("got %d (close = %v), want 403 and a closed connection", resp.StatusCode, resp.Close)
		}
		if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
			t.Errorf("after the answer, reading gave %v, want the end of the connection", err)
		}
	})
}

// A refused request is not worth a connection; an answer of the API is.
func TestRefusalsCloseTheConnectionButAnswersOfTheAPIDoNot(t *testing.T) {
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not_found"}`)
	})

	for _, path := range []string{"/teams/other/posts", "/teams/example/posts?page=0", "/teams/example/posts/1/archive"} {
		if resp, _ := b.get(t, path); !resp.Close || resp.StatusCode < 400 || resp.StatusCode >= 500 {
			t.Errorf("GET %s: status %d, close = %v; want a 4xx answer that closes the connection", path, resp.StatusCode, resp.Close)
		}
	}
	if resp, _ := b.do(t, http.MethodPost, "/teams/example/posts", nil); !resp.Close {
		t.Error("POST: the connection is kept")
	}

	resp, _ := b.get(t, "/teams/example/posts/404")
	if resp.StatusCode != http.StatusNotFound || resp.Close {
		t.Errorf("an answer of the API: status %d, close = %v; want 404 and a connection that is kept", resp.StatusCode, resp.Close)
	}
}

func TestConnectionLimit(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil, func(s *Server) { s.maxConns = 2 })

	// Two connections take both places. A request is answered on each, so
	// that both are certainly accepted.
	var first net.Conn
	for i := range 2 {
		conn := dialBroker(t, b)
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.WriteString(conn, statusRequest); err != nil {
			t.Fatalf("write: %v", err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("connection %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		if i == 0 {
			first = conn
		}
	}

	// A third one waits: the kernel takes the connection, but the broker does not serve it.
	third := dialBroker(t, b)
	if _, err := io.WriteString(third, statusRequest); err != nil {
		t.Fatalf("write: %v", err)
	}
	reader := bufio.NewReader(third)
	_ = third.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := http.ReadResponse(reader, nil); err == nil {
		t.Fatal("a third connection was served although two were open")
	}

	// When a connection is closed, the next one is served.
	_ = first.Close()
	_ = third.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("the waiting connection was not served after a place became free: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// However slowly a client reads, no more than four answers are held in memory.
func TestSlotIsHeldUntilTheAnswerHasBeenWritten(t *testing.T) {
	big := strings.Repeat("x", 4<<20)
	entered := make(chan struct{}, maxInflight+2)
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, big)
	})

	// Four clients ask and then do not read.
	var conns []net.Conn
	for range maxInflight {
		conn := dialBroker(t, b)
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
		if _, err := io.WriteString(conn, "GET /teams/example/posts HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		conns = append(conns, conn)
	}
	for range maxInflight {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the upstream did not receive the requests in time")
		}
	}
	// Each has begun to receive its answer, so the broker is writing it. It cannot
	// finish: the client does not read the rest of the 4 MiB.
	for _, conn := range conns {
		if _, err := conn.Read(make([]byte, 1)); err != nil {
			t.Fatalf("no answer began: %v", err)
		}
	}

	// All four answers are held, so there is no room for a fifth request.
	resp, body := b.get(t, "/teams/example/groups")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 while four answers are still being written", resp.StatusCode)
	}
	errorMessage(t, body)

	// When the clients give up, the broker lets go of the answers and the places.
	for _, conn := range conns {
		_ = conn.Close()
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, _ := b.get(t, "/teams/example/groups")
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the places were not given back: status %d", resp.StatusCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Whatever the upstream does with the token it receives, the client does not get it back.
func TestReflectedTokenNeverReachesTheClient(t *testing.T) {
	b := newTestBroker(t, time.Hour, func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-DocBaseToken")
		w.Header().Set("Content-Type", "application/json; reflected="+token)
		w.Header().Set("Retry-After", token)
		w.Header().Set("X-RateLimit-Limit", "limit-"+token)
		w.Header().Set("X-RateLimit-Remaining", token+token)
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"invalid token %s","messages":["%s","%s"]}`, token, token, token)
	})

	resp, body := b.get(t, "/teams/example/posts")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want the 400 of the upstream", resp.StatusCode)
	}

	if strings.Contains(body, testToken) {
		t.Errorf("the body contains the token: %q", body)
	}
	var payload struct {
		Error    string   `json:"error"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("the body is not valid JSON any more: %v\n%s", err, body)
	}
	if payload.Error != "invalid token [redacted]" || len(payload.Messages) != 2 || payload.Messages[0] != "[redacted]" {
		t.Errorf("payload = %+v, want the token replaced by [redacted]", payload)
	}

	for name, values := range resp.Header {
		for _, value := range values {
			if strings.Contains(value, testToken) {
				t.Errorf("header %s = %q contains the token", name, value)
			}
		}
	}
	if got := resp.Header.Get("Retry-After"); got != "[redacted]" {
		t.Errorf("Retry-After = %q, want [redacted]", got)
	}
	if got := resp.Header.Get("X-Ratelimit-Remaining"); got != "[redacted][redacted]" {
		t.Errorf("X-RateLimit-Remaining = %q, want the token replaced twice", got)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json; reflected=[redacted]" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestRedactBytes(t *testing.T) {
	body := []byte(`{"a":"` + testToken + `","b":"` + testToken + `"}`)
	if got := string(redactBytes(body, testToken)); got != `{"a":"[redacted]","b":"[redacted]"}` {
		t.Errorf("redactBytes = %q", got)
	}

	// A body without the token is returned as it is, not copied.
	clean := []byte(strings.Repeat("y", 1000))
	if got := redactBytes(clean, testToken); &got[0] != &clean[0] {
		t.Error("a body without the token was copied")
	}
	if got := redactBytes(clean, ""); &got[0] != &clean[0] {
		t.Error("an empty token changed the body")
	}
	if got := redact("abc", ""); got != "abc" {
		t.Errorf("redact with an empty token = %q", got)
	}
}
