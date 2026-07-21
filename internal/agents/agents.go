// Package agents renders the private owner integration overview at /agents.
package agents

import (
	"net/http"

	"mu/internal/app"
)

// Handler renders the owner API landing page.
func Handler(w http.ResponseWriter, r *http.Request) {
	body := `<p style="max-width:560px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 28px">Mu's MCP and REST interfaces are private owner interfaces. Authenticate with an owner Personal Access Token to use GitHub, news, weather, search, mail, and other services programmatically.</p>

<div class="pcards">
  <a class="pcard" href="/mcp"><h3>MCP endpoint</h3><p>Use the authenticated owner MCP endpoint at <code>/mcp</code>.</p></a>
  <a class="pcard" href="/api"><h3>REST and API docs</h3><p>View the owner-authenticated HTTP reference.</p></a>
  <a class="pcard" href="/token"><h3>Personal Access Token</h3><p>Create a token after owner login.</p></a>
</div>

<div class="px402"><h2>Payments</h2><p>Metered work uses owner credits. x402 is available only when the owner agent makes an outbound call to a configured remote service; incoming payments never grant access.</p></div>

<div style="margin-top:26px"><a class="pcta" href="/login">Owner login</a></div>

<style>
.pcards{display:flex;flex-wrap:wrap;gap:14px;max-width:760px;justify-content:center}
.pcard{flex:1 1 220px;min-width:220px;max-width:240px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;text-decoration:none;color:inherit;background:#fff;text-align:left}
.pcard h3{margin:0 0 6px;font-size:1em}.pcard p,.px402 p{margin:0;font-size:14px;color:#666;line-height:1.5}.pcard code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.px402{max-width:620px;margin:44px auto 0;text-align:left;border-top:1px solid #eee;padding-top:28px}.px402 h2{font-size:1.15em;margin:0 0 12px}
.pcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:11px 20px;border-radius:8px;font-weight:700;font-size:15px}
</style>`

	page := app.RenderLanding(app.Landing{
		Title:       "Mu - owner integrations",
		Description: "Private owner MCP and REST integrations.",
		Brand:       "Mu",
		Tagline:     "Owner integrations",
		TopRight:    `<a href="/login">Owner login</a>`,
		Body:        body,
		Footer:      `<a href="/mcp">MCP</a><a href="/api">API</a><a href="/docs">Docs</a><a href="/login">Owner login</a>`,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}
