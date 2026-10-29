package credential

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
func TestMain(m *testing.M) {
	keyring.MockInit()
	SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}

func TestRoundTrip(t *testing.T) {
	keyring.MockInit()

	if err := Set(" Example ", "token"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Team domains are case-insensitive: the account is the lowercased team.
	if got, err := keyring.Get(service, "example"); err != nil || got != "token" {
		t.Errorf("keyring entry: got %q, %v; want %q", got, err, "token")
	}
	if got, err := Get("EXAMPLE"); err != nil || got != "token" {
		t.Errorf("Get: got %q, %v; want %q", got, err, "token")
	}

	if err := Delete("example"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Get("example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: got %v, want ErrNotFound", err)
	}
	if err := Delete("example"); err != nil {
		t.Errorf("Delete of a missing entry: %v", err)
	}
}

func TestEmptyTeam(t *testing.T) {
	keyring.MockInit()

	for _, team := range []string{"", "  "} {
		if _, err := Get(team); err == nil || !strings.Contains(err.Error(), "team domain is required") {
			t.Errorf("Get(%q): got %v, want the team domain error", team, err)
		}
		if err := Set(team, "token"); err == nil || !strings.Contains(err.Error(), "team domain is required") {
			t.Errorf("Set(%q): got %v, want the team domain error", team, err)
		}
		if err := Delete(team); err == nil || !strings.Contains(err.Error(), "team domain is required") {
			t.Errorf("Delete(%q): got %v, want the team domain error", team, err)
		}
	}
}

func TestStoreErrors(t *testing.T) {
	boom := errors.New("boom")
	keyring.MockInitWithError(boom)

	// Errors other than "not found" are wrapped and still match with errors.Is.
	if _, err := Get("example"); !errors.Is(err, boom) || errors.Is(err, ErrNotFound) {
		t.Errorf("Get: got %v, want an error wrapping %v", err, boom)
	}
	if err := Set("example", "token"); !errors.Is(err, boom) {
		t.Errorf("Set: got %v, want an error wrapping %v", err, boom)
	}
	if err := Delete("example"); !errors.Is(err, boom) {
		t.Errorf("Delete: got %v, want an error wrapping %v", err, boom)
	}
}

func TestNoSessionBus(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(service, "example", "stored"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}

	restore := SetBusCheck(func() bool { return false })
	t.Cleanup(restore)

	checkBlocked(t)

	// The team is validated first, so an empty team still reports its own error.
	if _, err := Get(""); err == nil || !strings.Contains(err.Error(), "team domain is required") {
		t.Errorf("Get(\"\"): got %v, want the team domain error", err)
	}
	if err := Set("", "token"); err == nil || !strings.Contains(err.Error(), "team domain is required") {
		t.Errorf("Set(\"\"): got %v, want the team domain error", err)
	}
	if err := Delete(""); err == nil || !strings.Contains(err.Error(), "team domain is required") {
		t.Errorf("Delete(\"\"): got %v, want the team domain error", err)
	}

	// The returned function puts the previous check back.
	restore()
	if got, err := Get("example"); err != nil || got != "stored" {
		t.Errorf("Get after restore: got %q, %v; want %q", got, err, "stored")
	}
}

// checkBlocked checks that, with no session bus, Get, Set and Delete fail with
// the session bus error and leave the keyring entry for "example" (which holds
// "stored") as it was, so none of them reached the keyring.
func checkBlocked(t *testing.T) {
	t.Helper()

	token, err := Get("example")
	checkNoSessionBus(t, "Get", err)
	if token != "" {
		t.Errorf("Get: got token %q, want none", token)
	}
	checkNoSessionBus(t, "Set", Set("example", "replacement"))
	checkNoSessionBus(t, "Delete", Delete("example"))

	if got, err := keyring.Get(service, "example"); err != nil || got != "stored" {
		t.Errorf("keyring entry: got %q, %v; want %q untouched", got, err, "stored")
	}
}

// checkNoSessionBus checks that err wraps the session bus error in the credential
// store's error format and is not mistaken for a missing entry.
func checkNoSessionBus(t *testing.T, name string, err error) {
	t.Helper()

	if !errors.Is(err, errNoSessionBus) || errors.Is(err, ErrNotFound) {
		t.Errorf("%s: got %v, want an error wrapping %v", name, err, errNoSessionBus)
		return
	}
	if !strings.HasPrefix(err.Error(), "OS credential store: ") {
		t.Errorf("%s: got %q, want the %q prefix", name, err, "OS credential store: ")
	}
}

func TestWithTimeout(t *testing.T) {
	original := timeout
	timeout = 20 * time.Millisecond
	t.Cleanup(func() { timeout = original })

	release := make(chan struct{})
	defer close(release)

	_, err := withTimeout(func() (string, error) {
		<-release
		return "too late", nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out after 20ms waiting for the OS credential store") {
		t.Errorf("got %v, want a timeout error", err)
	}

	// A call that finishes in time is not affected.
	timeout = time.Second
	got, err := withTimeout(func() (string, error) { return "in time", nil })
	if err != nil || got != "in time" {
		t.Errorf("got %q, %v; want %q", got, err, "in time")
	}
}
