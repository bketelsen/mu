# Security

Mu is an owner-only server. Its primary security invariant is that every request
and tool call resolves to the configured owner, never to an identity supplied by
an LLM, client argument, or channel message.

## Authentication

After first-run setup, web pages, APIs, MCP, A2A, and CLI calls require owner
authentication. Passwords, passkeys, linked Google, sessions, OAuth, and PATs
all resolve to the same owner. Discord, Telegram, and WhatsApp accept only
linked-owner direct messages.

## Tool binding

The LLM treats fetched content as untrusted data. Account-scoped tools bind the
internal owner ID server-side, and mutations verify ownership. Internal IDs are
architectural storage keys, not client-selectable principals.

## Payments

Wallet credits and card top-ups belong to the owner. Outbound x402 requests are
restricted to configured services and spend limits. Incoming x402 payment never
grants access, authenticates an API request, or changes the owner boundary.

## Operations

Keep Mu behind private network controls, use TLS for remote access, protect PATs
and environment secrets, and back up the complete data directory before upgrades
or migration. Review auth, wallet, and tool-registration changes for owner
binding regressions.
