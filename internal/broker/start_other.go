//go:build !linux

package broker

import "os/exec"

// detach does nothing on this platform. The broker runs on Linux only.
func detach(*exec.Cmd) {}
