package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"lodestar/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type heartbeatApp struct {
	AppID       string `json:"app_id"`
	Version     string `json:"version"`
	VersionCode int64  `json:"version_code"`
	DeployDir   string `json:"deploy_dir"`
}

type heartbeatRequest struct {
	Name           string         `json:"name"`
	Hostname       string         `json:"hostname"`
	OS             string         `json:"os"`
	Arch           string         `json:"arch"`
	AgentVersion   string         `json:"agent_version"`
	CurrentVersion string         `json:"current_version"`
	CurrentCode    int64          `json:"current_code"`
	Status         string         `json:"status"`
	Apps           []heartbeatApp `json:"apps"`
}

func (a *API) AgentHeartbeat(c *gin.Context) {
	target := CurrentTarget(c)
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	now := time.Now()
	updates := map[string]any{
		"last_seen_at":  now,
		"hostname":      truncate(req.Hostname, 120),
		"os":            truncate(req.OS, 60),
		"agent_version": truncate(req.AgentVersion, 60),
	}
	if req.CurrentVersion != "" {
		updates["current_version"] = truncate(req.CurrentVersion, 60)
		updates["current_code"] = req.CurrentCode
	}
	// Per-app versions let the UI stay accurate for hosts that run several
	// applications, and survive an agent restart.
	for _, app := range req.Apps {
		appID := strings.TrimSpace(app.AppID)
		if appID == "" {
			continue
		}
		var project models.Project
		if err := a.DB.Where("app_id = ?", appID).First(&project).Error; err != nil {
			continue
		}
		updates := map[string]any{"last_seen_at": now}
		if app.Version != "" {
			updates["current_version"] = truncate(app.Version, 60)
			updates["current_code"] = app.VersionCode
			updates["last_deploy_at"] = now
		}
		result := a.DB.Model(&models.TargetApp{}).
			Where("target_id = ? AND project_id = ?", target.ID, project.ID).
			Updates(updates)
		if result.Error == nil && result.RowsAffected == 0 && app.Version != "" {
			record := models.TargetApp{
				TargetID:       target.ID,
				ProjectID:      project.ID,
				DeployDir:      truncate(app.DeployDir, 500),
				CurrentVersion: truncate(app.Version, 60),
				CurrentCode:    app.VersionCode,
				LastDeployAt:   &now,
			}
			a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		}
	}

	// Never let a heartbeat overwrite "deploying" while a job is in flight.
	var running int64
	a.DB.Model(&models.DeployJob{}).Where("target_id = ? AND status = ?", target.ID, "running").Count(&running)
	if running == 0 {
		status := "online"
		if req.Status == "error" {
			status = "error"
		}
		updates["status"] = status
	}
	if err := a.DB.Model(&models.Target{}).Where("id = ?", target.ID).Updates(updates).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"server_time": now,
		"target": gin.H{
			"id":          target.ID,
			"name":        target.Name,
			"deploy_root": target.DeployRoot,
			"platform":    target.Platform,
			"channel":     target.Channel,
		},
	})
}

type agentJobPayload struct {
	ID             uint     `json:"id"`
	Mode           string   `json:"mode"`
	AppID          string   `json:"app_id"`
	Version        string   `json:"version"`
	VersionCode    int64    `json:"version_code"`
	PackageID      uint     `json:"package_id"`
	PackageName    string   `json:"package_name"`
	PackageSize    int64    `json:"package_size"`
	SHA256         string   `json:"sha256"`
	DownloadURL    string   `json:"download_url"`
	StopCommand    string   `json:"stop_command"`
	StartCommand   string   `json:"start_command"`
	RestartCommand string   `json:"restart_command"`
	HealthCheckURL string   `json:"health_check_url"`
	Preserve       []string `json:"preserve"`
	BackupKeep     int      `json:"backup_keep"`
	// DeployDir is where this app lives on the host. Empty means the agent
	// derives <deploy_root>/<app_id> (or its own configured default).
	DeployDir string `json:"deploy_dir"`
	BackupDir string `json:"backup_dir"`
	Message   string `json:"message"`
}

// AgentNextJob long-polls for a pending job, claims it atomically, and returns the full spec.
func (a *API) AgentNextJob(c *gin.Context) {
	target := CurrentTarget(c)
	wait, _ := time.ParseDuration(c.DefaultQuery("wait", "20s"))
	if wait <= 0 || wait > 30*time.Second {
		wait = 20 * time.Second
	}
	deadline := time.Now().Add(wait)

	a.touchTarget(target, "")

	for {
		var job models.DeployJob
		err := a.DB.Where("target_id = ? AND status = ?", target.ID, "pending").Order("id ASC").First(&job).Error
		if err == nil {
			claim := a.DB.Model(&models.DeployJob{}).
				Where("id = ? AND status = ?", job.ID, "pending").
				Updates(map[string]any{"status": "running", "started_at": time.Now()})
			if claim.Error == nil && claim.RowsAffected == 1 {
				a.DB.Model(&models.Target{}).Where("id = ?", target.ID).Update("status", "deploying")
				payload, buildErr := a.buildAgentPayload(target, job)
				if buildErr != nil {
					now := time.Now()
					a.DB.Model(&models.DeployJob{}).Where("id = ?", job.ID).Updates(map[string]any{
						"status": "failed", "error": buildErr.Error(), "finished_at": now,
					})
					a.DB.Model(&models.Target{}).Where("id = ?", target.ID).Update("status", "error")
					c.JSON(http.StatusOK, gin.H{"job": nil, "error": buildErr.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"job": payload})
				return
			}
		}

		if time.Now().After(deadline) {
			c.Status(http.StatusNoContent)
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (a *API) buildAgentPayload(target *models.Target, job models.DeployJob) (*agentJobPayload, error) {
	project, err := a.projectByID(job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found")
	}
	var app models.TargetApp
	_ = a.DB.Where("target_id = ? AND project_id = ?", target.ID, job.ProjectID).First(&app).Error

	payload := &agentJobPayload{
		ID:          job.ID,
		Mode:        job.Mode,
		AppID:       project.AppID,
		Version:     job.Version,
		VersionCode: job.VersionCode,
		// Commands and the whitelist belong to the app, never to the host.
		// All of them are optional: empty means "just swap the files".
		StopCommand:    app.StopCommand,
		StartCommand:   app.StartCommand,
		RestartCommand: app.RestartCommand,
		HealthCheckURL: app.HealthCheckURL,
		BackupKeep:     pickBackupKeep(app.BackupKeep),
		Message:        job.Message,
		DeployDir:      firstNonEmpty(job.DeployDir, resolveAppDir(*target, &app, *project)),
		BackupDir:      firstNonEmpty(job.BackupDir, resolveBackupDir(*target, &app)),
	}
	payload.Preserve = splitPreserve(app.Preserve)

	if job.Mode == "rollback" {
		return payload, nil
	}

	var release models.Release
	if err := a.DB.Preload("Package").First(&release, job.ReleaseID).Error; err != nil {
		return nil, fmt.Errorf("release not found")
	}
	if release.Package.ID == 0 {
		return nil, fmt.Errorf("release has no package")
	}
	payload.PackageID = release.Package.ID
	payload.PackageName = release.Package.FileName
	payload.PackageSize = release.Package.Size
	payload.SHA256 = release.Package.SHA256
	payload.DownloadURL = fmt.Sprintf("/api/v1/agent/packages/%d/download", release.Package.ID)
	return payload, nil
}

type agentLogLine struct {
	Seq    int    `json:"seq"`
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

type agentLogsRequest struct {
	Lines []agentLogLine `json:"lines"`
}

func (a *API) AgentLogs(c *gin.Context) {
	target := CurrentTarget(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.First(&job, id).Error; err != nil || job.TargetID != target.ID {
		notFound(c, "部署任务不存在")
		return
	}
	var req agentLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	if len(req.Lines) > 500 {
		req.Lines = req.Lines[:500]
	}
	entries := make([]models.DeployLog, 0, len(req.Lines))
	for _, line := range req.Lines {
		stream := line.Stream
		switch stream {
		case "info", "stdout", "stderr", "error":
		default:
			stream = "info"
		}
		entries = append(entries, models.DeployLog{
			JobID:  job.ID,
			Seq:    line.Seq,
			Stream: stream,
			Line:   truncate(line.Line, 8000),
		})
	}
	if len(entries) > 0 {
		onConflict := clause.OnConflict{
			Columns:   []clause.Column{{Name: "job_id"}, {Name: "seq"}},
			DoNothing: true,
		}
		if err := a.DB.Clauses(onConflict).Create(&entries).Error; err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
	}
	a.touchTarget(target, "")
	c.JSON(http.StatusOK, gin.H{"received": len(entries)})
}

type agentFinishRequest struct {
	Status      string `json:"status"`
	Error       string `json:"error"`
	BackupPath  string `json:"backup_path"`
	RolledBack  bool   `json:"rolled_back"`
	DurationMs  int64  `json:"duration_ms"`
	Version     string `json:"version"`
	VersionCode int64  `json:"version_code"`
}

func (a *API) AgentFinish(c *gin.Context) {
	target := CurrentTarget(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.First(&job, id).Error; err != nil || job.TargetID != target.ID {
		notFound(c, "部署任务不存在")
		return
	}
	var req agentFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	if req.Status != "success" && req.Status != "failed" {
		badRequest(c, "status 必须是 success 或 failed")
		return
	}
	now := time.Now()
	updates := map[string]any{
		"status":      req.Status,
		"error":       truncate(req.Error, 1000),
		"backup_path": truncate(req.BackupPath, 1000),
		"rolled_back": req.RolledBack,
		"duration_ms": req.DurationMs,
		"finished_at": now,
	}
	if err := a.DB.Model(&models.DeployJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	targetUpdates := map[string]any{"status": "online", "last_seen_at": now}
	if req.Status == "success" && !req.RolledBack {
		version := req.Version
		if version == "" {
			version = job.Version
		}
		code := req.VersionCode
		if code == 0 {
			code = job.VersionCode
		}
		targetUpdates["current_version"] = version
		targetUpdates["current_code"] = code
		targetUpdates["last_deploy_at"] = now
		a.saveAppVersion(target.ID, job.ProjectID, version, code)
	}
	if req.Status == "failed" {
		targetUpdates["status"] = "error"
	}
	a.DB.Model(&models.Target{}).Where("id = ?", target.ID).Updates(targetUpdates)
	c.JSON(http.StatusOK, gin.H{"status": "recorded"})
}

// AgentDownloadPackage serves a package to an authenticated agent of the same project.
func (a *API) AgentDownloadPackage(c *gin.Context) {
	target := CurrentTarget(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var pkg models.Package
	if err := a.DB.First(&pkg, id).Error; err != nil {
		notFound(c, "安装包不存在")
		return
	}
	// Only serve a package this host was actually asked to install.
	var jobs int64
	a.DB.Model(&models.DeployJob{}).
		Where("target_id = ? AND release_id = ?", target.ID, pkg.ReleaseID).
		Count(&jobs)
	if jobs == 0 {
		notFound(c, "安装包不存在")
		return
	}
	if pkg.ExternalURL != "" {
		c.Redirect(http.StatusFound, pkg.ExternalURL)
		return
	}
	if pkg.StoragePath == "" {
		notFound(c, "安装包没有本地文件")
		return
	}
	abs, err := safeStoragePath(a.Cfg.StorageDir, pkg.StoragePath)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(abs, filepath.Base(pkg.FileName))
}

func splitPreserve(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pickBackupKeep(appValue int) int {
	if appValue > 0 {
		return appValue
	}
	return 5
}
