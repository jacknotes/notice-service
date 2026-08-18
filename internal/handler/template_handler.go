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
}

func NewTemplateHandler(db *sql.DB) *TemplateHandler {
	return &TemplateHandler{svc: service.NewTemplateService(db)}
}

func (h *TemplateHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

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
	c.JSON(http.StatusOK, in)
}

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
	c.JSON(http.StatusOK, in)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(c.GetInt64("uid"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

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
