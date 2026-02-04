package docbase

import (
	"fmt"
	"strconv"

	"github.com/go-resty/resty/v2"
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

	var memoList MemoListResponse
	if err := s.client.Request("GET", "/posts", nil, &memoList, func(r *resty.Request) {
		r.SetQueryParams(params)
	}); err != nil {
		return nil, err
	}

	return &memoList, nil
}

// Get returns a memo by ID
func (s *MemoService) Get(id int) (*Memo, error) {
	path := fmt.Sprintf("/posts/%d", id)

	// Try parsing as wrapped response first
	var memoResp MemoResponse
	if err := s.client.Request("GET", path, nil, &memoResp); err != nil {
		return nil, err
	}

	// If wrapped response worked, return it
	if memoResp.Memo.ID != 0 {
		return &memoResp.Memo, nil
	}

	// Fallback: try parsing as raw Memo (some API versions may return unwrapped)
	var memo Memo
	if err := s.client.Request("GET", path, nil, &memo); err != nil {
		return nil, err
	}

	if memo.ID == 0 {
		return nil, fmt.Errorf("memo not found: %d", id)
	}

	return &memo, nil
}

// Create creates a new memo
func (s *MemoService) Create(req *CreateMemoRequest) (*Memo, error) {
	var memoResp MemoResponse
	if err := s.client.Request("POST", "/posts", req, &memoResp); err != nil {
		return nil, err
	}

	return &memoResp.Memo, nil
}

// Update updates a memo
func (s *MemoService) Update(id int, req *UpdateMemoRequest) (*Memo, error) {
	var memoResp MemoResponse
	path := fmt.Sprintf("/posts/%d", id)
	if err := s.client.Request("PUT", path, req, &memoResp); err != nil {
		return nil, err
	}

	return &memoResp.Memo, nil
}

// Delete deletes a memo
func (s *MemoService) Delete(id int) error {
	path := fmt.Sprintf("/posts/%d", id)
	return s.client.Request("DELETE", path, nil, nil)
}

// Archive archives a memo
func (s *MemoService) Archive(id int) error {
	path := fmt.Sprintf("/posts/%d/archive", id)
	return s.client.Request("PUT", path, nil, nil)
}

// Unarchive unarchives a memo
func (s *MemoService) Unarchive(id int) error {
	path := fmt.Sprintf("/posts/%d/unarchive", id)
	return s.client.Request("PUT", path, nil, nil)
}