package handler

import (
	"database/sql"
	"net/http"
	"strconv"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "task.batch_delete", "批量删除任务 %s", auditRefs(names, req.IDs))
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logs, err := h.svc.Logs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	jobID, err := h.queue.Enqueue(id, nil, "",
		service.Trigger{Type: "manual", By: c.GetString("username"), IP: c.ClientIP()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// LogsAll 分页/筛选查询全部发送日志（筛选条件后端下推 DB）。
// LogsAll 发送日志（分页/筛选）
// @Summary 分页查询全部发送日志
// @Tags 任务
// @Security BearerAuth
// @Param task_id query int false "任务 ID"
// @Param status query string false "状态 success/failed"
// @Param from query string false "开始日期 YYYY-MM-DD"
// @Param to query string false "结束日期 YYYY-MM-DD"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/logs [get]
func (h *TaskHandler) LogsAll(c *gin.Context) {
	f := repository.LogFilter{Page: 1, PageSize: 50}
	if v := c.Query("task_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.TaskID = n
		}
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
	total, logs, err := h.svc.QueryLogs(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": logs})
}
