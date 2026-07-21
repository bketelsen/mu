package github

import (
	"fmt"
	"time"
)

const (
	defaultPerPage = 30
	maxPerPage     = 100
	maxQueryLength = 256
	maxBodyBytes   = 4 << 20
)

type ErrorKind string

const (
	ErrorNotConfigured ErrorKind = "not_configured"
	ErrorUnauthorized  ErrorKind = "unauthorized"
	ErrorForbidden     ErrorKind = "forbidden"
	ErrorNotFound      ErrorKind = "not_found"
	ErrorRateLimited   ErrorKind = "rate_limited"
	ErrorUpstream      ErrorKind = "upstream"
	ErrorInvalid       ErrorKind = "invalid"
)

type APIError struct {
	Kind       ErrorKind
	Status     int
	Reset      time.Time
	retryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.Kind {
	case ErrorNotConfigured:
		return "GITHUB_TOKEN is not configured"
	case ErrorUnauthorized:
		return "GitHub token is invalid or expired"
	case ErrorForbidden:
		return "GitHub token does not have access to this resource"
	case ErrorNotFound:
		return "GitHub repository or item was not found or is not accessible"
	case ErrorRateLimited:
		if !e.Reset.IsZero() {
			return fmt.Sprintf("GitHub rate limit reached; resets at %s", e.Reset.UTC().Format(time.RFC3339))
		}
		return "GitHub rate limit reached"
	case ErrorInvalid:
		return "invalid GitHub request"
	default:
		return "GitHub request failed"
	}
}

type PageInfo struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Next    int `json:"next,omitempty"`
	Prev    int `json:"prev,omitempty"`
	First   int `json:"first,omitempty"`
	Last    int `json:"last,omitempty"`
}

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Repository struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	HTMLURL         string    `json:"html_url"`
	Private         bool      `json:"private"`
	Fork            bool      `json:"fork"`
	DefaultBranch   string    `json:"default_branch"`
	Language        string    `json:"language"`
	StargazersCount int       `json:"stargazers_count"`
	OpenIssuesCount int       `json:"open_issues_count"`
	UpdatedAt       time.Time `json:"updated_at"`
	Owner           User      `json:"owner"`
}

type PullMarker struct {
	URL string `json:"url"`
}

type Issue struct {
	ID            int64       `json:"id"`
	Number        int         `json:"number"`
	Title         string      `json:"title"`
	Body          string      `json:"body"`
	State         string      `json:"state"`
	StateReason   string      `json:"state_reason"`
	HTMLURL       string      `json:"html_url"`
	User          User        `json:"user"`
	Labels        []Label     `json:"labels"`
	Comments      int         `json:"comments"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	ClosedAt      *time.Time  `json:"closed_at"`
	PullRequest   *PullMarker `json:"pull_request,omitempty"`
	RepositoryURL string      `json:"repository_url"`
}

type PullRequest struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	Mergeable *bool      `json:"mergeable"`
	User      User       `json:"user"`
	Labels    []Label    `json:"labels"`
	Comments  int        `json:"comments"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	MergedAt  *time.Time `json:"merged_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Thread struct {
	Issue       Issue        `json:"issue"`
	PullRequest *PullRequest `json:"pull_request,omitempty"`
	Comments    []Comment    `json:"comments"`
}
