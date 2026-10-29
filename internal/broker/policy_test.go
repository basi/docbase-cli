package broker

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	return httptest.NewRequest(method, target, nil)
}

func TestAuthorizeAllowed(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string // path and query sent upstream
	}{
		{"posts", "/teams/example/posts", "/teams/example/posts"},
		{"posts with every key", "/teams/example/posts?q=tag%3Aweekly&page=2&per_page=100", "/teams/example/posts?page=2&per_page=100&q=tag%3Aweekly"},
		{"posts with an empty search", "/teams/example/posts?q=", "/teams/example/posts?q="},
		{"posts with a UTF-8 search", "/teams/example/posts?q=%E9%80%B1%E5%A0%B1", "/teams/example/posts?q=%E9%80%B1%E5%A0%B1"},
		{"posts with a plus in the search", "/teams/example/posts?q=a+b", "/teams/example/posts?q=a+b"},
		{"posts search at the size limit", "/teams/example/posts?q=" + strings.Repeat("a", 1024), "/teams/example/posts?q=" + strings.Repeat("a", 1024)},
		{"upper-case team", "/teams/EXAMPLE/posts", "/teams/example/posts"},
		{"mixed-case team", "/teams/ExAmPlE/groups?page=1", "/teams/example/groups?page=1"},
		{"post", "/teams/example/posts/12345", "/teams/example/posts/12345"},
		{"post with the largest id", "/teams/example/posts/999999999999", "/teams/example/posts/999999999999"},
		{"post with the smallest id", "/teams/example/posts/1", "/teams/example/posts/1"},
		{"comments", "/teams/example/posts/7/comments", "/teams/example/posts/7/comments"},
		{"comments with paging", "/teams/example/posts/7/comments?per_page=20&page=3", "/teams/example/posts/7/comments?page=3&per_page=20"},
		{"groups", "/teams/example/groups", "/teams/example/groups"},
		{"groups with paging", "/teams/example/groups?page=1&per_page=1", "/teams/example/groups?page=1&per_page=1"},
		{"group", "/teams/example/groups/42", "/teams/example/groups/42"},
		{"tags", "/teams/example/tags", "/teams/example/tags"},
		{"tags with a search", "/teams/example/tags?q=weekly&page=1&per_page=50", "/teams/example/tags?page=1&per_page=50&q=weekly"},
		{"highest page", "/teams/example/tags?page=10000", "/teams/example/tags?page=10000"},
		{"empty query parts are dropped", "/teams/example/tags?&page=1&", "/teams/example/tags?page=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authorize("example", newRequest(t, http.MethodGet, tt.target))
			if err != nil {
				t.Fatalf("authorize(%q) = %v, want it allowed", tt.target, err)
			}
			if got.String() != tt.want {
				t.Errorf("authorize(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestAuthorizeRejected(t *testing.T) {
	longQuery := "/teams/example/posts?q=" + strings.Repeat("a", 1025)

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		// Methods
		{"POST", http.MethodPost, "/teams/example/posts", http.StatusMethodNotAllowed},
		{"PUT", http.MethodPut, "/teams/example/posts/1", http.StatusMethodNotAllowed},
		{"DELETE", http.MethodDelete, "/teams/example/posts/1", http.StatusMethodNotAllowed},
		{"PATCH", http.MethodPatch, "/teams/example/posts/1", http.StatusMethodNotAllowed},
		{"HEAD", http.MethodHead, "/teams/example/posts", http.StatusMethodNotAllowed},
		{"OPTIONS", http.MethodOptions, "/teams/example/posts", http.StatusMethodNotAllowed},

		// Another team
		{"other team", http.MethodGet, "/teams/other/posts", http.StatusForbidden},
		{"prefix of the team", http.MethodGet, "/teams/exampl/posts", http.StatusForbidden},
		{"extension of the team", http.MethodGet, "/teams/example2/posts", http.StatusForbidden},

		// Characters that must not be normalized
		{"empty segment", http.MethodGet, "/teams/example//posts", http.StatusBadRequest},
		{"empty segment before the team", http.MethodGet, "/teams//example/posts", http.StatusBadRequest},
		{"double slash at the start", http.MethodGet, "//teams/example/posts", http.StatusBadRequest},
		{"dot dot segment", http.MethodGet, "/teams/example/posts/../groups", http.StatusForbidden},
		{"dot segment", http.MethodGet, "/teams/example/./posts", http.StatusForbidden},
		{"encoded dot dot", http.MethodGet, "/teams/example/posts/%2e%2e/groups", http.StatusBadRequest},
		{"encoded upper-case dot dot", http.MethodGet, "/teams/example/posts/%2E%2E", http.StatusBadRequest},
		{"encoded slash", http.MethodGet, "/teams/example%2Fposts", http.StatusBadRequest},
		{"encoded slash in an id", http.MethodGet, "/teams/example/posts%2F1", http.StatusBadRequest},
		{"encoded letter", http.MethodGet, "/teams/example/p%6Fsts", http.StatusBadRequest},
		{"encoded digit", http.MethodGet, "/teams/example/posts/%31", http.StatusBadRequest},
		{"encoded space", http.MethodGet, "/teams/example/posts%20", http.StatusBadRequest},
		{"encoded percent", http.MethodGet, "/teams/example/posts/%25", http.StatusBadRequest},
		{"backslash", http.MethodGet, `/teams/example/posts\1`, http.StatusBadRequest},
		{"backslash separator", http.MethodGet, `/teams\example/posts`, http.StatusBadRequest},
		{"non-ASCII letter", http.MethodGet, "/teams/example/pösts", http.StatusBadRequest},
		{"absolute form", http.MethodGet, "http://docbase-broker/teams/example/posts", http.StatusBadRequest},

		// Ids
		{"id zero", http.MethodGet, "/teams/example/posts/0", http.StatusForbidden},
		{"id with a leading zero", http.MethodGet, "/teams/example/posts/012", http.StatusForbidden},
		{"id with 13 digits", http.MethodGet, "/teams/example/posts/1000000000000", http.StatusForbidden},
		{"id with a letter", http.MethodGet, "/teams/example/posts/12a", http.StatusForbidden},
		{"negative id", http.MethodGet, "/teams/example/posts/-1", http.StatusForbidden},
		{"signed id", http.MethodGet, "/teams/example/posts/+1", http.StatusForbidden},
		{"decimal id", http.MethodGet, "/teams/example/posts/1.5", http.StatusForbidden},
		{"hexadecimal id", http.MethodGet, "/teams/example/posts/0x10", http.StatusForbidden},
		{"empty id", http.MethodGet, "/teams/example/posts/", http.StatusForbidden},
		{"group id zero", http.MethodGet, "/teams/example/groups/0", http.StatusForbidden},
		{"comments of a bad id", http.MethodGet, "/teams/example/posts/0/comments", http.StatusForbidden},

		// Endpoints that are not allowed
		{"archive", http.MethodGet, "/teams/example/posts/1/archive", http.StatusForbidden},
		{"unarchive", http.MethodGet, "/teams/example/posts/1/unarchive", http.StatusForbidden},
		{"comment by id", http.MethodGet, "/teams/example/comments/1", http.StatusForbidden},
		{"comment of a post by id", http.MethodGet, "/teams/example/posts/1/comments/2", http.StatusForbidden},
		{"tag by name", http.MethodGet, "/teams/example/tags/weekly", http.StatusForbidden},
		{"users", http.MethodGet, "/users", http.StatusForbidden},
		{"team users", http.MethodGet, "/teams/example/users", http.StatusForbidden},
		{"team itself", http.MethodGet, "/teams/example", http.StatusForbidden},
		{"teams", http.MethodGet, "/teams", http.StatusForbidden},
		{"root", http.MethodGet, "/", http.StatusForbidden},
		{"trailing slash", http.MethodGet, "/teams/example/posts/", http.StatusForbidden},
		{"upper-case segment name", http.MethodGet, "/teams/example/Posts", http.StatusForbidden},
		{"upper-case teams", http.MethodGet, "/Teams/example/posts", http.StatusForbidden},
		{"group members", http.MethodGet, "/teams/example/groups/1/users", http.StatusForbidden},
		{"management path with a suffix", http.MethodGet, "/-/status/x", http.StatusForbidden},

		// Queries
		{"unknown key", http.MethodGet, "/teams/example/posts?foo=1", http.StatusBadRequest},
		{"unknown key next to a known one", http.MethodGet, "/teams/example/posts?page=1&foo=1", http.StatusBadRequest},
		{"key of another endpoint", http.MethodGet, "/teams/example/groups?q=x", http.StatusBadRequest},
		{"comments take no search", http.MethodGet, "/teams/example/posts/1/comments?q=x", http.StatusBadRequest},
		{"upper-case key", http.MethodGet, "/teams/example/posts?Page=1", http.StatusBadRequest},
		{"duplicate page", http.MethodGet, "/teams/example/posts?page=1&page=2", http.StatusBadRequest},
		{"duplicate search", http.MethodGet, "/teams/example/posts?q=a&q=b", http.StatusBadRequest},
		{"page zero", http.MethodGet, "/teams/example/posts?page=0", http.StatusBadRequest},
		{"page above the limit", http.MethodGet, "/teams/example/posts?page=10001", http.StatusBadRequest},
		{"page with many digits", http.MethodGet, "/teams/example/posts?page=100000", http.StatusBadRequest},
		{"negative page", http.MethodGet, "/teams/example/posts?page=-1", http.StatusBadRequest},
		{"signed page", http.MethodGet, "/teams/example/posts?page=%2B1", http.StatusBadRequest},
		{"page with a leading zero", http.MethodGet, "/teams/example/posts?page=01", http.StatusBadRequest},
		{"decimal page", http.MethodGet, "/teams/example/posts?page=1.0", http.StatusBadRequest},
		{"page that is not a number", http.MethodGet, "/teams/example/posts?page=abc", http.StatusBadRequest},
		{"empty page", http.MethodGet, "/teams/example/posts?page=", http.StatusBadRequest},
		{"page without a value", http.MethodGet, "/teams/example/posts?page", http.StatusBadRequest},
		{"per_page zero", http.MethodGet, "/teams/example/posts?per_page=0", http.StatusBadRequest},
		{"per_page above the limit", http.MethodGet, "/teams/example/posts?per_page=101", http.StatusBadRequest},
		{"per_page of the old group lookup", http.MethodGet, "/teams/example/groups?per_page=200", http.StatusBadRequest},
		{"search above the limit", http.MethodGet, longQuery, http.StatusBadRequest},
		{"search that is not UTF-8", http.MethodGet, "/teams/example/posts?q=%ff", http.StatusBadRequest},
		{"bad escape in the query", http.MethodGet, "/teams/example/posts?q=%zz", http.StatusBadRequest},
		{"semicolon in the query", http.MethodGet, "/teams/example/posts?page=1;per_page=2", http.StatusBadRequest},
		{"query on a post", http.MethodGet, "/teams/example/posts/1?page=1", http.StatusBadRequest},
		{"query on a group", http.MethodGet, "/teams/example/groups/1?x=1", http.StatusBadRequest},
		{"empty key", http.MethodGet, "/teams/example/posts?=1", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authorize("example", newRequest(t, tt.method, tt.target))
			if err == nil {
				t.Fatalf("authorize(%s %q) = %q, want it rejected with %d", tt.method, tt.target, got, tt.want)
			}
			var rej *rejection
			if !errors.As(err, &rej) {
				t.Fatalf("authorize(%s %q): error %v is not a rejection", tt.method, tt.target, err)
			}
			if rej.status != tt.want {
				t.Errorf("authorize(%s %q): status = %d (%s), want %d", tt.method, tt.target, rej.status, rej.message, tt.want)
			}
			if got != (target{}) {
				t.Errorf("authorize(%s %q) returned %q along with the rejection", tt.method, tt.target, got)
			}
		})
	}
}

func TestAuthorizeRejectsABody(t *testing.T) {
	t.Run("with a length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/teams/example/posts", strings.NewReader("x"))
		if _, err := authorize("example", req); err == nil {
			t.Error("a GET with a body was allowed")
		}
	})
	t.Run("chunked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/teams/example/posts", nil)
		req.TransferEncoding = []string{"chunked"}
		if _, err := authorize("example", req); err == nil {
			t.Error("a chunked GET was allowed")
		}
	})
	t.Run("unknown length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/teams/example/posts", nil)
		req.ContentLength = -1
		if _, err := authorize("example", req); err == nil {
			t.Error("a GET of unknown length was allowed")
		}
	})
	t.Run("the status is 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/teams/example/posts", strings.NewReader("x"))
		_, err := authorize("example", req)
		var rej *rejection
		if !errors.As(err, &rej) || rej.status != http.StatusBadRequest {
			t.Errorf("got %v, want a 400 rejection", err)
		}
	})
}

// A request not made by net/http's server may lack the request target.
func TestAuthorizeNeedsTheRequestTarget(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://docbase-broker/teams/example/posts", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := authorize("example", req); err == nil {
		t.Error("a request without RequestURI was allowed")
	}

	// The path found by the URL parser must be the path of the target.
	req = httptest.NewRequest(http.MethodGet, "/teams/example/posts", nil)
	req.URL.Path = "/teams/example/groups"
	if _, err := authorize("example", req); err == nil {
		t.Error("a request whose URL differs from its target was allowed")
	}
}

func TestAuthorizeUsesThePinnedTeam(t *testing.T) {
	got, err := authorize("Example", newRequest(t, http.MethodGet, "/teams/EXAMPLE/posts"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	// The team in the upstream path is the pinned one, not the one the client wrote.
	if got.path != "/teams/Example/posts" {
		t.Errorf("path = %q, want %q", got.path, "/teams/Example/posts")
	}
}

func TestValidTeam(t *testing.T) {
	valid := []string{"example", "Example", "a", "my-team", "team1", "1team", strings.Repeat("a", 63)}
	for _, team := range valid {
		if err := ValidTeam(team); err != nil {
			t.Errorf("ValidTeam(%q) = %v, want it valid", team, err)
		}
	}

	invalid := []string{"", "-team", "my team", "my_team", "my.team", "a/b", "a?b", "a#b", "a%2fb", "te@m", "tëam", "a\nb", strings.Repeat("a", 64)}
	for _, team := range invalid {
		if err := ValidTeam(team); err == nil {
			t.Errorf("ValidTeam(%q) = nil, want an error", team)
		}
	}
}

func TestValidToken(t *testing.T) {
	valid := []string{"a", "abcDEF0123456789", "tok-en_with.symbols~!", strings.Repeat("x", 4096)}
	for _, token := range valid {
		if err := ValidToken(token); err != nil {
			t.Errorf("ValidToken(%q) = %v, want it valid", token, err)
		}
	}

	invalid := []string{"", "a b", " a", "a ", "a\n", "a\r\nb", "a\tb", "a\x00b", "a\x7fb", "tökén", strings.Repeat("x", 4097)}
	for _, token := range invalid {
		if err := ValidToken(token); err == nil {
			t.Errorf("ValidToken(%q) = nil, want an error", token)
		}
	}

	// The error never repeats the token.
	for _, token := range []string{"s3cretvalue with space", "s3cretvalue\nwith newline", strings.Repeat("s3cretvalue", 500)} {
		if err := ValidToken(token); err == nil || strings.Contains(err.Error(), "s3cretvalue") {
			t.Errorf("ValidToken(%.20q...) = %v, want an error without the token", token, err)
		}
	}
}

func TestValidCount(t *testing.T) {
	tests := []struct {
		s     string
		limit int
		want  bool
	}{
		{"1", 100, true}, {"100", 100, true}, {"99", 100, true},
		{"101", 100, false}, {"1000", 100, false}, {"0", 100, false}, {"00", 100, false}, {"01", 100, false},
		{"", 100, false}, {"-1", 100, false}, {"+1", 100, false}, {"1 ", 100, false}, {"1e1", 100, false},
		{"10000", 10000, true}, {"10001", 10000, false}, {"99999", 10000, false}, {"100000", 10000, false},
	}
	for _, tt := range tests {
		if got := validCount(tt.s, tt.limit); got != tt.want {
			t.Errorf("validCount(%q, %d) = %v, want %v", tt.s, tt.limit, got, tt.want)
		}
	}
}
