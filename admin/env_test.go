package admin

import (
	"strings"
	"testing"
)

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

func TestSettingGroupsExcludePayments(t *testing.T) {
	for _, group := range settingGroups {
		for _, key := range group.Vars {
			upper := strings.ToUpper(key)
			if strings.HasPrefix(upper, "STRIPE_") || strings.HasPrefix(upper, "X402_") ||
				strings.HasPrefix(upper, "CREDIT_COST_") || upper == "DAILY_QUOTA" ||
				upper == "FREE_DAILY_QUOTA" || upper == "TRADE_RPC_URL" || upper == "TRADE_CHAIN" {
				t.Errorf("payment setting remains: %s", key)
			}
		}
	}
}
