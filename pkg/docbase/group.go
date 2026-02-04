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

	// まずオブジェクト形式として解析を試みる
	var groupList GroupListResponse
	if err := json.Unmarshal(resp.Body(), &groupList); err == nil {
		return &groupList, nil
	}

	// オブジェクト形式での解析に失敗した場合、配列形式として解析を試みる
	var groups []Group
	if err := json.Unmarshal(resp.Body(), &groups); err != nil {
		// デバッグ情報を含めたエラーメッセージを返す
		return nil, fmt.Errorf("failed to parse response as object or array: %w, response body: %s", err, string(resp.Body()))
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
	// DocBase API returns users as part of the group detail response
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

	var group Group
	if err := json.Unmarshal(resp.Body(), &group); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return group.Users, nil
}