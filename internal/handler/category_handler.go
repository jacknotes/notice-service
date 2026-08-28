package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type CategoryHandler struct {
	svc *service.CategoryService
	db  *sql.DB
}

func NewCategoryHandler(db *sql.DB) *CategoryHandler {
	return &CategoryHandler{svc: service.NewCategoryService(db), db: db}
}

// List 分类列表（渠道/模板/任务统一引用的共享分类池）
// @Summary 共享分类列表
// @Tags 分类
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/categories [get]
func (h *CategoryHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Unused 未被引用的分类（供「分类管理」页提示可删除）
// @Summary 未被引用的分类
// @Tags 分类
// @Security BearerAuth
// @Success 200 {array} string
// @Router /api/categories/unused [get]
func (h *CategoryHandler) Unused(c *gin.Context) {
	names, err := h.svc.UnusedNames()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, names)
}

// Create 新增分类（仅 admin）
// @Summary 新增分类
// @Tags 分类
// @Security BearerAuth
// @Accept json
// @Param body body object true "分类信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/categories [post]
func (h *CategoryHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cat, err := h.svc.Create(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "category.create", "新增分类 %q", cat.Name)
	c.JSON(http.StatusOK, cat)
}

// Update 重命名分类（仅 admin）。改名后渠道/模板/任务中的引用同步更新。
// @Summary 重命名分类
// @Tags 分类
// @Security BearerAuth
// @Accept json
// @Param name path string true "分类名称"
// @Param body body object true "新名称"
// @Success 200 {object} map[string]interface{}
// @Router /api/categories/{name} [put]
func (h *CategoryHandler) Update(c *gin.Context) {
	old := c.Param("name")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cat, err := h.svc.Update(old, req.Name)
	if err != nil {
		if err == repository.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "分类不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "category.update", "重命名分类 %q → %q", old, cat.Name)
	c.JSON(http.StatusOK, cat)
}

// Delete 删除分类（仅 admin）
// @Summary 删除分类
// @Tags 分类
// @Security BearerAuth
// @Param name path string true "分类名称"
// @Success 200 {object} map[string]interface{}
// @Router /api/categories/{name} [delete]
func (h *CategoryHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(name); err != nil {
		if err == repository.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "分类不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "category.delete", "删除分类 %q", name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}