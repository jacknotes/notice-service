package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/service"
)

type ChannelHandler struct {
	svc *service.ChannelService
	db  *sql.DB
}

func NewChannelHandler(db *sql.DB, cipher *crypto.Cipher) *ChannelHandler {
	return &ChannelHandler{svc: service.NewChannelService(db, cipher), db: db}
}

// List 渠道列表
// @Summary 渠道列表
// @Tags 渠道
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/channels [get]
func (h *ChannelHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Create 新建渠道
// @Summary 新建渠道
// @Tags 渠道
// @Security BearerAuth
// @Accept json
// @Param body body object true "渠道配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels [post]
func (h *ChannelHandler) Create(c *gin.Context) {
	var in model.Channel
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "channel.create", "创建渠道 %q (type=%s)", in.Name, in.Type)
	c.JSON(http.StatusOK, in)
}

// Update 更新渠道
// @Summary 更新渠道
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Accept json
// @Param body body object true "渠道配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id} [put]
func (h *ChannelHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Channel
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, _ := h.svc.Name(id)
	auditf(c, h.db, "channel.update", "更新渠道 %s", auditRef(name, id))
	c.JSON(http.StatusOK, in)
}

// Delete 删除渠道
// @Summary 删除渠道
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id} [delete]
func (h *ChannelHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	name, _ := h.svc.Name(id) // 删除前取名称
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "channel.delete", "删除渠道 %s", auditRef(name, id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除渠道（仅 admin）。
// BatchDelete 批量删除渠道
// @Summary 批量删除渠道
// @Tags 渠道
// @Security BearerAuth
// @Param body body object true "ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/batch-delete [post]
func (h *ChannelHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	names := make([]string, len(req.IDs))
	for i, cid := range req.IDs {
		names[i], _ = h.svc.Name(cid)
	}
	if err := h.svc.BatchDelete(req.IDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "channel.batch_delete", "批量删除渠道 %s", auditRefs(names, req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test 测试渠道
// @Summary 测试渠道连通性
// @Tags 渠道
// @Security BearerAuth
// @Param id path int true "渠道 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/channels/{id}/test [post]
func (h *ChannelHandler) Test(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Type   string            `json:"type"`
		Config map[string]string `json:"config"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.Test(c.GetInt64("uid"), id, withType(req.Config, req.Type)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// withType 把请求里的 type 塞进 config，供 ChannelService.Test 新建前测试使用。
func withType(cfg map[string]string, t string) map[string]string {
	if cfg == nil {
		cfg = map[string]string{}
	}
	if t != "" {
		cfg["type"] = t
	}
	return cfg
}
