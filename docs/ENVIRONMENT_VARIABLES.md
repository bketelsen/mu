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
export GITHUB_TOKEN="github_pat_..."
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

`GITHUB_TOKEN` is a fine-grained GitHub token for Mu's owner-only, read-only
repository, issue, and pull-request service. Grant metadata, issues, and pull
request read access only to repositories Mu should expose. It is separate from
`COPILOT_GITHUB_TOKEN`.

## Costs and storage

`NOTES` controls the background owner-note loop. It is enabled by default; set
it to `off`, `false`, `0`, or `no` to disable it.

Legacy payment-related environment variables are no longer read by Mu and may
be removed manually.

`MU_USE_SQLITE=1` enables the SQLite search index. Back up the complete
`~/.mu` data directory before upgrades or migration.
