package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodestar/internal/config"
)

func newTestAgent(root string) *Agent {
	return &Agent{cfg: config.AgentConfig{DeployRoot: root}}
}

func TestResolveDeployDirDerivesFromRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "srv")
	a := newTestAgent(root)

	got, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "demo.app")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveDeployDirUsesJobPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "srv")
	a := newTestAgent(root)
	jobPath := filepath.Join(root, "custom")

	got, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app", DeployDir: jobPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != jobPath {
		t.Fatalf("got %q, want %q", got, jobPath)
	}
}

// deploy_root only supplies the default: an explicit absolute path anywhere on
// the host must be honoured.
func TestResolveDeployDirAllowsPathOutsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "srv")
	a := newTestAgent(root)

	outside := filepath.Join(t.TempDir(), "test-deploy")
	got, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app", DeployDir: outside})
	if err != nil {
		t.Fatalf("absolute paths outside the root should be allowed: %v", err)
	}
	if got != outside {
		t.Fatalf("got %q, want %q", got, outside)
	}
}

func TestResolveDeployDirRefusesFilesystemRoot(t *testing.T) {
	a := newTestAgent(filepath.Join(t.TempDir(), "srv"))

	// "/" on Unix, "C:////" on Windows — on Windows a bare separator is not
	// absolute, so the volume has to be included.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.VolumeName(wd) + string(filepath.Separator)

	if _, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app", DeployDir: root}); err == nil {
		t.Fatalf("deploying into the filesystem root (%q) must be refused", root)
	}
}

func TestResolveDeployDirRelativeJobPathJoinsRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "srv")
	a := newTestAgent(root)

	got, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app", DeployDir: "apps/demo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Join(root, "apps", "demo") {
		t.Fatalf("relative path should resolve under the root, got %q", got)
	}
}

func TestResolveDeployDirRequiresRoot(t *testing.T) {
	a := newTestAgent("")
	if _, err := a.resolveDeployDir(&jobPayload{AppID: "demo.app"}); err == nil {
		t.Fatal("expected an error when no root is configured")
	}
}

func TestSanitizeAppID(t *testing.T) {
	cases := map[string]string{
		"demo.app":   "demo.app",
		"a/b":        "a-b",
		`back\slash`: "back-slash",
		"  ..  ":     "app",
		"":           "app",
	}
	for input, want := range cases {
		if got := sanitizeAppID(input); got != want {
			t.Fatalf("sanitizeAppID(%q) = %q, want %q", input, got, want)
		}
	}
	if strings.Contains(sanitizeAppID("a/b"), "/") {
		t.Fatal("sanitized id must not contain a separator")
	}
}

// An empty command must stay empty: the agent has no local fallback any more.
func TestCommandDoesNotFallBack(t *testing.T) {
	a := newTestAgent("/srv")
	if got := a.command(""); got != "" {
		t.Fatalf("empty command should stay empty, got %q", got)
	}
	if got := a.command("  "); got != "" {
		t.Fatalf("blank command should stay empty, got %q", got)
	}
	if got := a.command(" systemctl restart demo "); got != "systemctl restart demo" {
		t.Fatalf("command should be trimmed, got %q", got)
	}
}

func TestCommandDisabledByConfig(t *testing.T) {
	disabled := false
	a := &Agent{cfg: config.AgentConfig{DeployRoot: "/srv", AllowServerCommands: &disabled}}
	if got := a.command("systemctl restart demo"); got != "" {
		t.Fatalf("allow_server_commands=false must suppress commands, got %q", got)
	}
}

func TestPickBackupKeepDefault(t *testing.T) {
	if got := pickBackupKeep(0); got != 5 {
		t.Fatalf("default should be 5, got %d", got)
	}
	if got := pickBackupKeep(2); got != 2 {
		t.Fatalf("explicit value should win, got %d", got)
	}
}
