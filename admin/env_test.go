package admin

import "testing"

func TestGitHubTokenSetting(t *testing.T) {
	count := 0
	for _, group := range settingGroups {
		if group.Name != "GitHub" {
			continue
		}
		count++
		if len(group.Vars) != 1 || group.Vars[0] != "GITHUB_TOKEN" {
			t.Fatalf("GitHub settings = %v, want [GITHUB_TOKEN]", group.Vars)
		}
	}
	if count != 1 {
		t.Fatalf("GitHub setting groups = %d, want 1", count)
	}
	if !isSecretSetting("GITHUB_TOKEN") {
		t.Fatal("GITHUB_TOKEN should be treated as secret")
	}
}
