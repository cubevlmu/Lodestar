package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lodestar/internal/models"
	"lodestar/internal/security"
	"lodestar/internal/versioning"

	"gorm.io/gorm/clause"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireClientSignature authenticates an app client with HMAC-SHA256 over
// method/path/timestamp/nonce/body, plus a single-use nonce.
func (a *API) RequireClientSignature() gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			badRequest(c, "read body failed")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		appID := c.GetHeader("X-App-Id")
		timestamp := c.GetHeader("X-Timestamp")
		nonce := c.GetHeader("X-Nonce")
		signature := c.GetHeader("X-Signature")
		if appID == "" || timestamp == "" || nonce == "" || signature == "" {
			fail(c, http.StatusUnauthorized, "missing client auth headers")
			return
		}

		var project models.Project
		if err := a.DB.Where("app_id = ? AND enabled = ?", appID, true).First(&project).Error; err != nil {
			fail(c, http.StatusUnauthorized, "unknown app")
			return
		}

		n, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			fail(c, http.StatusUnauthorized, "bad timestamp")
			return
		}
		now := time.Now()
		signedAt := time.Unix(n, 0)
		if signedAt.Before(now.Add(-a.Cfg.ClientClockSkew)) || signedAt.After(now.Add(a.Cfg.ClientClockSkew)) {
			fail(c, http.StatusUnauthorized, "timestamp outside allowed window")
			return
		}

		if !security.VerifyRequestHMAC(project.ClientSecret, c.Request.Method, c.Request.URL.Path, timestamp, nonce, body, signature) {
			fail(c, http.StatusUnauthorized, "bad request signature")
			return
		}

		_ = a.DB.Where("expires_at < ?", now).Delete(&models.RequestNonce{}).Error
		record := models.RequestNonce{ProjectID: project.ID, Nonce: nonce, ExpiresAt: now.Add(a.Cfg.ClientClockSkew)}
		// DoNothing instead of relying on the unique-constraint error: a replay is
		// an expected outcome, and letting SQLite raise it would log a scary
		// "constraint failed" line for every duplicate request.
		result := a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			fail(c, http.StatusUnauthorized, "bad nonce")
			return
		}
		if result.RowsAffected == 0 {
			fail(c, http.StatusUnauthorized, "replayed nonce")
			return
		}

		c.Set(ctxProject, &project)
		c.Next()
	}
}

type CheckUpdateRequest struct {
	AppID       string `json:"app_id"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	Channel     string `json:"channel"`
	Version     string `json:"version"`
	VersionCode int64  `json:"version_code"`
	ClientID    string `json:"client_id"`
	InstallID   string `json:"install_id"`
	DeviceID    string `json:"device_id"`
	OSVersion   string `json:"os_version"`
}

type LegalInfo struct {
	Allowed bool   `json:"allowed"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PackageInfo struct {
	ID          uint   `json:"id"`
	FileName    string `json:"file_name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type UpdateInfo struct {
	Available         bool        `json:"available"`
	UpdateType        string      `json:"update_type"`
	Version           string      `json:"version"`
	VersionCode       int64       `json:"version_code"`
	Platform          string      `json:"platform"`
	Arch              string      `json:"arch"`
	Channel           string      `json:"channel"`
	Title             string      `json:"title"`
	Changelog         string      `json:"changelog"`
	MinSupportedCode  int64       `json:"min_supported_code"`
	Force             bool        `json:"force"`
	Package           PackageInfo `json:"package"`
	ManifestSignature string      `json:"manifest_signature"`
	CreatedAt         time.Time   `json:"created_at"`
}

type CheckUpdateResponse struct {
	RequestID  string      `json:"request_id"`
	ServerTime int64       `json:"server_time"`
	Legal      LegalInfo   `json:"legal"`
	Update     *UpdateInfo `json:"update,omitempty"`
}

func (a *API) CheckUpdate(c *gin.Context) {
	project := CurrentProject(c)
	if project == nil {
		fail(c, http.StatusUnauthorized, "missing project context")
		return
	}

	var req CheckUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.AppID != "" && req.AppID != project.AppID {
		badRequest(c, "app_id does not match X-App-Id")
		return
	}
	if strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.Version) == "" {
		badRequest(c, "platform and version are required")
		return
	}
	req.Platform = versioning.NormalizePlatform(req.Platform)
	req.Arch = versioning.NormalizeArch(req.Arch)
	req.Channel = strings.ToLower(strings.TrimSpace(req.Channel))
	if req.Channel == "" {
		req.Channel = "stable"
	}

	legal := a.evaluateLegal(project.ID, req)
	latest, pkg, found := a.findLatestRelease(project.ID, req)

	resp := CheckUpdateResponse{
		RequestID:  uuid.NewString(),
		ServerTime: time.Now().Unix(),
		Legal:      legal,
	}
	if found {
		info, err := a.buildUpdateInfo(c, *project, req, latest, pkg, legal)
		if err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
		resp.Update = &info
	} else {
		resp.Update = &UpdateInfo{Available: false, UpdateType: "none"}
	}
	c.JSON(http.StatusOK, resp)
}

func (a *API) DownloadPackage(c *gin.Context) {
	id, ok := parseID(c, "package_id")
	if !ok {
		return
	}
	var pkg models.Package
	if err := a.DB.First(&pkg, id).Error; err != nil {
		notFound(c, "package not found")
		return
	}
	token := c.Query("token")
	if token == "" {
		fail(c, http.StatusUnauthorized, "missing token")
		return
	}
	if err := security.VerifyDownloadToken(a.Cfg.DownloadTokenSecret, pkg.ID, "", token); err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	if pkg.ExternalURL != "" {
		c.Redirect(http.StatusFound, pkg.ExternalURL)
		return
	}
	if pkg.StoragePath == "" {
		notFound(c, "package has no local file")
		return
	}
	abs, err := safeStoragePath(a.Cfg.StorageDir, pkg.StoragePath)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(abs, pkg.FileName)
}

func (a *API) evaluateLegal(projectID uint, req CheckUpdateRequest) LegalInfo {
	var policy models.VersionPolicy
	if err := a.DB.Where("project_id = ? AND platform = ? AND channel = ?", projectID, req.Platform, req.Channel).First(&policy).Error; err == nil {
		if policy.Maintenance {
			message := policy.MaintenanceMsg
			if message == "" {
				message = "service is under maintenance"
			}
			return LegalInfo{Allowed: false, Code: "maintenance", Message: message}
		}
		if policy.MinAllowedCode > 0 && req.VersionCode < policy.MinAllowedCode {
			message := policy.Message
			if message == "" {
				message = "this app version is no longer supported"
			}
			return LegalInfo{Allowed: false, Code: "version_too_old", Message: message}
		}
	}

	var blocked models.BlockedVersion
	if err := a.DB.Where(
		"project_id = ? AND platform = ? AND channel = ? AND version_code = ? AND enabled = ?",
		projectID, req.Platform, req.Channel, req.VersionCode, true,
	).First(&blocked).Error; err == nil {
		message := blocked.Reason
		if message == "" {
			message = "this app version has been blocked"
		}
		return LegalInfo{Allowed: false, Code: "version_blocked", Message: message}
	}

	return LegalInfo{Allowed: true, Code: "ok", Message: "ok"}
}

func (a *API) findLatestRelease(projectID uint, req CheckUpdateRequest) (models.Release, models.Package, bool) {
	var releases []models.Release
	err := a.DB.Preload("Package").Where(
		"project_id = ? AND platform = ? AND channel = ? AND enabled = ? AND version_code > ? AND (arch = ? OR arch = ? OR arch = '')",
		projectID, req.Platform, req.Channel, true, req.VersionCode, req.Arch, "universal",
	).Order("version_code DESC").Limit(20).Find(&releases).Error
	if err != nil || len(releases) == 0 {
		return models.Release{}, models.Package{}, false
	}

	clientKey := req.ClientID
	if clientKey == "" {
		clientKey = req.InstallID
	}
	if clientKey == "" {
		clientKey = req.DeviceID
	}

	for _, release := range releases {
		if release.RolloutPercent <= 0 {
			continue
		}
		if release.RolloutPercent >= 100 ||
			security.StablePercent(fmt.Sprintf("%d:%s:%d", projectID, clientKey, release.VersionCode)) < release.RolloutPercent {
			return release, release.Package, true
		}
	}
	return models.Release{}, models.Package{}, false
}

func (a *API) buildUpdateInfo(c *gin.Context, project models.Project, req CheckUpdateRequest, release models.Release, pkg models.Package, legal LegalInfo) (UpdateInfo, error) {
	clientKey := req.ClientID
	if clientKey == "" {
		clientKey = req.InstallID
	}
	if clientKey == "" {
		clientKey = req.DeviceID
	}

	downloadURL := pkg.ExternalURL
	if downloadURL == "" {
		token := security.IssueDownloadToken(a.Cfg.DownloadTokenSecret, pkg.ID, clientKey, a.Cfg.DownloadTokenTTL)
		downloadURL = fmt.Sprintf("%s/api/v1/packages/%d/download?token=%s", a.publicBaseURL(c), pkg.ID, token)
	}

	force := release.Force || !legal.Allowed || (release.MinSupportedCode > 0 && req.VersionCode < release.MinSupportedCode)
	updateType := "recommended"
	if force {
		updateType = "force"
	}

	payload := security.ManifestPayload{
		AppID:            project.AppID,
		Version:          release.Version,
		VersionCode:      release.VersionCode,
		Platform:         release.Platform,
		Arch:             release.Arch,
		Channel:          release.Channel,
		PackageURL:       downloadURL,
		PackageSize:      pkg.Size,
		PackageSHA256:    pkg.SHA256,
		Force:            force,
		MinSupportedCode: release.MinSupportedCode,
	}
	sig, err := security.SignManifest(project.Ed25519PrivateKey, payload)
	if err != nil {
		return UpdateInfo{}, err
	}

	return UpdateInfo{
		Available:        true,
		UpdateType:       updateType,
		Version:          release.Version,
		VersionCode:      release.VersionCode,
		Platform:         release.Platform,
		Arch:             release.Arch,
		Channel:          release.Channel,
		Title:            release.Title,
		Changelog:        release.Changelog,
		MinSupportedCode: release.MinSupportedCode,
		Force:            force,
		Package: PackageInfo{
			ID:          pkg.ID,
			FileName:    pkg.FileName,
			DownloadURL: downloadURL,
			Size:        pkg.Size,
			SHA256:      pkg.SHA256,
		},
		ManifestSignature: sig,
		CreatedAt:         release.CreatedAt,
	}, nil
}

func safeStoragePath(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", errors.New("bad storage path")
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cleanPath, err := filepath.Abs(filepath.Join(cleanRoot, filepath.Clean(filepath.FromSlash(rel))))
	if err != nil {
		return "", err
	}
	if cleanPath != cleanRoot && !strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) {
		return "", errors.New("storage path escapes root")
	}
	return cleanPath, nil
}
