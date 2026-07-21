# Mu

A personal home server. News, mail, search, weather, markets, video — the everyday internet, handled by one agent you talk to and run yourself. The big platforms own a service for everything; Mu is the alternative — one home server across all the everyday things, each a real service, and open/self-hostable so you can run the whole stack yourself. Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain (news, markets, mail, weather, blog, social, video, search, places) is a package under the top level
- **Agents** — `agent/micro/` contains specialised micro-agents per domain, routed by keyword + LLM
- **Channels** — Discord (`client/discord/`), Telegram (`client/telegram/`), WhatsApp (`client/whatsapp/`)
- **Protocols** — owner-authenticated MCP server at `/mcp`, A2A at `/a2a`, outbound x402 payments
- **AI** — `internal/ai/` supports Anthropic Claude, Atlas Cloud (DeepSeek), and local models (Ollama)
- **Config** — `internal/settings/` for live-reloadable settings, owner admin UI at `/admin/env`

## Key Packages

| Package | Purpose |
|---------|---------|
| `agent/` | Main agent pipeline (plan → execute → synthesise) |
| `agent/micro/` | Multi-agent system — registry, router, executor, orchestrator |
| `news/` | RSS feed aggregation, sentiment tagging |
| `markets/` | Crypto, futures, commodities, currencies via CoinGecko/Yahoo |
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
| `wallet/` | Credit system, Stripe, x402 |
| `search/` | Brave web search, readability reader |
| `docs/` | Embedded documentation served at /docs |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```

## Conventions

- No external dependencies for crypto (secp256k1, RLP, ECDSA implemented in pure Go in `wallet/evm.go`)
- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- First-run setup creates the only owner. Client integrations resolve only linked-owner direct messages and must never provision accounts.
- Every web, API, CLI, MCP, and A2A surface is owner-authenticated after setup. Incoming x402 payment never bypasses authentication; x402 is outbound owner spending only.
- The main branch is `main`
