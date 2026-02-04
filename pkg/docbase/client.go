package docbase

import (
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