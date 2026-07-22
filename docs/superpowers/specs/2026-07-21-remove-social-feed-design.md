# Remove Social Feed Design

## Goal

Remove Mu's local social feed and `/social` endpoints now that Mu is a private,
single-owner server. Preserve external X and Truth Social post context only as
news article enrichment.

## Architecture

Delete the top-level social feed domain: its HTTP handlers, Go Micro service,
agent tools, dashboard snapshot, persistence, search, breaking-story detector,
and integrations. Move URL detection, external-post fetching, and context
rendering into `news`, their only remaining consumer. News calls this code
directly rather than through a callback wired by `main.go`.

Removed routes use the normal authenticated `404`; there are no redirects,
tombstones, aliases, or compatibility handlers.

## Data Migration

A versioned startup migration creates a backup before destructive work, loads
the search index, removes every index entry whose type is `social`, deletes
`social.json` and `social_posts.json`, removes `social` from the owner's
`HomeCards` and `HomeCardsSeen`, and writes its completion marker last. Missing
files and absent index entries are successful. Any other failure aborts startup,
and partial runs are safe to repeat.

Both the JSON/in-memory and SQLite/FTS index backends must support synchronous
deletion by content type.

## Product Removal

Remove social from web routes, navigation, dashboard cards, updates, saved-item
routing, Apps SDK, MCP and agent tools, micro-agent routing, Discord commands,
CLI argument handling, wallet UI, assets, and current documentation. Delete the
hourly breaking-story detector rather than moving it.

The posting cost shared by status, app, and stream writes remains, but is renamed
from social terminology to `OpContentPost`, `CostContentPost`, and
`CREDIT_COST_CONTENT_POST`. Social search and reply costs are deleted. Historical
wallet transactions are not rewritten.

## Error Handling

The migration fails startup if backup, index purge, account persistence, file
deletion (other than not-exist), or marker persistence fails. News enrichment is
best-effort: unsupported URLs and fetch failures omit context without preventing
the article from rendering; generated HTML remains escaped.

## Acceptance

- No top-level `social` package remains.
- `/social` and `/social/thread` receive normal authenticated `404` responses.
- No social feed route, tool, card, command, SDK, wallet, event, or service
  surface remains.
- Old social records cannot be returned by recall or search.
- News articles retain X and Truth Social context enrichment.
- Current product and architecture documentation no longer lists social as a Mu
  service.
- `go test ./... -short`, `go build ./...`, and `go vet ./...` pass.
