package docbase

import (
	"encoding/json"
	"fmt"
	"strconv"
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

	resp, err := s.client.Get("/groups", params)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, s.client.errorFromResponse(resp)
	}

	var groups []Group
	if err := json.Unmarshal(resp.Body(), &groups); err != nil {
		return nil, fmt.Errorf("failed to parse groups response: %w", err)
	}

	groupList := &GroupListResponse{
		Groups: groups,
		Meta: Meta{
			Total: len(groups),
		},
	}
	if perPage > 0 && len(groups) == perPage {
		nextPage := strconv.Itoa(page + 1)
		groupList.Meta.NextPage = &nextPage
	}

	return groupList, nil
}

// Get returns a group by ID
func (s *GroupService) Get(id int) (*Group, error) {
	path := fmt.Sprintf("/groups/%d", id)
	var group Group
	if err := s.client.Request("GET", path, nil, &group); err != nil {
		return nil, err
	}
	if group.ID == 0 {
		return nil, fmt.Errorf("failed to parse group response: %d", id)
	}

	return &group, nil
}

// GetMembers returns the members of a group
func (s *GroupService) GetMembers(id int) ([]User, error) {
	group, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	return group.Users, nil
}
