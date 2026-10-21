package deploy

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxExtractBytes = int64(8) << 30 // 8 GiB uncompressed ceiling
	skippedMacOSX          = "__MACOSX"
)

// Extract unpacks archivePath into destDir. It supports zip, tar, tar.gz and a bare
// single-file artifact (a plain binary). Archive entries are validated against zip-slip.
func Extract(archivePath, destDir string, maxBytes int64, logf Logger) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxExtractBytes
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	kind, err := detectArchiveKind(archivePath)
	if err != nil {
		return err
	}

	switch kind {
	case kindZip:
		return extractZip(archivePath, destDir, maxBytes, logf)
	case kindTarGz:
		return extractTar(archivePath, destDir, maxBytes, true, logf)
	case kindTar:
		return extractTar(archivePath, destDir, maxBytes, false, logf)
	default:
		return extractSingleFile(archivePath, destDir, logf)
	}
}

type archiveKind int

const (
	kindSingle archiveKind = iota
	kindZip
	kindTarGz
	kindTar
)

func detectArchiveKind(path string) (archiveKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return kindSingle, err
	}
	defer f.Close()

	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if len(head) >= 4 && head[0] == 'P' && head[1] == 'K' && (head[2] == 3 || head[2] == 5) {
		return kindZip, nil
	}
	if len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		return kindTarGz, nil
	}

	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar"):
		return kindTar, nil
	case strings.HasSuffix(lower, ".zip"):
		return kindZip, nil
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return kindTarGz, nil
	}
	return kindSingle, nil
}

func extractZip(path, destDir string, maxBytes int64, logf Logger) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	var total int64
	for _, file := range reader.File {
		name := cleanArchiveName(file.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(destDir, name)
		if err != nil {
			logf("error", fmt.Sprintf("skip unsafe zip entry %q: %v", file.Name, err))
			continue
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !file.Mode().IsRegular() {
			logf("info", fmt.Sprintf("skip non-regular zip entry %q", file.Name))
			continue
		}
		total += int64(file.UncompressedSize64)
		if total > maxBytes {
			return fmt.Errorf("archive exceeds uncompressed size limit (%d bytes)", maxBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipFile(file, target); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	}
	return nil
}

func writeZipFile(file *zip.File, target string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func extractTar(path, destDir string, maxBytes int64, gzipped bool, logf Logger) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var reader io.Reader = f
	if gzipped {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("open gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	tr := tar.NewReader(reader)
	var total int64
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		name := cleanArchiveName(header.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(destDir, name)
		if err != nil {
			logf("error", fmt.Sprintf("skip unsafe tar entry %q: %v", header.Name, err))
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if total > maxBytes {
				return fmt.Errorf("archive exceeds uncompressed size limit (%d bytes)", maxBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			logf("info", fmt.Sprintf("skip tar entry %q (type %d)", header.Name, header.Typeflag))
		}
	}
}

func extractSingleFile(path, destDir string, logf Logger) error {
	// A bare artifact keeps its published file name.
	name := filepath.Base(path)
	target := filepath.Join(destDir, name)
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	logf("info", fmt.Sprintf("artifact is a single file, installed as %s", name))
	return nil
}

// cleanArchiveName normalises an entry name and drops macOS metadata.
func cleanArchiveName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." {
		return ""
	}
	parts := strings.Split(name, "/")
	if parts[0] == skippedMacOSX {
		return ""
	}
	for _, part := range parts {
		if part == ".DS_Store" || strings.HasPrefix(part, "._") {
			return ""
		}
	}
	return filepath.FromSlash(name)
}

// safeJoin joins name onto root and refuses anything that escapes root.
func safeJoin(root, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes destination")
	}
	joined := filepath.Join(root, clean)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if absJoined != absRoot && !strings.HasPrefix(absJoined, absRoot+string(filepath.Separator)) {
		return "", errors.New("path escapes destination")
	}
	return absJoined, nil
}

// archiveRoot returns the directory that should be mirrored into the deploy dir.
// Archives that wrap everything in a single top-level folder are unwrapped.
func archiveRoot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return dir
	}
	inner := filepath.Join(dir, entries[0].Name())
	if innerEntries, err := os.ReadDir(inner); err == nil && len(innerEntries) > 0 {
		return inner
	}
	return dir
}
