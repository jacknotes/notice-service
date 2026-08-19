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

func (h *ChannelHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

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
	auditf(c, h.db, "channel.update", "更新渠道 id=%d", id)
	c.JSON(http.StatusOK, in)
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "channel.delete", "删除渠道 id=%d", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除渠道（仅 admin）。
func (h *ChannelHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchDelete(req.IDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "channel.batch_delete", "批量删除渠道 ids=%v", req.IDs)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

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
