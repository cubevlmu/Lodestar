package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lodestar/internal/models"

	"github.com/gin-gonic/gin"
)

// deployPlan is one host plus the app record that says where the project lives
// there. The resolved paths are snapshotted into the job.
type deployPlan struct {
	Target models.Target
	App    *models.TargetApp
}

// enqueueJobs creates pending deploy jobs. A target that already has an active
// job for the same project is skipped so a slow rollout cannot stack duplicates.
func (a *API) enqueueJobs(project models.Project, release models.Release, plans []deployPlan, trigger, requestedBy, message string) ([]models.DeployJob, error) {
	jobs := make([]models.DeployJob, 0, len(plans))
	for _, plan := range plans {
		target := plan.Target
		var active int64
		a.DB.Model(&models.DeployJob{}).
			Where("target_id = ? AND project_id = ? AND status IN ?", target.ID, project.ID, []string{"pending", "running"}).
			Count(&active)
		if active > 0 {
			log.Printf("[deploy] skip target %s: an active job for %s is already queued", target.Name, project.AppID)
			continue
		}
		deployDir := resolveAppDir(target, plan.App, project)
		backupDir := resolveBackupDir(target, plan.App)
		// Remember the path the first time it is derived, so later deploys and
		// the UI show the real location instead of an empty field.
		if plan.App != nil && plan.App.ID != 0 {
			updates := map[string]any{}
			if strings.TrimSpace(plan.App.DeployDir) == "" && deployDir != "" {
				updates["deploy_dir"] = deployDir
			}
			if strings.TrimSpace(plan.App.BackupDir) == "" && backupDir != "" {
				updates["backup_dir"] = backupDir
			}
			if len(updates) > 0 {
				a.DB.Model(&models.TargetApp{}).Where("id = ?", plan.App.ID).Updates(updates)
			}
		}
		job := models.DeployJob{
			ProjectID:   project.ID,
			ReleaseID:   release.ID,
			TargetID:    target.ID,
			Version:     release.Version,
			VersionCode: release.VersionCode,
			Mode:        "deploy",
			Status:      "pending",
			Trigger:     trigger,
			RequestedBy: requestedBy,
			Message:     message,
			DeployDir:   deployDir,
			BackupDir:   backupDir,
		}
		if err := a.DB.Create(&job).Error; err != nil {
			return jobs, err
		}
		// Wake the agent instead of waiting for its next poll tick.
		a.jobs.notify(target.ID)
		job.Target = target
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (a *API) ListJobs(c *gin.Context) {
	page, size := pageParams(c)
	query := a.DB.Model(&models.DeployJob{}).Preload("Target")

	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		query = query.Where("project_id = ?", raw)
	}
	if raw := strings.TrimSpace(c.Query("target_id")); raw != "" {
		query = query.Where("target_id = ?", raw)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if value := strings.TrimSpace(c.Query("q")); value != "" {
		like := "%" + value + "%"
		query = query.Where("version LIKE ? OR requested_by LIKE ? OR message LIKE ?", like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	var jobs []models.DeployJob
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&jobs).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": jobs, "total": total, "page": page, "size": size})
}

func (a *API) GetJob(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.Preload("Target").First(&job, id).Error; err != nil {
		notFound(c, "部署任务不存在")
		return
	}
	var logs []models.DeployLog
	a.DB.Where("job_id = ?", job.ID).Order("seq ASC").Limit(2000).Find(&logs)
	c.JSON(http.StatusOK, gin.H{"job": job, "logs": logs})
}

func (a *API) JobLogs(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	after, _ := strconv.Atoi(c.DefaultQuery("after_seq", "0"))
	var logs []models.DeployLog
	if err := a.DB.Where("job_id = ? AND seq > ?", id, after).Order("seq ASC").Limit(2000).Find(&logs).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs})
}

func (a *API) CancelJob(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.First(&job, id).Error; err != nil {
		notFound(c, "部署任务不存在")
		return
	}
	if job.Status != "pending" {
		badRequest(c, "只有排队中的任务可以取消")
		return
	}
	now := time.Now()
	if err := a.DB.Model(&job).Updates(map[string]any{"status": "canceled", "finished_at": now}).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.audit(c, "job.cancel", fmt.Sprintf("%d", job.ID))
	c.JSON(http.StatusOK, gin.H{"status": "canceled"})
}

func (a *API) RetryJob(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.First(&job, id).Error; err != nil {
		notFound(c, "部署任务不存在")
		return
	}
	var target models.Target
	if err := a.DB.First(&target, job.TargetID).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	username := "system"
	if user := CurrentUser(c); user != nil {
		username = user.Username
	}
	retry := models.DeployJob{
		ProjectID:   job.ProjectID,
		ReleaseID:   job.ReleaseID,
		TargetID:    job.TargetID,
		Version:     job.Version,
		VersionCode: job.VersionCode,
		Mode:        job.Mode,
		Status:      "pending",
		Trigger:     "retry",
		RequestedBy: username,
		Message:     fmt.Sprintf("retry of job #%d", job.ID),
		DeployDir:   job.DeployDir,
		BackupDir:   job.BackupDir,
	}
	if err := a.DB.Create(&retry).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.jobs.notify(retry.TargetID)
	retry.Target = target
	a.audit(c, "job.retry", fmt.Sprintf("%d -> %d", job.ID, retry.ID))
	c.JSON(http.StatusOK, gin.H{"job": retry})
}

// RollbackJob asks the target host to restore its newest local backup.
func (a *API) RollbackJob(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var job models.DeployJob
	if err := a.DB.First(&job, id).Error; err != nil {
		notFound(c, "部署任务不存在")
		return
	}
	var target models.Target
	if err := a.DB.First(&target, job.TargetID).Error; err != nil {
		notFound(c, "目标主机不存在")
		return
	}
	var app models.TargetApp
	if err := a.DB.Where("target_id = ? AND project_id = ?", target.ID, job.ProjectID).First(&app).Error; err != nil {
		notFound(c, "该主机上没有这个项目的部署记录，无法回滚")
		return
	}
	if app.CurrentVersion == "" {
		badRequest(c, "该主机尚未部署过这个项目的任何版本，无法回滚")
		return
	}
	var active int64
	a.DB.Model(&models.DeployJob{}).Where("target_id = ? AND status IN ?", target.ID, []string{"pending", "running"}).Count(&active)
	if active > 0 {
		badRequest(c, "该主机还有进行中的任务，请稍后再回滚")
		return
	}
	username := "system"
	if user := CurrentUser(c); user != nil {
		username = user.Username
	}
	rollback := models.DeployJob{
		ProjectID:   job.ProjectID,
		ReleaseID:   job.ReleaseID,
		TargetID:    job.TargetID,
		Version:     app.CurrentVersion,
		VersionCode: app.CurrentCode,
		Mode:        "rollback",
		Status:      "pending",
		Trigger:     "rollback",
		RequestedBy: username,
		Message:     fmt.Sprintf("rollback of job #%d", job.ID),
		DeployDir:   job.DeployDir,
		BackupDir:   job.BackupDir,
	}
	if err := a.DB.Create(&rollback).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	a.jobs.notify(rollback.TargetID)
	rollback.Target = target
	a.audit(c, "job.rollback", fmt.Sprintf("job %d target %s", job.ID, target.Name))
	c.JSON(http.StatusOK, gin.H{"job": rollback})
}
