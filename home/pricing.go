package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/wallet"
)

// PricingHandler describes the owner wallet rather than a public service plan.
func PricingHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<div style="max-width:760px;margin:0 auto">`)
	b.WriteString(`<div class="card"><h2>Owner credits</h2><p>Mu uses one owner wallet for configured AI and external-service costs. Authenticate as the owner to inspect balance or top up credits.</p><p><a href="/login">Owner login</a></p></div>`)
	b.WriteString(`<div class="card"><h3>Configured costs</h3><table class="stats-table"><tr><th>Action</th><th>Credits</th></tr>`)
	for _, cost := range []struct {
		name   string
		amount int
	}{
		{"AI agent", wallet.CostAgentQuery},
		{"Chat", wallet.CostChatQuery},
		{"Web search", wallet.CostWebSearch},
		{"External email", wallet.CostExternalEmail},
	} {
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, cost.name, cost.amount))
	}
	b.WriteString(`</table></div>`)
	b.WriteString(`<div class="card"><h3>Outbound x402</h3><p>The owner agent can make outbound x402 payments to configured remote services within spend limits. Incoming payment never replaces owner authentication.</p></div></div>`)

	page := app.RenderHTMLForRequest("Credits", "Owner wallet and configured costs", b.String(), r)
	w.Write([]byte(page))
}
