# Telegram

Mu's Telegram bot is a private owner channel. It accepts only direct messages
from the Telegram identity linked to the owner. Groups are not a Mu interface,
and the bot does not create accounts.

## Setup

1. Create a bot with [@BotFather](https://t.me/BotFather).
2. Set `TELEGRAM_BOT_TOKEN` in `/admin/env` or the server environment.
3. Link the owner to the bot in a direct message using Mu's account link flow.

Mu uses Telegram long polling and needs no inbound webhook.

## Use and privacy

Send the linked bot a direct message or a supported command such as `/ask`,
`/news`, `/weather`, or `/usage`. Each request runs as the owner.
Unlinking removes the association. Do not send owner credentials or link codes
outside a private direct message.
