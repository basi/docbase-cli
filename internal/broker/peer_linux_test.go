//go:build linux

package broker

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeerUID(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "peer.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- conn
	}()
	client, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = client.Close() }()
	server, ok := <-accepted
	if !ok {
		t.Fatal("Accept failed")
	}
	defer func() { _ = server.Close() }()

	// Both ends see this very process, whose user is the current one.
	for name, conn := range map[string]net.Conn{"server side": server, "client side": client} {
		uid, err := peerUID(conn)
		if err != nil {
			t.Errorf("%s: peerUID: %v", name, err)
			continue
		}
		if uid != uint32(os.Getuid()) {
			t.Errorf("%s: peerUID = %d, want %d", name, uid, os.Getuid())
		}
	}
}

func TestPeerUIDNeedsAUnixConnection(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = a.Close(); _ = b.Close() }()

	if _, err := peerUID(a); err == nil {
		t.Error("peerUID accepted a connection that is not a Unix socket")
	}
}

// With the real lookup, a broker serves the user that runs it and nobody else.
func TestBrokerChecksThePeerWithTheRealLookup(t *testing.T) {
	same := newTestBroker(t, time.Hour, nil, func(s *Server) { s.peerUID = peerUID })
	if _, err := Status(same.socket); err != nil {
		t.Errorf("Status: %v", err)
	}

	// A broker that expects another user refuses this process.
	other := newTestBroker(t, time.Hour, nil, func(s *Server) {
		s.peerUID = peerUID
		s.uid++
	})
	_, err := Status(other.socket)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("Status = %v, want a 403 answer", err)
	}
	if got := other.upstream.seen(); len(got) != 0 {
		t.Errorf("the upstream saw %v", got)
	}
}

// The connections of the limiting listener, which the broker serves, are looked up too.
func TestPeerUIDThroughTheLimitListener(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "peer.sock")
	inner, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := newLimitListener(inner, 4)
	defer func() { _ = listener.Close() }()

	client, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = client.Close() }()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = conn.Close() }()

	uid, err := peerUID(conn)
	if err != nil || uid != uint32(os.Getuid()) {
		t.Errorf("peerUID = %d, %v; want %d", uid, err, os.Getuid())
	}
}

// Only a Unix socket has peer credentials of this kind.
func TestPeerUIDRefusesATCPConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback network here: %v", err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- conn
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = client.Close() }()
	server, ok := <-accepted
	if !ok {
		t.Fatal("Accept failed")
	}
	defer func() { _ = server.Close() }()

	if uid, err := peerUID(server); err == nil {
		t.Errorf("peerUID of a TCP connection = %d, nil; want an error", uid)
	}
}
