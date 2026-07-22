# Remove Wallet and Payments

## Goal

Remove Mu's wallet and every payment capability completely. Mu will no longer
maintain credit balances, meter operations, sell credits, process Stripe
payments, hold cryptocurrency keys, make x402 payments, price apps, or expose
payment-related interfaces.

Capabilities that were previously metered remain available to the authenticated
owner without quota checks or charges. Existing authentication, CSRF, rate
limiting, provider credential checks, and service-specific validation remain in
force.

## Compatibility

This is an intentional breaking removal. Deleted HTTP routes use normal
not-found behavior, deleted MCP tools use normal unknown-tool behavior, and
deleted CLI or client commands use their existing unknown-command behavior.
There will be no redirects, deprecation handlers, no-op quota interfaces, or
legacy payment response models.

Existing app JSON may contain `price` and `earnings` fields. Go's JSON decoder
will ignore those unknown fields after they are removed from the app model, and
subsequent saves will omit them. No compatibility fields will remain in code.

## Architecture

The wallet ceases to be a subsystem. Delete the complete `wallet/` package,
including its credit ledger, Stripe integration, Base wallet, cryptographic
implementation, x402 client, spend limits, HTTP handlers, and tests.

Remove payment composition from `main.go`: startup loading, account-cleanup
hooks, routes, Stripe webhook CSRF exceptions, and quota or charge callback
wiring. Services invoke their underlying work directly after their existing
authentication and validation.

Remove the generic metering abstractions rather than retaining inert shells.
This includes operation cost constants, `WalletOp`, `QuotaCheck`, `ChargeQuota`,
credit formatters, quota result metadata, and HTTP 402 responses used for Mu's
credit gating.

## Removal Scope

### Runtime and services

- Remove wallet imports, balance checks, quota checks, charges, transaction
  writes, payment retries, and credit-specific error handling from all services.
- Keep formerly metered news, video, chat, blog, social, mail, places, weather,
  web search, web fetch, image generation, app generation, and agent operations
  available to the authenticated owner.
- Remove wallet account-deletion hooks and payment-specific health or status
  checks.
- Remove Stripe webhook handling and its CSRF exception.
- Remove Stripe, x402, Base wallet, credit-cost, and daily-quota settings from
  configuration and the admin environment UI.

### Apps

- Remove `Price` and `Earnings` from app models, API payloads, editor forms,
  generated app metadata, listings, launch controls, and persisted writes.
- Remove app-use charging and author revenue transfers.
- Treat every existing and newly created app as free to launch.
- Remove generic app fetch and generation quota callbacks and charges.

### API, MCP, agents, and clients

- Remove `/wallet` and all subroutes from the HTTP server and API catalog.
- Remove wallet balance and top-up MCP tools and agent result formatters.
- Remove metering fields and checks from MCP tool registration and execution.
- Remove wallet and x402 CLI commands and their dispatch entries.
- Remove Discord wallet commands, interactions, and presentation metadata.
- Remove payment language and pricing badges from API, MCP, and agent pages.

### User interface and assets

- Remove wallet navigation, balance badges, pricing pages, top-up controls,
  payment status displays, and dedicated wallet assets.
- Remove payment-specific CSS and JavaScript when no remaining feature uses it.
- Remove credit prices and top-up instructions from service pages and errors.

### Scripts and documentation

- Delete x402 verification scripts and wallet/x402-specific documentation.
- Remove payment setup, environment variables, architecture descriptions,
  security claims, product copy, examples, and command references from current
  documentation.
- Scrub payment claims from historical specs and plans under
  `docs/superpowers/`; they are not retained as immutable history.
- Update repository guidance, including `CLAUDE.md`, so it no longer describes
  wallet or payment architecture and conventions.
- Preserve generic finance or payment language in news, mail spam detection,
  search results, editorial prompts, and fixtures when it does not describe a Mu
  product capability.

## Destructive Data Migration

Before services load, a one-time migration permanently deletes all known Mu
payment data:

- `wallets.json`
- `transactions.json`
- `daily_usage.json`
- `trade_wallets.json`
- `~/.mu/keys/wallet.seed`

The migration does not inspect balances, export keys, create backups, or offer
recovery. Deleting `trade_wallets.json` and `wallet.seed` can permanently destroy
access to on-chain funds; this is explicitly required.

The migration is idempotent. A missing target counts as successfully deleted.
It records a durable `remove-wallet-payments-v1.done` marker in Mu's data
directory only after all targets are absent. If the home directory cannot be
resolved or any target cannot be removed, startup fails with the affected path
and cause, and the marker is not written. A later startup retries the complete
deletion set.

Payment-related environment variables in an operator's shell or `.env` are not
files owned by Mu and will not be rewritten. Mu stops reading and exposing them,
and migration documentation identifies them as obsolete for manual removal.

## Runtime Flow

After migration, authenticated requests flow from HTTP, API, MCP, agent, or
client entry points directly to their service handlers. No request performs a
balance lookup, quota decision, credit deduction, transaction write, payment
settlement, or payment retry. Failures come only from authentication,
authorization, rate limiting, provider calls, validation, or the underlying
service.

## Error Handling

- Data deletion failures stop startup and identify the exact path and operating
  system error.
- Removed interfaces use existing not-found or unknown-command behavior.
- Formerly metered operations no longer return insufficient-credit or top-up
  errors.
- The implementation introduces no replacement billing, quota, or spending
  mechanism.

## Testing

- Add migration tests using temporary data and home directories. Verify deletion
  of every target, success for missing targets, safe reruns, marker creation only
  after complete deletion, and startup failure when deletion is impossible.
- Rewrite service, MCP, agent, app, client, and main tests to verify direct
  execution without quota callbacks or credit failures.
- Verify app JSON with legacy `price` and `earnings` fields still loads and is
  saved without those fields.
- Verify route, tool, command, navigation, environment, and diagnostics listings
  no longer expose wallet or payment features.
- Run `go test ./... -short`, `go build ./...`, and `go vet ./...`.
- Search the repository for wallet imports, payment routes, credit pricing,
  Stripe, x402, Base wallet, billing, top-up, and payment-gating terminology.
  Remaining matches must be this removal specification or legitimate generic
  content unrelated to a Mu payment capability.

## Success Criteria

- The repository contains no `wallet/` package, payment implementation, payment
  script, or wallet-specific asset.
- The binary exposes no wallet, credit, Stripe, cryptocurrency, x402, paid-app,
  billing, top-up, or payment capability.
- All formerly metered owner capabilities remain usable without payment checks.
- Paid-app fields and revenue behavior are gone; all apps launch without charge.
- Startup permanently removes all specified ledgers and private-key files before
  serving requests, with no backup or recovery path.
- Current and historical documentation no longer claims Mu supports payments,
  except for this removal specification.
- The short test suite, build, and vet pass.
