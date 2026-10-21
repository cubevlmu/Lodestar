package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lodestar/internal/models"
	"lodestar/internal/versioning"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type releaseMeta struct {
	Version          string
	Platform         string
	Arch             string
	Channel          string
	Title            string
	Changelog        string
	MinSupportedCode int64
	Force            bool
	Enabled          bool
	RolloutPercent   int
	Source           string
	CreatedBy        string
}

func normaliseMeta(meta *releaseMeta) error {
	meta.Version = strings.TrimSpace(meta.Version)
	meta.Platform = versioning.NormalizePlatform(meta.Platform)
	meta.Arch = versioning.NormalizeArch(meta.Arch)
	meta.Channel = strings.ToLower(strings.TrimSpace(meta.Channel))
	meta.Title = strings.TrimSpace(meta.Title)
	if meta.Version == "" {
		return fmt.Errorf("version 不能为空")
	}
	if meta.Platform == "" {
		return fmt.Errorf("platform 不能为空")
	}
	if meta.Arch == "" {
		meta.Arch = "universal"
	}
	if meta.Channel == "" {
		meta.Channel = "stable"
	}
	if meta.RolloutPercent <= 0 {
		meta.RolloutPercent = 100
	}
	if meta.RolloutPercent > 100 {
		meta.RolloutPercent = 100
	}
	if meta.Source == "" {
		meta.Source = "manual"
	}
	return nil
}

func (a *API) ListReleases(c *gin.Context) {
	page, size := pageParams(c)
	query := a.DB.Model(&models.Release{}).Preload("Package")

	if raw := c.Query("project_id"); raw != "" {
		if id, err := strconv.ParseUint(raw, 10, 64); err == nil {
			query = query.Where("project_id = ?", id)
		}
	}
	if value := strings.TrimSpace(c.Query("platform")); value != "" {
		query = query.Where("platform = ?", strings.ToLower(value))
	}
	if value := strings.TrimSpace(c.Query("channel")); value != "" {
		query = query.Where("channel = ?", strings.ToLower(value))
	}
	if value := strings.TrimSpace(c.Query("q")); value != "" {
		like := "%" + value + "%"
		query = query.Where("version LIKE ? OR title LIKE ? OR changelog LIKE ?", like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	var releases []models.Release
	if err := query.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&releases).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": releases, "total": total, "page": page, "size": size})
}

func (a *API) CreateRelease(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.Cfg.MaxUploadBytes)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		badRequest(c, "上传内容解析失败（可能超过大小限制）: "+err.Error())
		return
	}

	projectID, err := strconv.ParseUint(c.PostForm("project_id"), 10, 64)
	if err != nil || projectID == 0 {
		badRequest(c, "project_id 无效")
		return
	}
	project, err := a.projectByID(uint(projectID))
	if err != nil {
		notFound(c, "项目不存在")
		return
	}

	meta := releaseMeta{
		Version:          c.PostForm("version"),
		Platform:         c.PostForm("platform"),
		Arch:             c.PostForm("arch"),
		Channel:          c.PostForm("channel"),
		Title:            c.PostForm("title"),
		Changelog:        c.PostForm("changelog"),
		MinSupportedCode: parseInt64(c.PostForm("min_supported_code")),
		Force:            parseBool(c.PostForm("force")),
		Enabled:          parseBoolDefault(c.PostForm("enabled"), true),
		RolloutPercent:   int(parseInt64(c.PostForm("rollout_percent"))),
		Source:           "manual",
	}
	if err := normaliseMeta(&meta); err != nil {
		badRequest(c, err.Error())
		return
	}
	if user := CurrentUser(c); user != nil {
		meta.CreatedBy = user.Username
	}

	file, header, err := c.Request.FormFile("package")
	if err != nil {
		badRequest(c, "缺少 package 文件")
		return
	}
	defer file.Close()

	release, pkg, err := a.createReleaseWithFile(project, meta, file, header)
	if err != nil {
		if status, ok := err.(httpError); ok {
			fail(c, status.status, status.message)
			return
		}
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "release.create", fmt.Sprintf("%s %s %s", project.AppID, release.Version, release.Platform))

	response := gin.H{"release": release, "package": pkg}
	if parseBool(c.PostForm("deploy")) {
		plans, planErr := a.planReleaseDeploy(*project, release, deployReleaseRequest{
			TargetIDs: splitIDs(c.PostForm("target_ids")),
			DeployDir: c.PostForm("deploy_dir"),
		})
		if planErr != nil {
			badRequest(c, planErr.Error())
			return
		}
		jobs, err := a.enqueueJobs(*project, release, plans, "manual", meta.CreatedBy, meta.Changelog)
		if err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
		response["jobs"] = jobs
	}
	c.JSON(http.StatusOK, response)
}

type externalReleaseRequest struct {
	ProjectID        uint   `json:"project_id"`
	Version          string `json:"version"`
	Platform         string `json:"platform"`
	Arch             string `json:"arch"`
	Channel          string `json:"channel"`
	Title            string `json:"title"`
	Changelog        string `json:"changelog"`
	MinSupportedCode int64  `json:"min_supported_code"`
	Force            bool   `json:"force"`
	Enabled          *bool  `json:"enabled"`
	RolloutPercent   int    `json:"rollout_percent"`
	ExternalURL      string `json:"external_url"`
	FileName         string `json:"file_name"`
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
}

// CreateReleaseExternal registers a release whose artifact lives somewhere else (CDN, R2, GitHub Releases).
func (a *API) CreateReleaseExternal(c *gin.Context) {
	var req externalReleaseRequest
	if !decodeJSON(c, &req) {
		return
	}
	project, err := a.projectByID(req.ProjectID)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	meta := releaseMeta{
		Version:          req.Version,
		Platform:         req.Platform,
		Arch:             req.Arch,
		Channel:          req.Channel,
		Title:            req.Title,
		Changelog:        req.Changelog,
		MinSupportedCode: req.MinSupportedCode,
		Force:            req.Force,
		Enabled:          true,
		RolloutPercent:   req.RolloutPercent,
		Source:           "manual",
	}
	if req.Enabled != nil {
		meta.Enabled = *req.Enabled
	}
	if err := normaliseMeta(&meta); err != nil {
		badRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(req.ExternalURL) == "" {
		badRequest(c, "external_url 不能为空")
		return
	}
	if user := CurrentUser(c); user != nil {
		meta.CreatedBy = user.Username
	}

	name := strings.TrimSpace(req.FileName)
	if name == "" {
		name = filepath.Base(strings.Split(req.ExternalURL, "?")[0])
	}
	pkg := models.Package{
		FileName:    name,
		ExternalURL: strings.TrimSpace(req.ExternalURL),
		Size:        req.Size,
		SHA256:      strings.ToLower(strings.TrimSpace(req.SHA256)),
	}
	release, err := a.createReleaseRecord(project, meta, pkg)
	if err != nil {
		if status, ok := err.(httpError); ok {
			fail(c, status.status, status.message)
			return
		}
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "release.create_external", fmt.Sprintf("%s %s", project.AppID, release.Version))
	c.JSON(http.StatusOK, gin.H{"release": release})
}

type httpError struct {
	status  int
	message string
}

func (e httpError) Error() string { return e.message }

func (a *API) createReleaseWithFile(project *models.Project, meta releaseMeta, file multipart.File, header *multipart.FileHeader) (models.Release, models.Package, error) {
	versionCode, err := versioning.ParseVersionCode(meta.Version)
	if err != nil {
		return models.Release{}, models.Package{}, httpError{http.StatusBadRequest, "version 无法解析: " + err.Error()}
	}
	var duplicate int64
	a.DB.Model(&models.Release{}).Where(
		"project_id = ? AND platform = ? AND arch = ? AND channel = ? AND version_code = ?",
		project.ID, meta.Platform, meta.Arch, meta.Channel, versionCode,
	).Count(&duplicate)
	if duplicate > 0 {
		return models.Release{}, models.Package{}, httpError{http.StatusConflict, "该版本已存在（同一平台/架构/通道）"}
	}

	fileName := sanitizeFileName(header.Filename)
	relPath := filepath.Join(project.AppID, meta.Platform, meta.Channel, meta.Arch, meta.Version, fileName)
	absPath := filepath.Join(a.Cfg.StorageDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return models.Release{}, models.Package{}, err
	}
	absPath = dedupePath(absPath)

	out, err := os.Create(absPath)
	if err != nil {
		return models.Release{}, models.Package{}, err
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(out, hasher), file)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(absPath)
		return models.Release{}, models.Package{}, copyErr
	}
	if closeErr != nil {
		os.Remove(absPath)
		return models.Release{}, models.Package{}, closeErr
	}

	relative, err := filepath.Rel(a.Cfg.StorageDir, absPath)
	if err != nil {
		relative = relPath
	}
	pkg := models.Package{
		FileName:    fileName,
		StoragePath: filepath.ToSlash(relative),
		Size:        size,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
	}

	release, err := a.createReleaseRecord(project, meta, pkg)
	if err != nil {
		os.Remove(absPath)
		return models.Release{}, models.Package{}, err
	}
	return release, release.Package, nil
}

func (a *API) createReleaseRecord(project *models.Project, meta releaseMeta, pkg models.Package) (models.Release, error) {
	versionCode, err := versioning.ParseVersionCode(meta.Version)
	if err != nil {
		return models.Release{}, httpError{http.StatusBadRequest, "version 无法解析: " + err.Error()}
	}
	release := models.Release{
		ProjectID:        project.ID,
		Version:          meta.Version,
		VersionCode:      versionCode,
		Platform:         meta.Platform,
		Arch:             meta.Arch,
		Channel:          meta.Channel,
		Title:            meta.Title,
		Changelog:        meta.Changelog,
		MinSupportedCode: meta.MinSupportedCode,
		Force:            meta.Force,
		Enabled:          meta.Enabled,
		RolloutPercent:   meta.RolloutPercent,
		Source:           meta.Source,
		CreatedBy:        meta.CreatedBy,
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&release).Error; err != nil {
			return err
		}
		pkg.ReleaseID = release.ID
		if err := tx.Create(&pkg).Error; err != nil {
			return err
		}
		release.Package = pkg
		return nil
	})
	if err != nil {
		return models.Release{}, err
	}
	return release, nil
}

type releasePatch struct {
	Title            *string `json:"title"`
	Changelog        *string `json:"changelog"`
	Enabled          *bool   `json:"enabled"`
	Force            *bool   `json:"force"`
	RolloutPercent   *int    `json:"rollout_percent"`
	MinSupportedCode *int64  `json:"min_supported_code"`
}

func (a *API) UpdateRelease(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var release models.Release
	if err := a.DB.First(&release, id).Error; err != nil {
		notFound(c, "版本不存在")
		return
	}
	var req releasePatch
	if !decodeJSON(c, &req) {
		return
	}
	updates := map[string]any{}
	if req.Title != nil {
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Changelog != nil {
		updates["changelog"] = *req.Changelog
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Force != nil {
		updates["force"] = *req.Force
	}
	if req.RolloutPercent != nil {
		value := *req.RolloutPercent
		if value < 0 {
			value = 0
		}
		if value > 100 {
			value = 100
		}
		updates["rollout_percent"] = value
	}
	if req.MinSupportedCode != nil {
		updates["min_supported_code"] = *req.MinSupportedCode
	}
	if len(updates) == 0 {
		badRequest(c, "没有需要更新的字段")
		return
	}
	if err := a.DB.Model(&release).Updates(updates).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.DB.Preload("Package").First(&release, id)
	a.audit(c, "release.update", fmt.Sprintf("%d %s", release.ID, release.Version))
	c.JSON(http.StatusOK, gin.H{"release": release})
}

func (a *API) DeleteRelease(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var release models.Release
	if err := a.DB.Preload("Package").First(&release, id).Error; err != nil {
		notFound(c, "版本不存在")
		return
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_id = ?", release.ID).Delete(&models.Package{}).Error; err != nil {
			return err
		}
		return tx.Delete(&release).Error
	})
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	if release.Package.StoragePath != "" {
		path := filepath.Join(a.Cfg.StorageDir, filepath.FromSlash(release.Package.StoragePath))
		if strings.HasPrefix(path, filepath.Clean(a.Cfg.StorageDir)) {
			os.Remove(path)
		}
	}
	a.audit(c, "release.delete", fmt.Sprintf("%d %s", release.ID, release.Version))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// releaseTargetSpec lets one release go to several hosts, each with its own
// install path (an agent usually serves more than one application).
type releaseTargetSpec struct {
	TargetID  uint   `json:"target_id"`
	DeployDir string `json:"deploy_dir"`
	BackupDir string `json:"backup_dir"`
}

type deployReleaseRequest struct {
	// Targets is the modern form; TargetIDs is kept for older clients.
	Targets   []releaseTargetSpec `json:"targets"`
	TargetIDs []uint              `json:"target_ids"`
	// DeployDir applies to every selected target when set.
	DeployDir string `json:"deploy_dir"`
	Message   string `json:"message"`
}

// DeployRelease pushes an existing release to selected (or all matching) targets.
func (a *API) DeployRelease(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var release models.Release
	if err := a.DB.Preload("Package").First(&release, id).Error; err != nil {
		notFound(c, "版本不存在")
		return
	}
	project, err := a.projectByID(release.ProjectID)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	var req deployReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		badRequest(c, "请求体格式错误")
		return
	}
	plans, err := a.planReleaseDeploy(*project, release, req)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if len(plans) == 0 {
		badRequest(c, "没有匹配的目标主机，请先在“目标主机”中登记")
		return
	}
	requestedBy := "system"
	if user := CurrentUser(c); user != nil {
		requestedBy = user.Username
	}
	jobs, err := a.enqueueJobs(*project, release, plans, "manual", requestedBy, req.Message)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "release.deploy", fmt.Sprintf("%s %s -> %d targets", project.AppID, release.Version, len(jobs)))
	c.JSON(http.StatusOK, gin.H{"jobs": jobs, "count": len(jobs)})
}

// planReleaseDeploy resolves which hosts get the release and where it goes on
// each of them. Explicit specs win; otherwise every auto-deploy host that
// matches the release platform/channel is used.
func (a *API) planReleaseDeploy(project models.Project, release models.Release, req deployReleaseRequest) ([]deployPlan, error) {
	specs := req.Targets
	if len(specs) == 0 && len(req.TargetIDs) > 0 {
		for _, id := range req.TargetIDs {
			specs = append(specs, releaseTargetSpec{TargetID: id})
		}
	}

	var targets []models.Target
	if len(specs) > 0 {
		ids := make([]uint, 0, len(specs))
		for _, spec := range specs {
			ids = append(ids, spec.TargetID)
		}
		if err := a.DB.Where("id IN ?", ids).Order("id ASC").Find(&targets).Error; err != nil {
			return nil, err
		}
	} else {
		if err := a.DB.Where(
			"auto_deploy = ? AND platform = ? AND channel = ?",
			true, release.Platform, release.Channel,
		).Order("id ASC").Find(&targets).Error; err != nil {
			return nil, err
		}
	}

	byID := make(map[uint]models.Target, len(targets))
	for _, target := range targets {
		byID[target.ID] = target
	}

	plans := make([]deployPlan, 0, len(targets))
	for _, spec := range specs {
		target, ok := byID[spec.TargetID]
		if !ok {
			continue
		}
		deployDir := spec.DeployDir
		if strings.TrimSpace(deployDir) == "" {
			deployDir = req.DeployDir
		}
		app, err := a.upsertTargetApp(target, project, targetAppSettings{
			DeployDir: deployDir,
			BackupDir: spec.BackupDir,
		})
		if err != nil {
			return nil, err
		}
		plans = append(plans, deployPlan{Target: target, App: app})
	}
	if len(specs) > 0 {
		return plans, nil
	}

	for _, target := range targets {
		app, err := a.upsertTargetApp(target, project, targetAppSettings{DeployDir: req.DeployDir})
		if err != nil {
			return nil, err
		}
		plans = append(plans, deployPlan{Target: target, App: app})
	}
	return plans, nil
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." {
		return "package.bin"
	}
	return name
}

func dedupePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s-%s%s", base, time.Now().Format("20060102150405"), ext)
}

func parseInt64(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func parseBoolDefault(value string, fallback bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return parseBool(value)
}

func splitIDs(value string) []uint {
	var ids []uint
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseUint(part, 10, 64); err == nil && n > 0 {
			ids = append(ids, uint(n))
		}
	}
	return ids
}
