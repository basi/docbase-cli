//go:build linux

package credential

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runUserDir holds the per-user runtime directories (/run/user/<uid>). It is a
// variable so that tests can point it at a temporary directory.
var runUserDir = "/run/user"

// sessionBusAvailable reports whether a D-Bus session bus can be reached without
// starting one. go-keyring talks to the Secret Service through godbus, which runs
// dbus-launch when it finds no session bus. On hosts without a login session,
// such as containers and CI runners, that spawns a bus daemon as a side effect
// and the Secret Service call fails anyway. The lookup follows godbus, so that a
// bus godbus would find is not reported as missing.
func sessionBusAvailable() bool {
	// An autolaunch address is no bus: godbus treats "autolaunch:" as unset and
	// runs dbus-launch.
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address != "" && !strings.HasPrefix(address, "autolaunch:") {
		return true
	}

	// The per-user bus listens on $XDG_RUNTIME_DIR/bus.
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" && fileExists(filepath.Join(runtimeDir, "bus")) {
		return true
	}

	// godbus does not read XDG_RUNTIME_DIR. It looks in /run/user/<uid> for the bus
	// socket and for a dbus-session file that holds the bus address.
	userDir := filepath.Join(runUserDir, strconv.Itoa(os.Getuid()))

	return fileExists(filepath.Join(userDir, "bus")) || hasSessionAddress(filepath.Join(userDir, "dbus-session"))
}

// fileExists reports whether path can be stat'ed. A bus entry is a socket, so any
// file type counts.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// hasSessionAddress reports whether path is a dbus-session file that godbus takes
// the bus address from: its content starts with DBUS_SESSION_BUS_ADDRESS= and the
// address is not empty.
func hasSessionAddress(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	address, ok := strings.CutPrefix(string(content), "DBUS_SESSION_BUS_ADDRESS=")

	return ok && strings.TrimRight(address, "\r\n") != ""
}
