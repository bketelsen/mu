# Editable Topics Final Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close final branch-review findings without changing the approved exact canonical-name Update/Delete interface.

**Architecture:** News will expose only a package-local parse synchronization seam so a test can pause after feed collection and before the locked publication step. The publication step will re-snapshot active feeds under its existing lock, prune removed state, and publish the card after unlocking. Topics validation and no-op detection remain in the store, while chat avoids per-topic AI configuration failures.

**Tech Stack:** Go standard library, existing gofeed parser, project test helpers.

## Global Constraints

- Do not change exact canonical-name lookup for `topics.Update` or `topics.Delete`.
- Do not add dependencies or broad refactors.
- Do not make real network requests in tests.
- Preserve historical indexed data when a configured news topic is deleted.

---

### Task 1: Make news publication deletion-safe and deterministic

**Files:**
- Modify: `news/news.go`
- Test: `news/news_test.go`

**Interfaces:**
- Produces package-local test seam around the interval between parsing and publication.
- Retains `parseFeedOnce()` as the parser entry point and `runFeedParse` as the refresh-loop seam.

- [ ] Add a test that starts a parse using a local blocking HTTP test server, waits for collection to complete, deletes a topic through the existing subscription path, then releases publication.
- [ ] Assert the deleted topic is absent from `feeds`, `status`, in-memory `feed`, rendered headlines and navigation; assert the indexed JSON/file fixture is unchanged.
- [ ] Add the smallest package-local hook/channel needed to pause parse publication in the test, always clearing it in cleanup.
- [ ] Move `cardSnap.Publish(headlineHTML)` after `mutex.Unlock()` while preserving the exact published HTML.
- [ ] Delete `parseFeed`, which only calls `feedRefreshLoop`, and its stale comments.
- [ ] Run `go test ./news -run 'Test.*(Delete|Prune)'` and `go test -race ./news`.

### Task 2: Avoid redundant topic persistence and validate defaults

**Files:**
- Modify: `topics/topics.go`
- Test: `topics/topics_test.go`

**Interfaces:**
- `Load() error` validates normalized defaults before calling `persist`.
- `Update(name string, replacement Topic) (Change, error)` returns an empty `Change` for an identical normalized replacement without persistence or notification.

- [ ] Add a test replacing a topic with its unchanged normalized values; assert empty change, no persist call, and no subscriber invocation.
- [ ] Add a test overriding package defaults with invalid input and `persist`; assert `Load` returns validation error and persistence is not attempted.
- [ ] Validate defaults after normalization and before first-run persistence.
- [ ] Compare the existing record and normalized replacement before writing or collecting subscribers; return `Change{}` immediately for equality.
- [ ] Run `go test ./topics` and `go test -race ./topics`.

### Task 3: Restore quiet unconfigured chat batches and remove dead state

**Files:**
- Modify: `chat/chat.go`
- Test: `chat/chat_test.go`

**Interfaces:**
- Summary batch processing checks `ai.Configured()` before per-topic generation.
- `applyTopicSnapshot` stores topic names and prompts only.

- [ ] Locate batch queue processing and add a single configuration guard before its per-topic loop, logging one skipped-batch message and retaining correct queue/persistence behavior.
- [ ] Add or update a test proving unconfigured multi-topic batches do not call the per-topic generator.
- [ ] Remove the unused `head` field and its assignment.
- [ ] Run `go test ./chat` and `go test -race ./chat`.

### Task 4: Correct stale documentation and verify branch

**Files:**
- Modify: `blog/notes.go`
- Create: `.superpowers/sdd/final-fix-report.md`

- [ ] Update notes comments to refer only to `notes.json`, not the deleted `chat/prompts.json`.
- [ ] Run focused package tests and races for every changed Go package.
- [ ] Run `go test ./... -short`, `go vet ./...`, and `go build ./...`.
- [ ] Write the final-fix report with changed files and every command/result.
- [ ] Commit all intended changes using a concise fix commit.
