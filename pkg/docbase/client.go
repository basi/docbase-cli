package docbase

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// BaseURL is the base URL of DocBase API
	BaseURL = "https://api.docbase.io"
	// APIVersion is the version of DocBase API
	APIVersion = "3"
	// DefaultTimeout is the default timeout for API requests
	DefaultTimeout = 30 * time.Second
)

// Client represents a DocBase API client
type Client struct {
	httpClient  *resty.Client
	TeamDomain  string
	AccessToken string
	BaseURL     string
}

// NewClient creates a new DocBase API client
func NewClient(teamDomain, accessToken string) *Client {
	client := resty.New()
	client.SetTimeout(DefaultTimeout)
	client.SetHeader("X-DocBaseToken", accessToken)
	client.SetHeader("X-Api-Version", APIVersion)
	client.SetHeader("Content-Type", "application/json")

	return &Client{
		httpClient:  client,
		TeamDomain:  teamDomain,
		AccessToken: accessToken,
		BaseURL:     BaseURL,
	}
}

// RequestOption allows setting custom options for requests
type RequestOption func(*resty.Request)

func (c *Client) errorFromResponse(resp *resty.Response) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
		return fmt.Errorf("failed to parse error response: %w, status: %s", err, resp.Status())
	}

	// Improve error message
	errMsg := resp.Status()
	if len(errResp.Messages) > 0 {
		errMsg = fmt.Sprintf("%v", errResp.Messages)
	} else if errResp.Error != "" {
		errMsg = errResp.Error
	}
	return fmt.Errorf("API error: %s", errMsg)
}

// Request performs the API request and handles standard error checking and unmarshaling
func (c *Client) Request(method, path string, body interface{}, result interface{}, opts ...RequestOption) error {
	url := c.buildURL(path, nil)
	req := c.httpClient.R()

	if body != nil {
		req.SetBody(body)
	}

	for _, opt := range opts {
		opt(req)
	}

	var resp *resty.Response
	var err error

	switch method {
	case "GET":
		resp, err = req.Get(url)
	case "POST":
		resp, err = req.Post(url)
	case "PUT":
		resp, err = req.Put(url)
	case "DELETE":
		resp, err = req.Delete(url)
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return err
	}

	if resp.IsError() {
		return c.errorFromResponse(resp)
	}

	if result != nil {
		if err := json.Unmarshal(resp.Body(), result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// SetBaseURL sets the base URL of the API
func (c *Client) SetBaseURL(baseURL string) {
	c.BaseURL = baseURL
}

// SetTimeout sets the timeout for API requests
func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.SetTimeout(timeout)
}

// buildURL builds the URL for the API request
func (c *Client) buildURL(path string, params map[string]string) string {
	baseURL := fmt.Sprintf("%s/teams/%s%s", c.BaseURL, c.TeamDomain, path)

	if len(params) == 0 {
		return baseURL
	}

	// Add query parameters
	values := url.Values{}
	for key, value := range params {
		values.Add(key, value)
	}

	return fmt.Sprintf("%s?%s", baseURL, values.Encode())
}

// Get sends a GET request to the API
func (c *Client) Get(path string, params map[string]string) (*resty.Response, error) {
	url := c.buildURL(path, params)
	return c.httpClient.R().Get(url)
}

// Post sends a POST request to the API
func (c *Client) Post(path string, body interface{}) (*resty.Response, error) {
	url := c.buildURL(path, nil)
	return c.httpClient.R().SetBody(body).Post(url)
}

// Put sends a PUT request to the API
func (c *Client) Put(path string, body interface{}) (*resty.Response, error) {
	url := c.buildURL(path, nil)
	return c.httpClient.R().SetBody(body).Put(url)
}

// Delete sends a DELETE request to the API
func (c *Client) Delete(path string) (*resty.Response, error) {
	url := c.buildURL(path, nil)
	return c.httpClient.R().Delete(url)
}
