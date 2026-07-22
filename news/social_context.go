package news

import (
	"fmt"
	htmlpkg "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mu/internal/app"
)

const nitterInstance = "nitter.poast.org"

type socialContext struct {
	Posts []socialContextPost
}

type socialContextPost struct {
	Author   string
	Handle   string
	Platform string
	Content  string
	URL      string
}

type externalSocialPost struct {
	Author  string
	Content string
}

var fetchExternalSocialPostForContext = fetchExternalSocialPost

// fetchSocialContext enriches articles with referenced X or Truth Social posts.
func fetchSocialContext(articleURL, articleContent string) *socialContext {
	if articleURL == "" {
		return nil
	}

	urls := detectSocialURLs(articleContent)
	if len(urls) == 0 {
		return nil
	}

	var posts []socialContextPost
	for _, u := range urls {
		if u == articleURL {
			continue
		}

		post, err := fetchExternalSocialPostForContext(u)
		if err != nil {
			app.Log("news", "Failed to fetch social context post %s: %v", u, err)
			continue
		}

		platform := "X"
		if isTruthSocialURL(u) {
			platform = "Truth Social"
		}
		posts = append(posts, socialContextPost{
			Author: post.Author, Handle: post.Author, Platform: platform,
			Content: post.Content, URL: u,
		})
	}

	if len(posts) == 0 {
		return nil
	}
	return &socialContext{Posts: posts}
}

func detectSocialURLs(content string) []string {
	re := regexp.MustCompile(`https?://(?:(?:(?:www|mobile)\.)?(?:twitter\.com|x\.com)|(?:(?:www\.)?truthsocial\.com))/[^\s"'<>\])+]+`)
	matches := re.FindAllString(content, -1)

	seen := map[string]bool{}
	var unique []string
	for _, match := range matches {
		match = strings.TrimRight(match, ".,;:!?)")
		if !seen[match] {
			seen[match] = true
			unique = append(unique, match)
		}
	}
	return unique
}

func isTruthSocialURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "truthsocial.com" || strings.HasSuffix(host, ".truthsocial.com")
}

func fetchExternalSocialPost(rawURL string) (*externalSocialPost, error) {
	fetchURL := rawURL
	parsed, err := url.Parse(rawURL)
	if err == nil && isXURL(parsed) {
		parsed.Host = nitterInstance
		parsed.Scheme = "https"
		fetchURL = parsed.String()
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fetchURL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	content := stripSocialHTML(string(body))
	if len(content) > 1000 {
		content = content[:1000] + "..."
	}

	handle := ""
	if parsed != nil && len(parsed.Path) > 1 {
		handle = strings.TrimPrefix(strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")[0], "@")
	}
	return &externalSocialPost{Author: handle, Content: content}, nil
}

func isXURL(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	return host == "twitter.com" || host == "www.twitter.com" ||
		host == "x.com" || host == "www.x.com" ||
		host == "mobile.twitter.com" || host == "mobile.x.com"
}

func stripSocialHTML(content string) string {
	tags := regexp.MustCompile(`<[^>]*>`)
	whitespace := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(whitespace.ReplaceAllString(tags.ReplaceAllString(content, " "), " "))
}

func renderSocialContextHTML(ctx *socialContext) string {
	if ctx == nil || len(ctx.Posts) == 0 {
		return ""
	}

	html := `<div class="social-context" style="margin:12px 0;">`
	for _, post := range ctx.Posts {
		content := post.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		html += fmt.Sprintf(`<blockquote style="border-left:3px solid #ccc;margin:8px 0;padding:8px 12px;background:#fafafa;border-radius:0 4px 4px 0;">
  <div style="font-size:12px;color:#666;margin-bottom:4px;"><b>@%s</b> · %s</div>
  <div style="font-size:13px;">%s</div>
  <a href="%s" target="_blank" rel="noopener noreferrer" style="font-size:12px;color:#888;">View original</a>
</blockquote>`, htmlpkg.EscapeString(post.Handle), htmlpkg.EscapeString(post.Platform), htmlpkg.EscapeString(content), htmlpkg.EscapeString(post.URL))
	}
	return html + `</div>`
}
