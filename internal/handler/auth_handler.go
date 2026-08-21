package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type AuthHandler struct {
	Svc *service.AuthService
	db  *sql.DB
}

func NewAuthHandler(db *sql.DB, authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{Svc: authSvc, db: db}
}

// Login 登录（两步：密码 → 双因子认证）。
// @Summary 账号密码登录；已启用 2FA 时返回待验证令牌
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body object true "登录信息" SchemaExample({"username":"admin","password":"admin123"})
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
		return
	}
	res, err := h.Svc.Login(req.Username, req.Password)
	if err != nil {
		auditActor(h.db, 0, req.Username, c.ClientIP(), "login.failed", "登录失败")
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if res.Requires2FA {
		auditActor(h.db, res.User.ID, res.User.Username, c.ClientIP(), "login.step1", "密码验证通过，等待双因子验证")
		c.JSON(http.StatusOK, gin.H{"requires_2fa": true, "pending_token": res.PendingToken, "user": res.User})
		return
	}
	auditActor(h.db, res.User.ID, res.User.Username, c.ClientIP(), "login.success", "登录成功")
	c.JSON(http.StatusOK, gin.H{"token": res.Token, "user": res.User})
}

// Setup2FA 生成 TOTP 密钥与备用码（仅展示一次，验证后启用）。
// @Summary 开启双因子认证：生成密钥与备用码
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/2fa/setup [post]
func (h *AuthHandler) Setup2FA(c *gin.Context) {
	secret, uri, codes, err := h.Svc.Setup2FA(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "auth.2fa_setup", "重新生成双因子认证密钥与备用码")
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": uri, "recovery_codes": codes})
}

// Enable2FA 用动态码验证后启用双因子认证。
// @Summary 启用双因子认证
// @Tags 认证
// @Security BearerAuth
// @Accept json
// @Param body body object true "code"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/2fa/enable [post]
func (h *AuthHandler) Enable2FA(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 6 位动态验证码"})
		return
	}
	if err := h.Svc.Enable2FA(c.GetInt64("uid"), req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "auth.2fa_enable", "启用双因子认证")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Disable2FA 校验当前动态码/备用码后关闭双因子认证。
// @Summary 关闭双因子认证
// @Tags 认证
// @Security BearerAuth
// @Accept json
// @Param body body object true "code"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/2fa/disable [post]
func (h *AuthHandler) Disable2FA(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 6 位动态验证码或备用码"})
		return
	}
	if err := h.Svc.Disable2FA(c.GetInt64("uid"), req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "auth.2fa_disable", "关闭双因子认证")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Verify2FA 登录第二步：校验动态码/备用码，返回完整 JWT。
// @Summary 双因子验证（登录第二步）
// @Tags 认证
// @Accept json
// @Param body body object true "待验证令牌与验证码"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/2fa/verify [post]
func (h *AuthHandler) Verify2FA(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	token, user, err := h.Svc.Verify2FA(req.Token, req.Code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	auditActor(h.db, user.ID, user.Username, c.ClientIP(), "login.success", "双因子验证通过，登录成功")
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Logout 登出
// @Summary 退出登录
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	auditCtx(c, h.db, "logout", "登出")
	// v1：前端丢弃令牌即可实现登出
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Me 当前用户
// @Summary 获取当前登录用户信息
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	u, err := h.Svc.User(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号已被禁用"})
		return
	}
	c.JSON(http.StatusOK, u)
}

// ChangePassword 修改密码
// @Summary 修改当前用户密码
// @Tags 认证
// @Security BearerAuth
// @Accept json
// @Param body body object true "原/新密码"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.Svc.ChangePassword(c.GetInt64("uid"), req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ForgotPassword 忘记密码：用管理员生成的一次性令牌自助重置密码（公开接口）。
// @Summary 用管理员生成的一次性令牌重置密码
// @Tags 认证
// @Accept json
// @Param body body object true "重置信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Username    string `json:"username"`
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Token == "" || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名、重置令牌和新密码"})
		return
	}
	if err := h.Svc.ResetPassword(req.Username, req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
