package github

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/auth"
)

func stubTools(t *testing.T, account *auth.Account, err error, call func(string, any, any) error) {
	t.Helper()
	originalGetAccount := getAccount
	originalCallService := callService
	originalIsOwner := isOwner
	getAccount = func(string) (*auth.Account, error) { return account, err }
	isOwner = func(id string) bool { return account != nil && id == account.ID }
	callService = func(_ context.Context, _ string, method string, req, rsp any) error {
		return call(method, req, rsp)
	}
	t.Cleanup(func() {
		getAccount = originalGetAccount
		callService = originalCallService
		isOwner = originalIsOwner
	})
}

func TestGitHubToolRequiresOwner(t *testing.T) {
	called := false
	stubTools(t, &auth.Account{ID: "user"}, nil, func(string, any, any) error {
		called = true
		return nil
	})
	isOwner = func(string) bool { return false }

	_, err := repositoriesTool(nil, "user")
	if err == nil || err.Error() != "owner access required" {
		t.Fatalf("repositoriesTool error = %v, want owner access required", err)
	}
	if called {
		t.Fatal("repositoriesTool called the service before checking owner access")
	}
}

func TestGitHubToolRequestMapping(t *testing.T) {
	tests := []struct {
		name    string
		handler func(map[string]any, string) (string, error)
		args    map[string]any
		method  string
		check   func(*testing.T, any)
	}{
		{
			name: "repositories", handler: repositoriesTool,
			args: map[string]any{"query": "mu", "page": "2", "per_page": float64(50)}, method: "Server.Repositories",
			check: func(t *testing.T, req any) {
				got, ok := req.(*RepositoriesRequest)
				if !ok || got.Query != "mu" || got.Page != 2 || got.PerPage != 50 {
					t.Fatalf("request = %#v", req)
				}
			},
		},
		{
			name: "repository issues", handler: repositoryTool,
			args: map[string]any{"owner": "micro", "repo": "mu", "resource": "issues", "state": "open", "query": "bug", "page": float64(3), "per_page": "20"}, method: "Server.Repository",
			check: func(t *testing.T, req any) {
				got, ok := req.(*RepositoryRequest)
				if !ok || got.Owner != "micro" || got.Repo != "mu" || got.Resource != "issues" || got.State != "open" || got.Query != "bug" || got.Page != 3 || got.PerPage != 20 {
					t.Fatalf("request = %#v", req)
				}
			},
		},
		{
			name: "search", handler: searchTool,
			args: map[string]any{"owner": "micro", "repo": "mu", "resource": "pulls", "state": "closed", "query": "docs", "page": "4", "per_page": float64(10)}, method: "Server.Search",
			check: func(t *testing.T, req any) {
				got, ok := req.(*SearchRequest)
				if !ok || got.Owner != "micro" || got.Repo != "mu" || got.Resource != "pulls" || got.State != "closed" || got.Query != "docs" || got.Page != 4 || got.PerPage != 10 {
					t.Fatalf("request = %#v", req)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTools(t, &auth.Account{ID: "admin"}, nil, func(method string, req, rsp any) error {
				if method != tt.method {
					t.Fatalf("method = %q, want %q", method, tt.method)
				}
				tt.check(t, req)
				switch response := rsp.(type) {
				case *RepositoriesResponse:
					response.Text = "repositories text"
				case *RepositoryResponse:
					response.Text = "repository text"
				case *SearchResponse:
					response.Text = "search text"
				}
				return nil
			})

			got, err := tt.handler(tt.args, "admin")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(got, " text") {
				t.Fatalf("handler result = %q, want RPC response text", got)
			}
		})
	}
}

func TestGitHubIssueToolNumberMapping(t *testing.T) {
	for _, number := range []any{"42", float64(43)} {
		t.Run("number", func(t *testing.T) {
			stubTools(t, &auth.Account{ID: "admin"}, nil, func(method string, req, rsp any) error {
				got, ok := req.(*IssueRequest)
				if method != "Server.Issue" || !ok || got.Owner != "micro" || got.Repo != "mu" || got.Number == 0 {
					t.Fatalf("method = %q, request = %#v", method, req)
				}
				rsp.(*IssueResponse).Text = "issue text"
				return nil
			})
			got, err := issueTool(map[string]any{"owner": "micro", "repo": "mu", "number": number}, "admin")
			if err != nil || got != "issue text" {
				t.Fatalf("issueTool() = %q, %v", got, err)
			}
		})
	}
}

func TestRegisterTools(t *testing.T) {
	RegisterTools()
	RegisterTools()
	descriptions := api.ToolDescriptions()
	for _, name := range []string{"github_repositories", "github_repository", "github_search", "github_issue"} {
		if count := strings.Count(descriptions, "- "+name+":"); count != 1 {
			t.Fatalf("%s registrations = %d, want 1", name, count)
		}
	}
	if strings.Contains(descriptions, "github_create") || strings.Contains(descriptions, "github_update") || strings.Contains(descriptions, "github_delete") {
		t.Fatal("GitHub tool registration exposed a write operation")
	}
}

func TestRequireOwnerAccountLookupFailure(t *testing.T) {
	stubTools(t, nil, errors.New("missing"), func(string, any, any) error { return nil })
	if err := requireOwnerAccount("owner"); err == nil || err.Error() != "owner access required" {
		t.Fatalf("requireOwnerAccount error = %v, want owner access required", err)
	}
}
