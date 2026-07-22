# Mu: A Private Owner-Operated Service Server

## Abstract

Mu is a self-hosted, private home server that combines everyday services and an
AI agent in one Go binary. It has one owner, one private data boundary, and one
set of authenticated interfaces: web, CLI, API, MCP, A2A, and linked messaging
direct messages.

## Motivation

Large internet platforms often finance services through advertising, tracking,
and engagement incentives. Historical industry context includes fragmented
identities, many unrelated provider accounts, and public social distribution.
Mu's current product response is not a shared network: it is an owner-operated
server that keeps the owner's services and data together.

## Runtime model

First-run setup creates the owner. No later local accounts are added. Password,
passkey, linked Google identity, PAT, OAuth, CLI, API, MCP, and A2A credentials
all resolve to the owner. Discord, Telegram, and WhatsApp run only in linked
owner direct messages.

Internal account IDs remain in storage and service interfaces to namespace owner
data. They are architectural identifiers, not a local user-provisioning model.
Every service surface is private after setup.

## Architecture

Mu runs as one Go binary on Go Micro. In-process services provide news, search,
mail, GitHub, weather, video, apps, and owner data. The agent composes
those services through authenticated tools. JSON files provide persistence, with
an optional SQLite search index; static assets and documentation are embedded.

## Migration and operation

Before legacy migration, Mu creates a backup of the entire data directory. It
keeps the oldest legacy admin as owner, or resets an instance that has no admin.
Operators retain their data locally and should back up the complete directory
before upgrades.

## Scope

Mu is intentionally not a hosted multi-user network. It does not provide local
account provisioning, public profiles, or federation. The
historical ideas of a shared settlement network and distributed social publishing
are not capabilities of the current product.
