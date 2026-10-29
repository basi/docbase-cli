package broker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusAndStop(t *testing.T) {
	b := newTestBroker(t, time.Hour, nil)

	info, err := Status(b.socket)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if info.Team != "example" {
		t.Errorf("Team = %q, want example", info.Team)
	}
	if until := time.Until(info.ExpiresAt); until < 59*time.Minute || until > time.Hour+time.Second {
		t.Errorf("ExpiresAt is in %s, want about an hour", until)
	}

	if err := Stop(b.socket); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := b.waitStopped(t); err != nil {
		t.Errorf("Serve returned %v, want nil", err)
	}
	if _, err := os.Stat(b.socket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket still exists (Stat error: %v)", err)
	}

	// Once stopped, the broker cannot be reached.
	if _, err := Status(b.socket); err == nil {
		t.Error("Status succeeded after Stop")
	}
	if err := Stop(b.socket); err == nil {
		t.Error("Stop succeeded after Stop")
	}
}

func TestStatusAndStopWhenUnreachable(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "missing.sock")

	_, err := Status(socket)
	if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker at "+socket) {
		t.Errorf("Status: got %v, want the unreachable error", err)
	}
	err = Stop(socket)
	if err == nil || !strings.Contains(err.Error(), "cannot reach the DocBase broker at "+socket) {
		t.Errorf("Stop: got %v, want the unreachable error", err)
	}

	// A socket nobody listens on
	stale := filepath.Join(shortTempDir(t), "stale.sock")
	listener, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if ul, ok := listener.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	_ = listener.Close()
	if _, err := Status(stale); err == nil {
		t.Error("Status succeeded on a stale socket")
	}
}

// fakeBroker serves handler on a Unix socket, to play a broker that misbehaves.
func fakeBroker(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	socket := filepath.Join(shortTempDir(t), "fake.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	return socket
}

func TestStatusRejectsBadAnswers(t *testing.T) {
	tests := []struct {
		name    string
		respond http.HandlerFunc
		want    string
	}{
		{"not JSON", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "hello") }, "unexpected answer"},
		{"no team", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"expires_at":"2030-01-01T00:00:00Z"}`)
		}, "unexpected answer"},
		{"team that is not a name", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"team":"a/../b","expires_at":"2030-01-01T00:00:00Z"}`)
		}, "unexpected answer"},
		{"bad expiry", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"team":"example","expires_at":"soon"}`)
		}, "unexpected answer"},
		{"error status", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"forbidden"}`)
		}, "403 Forbidden: forbidden"},
		{"error status without a message", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, "500 Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Status(fakeBroker(t, tt.respond))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestStopRejectsBadAnswers(t *testing.T) {
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"fine"}`) // 200, not 202
	})
	if err := Stop(socket); err == nil {
		t.Error("Stop accepted an answer that is not 202")
	}
}

func TestStatusLimitsWhatItReads(t *testing.T) {
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"team":"example","expires_at":"2030-01-01T00:00:00Z","padding":"`)
		_, _ = io.WriteString(w, strings.Repeat("x", 1<<20))
		_, _ = io.WriteString(w, `"}`)
	})
	if _, err := Status(socket); err == nil {
		t.Error("Status accepted an answer larger than the limit")
	}
}

func TestErrorDetailIsShort(t *testing.T) {
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("é", 1000)+`"}`)
	})
	_, err := Status(socket)
	if err == nil || len([]rune(err.Error())) > 400 {
		t.Errorf("got %v, want a short error", err)
	}
}

func TestTransportDialsTheSocketAndIgnoresProxies(t *testing.T) {
	// Requests for any host go to the socket, and a proxy from the environment is not used.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("http_proxy", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:9")

	var host string
	socket := fakeBroker(t, func(w http.ResponseWriter, r *http.Request) {
		host = r.Host
		_, _ = io.WriteString(w, "from the socket")
	})

	transport := NewTransport(socket)
	if tr, ok := transport.(*http.Transport); !ok || tr.Proxy != nil {
		t.Fatalf("transport = %#v, want an *http.Transport without a proxy", transport)
	}

	for _, url := range []string{BaseURL + "/x", "http://api.docbase.io/x", "http://example.invalid:8080/x"} {
		resp, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if string(body) != "from the socket" {
			t.Errorf("GET %s: body = %q, want it from the socket", url, body)
		}
	}
	if host == "" {
		t.Error("the fake broker saw no request")
	}
}
