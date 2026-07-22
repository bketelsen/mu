package main

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/wallet"
)

func TestIsServerMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no args", args: nil, want: false},
		{name: "cli command", args: []string{"news"}, want: false},
		{name: "long flag", args: []string{"--serve"}, want: true},
		{name: "short flag", args: []string{"-serve"}, want: true},
		{name: "long flag with value", args: []string{"--serve=false"}, want: true},
		{name: "short flag with value", args: []string{"-serve=true"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isServerMode(tt.args); got != tt.want {
				t.Fatalf("isServerMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPlacesRemovalRunsBeforeDataAndServices(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	ownerMigration := strings.Index(string(source), "if err := migrateSingleOwner(); err != nil")
	placesMigration := strings.Index(string(source), "if err := migration.RemovePlaces(); err != nil")
	dataLoad := strings.Index(string(source), "data.Load()")
	if ownerMigration < 0 || placesMigration < 0 || dataLoad < 0 || ownerMigration >= placesMigration || placesMigration >= dataLoad {
		t.Fatalf("startup order is owner=%d places=%d data=%d", ownerMigration, placesMigration, dataLoad)
	}
}

func TestAdminRoutesExcludeLocalUserManagement(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/admin/users", "/admin/moderate"} {
		if strings.Contains(string(source), route) {
			t.Errorf("main route registration retains %s", route)
		}
	}
	for _, route := range []string{"/admin/blocklist", "/admin/spam", "/admin/console", "/admin/delete"} {
		if !strings.Contains(string(source), route) {
			t.Errorf("main route registration is missing %s", route)
		}
	}
}

func TestRoutesExcludeProfilesFederationAndPresence(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		"/.well-known/webfinger",
		"/presence",
		"/ping",
		"strings.HasPrefix(r.URL.Path, \"/@\")",
	} {
		if strings.Contains(string(source), route) {
			t.Errorf("main route registration retains %s", route)
		}
	}
	if !strings.Contains(string(source), `http.HandleFunc("/user/status", user.StatusHandler)`) {
		t.Error("owner-private status route is missing")
	}
}

func TestExecutableSourcesExcludeProfileDiscovery(t *testing.T) {
	for _, file := range []string{
		"blog/blog.go",
		"mail/mail.go",
		"stream/handlers.go",
		"internal/app/app.go",
		"internal/app/ui.go",
		"internal/api/api.go",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "/@") {
			t.Errorf("%s retains executable profile discovery", file)
		}
	}
}

func TestVersionInfoDoesNotExposeServiceTopology(t *testing.T) {
	info := versionInfo()
	if _, ok := info["services"]; ok {
		t.Fatalf("public version exposes services: %#v", info)
	}
}

func TestChargedWriteOp(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "reads are free", method: "GET", path: "/social", want: ""},
		{name: "status post", method: "POST", path: "/user/status", want: wallet.OpSocialPost},
		{name: "social thread", method: "POST", path: "/social", want: wallet.OpSocialPost},
		{name: "social reply", method: "POST", path: "/social/thread", want: wallet.OpSocialReply},
		{name: "new blog post", method: "POST", path: "/blog", want: wallet.OpBlogCreate},
		{name: "blog update free", method: "POST", path: "/blog?id=post-1", want: ""},
		{name: "blog comment", method: "POST", path: "/blog/post/post-1/comment", want: wallet.OpBlogComment},
		{name: "app generation", method: "POST", path: "/apps/generate", want: wallet.OpAppBuild},
		{name: "uncharged post", method: "POST", path: "/mail", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			if got := chargedWriteOp(r); got != tt.want {
				t.Fatalf("chargedWriteOp(%s %s) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestArgFloat(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{name: "float", in: 1.25, want: 1.25},
		{name: "int", in: 2, want: 2},
		{name: "string", in: "3.5", want: 3.5},
		{name: "invalid string", in: "nope", want: 0},
		{name: "unsupported", in: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := argFloat(tt.in); got != tt.want {
				t.Fatalf("argFloat(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMigrateSingleOwnerUsesDataBackup(t *testing.T) {
	oldBackup := backupData
	oldRun := runOwnerMigration
	called := false
	backupData = func() (string, error) { called = true; return "/tmp/mu-backup", nil }
	runOwnerMigration = func(backup func() (string, error)) (auth.MigrationResult, error) {
		path, err := backup()
		return auth.MigrationResult{Migrated: true, BackupPath: path, OwnerID: "owner"}, err
	}
	t.Cleanup(func() { backupData, runOwnerMigration = oldBackup, oldRun })
	if err := migrateSingleOwner(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("startup migration did not invoke data backup")
	}
}
