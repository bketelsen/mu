# System Design

Mu is a private, single-owner home server. A single Go binary runs the home
dashboard, AI agent, mail, news, search, markets, weather, video, places, and
apps through in-process Go Micro services.

## Layers

1. `internal/data`, `internal/auth`, `internal/app`, `internal/ai`, and
   `internal/api` provide persistence, owner identity, rendering, models, and
   protocol dispatch.
2. Top-level service packages provide the owner-facing capabilities.
3. `agent/` composes registered tools using the authenticated owner context.

The web UI, CLI, API, MCP, and A2A endpoint use the same services. The owner is
created only in first-run setup; all later requests authenticate as that owner.
Internal account IDs are retained solely as data namespaces.

## Private operation

Logged-out runtime pages direct the operator to owner login, or to setup only on
a fresh server. Linked messaging channels process owner direct messages. Data,
apps, blog entries, and mail remain private to the owner.

## Money and migration

Credits meter configured expensive work and card payments top up the owner
wallet. The agent may make outbound x402 payments to a remote service under
spend limits. Inbound payment headers do not bypass authentication. Legacy
migration takes a full backup and selects the oldest admin as owner, resetting
instances that have no admin.
