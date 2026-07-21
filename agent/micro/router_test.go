package micro

import (
	"reflect"
	"testing"
)

func TestMarketsCapabilityIsNotRegistered(t *testing.T) {
	if agent := Get("markets"); agent != nil {
		t.Fatalf("removed Markets agent is still registered: %#v", agent)
	}
	for _, agent := range All() {
		if agent.ID == "markets" {
			t.Fatalf("All() exposed removed Markets agent: %#v", agent)
		}
	}
}

func TestKeywordRouteDoesNotSpecialCaseMarketQuestions(t *testing.T) {
	if got := keywordRoute("what is the BTC price?"); len(got) != 0 {
		t.Fatalf("keywordRoute() = %v, want generic routing", got)
	}
}

func TestRouteDirectAddressAvoidsLLM(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{name: "at mention", prompt: "@weather what is the forecast?", want: []string{"weather"}},
		{name: "at mention with punctuation", prompt: "@weather, what is the forecast?", want: []string{"weather"}},
		{name: "at mention with leading whitespace", prompt: "  @weather what is the forecast?", want: []string{"weather"}},
		{
			name:   "ask the agent",
			prompt: "ask the weather agent about Lisbon tomorrow",
			want:   []string{"weather"},
		},
		{
			name:   "use agent",
			prompt: "use mail to summarize unread messages",
			want:   []string{"mail"},
		},
		{
			name:   "github mention",
			prompt: "@github show micro/mu",
			want:   []string{"github"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Route(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Route(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestGitHubAgentRegistration(t *testing.T) {
	a := Get("github")
	if a == nil {
		t.Fatal("github agent is not registered")
	}
	want := []string{"github_repositories", "github_repository", "github_search", "github_issue"}
	if !reflect.DeepEqual(a.Tools, want) {
		t.Fatalf("Tools = %v, want %v", a.Tools, want)
	}
}

func TestKeywordRouteGitHub(t *testing.T) {
	for _, prompt := range []string{
		"show GitHub issues for micro/mu",
		"open pull requests in micro/mu",
		"find the repository micro/mu",
	} {
		if got := keywordRoute(prompt); !reflect.DeepEqual(got, []string{"github"}) {
			t.Fatalf("keywordRoute(%q) = %v, want [github]", prompt, got)
		}
	}
}

func TestKeywordRouteGitHubRepositoryCoordinates(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{name: "coordinate after in", prompt: "show issues in micro/mu", want: []string{"github"}},
		{name: "coordinate after for", prompt: "show issues for octo-org/hello_world", want: []string{"github"}},
		{name: "coordinate after on", prompt: "show issues on team/repo-2", want: []string{"github"}},
		{name: "coordinate with trailing punctuation", prompt: "show issues in micro/mu.", want: []string{"github"}},
		{name: "ci cd", prompt: "CI/CD issues", want: nil},
		{name: "tcp ip", prompt: "TCP/IP issues", want: nil},
		{name: "read write", prompt: "read/write issues", want: nil},
		{name: "and or", prompt: "and/or issues", want: nil},
		{name: "not applicable", prompt: "N/A issue", want: nil},
		{name: "date", prompt: "06/13 issue", want: nil},
		{name: "always", prompt: "24/7 issues", want: nil},
		{name: "too many segments", prompt: "show issues in micro/mu/extra", want: nil},
		{name: "missing owner", prompt: "show issues in /mu", want: nil},
		{name: "missing repository", prompt: "show issues in micro/", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keywordRoute(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keywordRoute(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestStripAddress(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "at mention",
			prompt: "@weather what is the forecast?",
			want:   "what is the forecast?",
		},
		{
			name:   "ask agent about",
			prompt: "ask the weather agent about Lisbon tomorrow",
			want:   "Lisbon tomorrow",
		},
		{
			name:   "at mention with leading whitespace",
			prompt: "  @weather what is the forecast?",
			want:   "what is the forecast?",
		},
		{
			name:   "use agent",
			prompt: "use mail summarize unread messages",
			want:   "summarize unread messages",
		},
		{
			name:   "unaddressed prompt",
			prompt: "summarize unread messages",
			want:   "summarize unread messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripAddress(tt.prompt); got != tt.want {
				t.Fatalf("StripAddress(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestKeywordRouteMultiSignalOrdering(t *testing.T) {
	got := keywordRoute("give me weather and news headlines")
	want := []string{"weather", "news"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keywordRoute() = %v, want %v", got, want)
	}
}

func TestKeywordRouteRequiresTermBoundaries(t *testing.T) {
	falsePositivePrompts := []string{
		"please postpone the team lunch",
		"this surprise party is busy",
		"watchtower status update",
	}

	for _, prompt := range falsePositivePrompts {
		t.Run(prompt, func(t *testing.T) {
			if got := keywordRoute(prompt); len(got) != 0 {
				t.Fatalf("keywordRoute(%q) = %v, want no keyword route", prompt, got)
			}
		})
	}
}

func TestAllExcludesFallbackAgent(t *testing.T) {
	for _, agent := range All() {
		if agent.ID == "micro" {
			t.Fatal("All() included the micro fallback agent")
		}
	}
}

func TestValidateAgentIDsDeduplicatesAndLimits(t *testing.T) {
	got := validateAgentIDs([]string{"bogus", "news", "news", "weather", "mail"})
	want := []string{"news", "weather", "mail"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validateAgentIDs() = %v, want %v", got, want)
	}
}

func TestValidateAgentIDsFallsBackToMicro(t *testing.T) {
	got := validateAgentIDs([]string{"bogus"})
	want := []string{"micro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validateAgentIDs() = %v, want %v", got, want)
	}
}

func TestKeywordRouteSingleDomainPriorityIsDeterministic(t *testing.T) {
	prompt := "summarize unread email about the team lunch restaurant"
	want := []string{"mail"}

	for i := 0; i < 100; i++ {
		if got := keywordRoute(prompt); !reflect.DeepEqual(got, want) {
			t.Fatalf("keywordRoute() iteration %d = %v, want %v", i, got, want)
		}
	}
}

func TestKeywordRouteCoreAskAnswerSmokeCoverage(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{name: "weather", prompt: "what's the weather in London?", want: []string{"weather"}},
		{name: "news", prompt: "what's happening in the news today?", want: []string{"news"}},
		{name: "mail", prompt: "do I have unread mail?", want: []string{"mail"}},
		{name: "search", prompt: "search the web for go-micro agents", want: []string{"search"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keywordRoute(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("keywordRoute(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}
