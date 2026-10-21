package api

import (
	"net/http"
	"time"

	"lodestar/internal/config"
	"lodestar/internal/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// New builds the full HTTP surface: operator API, CI deploy API, agent protocol,
// app update API, and the embedded WebUI.
func NewRouter(database *gorm.DB, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), securityHeaders())

	a := New(database, cfg)

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
	})

	v1 := router.Group("/api/v1")

	// Operator session auth.
	v1.POST("/auth/login", a.Login)

	// CI deployment API (project deploy token).
	ci := v1.Group("/deploy")
	ci.Use(a.RequireDeployToken())
	ci.POST("", a.CIDeploy)
	ci.POST("/json", a.CIDeployExternal)
	ci.GET("/jobs/:id", a.CIDeployStatus)

	// Production host agents (target token).
	agent := v1.Group("/agent")
	agent.Use(a.RequireTargetToken())
	agent.POST("/heartbeat", a.AgentHeartbeat)
	agent.GET("/jobs/next", a.AgentNextJob)
	agent.POST("/jobs/:id/logs", a.AgentLogs)
	agent.POST("/jobs/:id/finish", a.AgentFinish)
	agent.GET("/packages/:id/download", a.AgentDownloadPackage)

	// App auto-update API (HMAC client signature).
	v1.POST("/check", a.RequireClientSignature(), a.CheckUpdate)
	v1.GET("/packages/:package_id/download", a.DownloadPackage)

	// Authenticated operator API.
	admin := v1.Group("")
	admin.Use(a.RequireUser())
	{
		admin.POST("/auth/logout", a.Logout)
		admin.GET("/auth/me", a.Me)
		admin.POST("/auth/password", a.ChangePassword)

		admin.GET("/overview", a.Overview)

		admin.GET("/projects", a.ListProjects)
		admin.POST("/projects", a.RequireAdmin(), a.CreateProject)
		admin.GET("/projects/:id", a.GetProject)
		admin.PATCH("/projects/:id", a.RequireAdmin(), a.UpdateProject)
		admin.DELETE("/projects/:id", a.RequireAdmin(), a.DeleteProject)
		admin.POST("/projects/:id/rotate", a.RequireAdmin(), a.RotateProjectKey)

		admin.GET("/releases", a.ListReleases)
		admin.POST("/releases", a.RequireAdmin(), a.CreateRelease)
		admin.POST("/releases/json", a.RequireAdmin(), a.CreateReleaseExternal)
		admin.PATCH("/releases/:id", a.RequireAdmin(), a.UpdateRelease)
		admin.DELETE("/releases/:id", a.RequireAdmin(), a.DeleteRelease)
		admin.POST("/releases/:id/deploy", a.RequireAdmin(), a.DeployRelease)

		admin.GET("/targets", a.ListTargets)
		admin.POST("/targets", a.RequireAdmin(), a.CreateTarget)
		admin.PATCH("/targets/:id", a.RequireAdmin(), a.UpdateTarget)
		admin.DELETE("/targets/:id", a.RequireAdmin(), a.DeleteTarget)
		admin.POST("/targets/:id/rotate-token", a.RequireAdmin(), a.RotateTargetToken)
		admin.POST("/targets/:id/deploy", a.RequireAdmin(), a.DeployToTarget)
		admin.GET("/deploy-configs", a.ListDeployConfigs)
		admin.PATCH("/targets/:id/apps/:project_id", a.RequireAdmin(), a.UpdateTargetApp)
		admin.DELETE("/targets/:id/apps/:project_id", a.RequireAdmin(), a.DeleteTargetApp)

		admin.GET("/jobs", a.ListJobs)
		admin.GET("/jobs/:id", a.GetJob)
		admin.GET("/jobs/:id/logs", a.JobLogs)
		admin.POST("/jobs/:id/cancel", a.RequireAdmin(), a.CancelJob)
		admin.POST("/jobs/:id/retry", a.RequireAdmin(), a.RetryJob)
		admin.POST("/jobs/:id/rollback", a.RequireAdmin(), a.RollbackJob)

		admin.GET("/policies", a.ListPolicies)
		admin.PUT("/policies", a.RequireAdmin(), a.UpsertPolicy)
		admin.DELETE("/policies/:id", a.RequireAdmin(), a.DeletePolicy)
		admin.GET("/blocked-versions", a.ListBlockedVersions)
		admin.POST("/blocked-versions", a.RequireAdmin(), a.CreateBlockedVersion)
		admin.PATCH("/blocked-versions/:id", a.RequireAdmin(), a.UpdateBlockedVersion)
		admin.DELETE("/blocked-versions/:id", a.RequireAdmin(), a.DeleteBlockedVersion)

		admin.GET("/settings", a.GetSettings)
		admin.PUT("/settings", a.RequireAdmin(), a.UpdateSettings)
		admin.GET("/audit-logs", a.ListAuditLogs)
	}

	// WebUI (SPA fallback included).
	router.NoRoute(gin.WrapH(web.Handler()))

	return router
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "same-origin")
		c.Next()
	}
}
