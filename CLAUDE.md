# Mu

A personal home server. News, mail, search, weather, video — the everyday internet, handled by one agent you talk to and run yourself. The big platforms own a service for everything; Mu is the alternative — one home server across all the everyday things, each a real service, and open/self-hostable so you can run the whole stack yourself. Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain (news, mail, weather, blog, video, search) is a package under the top level
- **Agents** — `agent/micro/` contains specialised micro-agents per domain, routed by keyword + LLM
- **Channels** — Discord (`client/discord/`), Telegram (`client/telegram/`), WhatsApp (`client/whatsapp/`)
- **Protocols** — owner-authenticated MCP server at `/mcp` and A2A at `/a2a`
- **AI** — `internal/ai/` supports Anthropic Claude, Atlas Cloud (DeepSeek), and local models (Ollama)
- **Config** — `internal/settings/` for live-reloadable settings, owner admin UI at `/admin/env`

## Key Packages

| Package | Purpose |
|---------|---------|
| `agent/` | Main agent pipeline (plan → execute → synthesise) |
| `agent/micro/` | Multi-agent system — registry, router, executor, orchestrator |
| `news/` | RSS feed aggregation, sentiment tagging |
| `mail/` | SMTP server, DKIM, inbound filtering |
| `blog/` | Microblogging with AI-generated daily digests |
| `internal/ai/` | LLM abstraction — Anthropic, Atlas Cloud, local models |
| `internal/api/` | MCP server, tool registry |
| `internal/app/` | Web UI framework, templates, middleware |
| `internal/auth/` | Single-owner setup, sessions, passkeys, and PATs |
| `internal/memory/` | Owner persistent memory with scoped namespaces |
| `internal/settings/` | Live-reloadable configuration |
| `home/` | Landing page, assistant, home dashboard, summary |
| `client/discord/` | Discord bot with slash commands, embeds, briefings |
| `client/telegram/` | Telegram bot with commands and groups |
| `client/whatsapp/` | WhatsApp Business API integration |
| `search/` | Brave web search, readability reader |
| `docs/` | Embedded documentation served at /docs |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```

## Conventions

- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- First-run setup creates the only owner. Client integrations resolve only linked-owner direct messages and must never provision accounts.
- Every web, API, CLI, MCP, and A2A surface is owner-authenticated after setup.
- The main branch is `main`

## Documentation

**update documentation** After any change to source code, update
relevant documentation in CLAUDE.md, README.md and the `yeti/` folder.
A task is not complete without reviewing and updating relevant
documentation.

**yeti/ directory** The `yeti/` directory contains documentation
written for AI consumption and context enhancement, not primarily for
humans. Jobs like `doc-maintainer` and `issue-worker` instruct the AI
to read `yeti/OVERVIEW.md` and related files for codebase context
before performing tasks. Write content in this directory to be
maximally useful to an AI agent understanding the codebase — detailed
architecture, patterns, and decision rationale rather than user-facing
guides.
