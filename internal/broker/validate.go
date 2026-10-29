package broker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/basi/docbase-cli/pkg/docbase"
)

// maxValidationResponse caps what is read from the answer to a validation request.
const maxValidationResponse = 64 << 10

// Version is the version of the CLI. It goes into the User-Agent header of the
// request of ValidateToken. cmd/broker sets it.
var Version = "dev"

// validationAPI says where ValidateToken sends its request and with what: the
// base of the DocBase API and the client that the broker itself uses to reach it.
// Tests replace it.
var validationAPI = func() (base string, client *http.Client) {
	return docbase.BaseURL, newUpstreamClient()
}

// ValidateToken asks the DocBase API whether it accepts the access token for the
// team. It sends one request, for the first group of the team, the way the broker
// sends its own: with the same four headers and through the same client, so no
// proxy from the environment, no timeout longer than 30 seconds and no redirect.
// A redirect is an error, because following it could send the token to another
// host. The base is the fixed DocBase API; it cannot be changed. The errors never
// contain the token.
func ValidateToken(team, token string) error {
	if err := ValidTeam(team); err != nil {
		return err
	}
	if err := ValidToken(token); err != nil {
		return err
	}

	base, client := validationAPI()
	request := target{path: "/teams/" + team + "/groups", query: "per_page=1"}
	req, err := http.NewRequest(http.MethodGet, base+request.String(), nil)
	if err != nil {
		return errors.New("failed to build the request for the DocBase API")
	}
	setUpstreamHeaders(req.Header, token, userAgent(Version))

	resp, err := client.Do(req)
	if err != nil {
		return validationRequestError(err, token)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return errors.New("the DocBase API answered with a redirect, which is not followed")
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the access token was rejected by the DocBase API (%s)", resp.Status)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("the DocBase API answered %s: check the team domain", resp.Status)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxValidationResponse))
		return fmt.Errorf("the DocBase API answered %s%s", resp.Status, redact(errorDetail(body), token))
	}
}

// validationRequestError describes why a validation request got no answer.
func validationRequestError(err error, token string) error {
	if isTimeout(err) {
		return errors.New("the DocBase API did not answer in time")
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return fmt.Errorf("the DocBase API could not be reached: %s", redact(err.Error(), token))
}
