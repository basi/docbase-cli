package docbase

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientTokenHeader(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantToken bool
	}{
		{name: "token", token: "s3cret-token", wantToken: true},
		{name: "empty token sends no header", token: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				_, _ = io.WriteString(w, "[]")
			}))
			t.Cleanup(server.Close)

			api := NewAPI("example", tt.token)
			api.SetBaseURL(server.URL)
			if _, err := api.Group.List(1, 1); err != nil {
				t.Fatalf("Group.List: %v", err)
			}

			values, present := got["X-Docbasetoken"]
			if present != tt.wantToken {
				t.Fatalf("X-DocBaseToken present = %v, want %v (headers: %v)", present, tt.wantToken, got)
			}
			if tt.wantToken && (len(values) != 1 || values[0] != tt.token) {
				t.Errorf("X-DocBaseToken = %v, want [%s]", values, tt.token)
			}
			if got.Get("X-Api-Version") != APIVersion {
				t.Errorf("X-Api-Version = %q, want %q", got.Get("X-Api-Version"), APIVersion)
			}
		})
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSetTransport(t *testing.T) {
	var seen *http.Request
	api := NewAPI("example", "")
	api.SetBaseURL("http://docbase-broker")
	api.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader("[]")),
			Request:    r,
		}, nil
	}))

	if _, err := api.Group.List(2, 50); err != nil {
		t.Fatalf("Group.List: %v", err)
	}
	if seen == nil {
		t.Fatal("the transport was not used")
	}
	if got, want := seen.URL.String(), "http://docbase-broker/teams/example/groups?page=2&per_page=50"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}
