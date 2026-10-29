package broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ServeOptions says what a broker process serves and where its input and output are.
type ServeOptions struct {
	Team       string
	SocketPath string
	TTL        time.Duration
	Version    string

	// Stdin delivers the access token and is closed once it was read.
	Stdin io.ReadCloser
	// Status receives the progress lines (file descriptor 3 of the process); it is
	// closed when the broker is ready.
	Status io.WriteCloser

	// The fields below are set by tests.
	harden  func() error
	peerUID func(net.Conn) (uint32, error)
}

// RunServe runs a broker in this process: it hardens the process, takes the
// access token from Stdin, listens on the socket and serves until the TTL
// expires, ctx is done or a stop request arrives. It reports its progress on
// Status (see start.go) and any failure before it is ready as an "error:" line.
func RunServe(ctx context.Context, opts ServeOptions) error {
	status := &statusWriter{w: opts.Status}
	defer status.close()

	err := serve(ctx, opts, status)
	if err != nil {
		_ = status.send(msgErrorLabel + err.Error())
	}

	return err
}

func serve(ctx context.Context, opts ServeOptions, status *statusWriter) error {
	if err := ValidTeam(opts.Team); err != nil {
		return err
	}
	if err := CheckSocketPath(opts.SocketPath); err != nil {
		return err
	}
	if opts.TTL <= 0 {
		return errors.New("ttl must be positive")
	}

	// The token is not read before the process is hardened, and the process holds nothing secret before that.
	hardenProcess := opts.harden
	if hardenProcess == nil {
		hardenProcess = harden
	}
	if err := hardenProcess(); err != nil {
		return fmt.Errorf("failed to harden the process: %w", err)
	}
	if err := status.send(msgHardened); err != nil {
		return fmt.Errorf("failed to report to start: %w", err)
	}

	token, err := receiveToken(ctx, opts.Stdin)
	if err != nil {
		return err
	}
	server, err := NewServer(Config{Team: opts.Team, Token: token, Version: opts.Version, TTL: opts.TTL})
	if err != nil {
		return err
	}
	if opts.peerUID != nil {
		server.peerUID = opts.peerUID
	}

	uid := os.Getuid()
	dir := filepath.Dir(opts.SocketPath)
	if err := validateSocketDir(dir, uid); err != nil {
		return err
	}
	listener, err := net.Listen("unix", opts.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on the socket: %w", err)
	}
	// The directory was made for this broker, so it goes with the socket.
	defer func() {
		_ = listener.Close()
		_ = os.Remove(opts.SocketPath)
		_ = os.Remove(dir)
	}()

	if err := os.Chmod(opts.SocketPath, socketMode); err != nil {
		return fmt.Errorf("failed to set the mode of the socket: %w", err)
	}
	if err := validateSocket(opts.SocketPath, uid); err != nil {
		return err
	}

	if err := status.send(msgReady); err != nil {
		return fmt.Errorf("failed to report to start: %w", err)
	}
	status.close()

	return server.Serve(ctx, listener)
}

// receiveToken reads the access token from r, giving up when ctx is done or when
// start does not deliver it in time.
func receiveToken(ctx context.Context, r io.ReadCloser) (string, error) {
	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		token, err := readToken(r)
		done <- result{token, err}
	}()

	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()

	select {
	case res := <-done:
		return res.token, res.err
	case <-ctx.Done():
		return "", errors.New("interrupted while waiting for the access token")
	case <-timer.C:
		return "", errors.New("timed out waiting for the access token")
	}
}

// statusWriter writes the progress lines of serve.
type statusWriter struct {
	w      io.WriteCloser
	closed bool
}

// send writes one line. It does nothing once the writer is closed.
func (s *statusWriter) send(message string) error {
	if s.closed || s.w == nil {
		return nil
	}
	// A line break inside a message would end the line early.
	message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	_, err := io.WriteString(s.w, message+"\n")

	return err
}

func (s *statusWriter) close() {
	if s.closed || s.w == nil {
		return
	}
	s.closed = true
	_ = s.w.Close()
}
