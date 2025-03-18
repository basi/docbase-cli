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
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return nil, fmt.Errorf("failed to parse error response: %w", err)
		}
		return nil, fmt.Errorf("API error: %s", errResp.Messages)
	}

	// APIレスポンスが配列形式の場合の処理
	var groups []Group
	if err := json.Unmarshal(resp.Body(), &groups); err != nil {
		// 従来の形式（オブジェクト形式）での解析を試みる
		var groupList GroupListResponse
		if err2 := json.Unmarshal(resp.Body(), &groupList); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &groupList, nil
	}

	// 配列形式のレスポンスを GroupListResponse 形式に変換
	return &GroupListResponse{
		Groups: groups,
	}, nil
}

// Get returns a group by ID
func (s *GroupService) Get(id int) (*Group, error) {
	path := fmt.Sprintf("/groups/%d", id)
	resp, err := s.client.Get(path, nil)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return nil, fmt.Errorf("failed to parse error response: %w", err)
		}
		return nil, fmt.Errorf("API error: %s", errResp.Messages)
	}

	var groupResp GroupResponse
	if err := json.Unmarshal(resp.Body(), &groupResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &groupResp.Group, nil
}

// GetMembers returns the members of a group
func (s *GroupService) GetMembers(id int) ([]User, error) {
	path := fmt.Sprintf("/groups/%d/users", id)
	resp, err := s.client.Get(path, nil)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return nil, fmt.Errorf("failed to parse error response: %w", err)
		}
		return nil, fmt.Errorf("API error: %s", errResp.Messages)
	}

	var users struct {
		Users []User `json:"users"`
	}
	if err := json.Unmarshal(resp.Body(), &users); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return users.Users, nil
}