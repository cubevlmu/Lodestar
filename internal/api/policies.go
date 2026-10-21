package api

import (
	"fmt"
	"net/http"
	"strings"

	"lodestar/internal/models"
	"lodestar/internal/versioning"

	"github.com/gin-gonic/gin"
)

func (a *API) ListPolicies(c *gin.Context) {
	query := a.DB.Model(&models.VersionPolicy{})
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		query = query.Where("project_id = ?", raw)
	}
	var policies []models.VersionPolicy
	if err := query.Order("project_id ASC, platform ASC").Find(&policies).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": policies})
}

type policyRequest struct {
	ProjectID         uint   `json:"project_id"`
	Platform          string `json:"platform"`
	Channel           string `json:"channel"`
	MinAllowedCode    int64  `json:"min_allowed_code"`
	MinAllowedVersion string `json:"min_allowed_version"`
	Message           string `json:"message"`
	Maintenance       bool   `json:"maintenance"`
	MaintenanceMsg    string `json:"maintenance_msg"`
}

func (a *API) UpsertPolicy(c *gin.Context) {
	var req policyRequest
	if !decodeJSON(c, &req) {
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if req.ProjectID == 0 || platform == "" {
		badRequest(c, "project_id 和 platform 不能为空")
		return
	}
	if channel == "" {
		channel = "stable"
	}
	if _, err := a.projectByID(req.ProjectID); err != nil {
		notFound(c, "项目不存在")
		return
	}

	code := req.MinAllowedCode
	if code <= 0 && strings.TrimSpace(req.MinAllowedVersion) != "" {
		parsed, err := versioning.ParseVersionCode(req.MinAllowedVersion)
		if err != nil {
			badRequest(c, "min_allowed_version 无法解析: "+err.Error())
			return
		}
		code = parsed
	}

	var policy models.VersionPolicy
	err := a.DB.Where("project_id = ? AND platform = ? AND channel = ?", req.ProjectID, platform, channel).First(&policy).Error
	if err != nil && !isNotFound(err) {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	policy.ProjectID = req.ProjectID
	policy.Platform = platform
	policy.Channel = channel
	policy.MinAllowedCode = code
	policy.Message = strings.TrimSpace(req.Message)
	policy.Maintenance = req.Maintenance
	policy.MaintenanceMsg = strings.TrimSpace(req.MaintenanceMsg)
	if err := a.DB.Save(&policy).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "policy.upsert", fmt.Sprintf("%d %s/%s", req.ProjectID, platform, channel))
	c.JSON(http.StatusOK, gin.H{"policy": policy})
}

func (a *API) DeletePolicy(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := a.DB.Delete(&models.VersionPolicy{}, id).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "policy.delete", fmt.Sprintf("%d", id))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (a *API) ListBlockedVersions(c *gin.Context) {
	query := a.DB.Model(&models.BlockedVersion{})
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		query = query.Where("project_id = ?", raw)
	}
	var items []models.BlockedVersion
	if err := query.Order("id DESC").Find(&items).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type blockedRequest struct {
	ProjectID   uint   `json:"project_id"`
	Platform    string `json:"platform"`
	Channel     string `json:"channel"`
	Version     string `json:"version"`
	VersionCode int64  `json:"version_code"`
	Reason      string `json:"reason"`
	Enabled     *bool  `json:"enabled"`
}

func (a *API) CreateBlockedVersion(c *gin.Context) {
	var req blockedRequest
	if !decodeJSON(c, &req) {
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if req.ProjectID == 0 || platform == "" {
		badRequest(c, "project_id 和 platform 不能为空")
		return
	}
	if channel == "" {
		channel = "stable"
	}
	code := req.VersionCode
	if code <= 0 && strings.TrimSpace(req.Version) != "" {
		parsed, err := versioning.ParseVersionCode(req.Version)
		if err != nil {
			badRequest(c, "version 无法解析: "+err.Error())
			return
		}
		code = parsed
	}
	if code <= 0 {
		badRequest(c, "需要 version 或 version_code")
		return
	}
	item := models.BlockedVersion{
		ProjectID:   req.ProjectID,
		Platform:    platform,
		Channel:     channel,
		VersionCode: code,
		Reason:      strings.TrimSpace(req.Reason),
		Enabled:     true,
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if err := a.DB.Create(&item).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "blocked.create", fmt.Sprintf("%d %s/%s %d", req.ProjectID, platform, channel, code))
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (a *API) UpdateBlockedVersion(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var item models.BlockedVersion
	if err := a.DB.First(&item, id).Error; err != nil {
		notFound(c, "记录不存在")
		return
	}
	var req blockedRequest
	if !decodeJSON(c, &req) {
		return
	}
	if req.Reason != "" {
		item.Reason = strings.TrimSpace(req.Reason)
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if err := a.DB.Save(&item).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (a *API) DeleteBlockedVersion(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := a.DB.Delete(&models.BlockedVersion{}, id).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "blocked.delete", fmt.Sprintf("%d", id))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
