package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Spec is everything the pipeline needs to install one artifact on one host.
// Directories come from the agent's local config; commands come from the target
// record on the server (or from local config when the server does not send them).
type Spec struct {
	JobID           uint
	AppID           string
	Version         string
	VersionCode     int64
	ArchivePath     string
	TargetDir       string
	BackupDir       string
	WorkDir         string
	StopCommand     string
	StartCommand    string
	RestartCommand  string
	HealthCheckURL  string
	Preserve        []string
	BackupKeep      int
	MaxExtractBytes int64
	Timeout         time.Duration
}

// Result describes what the pipeline did, for reporting back to the server.
type Result struct {
	BackupPath string
	RolledBack bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// Run performs: extract -> stop -> backup -> mirror into place -> start -> health check.
// Any failure after the backup exists triggers an automatic restore of the previous version.
func Run(ctx context.Context, spec Spec, logf Logger) (Result, error) {
	result := Result{StartedAt: time.Now()}
	defer func() { result.FinishedAt = time.Now() }()
	if logf == nil {
		logf = func(string, string) {}
	}
	if err := normaliseSpec(&spec); err != nil {
		return result, err
	}
	if spec.Timeout <= 0 {
		spec.Timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	logf("info", fmt.Sprintf("lodestar deploy job=%d app=%s version=%s -> %s", spec.JobID, spec.AppID, spec.Version, spec.TargetDir))

	staging := filepath.Join(spec.WorkDir, fmt.Sprintf("staging-%d-%d", spec.JobID, time.Now().Unix()))
	if err := os.RemoveAll(staging); err != nil {
		return result, err
	}
	defer os.RemoveAll(staging)

	logf("info", "extracting "+filepath.Base(spec.ArchivePath))
	if err := Extract(spec.ArchivePath, staging, spec.MaxExtractBytes, logf); err != nil {
		return result, fmt.Errorf("extract: %w", err)
	}
	root := archiveRoot(staging)
	if isEmptyDir(root) {
		return result, errors.New("artifact is empty, nothing to install")
	}

	// Only stop something that exists: on a fresh install the directory (and
	// therefore the service) is not there yet, and running the command with a
	// missing working directory fails with a confusing chdir error.
	if strings.TrimSpace(spec.StopCommand) != "" {
		if dirExists(spec.TargetDir) {
			logf("info", "stopping service")
			if err := RunCommand(ctx, spec.StopCommand, spec.TargetDir, logf); err != nil {
				return result, fmt.Errorf("stop command: %w", err)
			}
		} else {
			logf("info", "nothing to stop: "+spec.TargetDir+" does not exist yet")
		}
	}

	if dirExists(spec.TargetDir) && !isEmptyDir(spec.TargetDir) {
		backupPath, err := Backup(spec.TargetDir, spec.BackupDir, fmt.Sprintf("%s-%s", spec.AppID, spec.Version), spec.Preserve, logf)
		if err != nil {
			return result, fmt.Errorf("backup: %w", err)
		}
		result.BackupPath = backupPath
	} else {
		logf("info", "no existing installation found, this is a fresh deploy")
	}

	fail := func(cause error) (Result, error) {
		logf("error", cause.Error())
		if result.BackupPath == "" {
			return result, cause
		}
		logf("error", "restoring previous version from "+result.BackupPath)
		if restoreErr := Restore(result.BackupPath, spec.TargetDir, spec.Preserve, logf); restoreErr != nil {
			logf("error", "rollback failed: "+restoreErr.Error())
			return result, fmt.Errorf("%w (rollback also failed: %v)", cause, restoreErr)
		}
		result.RolledBack = true
		logf("info", "previous version restored")
		startCmd := firstNonEmpty(spec.StartCommand, spec.RestartCommand)
		if startCmd != "" {
			if err := RunCommand(ctx, startCmd, spec.TargetDir, logf); err != nil {
				logf("error", "restart after rollback failed: "+err.Error())
			}
		}
		return result, cause
	}

	logf("info", "installing files into "+spec.TargetDir)
	if err := Sync(root, spec.TargetDir, spec.Preserve, logf); err != nil {
		return fail(fmt.Errorf("install: %w", err))
	}

	startCmd := firstNonEmpty(spec.StartCommand, spec.RestartCommand)
	if startCmd == "" {
		logf("info", "no start/restart command configured; files were updated without restarting the service")
	} else {
		logf("info", "starting service")
		if err := RunCommand(ctx, startCmd, spec.TargetDir, logf); err != nil {
			return fail(fmt.Errorf("start command: %w", err))
		}
	}

	if err := WaitHealthy(ctx, spec.HealthCheckURL, healthTimeout(spec.Timeout), logf); err != nil {
		return fail(err)
	}

	if spec.BackupKeep > 0 {
		PruneBackups(spec.BackupDir, spec.BackupKeep, logf)
	}
	logf("info", fmt.Sprintf("deploy finished in %s", time.Since(result.StartedAt).Round(time.Millisecond)))
	return result, nil
}

// Rollback restores the newest backup available on the host.
func Rollback(ctx context.Context, spec Spec, logf Logger) (Result, error) {
	result := Result{StartedAt: time.Now()}
	defer func() { result.FinishedAt = time.Now() }()
	if logf == nil {
		logf = func(string, string) {}
	}
	if err := normaliseSpec(&spec); err != nil {
		return result, err
	}
	if spec.Timeout <= 0 {
		spec.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	backupPath, err := LatestBackup(spec.BackupDir, spec.AppID)
	if err != nil {
		return result, err
	}
	result.BackupPath = backupPath
	logf("info", "rolling back to "+filepath.Base(backupPath))

	if spec.StopCommand != "" {
		if err := RunCommand(ctx, spec.StopCommand, spec.TargetDir, logf); err != nil {
			return result, fmt.Errorf("stop command: %w", err)
		}
	}
	if err := Restore(backupPath, spec.TargetDir, spec.Preserve, logf); err != nil {
		return result, fmt.Errorf("restore: %w", err)
	}
	result.RolledBack = true

	startCmd := firstNonEmpty(spec.StartCommand, spec.RestartCommand)
	if startCmd != "" {
		if err := RunCommand(ctx, startCmd, spec.TargetDir, logf); err != nil {
			return result, fmt.Errorf("start command: %w", err)
		}
	}
	if err := WaitHealthy(ctx, spec.HealthCheckURL, healthTimeout(spec.Timeout), logf); err != nil {
		return result, err
	}
	logf("info", "rollback finished")
	return result, nil
}

func normaliseSpec(spec *Spec) error {
	if strings.TrimSpace(spec.TargetDir) == "" {
		return errors.New("deploy_dir is not configured on the agent")
	}
	abs, err := filepath.Abs(spec.TargetDir)
	if err != nil {
		return err
	}
	spec.TargetDir = abs
	if spec.BackupDir == "" {
		spec.BackupDir = filepath.Join(filepath.Dir(abs), "lodestar-backups", filepath.Base(abs))
	}
	backupAbs, err := filepath.Abs(spec.BackupDir)
	if err != nil {
		return err
	}
	spec.BackupDir = backupAbs
	if spec.WorkDir == "" {
		spec.WorkDir = os.TempDir()
	}
	if err := os.MkdirAll(spec.BackupDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(spec.WorkDir, 0o755); err != nil {
		return err
	}
	if spec.BackupKeep <= 0 {
		spec.BackupKeep = 5
	}
	if spec.AppID == "" {
		spec.AppID = filepath.Base(abs)
	}
	return nil
}

// Backup copies targetDir into a timestamped directory under backupRoot.
// Backup copies the install tree aside. Whitelisted paths are skipped: they
// hold runtime data (databases, uploads) that must never travel with a release
// — restoring them later would resurrect stale state.
func Backup(targetDir, backupRoot, label string, preserve []string, logf Logger) (string, error) {
	dest := filepath.Join(backupRoot, fmt.Sprintf("%s-%s", sanitizeLabel(label), time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	start := time.Now()
	skipped := 0
	if err := copyTree(targetDir, dest, func(rel string) bool {
		if isPreserved(rel, preserve) {
			skipped++
			return true
		}
		return false
	}, logf); err != nil {
		return "", err
	}
	if skipped > 0 {
		logf("info", fmt.Sprintf("backup skipped %d whitelisted entr(ies)", skipped))
	}
	logf("info", fmt.Sprintf("backup created at %s (%s)", dest, time.Since(start).Round(time.Millisecond)))
	return dest, nil
}

// Restore replaces targetDir with the contents of backupPath. The broken tree is
// moved aside (never deleted) so an operator can inspect it.
// Restore puts a backup back in place. Whitelisted paths are not part of the
// backup, so the ones from the tree being replaced are carried over: rolling
// back must not wipe the database it is supposed to keep serving.
func Restore(backupPath, targetDir string, preserve []string, logf Logger) error {
	if !dirExists(backupPath) {
		return fmt.Errorf("backup %s not found", backupPath)
	}
	if logf == nil {
		logf = func(string, string) {}
	}
	aside := ""
	if dirExists(targetDir) {
		aside = fmt.Sprintf("%s.failed-%s", strings.TrimRight(targetDir, string(filepath.Separator)), time.Now().Format("20060102-150405"))
		if err := os.Rename(targetDir, aside); err != nil {
			logf("error", "could not move failed deployment aside ("+err.Error()+"), clearing it instead")
			aside = ""
			if err := os.RemoveAll(targetDir); err != nil {
				return err
			}
		} else {
			logf("info", "failed deployment kept at "+aside)
		}
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	if err := copyTree(backupPath, targetDir, nil, logf); err != nil {
		return err
	}
	if aside != "" {
		restored, err := carryOverPreserved(aside, targetDir, preserve, logf)
		if err != nil {
			return err
		}
		if restored > 0 {
			logf("info", fmt.Sprintf("kept %d whitelisted entr(ies) from the replaced tree", restored))
		}
	}
	return nil
}

// carryOverPreserved copies whitelisted entries from oldDir into newDir.
func carryOverPreserved(oldDir, newDir string, preserve []string, logf Logger) (int, error) {
	if len(preserve) == 0 {
		return 0, nil
	}
	count := 0
	for _, item := range preserve {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		src := filepath.Join(oldDir, item)
		info, err := os.Stat(src)
		if err != nil {
			continue
		}
		dst := filepath.Join(newDir, item)
		// A whitelisted entry can be a single file (appsettings.json) or a
		// directory (data/). WalkDir on a file yields only ".", which the tree
		// copier skips, so files are copied directly.
		if info.IsDir() {
			if err := copyTree(src, dst, nil, logf); err != nil {
				return count, err
			}
		} else if err := copyFile(src, dst); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Sync mirrors src into dst, keeping preserved relative paths untouched.
func Sync(src, dst string, preserve []string, logf Logger) error {
	if logf == nil {
		logf = func(string, string) {}
	}
	preserved := normalisePreserve(preserve)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	var remove []string
	err := filepath.WalkDir(dst, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dst, path)
		if err != nil || rel == "." {
			return err
		}
		if isPreserved(rel, preserved) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(src, rel)); errors.Is(statErr, os.ErrNotExist) {
			remove = append(remove, path)
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(remove)))
	for _, path := range remove {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		logf("info", "removed stale "+path)
	}

	copied := 0
	kept := 0
	err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		// A preserved path is only skipped when the host already has it; on a fresh
		// install there is nothing to preserve, so the artifact's defaults are used.
		if isPreserved(rel, preserved) {
			if _, statErr := os.Stat(filepath.Join(dst, rel)); statErr == nil {
				kept++
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			logf("info", "skipped special file "+rel)
			return nil
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return err
	}
	logf("info", fmt.Sprintf("installed %d files", copied))
	if kept > 0 {
		logf("info", fmt.Sprintf("kept %d whitelisted entr(ies) untouched", kept))
	}
	return nil
}

// LatestBackup returns the most recent backup directory for appID.
func LatestBackup(backupRoot, appID string) (string, error) {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return "", fmt.Errorf("no backups available: %w", err)
	}
	prefix := sanitizeLabel(appID) + "-"
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if appID == "" || strings.HasPrefix(entry.Name(), prefix) {
			candidates = append(candidates, entry.Name())
		}
	}
	if len(candidates) == 0 {
		return "", errors.New("no backups available on this host")
	}
	sort.Strings(candidates)
	return filepath.Join(backupRoot, candidates[len(candidates)-1]), nil
}

// PruneBackups keeps only the newest keep backup directories.
func PruneBackups(backupRoot string, keep int, logf Logger) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	if len(dirs) <= keep {
		return
	}
	sort.Strings(dirs)
	for _, name := range dirs[:len(dirs)-keep] {
		if err := os.RemoveAll(filepath.Join(backupRoot, name)); err != nil {
			if logf != nil {
				logf("error", "prune backup "+name+": "+err.Error())
			}
			continue
		}
		if logf != nil {
			logf("info", "pruned old backup "+name)
		}
	}
}

func copyTree(src, dst string, skip func(rel string) bool, logf Logger) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if skip != nil && skip(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			if logf != nil {
				logf("info", "skipped non-regular file "+rel)
			}
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func normalisePreserve(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		item = strings.TrimPrefix(item, "./")
		item = strings.Trim(item, `/\`)
		if item == "" || item == "." {
			continue
		}
		out = append(out, filepath.FromSlash(item))
	}
	return out
}

// isPreserved compares slash-normalised paths so a whitelist written as
// "storage/logs" also matches on Windows, where WalkDir hands back backslashes.
func isPreserved(rel string, preserved []string) bool {
	rel = filepath.ToSlash(rel)
	for _, item := range preserved {
		item = strings.Trim(filepath.ToSlash(strings.TrimSpace(item)), "/")
		if item == "" || item == "." {
			continue
		}
		if rel == item || strings.HasPrefix(rel, item+"/") {
			return true
		}
	}
	return false
}

func sanitizeLabel(label string) string {
	replacer := strings.NewReplacer("/", "_", `\`, "_", ":", "_", " ", "_", "*", "_", "?", "_", "|", "_")
	clean := replacer.Replace(strings.TrimSpace(label))
	clean = strings.Trim(clean, "._")
	if clean == "" {
		return "app"
	}
	return clean
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func healthTimeout(total time.Duration) time.Duration {
	if total <= 0 {
		return 60 * time.Second
	}
	if quarter := total / 4; quarter < 30*time.Second {
		return 30 * time.Second
	} else if quarter > 5*time.Minute {
		return 5 * time.Minute
	} else {
		return quarter
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return true
	}
	return len(entries) == 0
}
