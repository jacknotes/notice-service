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

func (h *TaskHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

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
	auditf(c, h.db, "task.update", "更新任务 id=%d", id)
	c.JSON(http.StatusOK, in)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "task.delete", "删除任务 id=%d", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchDelete 批量删除任务（仅 admin）。
func (h *TaskHandler) BatchDelete(c *gin.Context) {
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
	auditf(c, h.db, "task.batch_delete", "批量删除任务 ids=%v", req.IDs)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

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
	auditf(c, h.db, "task.toggle", "任务 id=%d enabled=%v", id, req.Enabled)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

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
func (h *TaskHandler) SendNow(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := h.svc.Get(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	jobID, err := h.queue.Enqueue(id, nil, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "task.send_now", "立即发送任务 id=%d job=%d", id, jobID)
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

// LogsAll 分页/筛选查询全部发送日志（筛选条件后端下推 DB）。
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
	total, logs, err := h.svc.QueryLogs(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": logs})
}
