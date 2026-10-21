package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"lodestar/internal/models"

	"github.com/gin-gonic/gin"
)

type ciDeployResponse struct {
	Release models.Release     `json:"release"`
	Jobs    []models.DeployJob `json:"jobs"`
	Status  string             `json:"status"`
	Message string             `json:"message,omitempty"`
}

// CIDeploy is the endpoint GitHub Actions (or any CI) calls after a successful build.
// It registers the release and queues a deployment on every matching target.
func (a *API) CIDeploy(c *gin.Context) {
	project := CurrentProject(c)
	if project == nil {
		fail(c, http.StatusUnauthorized, "missing project context")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.Cfg.MaxUploadBytes)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		badRequest(c, "上传内容解析失败（可能超过大小限制）: "+err.Error())
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
		Source:           "ci",
		CreatedBy:        "ci:" + project.AppID,
	}
	if err := normaliseMeta(&meta); err != nil {
		badRequest(c, err.Error())
		return
	}

	file, header, err := c.Request.FormFile("package")
	if err != nil {
		badRequest(c, "缺少 package 文件")
		return
	}
	defer file.Close()

	release, _, err := a.createReleaseWithFile(project, meta, file, header)
	if err != nil {
		if status, ok := err.(httpError); ok {
			fail(c, status.status, status.message)
			return
		}
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	response := ciDeployResponse{Release: release, Status: "registered", Jobs: []models.DeployJob{}}
	if !parseBoolDefault(c.PostForm("deploy"), true) {
		c.JSON(http.StatusOK, response)
		return
	}

	plans, planErr := a.planReleaseDeploy(*project, release, deployReleaseRequest{
		TargetIDs: splitIDs(c.PostForm("target_ids")),
		DeployDir: c.PostForm("deploy_dir"),
	})
	if planErr != nil {
		badRequest(c, planErr.Error())
		return
	}
	if len(plans) == 0 {
		response.Status = "no_targets"
		response.Message = "release registered but no auto-deploy target matched this platform/channel"
		c.JSON(http.StatusOK, response)
		return
	}
	jobs, err := a.enqueueJobs(*project, release, plans, "ci", meta.CreatedBy, meta.Changelog)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	if len(jobs) == 0 {
		response.Status = "busy"
		response.Message = "every matching target already has a queued or running deployment"
		c.JSON(http.StatusOK, response)
		return
	}
	a.audit(c, "deploy.ci", project.AppID+" "+release.Version)

	response.Jobs = jobs
	response.Status = "queued"

	waitSeconds, _ := strconv.Atoi(c.PostForm("wait_seconds"))
	if waitSeconds > 0 {
		if waitSeconds > 600 {
			waitSeconds = 600
		}
		jobs = a.waitForJobs(jobIDs(jobs), time.Duration(waitSeconds)*time.Second)
		response.Jobs = jobs
		response.Status = aggregateStatus(jobs)
	}
	c.JSON(http.StatusOK, response)
}

type externalCIDeployRequest struct {
	Version          string `json:"version"`
	Platform         string `json:"platform"`
	Arch             string `json:"arch"`
	Channel          string `json:"channel"`
	Title            string `json:"title"`
	Changelog        string `json:"changelog"`
	MinSupportedCode int64  `json:"min_supported_code"`
	Force            bool   `json:"force"`
	RolloutPercent   int    `json:"rollout_percent"`
	ExternalURL      string `json:"external_url"`
	FileName         string `json:"file_name"`
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
	Deploy           *bool  `json:"deploy"`
	TargetIDs        []uint `json:"target_ids"`
	// DeployDir is where to install on every selected host; omit to use the
	// path already recorded for that host.
	DeployDir   string `json:"deploy_dir"`
	WaitSeconds int    `json:"wait_seconds"`
}

// CIDeployExternal registers a release whose artifact is hosted elsewhere.
func (a *API) CIDeployExternal(c *gin.Context) {
	project := CurrentProject(c)
	if project == nil {
		fail(c, http.StatusUnauthorized, "missing project context")
		return
	}
	var req externalCIDeployRequest
	if !decodeJSON(c, &req) {
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
		Source:           "ci",
		CreatedBy:        "ci:" + project.AppID,
	}
	if err := normaliseMeta(&meta); err != nil {
		badRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(req.ExternalURL) == "" {
		badRequest(c, "external_url 不能为空")
		return
	}
	name := strings.TrimSpace(req.FileName)
	if name == "" {
		name = "package.bin"
	}
	release, err := a.createReleaseRecord(project, meta, models.Package{
		FileName:    name,
		ExternalURL: strings.TrimSpace(req.ExternalURL),
		Size:        req.Size,
		SHA256:      strings.ToLower(strings.TrimSpace(req.SHA256)),
	})
	if err != nil {
		if status, ok := err.(httpError); ok {
			fail(c, status.status, status.message)
			return
		}
		failErr(c, http.StatusInternalServerError, err)
		return
	}

	response := ciDeployResponse{Release: release, Status: "registered", Jobs: []models.DeployJob{}}
	deploy := true
	if req.Deploy != nil {
		deploy = *req.Deploy
	}
	if !deploy {
		c.JSON(http.StatusOK, response)
		return
	}
	plans, planErr := a.planReleaseDeploy(*project, release, deployReleaseRequest{
		TargetIDs: req.TargetIDs,
		DeployDir: req.DeployDir,
	})
	if planErr != nil {
		badRequest(c, planErr.Error())
		return
	}
	jobs, err := a.enqueueJobs(*project, release, plans, "ci", meta.CreatedBy, meta.Changelog)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	response.Jobs = jobs
	response.Status = "queued"
	if req.WaitSeconds > 0 && len(jobs) > 0 {
		if req.WaitSeconds > 600 {
			req.WaitSeconds = 600
		}
		jobs = a.waitForJobs(jobIDs(jobs), time.Duration(req.WaitSeconds)*time.Second)
		response.Jobs = jobs
		response.Status = aggregateStatus(jobs)
	}
	c.JSON(http.StatusOK, response)
}

// CIDeployStatus lets CI poll a job it created.
func (a *API) CIDeployStatus(c *gin.Context) {
	project := CurrentProject(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.Preload("Target").First(&job, id).Error; err != nil || job.ProjectID != project.ID {
		notFound(c, "部署任务不存在")
		return
	}
	var logs []models.DeployLog
	a.DB.Where("job_id = ?", job.ID).Order("seq DESC").Limit(80).Find(&logs)
	// Return in chronological order.
	for left, right := 0, len(logs)-1; left < right; left, right = left+1, right-1 {
		logs[left], logs[right] = logs[right], logs[left]
	}
	c.JSON(http.StatusOK, gin.H{"job": job, "logs": logs})
}

func jobIDs(jobs []models.DeployJob) []uint {
	ids := make([]uint, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	return ids
}

func (a *API) waitForJobs(ids []uint, timeout time.Duration) []models.DeployJob {
	deadline := time.Now().Add(timeout)
	for {
		var jobs []models.DeployJob
		a.DB.Preload("Target").Where("id IN ?", ids).Order("id ASC").Find(&jobs)
		if aggregateStatus(jobs) != "running" {
			return jobs
		}
		if time.Now().After(deadline) {
			return jobs
		}
		time.Sleep(time.Second)
	}
}

func aggregateStatus(jobs []models.DeployJob) string {
	if len(jobs) == 0 {
		return "no_targets"
	}
	success, failed, pending := 0, 0, 0
	for _, job := range jobs {
		switch job.Status {
		case "success":
			success++
		case "failed", "canceled":
			failed++
		default:
			pending++
		}
	}
	switch {
	case pending > 0:
		return "running"
	case failed > 0 && success > 0:
		return "partial"
	case failed > 0:
		return "failed"
	default:
		return "success"
	}
}
