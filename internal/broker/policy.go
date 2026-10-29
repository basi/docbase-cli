package broker

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// maxPage and maxPerPage bound the paging parameters.
	maxPage    = 10000
	maxPerPage = 100
	// maxQueryLength bounds the search text in bytes.
	maxQueryLength = 1024
	// maxTeamLength is the longest team domain: a DNS label.
	maxTeamLength = 63
	// maxIDLength is the most digits of a post or group id.
	maxIDLength = 12
)

// rejection is a request the policy refuses: the status to answer with and why.
type rejection struct {
	status  int
	message string
}

func (r *rejection) Error() string { return r.message }

func reject(status int, format string, args ...any) *rejection {
	return &rejection{status: status, message: fmt.Sprintf(format, args...)}
}

// target is a request the policy allows, as the broker sends it upstream: the
// path below the API base and the encoded query (without the '?').
type target struct {
	path  string
	query string
}

// String returns the path with the query, ready to be appended to the API base.
func (t target) String() string {
	if t.query == "" {
		return t.path
	}

	return t.path + "?" + t.query
}

// ValidTeam checks that a team domain is safe to put in a URL path: ASCII letters,
// digits and hyphens, not starting with a hyphen, at most 63 bytes.
func ValidTeam(team string) error {
	if team == "" {
		return errors.New("team domain is required")
	}
	if len(team) > maxTeamLength {
		return fmt.Errorf("team domain is longer than %d characters", maxTeamLength)
	}
	for i := range len(team) {
		c := team[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' && i > 0:
		default:
			return fmt.Errorf("team domain %q may only contain letters, digits and hyphens, and cannot start with a hyphen", team)
		}
	}

	return nil
}

// authorize checks a client request against the policy. It returns the request to
// send upstream, built from the pinned team and the validated parts of the request
// only, or a *rejection. Nothing is normalized: a request that does not match
// exactly is refused.
func authorize(team string, r *http.Request) (target, error) {
	if r.Method != http.MethodGet {
		return target{}, reject(http.StatusMethodNotAllowed, "only GET requests are allowed")
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		return target{}, reject(http.StatusBadRequest, "a request body is not allowed")
	}

	path, ok := plainPath(r)
	if !ok {
		return target{}, reject(http.StatusBadRequest, "the request path must not contain encoded or unusual characters")
	}

	suffix, keys, err := matchPath(team, path)
	if err != nil {
		return target{}, err
	}

	query, err := checkQuery(r.URL.RawQuery, keys)
	if err != nil {
		return target{}, err
	}

	return target{path: "/teams/" + team + suffix, query: query}, nil
}

// plainPath returns the path of the request as the client sent it, and false when
// that path is anything but plain: no percent-encoding, backslash or empty
// segment, and the same path that the URL parser found.
func plainPath(r *http.Request) (string, bool) {
	// RequestURI is the request target as sent. Only the origin form is accepted.
	if !strings.HasPrefix(r.RequestURI, "/") || r.URL == nil || r.URL.RawPath != "" {
		return "", false
	}
	path, _, _ := strings.Cut(r.RequestURI, "?")
	if r.URL.Path != path || strings.ContainsAny(path, "%\\") || strings.Contains(path, "//") {
		return "", false
	}

	return path, true
}

// matchPath matches the path against the allowed endpoints of the pinned team. It
// returns the path below the team and the query keys the endpoint accepts.
func matchPath(team, path string) (suffix string, keys []string, err error) {
	segments := strings.Split(path[1:], "/")
	if len(segments) < 3 || segments[0] != "teams" {
		return "", nil, errNotAllowed
	}
	if !strings.EqualFold(segments[1], team) {
		return "", nil, reject(http.StatusForbidden, "this broker serves team %q only", team)
	}

	collection, rest := segments[2], segments[3:]
	switch {
	case len(rest) == 0 && collection == "posts":
		return "/posts", []string{"q", "page", "per_page"}, nil
	case len(rest) == 0 && collection == "groups":
		return "/groups", []string{"page", "per_page"}, nil
	case len(rest) == 0 && collection == "tags":
		return "/tags", []string{"q", "page", "per_page"}, nil
	case len(rest) == 1 && (collection == "posts" || collection == "groups") && validID(rest[0]):
		return "/" + collection + "/" + rest[0], nil, nil
	case len(rest) == 2 && collection == "posts" && validID(rest[0]) && rest[1] == "comments":
		return "/posts/" + rest[0] + "/comments", []string{"page", "per_page"}, nil
	default:
		return "", nil, errNotAllowed
	}
}

var errNotAllowed = reject(http.StatusForbidden, "this request is not allowed by the broker policy")

// validID reports whether s is a post or group id: 1 to 12 digits, no leading zero.
func validID(s string) bool {
	return isNumber(s, maxIDLength)
}

// validCount reports whether s is a number from 1 to limit, written without sign
// or leading zero.
func validCount(s string, limit int) bool {
	if !isNumber(s, len(strconv.Itoa(limit))) {
		return false
	}
	n, err := strconv.Atoi(s)

	return err == nil && n <= limit
}

// isNumber reports whether s is a decimal number of at most maxDigits digits
// without sign or leading zero.
func isNumber(s string, maxDigits int) bool {
	if s == "" || len(s) > maxDigits || s[0] < '1' || s[0] > '9' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}

// checkQuery validates a raw query against the keys the endpoint accepts and
// returns it rebuilt from the validated values.
func checkQuery(raw string, allowed []string) (string, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", reject(http.StatusBadRequest, "malformed query string")
	}

	for _, key := range slices.Sorted(maps.Keys(values)) {
		if !slices.Contains(allowed, key) {
			if len(allowed) == 0 {
				return "", reject(http.StatusBadRequest, "this endpoint takes no query parameters")
			}
			return "", reject(http.StatusBadRequest, "unknown query parameter (allowed: %s)", strings.Join(allowed, ", "))
		}
		if len(values[key]) != 1 {
			return "", reject(http.StatusBadRequest, "query parameter %q must be given once", key)
		}

		value := values.Get(key)
		switch key {
		case "page":
			if !validCount(value, maxPage) {
				return "", reject(http.StatusBadRequest, "page must be a number from 1 to %d", maxPage)
			}
		case "per_page":
			if !validCount(value, maxPerPage) {
				return "", reject(http.StatusBadRequest, "per_page must be a number from 1 to %d", maxPerPage)
			}
		case "q":
			if len(value) > maxQueryLength || !utf8.ValidString(value) {
				return "", reject(http.StatusBadRequest, "q must be valid UTF-8 of at most %d bytes", maxQueryLength)
			}
		}
	}

	return values.Encode(), nil
}
