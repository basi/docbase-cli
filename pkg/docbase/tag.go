package docbase

import (
	"strconv"

	"github.com/go-resty/resty/v2"
)

// TagService handles communication with the tag related methods of the DocBase API
type TagService struct {
	client *Client
}

// NewTagService creates a new tag service
func NewTagService(client *Client) *TagService {
	return &TagService{
		client: client,
	}
}

// List returns a list of tags
func (s *TagService) List(page, perPage int) (*TagListResponse, error) {
	params := map[string]string{
		"page":     strconv.Itoa(page),
		"per_page": strconv.Itoa(perPage),
	}

	// The tags API returns a plain array, not a wrapped object
	var tags []Tag
	if err := s.client.Request("GET", "/tags", nil, &tags, func(r *resty.Request) {
		r.SetQueryParams(params)
	}); err != nil {
		return nil, err
	}

	// Build the response manually since there's no meta in the API response
	tagList := &TagListResponse{
		Tags: tags,
		Meta: Meta{
			Total: len(tags),
		},
	}

	return tagList, nil
}

// Search searches for tags by name
func (s *TagService) Search(query string, page, perPage int) (*TagListResponse, error) {
	params := map[string]string{
		"q":        query,
		"page":     strconv.Itoa(page),
		"per_page": strconv.Itoa(perPage),
	}

	// The tags API returns a plain array, not a wrapped object
	var tags []Tag
	if err := s.client.Request("GET", "/tags", nil, &tags, func(r *resty.Request) {
		r.SetQueryParams(params)
	}); err != nil {
		return nil, err
	}

	// Build the response manually since there's no meta in the API response
	tagList := &TagListResponse{
		Tags: tags,
		Meta: Meta{
			Total: len(tags),
		},
	}

	return tagList, nil
}