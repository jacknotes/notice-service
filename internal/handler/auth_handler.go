package handler

import (
	"database/sql"
	"net/http"

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

// Login 登录
// @Summary 账号密码登录，返回 JWT
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
	token, user, err := h.Svc.Login(req.Username, req.Password)
	if err != nil {
		auditActor(h.db, 0, req.Username, "login.failed", "登录失败")
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	auditActor(h.db, user.ID, user.Username, "login.success", "登录成功")
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
	uid := c.GetInt64("uid")
	role := c.GetString("role")
	c.JSON(http.StatusOK, gin.H{"id": uid, "role": role})
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
