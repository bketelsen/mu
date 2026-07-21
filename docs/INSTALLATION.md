# Installation

Mu is a private, single-owner home server. Keep it behind a network boundary
appropriate for private owner access.

## Start Mu

```bash
git clone https://github.com/micro/mu.git
cd mu
go build -o mu .
./mu setup
./mu --serve
```

Open `http://localhost:8080` and complete first-run setup. This creates the
only owner and configures an AI provider. Afterwards, `/setup` is unavailable
and no other local account can be created.

Set `ANTHROPIC_API_KEY`, `ATLAS_API_KEY`, `OPENAI_BASE_URL`, or a Copilot token
before starting if you want to configure the provider non-interactively. See
[Environment Variables](ENVIRONMENT_VARIABLES.md).

## Authentication

The owner can authenticate with password, passkey, linked Google identity, or a
PAT. Create PATs at `/token` for CLI, API, MCP, OAuth, and A2A clients. All
surfaces resolve to the same owner.

## Upgrade and migration

Back up the complete `~/.mu` data directory before upgrading. When migrating a
legacy data directory, Mu preserves that backup, selects the oldest admin as the
owner, or resets an instance that has no admin. Review the backup before
discarding legacy data.

## Optional services

Configure a reverse proxy and TLS for remote private access. Configure
`MAIL_PORT`, `MAIL_DOMAIN`, and DKIM for external mail. Discord, Telegram, and
WhatsApp operate only after their direct-message identity is linked to the owner.
