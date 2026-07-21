# Single-User Mu Design

## Summary

Mu will become a private, single-owner home server. First-run setup creates the only human account. After setup, every application and service requires owner authentication except narrowly defined authentication callbacks, signed provider webhooks, static assets, and health/version probes.

The implementation will enforce one owner at shared authentication boundaries while retaining account IDs as internal partition keys. This avoids a repository-wide storage rewrite and preserves the surviving owner's existing data.

## Goals

- Permit exactly one login-capable human account per Mu instance.
- Make the configured instance private by default.
- Preserve password, passkey, Google, session, PAT, OAuth, API, MCP, CLI, and linked messaging-channel access for the owner.
- Remove account creation and features that only make sense between local users.
- Migrate legacy multi-account installations deterministically and safely.
- Preserve the selected owner's account-scoped data without changing domain storage schemas.

## Non-Goals

- Removing account IDs from domain records or service method signatures.
- Removing the non-login `micro` system identity used for assistant-generated content.
- Adding a supported multi-user compatibility mode.
- Adding an HTTP endpoint that resets or deletes the sole owner.
- Preserving access or data for legacy accounts that are not selected as owner.

## Owner Model

`internal/auth` remains authoritative for accounts, sessions, PATs, and passkeys. It gains explicit owner operations for retrieving the sole account and checking whether an owner exists.

`auth.Create` is the invariant boundary. It succeeds only when no account exists and rejects every attempt to create a second account, regardless of caller. The created account is always an admin and approved. New-account waiting periods, email-verification posting gates, approval status, and bans do not apply to the owner.

First-run setup is the only supported account-creation flow. A zero-account instance exposes setup; once the owner exists, setup closes. Domain handlers continue to receive the owner's account ID through the existing session and service interfaces.

The `micro` system identity may continue to author assistant content, but it is not stored or treated as a login-capable human account.

## Authentication And Network Access

HTTP access changes from a public/private prefix map to default deny.

Before setup, the public allowlist contains only:

- `/setup` and assets required to render it.
- Health and version probes that disclose no private content.

After setup, the unauthenticated allowlist contains only:

- Password, passkey, and Google sign-in entry points and callbacks.
- Mu OAuth authorization and token endpoints required to authenticate an owner-controlled client.
- Signed provider webhooks, such as WhatsApp and Stripe callbacks, that validate their provider credentials before processing.
- Static assets.
- Minimal health and version probes.

Unauthenticated browser requests outside that list redirect to `/login`. API, MCP, A2A, and JSON requests receive HTTP `401`. A credential that is structurally valid but does not resolve to the current owner receives HTTP `403` and cannot create a session.

x402 payment no longer substitutes for authentication. Account-free inbound x402 calls are disabled. The owner's wallet remains for personal usage accounting and outbound paid calls.

Password login, passkey assertions, PATs, existing sessions, Mu OAuth, API, MCP, A2A, and CLI access remain available when they resolve to the owner. Google sign-in may authenticate an owner whose Google email is already linked, or link Google from an existing owner session. It never provisions an account.

Mu OAuth discovery and dynamic client registration remain publicly reachable for MCP client interoperability, but registration grants no owner access. Authorization requires an authenticated owner, registered redirect URIs are validated, and a token is issued only for the owner.

## Removed Product Surface

The following features and their routes, tools, controls, templates, and documentation are removed:

- Web and MCP signup.
- Invites, invite requests, and invite administration.
- User lists and user administration.
- Account approval, bans, moderation controls, and new-account posting restrictions.
- Local-user blocking and unblocking.
- Peer-to-peer credit transfers.
- Public user profiles and directories.
- Public blog, social, stream, app, documentation, agent, API, MCP, A2A, pricing, and service pages.
- ActivityPub and WebFinger publication, because federation requires unauthenticated content access.
- Public and account-free x402 service access.
- Self-service deletion of the sole owner account.

Owner-authored blog, social, stream, app, mail, memory, wallet, and preference data remain available privately. References to signup, invitations, public use, multiple users, and account-free paid calls are removed from UI copy and current product documentation.

Implementation code with no remaining caller should be deleted, including invite storage, account auto-provisioning helpers, and obsolete moderation operations. Account ownership fields remain where they provide stable storage partitioning.

## Messaging Channels

Discord, Telegram, and WhatsApp retain linked owner access but never auto-create accounts.

- An unlinked sender cannot invoke the agent or any tool and receives a short linking instruction.
- Link credentials or one-time codes must authenticate the current owner. No other account ID may be linked.
- Existing links to deleted accounts are removed during migration.
- Agent and tool invocation is accepted only from a linked owner in a direct message.
- Group and server-channel invocations are ignored without running the agent or emitting a response, so private owner data cannot be disclosed into shared rooms.
- Unlinking removes channel access but does not affect the owner account.

Channel handlers inject the owner ID into agent and tool calls. They do not accept a sender-provided account ID.

## Legacy Migration

Migration runs once after all account-deletion hooks are registered and before indexing, background jobs, messaging clients, or the HTTP server start.

### Selection Rules

- Zero accounts: leave account-scoped data unchanged and enter first-run setup.
- One account: retain it, promote it to admin and approved, and make it the owner.
- Multiple accounts with one or more admins: retain the admin with the earliest `Created` timestamp. Break equal or zero timestamps by lexicographically smallest account ID.
- Multiple accounts with no admin: retain no account, delete all account-owned data, and enter first-run setup.

### Backup

Before the first migration mutation, copy the complete active data directory to a timestamped sibling backup directory. The backup must include all files, preserve file modes, and be created using a temporary name that is atomically renamed when complete. The destination must not be inside the source directory.

If any backup operation fails, remove the incomplete temporary backup, abort startup, and leave the active data directory unchanged. Log the final backup path after success without logging private file contents or secrets.

### Cleanup

After backup succeeds, migration performs cleanup synchronously:

- Run every registered account-deletion hook for each non-surviving account.
- Remove deleted accounts' sessions, PATs, passkeys, channel links, content, inboxes, wallets, preferences, memories, and user-created agents.
- Remove invites, invite requests, and obsolete multi-user moderation state.
- When no admin survives, run the same cleanup for every account.
- Persist the survivor's owner/admin/approved state and a migration-version marker only after cleanup succeeds.

Cleanup functions used by migration must return errors. A cleanup error aborts startup and is reported with the affected component and account ID. Because mutation may already have started, operators recover from the completed pre-migration backup rather than relying on an unsafe partial rollback.

The migration is idempotent. The version marker is written only after successful cleanup. On a later start, an instance with at most one account and a completed marker performs no backup or destructive cleanup. If startup is retried after partial cleanup, deletion operations must tolerate already-absent records and converge on the same result.

## Request And Identity Flow

1. First-run setup creates the owner through `auth.Create` and starts an owner session.
2. A later request presents a session cookie, PAT, passkey result, Google identity, or OAuth token.
3. Authentication validates the credential and resolves its account ID.
4. The owner boundary compares that account ID with the sole current owner.
5. Middleware rejects missing or non-owner authentication before a domain handler runs.
6. Domain handlers and server-side tool adapters receive the owner ID from trusted server state, not request arguments.
7. Existing account-partitioned stores read and write records under that owner ID.

Background jobs that act for a user must resolve the owner at execution time. They must not select an arbitrary account or accept a persisted deleted account ID without validation.

## Error Handling And Observability

- Missing credentials return `401` for machine clients and redirect browsers to `/login`.
- Valid non-owner credentials and disallowed messaging contexts return `403` or a channel-specific private-use response.
- Removed signup, invite, federation, and multi-user endpoints return `404`; they never retain functional hidden handlers.
- A second `auth.Create` call returns a specific single-owner error.
- Backup failure aborts before active data mutation.
- Migration cleanup failure aborts startup and identifies the failed component and account without logging its content.
- Migration logs the selected owner, numbers of accounts deleted, backup location, and completion marker version.
- Authentication logs must not include passwords, raw PATs, session tokens, OAuth codes, or channel link secrets.

## Testing

### Authentication

- The first account succeeds and is owner/admin/approved.
- A second account fails through every creation boundary.
- Owner lookup handles zero and one account and rejects an invalid multi-account runtime state.
- Password, passkey, Google, session, PAT, and OAuth flows issue access only for the owner.
- Google sign-in cannot provision an account.

### Migration

- Zero-account, one-account, multiple-admin, and no-admin cases follow the selection rules.
- Oldest-admin selection and account-ID tie-breaking are deterministic.
- The complete data directory is backed up before mutation.
- Backup failure leaves active data untouched and aborts startup.
- Cleanup is synchronous, comprehensive, idempotent, and removes stale credentials and channel links.
- A partial-cleanup retry converges safely.
- The migration marker prevents repeated backups and deletion after success.
- The surviving owner's domain data remains intact.

### HTTP And Protocol Access

- Default-deny middleware protects every application and service route.
- Setup and authentication exceptions are reachable only in their intended lifecycle states.
- Browser and machine-client failures use the expected redirect, `401`, or `403` behavior.
- API, MCP, A2A, and CLI calls work for the owner.
- x402 cannot bypass owner authentication.
- Signup, invite, federation, public profile, transfer, and multi-user administration endpoints are absent.

### Messaging Channels

- Unlinked senders cannot invoke tools or create accounts.
- Only owner credentials or owner-generated codes can create links.
- Linked owner direct messages work.
- Groups and shared server channels cannot invoke private tools.
- Deleted-account links are purged by migration.

### Product Cleanup

- Signup, invite, public-use, moderation, user-transfer, and multi-user language is absent from current UI and documentation.
- Owner mail, memory, apps, wallet data, posts, settings, and authentication methods continue to work.

### Repository Verification

Run:

```bash
go test ./... -short
go vet ./...
go build ./...
```

## Success Criteria

- A configured Mu instance has exactly one login-capable human account.
- No web, API, protocol, OAuth, Google, or messaging path can create a second account.
- No private application or service content is reachable without owner authentication.
- Legacy migration always completes a verified backup before deletion and deterministically keeps the oldest admin or resets an admin-less instance.
- The surviving owner's existing data and supported authentication methods continue to work.
- Multi-user-only features and product language are removed rather than hidden.
