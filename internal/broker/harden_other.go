//go:build !linux

package broker

import "errors"

// harden is not supported on this platform. The broker runs on Linux only.
func harden() error {
	return errors.New("hardening the process is not supported on this platform")
}
