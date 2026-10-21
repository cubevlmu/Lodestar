package api

import (
	"testing"

	"lodestar/internal/models"
)

func TestNormalizeDeployPath(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"absolute", "/srv/demo", "/srv/demo", false},
		{"trailing slash", "/srv/demo/", "/srv/demo", false},
		{"windows separators", `E:\apps\demo`, "E:/apps/demo", false},
		{"dot segments", "/srv/./demo", "/srv/demo", false},
		{"traversal", "/srv/../../etc", "", true},
		{"relative traversal", "a/../../b", "", true},
		{"dot only", ".", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDeployPath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeDeployPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolveAppDirPrecedence(t *testing.T) {
	project := models.Project{AppID: "demo.app"}
	target := models.Target{DeployRoot: "/srv"}

	if got := resolveAppDir(target, nil, project); got != "/srv/demo.app" {
		t.Fatalf("expected derived path, got %q", got)
	}

	app := &models.TargetApp{DeployDir: "/opt/custom"}
	if got := resolveAppDir(target, app, project); got != "/opt/custom" {
		t.Fatalf("app path should win, got %q", got)
	}

	noRoot := models.Target{}
	if got := resolveAppDir(noRoot, nil, project); got != "" {
		t.Fatalf("no root should yield an empty path, got %q", got)
	}
}

func TestResolveBackupDir(t *testing.T) {
	target := models.Target{BackupDir: "/backups"}
	if got := resolveBackupDir(target, nil); got != "/backups" {
		t.Fatalf("expected host backup dir, got %q", got)
	}
	app := &models.TargetApp{BackupDir: "/backups/demo"}
	if got := resolveBackupDir(target, app); got != "/backups/demo" {
		t.Fatalf("app backup dir should win, got %q", got)
	}
}

func TestPickBackupKeep(t *testing.T) {
	if got := pickBackupKeep(0); got != 5 {
		t.Fatalf("default should be 5, got %d", got)
	}
	if got := pickBackupKeep(3); got != 3 {
		t.Fatalf("app value should apply, got %d", got)
	}
}
