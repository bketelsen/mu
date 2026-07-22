# Auth, Identity, and Channel Linking

Covers `internal/auth`, `internal/setup`, and the owner-linking model
used by `client/discord`, `client/telegram`, `client/whatsapp`.

## Single-owner model

Mu enforces exactly one account ("the owner"). `internal/setup`'s
`/setup` handler (`internal/setup/setup.go:28`) is only reachable while
no owner exists (`Needed`, `:21`); it validates username/password,
optionally stores an AI provider's credentials (`applyProvider`,
`:96-129`), creates the sole account via `auth.Create`
(`internal/auth/auth.go:189`), logs in, and redirects home. After that,
every surface — web, CLI, REST, MCP, A2A, Discord/Telegram/WhatsApp —
authenticates back to that one account. There is no local account
creation UI/flow after setup.

`internal/app/private.go`'s `Private` middleware
(`internal/app/private.go:41`) wraps almost the entire HTTP mux:
unauthenticated API/MCP/A2A requests get a 401 JSON response; browser
requests get redirected to `/setup` or `/login` as appropriate.

## `internal/auth` core types

- `Account` (`internal/auth/auth.go:30`) — the owner's record.
- `Session` (`:114`) — server-side session, looked up from a cookie or
  an `Authorization: Bearer` value.
- `Token` (`:123`) — Personal Access Tokens (PATs), for CLI/API/MCP
  clients.
- Singleton checks: `Owner`, `OwnerExists`, `IsOwner` (`:156-182`).
- `Login` (`:273`) — password auth.
- Request-level authn: `GetSession`, `RequireSession`, `RequireAdmin`
  (`:359-469`).

### Sessions
UUID token, base64-encoded, persisted in `sessions.json`. Accepted
from either a cookie or an `Authorization` bearer value — this is what
lets the same session mechanism serve both browser and API/CLI
clients.

### PATs (Personal Access Tokens)
Created via `CreateToken` (`:527`) — a random 32-byte token, bcrypt-
hashed at rest; the raw value is returned exactly once at creation
time (shown at `/token`). `ValidatePAT` (`:571`) checks expiry and
ownership. Used by `internal/cli` and any external MCP/REST client
(`export MU_TOKEN=...`).

### Passkeys
WebAuthn credential model: `Passkey`, `WebAuthnUser`
(`internal/auth/passkey.go:15-60`), CRUD at `:72-127`. Passkey auth
resolves to an ordinary owner session — no separate identity type.

### OAuth 2.1 (for MCP clients)
Dynamic client registration + authorization-code-with-PKCE
(`internal/auth/oauth.go:18-162`, HTTP endpoints from `:197`):
`/oauth/register`, `/oauth/authorize`, `/oauth/token`,
`/.well-known/oauth-authorization-server`,
`/.well-known/oauth-protected-resource`. Lets MCP clients that require
standard OAuth (rather than a bare PAT) authenticate.

### Google sign-in
Mu acts as an OAuth *client* of Google (`app.GoogleLogin`,
`GoogleConnect`, `GoogleCallback`) to link a Google identity to the
owner account — this is identity linking, not a separate account type.

### CSRF
`auth.SetCSRFCookie`/`ValidCSRF` (`internal/auth/csrf.go:63-94`)
protect state-changing form posts. Skipped for Bearer/PAT-authenticated
API calls, `/mcp` (own auth), auth endpoints (`/login`,
`/passkey/*`, `/oauth/*`), and inbound webhooks (paths ending `/inbox`).
Tokens are process-local and intentionally tolerate an absent token
from stale clients.

### Migration from legacy multi-account data
`MigrateSingleOwner` (`internal/auth/migration.go:21-110`) runs at
startup (`main.go:106-115`, called via `migrateSingleOwner`): backs up
data first, then retains the earliest non-`micro` admin as the owner
(or resets if there's no admin), deleting other accounts' dependent
data through registered cleanup hooks. Registered hooks
(`main.go:93-104`, `RegisterAccountCleanup`) span blog posts, apps,
stream entries, mail inbox, micro-agents, channel links (Discord/
Telegram/WhatsApp), content prefs, and memory — every package that
holds account-scoped data must register a hook here so account
deletion/migration doesn't leave orphaned data.

## Channel linking model (Discord / Telegram / WhatsApp)

All three channel integrations follow the same invariant: **link an
external identity to the existing owner; never provision a new
account.** Every link flow explicitly checks `auth.IsOwner` after
`auth.Login` succeeds and rejects non-owner accounts.

| | Discord | Telegram | WhatsApp |
|---|---|---|---|
| Enable via | `DISCORD_BOT_TOKEN` | `TELEGRAM_BOT_TOKEN` | `WHATSAPP_TOKEN` + `WHATSAPP_PHONE_ID` |
| Transport | Gateway WebSocket | Long polling | Business Cloud webhook (`/whatsapp/webhook`) |
| Link store | `discord_links.json` | `telegram_links.json` | `whatsapp_links.json` |
| Link flow | `link <user> <pass>` or one-time `link <code>` (`GenerateLinkCode`, 5 min expiry) | `link <user> <pass>` only | `link <user> <pass>` only |
| Unlink | `unlink` | `unlink` | `unlink` |
| Scope | DMs only, ignores guild messages and bots | Private chats only | Ignores group messages / non-text |
| Notify | `NotifyUser`/`NotifyEmbed`, `NotifyNewMail` (uses `SummariseEmail` on the background model) | `NotifyUser` | `NotifyUser` (Meta Graph API, replies capped ~4000 chars) |
| Cleanup hook | `discord.DeleteLinks` | `telegram.DeleteLinks` | `whatsapp.DeleteLinks` |

All non-command DMs/messages become private `agent.QueryWithOpts(...,
Public: false)` calls; each channel keeps a small in-memory rolling
history (10 messages) per external identity for conversational
context. WhatsApp webhook POSTs are additionally HMAC-SHA256 verified
against `X-Hub-Signature-256` using `WHATSAPP_APP_SECRET` before any
processing.

`mail.OnNewMail` (wired in `main.go:269-276`) fans a summarized
new-mail notification out to every linked channel simultaneously.

See `docs/DISCORD.md`, `docs/TELEGRAM.md`, `docs/SECURITY.md` for the
operator-facing version of these invariants.
