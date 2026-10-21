package models

import "time"

// User is a WebUI operator. Credentials are stored as bcrypt hashes.
type User struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	DisplayName  string     `gorm:"size:128" json:"display_name"`
	Role         string     `gorm:"size:32;not null;default:admin" json:"role"` // admin | viewer
	Enabled      bool       `gorm:"not null;default:true" json:"enabled"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Session is an opaque bearer token. Only the SHA-256 hash of the token is persisted.
type Session struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	TokenHash string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expires_at"`
	IP        string    `gorm:"size:64" json:"ip"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

// Project is one deliverable application (app_id), with its own signing keys.
type Project struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	AppID             string    `gorm:"uniqueIndex;size:128;not null" json:"app_id"`
	Name              string    `gorm:"size:128;not null" json:"name"`
	Description       string    `gorm:"size:512" json:"description"`
	RepoURL           string    `gorm:"size:512" json:"repo_url"`
	ClientSecret      string    `gorm:"size:256;not null" json:"-"`
	DeployToken       string    `gorm:"uniqueIndex;size:128;not null" json:"-"`
	Ed25519PublicKey  string    `gorm:"size:256;not null" json:"ed25519_public_key"`
	Ed25519PrivateKey string    `gorm:"size:512;not null" json:"-"`
	Enabled           bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Release is one version of a project for a platform/arch/channel combination.
type Release struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProjectID        uint      `gorm:"index;not null" json:"project_id"`
	Project          Project   `json:"-"`
	Version          string    `gorm:"size:64;not null" json:"version"`
	VersionCode      int64     `gorm:"index;not null" json:"version_code"`
	Platform         string    `gorm:"size:64;index;not null" json:"platform"` // windows, linux, darwin, android, ios
	Arch             string    `gorm:"size:64;index;not null;default:universal" json:"arch"`
	Channel          string    `gorm:"size:64;index;not null;default:stable" json:"channel"`
	Title            string    `gorm:"size:256" json:"title"`
	Changelog        string    `gorm:"type:text" json:"changelog"`
	MinSupportedCode int64     `gorm:"not null;default:0" json:"min_supported_code"`
	Force            bool      `gorm:"not null;default:false" json:"force"`
	Enabled          bool      `gorm:"not null;default:true" json:"enabled"`
	RolloutPercent   int       `gorm:"not null;default:100" json:"rollout_percent"`
	Source           string    `gorm:"size:32;default:manual" json:"source"` // manual | ci | api
	CreatedBy        string    `gorm:"size:64" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Package          Package   `gorm:"constraint:OnDelete:CASCADE" json:"package"`
}

// Package is the downloadable artifact behind a release.
type Package struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ReleaseID   uint      `gorm:"uniqueIndex;not null" json:"release_id"`
	FileName    string    `gorm:"size:256;not null" json:"file_name"`
	StoragePath string    `gorm:"size:1024" json:"storage_path"`
	ExternalURL string    `gorm:"size:2048" json:"external_url"`
	Size        int64     `gorm:"not null" json:"size"`
	SHA256      string    `gorm:"size:64;not null" json:"sha256"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Target is a production host running `lodestar agent`.
//
// A host is not owned by a project: one agent commonly serves several
// applications, each installed into its own directory. Per-application
// settings (deploy path, commands, current version) live in TargetApp.
type Target struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	Name           string `gorm:"size:128;not null" json:"name"`
	Token          string `gorm:"uniqueIndex;size:128;not null" json:"-"`
	Platform       string `gorm:"size:64;not null" json:"platform"`
	Arch           string `gorm:"size:64;not null;default:universal" json:"arch"`
	Channel        string `gorm:"size:64;not null;default:stable" json:"channel"`
	Status         string `gorm:"size:32;not null;default:offline" json:"status"` // online | offline | deploying | error
	CurrentVersion string `gorm:"size:64" json:"current_version"`
	CurrentCode    int64  `gorm:"default:0" json:"current_code"`
	Hostname       string `gorm:"size:128" json:"hostname"`
	OS             string `gorm:"size:64" json:"os"`
	AgentVersion   string `gorm:"size:64" json:"agent_version"`
	// DeployRoot is the default base for applications. Each app carries its own
	// path and restart commands, so one host can serve many projects.
	DeployRoot   string      `gorm:"size:512" json:"deploy_root"`
	BackupDir    string      `gorm:"size:512" json:"backup_dir"`
	AutoDeploy   bool        `gorm:"not null;default:true" json:"auto_deploy"`
	Notes        string      `gorm:"size:512" json:"notes"`
	LastSeenAt   *time.Time  `json:"last_seen_at"`
	LastDeployAt *time.Time  `json:"last_deploy_at"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	Apps         []TargetApp `gorm:"foreignKey:TargetID" json:"apps,omitempty"`
}

// TargetApp is one application installed on one host: where it lives, how to
// restart it, and which version is currently running there.
type TargetApp struct {
	ID        uint    `gorm:"primaryKey" json:"id"`
	TargetID  uint    `gorm:"uniqueIndex:uniq_target_app;not null" json:"target_id"`
	ProjectID uint    `gorm:"uniqueIndex:uniq_target_app;index;not null" json:"project_id"`
	Project   Project `json:"-"`
	// DeployDir is the absolute path on the host. Empty means
	// <target.deploy_root>/<app_id>, resolved by the agent.
	DeployDir string `gorm:"size:512" json:"deploy_dir"`
	BackupDir string `gorm:"size:512" json:"backup_dir"`
	// Every command is optional: leave them empty and the agent just swaps the
	// files without touching the running process.
	StopCommand    string     `gorm:"size:512" json:"stop_command"`
	StartCommand   string     `gorm:"size:512" json:"start_command"`
	RestartCommand string     `gorm:"size:512" json:"restart_command"`
	HealthCheckURL string     `gorm:"size:512" json:"health_check_url"`
	Preserve       string     `gorm:"size:512" json:"preserve"`
	BackupKeep     int        `gorm:"not null;default:5" json:"backup_keep"`
	CurrentVersion string     `gorm:"size:64" json:"current_version"`
	CurrentCode    int64      `gorm:"default:0" json:"current_code"`
	LastDeployAt   *time.Time `json:"last_deploy_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// DeployJob is one deployment of one release onto one target.
type DeployJob struct {
	ID          uint    `gorm:"primaryKey" json:"id"`
	ProjectID   uint    `gorm:"index;not null" json:"project_id"`
	ReleaseID   uint    `gorm:"index;not null" json:"release_id"`
	TargetID    uint    `gorm:"index;not null" json:"target_id"`
	Target      Target  `json:"target"`
	Release     Release `json:"-"`
	Version     string  `gorm:"size:64;not null" json:"version"`
	VersionCode int64   `gorm:"not null" json:"version_code"`
	Mode        string  `gorm:"size:16;not null;default:deploy" json:"mode"` // deploy | rollback
	Status      string  `gorm:"size:16;index;not null;default:pending" json:"status"`
	Trigger     string  `gorm:"size:16;not null;default:manual" json:"trigger"` // manual | ci | retry | rollback
	RequestedBy string  `gorm:"size:64" json:"requested_by"`
	Message     string  `gorm:"size:512" json:"message"`
	Error       string  `gorm:"size:1024" json:"error"`
	BackupPath  string  `gorm:"size:1024" json:"backup_path"`
	// DeployDir/BackupDir are snapshotted when the job is queued so history
	// stays accurate even after the target's settings change.
	DeployDir  string      `gorm:"size:512" json:"deploy_dir"`
	BackupDir  string      `gorm:"size:512" json:"backup_dir"`
	RolledBack bool        `gorm:"not null;default:false" json:"rolled_back"`
	DurationMs int64       `gorm:"not null;default:0" json:"duration_ms"`
	StartedAt  *time.Time  `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at"`
	Logs       []DeployLog `gorm:"foreignKey:JobID" json:"-"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// DeployLog is a single log line produced by the agent while executing a job.
type DeployLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	JobID     uint      `gorm:"uniqueIndex:uniq_job_seq;not null" json:"job_id"`
	Seq       int       `gorm:"uniqueIndex:uniq_job_seq;not null" json:"seq"`
	Stream    string    `gorm:"size:16;not null" json:"stream"` // info | stdout | stderr | error
	Line      string    `gorm:"type:text" json:"line"`
	CreatedAt time.Time `json:"created_at"`
}

// VersionPolicy gates the client update check per platform/channel.
type VersionPolicy struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProjectID      uint      `gorm:"uniqueIndex:uniq_policy;not null" json:"project_id"`
	Platform       string    `gorm:"uniqueIndex:uniq_policy;size:64;not null" json:"platform"`
	Channel        string    `gorm:"uniqueIndex:uniq_policy;size:64;not null;default:stable" json:"channel"`
	MinAllowedCode int64     `gorm:"not null;default:0" json:"min_allowed_code"`
	Message        string    `gorm:"size:512" json:"message"`
	Maintenance    bool      `gorm:"not null;default:false" json:"maintenance"`
	MaintenanceMsg string    `gorm:"size:512" json:"maintenance_msg"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// BlockedVersion refuses a specific version code.
type BlockedVersion struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ProjectID   uint      `gorm:"index;not null" json:"project_id"`
	Platform    string    `gorm:"size:64;index;not null" json:"platform"`
	Channel     string    `gorm:"size:64;index;not null;default:stable" json:"channel"`
	VersionCode int64     `gorm:"index;not null" json:"version_code"`
	Reason      string    `gorm:"size:512" json:"reason"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RequestNonce prevents replay of signed client requests.
type RequestNonce struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"uniqueIndex:uniq_nonce;not null"`
	Nonce     string    `gorm:"uniqueIndex:uniq_nonce;size:128;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

// AuditLog records operator actions.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Username  string    `gorm:"size:64" json:"username"`
	Action    string    `gorm:"size:64;not null" json:"action"`
	Detail    string    `gorm:"size:1024" json:"detail"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

// Setting is a small key/value store for runtime-editable options.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (t *Target) IsOnline(window int) bool {
	if t.LastSeenAt == nil {
		return false
	}
	return time.Since(*t.LastSeenAt) < time.Duration(window)*time.Second
}
