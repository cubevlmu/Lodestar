package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"lodestar/internal/models"
	"lodestar/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var appIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,127}$`)

type projectListItem struct {
	projectView
	ReleaseCount  int64  `json:"release_count"`
	TargetCount   int64  `json:"target_count"`
	OnlineTargets int64  `json:"online_targets"`
	LatestVersion string `json:"latest_version"`
}

func (a *API) ListProjects(c *gin.Context) {
	var projects []models.Project
	if err := a.DB.Order("id ASC").Find(&projects).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	items := make([]projectListItem, 0, len(projects))
	for _, project := range projects {
		item := projectListItem{projectView: viewProject(project)}
		a.DB.Model(&models.Release{}).Where("project_id = ?", project.ID).Count(&item.ReleaseCount)
		a.DB.Model(&models.Target{}).Where("project_id = ?", project.ID).Count(&item.TargetCount)
		a.DB.Model(&models.Target{}).Where("project_id = ? AND last_seen_at > ?", project.ID, time.Now().Add(-90*time.Second)).Count(&item.OnlineTargets)

		var latest models.Release
		if err := a.DB.Where("project_id = ?", project.ID).Order("version_code DESC").First(&latest).Error; err == nil {
			item.LatestVersion = latest.Version
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type projectRequest struct {
	AppID       string `json:"app_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	RepoURL     string `json:"repo_url"`
	Enabled     *bool  `json:"enabled"`
}

func (a *API) CreateProject(c *gin.Context) {
	var req projectRequest
	if !decodeJSON(c, &req) {
		return
	}
	appID := strings.ToLower(strings.TrimSpace(req.AppID))
	name := strings.TrimSpace(req.Name)
	if !appIDPattern.MatchString(appID) {
		badRequest(c, "app_id 只能包含小写字母、数字、点、下划线和短横线，长度 2-128")
		return
	}
	if name == "" {
		name = appID
	}

	var existing int64
	a.DB.Model(&models.Project{}).Where("app_id = ?", appID).Count(&existing)
	if existing > 0 {
		fail(c, http.StatusConflict, "app_id 已存在")
		return
	}

	clientSecret, err := security.RandomBase64(32)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	pub, priv, err := security.GenerateEd25519KeyPair()
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	project := models.Project{
		AppID:             appID,
		Name:              name,
		Description:       strings.TrimSpace(req.Description),
		RepoURL:           strings.TrimSpace(req.RepoURL),
		ClientSecret:      clientSecret,
		DeployToken:       security.NewToken(32),
		Ed25519PublicKey:  pub,
		Ed25519PrivateKey: priv,
		Enabled:           true,
	}
	if err := a.DB.Create(&project).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "project.create", project.AppID)
	c.JSON(http.StatusOK, gin.H{"project": viewProject(project)})
}

func (a *API) GetProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := a.projectByID(id)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": viewProject(*project)})
}

func (a *API) UpdateProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := a.projectByID(id)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	var req projectRequest
	if !decodeJSON(c, &req) {
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		project.Name = name
	}
	project.Description = strings.TrimSpace(req.Description)
	project.RepoURL = strings.TrimSpace(req.RepoURL)
	if req.Enabled != nil {
		project.Enabled = *req.Enabled
	}
	if err := a.DB.Save(project).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "project.update", project.AppID)
	c.JSON(http.StatusOK, gin.H{"project": viewProject(*project)})
}

func (a *API) DeleteProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := a.projectByID(id)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.DeployLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.DeployJob{}).Error; err != nil {
			return err
		}
		if err := tx.Where("release_id IN (SELECT id FROM releases WHERE project_id = ?)", project.ID).Delete(&models.Package{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.Release{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.Target{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.VersionPolicy{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.BlockedVersion{}).Error; err != nil {
			return err
		}
		return tx.Delete(project).Error
	})
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "project.delete", project.AppID)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type rotateRequest struct {
	Kind string `json:"kind"` // client_secret | deploy_token | ed25519
}

func (a *API) RotateProjectKey(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := a.projectByID(id)
	if err != nil {
		notFound(c, "项目不存在")
		return
	}
	var req rotateRequest
	if !decodeJSON(c, &req) {
		return
	}

	switch req.Kind {
	case "client_secret":
		secret, err := security.RandomBase64(32)
		if err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
		project.ClientSecret = secret
	case "deploy_token":
		project.DeployToken = security.NewToken(32)
	case "ed25519":
		pub, priv, err := security.GenerateEd25519KeyPair()
		if err != nil {
			failErr(c, http.StatusInternalServerError, err)
			return
		}
		project.Ed25519PublicKey = pub
		project.Ed25519PrivateKey = priv
	default:
		badRequest(c, "kind 必须是 client_secret、deploy_token 或 ed25519")
		return
	}

	if err := a.DB.Save(project).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "project.rotate", fmt.Sprintf("%s %s", project.AppID, req.Kind))
	c.JSON(http.StatusOK, gin.H{"project": viewProject(*project)})
}
