// Package credential stores DocBase access tokens in the OS credential store
// (macOS Keychain, Windows Credential Manager, Secret Service on Linux).
package credential

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// service is the credential store service name; the account is the team domain.
const service = "docbase-cli"

// timeout bounds every credential store call so that a locked or prompting
// store cannot hang non-interactive runs. It is a variable so tests can shorten it.
var timeout = 30 * time.Second

// ErrNotFound is returned when no access token is stored for the team.
var ErrNotFound = errors.New("access token not found in the OS credential store")

// Get returns the access token stored for the team.
func Get(team string) (string, error) {
	account, err := accountName(team)
	if err != nil {
		return "", err
	}

	return withTimeout(func() (string, error) {
		token, err := keyring.Get(service, account)
		return token, normalize(err)
	})
}

// Set stores the access token for the team and reads it back to verify it.
func Set(team, token string) error {
	account, err := accountName(team)
	if err != nil {
		return err
	}

	if _, err := withTimeout(func() (struct{}, error) {
		return struct{}{}, normalize(keyring.Set(service, account, token))
	}); err != nil {
		return err
	}

	// Read the value back before callers rely on it (e.g. by removing the plaintext
	// copy): the macOS backend reads the security tool's combined stdout/stderr,
	// so a mangled value would otherwise go unnoticed.
	saved, err := Get(team)
	if err != nil {
		return fmt.Errorf("failed to read back the saved access token: %w", err)
	}
	if saved != token {
		return errors.New("the OS credential store returned a different value than was saved")
	}

	return nil
}

// Delete removes the access token stored for the team. A missing entry is not an error.
func Delete(team string) error {
	account, err := accountName(team)
	if err != nil {
		return err
	}

	_, err = withTimeout(func() (struct{}, error) {
		return struct{}{}, normalize(keyring.Delete(service, account))
	})
	if errors.Is(err, ErrNotFound) {
		return nil
	}

	return err
}

// accountName returns the credential store account for a team domain.
// Team domains are case-insensitive, so the account is normalized.
func accountName(team string) (string, error) {
	account := strings.ToLower(strings.TrimSpace(team))
	if account == "" {
		return "", errors.New("team domain is required")
	}

	return account, nil
}

// normalize maps a go-keyring error to this package's errors.
func normalize(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotFound
	default:
		return fmt.Errorf("OS credential store: %w", err)
	}
}

// withTimeout runs fn in a goroutine and gives up after timeout. A call that
// never returns is abandoned; the channel is buffered so that the goroutine can
// still finish without blocking.
func withTimeout[T any](fn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}

	done := make(chan result, 1)
	go func() {
		value, err := fn()
		done <- result{value: value, err: err}
	}()

	select {
	case r := <-done:
		return r.value, r.err
	case <-time.After(timeout):
		var zero T
		return zero, fmt.Errorf("timed out after %s waiting for the OS credential store", timeout)
	}
}
