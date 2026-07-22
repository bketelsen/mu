# Messaging

Mail belongs to Mu's owner. The web interface, API, MCP, and linked owner
channels access one private mailbox; there is no local recipient directory.

## External mail

Set `MAIL_PORT` and `MAIL_DOMAIN` to receive mail, and configure DKIM for
outbound delivery:

```bash
export MAIL_PORT="25"
export MAIL_DOMAIN="mu.example.com"
export MAIL_SELECTOR="default"
./scripts/generate-dkim-keys.sh mu.example.com default
```

Publish MX, SPF, DKIM, and optional DMARC DNS records for the configured domain.
The SMTP server accepts mail only for the owner mailbox and is not an open relay.
Sending external mail requires owner authentication.

## Channels

Discord, Telegram, and WhatsApp are separate owner channels, not shared inboxes.
Link the owner's channel identity and use direct messages only. Channel requests
use the owner session and cannot select another local account.

## Operations

Back up `~/.mu` with the rest of the server data. Protect DKIM private keys with
filesystem permissions and keep the admin interface private.
