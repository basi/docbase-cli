package broker

import (
	"errors"
	"net"
	"sync"
	"syscall"
)

// limitListener accepts at most n connections at a time: a connection counts
// until it is closed, and Accept waits while n are open. It does what
// golang.org/x/net/netutil.LimitListener does, except that its connections keep
// SyscallConn. The lookup of the user on the other end of a connection needs the
// file descriptor, and LimitListener's own connection type hides it, which would
// make the broker refuse every request.
type limitListener struct {
	net.Listener
	sem       chan struct{}
	done      chan struct{} // closed when the listener is closed
	closeOnce sync.Once
}

// newLimitListener wraps the listener. At least one connection is allowed.
func newLimitListener(l net.Listener, n int) net.Listener {
	return &limitListener{Listener: l, sem: make(chan struct{}, max(n, 1)), done: make(chan struct{})}
}

func (l *limitListener) Accept() (net.Conn, error) {
	select {
	case l.sem <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}

	conn, err := l.Listener.Accept()
	if err != nil {
		<-l.sem
		return nil, err
	}

	return &limitConn{Conn: conn, release: func() { <-l.sem }}, nil
}

func (l *limitListener) Close() error {
	err := l.Listener.Close()
	l.closeOnce.Do(func() { close(l.done) })

	return err
}

// limitConn gives its place back to the listener when it is closed.
type limitConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)

	return err
}

// CloseWrite shuts down the writing side of the connection that is wrapped, as
// net/http does before it closes a connection whose request it did not read to the end.
func (c *limitConn) CloseWrite() error {
	if inner, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return inner.CloseWrite()
	}

	return nil
}

// SyscallConn returns the file descriptor of the connection that is wrapped.
func (c *limitConn) SyscallConn() (syscall.RawConn, error) {
	inner, ok := c.Conn.(syscall.Conn)
	if !ok {
		return nil, errors.New("the connection has no file descriptor")
	}

	return inner.SyscallConn()
}
