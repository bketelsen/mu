package github

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
)

var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func (c *Client) Repositories(ctx context.Context, query string, page, perPage int) ([]Repository, PageInfo, error) {
	if len(query) > maxQueryLength {
		return nil, PageInfo{}, &APIError{Kind: ErrorInvalid}
	}

	page, perPage = normalizePage(page, perPage)
	parameters := url.Values{
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(perPage)},
	}
	if query == "" {
		parameters.Set("affiliation", "owner,collaborator,organization_member")
		parameters.Set("sort", "updated")
		parameters.Set("direction", "desc")
		var repositories []Repository
		pageInfo, err := c.get(ctx, "/user/repos", parameters, &repositories)
		return repositories, pageInfo, err
	}

	parameters.Set("q", query)
	var response struct {
		Items []Repository `json:"items"`
	}
	pageInfo, err := c.get(ctx, "/search/repositories", parameters, &response)
	return response.Items, pageInfo, err
}

func (c *Client) Repository(ctx context.Context, owner, repo string) (Repository, error) {
	if err := validateRepository(owner, repo); err != nil {
		return Repository{}, err
	}

	var repository Repository
	_, err := c.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo), nil, &repository)
	return repository, err
}

func normalizePage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

func validateRepository(owner, repo string) error {
	if len(owner) == 0 || len(owner) > 39 || len(repo) == 0 || len(repo) > 100 ||
		!repositoryPart.MatchString(owner) || !repositoryPart.MatchString(repo) {
		return &APIError{Kind: ErrorInvalid}
	}
	return nil
}
