package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"mu/internal/app"
	"mu/internal/auth"
)

type workspaceState struct {
	Owner, Repo, Tab, State, Query string
	Page, Number                   int
}

type workspaceData struct {
	State        workspaceState
	Repositories RepositoriesResponse
	Repository   RepositoryResponse
	Thread       IssueResponse
	Err          error
	ContentErr   error
}

type adminCheck func(*http.Request) (*auth.Session, *auth.Account, error)

func newHandler(server *Server, authorize adminCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWorkspace(server, authorize, w, r)
	})
}

// NewHandler returns the production GitHub workspace handler.
func NewHandler(server *Server) http.Handler { return newHandler(server, auth.RequireAdmin) }

// Handler serves the production GitHub workspace.
func Handler(w http.ResponseWriter, r *http.Request) { NewHandler(defaultServer).ServeHTTP(w, r) }

func handleWorkspace(server *Server, authorize adminCheck, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if _, _, err := authorize(r); err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	state := parseWorkspaceState(r)
	data := workspaceData{State: state}
	data.Err = server.Repositories(context.Background(), &RepositoriesRequest{Page: state.Page, PerPage: defaultPerPage}, &data.Repositories)
	if data.Err == nil && state.Owner == "" && state.Repo == "" && len(data.Repositories.Repositories) > 0 {
		data.State.Owner = data.Repositories.Repositories[0].Owner.Login
		data.State.Repo = data.Repositories.Repositories[0].Name
		if data.State.Owner == "" || data.State.Repo == "" {
			data.State.Owner, data.State.Repo = repositoryParts(data.Repositories.Repositories[0].FullName)
		}
	}
	if data.Err == nil && data.State.Owner != "" && data.State.Repo != "" {
		data.ContentErr = server.Repository(context.Background(), &RepositoryRequest{Owner: data.State.Owner, Repo: data.State.Repo, Resource: data.State.Tab, State: data.State.State, Query: data.State.Query, Page: data.State.Page, PerPage: defaultPerPage}, &data.Repository)
	}
	if data.ContentErr == nil && data.State.Number > 0 && data.State.Owner != "" && data.State.Repo != "" {
		data.ContentErr = server.Issue(context.Background(), &IssueRequest{Owner: data.State.Owner, Repo: data.State.Repo, Number: data.State.Number}, &data.Thread)
	}
	app.Respond(w, r, app.Response{Title: "GitHub", Description: "Repositories, issues, and pull requests", HTML: renderWorkspace(data)})
}

func parseWorkspaceState(r *http.Request) workspaceState {
	query := r.URL.Query()
	state := workspaceState{Owner: query.Get("owner"), Repo: query.Get("repo"), Tab: query.Get("tab"), State: query.Get("state"), Query: query.Get("q"), Page: positiveInt(query.Get("page")), Number: positiveInt(query.Get("number"))}
	if state.Tab != "pulls" {
		state.Tab = "issues"
	}
	if state.State != "closed" && state.State != "all" {
		state.State = "open"
	}
	if state.Page == 0 {
		state.Page = 1
	}
	return state
}

func positiveInt(value string) int {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		return 0
	}
	return number
}

func repositoryParts(fullName string) (string, string) {
	for i := range fullName {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return "", ""
}

func isNotConfigured(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Kind == ErrorNotConfigured
}
