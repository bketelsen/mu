# Architecture

Mu is structured as a set of **building blocks** composed on top of shared **subsystems**.

## Directory Layout

```
mu/
├── main.go                 # Wiring: Load(), route registration, middleware
├── internal/               # Subsystems (infrastructure, not features)
│   ├── ai/                 # LLM integration (Anthropic)
│   ├── api/                # MCP protocol, tool registration, execution
│   ├── app/                # HTTP utilities, rendering, logging, static assets
│   ├── auth/               # Sessions, accounts, passkeys, presence
│   ├── data/               # File persistence, indexing, event pub/sub
│   └── moderation/         # Content flagging, hiding, auto-moderation
├── admin/                  # Content moderation, flagging, admin panel
├── agent/                  # AI agent (plans + executes via MCP tools)
├── blog/                   # Posts, comments, opinions, ActivityPub federation
├── chat/                   # Real-time chat rooms with AI
├── docs/                   # Documentation pages
├── home/                   # Home screen cards (composition layer)
├── mail/                   # Email inbox, SMTP server, DKIM, spam filtering
├── news/                   # RSS feed aggregation
│   └── digest/             # Daily news digest (composition layer)
├── places/                 # Map and location search
├── search/                 # Local index search + Brave web search
├── social/                 # Social media feed aggregation (X, Truth Social)
├── user/                   # User profiles, presence tracking
├── video/                  # YouTube channel aggregation
├── wallet/                 # Credit system, Stripe payments
└── weather/                # Weather forecasts
```

## Subsystems vs Building Blocks

### Subsystems (`internal/`)

Subsystems provide **infrastructure** that building blocks depend on. They live in
`internal/` to enforce at the Go compiler level that only code within this module
can import them — they are not features, they are plumbing.

| Package           | Purpose                                       | Dependencies          |
|-------------------|-----------------------------------------------|-----------------------|
| `internal/data`   | JSON file persistence, full-text indexing, event pub/sub | (none)          |
| `internal/auth`   | Account CRUD, sessions, tokens, passkeys      | `data`                |
| `internal/app`    | HTTP response helpers, HTML rendering, logging | `auth`, `data`        |
| `internal/ai`     | LLM provider abstraction (Anthropic API)      | `app`                 |
| `internal/api`    | MCP server, tool registry, tool execution     | `app`                 |
| `internal/moderation` | Content flagging, hiding, auto-moderation | `data`                |

**Layering rule:** Subsystems may only import other subsystems (and only downward:
`data` ← `auth` ← `app` ← `ai`, `api`). Subsystems must **never** import building blocks.

### Building Blocks (top-level packages)

Building blocks are **features**. Each building block:

1. Has a `Load()` function called from `main.go` at startup
2. Has a `Handler(w, r)` function registered as an HTTP route
3. Imports only subsystems (`internal/*`) and the `wallet` package for quota
4. Does **not** import other building blocks (with documented exceptions below)

| Package     | Route(s)                 | Subsystems Used                     |
|-------------|--------------------------|-------------------------------------|
| `admin`     | `/admin`, `/flag`        | `app`, `auth`                       |
| `agent`     | `/agent`                 | `ai`, `api`, `app`, `auth`, `data`  |
| `blog`      | `/blog`, `/post`         | `ai`, `app`, `auth`, `data`, `moderation` |
| `chat`      | `/chat`                  | `ai`, `app`, `auth`, `data`, `moderation` |
| `docs`      | `/docs`, `/about`        | `app`                               |
| `mail`      | `/mail`                  | `app`, `auth`, `data`               |
| `news`      | `/news`                  | `app`, `auth`, `data`               |
| `places`    | `/places`                | `app`, `auth`, `data`               |
| `search`    | `/search`, `/web`        | `ai`, `app`, `auth`, `data`         |
| `social`    | `/social`                | `app`, `auth`, `data`               |
| `user`      | `/@{username}`           | `app`, `auth`, `data`               |
| `video`     | `/video`                 | `app`, `auth`, `data`               |
| `wallet`    | `/wallet`                | `app`, `auth`, `data`               |
| `weather`   | `/weather`               | `app`, `auth`                       |

Most building blocks also import `wallet` for quota checking on metered operations.

### Composition Layers

Some packages act as **composition layers** that aggregate content from multiple
building blocks to render combined views:

- **`home/`** — renders home screen cards by importing `blog`, `news`, `social`,
  `video`, `agent`. This is intentional: home is a
  read-only aggregation view.

- **`news/digest/`** — generates a daily news digest by pulling from `news` and
  `video`. This is a scheduled background job that stores its own
  `digests.json` — it is a news summary, not a blog post.

- **`blog/opinion.go`** — generates a daily opinion piece using `news`, `search`,
  `video` as context. The opinion is published as a blog
  post. The editorial memory system (`opinion_memory.go`) tracks stances,
  directives, and topic history so the agent evolves its perspective over time.

- **`news` ← `social` (via callback)** — `main.go` wires `news.FetchSocialContext`
  to `social.FetchContext` so news articles that reference social posts can show
  the original post inline. No direct import — uses a function callback.

These cross-building-block imports are documented exceptions. The long-term goal
is to replace them with the event system (`data.Subscribe`/`data.Publish`).

## Key Patterns

### Initialization

Every building block defines `Load()` (even if it's a no-op). `main.go` calls
them in dependency order:

```go
data.Load()       // Index system first
admin.Load()      // Moderation flags
chat.Load()       // Chat topics
news.Load()       // RSS feeds
video.Load()      // YouTube channels
blog.Load()       // Blog posts + comments
mail.Load()       // SMTP + DKIM
places.Load()     // (no-op)
weather.Load()    // (no-op)
wallet.Load()     // Credit balances
apps.Load()       // User apps
social.Load()     // Social feeds
home.Load()       // Home screen cards
agent.Load()      // (no-op)
digest.Load()     // Digest scheduler
user.Load()       // Presence tracking
search.Load()     // (no-op)
docs.Load()       // (no-op)
```

### Handler Dispatch

All handlers follow `func Handler(w http.ResponseWriter, r *http.Request)` and
are registered in `main.go` via `http.HandleFunc`. Handlers use:

- `auth.TrySession(r)` for optional auth (public pages with auth features)
- `auth.RequireSession(r)` for required auth (write operations)
- `app.WantsJSON(r)` / `app.RespondJSON()` for JSON API responses
- `wallet.CheckQuota()` / `wallet.ConsumeQuota()` for metered operations

### Data Storage

Building blocks persist state via `data.LoadFile()` / `data.SaveFile()` using
JSON files. Each block owns its own files (e.g., `blog.json`).

Searchable content is indexed via `data.Index(id, type, title, content, meta)`.

### MCP Tool Registration

Tools are registered in `main.go` and `internal/api/mcp.go` via `api.RegisterTool()`.
The agent executes tools through `api.ExecuteTool()` which creates internal HTTP
requests — it does **not** import building blocks directly.

### Event System

`internal/data` provides `Subscribe(event, callback)` and `Publish(event, payload)`.
Currently used by `blog` for auto-tagging workflows. Available for future use to
decouple composition layers from direct imports.

### Wallet Quota

Metered operations (search, AI, web fetch) check credits before executing:

```go
canProceed, _, cost, _ := wallet.CheckQuota(accountID, wallet.OpSomeAction)
if !canProceed { /* deny */ }
// ... do work ...
wallet.ConsumeQuota(accountID, wallet.OpSomeAction)
```

## Dependency Rules

1. **Subsystems never import building blocks** — enforced by `internal/`
2. **Building blocks import subsystems freely** — that's what they're for
3. **Building blocks should not import each other** — except documented composition layers
4. **`wallet` is the one cross-cutting building block** — most blocks import it for quota
5. **`admin` imports `mail`** — for spam filter and blocklist management in the
   admin panel. This is an acceptable coupling since admin is a management UI
