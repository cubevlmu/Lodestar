package deploy

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func failingCommand() string {
	if runtime.GOOS == "windows" {
		return "exit 1"
	}
	return "exit 1"
}

func succeedingCommand() string {
	if runtime.GOOS == "windows" {
		return "echo started"
	}
	return "echo started"
}

func TestRunFreshDeployInstallsArtifact(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "app-1.0.0.zip")
	writeZip(t, archive, map[string]string{
		"hello.txt":       "v1",
		"data/config.yml": "default: true",
	})

	spec := Spec{
		AppID:        "demo",
		Version:      "1.0.0",
		ArchivePath:  archive,
		TargetDir:    filepath.Join(dir, "app"),
		BackupDir:    filepath.Join(dir, "backups"),
		WorkDir:      filepath.Join(dir, "work"),
		Preserve:     []string{"data"},
		BackupKeep:   5,
		StartCommand: succeedingCommand(),
		Timeout:      30 * time.Second,
	}

	result, err := Run(context.Background(), spec, noopLog)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.BackupPath != "" {
		t.Fatalf("a fresh install must not create a backup, got %s", result.BackupPath)
	}
	if got := readFile(t, filepath.Join(spec.TargetDir, "hello.txt")); got != "v1" {
		t.Fatalf("artifact was not installed: %q", got)
	}
	// Preserved defaults must land on a fresh install.
	if got := readFile(t, filepath.Join(spec.TargetDir, "data", "config.yml")); got != "default: true" {
		t.Fatalf("preserved defaults missing on fresh install: %q", got)
	}
}

func TestRunRollsBackWhenStartCommandFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	writeFile(t, filepath.Join(target, "hello.txt"), "v1")
	writeFile(t, filepath.Join(target, "data", "state.db"), "runtime")

	archive := filepath.Join(dir, "app-2.0.0.zip")
	writeZip(t, archive, map[string]string{
		"hello.txt": "v2",
	})

	spec := Spec{
		AppID:          "demo",
		Version:        "2.0.0",
		ArchivePath:    archive,
		TargetDir:      target,
		BackupDir:      filepath.Join(dir, "backups"),
		WorkDir:        filepath.Join(dir, "work"),
		Preserve:       []string{"data"},
		BackupKeep:     5,
		RestartCommand: failingCommand(),
		Timeout:        30 * time.Second,
	}

	result, err := Run(context.Background(), spec, noopLog)
	if err == nil {
		t.Fatal("expected the deploy to fail")
	}
	if !result.RolledBack {
		t.Fatal("expected an automatic rollback")
	}
	if result.BackupPath == "" {
		t.Fatal("expected a backup to have been created")
	}
	if got := readFile(t, filepath.Join(target, "hello.txt")); got != "v1" {
		t.Fatalf("rollback did not restore v1, got %q", got)
	}
	if got := readFile(t, filepath.Join(target, "data", "state.db")); got != "runtime" {
		t.Fatalf("runtime data was lost during rollback: %q", got)
	}
}

func TestRunRejectsEmptyArtifact(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "empty.zip")
	writeZip(t, archive, map[string]string{})

	spec := Spec{
		AppID:       "demo",
		Version:     "1.0.0",
		ArchivePath: archive,
		TargetDir:   filepath.Join(dir, "app"),
		BackupDir:   filepath.Join(dir, "backups"),
		WorkDir:     filepath.Join(dir, "work"),
		Timeout:     30 * time.Second,
	}
	if _, err := Run(context.Background(), spec, noopLog); err == nil {
		t.Fatal("expected an error for an empty artifact")
	}
	if _, err := os.Stat(spec.TargetDir); !os.IsNotExist(err) {
		t.Fatalf("nothing should have been installed")
	}
}

func TestRollbackRestoresLatestBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	backups := filepath.Join(dir, "backups")
	writeFile(t, filepath.Join(target, "hello.txt"), "v2")

	backupPath, err := Backup(target, backups, "demo-2.0.0", nil, noopLog)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	writeFile(t, filepath.Join(target, "hello.txt"), "v3-broken")

	spec := Spec{
		AppID:        "demo",
		TargetDir:    target,
		BackupDir:    backups,
		WorkDir:      filepath.Join(dir, "work"),
		StartCommand: succeedingCommand(),
		Timeout:      30 * time.Second,
	}
	result, err := Rollback(context.Background(), spec, noopLog)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if result.BackupPath != backupPath {
		t.Fatalf("expected rollback from %s, got %s", backupPath, result.BackupPath)
	}
	if got := readFile(t, filepath.Join(target, "hello.txt")); got != "v2" {
		t.Fatalf("expected v2 after rollback, got %q", got)
	}
}

func TestRollbackWithoutBackupFails(t *testing.T) {
	dir := t.TempDir()
	spec := Spec{
		AppID:     "demo",
		TargetDir: filepath.Join(dir, "app"),
		BackupDir: filepath.Join(dir, "backups"),
		WorkDir:   filepath.Join(dir, "work"),
		Timeout:   10 * time.Second,
	}
	if _, err := Rollback(context.Background(), spec, noopLog); err == nil {
		t.Fatal("expected rollback to fail without a backup")
	}
}

func TestRunRequiresTargetDir(t *testing.T) {
	spec := Spec{AppID: "demo", Version: "1.0.0", WorkDir: t.TempDir()}
	if _, err := Run(context.Background(), spec, noopLog); err == nil {
		t.Fatal("expected an error when deploy_dir is empty")
	}
}

// A fresh install has no running service, so a configured stop command must not
// be executed — it used to fail with "chdir <dir>: no such file".
func TestRunSkipsStopCommandOnFreshInstall(t *testing.T) {
	target := filepath.Join(t.TempDir(), "not-yet-there")
	archive := filepath.Join(t.TempDir(), "artifact.zip")
	writeZip(t, archive, map[string]string{"app.txt": "v1"})

	spec := Spec{
		JobID:       1,
		AppID:       "demo.app",
		Version:     "1.0.0",
		VersionCode: 1000000,
		ArchivePath: archive,
		TargetDir:   target,
		BackupDir:   filepath.Join(t.TempDir(), "backups"),
		// `exit 7` would fail the job if it ran.
		StopCommand: "exit 7",
		Timeout:     time.Minute,
	}

	if _, err := Run(context.Background(), spec, noopLog); err != nil {
		t.Fatalf("stop command must not run on a fresh install: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "app.txt")); got != "v1" {
		t.Fatalf("artifact should still be installed, got %q", got)
	}
}
