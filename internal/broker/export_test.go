package broker

import (
	"net"
	"net/http"
)

// Hooks for the tests in package broker_test, which cannot reach the fields.

// SetUpstream points the server at another API, such as a test server, and applies
// the settings every upstream client has.
func (s *Server) SetUpstream(base string, client *http.Client) {
	s.upstreamBase = base
	s.upstream = hardenUpstream(client)
}

// SetPeerUID replaces the lookup of the user on the other end of a connection.
func (s *Server) SetPeerUID(lookup func(net.Conn) (uint32, error)) {
	s.peerUID = lookup
}
