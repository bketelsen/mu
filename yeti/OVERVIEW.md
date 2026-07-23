# Mu — Architecture Overview

## Purpose

Mu is a single-owner, self-hostable personal home server: one Go binary
(`module mu`) that provides news, mail, weather, video, search, a
microblog, small hosted apps, and an AI agent, all behind one owner
identity. Every surface — web UI, CLI, REST, MCP, A2A, and chat
channels (Discord/Telegram/WhatsApp) — resolves to the same single
owner account; there is no multi-tenant user model.

Built on [go-micro](https://github.com/micro/go-micro) v6: each domain
capability (news, mail, weather, blog, video, search, apps, github,
recall) is registered as an in-process go-micro RPC service, and the
conversational assistant is itself a go-micro agent. This lets Mu run
as one process today while keeping a path to splitting services out
later.

## Architecture

```
main.go            single entrypoint: `mu --serve` runs the server,
                    any other invocation is a CLI command (internal/cli)
                    that talks to /mcp over HTTP.

internal/service    go-micro runtime spine — in-memory registry/broker/
                    transport/client, file-backed store. service.Init()
                    boots it; service.Register() exposes a struct's
                    methods as an RPC service; service.Call() invokes one.

internal/api        canonical tool registry (api.Tool) shared by MCP,
                    the agent planner, the API docs page, and the CLI.
                    RegisterTool / RegisterToolWithAuth in main.go wire
                    domain services in as agent-callable tools.

agent/              main conversational agent: agent.Query() — routes to
                    a micro-agent, a native go-micro agent, or a hand-
                    rolled plan→execute→synthesize fallback.
agent/micro/        specialized agent registry + keyword/LLM router +
                    executor + multi-agent orchestrator.
internal/ai/        LLM abstraction over Anthropic Claude, GitHub
                    Copilot, Atlas Cloud (DeepSeek/Qwen), and local/
                    Ollama models — all dispatched through go-micro.

news/ mail/ blog/   domain services. Each has Load() (registers itself,
video/ weather/     starts background loops) plus an HTTP Handler and
search/ apps/       usually a go-micro Server type exposing RPC methods
github/ chat/       (Server.Recent, Server.Search, Server.Forecast, …).
stream/ topics/
recall/ home/

client/discord/     channel bots — DM-only, owner-linked, never
client/telegram/    provision accounts. Fan mail/agent traffic in and
client/whatsapp/    out through agent.Query and mail.OnNewMail.

internal/auth       single-owner accounts, sessions, passkeys, PATs,
                    OAuth 2.1 (for MCP clients), Google sign-in.
internal/settings   live-reloadable config: env var wins over the
                    persisted ~/.mu/data/settings.json value.
internal/data       file store under ~/.mu/data, JSON load/save,
                    shared search index, timestamped backups.
internal/userdb     owner-scoped JSON "collections" (mu.db / db_* tools,
                    apps/<slug> namespaces, images).
internal/memory     small per-owner persistent memory (facts/prefs).
internal/setup      first-run wizard that creates the sole owner.
internal/a2a        Agent2Agent protocol (agent discovery + tasks).
internal/migration  idempotent, versioned startup data migrations.

admin/              owner-only ops UI: settings, logs, usage, mail
                    spam/blocklist, topics, diagnostics, console.
docs/               embedded Markdown docs served at /docs (this is
                    the *product* docs/, not yeti/ — see below).
```

See dedicated docs for depth:
- [agent-system.md](agent-system.md) — agent/, agent/micro/, internal/ai, native go-micro agent path
- [tool-registry-and-mcp.md](tool-registry-and-mcp.md) — internal/api, MCP, A2A, how tools are registered/invoked
- [domain-services.md](domain-services.md) — news/mail/blog/video/weather/search/apps/github/chat/stream/topics/recall/home/admin
- [auth-and-identity.md](auth-and-identity.md) — internal/auth, internal/setup, channel linking model
- [data-and-storage.md](data-and-storage.md) — internal/data, internal/userdb, internal/memory, internal/snapshot, internal/event
- [deployment.md](deployment.md) — Docker, install.sh, systemd socket activation, CI/CD

## Key Patterns

- **Single owner, no multi-tenancy.** First-run `/setup` creates the
  only account (`internal/setup`); afterward every surface (web,
  CLI, REST, MCP, A2A, Discord/Telegram/WhatsApp) authenticates back
  to that one owner. Client channel integrations link an external
  identity to the owner and must never provision new accounts —
  enforced with `auth.IsOwner` checks in every channel's link flow.
  See `docs/SECURITY.md`: no LLM, client argument, or channel message
  ever chooses an account.

- **go-micro as the internal service bus.** `internal/service.Init()`
  brings up an in-memory registry/broker/transport before any domain
  package loads (`main.go`); `service.Register("news", &Server{})`
  exposes RPC methods; `service.Call(ctx, "news", "Server.Headlines",
  req, &rsp)` invokes them. This indirection is what lets `agent/`'s
  *native* mode discover and call domain capabilities as go-micro
  services rather than as ad hoc Go function calls, and is the seam
  along which services could later move out-of-process.

- **One tool registry, three front doors.** `internal/api` holds every
  `api.Tool` (name, params, optional `Handle`/`HandleAuth`, or a
  `Method`+`Path` to dispatch internally over HTTP). MCP, the agent
  planner (`agent/`, `agent/micro/`), the `/api` docs page, and the CLI
  all read from this same registry — a tool registered once is usable
  everywhere. `RegisterToolWithAuth` binds `accountID` server-side from
  the session; tool handlers must never trust a client- or model-
  supplied identity.

- **Three ways a query gets answered.** `agent.Query()` tries, in
  order: (1) micro-agent routing (`agent/micro`, keyword then LLM
  classification) for narrow specialist tasks; (2) the *native*
  go-micro agent (framework-native tool calling against registered
  services) when `AGENT_NATIVE` is enabled and a provider supports it;
  (3) a hand-rolled plan → execute → synthesize fallback using the
  `internal/api` tool registry. See `agent-system.md` for the full
  decision tree.

- **Settings: env wins, else persisted.** `internal/settings.Get(key)`
  returns the environment variable if set, otherwise the value in
  `~/.mu/data/settings.json` (editable at `/admin/env`). There's no
  file watcher — "live reload" means `Set` updates the in-memory value
  and file immediately, not that files are polled.

- **Background loops start in `Load()` or `main.go`.** Nearly every
  domain package's `Load()` both registers its go-micro service and
  kicks off any background goroutines (feed refresh, sentiment
  scoring, digest scheduling, image generation retries). `main.go`'s
  `main()` is a long, explicit sequence of `X.Load()` calls plus a
  handful of cross-package callback wirings (e.g. `mail.OnNewMail`,
  `digest.PublishBlogPost`, `stream.AIReplyHook`) — read it top-to-
  bottom to see full startup order and every integration seam.

- **Owner-scoped vs. public data.** `internal/data` indexes public
  content (news, blog, video) that anyone/any agent can search;
  `internal/userdb` and `internal/memory` are owner-scoped. `recall`
  (the cross-source search tool) explicitly keeps mail bodies out of
  the shared public index and instead does a live, owner-scoped mail
  search — see `data-and-storage.md`.

- **Untrusted content is untrusted.** Fetched web pages, incoming
  mail, and model output are never used to select an account or
  bypass auth. `internal/safefetch` blocks SSRF (private/link-local/
  multicast destinations, redirect revalidation) for any
  user/app-supplied URL fetch (apps, web_fetch tool, etc).

- **Idempotent, versioned migrations run before services load.**
  `internal/migration` and ad hoc migration functions in `main.go`
  (`migrateWalletPayments`, `migrateSingleOwner`, `migrateRemoveSocial`,
  `migration.RemovePlaces`) write a version marker file and re-check it
  on every startup, aborting startup (`os.Exit(1)`) on failure. Backup
  behavior is inconsistent between them: `migrateRemoveSocial` backs up
  data first; `migrateWalletPayments` and `migration.RemovePlaces` do
  not (`main.go:200-229`).

- **CLI and server share one binary, split at the first flag.**
  `mu --serve` (or `-serve`) runs the full server; any other
  invocation is handed to `internal/cli`, which never touches server
  state directly — it authenticates and calls `/mcp` over HTTP just
  like any other MCP client. Edge case: `--serve=false` is still
  detected as server mode (`isServerMode`, `main.go:1374-1385`) but
  then exits immediately without starting the server or falling back
  to the CLI — see `deployment.md`.

## Configuration

Environment variables always take priority over the persisted
`~/.mu/data/settings.json` value (`internal/settings`). Full reference:
`docs/ENVIRONMENT_VARIABLES.md`. Highlights:

| Variable | Purpose |
|----------|---------|
| `ADMIN` | Bootstrap value for the first owner account |
| `MU_DOMAIN` | Public domain, used for A2A base URL and email `Onion-Location`-style routing |
| `PUBLIC_URL` | Overrides the externally-visible base URL when it differs from `MU_DOMAIN` |
| `MAIL_DOMAIN` | Enables outbound verification email via Mu's own SMTP relay when set to a real domain |
| `MCP_GATEWAY_ADDR` | Optional separate go-micro MCP gateway port, additive to `/mcp` |
| `AGENT_NATIVE` / `AGENT_NATIVE_STREAM` | Toggle the native go-micro agent path and its streaming |
| `COPILOT_GITHUB_TOKEN` | GitHub Copilot as LLM provider (see `internal/ai/copilot`) |
| Anthropic / Atlas Cloud / Ollama vars | Alternate/fallback LLM providers — see `internal/ai/providers.go` and `docs/ENVIRONMENT_VARIABLES.md` |
| `ANTHROPIC_MODEL` / `ANTHROPIC_PREMIUM_MODEL` / `COPILOT_CHAT_MODEL` / `COPILOT_BACKGROUND_MODEL` / `COPILOT_PREMIUM_MODEL` | Per-provider model overrides for chat vs. background vs. premium tasks |
| `IMAGE_MODEL` / `IMAGE_BASE_URL` / `IMAGE_API_KEY` / `IMAGE_SIZE` | Image-generation provider config (`internal/ai/image.go`) |
| `BRAVE_API_KEY` | Brave Search API key for `search/` |
| `YOUTUBE_API_KEY` / `GOOGLE_API_KEY` | Google/YouTube API keys for `video/` and Google sign-in |
| `DISCORD_BOT_TOKEN` | Enables the Discord channel bot |
| `TELEGRAM_BOT_TOKEN` | Enables the Telegram channel bot (long polling) |
| `WHATSAPP_TOKEN` / `WHATSAPP_PHONE_ID` / `WHATSAPP_VERIFY_TOKEN` / `WHATSAPP_APP_SECRET` | Enable the WhatsApp Business Cloud webhook |
| `MU_ENCRYPTION_KEY` / `GPG_KEYRING` / `GPG_HOME` (or `GNUPGHOME`) | Mail-at-rest encryption key and GPG keyring location (`mail/encrypt.go`) |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` | Outbound relay SMTP credentials, when not using Mu's own SMTP server |
| `MU_URL` / `MU_TOKEN` | CLI target server URL and PAT, so `internal/cli` can reach a remote `/mcp` (`internal/cli/config.go`) |
| `MU_NO_COLOR` / `NO_COLOR` | Disable ANSI color in CLI output |
| `MU_USE_SQLITE` | Switch `internal/data` indexing to a SQLite/FTS5 backend |
| `NOTES` | Set to `off` to disable Mu's own low-cadence "notes" blog loop |
| `TOR_ONION` | Advertise a Tor onion address alongside the clearnet domain |
| `LISTEN_PID` / `LISTEN_FDS` | Set by systemd socket activation; adopted automatically for zero-downtime restarts |

Note: `internal/env` implements a dotenv loader (`MU_ENV_FILE`, else
`~/.env`, else `~/.mu/.env`, without overriding already-set env vars)
but as of this writing has no import sites in the repo, so it is
currently dormant/unwired — don't rely on `.env` file loading until
something calls it.

Development commands (from `CLAUDE.md`):

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```
