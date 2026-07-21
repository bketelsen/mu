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
export GOOGLE_CLIENT_ID="..."
export GOOGLE_CLIENT_SECRET="..."
```

Google authentication resolves only an already-linked owner identity.

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
```

Stripe tops up the owner's credit balance. x402 configuration enables owner
initiated payments to remote services. It does not expose an incoming payment
route, and payment headers never replace authentication.

## Costs and storage

`CREDIT_COST_NEWS`, `CREDIT_COST_VIDEO`, `CREDIT_COST_CHAT`,
`CREDIT_COST_EMAIL`, `CREDIT_COST_PLACES_SEARCH`, and
`CREDIT_COST_PLACES_NEARBY` override operation costs. `MU_USE_SQLITE=1` enables
the SQLite search index. Back up the complete `~/.mu` data directory before
upgrades or migration.
