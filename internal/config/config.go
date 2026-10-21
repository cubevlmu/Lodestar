package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig is the on-host half of Lodestar. It is read from the `agent:` section of the
// system config (or a dedicated agent.yaml) and only matters in `lodestar agent` mode.
type AgentConfig struct {
	ServerURL   string `yaml:"server_url"`
	TargetToken string `yaml:"target_token"`
	Name        string `yaml:"name"`
	// DeployRoot is the default base for applications. Each job carries its
	// own deploy path, which may point anywhere on the host.
	DeployRoot string `yaml:"deploy_root"`
	// DeployDir is the legacy single-app setting. When set it is used for jobs
	// that do not carry a path, and it is also accepted as the deploy root.
	DeployDir           string        `yaml:"deploy_dir"`
	BackupDir           string        `yaml:"backup_dir"`
	WorkDir             string        `yaml:"work_dir"`
	PollInterval        time.Duration `yaml:"-"`
	PollIntervalSeconds int           `yaml:"poll_interval_seconds"`
	AllowServerCommands *bool         `yaml:"allow_server_commands"`
	InsecureSkipVerify  bool          `yaml:"insecure_skip_verify"`
}

type fileConfig struct {
	Addr                string      `yaml:"addr"`
	DBPath              string      `yaml:"db_path"`
	StorageDir          string      `yaml:"storage_dir"`
	DataDir             string      `yaml:"data_dir"`
	PublicBaseURL       string      `yaml:"public_base_url"`
	DownloadTokenSecret string      `yaml:"download_token_secret"`
	ClientClockSkewSec  int         `yaml:"client_clock_skew_seconds"`
	DownloadTokenTTLSec int         `yaml:"download_token_ttl_seconds"`
	SessionTTLHours     int         `yaml:"session_ttl_hours"`
	MaxUploadMB         int         `yaml:"max_upload_mb"`
	AdminUsername       string      `yaml:"admin_username"`
	AdminPassword       string      `yaml:"admin_password"`
	SQLDebug            bool        `yaml:"sql_debug"`
	Agent               AgentConfig `yaml:"agent"`
}

// Config is the fully resolved runtime configuration.
type Config struct {
	Addr                string
	DBPath              string
	StorageDir          string
	DataDir             string
	PublicBaseURL       string
	DownloadTokenSecret string
	ClientClockSkew     time.Duration
	DownloadTokenTTL    time.Duration
	SessionTTL          time.Duration
	MaxUploadBytes      int64
	AdminUsername       string
	AdminPassword       string
	// SQLDebug turns GORM's statement log back on. Off by default: the log
	// otherwise fills with one line per query.
	SQLDebug bool
	Agent    AgentConfig
}

func Load() Config {
	path := getenv("LODESTAR_CONFIG", "data/config/system.yaml")
	fc := loadFileDefaults(path)

	cfg := Config{
		Addr:                getenv("LODESTAR_ADDR", fc.Addr),
		DBPath:              getenv("LODESTAR_DB_PATH", fc.DBPath),
		StorageDir:          getenv("LODESTAR_STORAGE_DIR", fc.StorageDir),
		DataDir:             getenv("LODESTAR_DATA_DIR", fc.DataDir),
		PublicBaseURL:       strings.TrimRight(getenv("LODESTAR_PUBLIC_BASE_URL", fc.PublicBaseURL), "/"),
		DownloadTokenSecret: getenv("LODESTAR_DOWNLOAD_TOKEN_SECRET", fc.DownloadTokenSecret),
		ClientClockSkew:     time.Duration(getenvInt("LODESTAR_CLIENT_CLOCK_SKEW_SECONDS", fc.ClientClockSkewSec)) * time.Second,
		DownloadTokenTTL:    time.Duration(getenvInt("LODESTAR_DOWNLOAD_TOKEN_TTL_SECONDS", fc.DownloadTokenTTLSec)) * time.Second,
		SessionTTL:          time.Duration(getenvInt("LODESTAR_SESSION_TTL_HOURS", fc.SessionTTLHours)) * time.Hour,
		MaxUploadBytes:      int64(getenvInt("LODESTAR_MAX_UPLOAD_MB", fc.MaxUploadMB)) * 1024 * 1024,
		AdminUsername:       getenv("LODESTAR_ADMIN_USERNAME", fc.AdminUsername),
		AdminPassword:       getenv("LODESTAR_ADMIN_PASSWORD", fc.AdminPassword),
		SQLDebug:            getenvBool("LODESTAR_SQL_DEBUG", fc.SQLDebug),
		Agent:               fc.Agent,
	}

	cfg.Agent.ServerURL = strings.TrimRight(getenv("LODESTAR_AGENT_SERVER_URL", cfg.Agent.ServerURL), "/")
	cfg.Agent.TargetToken = getenv("LODESTAR_AGENT_TARGET_TOKEN", cfg.Agent.TargetToken)
	cfg.Agent.Name = getenv("LODESTAR_AGENT_NAME", cfg.Agent.Name)
	cfg.Agent.DeployRoot = getenv("LODESTAR_AGENT_DEPLOY_ROOT", cfg.Agent.DeployRoot)
	cfg.Agent.DeployDir = getenv("LODESTAR_AGENT_DEPLOY_DIR", cfg.Agent.DeployDir)
	cfg.Agent.BackupDir = getenv("LODESTAR_AGENT_BACKUP_DIR", cfg.Agent.BackupDir)
	cfg.Agent.WorkDir = getenv("LODESTAR_AGENT_WORK_DIR", cfg.Agent.WorkDir)
	// The rest of the agent settings must be settable from the environment too:
	// in a container there is no yaml file to edit, and the restart command is
	// the whole point of the agent.
	cfg.Agent.PollIntervalSeconds = getenvInt("LODESTAR_AGENT_POLL_INTERVAL_SECONDS", cfg.Agent.PollIntervalSeconds)
	cfg.Agent.InsecureSkipVerify = getenvBool("LODESTAR_AGENT_INSECURE_SKIP_VERIFY", cfg.Agent.InsecureSkipVerify)
	if cfg.Agent.AllowServerCommands == nil {
		if _, ok := os.LookupEnv("LODESTAR_AGENT_ALLOW_SERVER_COMMANDS"); ok {
			allowed := getenvBool("LODESTAR_AGENT_ALLOW_SERVER_COMMANDS", true)
			cfg.Agent.AllowServerCommands = &allowed
		}
	}

	if cfg.Agent.PollIntervalSeconds <= 0 {
		cfg.Agent.PollIntervalSeconds = 5
	}
	cfg.Agent.PollInterval = time.Duration(cfg.Agent.PollIntervalSeconds) * time.Second
	if cfg.Agent.WorkDir == "" {
		cfg.Agent.WorkDir = "data/agent"
	}
	return cfg
}

func loadFileDefaults(path string) fileConfig {
	fc := fileConfig{
		Addr:                ":8080",
		DBPath:              "data/lodestar.db",
		StorageDir:          "storage",
		DataDir:             "data",
		PublicBaseURL:       "",
		DownloadTokenSecret: "change-me-download-token-secret",
		ClientClockSkewSec:  300,
		DownloadTokenTTLSec: 900,
		SessionTTLHours:     168,
		MaxUploadMB:         2048,
		AdminUsername:       "admin",
		Agent: AgentConfig{
			ServerURL:           "http://127.0.0.1:8080",
			Name:                "production",
			PollIntervalSeconds: 5,
		},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fc
	}
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fc
	}
	if fc.MaxUploadMB <= 0 {
		fc.MaxUploadMB = 2048
	}
	if fc.SessionTTLHours <= 0 {
		fc.SessionTTLHours = 168
	}
	return fc
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getenvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
