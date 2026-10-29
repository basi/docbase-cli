//go:build linux

package broker

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// Environment variables that turn the test binary into a helper process.
const (
	helperEnv      = "BROKER_TEST_HELPER"
	helperSetup    = "harden"   // run harden() in the helper
	helperFallback = "fallback" // run harden() as if the runtime could not run a call on all threads (cgo)
	helperPlain    = "plain"    // do not (the control case)
)

// TestHelperProcess is not a test: the tests below run it in a child process. The
// child optionally hardens itself, checks what harden() promises, says READY and
// waits for its standard input to be closed.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("helper process")
	}

	if mode == helperSetup || mode == helperFallback {
		// harden() and the check must run on the same thread: with the fallback,
		// no_new_privs is only set on the thread that calls it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if mode == helperFallback {
			allThreadsSyscall = func(trap, a1, a2, a3 uintptr) (uintptr, uintptr, syscall.Errno) {
				return 0, 0, syscall.ENOTSUP
			}
		}
		if err := harden(); err != nil {
			t.Fatalf("harden: %v", err)
		}
		checkHardened(t, mode == helperSetup)
	}

	_, _ = os.Stdout.WriteString("READY\n")
	_, _ = os.Stdin.Read(make([]byte, 1))
}

// checkHardened checks the process-wide effects of harden() and that no_new_privs
// is set on the calling thread and, if allThreads is set, on every thread.
func checkHardened(t *testing.T, allThreads bool) {
	t.Helper()

	if dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0); err != nil || dumpable != 0 {
		t.Errorf("PR_GET_DUMPABLE = %d, %v; want 0", dumpable, err)
	}
	if got, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0); err != nil || got != 1 {
		t.Errorf("PR_GET_NO_NEW_PRIVS = %d, %v; want 1", got, err)
	}

	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &limit); err != nil {
		t.Fatalf("Getrlimit: %v", err)
	}
	if limit.Cur != 0 || limit.Max != 0 {
		t.Errorf("RLIMIT_CORE = %d/%d, want 0/0", limit.Cur, limit.Max)
	}
	// The hard limit was lowered too, so an unprivileged process cannot raise it again.
	if os.Geteuid() != 0 {
		if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 1 << 20, Max: 1 << 20}); err == nil {
			t.Error("the core file limit could be raised again")
		}
	}

	// no_new_privs belongs to a thread: look at many threads at once. Setting it on all
	// of them only works where the runtime can run a system call on every thread (not
	// with cgo), which a harmless call tells.
	if !allThreads {
		return
	}
	if _, _, errno := syscall.AllThreadsSyscall(unix.SYS_GETPID, 0, 0, 0); errno != 0 {
		t.Logf("not checking every thread: %v", errno)
		return
	}
	const threads = 32
	var ready, done sync.WaitGroup
	ready.Add(threads)
	done.Add(threads)
	for range threads {
		go func() {
			defer done.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ready.Done()
			ready.Wait() // all goroutines are here at the same time, so on different threads
			if got, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0); err != nil || got != 1 {
				t.Errorf("PR_GET_NO_NEW_PRIVS on a thread = %d, %v; want 1", got, err)
			}
		}()
	}
	done.Wait()
}

// startHelper runs the helper process in the given mode and returns it once it is ready.
func startHelper(t *testing.T, mode string) (*exec.Cmd, func() error) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, "BROKER_TEST_SECRET="+testToken)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var output []string
	ready := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		output = append(output, scanner.Text())
		if scanner.Text() == "READY" {
			ready = true
			break
		}
	}
	if !ready {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("the helper process never got ready; output:\n%s", strings.Join(output, "\n"))
	}

	finish := func() error {
		_ = stdin.Close()
		// The rest of the output belongs to a failure, if there is one.
		for scanner.Scan() {
			output = append(output, scanner.Text())
		}
		err := cmd.Wait()
		if err != nil {
			t.Logf("helper output:\n%s", strings.Join(output, "\n"))
		}
		return err
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })

	return cmd, finish
}

// TestHarden runs harden() in a child process (it cannot be undone) and checks its effects there.
func TestHarden(t *testing.T) {
	_, finish := startHelper(t, helperSetup)
	if err := finish(); err != nil {
		t.Errorf("the helper process failed: %v", err)
	}
}

// TestHardenWithoutAllThreadsSupport does the same where the runtime cannot set no_new_privs
// on every thread, as in a binary that uses cgo.
func TestHardenWithoutAllThreadsSupport(t *testing.T) {
	_, finish := startHelper(t, helperFallback)
	if err := finish(); err != nil {
		t.Errorf("the helper process failed: %v", err)
	}
}

// TestHardenClosesTheProcessToTheSameUser checks the point of it all: a hardened process
// cannot be read by another process of the same, unprivileged, user.
func TestHardenClosesTheProcessToTheSameUser(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any process")
	}

	// The control: an ordinary process of the same user can be read.
	plain, finishPlain := startHelper(t, helperPlain)
	environ, err := os.ReadFile("/proc/" + strconv.Itoa(plain.Process.Pid) + "/environ")
	if err != nil {
		t.Skipf("cannot read the environment of an ordinary process either: %v", err)
	}
	if !strings.Contains(string(environ), "BROKER_TEST_SECRET="+testToken) {
		t.Fatalf("the control process does not show its environment: %q", environ)
	}
	_ = finishPlain()

	// A hardened process cannot.
	hardened, finish := startHelper(t, helperSetup)
	defer func() { _ = finish() }()
	pid := strconv.Itoa(hardened.Process.Pid)
	for _, name := range []string{"environ", "mem", "maps", "auxv"} {
		f, err := os.Open("/proc/" + pid + "/" + name)
		if err == nil {
			_ = f.Close()
			t.Errorf("/proc/%s/%s can be opened by the same user", pid, name)
			continue
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("/proc/%s/%s: %v, want a permission error", pid, name, err)
		}
	}
}
