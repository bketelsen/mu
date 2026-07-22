# Remove Social Feed Implementation Plan

> **For agentic workers:** Follow test-driven development. Do not commit; the user did not request commits.

**Goal:** Remove Mu's social feed and preserve external social-post context in news.

**Architecture:** Delete the social domain and integrations. Move context enrichment into news and run an idempotent, backed-up startup migration for persisted data and index entries.

**Tech Stack:** Go, `net/http`, JSON persistence, SQLite/FTS5, Go Micro.

## Global Constraints

- `/social` and `/social/thread` receive normal authenticated `404` responses.
- Do not add redirects, tombstones, aliases, or compatibility handlers.
- Delete `social.json` and `social_posts.json`.
- Remove the breaking-story detector.
- Keep external social-post context only as an internal news capability.
- Rename shared posting metering to `OpContentPost` and `CREDIT_COST_CONTENT_POST`.
- Preserve historical wallet transactions without rewriting old operation strings.

---

### Task 1: Move External Context Into News

Move social URL recognition, external X/Truth Social fetching, context types, and HTML rendering from `social` into private helpers in `news/social_context.go`. Move and expand tests in `news/social_context_test.go`. Remove `news.FetchSocialContext` and the callback in `main.go`; render context directly from `news/news.go`. Remove the moved code and tests from `social`. Verify with `go test ./news ./social -count=1`.

### Task 2: Add Search Index Type Deletion

Add `data.DeleteIndexType(entryType string) error` for both backends. In-memory deletion must synchronously persist `index.json`. SQLite deletion must transactionally remove matching FTS rows before base rows. Add tests proving social entries disappear, unrelated entries survive, and SQLite search no longer returns deleted rows. Verify with `go test ./internal/data -count=1`.

### Task 3: Add The Removal Migration

Add `auth.RemoveHomeCard(id string) error` and tests. Add a version-1 `remove_social_migration.json` startup migration that backs up before loading the index, loads the index on every startup, purges social index entries, deletes both social files, cleans card preferences, and writes its marker last. Missing files are success; all other failures abort startup. Test ordering, idempotence, partial failure, marker behavior, and preference cleanup. Verify with `go test . ./internal/auth ./internal/data -count=1`.

### Task 4: Remove Service, Routes, Tools, And Commands

Delete the remaining `social` package. Remove its import, load, cleanup hook, routes, update count, dynamic/static tools, tool cards, planner catalogue entries, recall wording, native service entries, micro-agent and routing, guest allowlists, CLI mapping, and Discord command. Route blog/post intents to an appropriate remaining blog agent. Update inventory and routing tests. Verify with `go test . ./agent/... ./internal/api ./internal/cli ./client/discord -count=1`.

### Task 5: Remove UI, SDK, And Content Integrations

Remove social navigation, public asset allowlisting, dashboard configuration/function/preferences/tooltip/event, content permalink/delete handling, Apps SDK methods, validation, and `social.svg`. Add or update tests proving no configured card or SDK references social. Verify with `go test ./home ./internal/app ./apps -count=1`.

### Task 6: Rename Shared Posting Metering

Rename `OpSocialPost`/`CostSocialPost` to `OpContentPost`/`CostContentPost`, backed by `CREDIT_COST_CONTENT_POST`, for status, app, and stream writes. Remove social search/reply operations, costs, UI rows, and environment variables. Do not rewrite old wallet transactions. Update tests and verify with `go test ./wallet . -count=1`.

### Task 7: Update Current Documentation

Remove social service references from `CLAUDE.md`, `docs/MIGRATION.md`, `docs/GO_MICRO_ARCHITECTURE.md`, and current environment-variable documentation. Leave historical plans/specs unchanged and retain generic references to external social media used by news. Search all non-historical files for stale product references.

### Task 8: Final Verification

Run `gofmt` on modified Go files, `go test ./... -short`, `go build ./...`, and `go vet ./...`. Confirm the top-level package is gone and no route, tool, card, SDK, command, event, snapshot, or current-doc service reference remains. Conduct a whole-branch code review and fix all critical or important findings.
