package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Runtime data must never travel with a release: a backup that contains the
// database would resurrect stale state when rolled back.
func TestBackupSkipsWhitelistedPaths(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "app.txt"), "v1")
	writeFile(t, filepath.Join(target, "data", "db.sqlite"), "rows")
	writeFile(t, filepath.Join(target, "uploads", "a.png"), "png")
	writeFile(t, filepath.Join(target, "config", "app.yaml"), "cfg")

	backups := t.TempDir()
	backupPath, err := Backup(target, backups, "demo-1.0.0", []string{"data", "uploads"}, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if !exists(filepath.Join(backupPath, "app.txt")) {
		t.Fatal("regular file should be in the backup")
	}
	if exists(filepath.Join(backupPath, "data")) {
		t.Fatal("whitelisted data/ must not be copied into the backup")
	}
	if exists(filepath.Join(backupPath, "uploads")) {
		t.Fatal("whitelisted uploads/ must not be copied into the backup")
	}
	if !exists(filepath.Join(backupPath, "config", "app.yaml")) {
		t.Fatal("paths outside the whitelist are backed up normally")
	}
}

// Restoring a backup replaces the tree, but the whitelisted entries from the
// tree being replaced must survive — otherwise a rollback wipes the database.
func TestRestoreKeepsWhitelistedRuntimeData(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "app.txt"), "v1")
	writeFile(t, filepath.Join(target, "data", "db.sqlite"), "old-rows")

	backups := t.TempDir()
	backupPath, err := Backup(target, backups, "demo-1.0.0", []string{"data"}, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Simulate a later version writing new runtime data, then failing.
	writeFile(t, filepath.Join(target, "app.txt"), "v2")
	writeFile(t, filepath.Join(target, "data", "db.sqlite"), "new-rows")

	if err := Restore(backupPath, target, []string{"data"}, noopLog); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if got := readFile(t, filepath.Join(target, "app.txt")); got != "v1" {
		t.Fatalf("binary should roll back, got %q", got)
	}
	if got := readFile(t, filepath.Join(target, "data", "db.sqlite")); got != "new-rows" {
		t.Fatalf("runtime data must survive the rollback, got %q", got)
	}
}

// A nested whitelist entry keeps working when only the parent directory is
// listed, and unrelated siblings are still restored.
func TestRestoreHandlesNestedWhitelist(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "app.txt"), "v1")
	writeFile(t, filepath.Join(target, "storage", "logs", "run.log"), "old-log")

	backups := t.TempDir()
	backupPath, err := Backup(target, backups, "demo-1.0.0", []string{"storage/logs"}, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if exists(filepath.Join(backupPath, "storage", "logs")) {
		t.Fatal("nested whitelist entry should be excluded from the backup")
	}

	writeFile(t, filepath.Join(target, "storage", "logs", "run.log"), "new-log")
	if err := Restore(backupPath, target, []string{"storage/logs"}, noopLog); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "storage", "logs", "run.log")); got != "new-log" {
		t.Fatalf("nested runtime data should survive, got %q", got)
	}
}

// With no whitelist the old behaviour is unchanged: the backup is a full copy.
func TestBackupWithoutWhitelistCopiesEverything(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "app.txt"), "v1")
	writeFile(t, filepath.Join(target, "data", "db.sqlite"), "rows")

	backupPath, err := Backup(target, t.TempDir(), "demo-1.0.0", nil, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !exists(filepath.Join(backupPath, "data", "db.sqlite")) {
		t.Fatal("without a whitelist everything should be backed up")
	}
}

// A whitelisted entry can be a single file: appsettings.json must survive an
// update even when the package ships its own copy.
func TestSyncKeepsWhitelistedFile(t *testing.T) {
	artifact := t.TempDir()
	writeFile(t, filepath.Join(artifact, "appsettings.json"), `{"from":"package"}`)
	writeFile(t, filepath.Join(artifact, "app.exe"), "v2")

	target := t.TempDir()
	writeFile(t, filepath.Join(target, "appsettings.json"), `{"from":"host"}`)
	writeFile(t, filepath.Join(target, "app.exe"), "v1")

	if err := Sync(artifact, target, []string{"appsettings.json"}, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "appsettings.json")); got != `{"from":"host"}` {
		t.Fatalf("whitelisted file must not be overwritten, got %q", got)
	}
	if got := readFile(t, filepath.Join(target, "app.exe")); got != "v2" {
		t.Fatalf("other files should update, got %q", got)
	}
}

// On a fresh install there is nothing to preserve, so the package default wins.
func TestSyncInstallsWhitelistedFileOnFirstDeploy(t *testing.T) {
	artifact := t.TempDir()
	writeFile(t, filepath.Join(artifact, "appsettings.json"), `{"from":"package"}`)
	target := t.TempDir()

	if err := Sync(artifact, target, []string{"appsettings.json"}, noopLog); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "appsettings.json")); got != `{"from":"package"}` {
		t.Fatalf("first deploy should install the default, got %q", got)
	}
}

func TestBackupSkipsWhitelistedFile(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "appsettings.json"), `{"from":"host"}`)
	writeFile(t, filepath.Join(target, "app.txt"), "v1")

	backupPath, err := Backup(target, t.TempDir(), "demo-1.0.0", []string{"appsettings.json"}, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if exists(filepath.Join(backupPath, "appsettings.json")) {
		t.Fatal("whitelisted file must not be in the backup")
	}
	if !exists(filepath.Join(backupPath, "app.txt")) {
		t.Fatal("other files should still be backed up")
	}
}

// Rolling back must not resurrect the config file from the backup: the live one
// on disk wins.
func TestRestoreKeepsWhitelistedFile(t *testing.T) {
	target := t.TempDir()
	writeFile(t, filepath.Join(target, "appsettings.json"), `{"generation":"v1"}`)
	writeFile(t, filepath.Join(target, "app.txt"), "v1")

	backupPath, err := Backup(target, t.TempDir(), "demo-1.0.0", []string{"appsettings.json"}, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	writeFile(t, filepath.Join(target, "appsettings.json"), `{"generation":"v2"}`)
	writeFile(t, filepath.Join(target, "app.txt"), "v2")

	if err := Restore(backupPath, target, []string{"appsettings.json"}, noopLog); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "appsettings.json")); got != `{"generation":"v2"}` {
		t.Fatalf("live config must survive a rollback, got %q", got)
	}
	if got := readFile(t, filepath.Join(target, "app.txt")); got != "v1" {
		t.Fatalf("binary should roll back, got %q", got)
	}
}
