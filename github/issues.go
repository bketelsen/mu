package github

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type ItemOptions struct {
	Owner   string
	Repo    string
	Query   string
	State   string
	Type    string
	Page    int
	PerPage int
}

func (c *Client) Issues(ctx context.Context, opts ItemOptions) ([]Issue, PageInfo, error) {
	if err := validateItemOptions(opts); err != nil {
		return nil, PageInfo{}, err
	}
	state := opts.State
	if state == "" {
		state = "open"
	}
	qualifiers := append(itemQualifiers(opts), "is:issue")
	if state != "all" {
		qualifiers = append(qualifiers, "is:"+state)
	}
	return c.searchIssues(ctx, opts, qualifiers)
}

func (c *Client) PullRequests(ctx context.Context, opts ItemOptions) ([]PullRequest, PageInfo, error) {
	if err := validateItemOptions(opts); err != nil {
		return nil, PageInfo{}, err
	}
	state := opts.State
	if state == "" {
		state = "all"
	}
	if opts.Query == "" && opts.Owner == "" {
		return nil, PageInfo{}, &APIError{Kind: ErrorInvalid}
	}
	if opts.Query == "" && opts.Owner != "" {
		page, perPage := normalizePage(opts.Page, opts.PerPage)
		parameters := url.Values{
			"state":     {state},
			"sort":      {"updated"},
			"direction": {"desc"},
			"page":      {strconv.Itoa(page)},
			"per_page":  {strconv.Itoa(perPage)},
		}
		var pulls []PullRequest
		pageInfo, err := c.get(ctx, "/repos/"+url.PathEscape(opts.Owner)+"/"+url.PathEscape(opts.Repo)+"/pulls", parameters, &pulls)
		return pulls, pageInfo, err
	}

	qualifiers := append(itemQualifiers(opts), "is:pr")
	if state != "all" {
		qualifiers = append(qualifiers, "is:"+state)
	}
	issues, page, err := c.searchIssues(ctx, opts, qualifiers)
	pulls := make([]PullRequest, len(issues))
	for i, issue := range issues {
		pulls[i] = PullRequest{ID: issue.ID, Number: issue.Number, Title: issue.Title, Body: issue.Body, State: issue.State, HTMLURL: issue.HTMLURL, User: issue.User, Labels: issue.Labels, Comments: issue.Comments, CreatedAt: issue.CreatedAt, UpdatedAt: issue.UpdatedAt, ClosedAt: issue.ClosedAt}
	}
	return pulls, page, err
}

func (c *Client) Search(ctx context.Context, opts ItemOptions) ([]Issue, PageInfo, error) {
	if err := validateItemOptions(opts); err != nil {
		return nil, PageInfo{}, err
	}
	if opts.Query == "" && opts.Owner == "" {
		return nil, PageInfo{}, &APIError{Kind: ErrorInvalid}
	}
	qualifiers := itemQualifiers(opts)
	switch opts.Type {
	case "issues":
		qualifiers = append(qualifiers, "is:issue")
	case "pulls":
		qualifiers = append(qualifiers, "is:pr")
	}
	if opts.State == "open" || opts.State == "closed" {
		qualifiers = append(qualifiers, "is:"+opts.State)
	}
	return c.searchIssues(ctx, opts, qualifiers)
}

func (c *Client) Thread(ctx context.Context, owner, repo string, number int) (Thread, error) {
	if number < 1 {
		return Thread{}, &APIError{Kind: ErrorInvalid}
	}
	if err := validateRepository(owner, repo); err != nil {
		return Thread{}, err
	}
	endpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues/" + strconv.Itoa(number)
	var thread Thread
	if _, err := c.get(ctx, endpoint, nil, &thread.Issue); err != nil {
		return Thread{}, err
	}
	if _, err := c.get(ctx, endpoint+"/comments", url.Values{"per_page": {"100"}}, &thread.Comments); err != nil {
		return Thread{}, err
	}
	sort.SliceStable(thread.Comments, func(i, j int) bool {
		return thread.Comments[i].CreatedAt.Before(thread.Comments[j].CreatedAt)
	})
	if thread.Issue.PullRequest != nil {
		pull := &PullRequest{}
		if _, err := c.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls/"+strconv.Itoa(number), nil, pull); err != nil {
			return Thread{}, err
		}
		thread.PullRequest = pull
	}
	return thread, nil
}

func validateItemOptions(opts ItemOptions) error {
	if len(opts.Query) > maxQueryLength || (opts.Owner != "" || opts.Repo != "") && validateRepository(opts.Owner, opts.Repo) != nil {
		return &APIError{Kind: ErrorInvalid}
	}
	if opts.State != "" && opts.State != "open" && opts.State != "closed" && opts.State != "all" {
		return &APIError{Kind: ErrorInvalid}
	}
	if opts.Type != "" && opts.Type != "issues" && opts.Type != "pulls" && opts.Type != "all" {
		return &APIError{Kind: ErrorInvalid}
	}
	return nil
}

func (c *Client) searchIssues(ctx context.Context, opts ItemOptions, qualifiers []string) ([]Issue, PageInfo, error) {
	page, perPage := normalizePage(opts.Page, opts.PerPage)
	query := strings.TrimSpace(strings.Join(append([]string{opts.Query}, qualifiers...), " "))
	parameters := url.Values{
		"q":        {query},
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(perPage)},
	}
	var result struct {
		Items []Issue `json:"items"`
	}
	pageInfo, err := c.get(ctx, "/search/issues", parameters, &result)
	return result.Items, pageInfo, err
}

func itemQualifiers(opts ItemOptions) []string {
	qualifiers := make([]string, 0, 1)
	if opts.Owner != "" {
		qualifiers = append(qualifiers, "repo:"+opts.Owner+"/"+opts.Repo)
	}
	return qualifiers
}
