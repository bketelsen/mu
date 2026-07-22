package github

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/service"
)

var callService = service.Call
var getAccount = auth.GetAccount
var isOwner = auth.IsOwner

func requireOwnerAccount(accountID string) error {
	acc, err := getAccount(accountID)
	if err != nil || !isOwner(acc.ID) {
		return errors.New("owner access required")
	}
	return nil
}

func toolInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

func repositoriesTool(args map[string]any, accountID string) (string, error) {
	if err := requireOwnerAccount(accountID); err != nil {
		return "", err
	}
	var rsp RepositoriesResponse
	err := callService(context.Background(), "github", "Server.Repositories", &RepositoriesRequest{
		Query: stringArg(args, "query"), Page: toolInt(args["page"]), PerPage: toolInt(args["per_page"]),
	}, &rsp)
	return rsp.Text, err
}

func repositoryTool(args map[string]any, accountID string) (string, error) {
	if err := requireOwnerAccount(accountID); err != nil {
		return "", err
	}
	var rsp RepositoryResponse
	err := callService(context.Background(), "github", "Server.Repository", &RepositoryRequest{
		Owner: stringArg(args, "owner"), Repo: stringArg(args, "repo"), Resource: stringArg(args, "resource"),
		State: stringArg(args, "state"), Query: stringArg(args, "query"), Page: toolInt(args["page"]), PerPage: toolInt(args["per_page"]),
	}, &rsp)
	return rsp.Text, err
}

func searchTool(args map[string]any, accountID string) (string, error) {
	if err := requireOwnerAccount(accountID); err != nil {
		return "", err
	}
	var rsp SearchResponse
	err := callService(context.Background(), "github", "Server.Search", &SearchRequest{
		Query: stringArg(args, "query"), Owner: stringArg(args, "owner"), Repo: stringArg(args, "repo"),
		Resource: stringArg(args, "resource"), State: stringArg(args, "state"), Page: toolInt(args["page"]), PerPage: toolInt(args["per_page"]),
	}, &rsp)
	return rsp.Text, err
}

func issueTool(args map[string]any, accountID string) (string, error) {
	if err := requireOwnerAccount(accountID); err != nil {
		return "", err
	}
	var rsp IssueResponse
	err := callService(context.Background(), "github", "Server.Issue", &IssueRequest{
		Owner: stringArg(args, "owner"), Repo: stringArg(args, "repo"), Number: toolInt(args["number"]),
	}, &rsp)
	return rsp.Text, err
}

func stringArg(args map[string]any, name string) string {
	v, _ := args[name].(string)
	return v
}

var registerToolsOnce sync.Once

// RegisterTools registers the read-only GitHub MCP tools once per process.
func RegisterTools() {
	registerToolsOnce.Do(func() {
		api.RegisterToolWithAuth(api.Tool{
			Name: "github_repositories", Description: "List accessible repositories or search public GitHub repositories",
			Params: repositoryListParams(),
		}, repositoriesTool)
		api.RegisterToolWithAuth(api.Tool{
			Name: "github_repository", Description: "Inspect a GitHub repository, its issues, or pull requests",
			Params: repositoryParams(false),
		}, repositoryTool)
		api.RegisterToolWithAuth(api.Tool{
			Name: "github_search", Description: "Search GitHub issues and pull requests",
			Params: searchParams(),
		}, searchTool)
		api.RegisterToolWithAuth(api.Tool{
			Name: "github_issue", Description: "Read a GitHub issue or pull-request thread and comments",
			Params: []api.ToolParam{
				{Name: "owner", Type: "string", Description: "Repository owner", Required: true},
				{Name: "repo", Type: "string", Description: "Repository name", Required: true},
				{Name: "number", Type: "number", Description: "Issue or pull-request number", Required: true},
			},
		}, issueTool)
	})
}

func repositoryListParams() []api.ToolParam {
	return []api.ToolParam{
		{Name: "query", Type: "string", Description: "Optional repository search query"},
		{Name: "page", Type: "number", Description: "Page number"},
		{Name: "per_page", Type: "number", Description: "Results per page"},
	}
}

func repositoryParams(required bool) []api.ToolParam {
	return []api.ToolParam{
		{Name: "owner", Type: "string", Description: "Repository owner", Required: required},
		{Name: "repo", Type: "string", Description: "Repository name", Required: required},
		{Name: "resource", Type: "string", Description: "Resource: metadata (default), issues, or pulls"},
		{Name: "state", Type: "string", Description: "Item state: open, closed, or all"},
		{Name: "query", Type: "string", Description: "Optional item search query"},
		{Name: "page", Type: "number", Description: "Page number"},
		{Name: "per_page", Type: "number", Description: "Results per page"},
	}
}

func searchParams() []api.ToolParam {
	params := repositoryParams(false)
	params[2].Description = "Resource: issues, pulls, or all"
	return params
}
