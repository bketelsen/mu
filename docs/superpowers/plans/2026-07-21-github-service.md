# GitHub Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an administrator-only, read-only GitHub.com service for browsing repositories and reading or searching issues and pull requests through Mu's web, go-micro, MCP/CLI, and agent surfaces.

**Architecture:** A focused top-level `github` package owns a small `net/http` GitHub REST client, typed resource operations, go-micro service contracts, authenticated MCP adapters, and a server-rendered repository workspace. Every surface calls the same `Server` methods; the GitHub client reads `GITHUB_TOKEN` for each operation and persists no upstream data.

**Tech Stack:** Go 1.25, standard-library `net/http` and `httptest`, go-micro v6 service RPC, Mu's `internal/settings`, `internal/auth`, `internal/api`, and server-rendered app shell.

## Global Constraints

- Target GitHub.com only at `https://api.github.com`; test injection may replace the API base URL.
- Read `GITHUB_TOKEN` through `settings.Get` for every operation; never reuse `COPILOT_GITHUB_TOKEN`.
- Require a Mu administrator at every web and legacy MCP entry point.
- Keep the integration read-only: no create, update, comment, review, close, merge, or other mutation endpoints.
- Add no third-party dependency, cache, background goroutine, store write, or search-index write.
- Do not log or return tokens, authorization headers, or unbounded upstream response bodies.
- Escape every GitHub-provided string before HTML rendering; render body/comment text with preserved whitespace, not executable Markdown or HTML.
- Keep the optional raw go-micro MCP gateway operator-trusted; this plan does not add gateway authentication.
- Default list pages to 30 records and cap requested pages at 100 records.
- Limit one GitHub response body to 4 MiB, search text to 256 characters, and one detail response to the first 100 chronological discussion comments.
- Use a 15-second production HTTP timeout and propagate request context cancellation.
- Keep GitHub MCP tools out of `guestAllowedTools`.

---

## File Map

### New Files

- `github/types.go`: GitHub resource records, pagination, typed upstream errors, validation constants.
- `github/client.go`: authenticated GET transport, bounded decoding, pagination parsing, error translation.
- `github/repositories.go`: repository list/search/detail REST operations.
- `github/issues.go`: issue, pull-request, search, and thread REST operations.
- `github/service.go`: go-micro `Server`, request/response contracts, validation, model-ready formatting.
- `github/github.go`: production client/server construction and service registration.
- `github/tools.go`: four admin-checked legacy MCP tool definitions and argument adapters.
- `github/handler.go`: admin-only repository workspace request handling.
- `github/render.go`: escaped server-rendered workspace HTML.
- `github/client_test.go`, `github/repositories_test.go`, `github/issues_test.go`, `github/service_test.go`, `github/tools_test.go`, `github/handler_test.go`: focused tests.
- `internal/app/html/github.svg`: monochrome navigation icon.

### Modified Files

- `main.go`: import/load GitHub, register tools and `/github`, and mark the route authenticated.
- `admin/env.go`: expose `GITHUB_TOKEN` in its own settings group.
- `admin/env_test.go`: verify the setting is allowlisted and treated as a secret.
- `agent/micro/registry.go`: register the GitHub specialist.
- `agent/micro/router.go`: deterministic GitHub routing.
- `agent/micro/router_test.go`: direct and keyword routing coverage.
- `agent/micro/execute_test.go`: verify GitHub tools remain private for guests.
- `internal/app/app.go`: add the GitHub navigation entry.
- `internal/app/html/mu.css`: repository workspace, item, tab, and mobile styles.
- `docs/ENVIRONMENT_VARIABLES.md`: document `GITHUB_TOKEN` and distinguish it from the Copilot token.
- `docs/SERVICES.md`: mark repositories/issues as shipped.
- `.gitignore`: ignore visual-companion state under `.superpowers/`.

---

### Task 1: GitHub Transport And Shared Types

**Files:**
- Create: `github/types.go`
- Create: `github/client.go`
- Create: `github/client_test.go`

**Interfaces:**
- Produces: `NewClient(httpClient *http.Client, baseURL string, token func() string) *Client`
- Produces: `(*Client).get(ctx context.Context, endpoint string, query url.Values, dst any) (PageInfo, error)`
- Produces: `APIError`, `ErrorKind`, `PageInfo`, and shared resource records consumed by all later tasks.

- [ ] **Step 1: Write failing transport tests**

Create table-driven tests that prove headers, live token lookup, pagination, body bounds, cancellation, and stable failures. Use this structure in `github/client_test.go`:

```go
func TestClientGetHeadersAndPagination(t *testing.T) {
	var token = "first-token"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
			t.Fatalf("X-GitHub-Api-Version = %q", got)
		}
		w.Header().Set("Link", `<https://api.github.com/resource?page=3>; rel="next", <https://api.github.com/resource?page=1>; rel="prev"`)
		io.WriteString(w, `[{"id":1}]`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return token })
	var got []struct{ ID int64 `json:"id"` }
	page, err := c.get(context.Background(), "/resource", nil, &got)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || page.Next != 3 || page.Prev != 1 {
		t.Fatalf("got %#v, page %#v", got, page)
	}

	token = "rotated-token"
	if _, err := c.get(context.Background(), "/resource", nil, &got); err != nil {
		t.Fatal(err)
	}
}
```

Add `TestClientGetErrors` with cases for no token, `401`, rate-limited `403` with `X-RateLimit-Remaining: 0` and `X-RateLimit-Reset`, other `403`, `404`, `429`, and `500`. Assert `errors.As(err, *APIError)` and the exact `Kind`. Add `TestClientRejectsOversizedResponse` and `TestClientHonorsCanceledContext`.

- [ ] **Step 2: Run the transport tests and confirm they fail**

Run: `go test ./github -run 'TestClient' -count=1`

Expected: FAIL because the `github` package, `Client`, and shared types do not exist.

- [ ] **Step 3: Add shared types and stable errors**

Create `github/types.go` with these exact public shapes:

```go
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
	Kind   ErrorKind
	Status int
	Reset  time.Time
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
	ID          int64       `json:"id"`
	Number      int         `json:"number"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	State       string      `json:"state"`
	StateReason string      `json:"state_reason"`
	HTMLURL     string      `json:"html_url"`
	User        User        `json:"user"`
	Labels      []Label     `json:"labels"`
	Comments    int         `json:"comments"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	ClosedAt    *time.Time  `json:"closed_at"`
	PullRequest *PullMarker `json:"pull_request,omitempty"`
	RepositoryURL string    `json:"repository_url"`
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
	Head      struct{ Ref string `json:"ref"` } `json:"head"`
	Base      struct{ Ref string `json:"ref"` } `json:"base"`
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
```

- [ ] **Step 4: Implement the bounded GET transport**

Create `github/client.go`. Define `githubAPIVersion = "2022-11-28"`, `userAgent = "Mu/1.0"`, and:

```go
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      func() string
}

func NewClient(httpClient *http.Client, baseURL string, token func() string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/"), token: token}
}
```

Implement `get` so it rejects an empty token before constructing the request, applies query values, sends only GET requests, reads through `io.LimitReader(resp.Body, maxBodyBytes+1)`, rejects a body larger than `maxBodyBytes`, translates status codes without including the upstream body, and decodes successful JSON. Implement `parseLinks(string) PageInfo` using `net/http` link values and `net/url` query parsing. For `403`, classify as rate-limited only when `X-RateLimit-Remaining == "0"`; parse `X-RateLimit-Reset` as Unix seconds. Treat `429` as rate-limited and parse `Retry-After` only for internal retry metadata, without sleeping.

- [ ] **Step 5: Run and format the transport tests**

Run: `gofmt -w github/types.go github/client.go github/client_test.go && go test ./github -run 'TestClient' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the transport foundation**

```bash
git add github/types.go github/client.go github/client_test.go
git commit -m "feat: add GitHub API transport"
```

---

### Task 2: Repository Operations

**Files:**
- Create: `github/repositories.go`
- Create: `github/repositories_test.go`

**Interfaces:**
- Consumes: `Client.get`, `Repository`, and `PageInfo` from Task 1.
- Produces: `(*Client).Repositories(ctx context.Context, query string, page, perPage int) ([]Repository, PageInfo, error)`
- Produces: `(*Client).Repository(ctx context.Context, owner, repo string) (Repository, error)`
- Produces: `normalizePage(page, perPage int) (int, int)` and `validateRepository(owner, repo string) error` for later issue operations.

- [ ] **Step 1: Write failing repository tests**

Test these exact behaviors with `httptest.Server`:

```go
func TestRepositoriesListsVisibleRepos(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" || r.URL.Query().Get("page") != "2" ||
			r.URL.Query().Get("per_page") != "30" || r.URL.Query().Get("sort") != "updated" ||
			r.URL.Query().Get("direction") != "desc" ||
			r.URL.Query().Get("affiliation") != "owner,collaborator,organization_member" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `[{"id":1,"full_name":"micro/mu"},{"id":2,"full_name":"micro/go-micro"}]`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	got, page, err := c.Repositories(context.Background(), "", 2, 30)
	if err != nil { t.Fatal(err) }
	if len(got) != 2 || got[0].FullName != "micro/mu" || page.Page != 2 {
		t.Fatalf("got %#v, page %#v", got, page)
	}
}

func TestRepositoriesSearchesWhenQueryPresent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" || r.URL.Query().Get("q") != "mu" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"items":[{"id":1,"full_name":"micro/mu"}]}`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	got, _, err := c.Repositories(context.Background(), "mu", 1, 30)
	if err != nil { t.Fatal(err) }
	if len(got) != 1 || got[0].FullName != "micro/mu" { t.Fatalf("got %#v", got) }
}

func TestRepositoryEscapesAndValidatesCoordinates(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/repos/micro/mu" { t.Fatalf("path = %q", r.URL.Path) }
		io.WriteString(w, `{"id":1,"full_name":"micro/mu"}`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	if _, err := c.Repository(context.Background(), "micro", "mu"); err != nil { t.Fatal(err) }
	if _, err := c.Repository(context.Background(), "micro/other", "mu"); err == nil { t.Fatal("accepted invalid owner") }
	if _, err := c.Repository(context.Background(), "micro", ""); err == nil { t.Fatal("accepted empty repo") }
	if calls != 1 { t.Fatalf("upstream calls = %d", calls) }
}
```

Also test page defaults (`0,0` becomes `1,30`), the `perPage` cap (`200` becomes `100`), and a 257-character query returning `ErrorInvalid`.

- [ ] **Step 2: Run repository tests and confirm failure**

Run: `go test ./github -run 'TestRepositor' -count=1`

Expected: FAIL because repository operations are undefined.

- [ ] **Step 3: Implement repository list, search, detail, and validation**

Create `github/repositories.go` with:

```go
var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func normalizePage(page, perPage int) (int, int) {
	if page < 1 { page = 1 }
	if perPage < 1 { perPage = defaultPerPage }
	if perPage > maxPerPage { perPage = maxPerPage }
	return page, perPage
}

func validateRepository(owner, repo string) error {
	if len(owner) == 0 || len(owner) > 39 || len(repo) == 0 || len(repo) > 100 ||
		!repositoryPart.MatchString(owner) || !repositoryPart.MatchString(repo) {
		return &APIError{Kind: ErrorInvalid}
	}
	return nil
}
```

For an empty query, call `/user/repos` with `affiliation=owner,collaborator,organization_member`, `sort=updated`, and `direction=desc`. For a non-empty query, call `/search/repositories`, decode `struct { Items []Repository }`, and reject text longer than `maxQueryLength`. Set returned `PageInfo.Page` and `PerPage` from normalized inputs because GitHub's `Link` header only describes neighboring pages. `Repository` must validate coordinates and call `"/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)`.

- [ ] **Step 4: Run repository tests**

Run: `gofmt -w github/repositories.go github/repositories_test.go && go test ./github -run 'TestRepositor' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit repository operations**

```bash
git add github/repositories.go github/repositories_test.go
git commit -m "feat: add GitHub repository queries"
```

---

### Task 3: Issue, Pull Request, Search, And Thread Operations

**Files:**
- Create: `github/issues.go`
- Create: `github/issues_test.go`

**Interfaces:**
- Consumes: Task 1 types/transport and Task 2 validation/pagination.
- Produces: `ItemOptions`, `(*Client).Issues`, `(*Client).PullRequests`, `(*Client).Search`, and `(*Client).Thread`.

- [ ] **Step 1: Write failing resource-operation tests**

Define tests around this options type:

```go
type ItemOptions struct {
	Owner   string
	Repo    string
	Query   string
	State   string
	Type    string
	Page    int
	PerPage int
}
```

Cover:

- `Issues` calls `/search/issues` with a query containing `repo:micro/mu is:issue is:open` and user search words, then decodes the `items` envelope.
- `PullRequests` without text calls `/repos/micro/mu/pulls?state=all&sort=updated&direction=desc`.
- `Search` requires non-empty text or repository scope, accepts only `issues`, `pulls`, or `all`, and adds `is:issue` or `is:pr` qualifiers when requested.
- `Thread` first calls `/repos/micro/mu/issues/42`, follows with `/issues/42/comments?per_page=100`, and only calls `/pulls/42` when the issue has a non-nil `pull_request` marker.
- Comment order is oldest first and is capped at 100.
- Invalid state, type, number, owner/repo, and overlong query fail before HTTP.

- [ ] **Step 2: Run issue tests and confirm failure**

Run: `go test ./github -run 'Test(Issues|PullRequests|Search|Thread)' -count=1`

Expected: FAIL because `ItemOptions` and the methods do not exist.

- [ ] **Step 3: Implement qualified search and pull-request listing**

Create `github/issues.go`. Implement `validateItemOptions` with states `open`, `closed`, or `all`, types `issues`, `pulls`, or `all`, and Task 2 repository validation whenever either owner or repo is set. Build qualifiers as separate strings and join with spaces; never concatenate an unvalidated owner/repo into a query.

Use these signatures:

```go
func (c *Client) Issues(ctx context.Context, opts ItemOptions) ([]Issue, PageInfo, error)
func (c *Client) PullRequests(ctx context.Context, opts ItemOptions) ([]PullRequest, PageInfo, error)
func (c *Client) Search(ctx context.Context, opts ItemOptions) ([]Issue, PageInfo, error)
```

`Issues` always adds `is:issue`; `Search` adds `is:issue` for `issues`, `is:pr` for `pulls`, and neither for `all`. For repository pull requests with empty query, use `/repos/{owner}/{repo}/pulls`; when text is present, use `/search/issues` with `is:pr` so text filtering remains server-side. Decode search responses through:

```go
var result struct {
	Items []Issue `json:"items"`
}
```

- [ ] **Step 4: Implement bounded thread detail**

Add:

```go
func (c *Client) Thread(ctx context.Context, owner, repo string, number int) (Thread, error)
```

Reject numbers below 1. Fetch the issue record and up to 100 issue-style discussion comments. Sort comments with `sort.SliceStable` by `CreatedAt`. If `Issue.PullRequest != nil`, fetch `/pulls/{number}` into `PullRequest` and set `Thread.PullRequest`; do not fetch reviews or review comments.

- [ ] **Step 5: Run all client resource tests**

Run: `gofmt -w github/issues.go github/issues_test.go && go test ./github -run 'Test(Issues|PullRequests|Search|Thread)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit issue and pull-request operations**

```bash
git add github/issues.go github/issues_test.go
git commit -m "feat: add GitHub issue and pull request queries"
```

---

### Task 4: Go-Micro Service Contracts And Registration

**Files:**
- Create: `github/service.go`
- Create: `github/github.go`
- Create: `github/service_test.go`

**Interfaces:**
- Consumes: all `Client` operations from Tasks 1-3.
- Produces: `NewServer(client *Client) *Server`, `DefaultServer() *Server`, `Load()`, and RPC endpoints `Server.Repositories`, `Server.Repository`, `Server.Search`, and `Server.Issue`.
- Produces: structured response types plus bounded `Text` fields used by tools and chat.

- [ ] **Step 1: Write failing service tests**

Create a fake GitHub server and instantiate `NewServer(NewClient(...))`. Test all four endpoints directly. Use request/response contracts with these fields:

```go
type RepositoriesRequest struct { Query string; Page, PerPage int }
type RepositoriesResponse struct { Repositories []Repository; Page PageInfo; Text string }

type RepositoryRequest struct {
	Owner, Repo, Resource, State, Query string
	Page, PerPage int
}
type RepositoryResponse struct {
	Repository Repository
	Issues []Issue
	PullRequests []PullRequest
	Page PageInfo
	Text string
}

type SearchRequest struct {
	Query, Owner, Repo, Resource, State string
	Page, PerPage int
}
type SearchResponse struct { Items []Issue; Page PageInfo; Text string }

type IssueRequest struct { Owner, Repo string; Number int }
type IssueResponse struct { Thread Thread; Text string }
```

Assert that `RepositoryRequest.Resource` accepts only `metadata` (default), `issues`, and `pulls`; that `SearchRequest.Resource` maps to `issues`, `pulls`, or `all`; and that text includes full repository names, item type/number, status, URL, labels, body, and comments without exceeding 32 KiB.

- [ ] **Step 2: Run service tests and confirm failure**

Run: `go test ./github -run 'TestServer' -count=1`

Expected: FAIL because `Server` and RPC contracts do not exist.

- [ ] **Step 3: Implement RPC contracts and model-ready formatting**

Create `github/service.go` with:

```go
type Server struct{ client *Client }

func NewServer(client *Client) *Server { return &Server{client: client} }
```

Implement the four `func (s *Server) Method(ctx context.Context, req *Request, rsp *Response) error` methods. Return `ErrorInvalid` for nil requests or unsupported modes. Keep structured fields intact for the web handler. Build text with `strings.Builder`; normalize body/comment whitespace with `strings.TrimSpace`, include at most 20 list records in text, include at most 100 comments for detail, and truncate final text safely at 32 KiB. Use plain text, not JSON or HTML.

- `Repositories` calls `Client.Repositories` and fills all three response fields.
- `Repository` always calls `Client.Repository`; `metadata` stops there, `issues` additionally calls `Client.Issues`, and `pulls` additionally calls `Client.PullRequests`.
- `Search` requires a non-empty query and calls `Client.Search` with the requested repository scope, type, state, and pagination.
- `Issue` calls `Client.Thread` and formats issue or pull-request metadata plus chronological comments.

Add JSON and `description` tags to every exported request/response field and `@example` comments above each RPC method so the optional go-micro MCP gateway can derive usable schemas.

- [ ] **Step 4: Add production construction and registration**

Create `github/github.go`:

```go
package github

import (
	"net/http"
	"time"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/settings"
)

var defaultServer = NewServer(NewClient(
	&http.Client{Timeout: 15 * time.Second},
	"https://api.github.com",
	func() string { return settings.Get("GITHUB_TOKEN") },
))

func DefaultServer() *Server { return defaultServer }

func Load() {
	if err := service.Register("github", defaultServer); err != nil {
		app.Log("github", "service register failed: %v", err)
	}
}
```

In `github/service_test.go`, add one unique-name mesh test that registers the fake-backed server and calls `service.Call(ctx, serviceName, "Server.Repositories", ...)`. Do not run this test in parallel.

- [ ] **Step 5: Run service and package tests**

Run: `gofmt -w github/service.go github/github.go github/service_test.go && go test ./github -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the go-micro service**

```bash
git add github/service.go github/github.go github/service_test.go
git commit -m "feat: expose GitHub service over go-micro"
```

---

### Task 5: Settings, Authenticated MCP Tools, And Agent Routing

**Files:**
- Create: `github/tools.go`
- Create: `github/tools_test.go`
- Modify: `admin/env.go:18-77`
- Create or Modify: `admin/env_test.go`
- Modify: `agent/micro/registry.go:3-93`
- Modify: `agent/micro/router.go:106-160`
- Modify: `agent/micro/router_test.go:8-171`
- Modify: `agent/micro/execute_test.go:25-41`
- Modify: `main.go:21-59,100-170`

**Interfaces:**
- Consumes: Task 4 RPC request/response types and `service.Call`.
- Produces: `github.RegisterTools()` and four legacy tool handlers.
- Produces: the `github` specialist and deterministic route.

- [ ] **Step 1: Write failing settings and tool-adapter tests**

In `admin/env_test.go`, assert that exactly one `settingGroups` entry named `GitHub` contains `GITHUB_TOKEN`, and that the secret-key predicate used by rendering recognizes `TOKEN`. Extract the inline predicate at `admin/env.go:129-132` to:

```go
func isSecretSetting(key string) bool {
	u := strings.ToUpper(key)
	return strings.Contains(u, "KEY") || strings.Contains(u, "SECRET") ||
		strings.Contains(u, "TOKEN") || strings.Contains(u, "PASS")
}
```

In `github/tools_test.go`, inject fake `callService` and `getAccount` functions, restore both with `t.Cleanup`, and call the unexported handler functions directly. Return `&auth.Account{ID: "admin", Admin: true}` for the admin case and `&auth.Account{ID: "user"}` for the denial case. Assert:

- non-admin calls return `admin access required` before `callService` runs;
- repository arguments map to `RepositoriesRequest`;
- `resource=issues` maps to `RepositoryRequest`;
- search maps owner/repo/resource/state/query;
- string or JSON-number `number` maps to `IssueRequest.Number`;
- each handler returns only the RPC response `Text`.

Add `TestRegisterTools` that calls `RegisterTools()`, reads `api.ToolDescriptions()`, and asserts all four canonical names appear exactly once with no aliases that expose a write operation.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `go test ./admin ./github -run 'Test(GitHubTokenSetting|GitHubTool)' -count=1`

Expected: FAIL because the setting and tools are absent.

- [ ] **Step 3: Add the live setting and four tool adapters**

Add this setting group after Search in `admin/env.go`:

```go
{"GitHub", []string{"GITHUB_TOKEN"}},
```

Create `github/tools.go`. Define:

```go
var callService = service.Call
var getAccount = auth.GetAccount

func requireAdminAccount(accountID string) error {
	acc, err := getAccount(accountID)
	if err != nil || !acc.Admin {
		return errors.New("admin access required")
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
```

Implement `repositoriesTool`, `repositoryTool`, `searchTool`, and `issueTool` as `func(map[string]any, string) (string, error)`. Each calls `requireAdminAccount` first, constructs the exact Task 4 request, invokes `callService(context.Background(), "github", "Server.<Method>", req, &rsp)`, and returns `rsp.Text`.

`RegisterTools()` calls `api.RegisterToolWithAuth` four times with names `github_repositories`, `github_repository`, `github_search`, and `github_issue`. Declare every parameter from the Task 4 contracts, mark owner/repo/number required only for `github_issue`, and describe `resource` enum values in parameter descriptions.

- [ ] **Step 4: Register the service and tools in server startup**

Add `"mu/github"` to `main.go` imports. After `app.Load()` and before dependent tools are available, call:

```go
github.Load()
github.RegisterTools()
```

Do not add CLI dispatch code: arbitrary MCP tool names and live tool help already work through `internal/cli`.

- [ ] **Step 5: Write failing agent tests**

Extend tests with:

```go
func TestGitHubAgentRegistration(t *testing.T) {
	a := Get("github")
	if a == nil { t.Fatal("github agent is not registered") }
	want := []string{"github_repositories", "github_repository", "github_search", "github_issue"}
	if !reflect.DeepEqual(a.Tools, want) { t.Fatalf("Tools = %v", a.Tools) }
}
```

Add route cases for `@github show micro/mu`, `show GitHub issues for micro/mu`, `open pull requests in micro/mu`, and `find the repository micro/mu`. Add all four GitHub tool names to the private-tool loop in `TestGuestAllowedToolsCoverPublicCoreServices` and assert each is rejected for guests.

- [ ] **Step 6: Implement the specialist and deterministic route**

Register:

```go
Register(&Agent{
	ID:          "github",
	Name:        "GitHub Agent",
	Description: "Repositories, issues, and pull requests",
	SystemPrompt: `You are the GitHub specialist on Mu. Use live GitHub tools to inspect repositories, issues, pull requests, and discussion comments. Quote repository names and item numbers, link to GitHub, distinguish issues from pull requests, and never claim to modify GitHub because your tools are read-only.`,
	Tools:       []string{"github_repositories", "github_repository", "github_search", "github_issue"},
	MemoryScope: "github",
})
```

In `keywordRoute`, before generic search routing, route to `github` when the prompt contains `github`, `repository`, `repositories`, `pull request`, or `pull requests`. Route bare `issue`/`issues` only when `github` or a repository coordinate is also present; do not route every conversational use of “issue.” Do not modify `guestAllowedTools`.

- [ ] **Step 7: Run settings, tool, and agent tests**

Run: `gofmt -w admin/env.go admin/env_test.go github/tools.go github/tools_test.go agent/micro/registry.go agent/micro/router.go agent/micro/router_test.go agent/micro/execute_test.go main.go && go test ./admin ./github ./agent/micro -count=1`

Expected: PASS.

- [ ] **Step 8: Commit settings and agent/tool access**

```bash
git add admin/env.go admin/env_test.go github/tools.go github/tools_test.go agent/micro/registry.go agent/micro/router.go agent/micro/router_test.go agent/micro/execute_test.go main.go
git commit -m "feat: add GitHub tools and agent routing"
```

---

### Task 6: Admin Repository Workspace

**Files:**
- Create: `github/handler.go`
- Create: `github/render.go`
- Create: `github/handler_test.go`
- Create: `internal/app/html/github.svg`
- Modify: `internal/app/html/mu.css`
- Modify: `internal/app/app.go:315-344`
- Modify: `main.go:1165-1226,1391-1404`

**Interfaces:**
- Consumes: `Server` and structured RPC responses from Task 4.
- Produces: `NewHandler(server *Server) http.Handler`, production `Handler`, and the `/github` workspace.

- [ ] **Step 1: Write failing handler security and rendering tests**

Use a fake GitHub server and the internal `newHandler(NewServer(fakeClient), adminCheck)` constructor. The injected `adminCheck` returns either an administrator account or an error, allowing each test to prove authorization happens before upstream access without mutating auth package globals.

Required tests:

```go
func denyAdmin(*http.Request) (*auth.Session, *auth.Account, error) {
	return nil, nil, errors.New("admin access required")
}

func allowAdmin(*http.Request) (*auth.Session, *auth.Account, error) {
	return &auth.Session{Account: "admin"}, &auth.Account{ID: "admin", Admin: true}, nil
}

func TestHandlerRequiresAdminBeforeGitHubCall(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer ts.Close()
	h := newHandler(NewServer(NewClient(ts.Client(), ts.URL, func() string { return "test-token" })), denyAdmin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github", nil))
	if w.Code != http.StatusForbidden { t.Fatalf("status = %d", w.Code) }
	if calls != 0 { t.Fatalf("upstream calls = %d", calls) }
}

func TestHandlerShowsMissingTokenSetup(t *testing.T) {
	h := newHandler(NewServer(NewClient(http.DefaultClient, "https://api.github.invalid", func() string { return "" })), allowAdmin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/github", nil))
	if body := w.Body.String(); !strings.Contains(body, "GITHUB_TOKEN") || !strings.Contains(body, `/admin/env`) {
		t.Fatalf("missing setup state: %s", body)
	}
}
```

Add `TestHandlerRendersRepositoryWorkspace`, `TestHandlerEscapesGitHubContent`, and `TestHandlerParsesBoundedQueryState` using one table-driven fake upstream handler. Return malicious `<script>alert(1)</script>` text in repository descriptions, issue bodies, and comments; assert the response contains `&lt;script&gt;` and not the raw tag. Assert the workspace response includes the repo rail, issue/pull tabs, selected item, pagination links, and external-link attributes. Assert invalid `tab`, `state`, negative `page`, and negative `number` normalize to `issues`, `open`, `1`, and no selected thread request.

Assert `target="_blank" rel="noopener noreferrer"` on GitHub links and presence of `.github-layout`, `.github-repos`, and `.github-content` hooks used by responsive CSS.

- [ ] **Step 2: Run handler tests and confirm failure**

Run: `go test ./github -run 'TestHandler' -count=1`

Expected: FAIL because the web handler and renderer do not exist.

- [ ] **Step 3: Implement admin-first request handling**

Create `github/handler.go` with:

```go
type workspaceState struct {
	Owner, Repo, Tab, State, Query string
	Page, Number int
}

type workspaceData struct {
	State workspaceState
	Repositories RepositoriesResponse
	Repository RepositoryResponse
	Thread IssueResponse
	Err error
}

type adminCheck func(*http.Request) (*auth.Session, *auth.Account, error)

func newHandler(server *Server, authorize adminCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWorkspace(server, authorize, w, r)
	})
}

func NewHandler(server *Server) http.Handler {
	return newHandler(server, auth.RequireAdmin)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	NewHandler(defaultServer).ServeHTTP(w, r)
}
```

The closure must call `auth.RequireAdmin(r)` before parsing state or calling the server. On denial, call `app.Forbidden(w, r, "Admin access required")`. Accept GET only. Normalize `tab` to `issues` or `pulls`, `state` to `open`, `closed`, or `all`, page to at least 1, and number to a positive integer. List repositories first; default to the first repository only when no owner/repo is selected. Call `Server.Repository` for the active tab and `Server.Issue` when `number` is set. If the error kind is `ErrorNotConfigured`, render the setup state; otherwise render a bounded inline error card.

- [ ] **Step 4: Implement escaped workspace rendering**

Create `github/render.go`. Centralize escaping:

```go
func esc(s string) string { return html.EscapeString(s) }

func githubLink(rawURL, label string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") {
		return esc(label)
	}
	return `<a href="` + esc(u.String()) + `" target="_blank" rel="noopener noreferrer">` + esc(label) + `</a>`
}
```

Render with `strings.Builder` and `app.Respond(app.Response{Title: "GitHub", Description: "Repositories, issues, and pull requests", HTML: body})`. The HTML must contain:

- a GET search form;
- a left repository rail using `owner` and `repo` query parameters;
- issue/pull tabs preserving repository state;
- state filters and previous/next links from `PageInfo`;
- compact issue/pull rows with type, number, state, labels, author, update time, and comment count;
- a selected detail card with escaped title/body and comments in elements styled `white-space: pre-wrap`;
- an empty state and a missing-token setup card linking to `/admin/env`.

Use `url.Values.Encode()` for every internal workspace link. Never build a query string by concatenating GitHub data.
Extend the escaping test with an upstream `html_url` of `javascript:alert(1)` and assert it is rendered as plain escaped label text with no `href`.

- [ ] **Step 5: Add responsive styles and navigation asset**

Append focused CSS to `internal/app/html/mu.css`:

```css
.github-layout { display:grid; grid-template-columns:minmax(220px, 30%) minmax(0, 1fr); gap:var(--spacing-lg); }
.github-repos { border-right:1px solid var(--divider); padding-right:var(--spacing-md); }
.github-repo, .github-item { display:block; padding:var(--spacing-sm); border-bottom:1px solid var(--divider); }
.github-repo:hover, .github-item:hover { background:var(--hover-background); text-decoration:none; }
.github-repo.active { background:var(--hover-background); border-left:3px solid var(--accent-color); }
.github-tabs { display:flex; gap:var(--spacing-md); border-bottom:1px solid var(--divider); margin-bottom:var(--spacing-md); }
.github-tabs a { padding:var(--spacing-sm) 0; }
.github-tabs a.active { border-bottom:2px solid var(--accent-color); }
.github-meta { color:var(--text-muted); font-size:.85em; }
.github-body { white-space:pre-wrap; overflow-wrap:anywhere; }
.github-label { display:inline-block; border:1px solid var(--card-border); border-radius:999px; padding:1px 7px; margin-right:4px; font-size:.8em; }
@media (max-width: 760px) {
  .github-layout { grid-template-columns:1fr; }
  .github-repos { border-right:0; border-bottom:1px solid var(--divider); padding-right:0; padding-bottom:var(--spacing-md); }
}
```

Create `internal/app/html/github.svg` as a 24x24 monochrome branch/network SVG using `stroke="currentColor"`, no script, external reference, or embedded metadata. Add to `internal/app/app.go` navigation:

```html
<a href="/github"><img src="/github.svg?` + Version + `"><span class="label">GitHub</span></a>
```

- [ ] **Step 6: Register and outer-authenticate the route**

In `main.go`, add `"/github": true` to `authenticated` and:

```go
http.HandleFunc("/github", github.Handler)
```

The handler still calls `RequireAdmin`; the outer middleware is defense in depth, not the authorization decision.

- [ ] **Step 7: Run handler and app tests**

Run: `gofmt -w github/handler.go github/render.go github/handler_test.go internal/app/app.go main.go && go test ./github ./internal/app -count=1`

Expected: PASS.

- [ ] **Step 8: Commit the web workspace**

```bash
git add github/handler.go github/render.go github/handler_test.go internal/app/html/github.svg internal/app/html/mu.css internal/app/app.go main.go
git commit -m "feat: add GitHub repository workspace"
```

---

### Task 7: Documentation, Cleanup, And Full Verification

**Files:**
- Modify: `docs/ENVIRONMENT_VARIABLES.md`
- Modify: `docs/SERVICES.md:122-126`
- Modify: `.gitignore`
- Verify: all files changed in Tasks 1-6

**Interfaces:**
- Consumes: the completed service.
- Produces: operator documentation and release-quality verification evidence.

- [ ] **Step 1: Document the distinct GitHub token**

Add `GITHUB_TOKEN` to the environment variable reference table with this description:

```markdown
| `GITHUB_TOKEN` | - | Fine-grained GitHub token used by the admin-only, read-only repositories/issues service. Grant metadata, issues, and pull-request read access only for repositories Mu should expose. This is separate from `COPILOT_GITHUB_TOKEN`. |
```

Add a short configuration example near other API keys:

```bash
export GITHUB_TOKEN="github_pat_xxxxxxxxxxxx"
```

State that it can also be saved under `/admin/env` without restarting Mu.

- [ ] **Step 2: Update the service inventory and ignore companion state**

Change `docs/SERVICES.md` item 78 from planned to shipped and remove the stale “existing GitHub integration” wording:

```markdown
78. **repos / issues** ✓
```

Add this line to `.gitignore`:

```gitignore
.superpowers/
```

- [ ] **Step 3: Run targeted tests with the race detector**

Run: `go test -race ./github ./agent/micro ./admin ./internal/api ./internal/app -count=1`

Expected: PASS with no data races. If a test mutates `callService`, auth globals, or registered services, ensure that test is not parallel and restores the original value with `t.Cleanup`.

- [ ] **Step 4: Run full project verification**

Run: `go test ./... -short`

Expected: PASS.

Run: `go vet ./...`

Expected: PASS with no diagnostics.

Run: `go build ./...`

Expected: PASS.

- [ ] **Step 5: Inspect the final diff for security and scope**

Run:

```bash
git diff --check
git status --short
git diff --stat
```

Confirm no token fixture resembles a real credential, no GitHub write endpoint or method was added, no `.superpowers/` file is tracked, and the only dependency files changed are those already required by the repository (no `go.mod` or `go.sum` change).

- [ ] **Step 6: Commit documentation and cleanup**

```bash
git add docs/ENVIRONMENT_VARIABLES.md docs/SERVICES.md .gitignore
git commit -m "docs: document GitHub service configuration"
```

---

## Acceptance Walkthrough

After all automated verification passes, start Mu with a fine-grained read-only token in a local environment and manually verify:

1. A guest and a signed-in non-admin receive no repository data from `/github` or any `github_*` MCP tool.
2. An admin with no token sees the `/admin/env` setup link.
3. Saving `GITHUB_TOKEN` under `/admin/env` makes `/github` work without restarting.
4. The repository rail lists private and public repositories visible to that token, sorted by recent updates.
5. Issue and pull-request tabs remain separate; state filters, search, pagination, and detail links preserve repository context.
6. An issue and a pull request both show escaped body text and chronological discussion comments; a pull request additionally shows head/base and merge metadata.
7. `mu github_repositories`, `mu github_repository`, `mu github_search`, and `mu github_issue` work with explicit flags through the existing CLI dispatcher.
8. `@github`, GitHub issue, repository, and pull-request prompts route to the GitHub specialist, while guest execution cannot use its tools.
9. Rotating the saved token changes subsequent requests without restarting.
10. A deliberately insufficient token, invalid token, inaccessible repository, and rate-limit fixture each produce the approved stable error without exposing the upstream body or credential.
