//go:build !linux

package credential

// sessionBusAvailable always reports true: only the Linux Secret Service backend
// needs a D-Bus session bus.
func sessionBusAvailable() bool {
	return true
}
