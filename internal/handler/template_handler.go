package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/service"
)

type TemplateHandler struct {
	svc *service.TemplateService
	db  *sql.DB
}

func NewTemplateHandler(db *sql.DB) *TemplateHandler {
	return &TemplateHandler{svc: service.NewTemplateService(db), db: db}
}

// List 模板列表
// @Summary 模板列表
// @Tags 模板
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/templates [get]
func (h *TemplateHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Create 新建模板
// @Summary 新建模板
// @Tags 模板
// @Security BearerAuth
// @Accept json
// @Param body body object true "模板信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates [post]
func (h *TemplateHandler) Create(c *gin.Context) {
	var in model.Template
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "template.create", "创建模板 %q", in.Name)
	c.JSON(http.StatusOK, in)
}

// Update 更新模板
// @Summary 更新模板
// @Tags 模板
// @Security BearerAuth
// @Param id path int true "模板 ID"
// @Accept json
// @Param body body object true "模板信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates/{id} [put]
func (h *TemplateHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Template
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "template.update", "更新模板 id=%d", id)
	c.JSON(http.StatusOK, in)
}

// Delete 删除模板
// @Summary 删除模板
// @Tags 模板
// @Security BearerAuth
// @Param id path int true "模板 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates/{id} [delete]
func (h *TemplateHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "template.delete", "删除模板 id=%d", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除模板（仅 admin）。
// BatchDelete 批量删除模板
// @Summary 批量删除模板
// @Tags 模板
// @Security BearerAuth
// @Param body body object true "ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates/batch-delete [post]
func (h *TemplateHandler) BatchDelete(c *gin.Context) {
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
	auditf(c, h.db, "template.batch_delete", "批量删除模板 ids=%v", req.IDs)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Preview 模板预览
// @Summary 用变量渲染模板预览
// @Tags 模板
// @Security BearerAuth
// @Param id path int true "模板 ID"
// @Accept json
// @Param body body object true "变量"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates/{id}/preview [post]
func (h *TemplateHandler) Preview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tpl, err := h.svc.Get(c.GetInt64("uid"), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	_ = c.ShouldBindJSON(&req)
	subject, content, err := h.svc.Preview(tpl, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subject": subject, "content": content})
}
