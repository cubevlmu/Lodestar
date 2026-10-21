package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// resetEnv clears every LODESTAR_* variable so one test cannot leak into another.
func resetEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				key := entry[:i]
				if len(key) > len("LODESTAR_") && key[:len("LODESTAR_")] == "LODESTAR_" {
					t.Setenv(key, "")
					_ = os.Unsetenv(key)
				}
				break
			}
		}
	}
	t.Setenv("LODESTAR_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
}

// A container has no yaml file to edit, so every agent setting must be
// reachable from the environment.
func TestAgentConfigFromEnvOnly(t *testing.T) {
	resetEnv(t)

	t.Setenv("LODESTAR_AGENT_SERVER_URL", "https://lodestar.example.com/")
	t.Setenv("LODESTAR_AGENT_TARGET_TOKEN", "tok-123")
	t.Setenv("LODESTAR_AGENT_NAME", "container")
	t.Setenv("LODESTAR_AGENT_DEPLOY_DIR", "/srv/app")
	t.Setenv("LODESTAR_AGENT_BACKUP_DIR", "/backups")
	t.Setenv("LODESTAR_AGENT_WORK_DIR", "/work")
	t.Setenv("LODESTAR_AGENT_POLL_INTERVAL_SECONDS", "7")
	t.Setenv("LODESTAR_AGENT_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("LODESTAR_AGENT_ALLOW_SERVER_COMMANDS", "false")

	cfg := Load()
	agent := cfg.Agent

	if agent.ServerURL != "https://lodestar.example.com" {
		t.Fatalf("server url: %q", agent.ServerURL)
	}
	if agent.TargetToken != "tok-123" || agent.Name != "container" {
		t.Fatalf("token/name: %q %q", agent.TargetToken, agent.Name)
	}
	if agent.DeployDir != "/srv/app" || agent.BackupDir != "/backups" || agent.WorkDir != "/work" {
		t.Fatalf("dirs: %q %q %q", agent.DeployDir, agent.BackupDir, agent.WorkDir)
	}
	if agent.PollInterval != 7*time.Second {
		t.Fatalf("poll interval: %s", agent.PollInterval)
	}
	if !agent.InsecureSkipVerify {
		t.Fatal("insecure skip verify should be true")
	}
	if agent.AllowServerCommands == nil || *agent.AllowServerCommands {
		t.Fatalf("allow server commands should be false, got %v", agent.AllowServerCommands)
	}
}

func TestSQLDebugFromEnv(t *testing.T) {
	resetEnv(t)

	if Load().SQLDebug {
		t.Fatal("sql debug should default to false")
	}

	t.Setenv("LODESTAR_SQL_DEBUG", "1")
	if !Load().SQLDebug {
		t.Fatal("LODESTAR_SQL_DEBUG=1 should enable sql logging")
	}

	t.Setenv("LODESTAR_SQL_DEBUG", "off")
	if Load().SQLDebug {
		t.Fatal("LODESTAR_SQL_DEBUG=off should disable sql logging")
	}
}

func TestDefaultsAreUsable(t *testing.T) {
	resetEnv(t)

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Fatalf("addr: %q", cfg.Addr)
	}
	if cfg.Agent.PollIntervalSeconds != 5 {
		t.Fatalf("agent poll default: %d", cfg.Agent.PollIntervalSeconds)
	}
	if cfg.Agent.WorkDir != "data/agent" {
		t.Fatalf("work dir default: %q", cfg.Agent.WorkDir)
	}
}

func TestHeartbeatIntervalDefaultsAndOverride(t *testing.T) {
	cfg := Load()
	if cfg.Agent.HeartbeatIntervalSeconds != 60 {
		t.Fatalf("default heartbeat interval should be 60s, got %d", cfg.Agent.HeartbeatIntervalSeconds)
	}
	if cfg.Agent.HeartbeatInterval != time.Minute {
		t.Fatalf("derived heartbeat duration: %s", cfg.Agent.HeartbeatInterval)
	}

	t.Setenv("LODESTAR_AGENT_HEARTBEAT_INTERVAL_SECONDS", "120")
	cfg = Load()
	if cfg.Agent.HeartbeatInterval != 2*time.Minute {
		t.Fatalf("env override not applied: %s", cfg.Agent.HeartbeatInterval)
	}
}
