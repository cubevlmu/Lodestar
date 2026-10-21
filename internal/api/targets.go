package api

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"lodestar/internal/models"
	"lodestar/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type targetAppView struct {
	models.TargetApp
	ProjectName  string `json:"project_name"`
	ProjectAppID string `json:"project_app_id"`
}

type targetListItem struct {
	targetView
	Apps []targetAppView `json:"apps"`
}

func (a *API) ListTargets(c *gin.Context) {
	query := a.DB.Model(&models.Target{})
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		// A host serves a project once it has an app record for it.
		query = query.Where("id IN (?)",
			a.DB.Model(&models.TargetApp{}).Select("target_id").Where("project_id = ?", raw))
	}
	var targets []models.Target
	if err := query.Order("id ASC").Find(&targets).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	var apps []models.TargetApp
	a.DB.Order("id ASC").Find(&apps)
	projects := map[uint]models.Project{}
	var projectList []models.Project
	a.DB.Find(&projectList)
	for _, project := range projectList {
		projects[project.ID] = project
	}

	byTarget := map[uint][]targetAppView{}
	for _, app := range apps {
		project := projects[app.ProjectID]
		byTarget[app.TargetID] = append(byTarget[app.TargetID], targetAppView{
			TargetApp:    app,
			ProjectName:  project.Name,
			ProjectAppID: project.AppID,
		})
	}

	items := make([]targetListItem, 0, len(targets))
	for _, target := range targets {
		item := targetListItem{targetView: viewTarget(target), Apps: byTarget[target.ID]}
		if item.Apps == nil {
			item.Apps = []targetAppView{}
		}
		if target.Status != "deploying" {
			if target.IsOnline(90) {
				item.Status = "online"
			} else {
				item.Status = "offline"
			}
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// targetRequest describes the host itself. Deploy paths and restart commands
// live on the per-app record, not here: one host serves many projects.
type deployConfigItem struct {
	models.TargetApp
	TargetName     string `json:"target_name"`
	TargetPlatform string `json:"target_platform"`
	TargetStatus   string `json:"target_status"`
	DeployRoot     string `json:"deploy_root"`
	ProjectName    string `json:"project_name"`
	ProjectAppID   string `json:"project_app_id"`
}

// ListDeployConfigs returns every "this project on this host" record — the
// deploy settings an operator edits on the 部署配置 page.
func (a *API) ListDeployConfigs(c *gin.Context) {
	query := a.DB.Model(&models.TargetApp{})
	if raw := strings.TrimSpace(c.Query("target_id")); raw != "" {
		query = query.Where("target_id = ?", raw)
	}
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		query = query.Where("project_id = ?", raw)
	}
	var apps []models.TargetApp
	if err := query.Order("target_id ASC, project_id ASC").Find(&apps).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	targets := map[uint]models.Target{}
	var targetList []models.Target
	a.DB.Find(&targetList)
	for _, target := range targetList {
		targets[target.ID] = target
	}
	projects := map[uint]models.Project{}
	var projectList []models.Project
	a.DB.Find(&projectList)
	for _, project := range projectList {
		projects[project.ID] = project
	}

	items := make([]deployConfigItem, 0, len(apps))
	for _, app := range apps {
		target := targets[app.TargetID]
		project := projects[app.ProjectID]
		item := deployConfigItem{
			TargetApp:      app,
			TargetName:     target.Name,
			TargetPlatform: target.Platform,
			TargetStatus:   target.Status,
			DeployRoot:     target.DeployRoot,
			ProjectName:    project.Name,
			ProjectAppID:   project.AppID,
		}
		if target.Status != "deploying" && !target.IsOnline(90) {
			item.TargetStatus = "offline"
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type targetRequest struct {
	// ProjectID is accepted for backwards compatibility and ignored.
	ProjectID  *uint  `json:"project_id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Arch       string `json:"arch"`
	Channel    string `json:"channel"`
	DeployRoot string `json:"deploy_root"`
	BackupDir  string `json:"backup_dir"`
	AutoDeploy *bool  `json:"auto_deploy"`
	Notes      string `json:"notes"`
}

func (a *API) CreateTarget(c *gin.Context) {
	var req targetRequest
	if !decodeJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	if req.Name == "" || req.Platform == "" {
		badRequest(c, "名称和平台不能为空")
		return
	}
	deployRoot, err := normalizeDeployPath(req.DeployRoot)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	backupDir, err := normalizeDeployPath(req.BackupDir)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	arch := strings.ToLower(strings.TrimSpace(req.Arch))
	if arch == "" {
		arch = "universal"
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel == "" {
		channel = "stable"
	}

	target := models.Target{
		Name:       req.Name,
		Token:      security.NewToken(32),
		Platform:   req.Platform,
		Arch:       arch,
		Channel:    channel,
		Status:     "offline",
		DeployRoot: deployRoot,
		BackupDir:  backupDir,
		AutoDeploy: true,
		Notes:      strings.TrimSpace(req.Notes),
	}
	if req.AutoDeploy != nil {
		target.AutoDeploy = *req.AutoDeploy
	}
	if err := a.DB.Create(&target).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "target.create", target.Name)
	c.JSON(http.StatusOK, gin.H{"target": viewTarget(target)})
}

func (a *API) UpdateTarget(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var target models.Target
	if err := a.DB.First(&target, id).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	var req targetRequest
	if !decodeJSON(c, &req) {
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		target.Name = name
	}
	if platform := strings.ToLower(strings.TrimSpace(req.Platform)); platform != "" {
		target.Platform = platform
	}
	if arch := strings.ToLower(strings.TrimSpace(req.Arch)); arch != "" {
		target.Arch = arch
	}
	if channel := strings.ToLower(strings.TrimSpace(req.Channel)); channel != "" {
		target.Channel = channel
	}
	deployRoot, err := normalizeDeployPath(req.DeployRoot)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	backupDir, err := normalizeDeployPath(req.BackupDir)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	target.DeployRoot = deployRoot
	target.BackupDir = backupDir
	target.Notes = strings.TrimSpace(req.Notes)
	if req.AutoDeploy != nil {
		target.AutoDeploy = *req.AutoDeploy
	}
	if err := a.DB.Save(&target).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "target.update", target.Name)
	c.JSON(http.StatusOK, gin.H{"target": viewTarget(target)})
}

func (a *API) DeleteTarget(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var target models.Target
	if err := a.DB.First(&target, id).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("target_id = ?", target.ID).Delete(&models.DeployJob{}).Error; err != nil {
			return err
		}
		if err := tx.Where("target_id = ?", target.ID).Delete(&models.TargetApp{}).Error; err != nil {
			return err
		}
		return tx.Delete(&target).Error
	}); err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "target.delete", target.Name)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (a *API) RotateTargetToken(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var target models.Target
	if err := a.DB.First(&target, id).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	target.Token = security.NewToken(32)
	if err := a.DB.Save(&target).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "target.rotate", target.Name)
	c.JSON(http.StatusOK, gin.H{"target": viewTarget(target)})
}

type targetDeployRequest struct {
	ReleaseID uint   `json:"release_id"`
	DeployDir string `json:"deploy_dir"`
	BackupDir string `json:"backup_dir"`
	Message   string `json:"message"`
}

// DeployToTarget installs a release on one host. The deploy path comes from the
// request, then from the app record, then from the host's deploy root.
func (a *API) DeployToTarget(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var target models.Target
	if err := a.DB.First(&target, id).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	var req targetDeployRequest
	if !decodeJSON(c, &req) {
		return
	}
	var release models.Release
	if err := a.DB.Preload("Package").First(&release, req.ReleaseID).Error; err != nil {
		notFound(c, "版本不存在")
		return
	}
	project, err := a.projectByID(release.ProjectID)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	app, err := a.upsertTargetApp(target, *project, targetAppSettings{
		DeployDir: req.DeployDir,
		BackupDir: req.BackupDir,
	})
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	requestedBy := "system"
	if user := CurrentUser(c); user != nil {
		requestedBy = user.Username
	}
	jobs, err := a.enqueueJobs(*project, release, []deployPlan{{Target: target, App: app}}, "manual", requestedBy, req.Message)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	if len(jobs) == 0 {
		badRequest(c, "该主机还有进行中的任务，请稍后再试")
		return
	}
	a.audit(c, "target.deploy", fmt.Sprintf("%s -> %s %s @ %s", target.Name, release.Version, release.Platform, jobDirLabel(jobs[0])))
	c.JSON(http.StatusOK, gin.H{"job": jobs[0]})
}

// targetAppRequest is the full per-app configuration: where the project lives
// on this host and how to restart it. Every field is optional.
type targetAppRequest struct {
	DeployDir      string `json:"deploy_dir"`
	BackupDir      string `json:"backup_dir"`
	StopCommand    string `json:"stop_command"`
	StartCommand   string `json:"start_command"`
	RestartCommand string `json:"restart_command"`
	HealthCheckURL string `json:"health_check_url"`
	Preserve       string `json:"preserve"`
	BackupKeep     int    `json:"backup_keep"`
}

// UpdateTargetApp edits the app record for one (host, project) pair.
func (a *API) UpdateTargetApp(c *gin.Context) {
	targetID, ok := parseID(c, "id")
	if !ok {
		return
	}
	projectID, ok := parseID(c, "project_id")
	if !ok {
		return
	}
	var target models.Target
	if err := a.DB.First(&target, targetID).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	var project models.Project
	if err := a.DB.First(&project, projectID).Error; err != nil {
		notFound(c, "项目不存在")
		return
	}
	var req targetAppRequest
	if !decodeJSON(c, &req) {
		return
	}
	deployDir, err := normalizeDeployPath(req.DeployDir)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	backupDir, err := normalizeDeployPath(req.BackupDir)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	var app models.TargetApp
	if err := a.DB.Where("target_id = ? AND project_id = ?", target.ID, project.ID).First(&app).Error; err != nil {
		if !isNotFound(err) {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
		app = models.TargetApp{TargetID: target.ID, ProjectID: project.ID}
	}
	// Replace semantics: clearing a field on purpose must clear it.
	app.DeployDir = deployDir
	app.BackupDir = backupDir
	app.StopCommand = strings.TrimSpace(req.StopCommand)
	app.StartCommand = strings.TrimSpace(req.StartCommand)
	app.RestartCommand = strings.TrimSpace(req.RestartCommand)
	app.HealthCheckURL = strings.TrimSpace(req.HealthCheckURL)
	app.Preserve = strings.TrimSpace(req.Preserve)
	app.BackupKeep = req.BackupKeep
	if app.BackupKeep <= 0 {
		app.BackupKeep = 5
	}
	if app.ID == 0 {
		if err := a.DB.Create(&app).Error; err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
	} else if err := a.DB.Save(&app).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "target.app.update", fmt.Sprintf("%s/%s", target.Name, project.AppID))
	c.JSON(http.StatusOK, gin.H{"app": app})
}

// DeleteTargetApp forgets that a project was deployed on a host. Files on the
// host are untouched; only the record goes away.
func (a *API) DeleteTargetApp(c *gin.Context) {
	targetID, ok := parseID(c, "id")
	if !ok {
		return
	}
	projectID, ok := parseID(c, "project_id")
	if !ok {
		return
	}
	result := a.DB.Where("target_id = ? AND project_id = ?", targetID, projectID).Delete(&models.TargetApp{})
	if result.Error != nil {
		failErr(c, http.StatusInternalServerError, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		notFound(c, "该主机上没有这个应用的记录")
		return
	}
	a.audit(c, "target.app.delete", fmt.Sprintf("%d/%d", targetID, projectID))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// targetAppSettings carries the optional per-app overrides coming from a request.
type targetAppSettings struct {
	DeployDir      string
	BackupDir      string
	StopCommand    string
	StartCommand   string
	RestartCommand string
	HealthCheckURL string
	Preserve       string
	BackupKeep     int
	// CommandsSet tells upsert to overwrite stored commands even when the new
	// value is empty (the operator cleared the field on purpose).
	CommandsSet bool
}

// upsertTargetApp records where a project lives on a host, creating the record
// on first deploy. Empty paths are resolved from the host defaults at payload time.
func (a *API) upsertTargetApp(target models.Target, project models.Project, settings targetAppSettings) (*models.TargetApp, error) {
	deployDir, err := normalizeDeployPath(settings.DeployDir)
	if err != nil {
		return nil, err
	}
	backupDir, err := normalizeDeployPath(settings.BackupDir)
	if err != nil {
		return nil, err
	}

	var app models.TargetApp
	err = a.DB.Where("target_id = ? AND project_id = ?", target.ID, project.ID).First(&app).Error
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	created := isNotFound(err)

	if deployDir != "" {
		app.DeployDir = deployDir
	}
	if backupDir != "" {
		app.BackupDir = backupDir
	}
	if settings.CommandsSet {
		app.StopCommand = strings.TrimSpace(settings.StopCommand)
		app.StartCommand = strings.TrimSpace(settings.StartCommand)
		app.RestartCommand = strings.TrimSpace(settings.RestartCommand)
		app.HealthCheckURL = strings.TrimSpace(settings.HealthCheckURL)
		app.Preserve = strings.TrimSpace(settings.Preserve)
		app.BackupKeep = settings.BackupKeep
	}
	app.TargetID = target.ID
	app.ProjectID = project.ID

	if created {
		if err := a.DB.Create(&app).Error; err != nil {
			return nil, err
		}
	} else if err := a.DB.Save(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

// resolveAppDir returns the absolute deploy path for a job, or "" when the agent
// should fall back to its own default.
func resolveAppDir(target models.Target, app *models.TargetApp, project models.Project) string {
	if app != nil && strings.TrimSpace(app.DeployDir) != "" {
		return strings.TrimSpace(app.DeployDir)
	}
	root := strings.TrimSpace(target.DeployRoot)
	if root == "" {
		return ""
	}
	// Deploy paths belong to the target host, which is normally Linux, so use
	// slash semantics instead of the server's own OS separators.
	return path.Join(root, project.AppID)
}

func resolveBackupDir(target models.Target, app *models.TargetApp) string {
	if app != nil && strings.TrimSpace(app.BackupDir) != "" {
		return strings.TrimSpace(app.BackupDir)
	}
	return strings.TrimSpace(target.BackupDir)
}

// normalizeDeployPath trims a path and rejects anything that climbs out of its
// parent. Relative paths are allowed (the agent resolves them against its own
// root) but ".." is not.
func normalizeDeployPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	raw := strings.ReplaceAll(value, "\\", "/")
	// Reject ".." before cleaning: path.Clean would happily collapse
	// "/srv/../../etc" into "/etc" and hide the escape attempt.
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", fmt.Errorf("部署路径不能包含 ..: %s", value)
		}
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == "/" {
		return "", fmt.Errorf("部署路径无效: %s", value)
	}
	return cleaned, nil
}

func jobDirLabel(job models.DeployJob) string {
	if job.DeployDir != "" {
		return job.DeployDir
	}
	return "(agent default)"
}

// TouchTarget records an agent heartbeat.
func (a *API) touchTarget(target *models.Target, status string) {
	now := time.Now()
	updates := map[string]any{"last_seen_at": now}
	if status != "" {
		updates["status"] = status
	}
	a.DB.Model(&models.Target{}).Where("id = ?", target.ID).Updates(updates)
}

// saveAppVersion records which version now runs on (target, project).
func (a *API) saveAppVersion(targetID, projectID uint, version string, code int64) {
	now := time.Now()
	result := a.DB.Model(&models.TargetApp{}).
		Where("target_id = ? AND project_id = ?", targetID, projectID).
		Updates(map[string]any{
			"current_version": version,
			"current_code":    code,
			"last_deploy_at":  now,
		})
	if result.Error == nil && result.RowsAffected == 0 {
		// The app record may not exist yet (e.g. a job queued by an older build).
		app := models.TargetApp{
			TargetID:       targetID,
			ProjectID:      projectID,
			CurrentVersion: version,
			CurrentCode:    code,
			LastDeployAt:   &now,
		}
		a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&app)
	}
}
