package broker

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// maxTokenLength is the longest access token the broker accepts, in bytes.
const maxTokenLength = 4096

// ValidToken checks that an access token can be used as a header value: 1 to
// 4096 printable ASCII characters without spaces. The error never contains the token.
func ValidToken(token string) error {
	if token == "" {
		return errors.New("the access token is empty")
	}
	if len(token) > maxTokenLength {
		return fmt.Errorf("the access token is longer than %d bytes", maxTokenLength)
	}
	for i := range len(token) {
		if c := token[i]; c <= ' ' || c >= 0x7f {
			return errors.New("the access token contains characters that are not allowed (only printable ASCII without spaces)")
		}
	}

	return nil
}

// readToken reads the access token that start hands over on stdin, closes the
// reader and returns the token. The token is the data up to its end, without the
// trailing line break, and is checked with ValidToken.
func readToken(r io.ReadCloser) (string, error) {
	defer func() { _ = r.Close() }()

	// Room for the token, a "\r\n" and one more byte to tell whether there is more.
	data, err := io.ReadAll(io.LimitReader(r, maxTokenLength+3))
	if err != nil {
		return "", fmt.Errorf("failed to read the access token: %w", err)
	}
	token := strings.TrimRight(string(data), "\r\n")
	if err := ValidToken(token); err != nil {
		return "", err
	}

	return token, nil
}
