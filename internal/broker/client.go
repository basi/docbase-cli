package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	// BaseURL is the base URL clients use for requests that travel over the
	// socket. The host name is not resolved; the transport dials the socket.
	BaseURL = "http://docbase-broker"

	// managementTimeout bounds a status or stop request.
	managementTimeout = 5 * time.Second
	// maxManagementResponse caps what is read from the answer to such a request.
	maxManagementResponse = 64 << 10
	// maxErrorDetail caps the characters of a broker's error message that are shown.
	maxErrorDetail = 200
)

// NewTransport returns an http.RoundTripper that sends every request to the
// broker listening on the Unix socket at socketPath, whatever the URL says.
func NewTransport(socketPath string) http.RoundTripper {
	return newTransport(socketPath)
}

func newTransport(socketPath string) *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableCompression: true,
		MaxIdleConns:       4,
		IdleConnTimeout:    30 * time.Second,
	}
}

// Status asks the broker at socketPath for its team and expiry time.
func Status(socketPath string) (Info, error) {
	body, err := manage(socketPath, http.MethodGet, statusPath, http.StatusOK)
	if err != nil {
		return Info{}, err
	}

	var info Info
	if err := json.Unmarshal(body, &info); err != nil {
		return Info{}, fmt.Errorf("unexpected answer from the broker at %s: %w", socketPath, err)
	}
	// The team ends up in request paths, so a broker is not believed blindly.
	if err := ValidTeam(info.Team); err != nil {
		return Info{}, fmt.Errorf("unexpected answer from the broker at %s: %w", socketPath, err)
	}

	return info, nil
}

// Stop asks the broker at socketPath to shut down.
func Stop(socketPath string) error {
	_, err := manage(socketPath, http.MethodPost, stopPath, http.StatusAccepted)

	return err
}

// manage sends a management request and returns the body of the answer, which
// must have the wanted status.
func manage(socketPath, method, path string, wantStatus int) ([]byte, error) {
	transport := newTransport(socketPath)
	defer transport.CloseIdleConnections()

	client := &http.Client{Transport: transport, Timeout: managementTimeout}
	req, err := http.NewRequest(method, BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("cannot reach the DocBase broker at %s: %w", socketPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManagementResponse))
	if err != nil {
		return nil, fmt.Errorf("failed to read the answer of the DocBase broker at %s: %w", socketPath, err)
	}
	if resp.StatusCode != wantStatus {
		return nil, fmt.Errorf("the DocBase broker at %s answered %s%s", socketPath, resp.Status, errorDetail(body))
	}

	return body, nil
}

// errorDetail returns ": <message>" for a broker error response, or "".
func errorDetail(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Error == "" {
		return ""
	}
	if message := []rune(payload.Error); len(message) > maxErrorDetail {
		payload.Error = string(message[:maxErrorDetail]) + "..."
	}

	return ": " + payload.Error
}
