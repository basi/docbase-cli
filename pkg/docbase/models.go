package docbase

import "time"

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Error    string   `json:"error"`
	Messages []string `json:"messages"`
}

// Meta represents metadata in API responses
type Meta struct {
	PreviousPage *string `json:"previous_page"`
	NextPage     *string `json:"next_page"`
	Total        int     `json:"total"`
}

// User represents a DocBase user
type User struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Username        string    `json:"username"`
	ProfileImageURL string    `json:"profile_image_url"`
	CreatedAt       time.Time `json:"created_at"`
	Admin           bool      `json:"admin"`
}

// Group represents a DocBase group
type Group struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	Description    string     `json:"description"`
	PostsCount     int        `json:"posts_count"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	Users          []User     `json:"users,omitempty"`
}

// Tag represents a DocBase tag
type Tag struct {
	Name string `json:"name"`
}

// Comment represents a comment on a memo
type Comment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      User      `json:"user"`
}

// Attachment represents a file attachment
type Attachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int       `json:"size"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Memo represents a DocBase memo
type Memo struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	Draft       bool         `json:"draft"`
	Archived    bool         `json:"archived"`
	URL         string       `json:"url"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Scope       string       `json:"scope"`
	SharingURL  string       `json:"sharing_url"`
	Tags        []Tag        `json:"tags"`
	User        User         `json:"user"`
	Groups      []Group      `json:"groups"`
	Comments    []Comment    `json:"comments"`
	Attachments []Attachment `json:"attachments"`
	LikedUsers  []User       `json:"liked_users"`
	Stars       int          `json:"stars"`
}

// MemoListResponse represents the response for listing memos
type MemoListResponse struct {
	Meta  Meta   `json:"meta"`
	Memos []Memo `json:"posts"`
}

// MemoResponse represents the response for a single memo
type MemoResponse struct {
	Memo Memo `json:"post"`
}

// GroupListResponse represents the response for listing groups
type GroupListResponse struct {
	Meta   Meta    `json:"meta"`
	Groups []Group `json:"groups"`
}

// GroupResponse represents the response for a single group
type GroupResponse struct {
	Group Group `json:"group"`
}

// TagListResponse represents the response for listing tags
type TagListResponse struct {
	Meta Meta  `json:"meta"`
	Tags []Tag `json:"tags"`
}

// CommentListResponse represents the response for listing comments
type CommentListResponse struct {
	Meta     Meta      `json:"meta"`
	Comments []Comment `json:"comments"`
}

// CommentResponse represents the response for a single comment
type CommentResponse struct {
	Comment Comment `json:"comment"`
}

// CreateMemoRequest represents the request for creating a memo
type CreateMemoRequest struct {
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Draft       bool     `json:"draft,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Groups      []int    `json:"groups,omitempty"`
	Notify      bool     `json:"notice,omitempty"`
	ExcludeBody bool     `json:"exclude_body,omitempty"`
}

// UpdateMemoRequest represents the request for updating a memo
type UpdateMemoRequest struct {
	Title       string   `json:"title,omitempty"`
	Body        string   `json:"body,omitempty"`
	Draft       *bool    `json:"draft,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Groups      []int    `json:"groups,omitempty"`
	Notify      *bool    `json:"notice,omitempty"`
	ExcludeBody bool     `json:"exclude_body,omitempty"`
}

// CreateCommentRequest represents the request for creating a comment
type CreateCommentRequest struct {
	Body   string `json:"body"`
	Notify bool   `json:"notice,omitempty"`
}

// PatchBodyOperation is one line-range replacement in a memo body
type PatchBodyOperation struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	OldContent string `json:"old_content"`
	Content    string `json:"content"`
}

// PatchBodyRequest represents the request for PATCH /posts/:id/body
type PatchBodyRequest struct {
	Operations  []PatchBodyOperation `json:"operations"`
	Notice      *bool                `json:"notice,omitempty"`
	IncludeBody bool                 `json:"include_body,omitempty"`
}
