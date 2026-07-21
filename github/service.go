package github

import (
	"context"
	"strconv"
	"strings"
)

const maxTextBytes = 32 << 10

// Server is the go-micro service handler for GitHub.
type Server struct{ client *Client }

// NewServer creates a GitHub service handler using client.
func NewServer(client *Client) *Server { return &Server{client: client} }

// RepositoriesRequest controls repository listing and search.
type RepositoriesRequest struct {
	Query   string `json:"query" description:"Optional repository search query"`
	Page    int    `json:"page" description:"Page number"`
	PerPage int    `json:"per_page" description:"Results per page"`
}

// RepositoriesResponse contains repository records and model-ready text.
type RepositoriesResponse struct {
	Repositories []Repository `json:"repositories" description:"GitHub repositories"`
	Page         PageInfo     `json:"page" description:"Pagination information"`
	Text         string       `json:"text" description:"Plain-text repository summary"`
}

// RepositoryRequest selects a repository and optional related resource.
type RepositoryRequest struct {
	Owner    string `json:"owner" description:"Repository owner"`
	Repo     string `json:"repo" description:"Repository name"`
	Resource string `json:"resource" description:"metadata (default), issues, or pulls"`
	State    string `json:"state" description:"Optional item state: open, closed, or all"`
	Query    string `json:"query" description:"Optional item search query"`
	Page     int    `json:"page" description:"Page number"`
	PerPage  int    `json:"per_page" description:"Results per page"`
}

// RepositoryResponse contains a repository and optionally its items.
type RepositoryResponse struct {
	Repository   Repository    `json:"repository" description:"GitHub repository metadata"`
	Issues       []Issue       `json:"issues" description:"Repository issues when requested"`
	PullRequests []PullRequest `json:"pull_requests" description:"Repository pull requests when requested"`
	Page         PageInfo      `json:"page" description:"Pagination information for requested items"`
	Text         string        `json:"text" description:"Plain-text repository summary"`
}

// SearchRequest controls issue and pull-request search.
type SearchRequest struct {
	Query    string `json:"query" description:"Required search query"`
	Owner    string `json:"owner" description:"Optional repository owner"`
	Repo     string `json:"repo" description:"Optional repository name"`
	Resource string `json:"resource" description:"issues, pulls, or all"`
	State    string `json:"state" description:"Optional item state: open, closed, or all"`
	Page     int    `json:"page" description:"Page number"`
	PerPage  int    `json:"per_page" description:"Results per page"`
}

// SearchResponse contains matching issues and model-ready text.
type SearchResponse struct {
	Items []Issue  `json:"items" description:"Matching GitHub issues or pull requests"`
	Page  PageInfo `json:"page" description:"Pagination information"`
	Text  string   `json:"text" description:"Plain-text search summary"`
}

// IssueRequest identifies an issue or pull request thread.
type IssueRequest struct {
	Owner  string `json:"owner" description:"Repository owner"`
	Repo   string `json:"repo" description:"Repository name"`
	Number int    `json:"number" description:"Issue or pull-request number"`
}

// IssueResponse contains the complete thread and model-ready text.
type IssueResponse struct {
	Thread Thread `json:"thread" description:"Issue or pull-request thread"`
	Text   string `json:"text" description:"Plain-text thread summary"`
}

// Repositories lists accessible repositories or searches public repositories.
// @example {"query":"mu","page":1,"per_page":30}
func (s *Server) Repositories(ctx context.Context, req *RepositoriesRequest, rsp *RepositoriesResponse) error {
	if req == nil {
		return invalidRequest()
	}
	repositories, page, err := s.client.Repositories(ctx, req.Query, req.Page, req.PerPage)
	if err != nil {
		return err
	}
	rsp.Repositories, rsp.Page = repositories, page
	rsp.Text = repositoriesText(repositories)
	return nil
}

// Repository gets repository metadata, issues, or pull requests.
// @example {"owner":"micro","repo":"mu","resource":"issues"}
func (s *Server) Repository(ctx context.Context, req *RepositoryRequest, rsp *RepositoryResponse) error {
	if req == nil {
		return invalidRequest()
	}
	resource := req.Resource
	if resource == "" {
		resource = "metadata"
	}
	if resource != "metadata" && resource != "issues" && resource != "pulls" {
		return invalidRequest()
	}
	repository, err := s.client.Repository(ctx, req.Owner, req.Repo)
	if err != nil {
		return err
	}
	rsp.Repository = repository
	if resource == "issues" {
		rsp.Issues, rsp.Page, err = s.client.Issues(ctx, itemOptions(req.Owner, req.Repo, req.Query, req.State, "issues", req.Page, req.PerPage))
		if err == nil {
			rsp.Text = repositoryText(repository, rsp.Issues, nil)
		}
		return err
	}
	if resource == "pulls" {
		rsp.PullRequests, rsp.Page, err = s.client.PullRequests(ctx, itemOptions(req.Owner, req.Repo, req.Query, req.State, "pulls", req.Page, req.PerPage))
		if err == nil {
			rsp.Text = repositoryText(repository, nil, rsp.PullRequests)
		}
		return err
	}
	rsp.Text = repositoryText(repository, nil, nil)
	return nil
}

// Search finds issues or pull requests using GitHub's item search.
// @example {"query":"memory leak","owner":"micro","repo":"mu","resource":"issues"}
func (s *Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	if req == nil || strings.TrimSpace(req.Query) == "" || (req.Resource != "" && req.Resource != "issues" && req.Resource != "pulls" && req.Resource != "all") {
		return invalidRequest()
	}
	items, page, err := s.client.Search(ctx, itemOptions(req.Owner, req.Repo, req.Query, req.State, req.Resource, req.Page, req.PerPage))
	if err != nil {
		return err
	}
	rsp.Items, rsp.Page = items, page
	rsp.Text = issuesText(req.Owner, req.Repo, items)
	return nil
}

// Issue gets an issue or pull-request thread with chronological comments.
// @example {"owner":"micro","repo":"mu","number":42}
func (s *Server) Issue(ctx context.Context, req *IssueRequest, rsp *IssueResponse) error {
	if req == nil {
		return invalidRequest()
	}
	thread, err := s.client.Thread(ctx, req.Owner, req.Repo, req.Number)
	if err != nil {
		return err
	}
	rsp.Thread = thread
	rsp.Text = truncateText("Repository: " + req.Owner + "/" + req.Repo + "\n" + threadText(thread))
	return nil
}

func invalidRequest() error { return &APIError{Kind: ErrorInvalid} }

func itemOptions(owner, repo, query, state, itemType string, page, perPage int) ItemOptions {
	return ItemOptions{Owner: owner, Repo: repo, Query: query, State: state, Type: itemType, Page: page, PerPage: perPage}
}

func repositoriesText(repositories []Repository) string {
	var b strings.Builder
	for _, repository := range repositories[:min(len(repositories), 20)] {
		writeRepository(&b, repository)
	}
	return truncateText(b.String())
}

func repositoryText(repository Repository, issues []Issue, pulls []PullRequest) string {
	var b strings.Builder
	writeRepository(&b, repository)
	for _, issue := range issues[:min(len(issues), 20)] {
		writeIssue(&b, issue, "Issue")
	}
	for _, pull := range pulls[:min(len(pulls), 20)] {
		writePullRequest(&b, pull)
	}
	return truncateText(b.String())
}

func issuesText(owner, repo string, issues []Issue) string {
	var b strings.Builder
	if owner != "" && repo != "" {
		b.WriteString("Repository: " + owner + "/" + repo + "\n")
	}
	for _, issue := range issues[:min(len(issues), 20)] {
		kind := "Issue"
		if issue.PullRequest != nil {
			kind = "Pull request"
		}
		writeIssue(&b, issue, kind)
	}
	return truncateText(b.String())
}

func threadText(thread Thread) string {
	var b strings.Builder
	if thread.PullRequest != nil {
		writePullRequest(&b, *thread.PullRequest)
	} else {
		writeIssue(&b, thread.Issue, "Issue")
	}
	for _, comment := range thread.Comments[:min(len(thread.Comments), 100)] {
		b.WriteString("Comment")
		if comment.User.Login != "" {
			b.WriteString(" by " + comment.User.Login)
		}
		b.WriteString(": " + strings.TrimSpace(comment.Body) + "\n")
		if comment.HTMLURL != "" {
			b.WriteString(comment.HTMLURL + "\n")
		}
	}
	return truncateText(b.String())
}

func writeRepository(b *strings.Builder, repository Repository) {
	b.WriteString("Repository: " + repository.FullName + "\n")
	if repository.Description != "" {
		b.WriteString(strings.TrimSpace(repository.Description) + "\n")
	}
	if repository.HTMLURL != "" {
		b.WriteString(repository.HTMLURL + "\n")
	}
}

func writeIssue(b *strings.Builder, issue Issue, kind string) {
	b.WriteString(kind + " #" + strconv.Itoa(issue.Number) + ": " + issue.Title + " [" + issue.State + "]\n")
	writeLabels(b, issue.Labels)
	if body := strings.TrimSpace(issue.Body); body != "" {
		b.WriteString(body + "\n")
	}
	if issue.HTMLURL != "" {
		b.WriteString(issue.HTMLURL + "\n")
	}
}

func writePullRequest(b *strings.Builder, pull PullRequest) {
	b.WriteString("Pull request #" + strconv.Itoa(pull.Number) + ": " + pull.Title + " [" + pull.State + "]\n")
	writeLabels(b, pull.Labels)
	if body := strings.TrimSpace(pull.Body); body != "" {
		b.WriteString(body + "\n")
	}
	if pull.HTMLURL != "" {
		b.WriteString(pull.HTMLURL + "\n")
	}
}

func writeLabels(b *strings.Builder, labels []Label) {
	if len(labels) == 0 {
		return
	}
	b.WriteString("Labels: ")
	for i, label := range labels {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(label.Name)
	}
	b.WriteByte('\n')
}

func truncateText(text string) string {
	if len(text) <= maxTextBytes {
		return text
	}
	end := maxTextBytes
	for end > 0 && text[end]&0xc0 == 0x80 {
		end--
	}
	return text[:end]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
