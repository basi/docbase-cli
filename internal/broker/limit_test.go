package broker

import (
	"errors"
	"net"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// acceptResult is what one call of Accept returned.
type acceptResult struct {
	conn net.Conn
	err  error
}

func TestLimitListener(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "limit.sock")
	inner, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := newLimitListener(inner, 2)
	defer func() { _ = listener.Close() }()

	// Accept in the background, one connection at a time.
	accepted := make(chan acceptResult)
	go func() {
		for {
			conn, err := listener.Accept()
			accepted <- acceptResult{conn, err}
			if err != nil {
				return
			}
		}
	}()
	dial := func() net.Conn {
		t.Helper()
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	next := func(wait time.Duration) (acceptResult, bool) {
		select {
		case res := <-accepted:
			return res, true
		case <-time.After(wait):
			return acceptResult{}, false
		}
	}

	// Two connections are accepted.
	dial()
	dial()
	var served []net.Conn
	for range 2 {
		res, ok := next(5 * time.Second)
		if !ok || res.err != nil {
			t.Fatalf("Accept = %v, %v; want a connection", res.err, ok)
		}
		served = append(served, res.conn)
	}

	// The third waits.
	dial()
	if res, ok := next(200 * time.Millisecond); ok {
		t.Fatalf("a third connection was accepted (%v, %v) while two were open", res.conn, res.err)
	}

	// A close gives the place back, once: closing twice must not free a second place.
	_ = served[0].Close()
	_ = served[0].Close()
	res, ok := next(5 * time.Second)
	if !ok || res.err != nil {
		t.Fatalf("the waiting connection was not accepted after a close: %v, %v", res.err, ok)
	}
	served = append(served, res.conn)

	dial()
	if res, ok := next(200 * time.Millisecond); ok {
		t.Fatalf("a fourth connection was accepted (%v, %v): the double close freed two places", res.conn, res.err)
	}

	// Closing the listener ends the wait.
	_ = listener.Close()
	res, ok = next(5 * time.Second)
	if !ok || res.err == nil {
		t.Errorf("Accept after Close = %v, %v; want an error", res.err, ok)
	}
	for _, conn := range served {
		_ = conn.Close()
	}
}

func TestLimitListenerCloseUnblocksAWaitingAccept(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "limit.sock")
	inner, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := newLimitListener(inner, 1)

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

	// The only place is taken, so this call waits for a place.
	done := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Accept returned %v while the place was taken", err)
	case <-time.After(100 * time.Millisecond):
	}

	_ = listener.Close()
	_ = listener.Close() // twice is fine
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("Accept = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept still waits after Close")
	}
}

// The connections of the limiting listener keep their file descriptor, which the lookup of the peer needs.
func TestLimitConnHasAFileDescriptor(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "limit.sock")
	inner, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := newLimitListener(inner, 2)
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

	withFD, ok := conn.(syscall.Conn)
	if !ok {
		t.Fatalf("%T is not a syscall.Conn", conn)
	}
	raw, err := withFD.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}
	var fd uintptr
	if err := raw.Control(func(f uintptr) { fd = f }); err != nil || fd == 0 {
		t.Errorf("Control: fd = %d, err = %v", fd, err)
	}

	// A connection without a file descriptor is an error, not a panic.
	pipeEnd, other := net.Pipe()
	defer func() { _ = pipeEnd.Close(); _ = other.Close() }()
	if _, err := (&limitConn{Conn: pipeEnd, release: func() {}}).SyscallConn(); err == nil {
		t.Error("SyscallConn of a connection without a file descriptor succeeded")
	}
}

func TestLimitListenerIsUsedConcurrently(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "limit.sock")
	inner, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	listener := newLimitListener(inner, 3)
	defer func() { _ = listener.Close() }()

	var mu sync.Mutex
	open, most := 0, 0
	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			open++
			most = max(most, open)
			mu.Unlock()
			wg.Go(func() {
				time.Sleep(20 * time.Millisecond)
				mu.Lock()
				open--
				mu.Unlock()
				_ = conn.Close()
			})
		}
	}()

	for range 12 {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer func() { _ = conn.Close() }()
	}
	time.Sleep(500 * time.Millisecond)
	_ = listener.Close()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if most > 3 {
		t.Errorf("%d connections were open at once, want at most 3", most)
	}
}
