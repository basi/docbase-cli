package docbase

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-resty/resty/v2"
)

// GroupService handles communication with the group related methods of the DocBase API
type GroupService struct {
	client *Client
}

// NewGroupService creates a new group service
func NewGroupService(client *Client) *GroupService {
	return &GroupService{
		client: client,
	}
}

// List returns a list of groups
func (s *GroupService) List(page, perPage int) (*GroupListResponse, error) {
	params := map[string]string{
		"page":     strconv.Itoa(page),
		"per_page": strconv.Itoa(perPage),
	}

	// Try parsing as object first
	var groupList GroupListResponse
	if err := s.client.Request("GET", "/groups", nil, &groupList, func(r *resty.Request) {
		r.SetQueryParams(params)
	}); err == nil {
		return &groupList, nil
	}

	// If parsing as object fails, try parsing as array
	// Note: The new Request method returns error if unmarshal fails.
	// We might need to handle this manually or adjust Request method to be more flexible,
	// but for now let's stick to the previous pattern of checking both.
	// However, since Request consumes the body, we can't easily re-read it unless we change Request.
	// Actually, the previous implementation read body multiple times (resty buffers it).
	// So we can try Request with one type, if it fails, try another?
	// But Request does a lot of things.
	
	// Let's implement the specific logic here using client.Get for now to support this hybrid response,
	// or refactor to use a unified type.
	// The original code handled both object and array response.
	// Let's use the lower level client.Get here because of the complex unmarshaling logic
	// that depends on the response structure.
	
	resp, err := s.client.Get("/groups", params)
	if err != nil {
		return nil, err
	}
	
	if resp.IsError() {
		return nil, fmt.Errorf("API error: %s", resp.Status())
	}
	
	// Try parsing as object first
	if err := json.Unmarshal(resp.Body(), &groupList); err == nil {
		return &groupList, nil
	}
	
	// If parsing as object fails, try parsing as array
	var groups []Group
	if err := json.Unmarshal(resp.Body(), &groups); err != nil {
		return nil, fmt.Errorf("failed to parse response as object or array: %w", err)
	}
	
	// Convert array response to GroupListResponse format
	return &GroupListResponse{
		Groups: groups,
	}, nil
}

// Get returns a group by ID
func (s *GroupService) Get(id int) (*Group, error) {
	path := fmt.Sprintf("/groups/%d", id)
	var groupResp GroupResponse
	if err := s.client.Request("GET", path, nil, &groupResp); err != nil {
		return nil, err
	}

	return &groupResp.Group, nil
}

// GetMembers returns the members of a group
func (s *GroupService) GetMembers(id int) ([]User, error) {
	// DocBase API returns users as part of the group detail response
	path := fmt.Sprintf("/groups/%d", id)
	var group Group
	if err := s.client.Request("GET", path, nil, &group); err != nil {
		return nil, err
	}

	return group.Users, nil
}