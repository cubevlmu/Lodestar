package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"lodestar/internal/config"
	"lodestar/internal/models"
	"lodestar/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ctxUser    = "lodestar.user"
	ctxSession = "lodestar.session"
	ctxProject = "lodestar.project"
	ctxTarget  = "lodestar.target"
)

// API holds the shared dependencies for every handler.
type API struct {
	DB      *gorm.DB
	Cfg     config.Config
	Started time.Time
	limiter *loginLimiter
}

func New(database *gorm.DB, cfg config.Config) *API {
	return &API{DB: database, Cfg: cfg, Started: time.Now(), limiter: newLoginLimiter()}
}

// ---------- response helpers ----------

func fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

func failErr(c *gin.Context, status int, err error) {
	fail(c, status, err.Error())
}

func badRequest(c *gin.Context, message string) {
	fail(c, http.StatusBadRequest, message)
}

func notFound(c *gin.Context, message string) {
	fail(c, http.StatusNotFound, message)
}

// ---------- auth middleware ----------

func bearerToken(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(header) > 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

// RequireUser authenticates an operator session token.
func (a *API) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			fail(c, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		var session models.Session
		if err := a.DB.Where("token_hash = ?", security.HashToken(token)).First(&session).Error; err != nil {
			fail(c, http.StatusUnauthorized, "会话无效，请重新登录")
			return
		}
		if time.Now().After(session.ExpiresAt) {
			a.DB.Delete(&session)
			fail(c, http.StatusUnauthorized, "会话已过期，请重新登录")
			return
		}
		var user models.User
		if err := a.DB.First(&user, session.UserID).Error; err != nil || !user.Enabled {
			fail(c, http.StatusUnauthorized, "账户不可用")
			return
		}
		c.Set(ctxUser, &user)
		c.Set(ctxSession, &session)
		c.Next()
	}
}

// RequireAdmin must be mounted after RequireUser.
func (a *API) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || user.Role != "admin" {
			fail(c, http.StatusForbidden, "需要管理员权限")
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *models.User {
	value, ok := c.Get(ctxUser)
	if !ok {
		return nil
	}
	user, _ := value.(*models.User)
	return user
}

// RequireDeployToken authenticates a CI job with a project deploy token.
func (a *API) RequireDeployToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("X-Deploy-Token"))
		if token == "" {
			token = bearerToken(c)
		}
		if token == "" {
			fail(c, http.StatusUnauthorized, "missing deploy token")
			return
		}
		var project models.Project
		if err := a.DB.Where("deploy_token = ? AND enabled = ?", token, true).First(&project).Error; err != nil {
			fail(c, http.StatusUnauthorized, "invalid deploy token")
			return
		}
		c.Set(ctxProject, &project)
		c.Next()
	}
}

// RequireTargetToken authenticates a production host agent.
func (a *API) RequireTargetToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("X-Target-Token"))
		if token == "" {
			token = bearerToken(c)
		}
		if token == "" {
			fail(c, http.StatusUnauthorized, "missing target token")
			return
		}
		var target models.Target
		if err := a.DB.Where("token = ?", token).First(&target).Error; err != nil {
			fail(c, http.StatusUnauthorized, "unknown target")
			return
		}
		// A host is not bound to a project: the job payload carries the project.
		c.Set(ctxTarget, &target)
		c.Next()
	}
}

func CurrentProject(c *gin.Context) *models.Project {
	value, ok := c.Get(ctxProject)
	if !ok {
		return nil
	}
	project, _ := value.(*models.Project)
	return project
}

func CurrentTarget(c *gin.Context) *models.Target {
	value, ok := c.Get(ctxTarget)
	if !ok {
		return nil
	}
	target, _ := value.(*models.Target)
	return target
}

// ---------- misc helpers ----------

func parseID(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		badRequest(c, "无效的 ID")
		return 0, false
	}
	return uint(value), true
}

func pageParams(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	return page, size
}

func (a *API) setting(key string) string {
	var setting models.Setting
	if err := a.DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return ""
	}
	return setting.Value
}

func (a *API) saveSetting(key, value string) error {
	return a.DB.Save(&models.Setting{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}

// publicBaseURL resolves the externally reachable origin used in download URLs.
func (a *API) publicBaseURL(c *gin.Context) string {
	if value := strings.TrimRight(a.setting("public_base_url"), "/"); value != "" {
		return value
	}
	if a.Cfg.PublicBaseURL != "" {
		return a.Cfg.PublicBaseURL
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

func (a *API) audit(c *gin.Context, action, detail string) {
	entry := models.AuditLog{Action: action, Detail: detail, IP: c.ClientIP()}
	if user := CurrentUser(c); user != nil {
		entry.UserID = user.ID
		entry.Username = user.Username
	}
	if err := a.DB.Create(&entry).Error; err != nil {
		log.Printf("[audit] %s %s: %v", action, detail, err)
	}
}

func (a *API) projectByID(id uint) (*models.Project, error) {
	var project models.Project
	if err := a.DB.First(&project, id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

// projectView exposes secrets to authenticated operators only.
type projectView struct {
	models.Project
	ClientSecret string `json:"client_secret"`
	DeployToken  string `json:"deploy_token"`
}

func viewProject(project models.Project) projectView {
	return projectView{Project: project, ClientSecret: project.ClientSecret, DeployToken: project.DeployToken}
}

type targetView struct {
	models.Target
	Token string `json:"token"`
}

func viewTarget(target models.Target) targetView {
	return targetView{Target: target, Token: target.Token}
}

func decodeJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		badRequest(c, "请求体格式错误: "+err.Error())
		return false
	}
	return true
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// ---------- login rate limiting ----------

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptWindow
}

type attemptWindow struct {
	count int
	reset time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*attemptWindow)}
}

const (
	loginMaxAttempts = 8
	loginWindow      = 5 * time.Minute
)

func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	window, ok := l.attempts[key]
	if !ok || time.Now().After(window.reset) {
		return false
	}
	return window.count >= loginMaxAttempts
}

func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	window, ok := l.attempts[key]
	if !ok || time.Now().After(window.reset) {
		l.attempts[key] = &attemptWindow{count: 1, reset: time.Now().Add(loginWindow)}
		return
	}
	window.count++
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
