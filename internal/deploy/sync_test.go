package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func TestSyncInstallsNewFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "bin", "app"), "v2")
	writeFile(t, filepath.Join(src, "config", "default.yml"), "default")

	if err := Sync(src, dst, nil, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "bin", "app")); got != "v2" {
		t.Fatalf("expected v2, got %q", got)
	}
}

func TestSyncOverwritesChangedFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "app.txt"), "v2")
	writeFile(t, filepath.Join(dst, "app.txt"), "v1")

	if err := Sync(src, dst, nil, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "app.txt")); got != "v2" {
		t.Fatalf("expected the new version, got %q", got)
	}
}

func TestSyncRemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "keep.txt"), "keep")
	writeFile(t, filepath.Join(dst, "keep.txt"), "keep")
	writeFile(t, filepath.Join(dst, "removed-in-v2.txt"), "stale")
	writeFile(t, filepath.Join(dst, "olddir", "nested.txt"), "stale")

	if err := Sync(src, dst, nil, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "removed-in-v2.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file survived the sync")
	}
	if _, err := os.Stat(filepath.Join(dst, "olddir")); !os.IsNotExist(err) {
		t.Fatalf("stale directory survived the sync")
	}
	if _, err := os.Stat(filepath.Join(dst, "keep.txt")); err != nil {
		t.Fatalf("current file was removed: %v", err)
	}
}

func TestSyncPreservesRuntimeData(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "app"), "v2")
	writeFile(t, filepath.Join(src, "data", "config.json"), `{"from":"package"}`)
	writeFile(t, filepath.Join(dst, "app"), "v1")
	writeFile(t, filepath.Join(dst, "data", "config.json"), `{"from":"host"}`)
	writeFile(t, filepath.Join(dst, "data", "runtime.db"), "runtime")

	if err := Sync(src, dst, []string{"data"}, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "app")); got != "v2" {
		t.Fatalf("app was not updated: %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "data", "config.json")); !strings.Contains(got, "host") {
		t.Fatalf("preserved config was overwritten: %s", got)
	}
	if got := readFile(t, filepath.Join(dst, "data", "runtime.db")); got != "runtime" {
		t.Fatalf("runtime file inside a preserved dir was touched: %q", got)
	}
}

// On a fresh install there is nothing to preserve, so the artifact's defaults must
// still be installed. This was a real bug: the first deploy left data/ empty.
func TestSyncInstallsPreservedDefaultsOnFreshInstall(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "app"), "v1")
	writeFile(t, filepath.Join(src, "data", "config.json"), `{"from":"package"}`)

	if err := Sync(src, dst, []string{"data"}, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "data", "config.json")); !strings.Contains(got, "package") {
		t.Fatalf("expected package defaults on a fresh install, got %q", got)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	backups := filepath.Join(dir, "backups")
	writeFile(t, filepath.Join(target, "app"), "v1")
	writeFile(t, filepath.Join(target, "data", "state.db"), "state")

	backupPath, err := Backup(target, backups, "demo-1.0.0", nil, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	// Simulate a broken deploy.
	writeFile(t, filepath.Join(target, "app"), "v2-broken")
	if err := os.RemoveAll(filepath.Join(target, "data")); err != nil {
		t.Fatalf("rm data: %v", err)
	}

	if err := Restore(backupPath, target, nil, noopLog); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "app")); got != "v1" {
		t.Fatalf("expected v1 after restore, got %q", got)
	}
	if got := readFile(t, filepath.Join(target, "data", "state.db")); got != "state" {
		t.Fatalf("runtime data was not restored: %q", got)
	}
}

func TestLatestBackupPicksNewest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo-1.0.0-20260101-000000", "app"), "v1")
	writeFile(t, filepath.Join(dir, "demo-1.1.0-20260102-000000", "app"), "v2")
	writeFile(t, filepath.Join(dir, "other-9.9.9-20260103-000000", "app"), "x")

	latest, err := LatestBackup(dir, "demo")
	if err != nil {
		t.Fatalf("latest backup: %v", err)
	}
	if filepath.Base(latest) != "demo-1.1.0-20260102-000000" {
		t.Fatalf("unexpected latest backup: %s", latest)
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"demo-1.0.0-20260101-000000",
		"demo-1.1.0-20260102-000000",
		"demo-1.2.0-20260103-000000",
	} {
		writeFile(t, filepath.Join(dir, name, "app"), name)
	}
	PruneBackups(dir, 2, noopLog)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backups: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups to remain, got %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, "demo-1.2.0-20260103-000000")); err != nil {
		t.Fatalf("newest backup was pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo-1.0.0-20260101-000000")); !os.IsNotExist(err) {
		t.Fatalf("oldest backup should have been pruned")
	}
}
