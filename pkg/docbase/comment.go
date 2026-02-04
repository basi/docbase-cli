package docbase

import (
	"fmt"
	"strconv"

	"github.com/go-resty/resty/v2"
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

	var commentList CommentListResponse
	if err := s.client.Request("GET", path, nil, &commentList, func(r *resty.Request) {
		r.SetQueryParams(params)
	}); err != nil {
		return nil, err
	}

	return &commentList, nil
}

// Create creates a new comment for a memo
func (s *CommentService) Create(memoID int, req *CreateCommentRequest) (*Comment, error) {
	path := fmt.Sprintf("/posts/%d/comments", memoID)
	var comment Comment
	if err := s.client.Request("POST", path, req, &comment); err != nil {
		return nil, err
	}

	return &comment, nil
}

// Delete deletes a comment
func (s *CommentService) Delete(commentID int) error {
	path := fmt.Sprintf("/comments/%d", commentID)
	return s.client.Request("DELETE", path, nil, nil)
}