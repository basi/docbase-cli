package broker

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketBase(t *testing.T) {
	t.Run("runner temp comes first", func(t *testing.T) {
		t.Setenv("RUNNER_TEMP", "/runner/temp")
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		if got := SocketBase(); got != "/runner/temp" {
			t.Errorf("SocketBase() = %q, want /runner/temp", got)
		}
	})
	t.Run("then the runtime directory", func(t *testing.T) {
		t.Setenv("RUNNER_TEMP", "")
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		if got := SocketBase(); got != "/run/user/1000" {
			t.Errorf("SocketBase() = %q, want /run/user/1000", got)
		}
	})
	t.Run("then the temporary directory", func(t *testing.T) {
		t.Setenv("RUNNER_TEMP", "")
		t.Setenv("XDG_RUNTIME_DIR", "")
		t.Setenv("TMPDIR", "/custom/tmp")
		if got := SocketBase(); got != os.TempDir() {
			t.Errorf("SocketBase() = %q, want %q", got, os.TempDir())
		}
	})
}

func TestCreateSocketDir(t *testing.T) {
	base := shortTempDir(t)

	dir, socket, err := CreateSocketDir(base)
	if err != nil {
		t.Fatalf("CreateSocketDir: %v", err)
	}
	if filepath.Dir(dir) != base || !strings.HasPrefix(filepath.Base(dir), "docbase-broker-") {
		t.Errorf("dir = %q, want a docbase-broker-* directory in %q", dir, base)
	}
	if want := filepath.Join(dir, "broker.sock"); socket != want {
		t.Errorf("socket = %q, want %q", socket, want)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Stat(%q) = %v, %v", dir, info, err)
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Errorf("mode = %o, want 700", mode)
	}
	if err := validateSocketDir(dir, os.Getuid()); err != nil {
		t.Errorf("validateSocketDir: %v", err)
	}

	// A new directory every time.
	other, _, err := CreateSocketDir(base)
	if err != nil || other == dir {
		t.Errorf("second CreateSocketDir = %q, %v; want another directory", other, err)
	}
}

func TestCreateSocketDirIgnoresTheUmask(t *testing.T) {
	// A parent that passes its group on to new directories must not leak into the mode.
	base := shortTempDir(t)
	if err := os.Chmod(base, 0o700|os.ModeSetgid); err != nil {
		t.Skipf("cannot set the setgid bit here: %v", err)
	}

	dir, _, err := CreateSocketDir(base)
	if err != nil {
		t.Fatalf("CreateSocketDir: %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if mode := exactMode(info); mode != 0o700 {
		t.Errorf("mode = %v, want 0700 without special bits", mode)
	}
}

func TestCreateSocketDirErrors(t *testing.T) {
	t.Run("relative base", func(t *testing.T) {
		if _, _, err := CreateSocketDir("relative/dir"); err == nil {
			t.Error("a relative base was accepted")
		}
	})

	t.Run("missing base", func(t *testing.T) {
		if _, _, err := CreateSocketDir(filepath.Join(shortTempDir(t), "missing")); err == nil {
			t.Error("a missing base was accepted")
		}
	})

	t.Run("path too long", func(t *testing.T) {
		base := filepath.Join(shortTempDir(t), strings.Repeat("d", 80))
		if err := os.Mkdir(base, 0o700); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}

		_, _, err := CreateSocketDir(base)
		if err == nil || !strings.Contains(err.Error(), "longer than") {
			t.Errorf("got %v, want the path length error", err)
		}
		// Nothing is left behind.
		if entries, _ := os.ReadDir(base); len(entries) != 0 {
			t.Errorf("left behind: %v", entries)
		}
	})

	t.Run("line break in the path", func(t *testing.T) {
		// The path is printed as a line for $GITHUB_ENV, where a line break would add a variable.
		base := filepath.Join(shortTempDir(t), "a\nINJECTED=1")
		if err := os.Mkdir(base, 0o700); err != nil {
			t.Skipf("cannot create such a directory here: %v", err)
		}

		_, _, err := CreateSocketDir(base)
		if err == nil || !strings.Contains(err.Error(), "control characters") {
			t.Errorf("got %v, want the control character error", err)
		}
		if entries, _ := os.ReadDir(base); len(entries) != 0 {
			t.Errorf("left behind: %v", entries)
		}
	})
}

func TestCheckSocketPath(t *testing.T) {
	long := "/" + strings.Repeat("a", 99)

	for _, path := range []string{"/tmp/x/broker.sock", long} {
		if err := CheckSocketPath(path); err != nil {
			t.Errorf("CheckSocketPath(%q) = %v, want nil", path, err)
		}
	}
	for _, path := range []string{"", "broker.sock", "./broker.sock", long + "a", "/tmp/a\nb", "/tmp/a\rb", "/tmp/a\x00b", "/tmp/a\x7fb"} {
		if err := CheckSocketPath(path); err == nil {
			t.Errorf("CheckSocketPath(%q) = nil, want an error", path)
		}
	}
}

func TestValidateSocketDir(t *testing.T) {
	uid := os.Getuid()
	newDir := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(shortTempDir(t), "sock")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		return dir
	}

	t.Run("private directory", func(t *testing.T) {
		if err := validateSocketDir(newDir(t), uid); err != nil {
			t.Errorf("got %v, want nil", err)
		}
	})

	t.Run("symlink to a private directory", func(t *testing.T) {
		dir := newDir(t)
		link := filepath.Join(filepath.Dir(dir), "link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatalf("Symlink: %v", err)
		}
		err := validateSocketDir(link, uid)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Errorf("got %v, want the symlink error", err)
		}
	})

	t.Run("not a directory", func(t *testing.T) {
		file := filepath.Join(shortTempDir(t), "file")
		if err := os.WriteFile(file, nil, 0o700); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := validateSocketDir(file, uid); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Errorf("got %v, want the not-a-directory error", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		if err := validateSocketDir(filepath.Join(shortTempDir(t), "missing"), uid); err == nil {
			t.Error("a missing directory was accepted")
		}
	})

	t.Run("modes other than 0700", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o750, 0o755, 0o770, 0o777, 0o701, 0o500, 0o600, 0o000, 0o700 | os.ModeSticky, 0o700 | os.ModeSetgid} {
			dir := newDir(t)
			if err := os.Chmod(dir, mode); err != nil {
				t.Logf("cannot set mode %v here: %v", mode, err)
				continue
			}
			// The mode may not be kept as set (some systems drop special bits), so look at what is there.
			info, err := os.Lstat(dir)
			if err != nil {
				t.Fatalf("Lstat: %v", err)
			}
			if exactMode(info) == 0o700 {
				continue
			}
			if err := validateSocketDir(dir, uid); err == nil || !strings.Contains(err.Error(), "mode") {
				t.Errorf("mode %v: got %v, want the mode error", mode, err)
			}
			_ = os.Chmod(dir, 0o700)
		}
	})

	t.Run("another owner", func(t *testing.T) {
		err := validateSocketDir(newDir(t), uid+1)
		if err == nil || !strings.Contains(err.Error(), "belongs to uid") {
			t.Errorf("got %v, want the owner error", err)
		}
	})

	t.Run("really owned by someone else", func(t *testing.T) {
		if uid != 0 {
			t.Skip("changing the owner needs root")
		}
		dir := newDir(t)
		if err := os.Chown(dir, 12345, 12345); err != nil {
			t.Skipf("cannot change the owner here: %v", err)
		}
		if err := validateSocketDir(dir, uid); err == nil || !strings.Contains(err.Error(), "belongs to uid 12345") {
			t.Errorf("got %v, want the owner error", err)
		}
	})
}

func TestValidateSocket(t *testing.T) {
	uid := os.Getuid()
	listen := func(t *testing.T) string {
		t.Helper()
		path := filepath.Join(shortTempDir(t), "s")
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		return path
	}

	t.Run("socket with mode 0600", func(t *testing.T) {
		path := listen(t)
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		if err := validateSocket(path, uid); err != nil {
			t.Errorf("got %v, want nil", err)
		}
	})

	t.Run("other modes", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o660, 0o666, 0o644, 0o700, 0o400} {
			path := listen(t)
			if err := os.Chmod(path, mode); err != nil {
				t.Fatalf("Chmod: %v", err)
			}
			if err := validateSocket(path, uid); err == nil || !strings.Contains(err.Error(), "mode") {
				t.Errorf("mode %o: got %v, want the mode error", mode, err)
			}
		}
	})

	t.Run("another owner", func(t *testing.T) {
		path := listen(t)
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		if err := validateSocket(path, uid+1); err == nil || !strings.Contains(err.Error(), "belongs to uid") {
			t.Errorf("got %v, want the owner error", err)
		}
	})

	t.Run("not a socket", func(t *testing.T) {
		file := filepath.Join(shortTempDir(t), "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := validateSocket(file, uid); err == nil || !strings.Contains(err.Error(), "not a socket") {
			t.Errorf("got %v, want the not-a-socket error", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		if err := validateSocket(filepath.Join(shortTempDir(t), "missing"), uid); err == nil {
			t.Error("a missing socket was accepted")
		}
	})
}
