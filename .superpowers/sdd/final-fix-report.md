# Editable Topics Final Fix Report

## Scope

Closed final whole-branch review findings on `feat/editable-topics` without changing exact canonical-name lookup for `topics.Update` or `topics.Delete`.

## Changes

- `news/news.go`: added a package-local pre-publication test seam; the final active-topic prune remains under the news lock; moved `cardSnap.Publish` after unlock; removed the unused `parseFeed` wrapper and stale refresh comments.
- `news/news_test.go`: added deterministic in-flight deletion coverage using a local `httptest` RSS server. It pauses after parse collection, deletes `Tech` through the topic subscriber, and verifies `feeds`, `status`, in-memory feed, headlines, and saved navigation/publication omit the deleted topic while a pre-existing indexed entry remains unchanged.
- `topics/topics.go`: validates normalized defaults before first-run persistence and returns directly for normalized no-op updates.
- `topics/topics_test.go`: verifies no-op updates do not persist or notify, and invalid defaults are not persisted.
- `chat/chat.go`: makes summary generation skip an unconfigured AI batch once while retaining the existing summary persistence pass; removed the dead `head` field.
- `chat/chat_test.go`: verifies an unconfigured multi-topic batch performs no per-topic generation; mock-provider tests explicitly declare their configured state.
- `blog/notes.go`: removed references to deleted `chat/prompts.json` from comments.
- `docs/superpowers/plans/2026-07-21-editable-topics-final-review.md`: implementation plan created before execution.

## Verification

All commands were run from `/home/bjk/projects/mu/.worktrees/editable-topics`.

| Command | Result |
| --- | --- |
| `go test ./news -run TestParsePrunesDeletedTopicBeforePublication -v` | PASS |
| `go test ./topics -run 'Test(UpdateNoOpDoesNotPersistOrNotify|LoadValidatesDefaultsBeforePersisting)' -v` | PASS |
| `go test ./chat -run TestSummaryBatchSkipsUnconfiguredAIOnce -v` | PASS |
| `go test ./news && go test -race ./news` | PASS |
| `go test ./topics && go test -race ./topics` | PASS |
| `go test ./chat && go test -race ./chat` | PASS |
| `go test ./blog && go test -race ./blog` | PASS |
| `go test ./... -short` | PASS |
| `go vet ./...` | PASS (exit 0; no output) |
| `go build ./...` | PASS (exit 0; no output) |

## Investigation Note

The initial news test assertion found the historical index entry absent because `data.Index` queues work and index workers are not automatically started in that package test process. The test now explicitly starts indexing and waits until its historical fixture is present before exercising the deletion race. This establishes the required historical-data precondition without changing production indexing behavior.
