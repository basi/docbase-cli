//go:build linux

package broker

import (
	"errors"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// peerUID returns the user id of the process on the other end of a Unix socket
// connection, as the kernel recorded it when the connection was made. Anything
// else is an error: on a socket of another kind the kernel answers the request for
// the credentials without an error, with the user id -1.
func peerUID(c net.Conn) (uint32, error) {
	if addr := c.LocalAddr(); addr == nil || addr.Network() != "unix" {
		return 0, errors.New("not a Unix socket connection")
	}
	conn, ok := c.(syscall.Conn)
	if !ok {
		return 0, errors.New("the connection has no file descriptor")
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}

	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credErr != nil {
		return 0, credErr
	}
	if cred.Uid == ^uint32(0) {
		return 0, errors.New("the kernel has no credentials for the peer")
	}

	return cred.Uid, nil
}
