package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type UserHandler struct {
	svc *service.UserService
	db  *sql.DB
}

func NewUserHandler(db *sql.DB) *UserHandler {
	return &UserHandler{svc: service.NewUserService(db), db: db}
}

// List 列出所有用户（仅 admin）。
func (h *UserHandler) List(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Create 创建用户（仅 admin）。
func (h *UserHandler) Create(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	u, err := h.svc.Create(req.Username, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "user.create", "创建用户 %s (role=%s)", u.Username, u.Role)
	c.JSON(http.StatusOK, u)
}

// Update 修改用户角色 / 重置密码（仅 admin）。
func (h *UserHandler) Update(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), c.GetString("role"), id, req.Role, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "user.update", "更新用户 id=%d (role=%v)", id, req.Role)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete 删除用户（仅 admin）。
func (h *UserHandler) Delete(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetString("role"), c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "user.delete", "删除用户 id=%d", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除用户（仅 admin）。
func (h *UserHandler) BatchDelete(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchDelete(c.GetInt64("uid"), c.GetString("role"), req.IDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "user.batch_delete", "批量删除用户 ids=%v", req.IDs)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ResetToken 生成一次性重置令牌（仅 admin），返回给管理员线下转交用户。
func (h *UserHandler) ResetToken(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	token, expires, err := h.svc.GenerateResetToken(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "user.reset_token", "为用户 id=%d 生成重置令牌（%s 过期）", id, expires.Format("2006-01-02 15:04:05"))
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": expires.Format("2006-01-02 15:04:05")})
}
