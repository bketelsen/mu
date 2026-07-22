package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/data"
)

func TestLoadTopicConfigurationReturnsError(t *testing.T) {
	want := errors.New("invalid topic configuration")
	oldLoadTopics := loadTopics
	loadTopics = func() error { return want }
	t.Cleanup(func() { loadTopics = oldLoadTopics })

	if err := loadTopicConfiguration(); err != want {
		t.Fatalf("loadTopicConfiguration() error = %v, want %v", err, want)
	}
}

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

func TestRetiredServiceRemovalRunsBeforeDataAndServices(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	ownerMigration := strings.Index(string(source), "if err := migrateSingleOwner(); err != nil")
	socialMigration := strings.Index(string(source), "if err := migrateRemoveSocial(); err != nil")
	placesMigration := strings.Index(string(source), "if err := migration.RemovePlaces(); err != nil")
	serviceStartup := strings.Index(string(source), "app.Load()")
	if ownerMigration < 0 || socialMigration < 0 || placesMigration < 0 || serviceStartup < 0 || ownerMigration >= socialMigration || socialMigration >= placesMigration || placesMigration >= serviceStartup {
		t.Fatalf("startup order is owner=%d social=%d places=%d services=%d", ownerMigration, socialMigration, placesMigration, serviceStartup)
	}
}

func TestRetiredServiceMigrationLogIsGeneric(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `app.Log("migration", "retired service migration failed: %v", err)`) {
		t.Fatal("retired-service migration log is not generic")
	}
}

func TestExecutableExcludesRetiredLocationRuntime(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{
		`"mu/pla` + `ces"`,
		"pla" + "ces.Load()",
		`http.HandleFunc("/pla` + `ces`,
	} {
		if strings.Contains(string(source), removed) {
			t.Errorf("main.go retains retired location runtime wiring %q", removed)
		}
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
		"/user/status",
		"/admin/flag",
	} {
		if strings.Contains(string(source), route) {
			t.Errorf("main route registration retains %s", route)
		}
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

func TestMigrateWalletPaymentsPropagatesFailure(t *testing.T) {
	original := runWalletPaymentsMigration
	defer func() { runWalletPaymentsMigration = original }()
	want := errors.New("cannot delete seed")
	runWalletPaymentsMigration = func() error { return want }
	if err := migrateWalletPayments(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
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

func TestMigrateRemoveSocialBacksUpBeforeLoadingAndCleansUp(t *testing.T) {
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "/backup", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(key string) error { calls = append(calls, "delete "+key); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	if err := migrateRemoveSocial(); err != nil {
		t.Fatalf("migrateRemoveSocial: %v", err)
	}
	want := []string{"backup", "load index", "delete index", "delete social.json", "delete social_posts.json", "delete profiles.json", "delete flags.json", "remove card", "remove card", "save marker"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestMigrateRemoveSocialPurgesPreferencesBeforeSavingMarker(t *testing.T) {
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "/backup", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(key string) error { calls = append(calls, "delete "+key); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)
	oldRemovePrefs := removeSocialPrefs
	removeSocialPrefs = func() error { calls = append(calls, "remove preferences"); return nil }
	t.Cleanup(func() { removeSocialPrefs = oldRemovePrefs })

	if err := migrateRemoveSocial(); err != nil {
		t.Fatalf("migrateRemoveSocial: %v", err)
	}
	want := []string{"backup", "load index", "delete index", "delete social.json", "delete social_posts.json", "delete profiles.json", "delete flags.json", "remove card", "remove card", "remove preferences", "save marker"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestMigrateRemoveSocialDoesNotSaveMarkerWhenPreferencePurgeFails(t *testing.T) {
	wantErr := errors.New("preferences write failed")
	markerSaves := 0
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { return "/backup", nil },
		func() {},
		func(string) error { return nil },
		func(string) error { return nil },
		func(string) error { return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { markerSaves++; return nil },
	)
	oldRemovePrefs := removeSocialPrefs
	removeSocialPrefs = func() error { return wantErr }
	t.Cleanup(func() { removeSocialPrefs = oldRemovePrefs })

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	if markerSaves != 0 {
		t.Fatalf("marker save attempts = %d, want 0", markerSaves)
	}
}

func TestMigrateRemoveSocialLoadsIndexWhenCompleted(t *testing.T) {
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "/backup", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(string) error { calls = append(calls, "delete file"); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(marker *map[string]int) error {
			*marker = map[string]int{"version": removeSocialMigrationVersion}
			return nil
		},
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	if err := migrateRemoveSocial(); err != nil {
		t.Fatalf("migrateRemoveSocial: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"load index"}) {
		t.Fatalf("calls = %v, want only index load", calls)
	}
}

func TestMigrateRemoveSocialTreatsMissingSocialFilesAsSuccess(t *testing.T) {
	markerSaved := false
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { return "/backup", nil },
		func() {},
		func(string) error { return nil },
		func(string) error { return os.ErrNotExist },
		func(string) error { return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { markerSaved = true; return nil },
	)

	if err := migrateRemoveSocial(); err != nil {
		t.Fatalf("migrateRemoveSocial: %v", err)
	}
	if !markerSaved {
		t.Fatal("migration did not save completion marker")
	}
}

func TestMigrateRemoveSocialAbortsOnCorruptMarker(t *testing.T) {
	wantErr := errors.New("corrupt marker")
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(string) error { calls = append(calls, "delete file"); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(*map[string]int) error { return wantErr },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	if len(calls) != 0 {
		t.Fatalf("calls after corrupt marker = %v, want none", calls)
	}
}

func TestMigrateRemoveSocialAbortsOnBackupFailure(t *testing.T) {
	wantErr := errors.New("backup failed")
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "", wantErr },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(string) error { calls = append(calls, "delete file"); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(calls, []string{"backup"}) {
		t.Fatalf("calls after backup failure = %v, want only backup", calls)
	}
}

func TestMigrateRemoveSocialAbortsOnIndexPurgeFailure(t *testing.T) {
	wantErr := errors.New("index purge failed")
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "/backup", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return wantErr },
		func(string) error { calls = append(calls, "delete file"); return nil },
		func(string) error { calls = append(calls, "remove card"); return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(calls, []string{"backup", "load index", "delete index"}) {
		t.Fatalf("calls after index purge failure = %v", calls)
	}
}

func TestMigrateRemoveSocialAbortsOnHomeCardCleanupFailure(t *testing.T) {
	wantErr := errors.New("account write failed")
	var calls []string
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { calls = append(calls, "backup"); return "/backup", nil },
		func() { calls = append(calls, "load index") },
		func(string) error { calls = append(calls, "delete index"); return nil },
		func(key string) error { calls = append(calls, "delete "+key); return nil },
		func(string) error { calls = append(calls, "remove card"); return wantErr },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { calls = append(calls, "save marker"); return nil },
	)

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	want := []string{"backup", "load index", "delete index", "delete social.json", "delete social_posts.json", "delete profiles.json", "delete flags.json", "remove card"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after home card failure = %v, want %v", calls, want)
	}
}

func TestMigrateRemoveSocialReturnsMarkerSaveFailure(t *testing.T) {
	wantErr := errors.New("marker write failed")
	markerSaves := 0
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { return "/backup", nil },
		func() {},
		func(string) error { return nil },
		func(string) error { return nil },
		func(string) error { return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { markerSaves++; return wantErr },
	)

	err := migrateRemoveSocial()
	if !errors.Is(err, wantErr) {
		t.Fatalf("migrateRemoveSocial error = %v, want %v", err, wantErr)
	}
	if markerSaves != 1 {
		t.Fatalf("marker save attempts = %d, want 1", markerSaves)
	}
}

func TestMigrateRemoveSocialRetriesAfterCleanupFailure(t *testing.T) {
	var backups, loads, indexDeletes, fileDeletes, cardRemovals, markerSaves int
	fail := true
	setRemoveSocialMigrationSeams(t,
		func() (string, error) { backups++; return "/backup", nil },
		func() { loads++ },
		func(string) error { indexDeletes++; return nil },
		func(key string) error {
			fileDeletes++
			if key == "social_posts.json" && fail {
				return errors.New("disk failure")
			}
			return nil
		},
		func(string) error { cardRemovals++; return nil },
		func(*map[string]int) error { return os.ErrNotExist },
		func(map[string]int) error { markerSaves++; return nil },
	)

	if err := migrateRemoveSocial(); err == nil {
		t.Fatal("expected cleanup failure")
	}
	if markerSaves != 0 || cardRemovals != 0 {
		t.Fatalf("failed migration saved marker=%d or removed cards=%d", markerSaves, cardRemovals)
	}
	fail = false
	if err := migrateRemoveSocial(); err != nil {
		t.Fatalf("retry migration: %v", err)
	}
	if backups != 2 || loads != 2 || indexDeletes != 2 || fileDeletes != 6 || cardRemovals != 2 || markerSaves != 1 {
		t.Fatalf("retry counts backup=%d load=%d index=%d files=%d cards=%d marker=%d", backups, loads, indexDeletes, fileDeletes, cardRemovals, markerSaves)
	}
}

func TestBackupSocialDataAcceptsBackupFromPartialRun(t *testing.T) {
	path, err := backupSocialData(func() (string, error) {
		return "", fmt.Errorf("retry: %w", data.ErrBackupAlreadyExists)
	})
	if err != nil || path != "" {
		t.Fatalf("backupSocialData = %q, %v; want success without a new path", path, err)
	}
}

func setRemoveSocialMigrationSeams(t *testing.T, backup func() (string, error), load func(), deleteIndex func(string) error, deleteFile func(string) error, removeCard func(string) error, loadMarker func(*map[string]int) error, saveMarker func(map[string]int) error) {
	t.Helper()
	oldBackup, oldLoad := backupRemoveSocialData, loadRemoveSocialIndex
	oldDeleteIndex, oldDeleteFile := deleteRemoveSocialIndexType, deleteRemoveSocialFile
	oldRemoveCard, oldRemovePrefs := removeSocialHomeCard, removeSocialPrefs
	oldLoadMarker, oldSaveMarker := loadRemoveSocialMarker, saveRemoveSocialMarker
	backupRemoveSocialData, loadRemoveSocialIndex = backup, load
	deleteRemoveSocialIndexType, deleteRemoveSocialFile = deleteIndex, deleteFile
	removeSocialHomeCard, removeSocialPrefs = removeCard, func() error { return nil }
	loadRemoveSocialMarker, saveRemoveSocialMarker = loadMarker, saveMarker
	t.Cleanup(func() {
		backupRemoveSocialData, loadRemoveSocialIndex = oldBackup, oldLoad
		deleteRemoveSocialIndexType, deleteRemoveSocialFile = oldDeleteIndex, oldDeleteFile
		removeSocialHomeCard, removeSocialPrefs = oldRemoveCard, oldRemovePrefs
		loadRemoveSocialMarker, saveRemoveSocialMarker = oldLoadMarker, oldSaveMarker
	})
}

func TestRequiresWritePermission(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "removed user status", method: http.MethodPost, path: "/user/status", want: false},
		{name: "removed social post", method: http.MethodPost, path: "/social", want: false},
		{name: "removed social thread", method: http.MethodPost, path: "/social/thread", want: false},
		{name: "blog create", method: http.MethodPost, path: "/blog", want: true},
		{name: "blog comment", method: http.MethodPost, path: "/blog/post/post-id/comment", want: true},
		{name: "new app", method: http.MethodPost, path: "/apps/new", want: true},
		{name: "generate app", method: http.MethodPost, path: "/apps/generate", want: true},
		{name: "stream post", method: http.MethodPost, path: "/stream", want: true},
		{name: "blog update", method: http.MethodPost, path: "/blog?id=post-id", want: false},
		{name: "status read", method: http.MethodGet, path: "/user/status", want: false},
		{name: "social read", method: http.MethodGet, path: "/social", want: false},
		{name: "unrelated post", method: http.MethodPost, path: "/news", want: false},
		{name: "comment-like path", method: http.MethodPost, path: "/blog/post/post-id/comments", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			if got := requiresWritePermission(r); got != tt.want {
				t.Fatalf("requiresWritePermission(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
