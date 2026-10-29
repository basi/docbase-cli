package broker

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// useValidationAPI points ValidateToken at the fake API for the test. It sets the
// version that goes into the User-Agent header as well.
func useValidationAPI(t *testing.T, upstream *fakeUpstream) {
	t.Helper()

	originalAPI, originalVersion := validationAPI, Version
	validationAPI = func() (string, *http.Client) {
		// The same settings as the client of the broker has.
		return upstream.URL, hardenUpstream(upstream.Client())
	}
	Version = "9.8.7"
	t.Cleanup(func() {
		validationAPI, Version = originalAPI, originalVersion
	})
}

func TestValidateToken(t *testing.T) {
	upstream := newFakeUpstream(t, nil)
	useValidationAPI(t, upstream)

	if err := ValidateToken("example", testToken); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	seen := upstream.seen()
	if len(seen) != 1 {
		t.Fatalf("the API saw %d requests, want 1", len(seen))
	}
	if got := seen[0]; got.Method != http.MethodGet || got.URI != "/teams/example/groups?per_page=1" {
		t.Errorf("request = %s %s, want GET /teams/example/groups?per_page=1", got.Method, got.URI)
	}

	// Exactly the four headers of every upstream request of the broker.
	want := map[string]string{
		"X-Docbasetoken": testToken,
		"X-Api-Version":  "3",
		"Accept":         "application/json",
		"User-Agent":     "docbase-cli-broker/9.8.7",
	}
	header := seen[0].Header
	if len(header) != len(want) {
		t.Errorf("the API saw the headers %v, want exactly %v", header, want)
	}
	for name, value := range want {
		if got := header.Values(name); len(got) != 1 || got[0] != value {
			t.Errorf("header %s = %v, want [%s]", name, got, value)
		}
	}
}

// The request that ValidateToken sends is one that the broker itself would forward.
func TestValidateTokenSendsARequestThePolicyAllows(t *testing.T) {
	upstream := newFakeUpstream(t, nil)
	useValidationAPI(t, upstream)
	if err := ValidateToken("Example", testToken); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	got, err := authorize("Example", httptest.NewRequest(http.MethodGet, upstream.seen()[0].URI, nil))
	if err != nil {
		t.Fatalf("the policy refuses the validation request: %v", err)
	}
	if got.String() != upstream.seen()[0].URI {
		t.Errorf("the policy turns %q into %q", upstream.seen()[0].URI, got)
	}
}

func TestValidateTokenStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string // "" for success; else a part of the error
	}{
		{"ok", http.StatusOK, `[]`, ""},
		{"no content", http.StatusNoContent, ``, ""},
		{"unauthorized", http.StatusUnauthorized, `{"error":"unauthorized"}`, "the access token was rejected by the DocBase API (401 Unauthorized)"},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, "the access token was rejected by the DocBase API (403 Forbidden)"},
		{"unknown team", http.StatusNotFound, `{"error":"not_found"}`, "404 Not Found: check the team domain"},
		{"rate limited", http.StatusTooManyRequests, `{"error":"rate_limit"}`, "the DocBase API answered 429 Too Many Requests: rate_limit"},
		{"server error", http.StatusInternalServerError, `{"error":"internal"}`, "the DocBase API answered 500 Internal Server Error: internal"},
		{"unavailable without a body", http.StatusServiceUnavailable, ``, "the DocBase API answered 503 Service Unavailable"},
		{"error that repeats the token", http.StatusBadRequest, `{"error":"bad request with ` + testToken + `"}`, "400 Bad Request: bad request with [redacted]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			useValidationAPI(t, upstream)

			err := ValidateToken("example", testToken)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateToken = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateToken = %v, want an error containing %q", err, tt.want)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Errorf("the error contains the token: %v", err)
			}
		})
	}
}

// Following a redirect would send the token to wherever the redirect points.
func TestValidateTokenDoesNotFollowRedirects(t *testing.T) {
	elsewhere := newFakeUpstream(t, nil)

	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, elsewhere.URL+"/teams/example/groups", status)
		})
		useValidationAPI(t, upstream)

		err := ValidateToken("example", testToken)
		if err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Errorf("redirect %d: ValidateToken = %v, want the redirect error", status, err)
		}
		if err != nil && (strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), elsewhere.URL)) {
			t.Errorf("redirect %d: unexpected error text %q", status, err)
		}
	}
	if got := elsewhere.seen(); len(got) != 0 {
		t.Errorf("the redirect target received %d requests, want none", len(got))
	}
}

func TestValidateTokenErrorsWithoutAnAnswer(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		upstream := newFakeUpstream(t, nil)
		useValidationAPI(t, upstream)
		upstream.Close()

		err := ValidateToken("example", testToken)
		if err == nil || !strings.Contains(err.Error(), "the DocBase API could not be reached") {
			t.Fatalf("ValidateToken = %v, want the unreachable error", err)
		}
		if strings.Contains(err.Error(), testToken) {
			t.Errorf("the error contains the token: %v", err)
		}
	})

	t.Run("certificate that is not trusted", func(t *testing.T) {
		upstream := newFakeUpstream(t, nil)
		originalAPI := validationAPI
		t.Cleanup(func() { validationAPI = originalAPI })
		// The client of the broker with the system roots, which do not know the test server.
		validationAPI = func() (string, *http.Client) { return upstream.URL, newUpstreamClient() }

		err := ValidateToken("example", testToken)
		if err == nil || !strings.Contains(err.Error(), "could not be reached") || strings.Contains(err.Error(), testToken) {
			t.Errorf("ValidateToken = %v, want an error about the connection without the token", err)
		}
		if got := upstream.seen(); len(got) != 0 {
			t.Errorf("a request got through a certificate that is not trusted: %v", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		})
		originalAPI := validationAPI
		t.Cleanup(func() { validationAPI = originalAPI })
		validationAPI = func() (string, *http.Client) {
			client := hardenUpstream(upstream.Client())
			client.Timeout = 100 * time.Millisecond
			return upstream.URL, client
		}

		err := ValidateToken("example", testToken)
		if err == nil || !strings.Contains(err.Error(), "did not answer in time") {
			t.Errorf("ValidateToken = %v, want the timeout error", err)
		}
	})
}

func TestValidateTokenChecksItsArguments(t *testing.T) {
	upstream := newFakeUpstream(t, nil)
	useValidationAPI(t, upstream)

	for _, tt := range []struct{ team, token string }{
		{"", testToken},
		{"a/b", testToken},
		{"example?x=1", testToken},
		{"example", ""},
		{"example", "has space"},
		{"example", testToken + "\n"},
	} {
		err := ValidateToken(tt.team, tt.token)
		if err == nil {
			t.Errorf("ValidateToken(%q, ...) = nil, want an error", tt.team)
			continue
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("the error contains the token: %v", err)
		}
	}
	if got := upstream.seen(); len(got) != 0 {
		t.Errorf("the API saw %d requests for arguments that are not valid", len(got))
	}
}

// The default is the client of the broker and the real API, and nothing can
// change that from outside the process.
func TestValidateTokenUsesTheHardenedClientAndTheFixedBase(t *testing.T) {
	// A proxy in the environment must not see the token.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:9")

	base, client := validationAPI()
	if base != "https://api.docbase.io" {
		t.Errorf("base = %q, want https://api.docbase.io", base)
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want 30s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is a %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Error("the validation transport uses a proxy")
	}
	if !transport.DisableCompression {
		t.Error("the validation transport asks for compression")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.docbase.io/", nil)
	if err := client.CheckRedirect(req, []*http.Request{req}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("CheckRedirect = %v, want http.ErrUseLastResponse", err)
	}
}
