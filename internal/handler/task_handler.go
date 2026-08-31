package handler

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type TaskHandler struct {
	svc   *service.TaskService
	queue *service.QueueService
	db    *sql.DB
}

func NewTaskHandler(db *sql.DB, sched service.Scheduler, queue *service.QueueService) *TaskHandler {
	return &TaskHandler{svc: service.NewTaskService(db, sched), queue: queue, db: db}
}

// List 任务列表
// @Summary 任务列表
// @Tags 任务
// @Security BearerAuth
// @Success 200 {array} map[string]interface{}
// @Router /api/tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	// api_key 是触发凭据：任务为共享读，最低权限用户不应拿到它去调 webhook。
	if c.GetString("role") != "admin" {
		for _, t := range list {
			t.APIKey = ""
		}
	}
	c.JSON(http.StatusOK, list)
}

// Create 新建任务
// @Summary 新建任务（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Accept json
// @Param body body object true "任务信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks [post]
func (h *TaskHandler) Create(c *gin.Context) {
	var in model.Task
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Create(c.GetInt64("uid"), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.create", "创建任务 %q id=%d", in.Name, in.ID)
	c.JSON(http.StatusOK, in)
}

// Update 更新任务
// @Summary 更新任务（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Accept json
// @Param body body object true "任务信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/{id} [put]
func (h *TaskHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in model.Task
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Update(c.GetInt64("uid"), id, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	name, _ := h.svc.Name(id)
	auditf(c, h.db, "task.update", "更新任务 %s", auditRef(name, id))
	c.JSON(http.StatusOK, in)
}

// Delete 删除任务
// @Summary 删除任务（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/{id} [delete]
func (h *TaskHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	name, _ := h.svc.Name(id) // 删除前取名称
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.delete", "删除任务 %s", auditRef(name, id))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除任务（仅 admin）。
// BatchDelete 批量删除任务
// @Summary 批量删除任务（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param body body object true "ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/batch-delete [post]
func (h *TaskHandler) BatchDelete(c *gin.Context) {
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
	auditf(c, h.db, "task.batch_delete", "批量删除任务 %s", auditRefs(names, req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchToggle 批量启用/禁用任务（仅 admin）。
// BatchToggle 批量启用/禁用任务
// @Summary 批量启用/禁用任务
// @Tags 任务
// @Security BearerAuth
// @Param body body object true "ids + enabled"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/batch-toggle [post]
func (h *TaskHandler) BatchToggle(c *gin.Context) {
	var req struct {
		IDs     []int64 `json:"ids"`
		Enabled bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchToggle(req.IDs, req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.batch_toggle", "批量%s任务 %d 条", boolWord(req.Enabled), len(req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchSetCategory 批量变更任务分类（仅 admin）。
// BatchSetCategory 批量变更任务分类
// @Summary 批量变更任务分类
// @Tags 任务
// @Security BearerAuth
// @Param body body object true "ids + category"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/batch-category [post]
func (h *TaskHandler) BatchSetCategory(c *gin.Context) {
	var req struct {
		IDs      []int64 `json:"ids"`
		Category string  `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchSetCategory(req.IDs, req.Category); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.batch_update", "批量变更 %d 条任务分类为 %q", len(req.IDs), req.Category)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchSetChannels 批量变更任务投递渠道（仅 admin）。
// BatchSetChannels 批量变更任务投递渠道
// @Summary 批量变更任务投递渠道
// @Tags 任务
// @Security BearerAuth
// @Param body body object true "ids + channel_ids"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/batch-channels [post]
func (h *TaskHandler) BatchSetChannels(c *gin.Context) {
	var req struct {
		IDs        []int64 `json:"ids"`
		ChannelIDs []int64 `json:"channel_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchSetChannels(req.IDs, req.ChannelIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.batch_update", "批量变更 %d 条任务投递渠道为 %d 个渠道", len(req.IDs), len(req.ChannelIDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchSetReceivers 批量变更任务接收地址（仅 admin）。
// BatchSetReceivers 批量变更任务接收地址
// @Summary 批量变更任务接收地址
// @Tags 任务
// @Security BearerAuth
// @Param body body object true "ids + receivers"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/batch-receivers [post]
func (h *TaskHandler) BatchSetReceivers(c *gin.Context) {
	var req struct {
		IDs       []int64  `json:"ids"`
		Receivers []string `json:"receivers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.BatchSetReceivers(req.IDs, req.Receivers); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "task.batch_update", "批量变更 %d 条任务接收地址", len(req.IDs))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Toggle 启停任务
// @Summary 启用/停用任务（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Param body body object true "enabled"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/{id}/toggle [post]
func (h *TaskHandler) Toggle(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.svc.Toggle(c.GetInt64("uid"), id, req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	name, _ := h.svc.Name(id)
	auditf(c, h.db, "task.toggle", "任务 %s enabled=%v", auditRef(name, id), req.Enabled)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Logs 任务日志
// @Summary 查看某任务发送日志
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Success 200 {array} map[string]interface{}
// @Router /api/tasks/{id}/logs [get]
func (h *TaskHandler) Logs(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := h.svc.Get(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	logs, err := h.svc.Logs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// SendNow 立即发送任务（入队，不依赖 cron 到点；仅 admin）。
// SendNow 立即发送
// @Summary 立即发送任务（入队，不依赖 cron；仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "任务 ID"
// @Success 202 {object} map[string]interface{}
// @Router /api/tasks/{id}/send [post]
func (h *TaskHandler) SendNow(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := h.svc.Get(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	jobID, err := h.queue.Enqueue(id, nil, "",
		service.Trigger{Type: "manual", By: c.GetString("username"), IP: c.ClientIP()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	name, _ := h.svc.Name(id)
	auditf(c, h.db, "task.send_now", "立即发送 %s job=%d", auditRef(name, id), jobID)
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

// RetryLog 重试一条失败日志（定向重发该条；仅 admin）。
// @Summary 重试失败日志（定向重发该条）
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 202 {object} map[string]interface{}
// @Router /api/logs/{id}/retry [post]
func (h *TaskHandler) RetryLog(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	jobID, err := h.queue.EnqueueLogRetry(id,
		service.Trigger{Type: "retry", By: c.GetString("username"), IP: c.ClientIP()})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	// 日志本身无名，用其所属任务名作为可读标识
	taskName := h.svc.TaskNameByLogID(id)
	auditf(c, h.db, "log.retry", "重试 %s job=%d", auditRef(taskName, id), jobID)
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

// Preview 任务发送预览：用模板 + 当前任务变量渲染最终效果（不落库、不发送）。
// @Summary 任务发送预览（渲染标题/正文/接收地址）
// @Tags 任务
// @Security BearerAuth
// @Accept json
// @Param body body object true "预览参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks/preview [post]
func (h *TaskHandler) Preview(c *gin.Context) {
	var req struct {
		TemplateID int64             `json:"template_id"`
		Variables  map[string]string `json:"variables"`
		Receivers  []string          `json:"receivers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.TemplateID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	out, err := h.svc.TaskPreview(req.TemplateID, req.Variables, req.Receivers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, out)
}

// logFilterFromQuery 从查询参数解析日志过滤条件（task_id/category/status/from/to/
// page/page_size/sort_by/sort_order），返回带默认 Page/PageSize 的 filter。
// LogsAll 与 ExportLogs 共用，保证导出与列表筛选一致。
func (h *TaskHandler) logFilterFromQuery(c *gin.Context) repository.LogFilter {
	f := repository.LogFilter{Page: 1, PageSize: 50}
	if v := c.Query("task_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.TaskID = n
		}
	}
	if v := c.Query("category"); v != "" {
		f.Category = strings.TrimSpace(v)
	}
	if v := c.Query("status"); v == "success" || v == "failed" {
		f.Status = v
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
	// 后端排序（白名单在仓储层校验，非法值回退 id desc）
	if v := c.Query("sort_by"); v != "" {
		f.SortBy = v
	}
	if v := c.Query("sort_order"); v == "asc" || v == "desc" {
		f.SortOrder = v
	}
	return f
}

// csvSafe 阻断 Excel 公式注入：单元格以 = + - @ 制表符/回车开头时加单引号前缀，
// 避免 Excel/WPS 打开导出文件时把内容当公式执行（DDE/宏下载类攻击面）。
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		// 负号前缀（如 "-1"）同样可被解析为公式起始，统一转义。
		return "'" + s
	}
	return s
}

// ExportLogs 导出发送日志为 CSV（仅管理员）。
// @Summary 导出发送日志 CSV（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param task_id query int false "任务 ID"
// @Param status query string false "状态"
// @Param category query string false "分类（任务的当前分类）"
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Success 200 {string} string "CSV"
// @Router /api/logs/export [get]
func (h *TaskHandler) ExportLogs(c *gin.Context) {
	f := h.logFilterFromQuery(c)
	rows, err := h.svc.ExportLogRows(f, 100000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	filename := "logs-" + time.Now().Format("20060102-150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM：Windows Excel 无 BOM 时会把中文按 ANSI/GBK 误读，导致中文内容乱码。
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "sent_at", "task_id", "task_name", "category", "channel_id", "channel_name", "status", "subject", "content", "request", "response", "error_msg", "retry_count", "trigger_type", "trigger_by", "trigger_ip"})
	for _, r := range rows {
		_ = w.Write([]string{
			strconv.FormatInt(r.ID, 10), r.SentAt.Format("2006-01-02 15:04:05"),
			strconv.FormatInt(r.TaskID, 10), csvSafe(r.TaskName), csvSafe(r.Category), strconv.FormatInt(r.ChannelID, 10), csvSafe(r.ChannelName),
			csvSafe(r.Status), csvSafe(r.Subject), csvSafe(r.Content), csvSafe(r.Request), csvSafe(r.Response), csvSafe(r.ErrorMsg),
			strconv.Itoa(r.RetryCount), csvSafe(r.TriggerType), csvSafe(r.TriggerBy), csvSafe(r.TriggerIP),
		})
	}
	w.Flush()
}

// LogsAll 分页/筛选查询全部发送日志（筛选条件后端下推 DB）。
// LogsAll 发送日志（分页/筛选）
// @Summary 分页查询全部发送日志
// @Tags 任务
// @Security BearerAuth
// @Param task_id query int false "任务 ID"
// @Param status query string false "状态 success/failed"
// @Param category query string false "分类（任务的当前分类）"
// @Param from query string false "开始日期 YYYY-MM-DD"
// @Param to query string false "结束日期 YYYY-MM-DD"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/logs [get]
func (h *TaskHandler) LogsAll(c *gin.Context) {
	f := h.logFilterFromQuery(c)
	total, logs, err := h.svc.QueryLogs(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": logs})
}

// LogByID 发送日志详情（完整内容）。
// @Summary 发送日志详情
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} model.TaskLog
// @Router /api/logs/{id} [get]
func (h *TaskHandler) LogByID(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	log, err := h.svc.GetLog(id)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	c.JSON(http.StatusOK, log)
}
