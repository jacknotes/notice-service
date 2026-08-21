package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/repository"
)

type AuditHandler struct {
	repo *repository.AuditRepo
}

func NewAuditHandler(db *sql.DB) *AuditHandler {
	return &AuditHandler{repo: repository.NewAuditRepo(db)}
}

// List 审计日志列表（分页/筛选；仅 admin）。
// @Summary 操作审计日志（仅管理员）
// @Tags 审计
// @Security BearerAuth
// @Param keyword query string false "关键词（匹配用户名/详情）"
// @Param action query string false "操作类型（精确匹配）"
// @Param from query string false "开始日期 YYYY-MM-DD"
// @Param to query string false "结束日期 YYYY-MM-DD"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/audit [get]
func (h *AuditHandler) List(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作"})
		return
	}
	f := repository.AuditFilter{Page: 1, PageSize: 50}
	if v := c.Query("keyword"); v != "" {
		f.Keyword = v
	}
	if v := c.Query("action"); v != "" {
		f.Action = v
	}
	if v := c.Query("module"); v != "" {
		f.Module = v
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			f.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			f.To = t.Add(24 * time.Hour) // 结束日期排他
		}
	}
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.PageSize = n
		}
	}
	total, logs, err := h.repo.Query(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": logs})
}
