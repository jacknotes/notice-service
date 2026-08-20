package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{svc: service.NewDashboardService(db)}
}

// Stats 仪表盘统计
// @Summary 区间内投递统计（缺省近 7 天）
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期 YYYY-MM-DD"
// @Param to query string false "结束日期 YYYY-MM-DD"
// @Success 200 {object} map[string]interface{}
// @Router /api/dashboard/stats [get]
func (h *DashboardHandler) Stats(c *gin.Context) {
	s, err := h.svc.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Trend 仪表盘趋势
// @Summary 区间内每日发送趋势
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Success 200 {array} map[string]interface{}
// @Router /api/dashboard/trend [get]
func (h *DashboardHandler) Trend(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 30 {
			days = n
		}
	}
	tr, err := h.svc.Trend(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tr)
}
