//go:build !linux

package broker

import (
	"errors"
	"net"
)

// peerUID cannot look up the peer of a connection on this platform, so that every
// request is refused. The broker runs on Linux only.
func peerUID(net.Conn) (uint32, error) {
	return 0, errors.New("peer credentials are not supported on this platform")
}
