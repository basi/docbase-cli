package broker

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestServeCommand(t *testing.T) {
	// A parent environment full of things serve must not inherit.
	t.Setenv("DOCBASE_API_TOKEN", testToken)
	t.Setenv("DOCBASE_BROKER_SOCKET", "/somewhere")
	t.Setenv("HTTPS_PROXY", "http://proxy.example.com:3128")
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:3128")
	t.Setenv("GITHUB_TOKEN", "ghs_secret")
	t.Setenv("HOME", "/home/someone")
	t.Setenv("SSL_CERT_FILE", "/etc/ssl/ca.pem")
	t.Setenv("SSL_CERT_DIR", "/etc/ssl/certs")
	t.Setenv("RUNNER_TRACKING_ID", "github_1234")

	cmd := serveCommand("/usr/local/bin/docbase", "example", "/tmp/docbase-broker-1/broker.sock", 30*time.Minute)

	wantArgs := []string{"/usr/local/bin/docbase", "broker", "serve", "--team", "example", "--socket", "/tmp/docbase-broker-1/broker.sock", "--ttl", "30m0s"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Errorf("Args = %q, want %q", cmd.Args, wantArgs)
	}
	if cmd.Path != "/usr/local/bin/docbase" {
		t.Errorf("Path = %q", cmd.Path)
	}

	wantEnv := []string{"SSL_CERT_FILE=/etc/ssl/ca.pem", "SSL_CERT_DIR=/etc/ssl/certs", "RUNNER_TRACKING_ID=github_1234"}
	if !slices.Equal(cmd.Env, wantEnv) {
		t.Errorf("Env = %q, want %q", cmd.Env, wantEnv)
	}

	if cmd.Dir != "/" {
		t.Errorf("Dir = %q, want /", cmd.Dir)
	}
	if cmd.Stdout != nil || cmd.Stderr != nil {
		t.Errorf("Stdout = %v and Stderr = %v, want both nil (the null device)", cmd.Stdout, cmd.Stderr)
	}
	if cmd.Stdin != nil || len(cmd.ExtraFiles) != 0 {
		t.Errorf("serveCommand connects stdin or extra files; the caller does that")
	}

	// The token is nowhere in the command.
	for _, part := range append(slices.Clone(cmd.Args), cmd.Env...) {
		if strings.Contains(part, testToken) {
			t.Errorf("%q contains the token", part)
		}
	}
}

func TestServeCommandEnvironmentIsNeverInherited(t *testing.T) {
	for _, key := range serveEnvKeys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv: %v", err)
		}
	}
	t.Setenv("DOCBASE_API_TOKEN", testToken)

	cmd := serveCommand("/usr/local/bin/docbase", "example", "/tmp/x/broker.sock", time.Hour)
	// A nil Env would hand the whole environment of the caller to serve.
	if cmd.Env == nil {
		t.Fatal("Env is nil: the child would inherit the environment")
	}
	if len(cmd.Env) != 0 {
		t.Errorf("Env = %q, want it empty", cmd.Env)
	}
	for _, entry := range cmd.Environ() {
		if strings.Contains(entry, testToken) || strings.HasPrefix(entry, "DOCBASE_") {
			t.Errorf("the environment of the child contains %q", entry)
		}
	}
}

func TestServeEnvKeys(t *testing.T) {
	want := []string{"SSL_CERT_FILE", "SSL_CERT_DIR", "RUNNER_TRACKING_ID"}
	if !slices.Equal(serveEnvKeys, want) {
		t.Errorf("serveEnvKeys = %q, want %q", serveEnvKeys, want)
	}
}

// newHandshakePipes returns the pipes of the conversation: what serve writes to
// file descriptor 3 (read end, write end), and the standard input of serve (read
// end, write end).
func newHandshakePipes(t *testing.T) (statusRead, statusWrite, stdinRead, stdinWrite *os.File) {
	t.Helper()

	var err error
	statusRead, statusWrite, err = os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	stdinRead, stdinWrite, err = os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	t.Cleanup(func() {
		for _, f := range []*os.File{statusRead, statusWrite, stdinRead, stdinWrite} {
			_ = f.Close()
		}
	})

	return statusRead, statusWrite, stdinRead, stdinWrite
}

func TestHandshakeSendsTheTokenOnlyAfterHardened(t *testing.T) {
	statusRead, statusWrite, stdinRead, stdinWrite := newHandshakePipes(t)

	result := make(chan error, 1)
	go func() { result <- handshake(statusRead, stdinWrite, testToken, 5*time.Second) }()

	// Serve has said nothing yet: no token may have been sent.
	received := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(stdinRead)
		received <- string(data)
	}()
	select {
	case got := <-received:
		t.Fatalf("serve received %q before it reported hardened", got)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := io.WriteString(statusWrite, "hardened\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-received:
		if got != testToken+"\n" {
			t.Errorf("serve received %q, want the token and a line break", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not receive the token")
	}

	// Stdin was closed (ReadAll returned), and the handshake waits for ready.
	select {
	case err := <-result:
		t.Fatalf("handshake returned %v before ready", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := io.WriteString(statusWrite, "ready\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-result; err != nil {
		t.Errorf("handshake = %v, want nil", err)
	}
}

func TestHandshakeFailures(t *testing.T) {
	tests := []struct {
		name      string
		serve     func(status, stdin *os.File) // what serve does; it may write lines and close the status pipe
		timeout   time.Duration
		want      []string
		wantToken bool // whether the token is sent to serve
	}{
		{
			name:  "error before hardened",
			serve: func(status, _ *os.File) { _, _ = io.WriteString(status, "error: prctl failed\n") },
			want:  []string{"the broker failed", "prctl failed"},
		},
		{
			name: "error after hardened",
			serve: func(status, _ *os.File) {
				_, _ = io.WriteString(status, "hardened\n")
				_, _ = io.WriteString(status, "error: socket directory is a symlink\n")
			},
			want:      []string{"the broker failed", "socket directory is a symlink"},
			wantToken: true,
		},
		{
			name:  "error that repeats the token",
			serve: func(status, _ *os.File) { _, _ = io.WriteString(status, "error: bad credentials "+testToken+"\n") },
			want:  []string{"the broker failed", "[redacted]"},
		},
		{
			name:  "exit before hardened",
			serve: func(status, _ *os.File) { _ = status.Close() },
			want:  []string{"the broker exited before it hardened itself"},
		},
		{
			name: "exit before ready",
			serve: func(status, _ *os.File) {
				_, _ = io.WriteString(status, "hardened\n")
				_ = status.Close()
			},
			want:      []string{"the broker exited before it was ready"},
			wantToken: true,
		},
		{
			name:    "silence",
			serve:   func(_, _ *os.File) {},
			timeout: 100 * time.Millisecond,
			want:    []string{"timed out after 100ms", "to harden itself"},
		},
		{
			name:      "silence after hardened",
			serve:     func(status, _ *os.File) { _, _ = io.WriteString(status, "hardened\n") },
			timeout:   100 * time.Millisecond,
			want:      []string{"timed out after 100ms", "to become ready"},
			wantToken: true,
		},
		{
			name:  "ready before hardened",
			serve: func(status, _ *os.File) { _, _ = io.WriteString(status, "ready\n") },
			want:  []string{"unexpected message"},
		},
		{
			name: "hardened twice",
			serve: func(status, _ *os.File) {
				_, _ = io.WriteString(status, "hardened\nhardened\n")
			},
			want:      []string{"unexpected message"},
			wantToken: true,
		},
		{
			name:  "nonsense",
			serve: func(status, _ *os.File) { _, _ = io.WriteString(status, "hello\n") },
			want:  []string{"unexpected message"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusRead, statusWrite, stdinRead, stdinWrite := newHandshakePipes(t)
			timeout := tt.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			go tt.serve(statusWrite, stdinRead)
			err := handshake(statusRead, stdinWrite, testToken, timeout)
			if err == nil {
				t.Fatal("handshake = nil, want an error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), testToken) {
				t.Errorf("error %q contains the token", err)
			}

			// Whatever happened, stdin of serve is closed, with or without the token.
			data, readErr := io.ReadAll(stdinRead)
			if readErr != nil {
				t.Fatalf("reading what serve received: %v", readErr)
			}
			wantData := ""
			if tt.wantToken {
				wantData = testToken + "\n"
			}
			if string(data) != wantData {
				t.Errorf("serve received %q, want %q", data, wantData)
			}
		})
	}
}

// writeScript writes an executable shell script that plays the serve process.
func writeScript(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not run on Windows")
	}
	path := filepath.Join(t.TempDir(), "fake-docbase")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	return path
}

// startFake runs Start with a script as the docbase executable.
func startFake(t *testing.T, script string) (pid int, socket string, err error) {
	t.Helper()

	dir := shortTempDir(t)
	socket = filepath.Join(dir, "broker.sock")
	pid, err = Start(StartOptions{Exe: writeScript(t, script), Team: "example", Token: testToken, SocketPath: socket, TTL: time.Minute})

	return pid, socket, err
}

// The scripts are called with: broker serve --team T --socket S --ttl D, so $6 is the socket.
func TestStartHandsOverTheTokenSafely(t *testing.T) {
	t.Setenv("DOCBASE_API_TOKEN", testToken)
	t.Setenv("HTTPS_PROXY", "http://proxy.example.com:3128")

	pid, socket, err := startFake(t, `
echo hardened >&3
read token
printf '%s' "$token" > "$6.token"
env > "$6.env"
printf '%s\n' "$@" > "$6.args"
echo ready >&3
`)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if pid <= 0 {
		t.Errorf("pid = %d, want a process id", pid)
	}

	// The script finished writing before it said ready, except for nothing: all files are there.
	token, err := os.ReadFile(socket + ".token")
	if err != nil {
		t.Fatalf("the fake serve did not run to the end: %v", err)
	}
	if string(token) != testToken {
		t.Errorf("serve received the token %q, want %q", token, testToken)
	}
	env, _ := os.ReadFile(socket + ".env")
	args, _ := os.ReadFile(socket + ".args")
	for name, content := range map[string][]byte{"environment": env, "arguments": args} {
		if strings.Contains(string(content), testToken) {
			t.Errorf("the %s of serve contain the token:\n%s", name, content)
		}
	}
	if strings.Contains(string(env), "DOCBASE_") || strings.Contains(string(env), "HTTPS_PROXY") {
		t.Errorf("the environment of serve is not minimal:\n%s", env)
	}
	if got, want := strings.Fields(string(args)), []string{"broker", "serve", "--team", "example", "--socket", socket, "--ttl", "1m0s"}; !slices.Equal(got, want) {
		t.Errorf("serve was called with %q, want %q", got, want)
	}
}

func TestStartFailures(t *testing.T) {
	short := func(t *testing.T) {
		original := readyTimeout
		readyTimeout = 300 * time.Millisecond
		t.Cleanup(func() { readyTimeout = original })
	}

	t.Run("serve reports an error", func(t *testing.T) {
		_, _, err := startFake(t, `echo hardened >&3; read token; echo "error: nothing works" >&3; exit 1`)
		if err == nil || !strings.Contains(err.Error(), "nothing works") {
			t.Errorf("got %v, want serve's message", err)
		}
	})

	t.Run("serve exits at once", func(t *testing.T) {
		_, _, err := startFake(t, `exit 3`)
		if err == nil || !strings.Contains(err.Error(), "exited before it hardened itself") || !strings.Contains(err.Error(), "exit status 3") {
			t.Errorf("got %v, want the exit error with the status", err)
		}
	})

	t.Run("serve never answers", func(t *testing.T) {
		short(t)
		started := time.Now()
		_, _, err := startFake(t, `exec sleep 30`)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("got %v, want the timeout error", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Errorf("Start took %s", time.Since(started))
		}
	})

	t.Run("serve hardens but never gets ready", func(t *testing.T) {
		short(t)
		_, _, err := startFake(t, `echo hardened >&3; exec sleep 30`)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("got %v, want the timeout error", err)
		}
	})

	t.Run("executable that does not exist", func(t *testing.T) {
		_, err := Start(StartOptions{Exe: "/nonexistent/docbase", Team: "example", Token: testToken, SocketPath: "/tmp/x/broker.sock", TTL: time.Minute})
		if err == nil || !strings.Contains(err.Error(), "failed to start the broker") {
			t.Errorf("got %v, want the start error", err)
		}
	})

	t.Run("token that cannot be sent", func(t *testing.T) {
		_, err := Start(StartOptions{Exe: "/bin/true", Team: "example", Token: "has space", SocketPath: "/tmp/x/broker.sock", TTL: time.Minute})
		if err == nil || strings.Contains(err.Error(), "has space") {
			t.Errorf("got %v, want an error without the token", err)
		}
	})
}

// A failed start leaves no process behind, even one that does not exit by itself.
func TestStartKillsAServeThatFailed(t *testing.T) {
	pidFile := filepath.Join(shortTempDir(t), "pid")
	_, _, err := startFake(t, `echo $$ > `+pidFile+`; echo hardened >&3; read token; echo "error: giving up" >&3; exec sleep 30`)
	if err == nil || !strings.Contains(err.Error(), "giving up") {
		t.Fatalf("Start = %v, want serve's error", err)
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the script did not run: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("pid file: %v", err)
	}

	// Start killed and reaped the process, so that signal 0 finds nobody.
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := process.Signal(syscall.Signal(0)); err == nil {
		_ = process.Kill()
		t.Errorf("process %d is still running after the failed start", pid)
	}
}
