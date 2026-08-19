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

func (h *AuthHandler) Logout(c *gin.Context) {
	auditCtx(c, h.db, "logout", "登出")
	// v1：前端丢弃令牌即可实现登出
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid := c.GetInt64("uid")
	role := c.GetString("role")
	c.JSON(http.StatusOK, gin.H{"id": uid, "role": role})
}

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
