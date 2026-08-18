package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/service"
)

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(db *sql.DB, sched service.Scheduler) *TaskHandler {
	return &TaskHandler{svc: service.NewTaskService(db, sched)}
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
	c.JSON(http.StatusOK, in)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
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
