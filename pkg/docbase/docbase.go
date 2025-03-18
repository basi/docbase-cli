package docbase

import (
	"time"

	"github.com/go-resty/resty/v2"
)

// API represents the DocBase API client
type API struct {
	client   *Client
	Memo     *MemoService
	Group    *GroupService
	Tag      *TagService
	Comment  *CommentService
}

// NewAPI creates a new DocBase API client
func NewAPI(teamDomain, accessToken string) *API {
	client := NewClient(teamDomain, accessToken)
	
	return &API{
		client:   client,
		Memo:     NewMemoService(client),
		Group:    NewGroupService(client),
		Tag:      NewTagService(client),
		Comment:  NewCommentService(client),
	}
}

// SetBaseURL sets the base URL of the API
func (a *API) SetBaseURL(baseURL string) {
	a.client.SetBaseURL(baseURL)
}

// SetTimeout sets the timeout for API requests
func (a *API) SetTimeout(timeout int) {
	a.client.SetTimeout(time.Duration(timeout) * time.Second)
}

// Get sends a GET request to the API
func (a *API) Get(path string, params map[string]string) (*resty.Response, error) {
	return a.client.Get(path, params)
}

// Post sends a POST request to the API
func (a *API) Post(path string, body interface{}) (*resty.Response, error) {
	return a.client.Post(path, body)
}

// Put sends a PUT request to the API
func (a *API) Put(path string, body interface{}) (*resty.Response, error) {
	return a.client.Put(path, body)
}

// Delete sends a DELETE request to the API
func (a *API) Delete(path string) (*resty.Response, error) {
	return a.client.Delete(path)
}