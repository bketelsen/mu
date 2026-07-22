# Discord

Mu's Discord integration is a private owner channel. It responds only to direct
messages from a Discord identity linked to the owner; it does not serve guild
channels or provision accounts.

## Setup

1. Create a Discord application and bot at
   [Discord Developer Portal](https://discord.com/developers/applications).
2. Set `DISCORD_BOT_TOKEN` in `/admin/env` or the server environment.
3. In a direct message, link the Discord identity to the owner using the account
   link flow in Mu.

The bot reconnects automatically after its token is configured.

## Use

After linking, direct-message the bot or use its supported commands, such as
`/agent`, `/news`, `/weather`, and `/mail`. Requests use the owner data.
Unlinking removes the channel association.

## Security

- Only linked-owner direct messages are processed.
- Credentials and link codes must never be sent in a server channel.
- The integration cannot create, select, or access another local account.
