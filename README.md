# Mu

A private, single-owner home server.

Mu is one Go binary for private GitHub, news, mail, search, weather, video,
places, apps, and an AI agent. Web, CLI, REST API, MCP, and A2A interfaces all
operate on the same owner data.

## Owner setup

On a fresh server, first-run setup creates Mu's only owner. After that, no local
accounts can be added and every surface requires owner authentication. Password,
passkey, linked Google identity, PAT, OAuth, CLI, API, MCP, and A2A credentials
all resolve to that owner.

```bash
git clone https://github.com/micro/mu
cd mu
go install
mu setup
mu --serve
```

Open `http://localhost:8080` to complete owner setup. Create a PAT at `/token`
for CLI and programmatic clients:

```bash
export MU_TOKEN="your-owner-pat"
mu news
mu agent "summarise today's news"
```

## Private channels

Discord, Telegram, and WhatsApp work only in direct messages after their
identity is linked to the owner. They never provision accounts or serve shared
channels.

## Migration and backup

Back up the complete data directory before upgrading. Legacy migration retains
that backup, uses the oldest admin as owner, or resets an instance with no admin.

## Payments

Credits meter configured AI and external operations. Card payments top up the
owner wallet. x402 is available only for owner-initiated outbound calls to remote
services; an incoming payment never bypasses authentication.

## Documentation

- [Installation](docs/INSTALLATION.md)
- [CLI](docs/CLI.md)
- [MCP](docs/MCP.md)
- [Environment Variables](docs/ENVIRONMENT_VARIABLES.md)
- [Security](docs/SECURITY.md)

Mu is open source under [AGPL-3.0](LICENSE).
