package docbase

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// MemoService handles communication with the memo related methods of the DocBase API
type MemoService struct {
	client *Client
}

// NewMemoService creates a new memo service
func NewMemoService(client *Client) *MemoService {
	return &MemoService{
		client: client,
	}
}

// List returns a list of memos
func (s *MemoService) List(page, perPage int, query string) (*MemoListResponse, error) {
	params := map[string]string{
		"page":     strconv.Itoa(page),
		"per_page": strconv.Itoa(perPage),
	}

	if query != "" {
		params["q"] = query
	}

	resp, err := s.client.Get("/posts", params)
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

	var memoList MemoListResponse
	if err := json.Unmarshal(resp.Body(), &memoList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &memoList, nil
}

// Get returns a memo by ID
func (s *MemoService) Get(id int) (*Memo, error) {
	path := fmt.Sprintf("/posts/%d", id)
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

	var memoResp MemoResponse
	if err := json.Unmarshal(resp.Body(), &memoResp); err != nil {
		// Try to unmarshal directly to Memo if MemoResponse fails
		var memo Memo
		if err := json.Unmarshal(resp.Body(), &memo); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &memo, nil
	}

	// If Memo is empty (ID is 0), try direct unmarshaling
	if memoResp.Memo.ID == 0 {
		var memo Memo
		if err := json.Unmarshal(resp.Body(), &memo); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &memo, nil
	}

	return &memoResp.Memo, nil
}

// Create creates a new memo
func (s *MemoService) Create(req *CreateMemoRequest) (*Memo, error) {
	resp, err := s.client.Post("/posts", req)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return nil, fmt.Errorf("failed to parse error response: %w", err)
		}
		// Improve error message to include the error type
		if errResp.Error != "" {
			return nil, fmt.Errorf("API error: [%s]", errResp.Error)
		}
		return nil, fmt.Errorf("API error: %s", errResp.Messages)
	}

	var memoResp MemoResponse
	if err := json.Unmarshal(resp.Body(), &memoResp); err != nil {
		// Try to unmarshal directly to Memo if MemoResponse fails
		var memo Memo
		if err := json.Unmarshal(resp.Body(), &memo); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &memo, nil
	}

	// If Memo is empty (ID is 0), try direct unmarshaling
	if memoResp.Memo.ID == 0 {
		var memo Memo
		if err := json.Unmarshal(resp.Body(), &memo); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &memo, nil
	}

	return &memoResp.Memo, nil
}

// Update updates a memo
func (s *MemoService) Update(id int, req *UpdateMemoRequest) (*Memo, error) {
	path := fmt.Sprintf("/posts/%d", id)
	resp, err := s.client.Put(path, req)
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

	var memoResp MemoResponse
	if err := json.Unmarshal(resp.Body(), &memoResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &memoResp.Memo, nil
}

// Delete deletes a memo
func (s *MemoService) Delete(id int) error {
	path := fmt.Sprintf("/posts/%d", id)
	resp, err := s.client.Delete(path)
	if err != nil {
		return err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return fmt.Errorf("failed to parse error response: %w", err)
		}
		return fmt.Errorf("API error: %s", errResp.Messages)
	}

	return nil
}

// Archive archives a memo
func (s *MemoService) Archive(id int) error {
	path := fmt.Sprintf("/posts/%d/archive", id)
	resp, err := s.client.Put(path, nil)
	if err != nil {
		return err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return fmt.Errorf("failed to parse error response: %w", err)
		}
		return fmt.Errorf("API error: %s", errResp.Messages)
	}

	return nil
}

// Unarchive unarchives a memo
func (s *MemoService) Unarchive(id int) error {
	path := fmt.Sprintf("/posts/%d/unarchive", id)
	resp, err := s.client.Put(path, nil)
	if err != nil {
		return err
	}

	if resp.IsError() {
		var errResp ErrorResponse
		if err := json.Unmarshal(resp.Body(), &errResp); err != nil {
			return fmt.Errorf("failed to parse error response: %w", err)
		}
		return fmt.Errorf("API error: %s", errResp.Messages)
	}

	return nil
}