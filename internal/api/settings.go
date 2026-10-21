package api

import (
	"net/http"
	"strings"
	"time"

	"lodestar/internal/buildinfo"
	"lodestar/internal/models"

	"github.com/gin-gonic/gin"
)

func (a *API) GetSettings(c *gin.Context) {
	var packageCount, releaseCount, jobCount int64
	a.DB.Model(&models.Package{}).Count(&packageCount)
	a.DB.Model(&models.Release{}).Count(&releaseCount)
	a.DB.Model(&models.DeployJob{}).Count(&jobCount)

	c.JSON(http.StatusOK, gin.H{
		"version":            buildinfo.Version,
		"commit":             buildinfo.Commit,
		"started_at":         a.Started,
		"uptime":             int64(time.Since(a.Started).Seconds()),
		"public_base_url":    a.setting("public_base_url"),
		"effective_base_url": a.publicBaseURL(c),
		"storage_dir":        a.Cfg.StorageDir,
		"db_path":            a.Cfg.DBPath,
		"data_dir":           a.Cfg.DataDir,
		"session_ttl_hours":  int(a.Cfg.SessionTTL.Hours()),
		"max_upload_bytes":   a.Cfg.MaxUploadBytes,
		"package_count":      packageCount,
		"release_count":      releaseCount,
		"job_count":          jobCount,
	})
}

type settingsRequest struct {
	PublicBaseURL string `json:"public_base_url"`
}

func (a *API) UpdateSettings(c *gin.Context) {
	var req settingsRequest
	if !decodeJSON(c, &req) {
		return
	}
	value := strings.TrimRight(strings.TrimSpace(req.PublicBaseURL), "/")
	if value != "" && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		badRequest(c, "public_base_url 必须以 http:// 或 https:// 开头")
		return
	}
	if err := a.saveSetting("public_base_url", value); err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "settings.update", "public_base_url="+value)
	c.JSON(http.StatusOK, gin.H{"public_base_url": value})
}

func (a *API) ListAuditLogs(c *gin.Context) {
	page, size := pageParams(c)
	query := a.DB.Model(&models.AuditLog{})
	var total int64
	query.Count(&total)
	var items []models.AuditLog
	query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}
