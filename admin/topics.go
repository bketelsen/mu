package admin

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/topics"
)

// TopicsHandler shows and manages the owner-configured news feeds and chat prompts.
func TopicsHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireOwner(r); err != nil {
		app.Forbidden(w, r, "Owner access required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		renderTopics(w, r, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			renderTopicsError(w, r, "Failed to parse form")
			return
		}
		if r.FormValue("_csrf") == "" || !auth.ValidCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}

		topic := topics.Topic{
			Name:    r.FormValue("name"),
			FeedURL: r.FormValue("feed_url"),
			Prompt:  r.FormValue("prompt"),
		}
		var err error
		switch r.FormValue("action") {
		case "create":
			_, err = topics.Create(topic)
		case "update":
			_, err = topics.Update(topic.Name, topic)
		case "delete":
			_, err = topics.Delete(topic.Name)
		default:
			renderTopicsError(w, r, "Unsupported action")
			return
		}
		if err != nil {
			renderTopicsError(w, r, err.Error())
			return
		}
		http.Redirect(w, r, "/admin/topics?saved=1", http.StatusSeeOther)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func renderTopicsError(w http.ResponseWriter, r *http.Request, message string) {
	w.WriteHeader(http.StatusBadRequest)
	renderTopics(w, r, message)
}

func renderTopics(w http.ResponseWriter, r *http.Request, message string) {
	var b strings.Builder
	token := html.EscapeString(auth.CSRFToken(r))

	if r.URL.Query().Get("saved") == "1" {
		b.WriteString(`<div class="card" style="background:#f0fff0;border-color:#a3d9a5"><p style="color:#27ae60;margin:0">Topic configuration saved. Refresh started.</p></div>`)
	}
	if message != "" {
		b.WriteString(`<div class="card" style="background:#fff0f0;border-color:#d9a3a3"><p style="color:#c00;margin:0">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p></div>`)
	}

	b.WriteString(`<div class="card"><h3>Add topic</h3><form method="POST" action="/admin/topics">`)
	writeTopicsCSRF(&b, token)
	b.WriteString(`<label>Name<input type="text" name="name" required></label><label>RSS feed URL<input type="url" name="feed_url" required></label><label>Chat prompt<textarea name="prompt" required></textarea></label><button type="submit" name="action" value="create">Add topic</button></form></div>`)

	for _, topic := range topics.Snapshot() {
		name := html.EscapeString(topic.Name)
		feedURL := html.EscapeString(topic.FeedURL)
		prompt := html.EscapeString(topic.Prompt)
		b.WriteString(`<div class="card"><h3><strong>`)
		b.WriteString(name)
		b.WriteString(`</strong></h3><form method="POST" action="/admin/topics">`)
		writeTopicsCSRF(&b, token)
		b.WriteString(`<input type="hidden" name="name" value="`)
		b.WriteString(name)
		b.WriteString(`"><label>RSS feed URL<input type="url" name="feed_url" value="`)
		b.WriteString(feedURL)
		b.WriteString(`" required></label><label>Chat prompt<textarea name="prompt" required>`)
		b.WriteString(prompt)
		b.WriteString(`</textarea></label><button type="submit" name="action" value="update">Save topic</button></form><form method="POST" action="/admin/topics">`)
		writeTopicsCSRF(&b, token)
		b.WriteString(`<input type="hidden" name="name" value="`)
		b.WriteString(name)
		b.WriteString(`"><p>Deletion hides active content but retains history.</p><button type="submit" name="action" value="delete">Delete topic</button></form></div>`)
	}
	b.WriteString(`<p><a href="/admin">Back to Admin</a></p>`)

	w.Write([]byte(app.RenderHTMLForRequest("Topics", "News feeds and chat prompts", b.String(), r)))
}

func writeTopicsCSRF(b *strings.Builder, token string) {
	b.WriteString(`<input type="hidden" name="_csrf" value="`)
	b.WriteString(token)
	b.WriteString(`">`)
}
