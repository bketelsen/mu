package github

import (
	"html"
	"net/url"
	"strconv"
	"strings"
)

func esc(s string) string { return html.EscapeString(s) }

func githubLink(rawURL, label string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") {
		return esc(label)
	}
	return `<a href="` + esc(u.String()) + `" target="_blank" rel="noopener noreferrer">` + esc(label) + `</a>`
}

func renderWorkspace(data workspaceData) string {
	var b strings.Builder
	renderSearchForm(&b, data.State)
	if isNotConfigured(data.Err) {
		b.WriteString(`<section class="card"><p>Set <code>GITHUB_TOKEN</code> to connect GitHub.</p><p><a href="/admin/env">Configure environment</a></p></section>`)
		return b.String()
	}
	if data.Err != nil {
		b.WriteString(`<section class="card"><p>GitHub workspace unavailable. Please try again.</p></section>`)
		return b.String()
	}
	b.WriteString(`<div class="github-layout"><aside class="github-repos"><h2>Repositories</h2>`)
	for _, repo := range data.Repositories.Repositories {
		owner, name := repo.Owner.Login, repo.Name
		if owner == "" || name == "" {
			owner, name = repositoryParts(repo.FullName)
		}
		class := "github-repo"
		if owner == data.State.Owner && name == data.State.Repo {
			class += " active"
		}
		b.WriteString(`<a class="` + class + `" href="` + workspaceURL(data.State, owner, name, 0, data.State.Page) + `">` + esc(repo.FullName) + `</a>`)
	}
	if len(data.Repositories.Repositories) == 0 {
		b.WriteString(`<p class="github-meta">No repositories found.</p>`)
	}
	b.WriteString(`</aside><main class="github-content">`)
	if data.ContentErr != nil {
		b.WriteString(`<section class="card"><p>GitHub workspace unavailable. Please try again.</p></section></main></div>`)
		return b.String()
	}
	if data.State.Owner == "" || data.State.Repo == "" {
		b.WriteString(`<p>Select a repository to view its work.</p></main></div>`)
		return b.String()
	}
	b.WriteString(`<p>` + githubLink(data.Repository.Repository.HTMLURL, data.State.Owner+`/`+data.State.Repo) + `</p><nav class="github-tabs">`)
	for _, tab := range []struct{ key, label string }{{"issues", "Issues"}, {"pulls", "Pull requests"}} {
		class := ""
		if tab.key == data.State.Tab {
			class = ` class="active"`
		}
		b.WriteString(`<a` + class + ` href="` + workspaceURL(data.State, data.State.Owner, data.State.Repo, 0, 1, tab.key) + `">` + tab.label + `</a>`)
	}
	b.WriteString(`</nav><p>`)
	for _, filter := range []string{"open", "closed", "all"} {
		b.WriteString(`<a href="` + workspaceURL(data.State, data.State.Owner, data.State.Repo, 0, 1, "", filter) + `">` + filter + `</a> `)
	}
	b.WriteString(`</p>`)
	if data.State.Tab == "issues" {
		for _, issue := range data.Repository.Issues {
			renderItem(&b, data.State, "Issue", issue.Number, issue.Title, issue.State, issue.Labels, issue.User.Login, issue.UpdatedAt.String(), issue.Comments, issue.HTMLURL)
		}
	} else {
		for _, pull := range data.Repository.PullRequests {
			renderItem(&b, data.State, "Pull request", pull.Number, pull.Title, pull.State, pull.Labels, pull.User.Login, pull.UpdatedAt.String(), pull.Comments, pull.HTMLURL)
		}
	}
	page := data.Repository.Page
	if page.Prev > 0 {
		b.WriteString(`<a href="` + workspaceURL(data.State, data.State.Owner, data.State.Repo, 0, page.Prev) + `">Previous</a> `)
	}
	if page.Next > 0 {
		b.WriteString(`<a href="` + workspaceURL(data.State, data.State.Owner, data.State.Repo, 0, page.Next) + `">Next</a>`)
	}
	if data.State.Number > 0 {
		renderThread(&b, data.Thread.Thread)
	}
	b.WriteString(`</main></div>`)
	return b.String()
}

func renderSearchForm(b *strings.Builder, state workspaceState) {
	placeholder := "Search issues"
	if state.Tab == "pulls" {
		placeholder = "Search pull requests"
	}
	b.WriteString(`<form method="get" action="/github"><input type="search" name="q" value="` + esc(state.Query) + `" placeholder="` + placeholder + `">`)
	if state.Owner != "" && state.Repo != "" {
		b.WriteString(`<input type="hidden" name="owner" value="` + esc(state.Owner) + `"><input type="hidden" name="repo" value="` + esc(state.Repo) + `">`)
	}
	b.WriteString(`<input type="hidden" name="tab" value="` + esc(state.Tab) + `"><input type="hidden" name="state" value="` + esc(state.State) + `"><button type="submit">Search</button></form>`)
}

func renderItem(b *strings.Builder, state workspaceState, kind string, number int, title, itemState string, labels []Label, author, updated string, comments int, rawURL string) {
	b.WriteString(`<article class="github-item"><strong>` + esc(kind) + ` #` + strconv.Itoa(number) + `</strong> ` + githubLink(rawURL, title) + `<div class="github-meta">` + esc(itemState) + ` by ` + esc(author) + ` updated ` + esc(updated) + ` · ` + strconv.Itoa(comments) + ` comments</div>`)
	for _, label := range labels {
		b.WriteString(`<span class="github-label">` + esc(label.Name) + `</span>`)
	}
	b.WriteString(`<a href="` + workspaceURL(state, state.Owner, state.Repo, number, state.Page) + `">View</a></article>`)
}

func renderThread(b *strings.Builder, thread Thread) {
	issue := thread.Issue
	title, body := issue.Title, issue.Body
	if thread.PullRequest != nil {
		title, body = thread.PullRequest.Title, thread.PullRequest.Body
	}
	b.WriteString(`<section class="card"><h2>` + esc(title) + `</h2><div class="github-body">` + esc(body) + `</div>`)
	for _, comment := range thread.Comments {
		b.WriteString(`<div class="github-body"><strong>` + esc(comment.User.Login) + `</strong><br>` + esc(comment.Body) + `</div>`)
	}
	b.WriteString(`</section>`)
}

func workspaceURL(state workspaceState, owner, repo string, number, page int, overrides ...string) string {
	tab, itemState := state.Tab, state.State
	if len(overrides) > 0 && overrides[0] != "" {
		tab = overrides[0]
	}
	if len(overrides) > 1 && overrides[1] != "" {
		itemState = overrides[1]
	}
	values := url.Values{"owner": {owner}, "repo": {repo}, "tab": {tab}, "state": {itemState}, "page": {strconv.Itoa(page)}}
	if state.Query != "" {
		values.Set("q", state.Query)
	}
	if number > 0 {
		values.Set("number", strconv.Itoa(number))
	}
	return "/github?" + values.Encode()
}
