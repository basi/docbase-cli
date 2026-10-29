package broker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// observedStdin counts reads of the standard input of serve and notes when it is closed.
type observedStdin struct {
	io.Reader
	reads  atomic.Int32
	closed atomic.Bool
}

func (o *observedStdin) Read(p []byte) (int, error) {
	o.reads.Add(1)
	return o.Reader.Read(p)
}

func (o *observedStdin) Close() error {
	o.closed.Store(true)
	return nil
}

// observedStatus notes when the status pipe of serve is closed.
type observedStatus struct {
	io.WriteCloser
	closed atomic.Bool
}

func (o *observedStatus) Close() error {
	o.closed.Store(true)
	return o.WriteCloser.Close()
}

// serveEnv is the world of a serve process: the pipes to start, a private socket directory and stubs for the Linux-only parts.
type serveEnv struct {
	opts       ServeOptions
	dir        string
	socket     string
	statusRead *os.File
	stdinWrite *os.File
	stdin      *observedStdin
	status     *observedStatus
	hardened   atomic.Int32
}

func newServeEnv(t *testing.T, ttl time.Duration) *serveEnv {
	t.Helper()

	dir, socket, err := CreateSocketDir(shortTempDir(t))
	if err != nil {
		t.Fatalf("CreateSocketDir: %v", err)
	}
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	t.Cleanup(func() {
		for _, f := range []*os.File{statusRead, statusWrite, stdinRead, stdinWrite} {
			_ = f.Close()
		}
	})

	env := &serveEnv{dir: dir, socket: socket, statusRead: statusRead, stdinWrite: stdinWrite}
	env.stdin = &observedStdin{Reader: stdinRead}
	env.status = &observedStatus{WriteCloser: statusWrite}
	env.opts = ServeOptions{
		Team:       "example",
		SocketPath: socket,
		TTL:        ttl,
		Version:    "test",
		Stdin:      env.stdin,
		Status:     env.status,
		harden: func() error {
			env.hardened.Add(1)
			return nil
		},
		peerUID: func(net.Conn) (uint32, error) { return uint32(os.Getuid()), nil },
	}

	return env
}

// run runs serve in the background.
func (e *serveEnv) run(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- RunServe(ctx, e.opts) }()

	return done
}

// lines returns what serve reported. It must only be called when serve has finished and closed the pipe.
func (e *serveEnv) lines(t *testing.T) []string {
	t.Helper()

	var lines []string
	scanner := bufio.NewScanner(e.statusRead)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

// waitUntil waits for the condition to hold.
func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitFor(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return")
		return nil
	}
}

func TestRunServeLifecycle(t *testing.T) {
	env := newServeEnv(t, time.Hour)
	done := env.run(t.Context())

	// start plays its part: wait for "hardened", send the token, wait for "ready".
	if err := handshake(env.statusRead, env.stdinWrite, testToken, 10*time.Second); err != nil {
		t.Fatalf("handshake: %v", err)
	}

	// The process was hardened before the token was read, and the pipes are closed.
	if got := env.hardened.Load(); got != 1 {
		t.Errorf("the process was hardened %d times, want once", got)
	}
	if got := env.stdin.reads.Load(); got == 0 || !env.stdin.closed.Load() {
		t.Errorf("stdin: %d reads, closed = %v; want it read and closed", got, env.stdin.closed.Load())
	}
	// The status pipe is closed right after "ready" was sent.
	waitUntil(t, "the status pipe is closed", env.status.closed.Load)

	// It serves, in a private directory, on a socket only the user can use.
	info, err := Status(env.socket)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if info.Team != "example" {
		t.Errorf("team = %q, want example", info.Team)
	}
	if err := validateSocketDir(env.dir, os.Getuid()); err != nil {
		t.Errorf("socket directory: %v", err)
	}
	if err := validateSocket(env.socket, os.Getuid()); err != nil {
		t.Errorf("socket: %v", err)
	}

	if err := Stop(env.socket); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := waitFor(t, done); err != nil {
		t.Errorf("RunServe = %v, want nil", err)
	}
	// The socket and its directory are gone.
	for _, path := range []string{env.socket, env.dir} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists (Lstat error: %v)", path, err)
		}
	}
}

func TestRunServeStopsWhenTheTTLExpires(t *testing.T) {
	env := newServeEnv(t, 300*time.Millisecond)
	done := env.run(context.Background())

	if err := handshake(env.statusRead, env.stdinWrite, testToken, 10*time.Second); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if _, err := Status(env.socket); err != nil {
		t.Fatalf("Status: %v", err)
	}

	if err := waitFor(t, done); err != nil {
		t.Errorf("RunServe = %v, want nil", err)
	}
	for _, path := range []string{env.socket, env.dir} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists after the TTL (Lstat error: %v)", path, err)
		}
	}
	if _, err := Status(env.socket); err == nil {
		t.Error("the broker still answers after its TTL")
	}
}

func TestRunServeStopsWhenInterrupted(t *testing.T) {
	env := newServeEnv(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := env.run(ctx)

	if err := handshake(env.statusRead, env.stdinWrite, testToken, 10*time.Second); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	cancel()
	if err := waitFor(t, done); err != nil {
		t.Errorf("RunServe = %v, want nil", err)
	}
	for _, path := range []string{env.socket, env.dir} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists (Lstat error: %v)", path, err)
		}
	}
}

// The failures that serve reports to start before it is ready.
func TestRunServeReportsFailures(t *testing.T) {
	// token is what start sends, "-" for nothing at all (stdin is closed at once).
	tests := []struct {
		name         string
		token        string
		change       func(t *testing.T, env *serveEnv)
		wantLines    []string // exact lines before the error line
		wantError    string
		wantHardened int32
		wantReads    bool
		leftAlone    func(t *testing.T, env *serveEnv) // what must not be touched
	}{
		{
			name:  "hardening fails",
			token: testToken,
			change: func(t *testing.T, env *serveEnv) {
				env.opts.harden = func() error { return errors.New("prctl: operation not permitted") }
			},
			wantError: "failed to harden the process: prctl: operation not permitted",
		},
		{
			name:      "invalid team",
			token:     testToken,
			change:    func(t *testing.T, env *serveEnv) { env.opts.Team = "a/b" },
			wantError: "team domain",
		},
		{
			name:      "relative socket path",
			token:     testToken,
			change:    func(t *testing.T, env *serveEnv) { env.opts.SocketPath = "broker.sock" },
			wantError: "not absolute",
		},
		{
			name:      "socket path too long",
			token:     testToken,
			change:    func(t *testing.T, env *serveEnv) { env.opts.SocketPath = "/" + strings.Repeat("a", 120) },
			wantError: "longer than",
		},
		{
			name:      "no ttl",
			token:     testToken,
			change:    func(t *testing.T, env *serveEnv) { env.opts.TTL = 0 },
			wantError: "ttl must be positive",
		},
		{
			name:         "no token",
			token:        "-",
			wantLines:    []string{"hardened"},
			wantError:    "the access token is empty",
			wantHardened: 1,
			wantReads:    true,
		},
		{
			name:         "token with a space",
			token:        "s3cretvalue with space",
			wantLines:    []string{"hardened"},
			wantError:    "characters that are not allowed",
			wantHardened: 1,
			wantReads:    true,
		},
		{
			name:         "token with a line break inside",
			token:        "s3cretvalue\nsecond-line",
			wantLines:    []string{"hardened"},
			wantError:    "characters that are not allowed",
			wantHardened: 1,
			wantReads:    true,
		},
		{
			name:         "token that is too long",
			token:        strings.Repeat("s3cretvalue", 1000),
			wantLines:    []string{"hardened"},
			wantError:    "longer than 4096 bytes",
			wantHardened: 1,
			wantReads:    true,
		},
		{
			name:  "socket directory with the wrong mode",
			token: testToken,
			change: func(t *testing.T, env *serveEnv) {
				if err := os.Chmod(env.dir, 0o755); err != nil {
					t.Fatalf("Chmod: %v", err)
				}
			},
			wantLines:    []string{"hardened"},
			wantError:    "has mode 0755",
			wantHardened: 1,
			wantReads:    true,
			leftAlone: func(t *testing.T, env *serveEnv) {
				if _, err := os.Lstat(env.dir); err != nil {
					t.Errorf("the directory was removed: %v", err)
				}
				if _, err := os.Lstat(env.socket); err == nil {
					t.Error("a socket was created in a directory with the wrong mode")
				}
			},
		},
		{
			name:  "socket directory that is a symlink",
			token: testToken,
			change: func(t *testing.T, env *serveEnv) {
				link := filepath.Join(filepath.Dir(env.dir), "link")
				if err := os.Symlink(env.dir, link); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
				env.opts.SocketPath = filepath.Join(link, "broker.sock")
			},
			wantLines:    []string{"hardened"},
			wantError:    "is a symlink",
			wantHardened: 1,
			wantReads:    true,
			leftAlone: func(t *testing.T, env *serveEnv) {
				if _, err := os.Lstat(env.dir); err != nil {
					t.Errorf("the directory behind the link was removed: %v", err)
				}
			},
		},
		{
			name:  "socket path that exists",
			token: testToken,
			change: func(t *testing.T, env *serveEnv) {
				if err := os.WriteFile(env.socket, []byte("not ours"), 0o600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			},
			wantLines:    []string{"hardened"},
			wantError:    "failed to listen on the socket",
			wantHardened: 1,
			wantReads:    true,
			leftAlone: func(t *testing.T, env *serveEnv) {
				if data, err := os.ReadFile(env.socket); err != nil || string(data) != "not ours" {
					t.Errorf("the existing file was touched: %q, %v", data, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newServeEnv(t, time.Hour)
			if tt.change != nil {
				tt.change(t, env)
			}
			if tt.token != "-" {
				// Pipes buffer the token, so serve does not wait for a writer.
				_, _ = io.WriteString(env.stdinWrite, tt.token+"\n")
			}
			_ = env.stdinWrite.Close()

			err := waitFor(t, env.run(context.Background()))
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("RunServe = %v, want an error containing %q", err, tt.wantError)
			}

			lines := env.lines(t)
			if len(lines) != len(tt.wantLines)+1 {
				t.Fatalf("serve reported %q, want %q and one error line", lines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if lines[i] != want {
					t.Errorf("line %d = %q, want %q", i, lines[i], want)
				}
			}
			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, "error: ") || !strings.Contains(last, tt.wantError) {
				t.Errorf("last line = %q, want an error line containing %q", last, tt.wantError)
			}

			// The token is never in what serve says.
			for _, line := range append(lines, err.Error()) {
				if strings.Contains(line, "s3cretvalue") || strings.Contains(line, testToken) {
					t.Errorf("%q contains the token", line)
				}
			}
			if got := env.hardened.Load(); got != tt.wantHardened {
				t.Errorf("hardened %d times, want %d", got, tt.wantHardened)
			}
			// Nothing is read before the process is hardened.
			if got := env.stdin.reads.Load() > 0; got != tt.wantReads {
				t.Errorf("stdin was read = %v, want %v", got, tt.wantReads)
			}
			if tt.leftAlone != nil {
				tt.leftAlone(t, env)
			}
		})
	}
}

func TestRunServeGivesUpWithoutAToken(t *testing.T) {
	original := readyTimeout
	readyTimeout = 150 * time.Millisecond
	t.Cleanup(func() { readyTimeout = original })

	env := newServeEnv(t, time.Hour)
	// The write end stays open: start never sends anything.
	err := waitFor(t, env.run(context.Background()))
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for the access token") {
		t.Errorf("RunServe = %v, want the timeout error", err)
	}
	if lines := env.lines(t); len(lines) != 2 || lines[0] != "hardened" || !strings.HasPrefix(lines[1], "error: timed out") {
		t.Errorf("serve reported %q", lines)
	}
}

func TestRunServeStopsWhileWaitingForAToken(t *testing.T) {
	env := newServeEnv(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := env.run(ctx)

	// serve says hardened, then waits for the token that never comes.
	scanner := bufio.NewScanner(env.statusRead)
	if !scanner.Scan() || scanner.Text() != "hardened" {
		t.Fatalf("first line = %q, want hardened", scanner.Text())
	}
	cancel()

	err := waitFor(t, done)
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("RunServe = %v, want the interrupted error", err)
	}
}

func TestRunServeWithoutAReaderOnTheStatusPipe(t *testing.T) {
	env := newServeEnv(t, time.Hour)
	// start went away: nobody reads the status pipe any more.
	_ = env.statusRead.Close()

	err := waitFor(t, env.run(context.Background()))
	if err == nil || !strings.Contains(err.Error(), "failed to report to start") {
		t.Errorf("RunServe = %v, want the report error", err)
	}
	if got := env.stdin.reads.Load(); got != 0 {
		t.Errorf("stdin was read %d times, want none: the token must not be requested of a start that is gone", got)
	}
}

func TestStatusWriter(t *testing.T) {
	var out strings.Builder
	w := &statusWriter{w: nopCloser{&out}}

	_ = w.send("error: first\nsecond\r\nthird")
	w.close()
	w.close() // closing twice is fine
	_ = w.send("after close")

	if got, want := out.String(), "error: first second  third\n"; got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}

	// A missing pipe is not an error for the caller.
	var none statusWriter
	if err := none.send("x"); err != nil {
		t.Errorf("send without a pipe = %v", err)
	}
	none.close()
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
