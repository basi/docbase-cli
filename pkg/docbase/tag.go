package docbase

import (
	"encoding/json"
	"fmt"
	"strconv"
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

	resp, err := s.client.Get("/tags", params)
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

	// The tags API returns a plain array, not a wrapped object
	var tags []Tag
	if err := json.Unmarshal(resp.Body(), &tags); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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

	resp, err := s.client.Get("/tags", params)
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

	// The tags API returns a plain array, not a wrapped object
	var tags []Tag
	if err := json.Unmarshal(resp.Body(), &tags); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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