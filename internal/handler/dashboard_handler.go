package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{svc: service.NewDashboardService(db)}
}

// parseDateRange 解析 from/to 查询参数（YYYY-MM-DD），缺省最近 7 天（含今天，to 排他）。
func parseDateRange(c *gin.Context) (from, to time.Time) {
	now := time.Now()
	to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	from = to.AddDate(0, 0, -7)
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t.Add(24 * time.Hour) // 结束日期排他
		}
	}
	if !to.After(from) {
		to = from.Add(24 * time.Hour)
	}
	return
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
	from, to := parseDateRange(c)
	s, err := h.svc.StatsRange(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
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
	from, to := parseDateRange(c)
	tr, err := h.svc.TrendRange(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, tr)
}

// TopTasks 仪表盘 TOP 任务
// @Summary 区间内发送量 TOP 任务
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Param limit query int false "返回条数（默认 5）"
// @Success 200 {array} map[string]interface{}
// @Router /api/dashboard/top-tasks [get]
func (h *DashboardHandler) TopTasks(c *gin.Context) {
	from, to := parseDateRange(c)
	limit := 5
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}
	top, err := h.svc.TopTasks(from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, top)
}

// ChannelStats 仪表盘渠道统计
// @Summary 区间内各渠道投递统计
// @Tags 仪表盘
// @Security BearerAuth
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Success 200 {array} map[string]interface{}
// @Router /api/dashboard/channel-stats [get]
func (h *DashboardHandler) ChannelStats(c *gin.Context) {
	from, to := parseDateRange(c)
	chs, err := h.svc.ChannelStats(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, chs)
}
