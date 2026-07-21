# GitHub Service Design

## Summary

Add a read-only GitHub.com service to Mu for browsing repositories and inspecting or searching issues and pull requests, including their comments. The service uses one instance-level `GITHUB_TOKEN`, is restricted to Mu administrators, and is available through a dedicated web page, go-micro RPC, the existing MCP/CLI surface, and the agent.

This design intentionally delivers one coherent slice of a larger GitHub integration. Mutations, notifications, webhooks, code browsing, GitHub Enterprise, per-user OAuth, and Mu's proposed single-user migration remain separate work.

## Goals

- Let an administrator browse every repository visible to the configured token.
- Let an administrator list, filter, search, and inspect issues and pull requests.
- Include issue and pull-request discussion comments in detail views and agent responses.
- Expose the same capabilities through web, go-micro RPC, MCP/CLI, and chat.
- Keep credentials and private repository data secure.
- Follow Mu's existing top-level service and server-rendered UI patterns.

## Non-Goals

- Creating, editing, commenting on, closing, reopening, reviewing, merging, or otherwise mutating GitHub resources.
- Notifications, review-request inboxes, webhooks, or background synchronization.
- Browsing repository file trees, file contents, commits, releases, or workflow runs.
- GitHub Enterprise Server or a configurable GitHub API host.
- Per-user OAuth or per-account GitHub identities.
- Persisting GitHub API responses in Mu's store or search index.
- Converting Mu from multi-account to single-user. That migration affects authentication, ownership, mail, memory, channels, and stored-data contracts and needs a separate design.

## Chosen Approach

Implement a small REST client in the new `github/` package using `net/http`. Define only the endpoints and response fields required by this service.

This approach is preferred over adding `go-github` because the MVP needs only a small endpoint set, while a direct client adds no dependency and keeps pagination, errors, and model-ready formatting under Mu's control. It is preferred over GraphQL because the approved repository workspace does not need deeply nested dashboard queries, and REST has simpler fixtures and failure semantics for this scope.

## Architecture

The top-level `github/` package owns the integration and contains focused units:

- `client.go` performs authenticated GitHub.com REST requests, decodes responses, captures pagination metadata, translates upstream failures, and supports an injected HTTP client and API base for tests.
- `types.go` defines minimal internal models for repositories, issues, pull requests, comments, users, labels, and pagination.
- `service.go` defines the go-micro request/response contracts and `Server` methods. It turns structured GitHub data into model-ready text where appropriate.
- `github.go` initializes the package, registers the go-micro service, and registers web routes.
- Handler and rendering files implement admin checks and the server-rendered `/github` workspace without duplicating API or formatting logic.

`github.Load()` is called from `main.go` after settings and app initialization. It registers the service as `github`. The package does not start background goroutines.

The legacy `/mcp` registry receives four admin-checked tools:

- `github_repositories`: list or search repositories visible to the token.
- `github_repository`: inspect one repository and optionally list its issues or pull requests.
- `github_search`: search issues and pull requests, either globally or within a repository.
- `github_issue`: inspect one issue or pull request with its comments.

The existing `agent/micro` registry receives a GitHub specialist with those tools, plus routing terms for repositories, GitHub issues, and pull requests. This maintains the current fallback path while the native go-micro agent remains the long-term direction.

## Trust Boundary And Access Control

The service uses one instance-level token, so all user-facing access is administrator-only.

- Every `/github` route calls `auth.RequireAdmin` before fetching GitHub data.
- Every legacy MCP/tool handler resolves the authenticated account and verifies `Account.Admin` before invoking the GitHub service.
- The internal go-micro RPC service is a trusted in-process boundary, matching existing Mu services.
- The optional raw go-micro MCP gateway auto-exposes registered RPC methods and is therefore operator-trusted. It must not be exposed to untrusted networks without gateway-level authentication. This design does not attempt to retrofit gateway authentication.

Moving Mu to single-user can later simplify these checks, but that migration is not a dependency of this service.

## Configuration

Add `GITHUB_TOKEN` to the settings and `/admin/env` allowlist. Resolve it through `internal/settings` for every request so an administrator can add or rotate it without restarting Mu.

The GitHub token is separate from `COPILOT_GITHUB_TOKEN`. The Copilot token is never reused because its scopes, lifecycle, and purpose differ.

The setup state recommends a fine-grained token with read-only access to repository metadata, issues, pull requests, and only the repositories the administrator wants Mu to expose. No write permission is required. The token is never logged, returned through a tool, embedded in HTML, or persisted by the GitHub package.

## Service Contract

The service exposes methods that correspond to user tasks rather than raw GitHub endpoints:

- List or search repositories, ordered by recent activity by default.
- Read repository metadata.
- List issues for a repository with open/closed filtering and pagination.
- List pull requests for a repository with open/closed filtering and pagination.
- Search issues and pull requests using a text query, optional repository scope, resource type, state, and pagination.
- Read one issue or pull request, including chronological discussion comments and pull-request-specific metadata when applicable.

Request contracts accept bounded page and page-size values. Repository coordinates are explicit `owner` and `repo` fields. Item numbers are positive integers. Search text and other string inputs are trimmed and length-limited before an upstream request.

Responses contain structured records and pagination metadata for web consumers. Agent-facing accessors also return concise text containing repository coordinates, item numbers, status, labels, authors, timestamps, GitHub URLs, bodies, and comments as applicable.

## GitHub API Mapping

The client uses GitHub's versioned REST API at `https://api.github.com`:

- `GET /user/repos` lists repositories visible to the token, including owned, collaborating, and organization repositories, sorted by recent updates.
- `GET /search/repositories` searches repositories.
- `GET /repos/{owner}/{repo}` reads repository metadata.
- `GET /repos/{owner}/{repo}/issues` lists issues.
- `GET /repos/{owner}/{repo}/pulls` lists pull requests.
- `GET /search/issues` searches issues and pull requests using GitHub qualifiers.
- `GET /repos/{owner}/{repo}/issues/{number}` reads common issue or pull-request conversation data.
- `GET /repos/{owner}/{repo}/issues/{number}/comments` reads chronological discussion comments.
- `GET /repos/{owner}/{repo}/pulls/{number}` adds pull-request branches, merge state, and other pull-specific metadata when the item is a pull request.

The client sends `Authorization: Bearer`, GitHub's current stable API version header, `Accept: application/vnd.github+json`, and a fixed Mu user agent. It parses GitHub's `Link` response header into page metadata rather than exposing header parsing to handlers.

Pull-request review comments and reviews are not part of v1. The detail contract includes the issue-style discussion attached to the pull request and pull-request metadata. Review-specific conversations can be added as a separate capability if needed.

## Web Experience

`/github` is a server-rendered repository workspace.

On desktop, a persistent left rail lists repositories visible to the token. The main pane shows the selected repository, repository metadata, and Issues/Pull requests tabs. Each tab supports state filtering, pagination, and repository-scoped search. Selecting an item opens its title, status, author, labels, timestamps, body, GitHub URL, and chronological comments while preserving the repository context.

On narrow screens, the repository rail becomes a repository selector above one content pane. The page uses Mu's existing spacing, borders, cards, list-item hover states, typography, and navigation patterns. It introduces no frontend framework and uses links and GET forms so views remain bookmarkable and functional without JavaScript.

The page query state identifies the selected `owner/repo`, tab, state, search text, page, and selected item number. Handlers validate all query values before passing them to the service.

When `GITHUB_TOKEN` is missing, the page renders a setup state with a link to `/admin/env` instead of making an upstream request.

## Data Flow

1. An administrator requests `/github` or invokes an authenticated GitHub tool.
2. The entry point verifies the Mu account is an administrator.
3. The service resolves the current `GITHUB_TOKEN` from settings.
4. The REST client validates and encodes request parameters, then calls GitHub with a bounded context and response size.
5. The client decodes GitHub data into minimal internal types and returns pagination or a translated error.
6. The service prepares structured and model-ready results.
7. The web handler renders escaped data, or the tool returns concise text to MCP/CLI/chat.

GitHub responses are not persisted. Calls are live query-plane operations. The MVP does not add an in-memory cache or conditional requests; those should be introduced only if observed usage or rate limits justify the added state and invalidation behavior.

## Security

- Verify administrator access at every web and legacy tool entry point.
- Never accept a token from request parameters or model-generated tool arguments.
- Never log authorization headers, tokens, or unbounded GitHub response bodies.
- Validate and bound owner, repository, item number, page, page size, state, resource type, and search query inputs.
- URL-encode path and query components instead of interpolating untrusted raw values.
- Limit response-body reads before JSON decoding.
- HTML-escape all GitHub-provided names, descriptions, issue bodies, comments, labels, and usernames.
- Preserve body/comment whitespace in safe text rendering; do not execute or directly render GitHub-flavored HTML or embedded markup in v1.
- Keep private repository names and errors out of unauthenticated and non-admin responses.

## Error Handling

The client maps upstream failures into stable service errors:

- No token: `GITHUB_TOKEN is not configured`; the web page shows the setup state.
- `401 Unauthorized`: report that the token is invalid or expired without including the upstream body.
- Rate-limit `403`: report that the GitHub rate limit was reached and include the parsed reset time when available.
- Other `403 Forbidden`: report insufficient access without disclosing unnecessary upstream details.
- `404 Not Found`: report that the repository or item was not found or is not accessible, avoiding private-resource disclosure.
- `429 Too Many Requests`: treat as rate limiting and respect reset or retry metadata when present.
- Timeouts, cancellation, `5xx`, malformed JSON, and oversized bodies: return bounded upstream errors while leaving the rest of Mu operational.

The HTTP client uses a fixed timeout and request contexts. Errors preserve enough categorization for handlers and tools to produce actionable messages without exposing secrets.

## Testing

Tests use `httptest.Server`; no automated test contacts GitHub.com or requires a real token.

### Client Tests

- Request paths, query encoding, authorization and version headers, user agent, and absence of token leakage.
- Repository, issue, pull-request, and comment decoding.
- `Link` pagination parsing and bounded page sizes.
- Context cancellation, timeout behavior, and response-size limits.
- Translation of missing token, `401`, rate-limited and non-rate-limited `403`, `404`, `429`, `5xx`, malformed JSON, and oversized responses.

### Service Tests

- Repository list and detail summaries.
- Separate issue and pull-request listings.
- Search qualifiers for scope, type, and state.
- Issue versus pull-request classification.
- Pull-request metadata and chronological discussion comments.
- Empty states, input validation, defaults, and model-ready text limits.

### Handler Tests

- Unauthenticated and non-admin denial before any GitHub request.
- Missing-token setup state and `/admin/env` link.
- Selected repository, tab, state, query, page, and item behavior.
- HTML escaping of malicious repository, body, and comment content.
- Pagination and external GitHub links.
- Desktop workspace and mobile stacking hooks.

### Integration Tests

- The four legacy tools map arguments to the correct service methods and reject non-admin accounts.
- The GitHub specialist contains only the approved tools.
- Direct addressing and GitHub/repository/issue/pull-request prompts route to the specialist without breaking existing routes.
- Service registration and representative in-process RPC calls succeed.

Targeted verification runs `go test ./github ./agent/micro ./internal/api`. Final verification runs `go test ./... -short`, `go vet ./...`, and `go build ./...`.

## Acceptance Criteria

- An administrator can configure or rotate `GITHUB_TOKEN` through settings without restarting Mu.
- An administrator can browse all repositories visible to the token in the approved repository-workspace page.
- An administrator can list, filter, search, and inspect issues and pull requests, including chronological discussion comments.
- Equivalent read-only questions work through MCP, CLI, and chat using the same service behavior.
- Non-admin accounts cannot retrieve GitHub data through the dedicated page or legacy authenticated tools.
- Missing, invalid, expired, rate-limited, and insufficient-scope tokens produce actionable errors without leaking credentials or private upstream details.
- GitHub-provided content cannot inject executable HTML into the Mu page.
- The package adds no new third-party dependency and persists no GitHub response data.
