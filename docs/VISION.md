# Vision

Mu is a private, single-owner home server for the everyday internet. One Go
binary combines news, mail, search, GitHub, weather, video, places, apps, and
an AI agent under the owner's control.

## The problem

Everyday services are often fragmented across advertising-driven platforms.
They compete for attention, profile people, and separate useful tools from the
person who depends on them. Mu keeps a personal set of services on an owner-run
server instead.

## The owner model

First-run setup creates the only owner. All web, CLI, API, MCP, and A2A access
authenticates as that owner through a password, passkey, linked Google identity,
session, OAuth credential, or PAT. Linked Discord, Telegram, and WhatsApp direct
messages use the same owner boundary.

The agent remembers owner preferences and works across private owner data. It
does not expose a shared social stream, public profile system, or local user
directory.

## Design choices

- **Intent over engagement.** No ads, tracking, infinite scroll, or addictive
  ranking mechanics.
- **Services, not widgets.** Each capability is an in-process Go Micro service
  reachable through authenticated owner interfaces.
- **One binary.** Mu is self-hosted with local persistence and optional local
  AI models through Ollama or another OpenAI-compatible endpoint.
- **Private channels.** Messaging integrations accept only linked-owner direct
  messages.
- **Bounded payments.** Credits meter configured work. x402 is only an outbound
  owner payment mechanism for a remote service and never authenticates incoming
  access.

## For developers

Create an owner PAT at `/token` and configure an MCP client for your server:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.example.com/mcp",
      "headers": {"Authorization": "Bearer YOUR_OWNER_PAT"}
    }
  }
}
```

The CLI (`mu news`, `mu agent "..."`) and API use the same owner credentials.
