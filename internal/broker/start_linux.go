//go:build linux

package broker

import (
	"os/exec"
	"syscall"
)

// detach gives the command its own session, so that it has no controlling
// terminal and does not receive the signals meant for the process group of start.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
