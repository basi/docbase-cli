package docbase

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// CommentService handles communication with the comment related methods of the DocBase API
type CommentService struct {
	client *Client
}

// NewCommentService creates a new comment service
func NewCommentService(client *Client) *CommentService {
	return &CommentService{
		client: client,
	}
}

// List returns a list of comments for a memo
func (s *CommentService) List(memoID, page, perPage int) (*CommentListResponse, error) {
	path := fmt.Sprintf("/posts/%d/comments", memoID)
	params := map[string]string{
		"page":     strconv.Itoa(page),
		"per_page": strconv.Itoa(perPage),
	}

	resp, err := s.client.Get(path, params)
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

	var commentList CommentListResponse
	if err := json.Unmarshal(resp.Body(), &commentList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &commentList, nil
}

// Create creates a new comment for a memo
func (s *CommentService) Create(memoID int, req *CreateCommentRequest) (*Comment, error) {
	path := fmt.Sprintf("/posts/%d/comments", memoID)
	resp, err := s.client.Post(path, req)
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

	var commentResp CommentResponse
	if err := json.Unmarshal(resp.Body(), &commentResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &commentResp.Comment, nil
}

// Delete deletes a comment
func (s *CommentService) Delete(memoID, commentID int) error {
	path := fmt.Sprintf("/posts/%d/comments/%d", memoID, commentID)
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