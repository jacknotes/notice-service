package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	name, _ := h.svc.Name(id)
	auditf(c, h.db, "template.update", "更新模板 %s", auditRef(name, id))
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
	name, _ := h.svc.Name(id) // 删除前取名称
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "template.delete", "删除模板 %s", auditRef(name, id))
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
	names := make([]string, len(req.IDs))
	for i, tid := range req.IDs {
		names[i], _ = h.svc.Name(tid)
	}
	if err := h.svc.BatchDelete(req.IDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "template.batch_delete", "批量删除模板 %s", auditRefs(names, req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Preview 模板预览：用当前表单值（而非已保存值）渲染，支持未保存的新模板。
// @Summary 用变量渲染模板预览（使用当前表单值）
// @Tags 模板
// @Security BearerAuth
// @Param id path int true "模板 ID（0 表示未保存的新模板）"
// @Accept json
// @Param body body object true "预览参数（subject/content_md 缺省回退已保存值）"
// @Success 200 {object} map[string]interface{}
// @Router /api/templates/{id}/preview [post]
func (h *TemplateHandler) Preview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Subject   string            `json:"subject"`
		ContentMD string            `json:"content_md"`
		Variables map[string]string `json:"variables"`
	}
	_ = c.ShouldBindJSON(&req)

	// 优先用请求里的当前值；已保存的模板只在请求值缺省时兜底。
	tpl := &model.Template{Subject: req.Subject, ContentMD: req.ContentMD}
	if id > 0 {
		saved, err := h.svc.Get(c.GetInt64("uid"), id)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
			return
		}
		if tpl.Subject == "" {
			tpl.Subject = saved.Subject
		}
		if tpl.ContentMD == "" {
			tpl.ContentMD = saved.ContentMD
		}
		tpl.Variables = saved.Variables // 模板默认值仍参与合并
	}
	if strings.TrimSpace(tpl.Subject) == "" && strings.TrimSpace(tpl.ContentMD) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入标题或内容后再预览"})
		return
	}
	subject, content, err := h.svc.Preview(tpl, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subject": subject, "content": content})
}
