// Package broker lets a process use the DocBase API without being able to read
// the access token. A broker process holds the token in memory and serves
// read-only requests for one team on a Unix socket; its clients only know the
// socket path. See policy.go for what is allowed.
//
// The broker defends against a non-root process of the same user. It does not
// defend against root.
//
// It keeps the token only from processes that start after it reported ready. A
// process of the same user that is already running while the broker starts can
// take the token on its way in (it can read the standard input of the new
// process before that is hardened, or answer in its place on the status pipe),
// just as it can read the environment or the credential store the token came
// from. So start the broker before any untrusted process, such as an agent,
// runs. On a machine where other programs of the same user are running, those
// programs can read the token anyway.
package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/basi/docbase-cli/pkg/docbase"
)

const (
	// TTL limits: how long a broker may run.
	DefaultTTL = time.Hour
	MinTTL     = time.Minute
	MaxTTL     = 12 * time.Hour

	// maxInflight is the most upstream requests in flight at once.
	maxInflight = 4
	// maxBodyBytes caps the upstream response body.
	maxBodyBytes = 32 << 20
	// upstreamTimeout bounds one upstream request.
	upstreamTimeout = 30 * time.Second

	// maxConnections is the most connections served at once. A client needs one or
	// two; the cap only keeps a flood of connections from using up the descriptors.
	maxConnections = 16

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 10 * time.Second    // the whole request; nothing here has a body worth waiting for
	writeTimeout      = 2 * upstreamTimeout // the wait for the upstream and the answer to the client
	idleTimeout       = 30 * time.Second
	maxHeaderBytes    = 16 << 10
	shutdownTimeout   = 5 * time.Second

	// statusPath and stopPath are the management endpoints. They are never forwarded.
	statusPath = "/-/status"
	stopPath   = "/-/stop"
)

// CheckTTL returns an error unless ttl is within the allowed range.
func CheckTTL(ttl time.Duration) error {
	if ttl < MinTTL || ttl > MaxTTL {
		return fmt.Errorf("ttl must be between %s and %s", MinTTL, MaxTTL)
	}

	return nil
}

// Info is what a broker reports about itself.
type Info struct {
	Team      string    `json:"team"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Config holds what a Server needs.
type Config struct {
	Team    string
	Token   string // never logged or returned in a response
	Version string // the CLI version, for the User-Agent header
	TTL     time.Duration
}

// Server serves read-only DocBase requests for one team. The zero value is not usable;
// call NewServer.
type Server struct {
	team      string
	token     string
	userAgent string
	expiresAt time.Time

	// The fields below are set by tests.
	upstreamBase string
	upstream     *http.Client
	peerUID      func(net.Conn) (uint32, error)
	uid          uint32
	maxBody      int64
	maxConns     int

	inflight chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
}

// NewServer creates a Server that expires cfg.TTL from now.
func NewServer(cfg Config) (*Server, error) {
	if err := ValidTeam(cfg.Team); err != nil {
		return nil, err
	}
	if err := ValidToken(cfg.Token); err != nil {
		return nil, err
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("ttl must be positive")
	}

	return &Server{
		team:         cfg.Team,
		token:        cfg.Token,
		userAgent:    userAgent(cfg.Version),
		expiresAt:    time.Now().Add(cfg.TTL),
		upstreamBase: docbase.BaseURL,
		upstream:     newUpstreamClient(),
		peerUID:      peerUID,
		uid:          uint32(os.Getuid()),
		maxBody:      maxBodyBytes,
		maxConns:     maxConnections,
		inflight:     make(chan struct{}, maxInflight),
		stop:         make(chan struct{}),
	}, nil
}

// newUpstreamClient returns the client that talks to DocBase.
func newUpstreamClient() *http.Client {
	return hardenUpstream(&http.Client{Transport: &http.Transport{
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: upstreamTimeout,
		IdleConnTimeout:       90 * time.Second,
	}})
}

// hardenUpstream applies the settings every upstream client must have: no proxy
// from the environment, no automatic decompression (the upstream sees only the
// headers the broker sets), a timeout, and no following of redirects, so that the
// token is only ever sent to the configured base.
func hardenUpstream(c *http.Client) *http.Client {
	c.Timeout = upstreamTimeout
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if t, ok := c.Transport.(*http.Transport); ok {
		t.Proxy = nil
		t.DisableCompression = true
	}

	return c
}

// Info returns the team and the expiry time of the server.
func (s *Server) Info() Info {
	return Info{Team: s.team, ExpiresAt: s.expiresAt.UTC().Truncate(time.Second)}
}

// peerKey is the context key under which ConnContext records the peer.
type peerKey struct{}

// peer is the outcome of looking up the credentials of a connection's peer.
type peer struct {
	uid uint32
	err error
}

// httpServer returns the HTTP server that serves the requests of the broker.
func (s *Server) httpServer() *http.Server {
	return &http.Server{
		Handler:           s,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          log.New(io.Discard, "", 0),
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			uid, err := s.peerUID(c)
			return context.WithValue(ctx, peerKey{}, peer{uid: uid, err: err})
		},
	}
}

// Serve serves on ln until ctx is done, the TTL expires or a stop request arrives,
// then shuts down gracefully and closes ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := s.httpServer()

	failed := make(chan error, 1)
	go func() { failed <- srv.Serve(newLimitListener(ln, s.maxConns)) }()

	expiry := time.NewTimer(time.Until(s.expiresAt))
	defer expiry.Stop()

	select {
	case <-ctx.Done():
	case <-expiry.C:
	case <-s.stop:
	case err := <-failed:
		// The listener broke without a shutdown request.
		_ = srv.Close()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
	}
	if err := <-failed; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		// Nothing here reads a request body. net/http would read the announced body
		// before it answers, and again before it lets go of the connection, and a
		// client that never sends it would keep both waiting until the read timeout.
		// So answer at once and close the connection.
		w.Header().Set("Connection", "close")
		_ = http.NewResponseController(w).SetReadDeadline(time.Now())
	}

	// Only the user that runs the broker may use it. A failed lookup is a refusal.
	if p, ok := r.Context().Value(peerKey{}).(peer); !ok || p.err != nil || p.uid != s.uid {
		s.writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	switch r.RequestURI {
	case statusPath:
		s.handleStatus(w, r)
		return
	case stopPath:
		s.handleStop(w, r)
		return
	}

	t, err := authorize(s.team, r)
	if err != nil {
		var rej *rejection
		if errors.As(err, &rej) {
			if rej.status == http.StatusMethodNotAllowed {
				w.Header().Set("Allow", http.MethodGet)
			}
			s.writeError(w, rej.status, rej.message)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.forward(w, r, t)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeError(w, http.StatusMethodNotAllowed, "only GET requests are allowed")
		return
	}

	s.writeJSON(w, http.StatusOK, s.Info())
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeError(w, http.StatusMethodNotAllowed, "only POST requests are allowed")
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	// Deliver the answer before the shutdown starts.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.stopOnce.Do(func() { close(s.stop) })
}

// errClientGone is the error of a request whose client went away before the answer.
var errClientGone = errors.New("the client went away")

// upstreamResponse is the answer of the DocBase API.
type upstreamResponse struct {
	status int
	header http.Header
	body   []byte
}

// forward sends the allowed request upstream and relays the answer. The slot of
// the request is held until the answer has been written to the client, so that
// no more than maxInflight answers are held in memory at any time, however slowly
// the clients read.
func (s *Server) forward(w http.ResponseWriter, r *http.Request, t target) {
	select {
	case s.inflight <- struct{}{}:
		defer func() { <-s.inflight }()
	default:
		s.writeError(w, http.StatusServiceUnavailable, "too many requests in flight")
		return
	}

	res, err := s.fetch(r.Context(), t)
	if err != nil {
		var rej *rejection
		switch {
		case errors.Is(err, errClientGone):
			// There is nobody to answer.
		case errors.As(err, &rej):
			s.writeError(w, rej.status, rej.message)
		default:
			s.writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Only these headers are passed on. Neither they nor the body may repeat the
	// token, whatever the upstream does with what it receives.
	header := w.Header()
	if contentType := res.header.Get("Content-Type"); isJSON(contentType) {
		header.Set("Content-Type", redact(contentType, s.token))
	} else {
		// A nil value stops net/http from guessing a type from the body.
		header["Content-Type"] = nil
	}
	for _, name := range []string{"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if value := res.header.Get(name); value != "" {
			header.Set(name, redact(value, s.token))
		}
	}
	w.WriteHeader(res.status)
	_, _ = w.Write(redactBytes(res.body, s.token))
}

// fetch sends the request upstream and reads the answer. The errors are *rejection
// values, or errClientGone.
func (s *Server) fetch(ctx context.Context, t target) (*upstreamResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.upstreamBase+t.String(), nil)
	if err != nil {
		return nil, reject(http.StatusInternalServerError, "internal error")
	}
	setUpstreamHeaders(req.Header, s.token, s.userAgent)

	resp, err := s.upstream.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return nil, errClientGone
		case isTimeout(err):
			return nil, reject(http.StatusBadGateway, "the DocBase API did not answer in time")
		default:
			return nil, reject(http.StatusBadGateway, "the DocBase API could not be reached")
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, reject(http.StatusBadGateway, "the DocBase API answered with a redirect")
	}

	// Read one byte more than the cap to tell a body at the cap from a longer one.
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.maxBody+1))
	if err != nil {
		return nil, reject(http.StatusBadGateway, "the answer of the DocBase API could not be read")
	}
	if int64(len(body)) > s.maxBody {
		return nil, reject(http.StatusBadGateway, "the answer of the DocBase API is too large")
	}

	return &upstreamResponse{status: resp.StatusCode, header: resp.Header, body: body}, nil
}

// setUpstreamHeaders sets the only headers a request to the DocBase API carries.
// Nothing is copied from a client.
func setUpstreamHeaders(header http.Header, token, userAgent string) {
	header.Set("X-DocBaseToken", token)
	header.Set("X-Api-Version", docbase.APIVersion)
	header.Set("Accept", "application/json")
	header.Set("User-Agent", userAgent)
}

// userAgent returns the User-Agent header of the requests to the DocBase API.
func userAgent(version string) string {
	return "docbase-cli-broker/" + version
}

// isJSON reports whether a Content-Type value is application/json.
func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)

	return err == nil && mediaType == "application/json"
}

// isTimeout reports whether err is a timeout.
func isTimeout(err error) bool {
	var netErr net.Error

	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}

// writeJSON writes a JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// writeError writes an error response. The message is cleaned of the token first,
// so that no code path can leak it.
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	if status >= 400 && status < 500 {
		// A refused request is not worth keeping the connection for.
		w.Header().Set("Connection", "close")
	}
	s.writeJSON(w, status, map[string]string{"error": redact(message, s.token)})
}
