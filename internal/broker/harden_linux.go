//go:build linux

package broker

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// harden makes the process keep its memory and environment from other processes
// of the same user: it is not dumpable (so /proc/<pid>/{environ,mem} and ptrace
// need root, and no core file is written), it cannot write a core file even if it
// were, and it cannot gain privileges by executing a program.
func harden() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_DUMPABLE): %w", err)
	}
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return fmt.Errorf("setrlimit(RLIMIT_CORE): %w", err)
	}
	if err := setNoNewPrivs(); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}

	// Read the settings back: a call that did not take effect must not go unnoticed.
	if dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0); err != nil || dumpable != 0 {
		return fmt.Errorf("the process is still dumpable (%d, %v)", dumpable, err)
	}

	return nil
}

// allThreadsSyscall runs a system call on every thread of the Go runtime. Tests replace it.
var allThreadsSyscall = syscall.AllThreadsSyscall

// setNoNewPrivs sets no_new_privs, which belongs to a thread, not to the process.
//
// It is set on all threads of the Go runtime where the runtime can run a system
// call on every one of them. With cgo it cannot, and the fallback sets the flag
// on the calling thread only: the other threads that exist at that point do not
// get it, and a thread created later has it only if the thread that creates it
// does. That is acceptable because the flag is a second line of defense: the
// broker never executes a program, so there is nothing for it to stop, and the
// release builds are made with CGO_ENABLED=0, which takes the first path.
func setNoNewPrivs() error {
	_, _, errno := allThreadsSyscall(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0)
	switch errno {
	case 0:
		return nil
	case syscall.ENOTSUP:
		return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	default:
		return errno
	}
}
