package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/service"
)

type UserHandler struct {
	svc *service.UserService
	db  *sql.DB
}

func NewUserHandler(db *sql.DB) *UserHandler {
	return &UserHandler{svc: service.NewUserService(db), db: db}
}

// operatorFromCtx 从请求上下文构造操作者，用于权限判定（内置 admin / 普通管理员）。
func operatorFromCtx(c *gin.Context) *model.User {
	return &model.User{ID: c.GetInt64("uid"), Username: c.GetString("username"), Role: c.GetString("role")}
}

// List 列出所有用户（仅 admin）。
// List 用户列表
// @Summary 用户列表（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/users [get]
func (h *UserHandler) List(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Create 创建用户（仅 admin）。
// Create 新建用户
// @Summary 新建用户（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Accept json
// @Param body body object true "用户信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/users [post]
func (h *UserHandler) Create(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	u, err := h.svc.Create(req.Username, req.DisplayName, req.Email, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.create", "创建用户 %s (显示名=%s 邮箱=%s role=%s)", u.Username, u.DisplayName, u.Email, u.Role)
	c.JSON(http.StatusOK, u)
}

// Update 修改用户角色 / 重置密码（仅 admin）。
// Update 更新用户
// @Summary 更新用户角色/密码（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Accept json
// @Param body body object true "更新字段"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id} [put]
func (h *UserHandler) Update(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Role        *string `json:"role"`
		Password    *string `json:"password"`
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), c.GetString("role"), id, req.Role, req.Password, req.DisplayName, req.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.update", "更新用户 %s (role=%s display_name=%s email=%s)", auditRef(h.svc.Username(id), id), auditStr(req.Role), auditStr(req.DisplayName), auditStr(req.Email))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete 删除用户（仅 admin）。
// Delete 删除用户
// @Summary 删除用户（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	name := h.svc.Username(id) // 删除前取用户名，软删除后查不到
	if err := h.svc.Delete(operatorFromCtx(c), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.delete", "删除用户 %s", auditRef(name, id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除用户（仅 admin）。
// BatchDelete 批量删除用户
// @Summary 批量删除用户（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param body body object true "ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/batch-delete [post]
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
	names := make([]string, len(req.IDs))
	for i, uid := range req.IDs {
		names[i] = h.svc.Username(uid) // 批量删除前取用户名
	}
	if err := h.svc.BatchDelete(operatorFromCtx(c), req.IDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.batch_delete", "批量删除用户 %s", auditRefs(names, req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Disable 禁用用户（仅 admin）：登录与已签发令牌立即失效，数据保留可重新启用。
// Disable 禁用用户
// @Summary 禁用用户（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id}/disable [post]
func (h *UserHandler) Disable(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DisableUser(operatorFromCtx(c), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.disable", "禁用用户 %s", auditRef(h.svc.Username(id), id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Enable 重新启用用户（仅 admin）。
// Enable 启用用户
// @Summary 启用用户（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id}/enable [post]
func (h *UserHandler) Enable(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.EnableUser(operatorFromCtx(c), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.enable", "启用用户 %s", auditRef(h.svc.Username(id), id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ResetToken 生成一次性重置令牌（仅 admin），返回给管理员线下转交用户。
// ResetToken 生成重置令牌
// @Summary 为用户生成一次性重置令牌（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id}/reset-token [post]
func (h *UserHandler) ResetToken(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	token, expires, err := h.svc.GenerateResetToken(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.reset_token", "为用户 %s 生成重置令牌（%s 过期）", auditRef(h.svc.Username(id), id), expires.Format("2006-01-02 15:04:05"))
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": expires.Format("2006-01-02 15:04:05")})
}

// ForceEnable2FA 管理员为用户强制开启双因子认证，返回密钥与备用码由管理员线下转交。
// ForceEnable2FA 强制开启双因子认证
// @Summary 管理员为用户强制开启双因子认证（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id}/2fa-enable [post]
func (h *UserHandler) ForceEnable2FA(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	secret, uri, codes, err := h.svc.ForceEnable2FA(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.2fa_force_enable", "强制开启 %s 的双因子认证", auditRef(h.svc.Username(id), id))
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": uri, "recovery_codes": codes})
}

// ForceDisable2FA 管理员为用户强制关闭双因子认证（用户丢失手机/备用码时的恢复路径）。
// ForceDisable2FA 强制关闭双因子认证
// @Summary 管理员为用户强制关闭双因子认证（仅管理员）
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/users/{id}/2fa-disable [post]
func (h *UserHandler) ForceDisable2FA(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.ForceDisable2FA(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "user.2fa_force_disable", "强制关闭 %s 的双因子认证", auditRef(h.svc.Username(id), id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
