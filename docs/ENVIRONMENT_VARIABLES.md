# Environment Variables

Configuration can be supplied through the environment or `/admin/env` after
owner authentication. Environment values are read before stored settings.

## Owner and authentication

`ADMIN` identifies the intended owner during first-run setup. If it is unset,
the first setup identity becomes the owner. Setup is one-time: no later local
accounts are added.

```bash
export ADMIN="you@example.com"
export PASSKEY_ORIGIN="https://mu.example.com"
export PASSKEY_RP_ID="mu.example.com"
export PASSKEY_EXTRA_ORIGINS="https://mu-onion.example.onion"
export GOOGLE_CLIENT_ID="..."
export GOOGLE_CLIENT_SECRET="..."
export GOOGLE_REDIRECT_URI="https://mu.example.com/oauth2/callback"
```

Google authentication resolves only an already-linked owner identity.
`PASSKEY_EXTRA_ORIGINS` is a comma-separated allowlist for additional WebAuthn
origins. `GOOGLE_REDIRECT_URI` defaults to `<request-origin>/oauth2/callback`.

## AI and services

```bash
export ANTHROPIC_API_KEY="..."       # or ATLAS_API_KEY
export OPENAI_BASE_URL="http://localhost:11434/v1"
export OPENAI_API_KEY="ollama"
export COPILOT_GITHUB_TOKEN="..."
export BRAVE_API_KEY="..."
export YOUTUBE_API_KEY="..."
export GOOGLE_API_KEY="..."
export MAIL_PORT="2525"
export MAIL_DOMAIN="mu.example.com"
export MAIL_SELECTOR="default"
export DKIM_PRIVATE_KEY="-----BEGIN ..."
```

Set one AI provider for chat and agent features. `MAIL_PORT`, `MAIL_DOMAIN`,
and optional DKIM values configure external mail delivery.

## Credits and outbound x402

```bash
export STRIPE_SECRET_KEY="..."
export STRIPE_PUBLISHABLE_KEY="..."
export STRIPE_WEBHOOK_SECRET="..."
export X402_PAY_TO="0xYourOwnerWallet"
export X402_FACILITATOR_URL="https://api.cdp.coinbase.com/platform/v2/x402"
export X402_NETWORK="eip155:8453"
export X402_ASSETS="USDC,EURC"
export CDP_API_KEY_ID="..."
export CDP_API_KEY_SECRET="..."
```

Stripe tops up the owner's credit balance. x402 configuration enables owner
initiated payments to remote services. It does not expose an incoming payment
route, and payment headers never replace authentication. CDP settlement uses an
Ed25519 Secret API Key; set `CDP_API_KEY_ID` and `CDP_API_KEY_SECRET` when the
configured CDP facilitator requires them.

## Costs and storage

`NOTES` controls the background owner-note loop. It is enabled by default; set
it to `off`, `false`, `0`, or `no` to disable it.

The following variables override integer credit costs: `CREDIT_COST_NEWS`,
`CREDIT_COST_VIDEO`, `CREDIT_COST_CHAT`, `CREDIT_COST_BLOG_CREATE`,
`CREDIT_COST_MAIL`, `CREDIT_COST_EMAIL`, `CREDIT_COST_PLACES_SEARCH`,
`CREDIT_COST_PLACES_NEARBY`, `CREDIT_COST_WEATHER`,
`CREDIT_COST_WEATHER_POLLEN`, `CREDIT_COST_SEARCH`, `CREDIT_COST_FETCH`,
`CREDIT_COST_DB_WRITE`, `CREDIT_COST_AGENT`, `CREDIT_COST_AGENT_PREMIUM`,
`CREDIT_COST_SOCIAL`, `CREDIT_COST_SOCIAL_POST`, `CREDIT_COST_SOCIAL_REPLY`,
`CREDIT_COST_BLOG_COMMENT`, `CREDIT_COST_IMAGE`, `CREDIT_COST_APP_BUILD`, and
`CREDIT_COST_APP_EDIT`.

`MU_USE_SQLITE=1` enables the SQLite search index. Back up the complete
`~/.mu` data directory before upgrades or migration.
