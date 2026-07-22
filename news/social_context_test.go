package news

import (
	"strings"
	"testing"
)

func TestDetectSocialURLsIncludesSupportedHostsAndDeduplicates(t *testing.T) {
	content := strings.Join([]string{
		"source https://mobile.twitter.com/mu/status/1,",
		"mirror https://mobile.twitter.com/mu/status/1",
		"x post https://www.x.com/mu/status/2)",
		"truth https://truthsocial.com/@mu/posts/3.",
		"ignore https://example.com/mu/status/4",
	}, " ")

	got := detectSocialURLs(content)
	want := []string{
		"https://mobile.twitter.com/mu/status/1",
		"https://www.x.com/mu/status/2",
		"https://truthsocial.com/@mu/posts/3",
	}
	if len(got) != len(want) {
		t.Fatalf("detectSocialURLs() returned %d URLs, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("detectSocialURLs()[%d] = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}
}

func TestIsTruthSocialURLRecognizesOnlyTruthSocialHosts(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "https://truthsocial.com/@mu/posts/123", want: true},
		{url: "https://www.truthsocial.com/@mu/posts/123", want: true},
		{url: "http://media.truthsocial.com/@mu/posts/123", want: true},
		{url: "https://nottruthsocial.com/@mu/posts/123", want: false},
		{url: "://truthsocial.com/@mu", want: false},
	}

	for _, tt := range tests {
		if got := isTruthSocialURL(tt.url); got != tt.want {
			t.Errorf("isTruthSocialURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestFetchSocialContextSkipsArticlesWithoutURL(t *testing.T) {
	originalFetch := fetchExternalSocialPostForContext
	fetchExternalSocialPostForContext = func(string) (*externalSocialPost, error) {
		t.Fatal("fetchExternalSocialPost must not be called for an article without a URL")
		return nil, nil
	}
	t.Cleanup(func() { fetchExternalSocialPostForContext = originalFetch })

	ctx := fetchSocialContext("", "Referenced post: https://x.com/mu/status/1")
	if ctx != nil {
		t.Fatalf("fetchSocialContext() = %#v, want nil", ctx)
	}
}

func TestRenderSocialContextHTMLEscapesPostContent(t *testing.T) {
	html := renderSocialContextHTML(&socialContext{Posts: []socialContextPost{{
		Handle:   `alice"><script>alert(1)</script>`,
		Platform: `<script>alert(1)</script>`,
		Content:  `<img src=x onerror=alert(1)>`,
		URL:      `https://x.com/alice/status/1?x="onmouseover=alert(1)`,
	}}})

	for _, unsafe := range []string{"<script>", "<img", `"onmouseover`} {
		if strings.Contains(html, unsafe) {
			t.Errorf("renderSocialContextHTML() contains unescaped %q: %s", unsafe, html)
		}
	}
	if !strings.Contains(html, "social-context") {
		t.Errorf("renderSocialContextHTML() = %q, want context markup", html)
	}
}
