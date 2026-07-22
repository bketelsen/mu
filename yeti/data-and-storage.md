# Data and Storage

Covers `internal/data`, `internal/userdb`, `internal/memory`,
`internal/snapshot`, `internal/event`, `internal/settings`, and
`internal/safefetch`.

## `internal/data`: file store, index, backups

Root: `~/.mu/data` (`data.Dir()`, `internal/data/data.go:75`). Paths
are confined to this directory to prevent traversal (`:54-72`).

- `SaveFile`/`LoadFile`, `SaveJSON`/`LoadJSON` (`:75-136`) — generic
  JSON persistence used by nearly every domain package for its own
  files (`feed.json`, `blog.json`, `chat_summaries.json`, `topics.json`,
  etc.)
- Delete registration (`:142-179`) — packages register a deleter for
  their own file/content types so admin's "delete any content type"
  and account-deletion cleanup can work generically.
- Shared search index: `Index`, `IndexOwned`, `Search`, `GetByID`,
  `GetByType` (`:225-462`). Owner-scoped entries (`IndexOwned`) are
  private by default and only returned `WithOwner`; public entries have
  an empty owner. This is the *public* content index (news, blog,
  video) — see `recall.md`-equivalent notes in `domain-services.md` for
  why mail is deliberately excluded.
- Indexing is queued with 4 workers, debounced JSON persistence, and
  index-completion events (`:195-206`, `:323-339`); optional SQLite/
  FTS5 backend via `MU_USE_SQLITE=1` (`sqlite.go:26-85`).
- `data.StartIndexing()` is called explicitly late in `main.go` (after
  all `Load()` calls) so the priority queue processes newly loaded
  content first.
- Backups: `Backup` (`internal/data/backup.go:19`) does an atomic,
  timestamped, fsync+rename sibling-directory copy. Called before
  every startup migration (`main.go`'s `backupData`/
  `backupRemoveSocialData`) and available to admin operators.

## `internal/userdb`: owner-scoped collections

Backs `mu.db` (the app SDK's storage), the `db_set`/`db_get`/
`db_list`/`db_delete` MCP tools (namespaced `"api"`), per-app storage
(namespaced `apps/<slug>`), and generated images
(`images/generated`).

- `Record` (`internal/userdb/userdb.go:54-63`).
- `Create`, `Get`, `List`, `Update`, `Delete` (`:76-194`).
- Records are owner-bound server-side (the caller becomes the owner);
  can optionally be marked `public`. `List` supports scopes `mine`
  (default), `public`, or `all`; unauthenticated/guest callers are
  forced to `public` scope.
- Limits: 2,000 records per owner/collection, 64 KiB per record, list
  results capped at 200. Filtering (`where`) uses a small set of
  constrained operators evaluated in Go, not a query language
  (`:255-345`).

## `internal/memory`: persistent agent memory

Small per-owner fact/preference store in `memory.json`:

- `Entry` (`internal/memory/memory.go:14`); `Set`, `Get`, `All`,
  `ForContext`, `ForScopedContext`, `Delete`, `Clear` (`:38-175`).
- Max 50 entries per user. Scoped keys use `scope:key`;
  `ForScopedContext` returns global entries plus the matching scope —
  this is how `agent/micro` specialists get their own memory slice via
  `Agent.MemoryScope` while still seeing global facts.
- `memory.Clear` is registered as an account-deletion cleanup hook in
  `main.go:103`.
- `UserContextFunc` (wired in `main.go:297-314`) surfaces
  `ForContext` output (plus unread mail count) into every agent query
  as private context — this is how the assistant "remembers" things
  the owner told it to.

## `internal/snapshot`: cached card read plane

A durable go-micro store plus a broker-fed in-memory mirror, used for
fast home/agent-answer card rendering without re-querying the
originating service on every request: `New`, `Publish`, `Get`
(`internal/snapshot/snapshot.go:40-94`). News, video, and blog all
publish their "preview"/"headlines" snapshot after refreshing so
`home` cards render instantly from cache.

## `internal/event`: internal pub/sub

Thin channel-style facade over the shared go-micro broker: `Subscribe`,
`Publish`, `Subscription.Close` (`internal/event/event.go:46-97`). Used
for cross-package async signals that shouldn't require a direct import
— e.g. `blog_updated`/`apps_updated` (consumed by `home`), summary/tag
request-and-result events between `news`/`blog` and `chat`.

## `internal/settings`: live-reloadable config

- `Load` (`internal/settings/settings.go:18`), `Get` (`:31`), `Set`
  (`:47`), `IsSet` (`:63`), `Source` (`:68`), `All` (`:81`).
- Precedence: a non-empty environment variable always wins; otherwise
  the mutex-protected in-memory map (persisted to
  `~/.mu/data/settings.json`, editable at `/admin/env`) is used.
- `Set` re-reads the JSON file before saving to avoid clobbering
  another process's concurrent update.
- "Live reload" here means `Set` takes effect immediately in-process —
  there is no filesystem watcher; nothing polls the settings file for
  external edits.

## `internal/safefetch`: SSRF protection

Used anywhere Mu fetches a URL supplied by an app, a tool argument, or
untrusted content (the `web_fetch` tool, app SDK fetches, etc.):
validates the initial destination *and* every redirect hop, blocks
private/link-local/multicast IP ranges, and caps both time and
response body size (`internal/safefetch/safefetch.go:108-219`). Any
new feature that fetches a user- or model-supplied URL should go
through this rather than a bare `http.Get`.
