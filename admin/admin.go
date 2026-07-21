package admin

import (
	"fmt"
	"html"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/mail"
)

// AdminHandler shows operational owner controls.
func AdminHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	content := `<div class="admin-links">
		<a href="/admin/console">Console</a>
		<a href="/admin/usage">API Usage</a>
		<a href="/admin/api">API Log</a>
		<a href="/admin/env">Environment</a>
		<a href="/admin/email">Mail Log</a>
		<a href="/admin/server">Server</a>
		<a href="/admin/blocklist">Mail Blocklist</a>
		<a href="/admin/spam">Spam Filter</a>
		<a href="/admin/log">System Log</a>
		<a href="/admin/diagnostics">Diagnostics</a>
		<a href="/admin/delete">Delete Content</a>
	</div>`

	html := app.RenderHTMLForRequest("Admin", "Admin Dashboard", content, r)
	w.Write([]byte(html))
}

// BlocklistHandler shows and manages the mail blocklist
func BlocklistHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// Handle POST requests for blocklist actions
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		action := r.FormValue("action")

		switch action {
		case "block_email":
			email := r.FormValue("email")
			if email == "" {
				http.Error(w, "Email required", http.StatusBadRequest)
				return
			}
			// Import mail package to access BlockEmail
			if err := blockEmail(email); err != nil {
				http.Error(w, "Failed to block email: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "block_ip":
			ip := r.FormValue("ip")
			if ip == "" {
				http.Error(w, "IP required", http.StatusBadRequest)
				return
			}
			if err := blockIP(ip); err != nil {
				http.Error(w, "Failed to block IP: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "unblock_email":
			email := r.FormValue("email")
			if err := unblockEmail(email); err != nil {
				http.Error(w, "Failed to unblock email: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "unblock_ip":
			ip := r.FormValue("ip")
			if err := unblockIP(ip); err != nil {
				http.Error(w, "Failed to unblock IP: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		http.Redirect(w, r, "/admin/blocklist", http.StatusSeeOther)
		return
	}

	// GET request - show blocklist
	bl := getBlocklist()

	content := `<h2>Mail Blocklist</h2>

	<div class="blocklist-section">
		<h3>Blocked Emails (` + fmt.Sprintf("%d", len(bl.Emails)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_email">
				<input type="text" name="email" placeholder="email@example.com or *@domain.com" required>
				<button type="submit">Block Email</button>
			</form>
			<p class="text-sm text-muted mt-1">Use *@domain.com to block entire domain</p>
		</div>`

	if len(bl.Emails) > 0 {
		content += `<table class="blacklist-table">
			<thead>
				<tr>
					<th>Email</th>
					<th class="text-center" style="width: 100px;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, email := range bl.Emails {
			content += blocklistEmailRow(email)
		}

		content += `</tbody></table>`
	} else {
		content += `<p>No blocked emails</p>`
	}

	content += `</div>

	<div class="blocklist-section">
		<h3>Blocked IPs (` + fmt.Sprintf("%d", len(bl.IPs)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_ip">
				<input type="text" name="ip" placeholder="192.168.1.1" required>
				<button type="submit">Block IP</button>
			</form>
		</div>`

	if len(bl.IPs) > 0 {
		content += `<table class="blacklist-table">
			<thead>
				<tr>
					<th>IP Address</th>
					<th class="text-center" style="width: 100px;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, ip := range bl.IPs {
			content += blocklistIPRow(ip)
		}

		content += `</tbody></table>`
	} else {
		content += `<p>No blocked IPs</p>`
	}

	content += `</div>
	<div class="mt-6">
		<p><a href="/admin">← Back to Admin</a></p>
	</div>`

	html := app.RenderHTMLForRequest("Admin", "Mail Blocklist", content, r)
	w.Write([]byte(html))
}

func blocklistEmailRow(email string) string {
	escapedEmail := html.EscapeString(email)
	return `
				<tr>
					<td><code>` + escapedEmail + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_email">
							<input type="hidden" name="email" value="` + escapedEmail + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
}

func blocklistIPRow(ip string) string {
	escapedIP := html.EscapeString(ip)
	return `
				<tr>
					<td><code>` + escapedIP + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_ip">
							<input type="hidden" name="ip" value="` + escapedIP + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
}

// SpamFilterHandler shows and manages the spam filter settings
func SpamFilterHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		action := r.FormValue("action")
		value := r.FormValue("value")

		switch action {
		case "toggle":
			sf := mail.GetSpamFilter()
			mail.SetSpamFilterEnabled(!sf.Enabled) //nolint:errcheck
		case "set_threshold":
			t := 5
			fmt.Sscanf(value, "%d", &t)
			mail.SetSpamThreshold(t) //nolint:errcheck
		case "toggle_reject":
			sf := mail.GetSpamFilter()
			mail.SetRejectSpam(!sf.RejectSpam) //nolint:errcheck
		case "toggle_autoblock":
			sf := mail.GetSpamFilter()
			mail.SetAutoBlockDomains(!sf.AutoBlockDomains) //nolint:errcheck
		case "add_tld":
			if value != "" {
				mail.AddBlockedTLD(value) //nolint:errcheck
			}
		case "remove_tld":
			if value != "" {
				mail.RemoveBlockedTLD(value) //nolint:errcheck
			}
		case "add_keyword":
			if value != "" {
				mail.AddBlockedKeyword(value) //nolint:errcheck
			}
		case "remove_keyword":
			if value != "" {
				mail.RemoveBlockedKeyword(value) //nolint:errcheck
			}
		case "add_allowed":
			if value != "" {
				mail.AddAllowedSender(value) //nolint:errcheck
			}
		case "remove_allowed":
			if value != "" {
				mail.RemoveAllowedSender(value) //nolint:errcheck
			}
		}

		http.Redirect(w, r, "/admin/spam", http.StatusSeeOther)
		return
	}

	sf := mail.GetSpamFilter()

	enabledStatus := "Disabled"
	enabledBtn := "Enable"
	if sf.Enabled {
		enabledStatus = "Enabled"
		enabledBtn = "Disable"
	}

	rejectStatus := "Drop silently"
	rejectBtn := "Switch to reject"
	if sf.RejectSpam {
		rejectStatus = "Save to filtered folder"
		rejectBtn = "Switch to silent drop"
	}

	autoBlockStatus := "Off"
	autoBlockBtn := "Enable"
	if sf.AutoBlockDomains {
		autoBlockStatus = "On"
		autoBlockBtn = "Disable"
	}

	content := fmt.Sprintf(`<h2>Spam Filter</h2>

	<div class="spam-settings">
		<h3>Settings</h3>
		<table class="blacklist-table">
			<tr>
				<td><strong>Filter Status</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Spam Handling</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_reject">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Auto-block spam domains</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_autoblock">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Score Threshold</strong></td>
				<td>%d</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="set_threshold">
						<input type="number" name="value" value="%d" min="1" max="100" style="width:60px">
						<button type="submit">Set</button>
					</form>
				</td>
			</tr>
		</table>
	</div>`, enabledStatus, enabledBtn, rejectStatus, rejectBtn,
		autoBlockStatus, autoBlockBtn, sf.Threshold, sf.Threshold)

	// Blocked TLDs
	content += `<div class="spam-section mt-4">
		<h3>Blocked TLDs (` + fmt.Sprintf("%d", len(sf.BlockedTLDs)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_tld">
			<input type="text" name="value" placeholder=".vn, .xyz, .top" required>
			<button type="submit">Block TLD</button>
		</form>`

	if len(sf.BlockedTLDs) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, tld := range sf.BlockedTLDs {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_tld">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, tld, tld)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Blocked keywords
	content += `<div class="spam-section mt-4">
		<h3>Blocked Keywords (` + fmt.Sprintf("%d", len(sf.BlockedKeywords)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_keyword">
			<input type="text" name="value" placeholder="keyword or phrase" required>
			<button type="submit">Block Keyword</button>
		</form>`

	if len(sf.BlockedKeywords) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, kw := range sf.BlockedKeywords {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_keyword">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, kw, kw)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Allowed senders
	content += `<div class="spam-section mt-4">
		<h3>Allowed Senders (` + fmt.Sprintf("%d", len(sf.AllowedSenders)) + `)</h3>
		<p class="text-sm text-muted">These senders bypass spam checks. Use @domain.com for entire domains.</p>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_allowed">
			<input type="text" name="value" placeholder="user@example.com or @domain.com" required>
			<button type="submit">Allow Sender</button>
		</form>`

	if len(sf.AllowedSenders) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, s := range sf.AllowedSenders {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_allowed">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, s, s)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	content += `<div class="mt-6">
		<p><a href="/admin">← Back to Admin</a></p>
	</div>`

	htmlPage := app.RenderHTMLForRequest("Admin", "Spam Filter", content, r)
	w.Write([]byte(htmlPage))
}

// Helper functions to access mail package functions
func blockEmail(email string) error {
	return mail.BlockEmail(email)
}

func blockIP(ip string) error {
	return mail.BlockIP(ip)
}

func unblockEmail(email string) error {
	return mail.UnblockEmail(email)
}

func unblockIP(ip string) error {
	return mail.UnblockIP(ip)
}

func getBlocklist() *mail.Blocklist {
	return mail.GetBlocklist()
}
