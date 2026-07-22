# Editable Topics Design

## Purpose

Replace the duplicated, compile-time topic configuration in `news/feeds.json` and `chat/prompts.json` with one owner-editable source of truth. Each active topic has a stable name, one RSS feed URL, and one chat summary prompt. Changes made in the admin UI take effect without restarting Mu.

## Scope

This change covers only the topic configuration shared by news and chat. It does not consolidate the other embedded JSON files, migrate historical content when a topic is removed, or add topic renaming.

## Data Model

Add a top-level `topics` package that owns an ordered collection of records:

```go
type Topic struct {
	Name    string `json:"name"`
	FeedURL string `json:"feed_url"`
	Prompt  string `json:"prompt"`
}
```

The collection is stored at `~/.mu/data/topics.json` as a JSON array. Consumers sort snapshots by topic name for display, preserving the current deterministic presentation; array order has no user-visible meaning.

Every field is required. Topic names are stable identifiers and must be unique case-insensitively. A topic name cannot be changed after creation. Feed URLs must be absolute HTTP or HTTPS URLs. Names must be nonempty and safe for their existing uses in HTML fragments and chat room identifiers. Prompts must contain non-whitespace text.

The initial implementation should preserve the current topic-name character set rather than invent broad identifier support. Accept letters, digits, spaces, `_`, and `-`, require the first character to be a letter or digit, and trim surrounding whitespace. This keeps names safe in current fragments and identifiers while allowing ordinary labels.

## Ownership And Persistence

The `topics` package is the only component that reads or writes topic configuration. It depends on the existing internal persistence infrastructure but does not depend on news or chat. News and chat depend on `topics`; neither service becomes authoritative for the other.

On startup, `topics.Load()` checks for `topics.json`. If the file does not exist, it writes the current seven topic records as defaults and loads them. The defaults live as Go data in the `topics` package, not as embedded JSON. Once the file exists it is authoritative, including when it contains an intentionally empty array. Defaults are never merged back during ordinary startup.

The existing `news/feeds.json` and `chat/prompts.json` files and their embedding code are removed. Existing installations receive the same seven records on their first startup after upgrading because those configurations were previously immutable.

Writes serialize under a package mutex. A write validates a complete proposed collection, writes JSON to a temporary file in the data directory, sets the intended file permissions, and atomically renames it over `topics.json`. The in-memory collection changes only after the rename succeeds. A failed validation or write leaves both persisted and live configuration unchanged.

The package exposes copied snapshots rather than its mutable backing slice. Consumers may retain and iterate a snapshot without locking or racing with updates.

## Topic Operations

The package supports these operations:

- Create a topic with name, RSS URL, and prompt.
- Update an existing topic's RSS URL and prompt while preserving its name.
- Delete a topic by its exact canonical name.
- Return a snapshot sorted by canonical topic name.

Operations return enough change information to distinguish added and deleted topics, feed URL changes, and prompt changes. This lets consumers avoid unrelated work after a save.

Renaming is deliberately unsupported because names currently identify news categories, chat summary keys, chat rooms, tags, search queries, and URL fragments. An owner can delete the old topic and add a new one. The UI must state that this does not migrate history.

## Consumer Updates

`main` loads topics after data storage is initialized and before loading chat or news. Both services derive their initial runtime configuration from a topic snapshot rather than reading JSON assets.

After a successful admin mutation, the topics package publishes the committed snapshot and change information to registered consumers after releasing its write lock. Consumer callbacks must return quickly and schedule expensive work asynchronously; the persisted topic commit does not roll back if an RSS fetch or LLM request later fails.

### News

News derives its feed-name-to-URL mapping from the snapshot. Added or deleted topics and feed URL changes update this mapping immediately and request one refresh. Prompt-only changes do not refresh news. The refresh mechanism coalesces concurrent requests and the hourly timer so only one feed parse runs at a time.

Added topics and feed URL changes are visible to the next refresh. Deleted topics are excluded from future fetches and navigation. Existing fetched posts and cached historical data retain their original categories. Status entries for deleted topics are pruned from the active status view.

### Chat

Chat derives its ordered topic list, prompt lookup, and topic navigation from the snapshot. Configuration reads use copied immutable data or a dedicated lock; the existing package globals must not be replaced while readers iterate them.

Added topics and changed prompts schedule summary generation only for the affected topics. Feed-only changes do not regenerate summaries. Deleted topics are removed from current navigation and current summary output, and their summary entries are removed from the persisted active summary map. Existing chat room files, historical messages, and historical tags remain untouched.

If summary generation fails, chat logs the error and retains the last usable summary for an existing active topic. A newly added topic remains visible without a summary until generation succeeds on a later attempt or scheduled cycle.

## Admin UI

Add an owner-only `/admin/topics` page and link it from the existing admin dashboard. Follow existing admin conventions: handlers enforce `auth.RequireAdmin`, normal CSRF middleware protects mutations, pages render through the shared app framework, and successful POST requests redirect with HTTP 303.

The page displays topics sorted by name and supports:

- Adding a topic with all three required fields.
- Editing the RSS URL and prompt of an existing topic.
- Deleting a topic after explicit confirmation.
- Displaying validation and persistence errors without changing live configuration.

Names are displayed as read-only on edit forms. Delete confirmation warns that the topic disappears from current news and chat views while historical content remains stored. Reordering is not part of this change.

Admin requests wait only for validation and durable persistence. News fetching and summary generation begin immediately after commit but run in the background. The page reports that the configuration was saved, not that external refreshes completed successfully.

## Error Handling

Malformed or duplicate records prevent the entire collection from loading or being committed; partially valid collections are never accepted. At startup, an existing but invalid `topics.json` is an error and must not be silently replaced with defaults. Mu should preserve the file and surface a clear startup error so owner edits are not lost.

Runtime persistence errors are returned to the admin page. Post-commit RSS and LLM failures are logged through the existing package mechanisms and retried by their normal scheduled loops. One consumer failure does not affect topic persistence or block the other consumer.

## Testing

Package tests for `topics` cover:

- First-run default seeding and identical restart loading.
- An existing empty array remaining authoritative.
- Required fields, URL validation, name validation, and case-insensitive uniqueness.
- Stable-name enforcement on update.
- Create, update, delete, deterministic sorting, and copied snapshots.
- Failed validation and failed persistence leaving memory and disk unchanged.
- Concurrent reads and serialized updates.

News and chat tests cover:

- Initial configuration derived from topic snapshots.
- Add, update, and delete propagation.
- RSS-only changes triggering news but not chat generation.
- Prompt-only changes triggering affected chat generation.
- Prompt-only changes not triggering news refreshes.
- Coalesced news refreshes with no overlapping parse operations.
- Deleted topics hidden while historical persisted data remains.
- Summary-generation failure retaining the previous usable summary.

Admin tests cover owner authorization, CSRF-protected mutations, successful redirects, form validation errors, immutable names, and delete confirmation. The affected package tests should also run under `go test -race` to verify concurrent reads, edits, and background refreshes.

## Success Criteria

- Topic names, RSS URLs, and prompts have one persisted source of truth.
- An owner can add, edit, and delete topics from the web UI.
- Successful changes affect news and chat without restarting Mu.
- Fresh installations receive the existing seven topics exactly once.
- Existing historical news and chat data is not destructively migrated.
- Concurrent background work and admin edits do not race or launch overlapping news refreshes.
