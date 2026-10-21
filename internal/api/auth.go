package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lodestar/internal/config"
	"lodestar/internal/models"
	"lodestar/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userView struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func toUserView(user models.User) userView {
	return userView{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		LastLoginAt: user.LastLoginAt,
	}
}

func (a *API) Login(c *gin.Context) {
	var req loginRequest
	if !decodeJSON(c, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		badRequest(c, "请输入用户名和密码")
		return
	}

	key := c.ClientIP() + "|" + strings.ToLower(req.Username)
	if a.limiter.blocked(key) {
		fail(c, http.StatusTooManyRequests, "尝试次数过多，请 5 分钟后再试")
		return
	}

	var user models.User
	err := a.DB.Where("username = ?", req.Username).First(&user).Error
	if err != nil || !user.Enabled || !security.CheckPassword(user.PasswordHash, req.Password) {
		a.limiter.recordFailure(key)
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	a.limiter.reset(key)

	token := security.NewToken(32)
	now := time.Now()
	session := models.Session{
		UserID:    user.ID,
		TokenHash: security.HashToken(token),
		ExpiresAt: now.Add(a.Cfg.SessionTTL),
		IP:        c.ClientIP(),
		UserAgent: truncate(c.Request.UserAgent(), 250),
	}
	if err := a.DB.Create(&session).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	user.LastLoginAt = &now
	a.DB.Model(&user).Update("last_login_at", now)

	// Opportunistic cleanup of expired sessions.
	a.DB.Where("expires_at < ?", now).Delete(&models.Session{})

	a.audit(c, "login", user.Username)
	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": session.ExpiresAt,
		"user":       toUserView(user),
	})
}

func (a *API) Logout(c *gin.Context) {
	value, ok := c.Get(ctxSession)
	if ok {
		if session, valid := value.(*models.Session); valid {
			a.DB.Delete(session)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "logged_out"})
}

func (a *API) Me(c *gin.Context) {
	user := CurrentUser(c)
	if user == nil {
		fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":       toUserView(*user),
		"server_now": time.Now(),
		"uptime":     int64(time.Since(a.Started).Seconds()),
	})
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (a *API) ChangePassword(c *gin.Context) {
	var req passwordRequest
	if !decodeJSON(c, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		badRequest(c, "新密码至少 8 位")
		return
	}
	user := CurrentUser(c)
	if user == nil {
		fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	if !security.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		badRequest(c, "当前密码不正确")
		return
	}
	hash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	if err := a.DB.Model(user).Update("password_hash", hash).Error; err != nil {
		failErr(c, http.StatusInternalServerError, err)
		return
	}
	// Invalidate every other session.
	if value, ok := c.Get(ctxSession); ok {
		if session, valid := value.(*models.Session); valid {
			a.DB.Where("user_id = ? AND id <> ?", user.ID, session.ID).Delete(&models.Session{})
		}
	}
	a.audit(c, "change_password", user.Username)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// EnsureBootstrapAdmin creates the first operator account. When no password is configured a
// random one is generated, printed once, and written to <data_dir>/initial-admin.txt.
func EnsureBootstrapAdmin(database *gorm.DB, cfg config.Config) (username, password string, created bool, err error) {
	var count int64
	if err := database.Model(&models.User{}).Count(&count).Error; err != nil {
		return "", "", false, err
	}
	if count > 0 {
		return "", "", false, nil
	}

	username = strings.TrimSpace(cfg.AdminUsername)
	if username == "" {
		username = "admin"
	}
	password = cfg.AdminPassword
	generated := false
	if len(password) < 8 {
		password = security.NewToken(12)
		generated = true
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return "", "", false, err
	}
	user := models.User{Username: username, PasswordHash: hash, DisplayName: "Administrator", Role: "admin", Enabled: true}
	if err := database.Create(&user).Error; err != nil {
		return "", "", false, err
	}

	if generated {
		path := filepath.Join(cfg.DataDir, "initial-admin.txt")
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
			content := fmt.Sprintf("Lodestar initial administrator\nusername: %s\npassword: %s\n\nChange this password after signing in, then delete this file.\n", username, password)
			_ = os.WriteFile(path, []byte(content), 0o600)
		}
	}
	return username, password, true, nil
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
