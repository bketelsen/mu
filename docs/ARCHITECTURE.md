# Architecture

Mu is one private Go binary. It runs service packages in-process behind Go
Micro, with a web UI, CLI, REST API, MCP at `/mcp`, and A2A at `/a2a`.

## Owner boundary

First-run setup creates the one owner. `internal/auth` resolves every accepted
credential to that owner: password, passkey, linked Google identity, PAT, OAuth,
CLI/API bearer token, MCP, and A2A. Internal account IDs scope persisted data;
they do not model selectable local users.

All application routes and service handlers require owner access after setup.
`/setup` is the only unauthenticated first-run surface. Linked Discord, Telegram,
and WhatsApp direct messages also resolve to the owner.

## Packages

| Package | Responsibility |
|---|---|
| `internal/auth` | Owner setup, credentials, sessions, passkeys, PATs, migration |
| `internal/app` | Private HTTP middleware, rendering, and embedded assets |
| `internal/api` | Authenticated REST, MCP, and A2A tool dispatch |
| `internal/data` | Owner-scoped file persistence, indexing, and events |
| `agent/` | Owner agent planning and tool execution |
| `home/`, `chat/`, `github/`, `mail/`, `news/`, `search/`, `places/` | Private owner services |

Each building block has `Load()` and an HTTP handler registered from `main.go`.
Agent tools are registered in `internal/api/mcp.go` and `main.go`; account-scoped
tools bind the owner on the server rather than accepting an identity argument.

## Migration

Legacy migration backs up the full data directory, retains the oldest admin as
owner, or resets an admin-less instance.
