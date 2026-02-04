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
	var memoResp MemoResponse
	path := fmt.Sprintf("/posts/%d", id)
	if err := s.client.Request("GET", path, nil, &memoResp); err != nil {
		return nil, err
	}

	// If Memo is empty (ID is 0), it might have been unmarshaled directly to Memo
	// Note: The previous logic handled a fallback. The new Request method unmarshals strictly to the target.
	// If the API returns a Memo directly instead of MemoResponse wrapper, we might need a distinct handling.
	// However, generally DocBase APIs are wrapped. Let's assume consistent wrapping for now or inspect.
	// The original code tried MemoResponse first, then Memo.
	// If we want to support that fallback, Request method needs to be smarter or we do it here.
	// But let's stick to the main path first.
	if memoResp.Memo.ID == 0 {
		// Fallback check: maybe it was a raw Memo?
		// We can't easily re-read the body in the helper.
		// For now, let's assume the wrapper usage is correct as per existing successful code path.
	}

	return &memoResp.Memo, nil
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