package deploy

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func writeTarGz(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	for name, body := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatalf("tar body %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

func noopLog(string, string) {}

func TestExtractZipWritesFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "app.zip")
	writeZip(t, archive, map[string]string{
		"bin/app":      "#!/bin/sh\n",
		"config/a.yml": "a: 1\n",
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "bin", "app"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(body) != "#!/bin/sh\n" {
		t.Fatalf("unexpected content: %q", body)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	writeZip(t, archive, map[string]string{
		"../escaped.txt": "should not exist",
		"ok.txt":         "fine",
	})

	dest := filepath.Join(dir, "nested", "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("zip-slip wrote outside the destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("zip-slip escaped one level up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "ok.txt")); err != nil {
		t.Fatalf("safe entry was not extracted: %v", err)
	}
}

func TestExtractSkipsMacOSMetadata(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "mac.zip")
	writeZip(t, archive, map[string]string{
		"__MACOSX/._app": "junk",
		"app/.DS_Store":  "junk",
		"app/main":       "real",
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "__MACOSX")); !os.IsNotExist(err) {
		t.Fatalf("__MACOSX should be dropped")
	}
	if _, err := os.Stat(filepath.Join(dest, "app", ".DS_Store")); !os.IsNotExist(err) {
		t.Fatalf(".DS_Store should be dropped")
	}
	if _, err := os.Stat(filepath.Join(dest, "app", "main")); err != nil {
		t.Fatalf("real file missing: %v", err)
	}
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "app.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"app/hello.txt": "hello",
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "app", "hello.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected content: %q", body)
	}
}

func TestExtractEnforcesSizeLimit(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "big.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"blob.bin": string(make([]byte, 2048)),
	})

	err := Extract(archive, filepath.Join(dir, "out"), 1024, noopLog)
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestExtractSingleBinary(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "myapp")
	if err := os.WriteFile(archive, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "myapp"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows has no executable bit; the mode is only meaningful on Unix.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("bare artifact should be executable, got %v", info.Mode())
	}
}

func TestArchiveRootUnwrapsSingleFolder(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "app.zip")
	writeZip(t, archive, map[string]string{
		"app-1.2.3/bin/app": "x",
		"app-1.2.3/README":  "y",
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	root := archiveRoot(dest)
	if filepath.Base(root) != "app-1.2.3" {
		t.Fatalf("expected the single top-level folder to be unwrapped, got %s", root)
	}
}

func TestArchiveRootKeepsFlatLayout(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "app.zip")
	writeZip(t, archive, map[string]string{
		"bin/app": "x",
		"conf/a":  "y",
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, 0, noopLog); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if root := archiveRoot(dest); root != dest {
		t.Fatalf("flat archive should not be unwrapped, got %s", root)
	}
}
