package api

import (
	"net/http"
	"time"

	"lodestar/internal/buildinfo"
	"lodestar/internal/models"

	"github.com/gin-gonic/gin"
)

type dailyCount struct {
	Day     string `json:"day"`
	Total   int64  `json:"total"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

func (a *API) Overview(c *gin.Context) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekAgo := now.AddDate(0, 0, -7)
	fortnightAgo := now.AddDate(0, 0, -14)

	stats := gin.H{}
	var count int64

	a.DB.Model(&models.Project{}).Count(&count)
	stats["projects"] = count
	a.DB.Model(&models.Release{}).Count(&count)
	stats["releases"] = count
	a.DB.Model(&models.Release{}).Where("created_at >= ?", weekAgo).Count(&count)
	stats["releases_week"] = count
	a.DB.Model(&models.Target{}).Count(&count)
	stats["targets"] = count
	a.DB.Model(&models.Target{}).Where("last_seen_at >= ?", now.Add(-90*time.Second)).Count(&count)
	stats["targets_online"] = count
	a.DB.Model(&models.DeployJob{}).Where("created_at >= ?", todayStart).Count(&count)
	stats["jobs_today"] = count
	a.DB.Model(&models.DeployJob{}).Where("created_at >= ?", weekAgo).Count(&count)
	stats["jobs_week"] = count

	var successWeek, failedWeek int64
	a.DB.Model(&models.DeployJob{}).Where("created_at >= ? AND status = ?", weekAgo, "success").Count(&successWeek)
	a.DB.Model(&models.DeployJob{}).Where("created_at >= ? AND status IN ?", weekAgo, []string{"failed", "canceled"}).Count(&failedWeek)
	stats["jobs_success_week"] = successWeek
	stats["jobs_failed_week"] = failedWeek
	if total := successWeek + failedWeek; total > 0 {
		stats["success_rate"] = float64(successWeek) / float64(total) * 100
	} else {
		stats["success_rate"] = float64(0)
	}

	var avg struct {
		Value float64
	}
	a.DB.Model(&models.DeployJob{}).
		Select("COALESCE(AVG(duration_ms), 0) as value").
		Where("created_at >= ? AND status = ? AND duration_ms > 0", weekAgo, "success").
		Scan(&avg)
	stats["avg_duration_ms"] = int64(avg.Value)

	var storage struct {
		Value int64
	}
	a.DB.Model(&models.Package{}).Select("COALESCE(SUM(size), 0) as value").Scan(&storage)
	stats["storage_bytes"] = storage.Value

	var recent []models.DeployJob
	a.DB.Preload("Target").Order("id DESC").Limit(8).Find(&recent)
	stats["recent_jobs"] = recent

	var targets []models.Target
	a.DB.Order("id ASC").Find(&targets)
	targetItems := make([]targetListItem, 0, len(targets))
	for _, target := range targets {
		item := targetListItem{targetView: viewTarget(target)}
		if target.Status != "deploying" {
			if target.IsOnline(90) {
				item.Status = "online"
			} else {
				item.Status = "offline"
			}
		}
		targetItems = append(targetItems, item)
	}
	stats["targets_list"] = targetItems

	var jobs []models.DeployJob
	a.DB.Where("created_at >= ?", fortnightAgo).Find(&jobs)
	buckets := map[string]*dailyCount{}
	order := make([]string, 0, 14)
	for i := 13; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		buckets[day] = &dailyCount{Day: day}
		order = append(order, day)
	}
	for _, job := range jobs {
		day := job.CreatedAt.Local().Format("2006-01-02")
		bucket, ok := buckets[day]
		if !ok {
			continue
		}
		bucket.Total++
		switch job.Status {
		case "success":
			bucket.Success++
		case "failed", "canceled":
			bucket.Failed++
		}
	}
	daily := make([]dailyCount, 0, len(order))
	for _, day := range order {
		daily = append(daily, *buckets[day])
	}
	stats["daily"] = daily

	stats["server"] = gin.H{
		"version":    buildinfo.Version,
		"commit":     buildinfo.Commit,
		"uptime":     int64(now.Sub(a.Started).Seconds()),
		"started_at": a.Started,
	}
	c.JSON(http.StatusOK, stats)
}
