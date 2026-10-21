package db

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodestar/internal/config"
	"lodestar/internal/models"

	"gorm.io/gorm"
)

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()

	fn()

	_ = writer.Close()
	return <-done
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		DBPath:     filepath.Join(dir, "test.db"),
		StorageDir: filepath.Join(dir, "storage"),
		DataDir:    dir,
	}
}

// closeDB releases the SQLite file handle so Windows can delete the temp dir.
func closeDB(t *testing.T, handle *gorm.DB) {
	t.Helper()
	sqlDB, err := handle.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// A missing row is normal here (session lookups, optional settings, nonce
// probes). GORM's default logger prints "record not found" for each of them,
// which is what used to flood the console with SQL.
func TestOpenDoesNotLogRecordNotFound(t *testing.T) {
	cfg := testConfig(t)

	var handle *gorm.DB
	output := captureStdout(t, func() {
		var err error
		handle, err = Open(cfg)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		var user models.User
		if err := handle.Where("username = ?", "nobody").First(&user).Error; err != nil {
			t.Logf("expected lookup miss: %v", err)
		}
	})
	closeDB(t, handle)

	if strings.Contains(output, "record not found") {
		t.Fatalf("lookup miss should not be logged, got:\n%s", output)
	}
	if strings.Contains(output, "SELECT") {
		t.Fatalf("statements should not be logged by default, got:\n%s", output)
	}
}

func TestSQLDebugLogsStatements(t *testing.T) {
	cfg := testConfig(t)
	cfg.SQLDebug = true

	var handle *gorm.DB
	output := captureStdout(t, func() {
		var err error
		handle, err = Open(cfg)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		var user models.User
		_ = handle.Where("username = ?", "nobody").First(&user).Error
	})
	closeDB(t, handle)

	if !strings.Contains(output, "SELECT") {
		t.Fatalf("debug mode should log statements, got:\n%s", output)
	}
	if strings.Contains(output, "record not found") {
		t.Fatalf("debug mode should still ignore record-not-found noise, got:\n%s", output)
	}
}
