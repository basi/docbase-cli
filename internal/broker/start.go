package broker

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// readyTimeout is how long start waits for serve to become ready, and how long
// serve waits for the token. It is a variable so that tests can shorten it.
var readyTimeout = 10 * time.Second

// errExited is wrapped by the error for a serve process that ended during the handshake.
var errExited = errors.New("the broker exited")

// The start process and the serve process talk over a pipe that serve inherits as
// file descriptor 3, one line at a time:
//
//	serve -> start   hardened      the process is hardened; the token may be sent
//	start -> serve   <token>\n     on stdin, which start then closes
//	serve -> start   ready         the socket is listening
//	serve -> start   error: <msg>  at any point; serve exits
//
// The token reaches serve only after it hardened itself, and it is never in the
// arguments or the environment of serve.
//
// This protects the token from processes that start after the broker reported
// ready. A process of the same user that is already running during the start can
// open /proc/<pid>/fd/0 of serve before it is hardened and read the token from
// the pipe, or write "hardened" to the status pipe itself. That is inherent: the
// same process can equally read the environment of the start step or the
// credential store the token came from. Start the broker before any untrusted
// process runs.
const (
	msgHardened   = "hardened"
	msgReady      = "ready"
	msgErrorLabel = "error: "
)

// serveEnvKeys are the only variables serve gets from the environment of start:
// where to find CA certificates, and what lets a CI runner track the process.
var serveEnvKeys = []string{"SSL_CERT_FILE", "SSL_CERT_DIR", "RUNNER_TRACKING_ID"}

// StartOptions says how to start a broker.
type StartOptions struct {
	Exe        string // the docbase binary that runs "broker serve"
	Team       string
	Token      string
	SocketPath string
	TTL        time.Duration
}

// Start runs a broker as a detached process and returns its process id once the
// broker is ready to serve on opts.SocketPath. The broker is not waited for. The
// token is handed over after the broker hardened itself; it is not part of the
// command line or the environment.
func Start(opts StartOptions) (pid int, err error) {
	if err := ValidToken(opts.Token); err != nil {
		return 0, err
	}

	cmd := serveCommand(opts.Exe, opts.Team, opts.SocketPath, opts.TTL)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create a pipe for the broker: %w", err)
	}
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return 0, fmt.Errorf("failed to create a pipe for the broker: %w", err)
	}
	defer func() { _ = statusRead.Close() }()
	cmd.ExtraFiles = []*os.File{statusWrite} // file descriptor 3 of serve

	err = cmd.Start()
	// The child has its own copy now. Without closing ours, the read would never see the end.
	_ = statusWrite.Close()
	if err != nil {
		_ = stdin.Close()
		return 0, fmt.Errorf("failed to start the broker: %w", err)
	}

	if err := handshake(statusRead, stdin, opts.Token, readyTimeout); err != nil {
		_ = cmd.Process.Kill()
		waitErr := cmd.Wait()
		if errors.Is(err, errExited) && waitErr != nil {
			// Say how it ended; the broker has no output of its own.
			err = fmt.Errorf("%w (%v)", err, waitErr)
		}
		return 0, err
	}

	pid = cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return pid, fmt.Errorf("failed to detach from the broker: %w", err)
	}

	return pid, nil
}

// serveCommand builds the command that runs serve. It carries no secret, runs in
// the root directory, gets a minimal environment, has no output and its own
// session. The caller connects stdin and file descriptor 3.
func serveCommand(exe, team, socketPath string, ttl time.Duration) *exec.Cmd {
	cmd := exec.Command(exe, "broker", "serve", "--team", team, "--socket", socketPath, "--ttl", ttl.String())
	cmd.Dir = "/"
	// Not nil: a nil Env would hand the whole environment of start to serve.
	cmd.Env = []string{}
	for _, key := range serveEnvKeys {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	detach(cmd)

	return cmd
}

// handshake runs the start side of the conversation with serve: status is what
// serve writes to file descriptor 3, stdin is the standard input of serve. It
// returns when serve is ready, or with the reason it is not. stdin is closed
// before it returns.
func handshake(status io.Reader, stdin io.WriteCloser, token string, timeout time.Duration) error {
	defer func() { _ = stdin.Close() }()

	done := make(chan struct{})
	defer close(done)
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(status)
		scanner.Buffer(make([]byte, 0, 512), 4096)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-done:
				return
			}
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	hardened := false
	for {
		var line string
		var open bool
		select {
		case line, open = <-lines:
		case <-timer.C:
			return fmt.Errorf("timed out after %s waiting for the broker %s", timeout, pending(hardened, "to harden itself", "to become ready"))
		}

		switch {
		case !open:
			return fmt.Errorf("%w %s", errExited, pending(hardened, "before it hardened itself", "before it was ready"))
		case strings.HasPrefix(line, msgErrorLabel):
			return fmt.Errorf("the broker failed: %s", redact(strings.TrimPrefix(line, msgErrorLabel), token))
		case line == msgHardened && !hardened:
			hardened = true
			if _, err := io.WriteString(stdin, token+"\n"); err != nil {
				return fmt.Errorf("failed to hand the access token to the broker: %w", err)
			}
			if err := stdin.Close(); err != nil {
				return fmt.Errorf("failed to hand the access token to the broker: %w", err)
			}
		case line == msgReady && hardened:
			return nil
		default:
			return errors.New("unexpected message from the broker")
		}
	}
}

// pending picks the wording for the step of the handshake that did not complete.
func pending(hardened bool, beforeHardened, afterHardened string) string {
	if hardened {
		return afterHardened
	}
	return beforeHardened
}

// redact removes the token from a message, as a precaution.
func redact(message, token string) string {
	if token == "" {
		return message
	}

	return strings.ReplaceAll(message, token, "[redacted]")
}

// redactBytes is redact for a body. It returns the body itself when the token is not in it.
func redactBytes(body []byte, token string) []byte {
	if token == "" || !bytes.Contains(body, []byte(token)) {
		return body
	}

	return bytes.ReplaceAll(body, []byte(token), []byte("[redacted]"))
}
