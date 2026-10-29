//go:build linux

package credential

import (
	"cmp"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zalando/go-keyring"
)

// writeBusFile creates the bus entry that a user session leaves in its runtime
// directory. It is a regular file here; the check accepts any file type.
func writeBusFile(t *testing.T, runtimeDir string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(runtimeDir, "bus"), nil, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// runUserBase returns a stand-in for /run/user that holds the given files (name
// to content) in the runtime directory of the user with the given uid.
func runUserBase(t *testing.T, uid int, files map[string]string) string {
	t.Helper()

	base := t.TempDir()
	dir := filepath.Join(base, strconv.Itoa(uid))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	return base
}

// useRunUserDir points the /run/user lookup at dir until the test ends, so that a
// bus on the machine running the tests cannot change the outcome.
func useRunUserDir(t *testing.T, dir string) {
	t.Helper()

	original := runUserDir
	runUserDir = dir
	t.Cleanup(func() { runUserDir = original })
}

func TestSessionBusAvailable(t *testing.T) {
	uid := os.Getuid()

	withBus := t.TempDir()
	writeBusFile(t, withBus)
	withoutBus := t.TempDir()

	// Stand-ins for /run/user. godbus reads the bus address from a dbus-session
	// file only when the content starts with DBUS_SESSION_BUS_ADDRESS=.
	emptyRunUser := runUserBase(t, uid, nil)
	runUserWithBus := runUserBase(t, uid, map[string]string{"bus": ""})
	otherUserWithBus := runUserBase(t, uid+1, map[string]string{"bus": ""})
	withSession := runUserBase(t, uid, map[string]string{"dbus-session": "DBUS_SESSION_BUS_ADDRESS=unix:abstract=/tmp/dbus-test\n"})
	withForeignSession := runUserBase(t, uid, map[string]string{"dbus-session": "export DBUS_SESSION_BUS_ADDRESS=unix:abstract=/tmp/dbus-test\n"})
	withEmptySession := runUserBase(t, uid, map[string]string{"dbus-session": "DBUS_SESSION_BUS_ADDRESS=\n"})

	tests := []struct {
		name       string
		address    string // DBUS_SESSION_BUS_ADDRESS
		runtimeDir string // XDG_RUNTIME_DIR
		runUser    string // stand-in for /run/user; an empty one when unset
		want       bool
	}{
		{name: "address set", address: "unix:path=/nonexistent/bus", want: true},
		{name: "address set, runtime dir without a bus", address: "unix:path=/nonexistent/bus", runtimeDir: withoutBus, want: true},
		{name: "autolaunch address, no bus anywhere", address: "autolaunch:", want: false},
		{name: "autolaunch address with parameters, no bus anywhere", address: "autolaunch:scope=*install-path", want: false},
		{name: "autolaunch address, bus in the runtime dir", address: "autolaunch:", runtimeDir: withBus, want: true},
		{name: "autolaunch address, bus in /run/user", address: "autolaunch:", runUser: runUserWithBus, want: true},
		{name: "bus in the runtime dir", runtimeDir: withBus, want: true},
		{name: "runtime dir without a bus", runtimeDir: withoutBus, want: false},
		{name: "runtime dir that does not exist", runtimeDir: filepath.Join(withoutBus, "missing"), want: false},
		{name: "bus in /run/user", runUser: runUserWithBus, want: true},
		{name: "bus in /run/user, runtime dir without a bus", runtimeDir: withoutBus, runUser: runUserWithBus, want: true},
		{name: "/run/user without a bus", runUser: emptyRunUser, want: false},
		{name: "bus of another user in /run/user", runUser: otherUserWithBus, want: false},
		{name: "dbus-session file with an address", runUser: withSession, want: true},
		{name: "dbus-session file not starting with the address", runUser: withForeignSession, want: false},
		{name: "dbus-session file with an empty address", runUser: withEmptySession, want: false},
		{name: "nothing set", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DBUS_SESSION_BUS_ADDRESS", tt.address)
			t.Setenv("XDG_RUNTIME_DIR", tt.runtimeDir)
			useRunUserDir(t, cmp.Or(tt.runUser, emptyRunUser))

			if got := sessionBusAvailable(); got != tt.want {
				t.Errorf("sessionBusAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

// An unset XDG_RUNTIME_DIR must not turn the lookup into a relative "bus" path.
func TestSessionBusUnsetRuntimeDir(t *testing.T) {
	workDir := t.TempDir()
	writeBusFile(t, workDir)
	t.Chdir(workDir)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	useRunUserDir(t, runUserBase(t, os.Getuid(), nil))

	if sessionBusAvailable() {
		t.Error("sessionBusAvailable() = true, want false: the bus file in the working directory does not count")
	}
}

// TestGuardFollowsTheEnvironment drives the guard with the real check, as a Linux
// CI job meets it: the in-memory keyring is only reached once a bus exists.
func TestGuardFollowsTheEnvironment(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(service, "example", "stored"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	t.Cleanup(SetBusCheck(sessionBusAvailable))

	runtimeDir := t.TempDir()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	useRunUserDir(t, runUserBase(t, os.Getuid(), nil))

	checkBlocked(t)

	writeBusFile(t, runtimeDir)
	if got, err := Get("example"); err != nil || got != "stored" {
		t.Errorf("Get with a bus: got %q, %v; want %q", got, err, "stored")
	}
}
