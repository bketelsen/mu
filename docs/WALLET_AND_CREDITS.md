# Wallet and Credits

Mu has one owner wallet. Credits meter configured AI and external-service costs;
they are not transferable between local accounts.

## Credits

Credits are integer balances, normally valued at one penny GBP. The owner can
inspect usage and balance at `/wallet`. Configured card payments add credits via
Stripe webhook. When payment providers are absent, self-hosted operators can
choose their own quota configuration.

## Outbound x402

Mu can act as an x402 payer for owner-initiated requests to a remote MCP service.
Set `X402_PAY_TO`, `X402_NETWORK`, `X402_ASSETS`, and a facilitator, then set
appropriate spend limits. The remote service returns a payment requirement; Mu
signs and settles the outbound payment before retrying its request.

This is distinct from removed inbound payment access: an incoming `X-PAYMENT`
header does not authenticate a request, add credits, or grant access to Mu.

## Security

The owner session binds all wallet operations. Transaction records and balance
changes are retained in the owner data directory. Back up the complete data
directory before migration or recovery work.
