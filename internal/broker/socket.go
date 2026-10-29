package broker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// socketDirPattern is the name pattern of the private directory of a socket.
	socketDirPattern = "docbase-broker-"
	// socketName is the name of the socket inside that directory.
	socketName = "broker.sock"
	// maxSocketPath is the longest socket path accepted. The limit of the OS is
	// about 104 bytes (108 on Linux), counting the terminating NUL.
	maxSocketPath = 100

	dirMode    fs.FileMode = 0o700
	socketMode fs.FileMode = 0o600
)

// SocketBase returns the directory in which the private directory of a socket is
// created: $RUNNER_TEMP, else $XDG_RUNTIME_DIR, else the temporary directory.
func SocketBase() string {
	for _, name := range []string{"RUNNER_TEMP", "XDG_RUNTIME_DIR"} {
		if dir := os.Getenv(name); dir != "" {
			return dir
		}
	}

	return os.TempDir()
}

// CreateSocketDir creates a private (mode 0700) directory in base for the socket
// of a broker and returns it with the socket path inside it. base must be an
// absolute path.
func CreateSocketDir(base string) (dir, socket string, err error) {
	if !filepath.IsAbs(base) {
		return "", "", fmt.Errorf("the socket directory %q is not an absolute path", base)
	}

	created, err := os.MkdirTemp(base, socketDirPattern)
	if err != nil {
		return "", "", fmt.Errorf("failed to create the socket directory: %w", err)
	}

	// MkdirTemp applies the umask, and a setgid parent directory is inherited: make the mode exact.
	err = os.Chmod(created, dirMode)
	if err != nil {
		err = fmt.Errorf("failed to set the mode of the socket directory: %w", err)
	} else if err = CheckSocketPath(filepath.Join(created, socketName)); err == nil {
		err = validateSocketDir(created, os.Getuid())
	}
	if err != nil {
		_ = os.Remove(created)
		return "", "", err
	}

	return created, filepath.Join(created, socketName), nil
}

// CheckSocketPath checks that path is usable as the socket path of a broker and as a
// line of output: absolute, short enough for a socket address and without control
// characters.
func CheckSocketPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("the socket path %q is not absolute", path)
	}
	if len(path) > maxSocketPath {
		return fmt.Errorf("the socket path is longer than %d bytes (%d): set RUNNER_TEMP or XDG_RUNTIME_DIR to a shorter directory", maxSocketPath, len(path))
	}
	for i := range len(path) {
		if c := path[i]; c < ' ' || c == 0x7f {
			return errors.New("the socket path contains control characters")
		}
	}

	return nil
}

// validateSocketDir checks that dir is a real directory (not a symlink) that
// belongs to uid and has mode 0700, so that nobody else can reach or replace the socket in it.
func validateSocketDir(dir string, uid int) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("socket directory: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("socket directory %s is a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("socket directory %s is not a directory", dir)
	}
	if err := checkOwner(info, uid, "socket directory "+dir); err != nil {
		return err
	}
	if mode := exactMode(info); mode != dirMode {
		return fmt.Errorf("socket directory %s has mode %04o, want %04o", dir, mode, dirMode)
	}

	return nil
}

// validateSocket checks that path is a socket that belongs to uid and has mode 0600.
func validateSocket(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("socket: %w", err)
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s is not a socket", path)
	}
	if err := checkOwner(info, uid, "socket "+path); err != nil {
		return err
	}
	if mode := exactMode(info); mode != socketMode {
		return fmt.Errorf("socket %s has mode %04o, want %04o", path, mode, socketMode)
	}

	return nil
}

// exactMode returns the permission bits of the file together with the setuid,
// setgid and sticky bits, so that any of them makes the mode differ from the expected one.
func exactMode(info fs.FileInfo) fs.FileMode {
	return info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
}

// checkOwner checks that the file belongs to uid.
func checkOwner(info fs.FileInfo, uid int, what string) error {
	owner, ok := ownerUID(info)
	if !ok {
		return fmt.Errorf("cannot determine the owner of the %s", what)
	}
	if owner != uid {
		return fmt.Errorf("the %s belongs to uid %d, not %d", what, owner, uid)
	}

	return nil
}
