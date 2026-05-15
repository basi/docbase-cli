package docbase

import (
	"encoding/json"
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

	resp, err := s.client.Get(path, nil)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, s.client.errorFromResponse(resp)
	}

	body := resp.Body()

	// Try parsing as wrapped response first
	var memoResp MemoResponse
	if err := json.Unmarshal(body, &memoResp); err == nil && memoResp.Memo.ID != 0 {
		return &memoResp.Memo, nil
	}

	// Fallback: try parsing as raw Memo (some API versions may return unwrapped)
	var memo Memo
	if err := json.Unmarshal(body, &memo); err != nil || memo.ID == 0 {
		return nil, fmt.Errorf("failed to parse memo response: %d", id)
	}

	return &memo, nil
}

// Create creates a new memo
func (s *MemoService) Create(req *CreateMemoRequest) (*Memo, error) {
	resp, err := s.client.Post("/posts", req)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, s.client.errorFromResponse(resp)
	}

	body := resp.Body()

	var memoResp MemoResponse
	if err := json.Unmarshal(body, &memoResp); err == nil && memoResp.Memo.ID != 0 {
		return &memoResp.Memo, nil
	}

	var memo Memo
	if err := json.Unmarshal(body, &memo); err == nil && memo.ID != 0 {
		return &memo, nil
	}

	return nil, fmt.Errorf("failed to parse create memo response")
}

// Update updates a memo
func (s *MemoService) Update(id int, req *UpdateMemoRequest) (*Memo, error) {
	path := fmt.Sprintf("/posts/%d", id)
	resp, err := s.client.Put(path, req)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, s.client.errorFromResponse(resp)
	}

	body := resp.Body()

	var memoResp MemoResponse
	if err := json.Unmarshal(body, &memoResp); err == nil && memoResp.Memo.ID != 0 {
		return &memoResp.Memo, nil
	}

	var memo Memo
	if err := json.Unmarshal(body, &memo); err == nil && memo.ID != 0 {
		return &memo, nil
	}

	return nil, fmt.Errorf("failed to parse update memo response: %d", id)
}

// PatchBody applies line-range replacements to a memo body
func (s *MemoService) PatchBody(id int, req *PatchBodyRequest) (*Memo, error) {
	path := fmt.Sprintf("/posts/%d/body", id)
	resp, err := s.client.Patch(path, req)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, s.client.errorFromResponse(resp)
	}

	body := resp.Body()

	var memoResp MemoResponse
	if err := json.Unmarshal(body, &memoResp); err == nil && memoResp.Memo.ID != 0 {
		return &memoResp.Memo, nil
	}

	var memo Memo
	if err := json.Unmarshal(body, &memo); err == nil && memo.ID != 0 {
		return &memo, nil
	}

	// When include_body is false the API may return minimal JSON or empty body.
	return &Memo{ID: id}, nil
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
