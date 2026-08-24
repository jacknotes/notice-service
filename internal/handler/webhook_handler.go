package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type WebhookHandler struct {
	repo      *repository.TaskRepo
	queue     *service.QueueService
	rateLimit *repository.RateLimitRepo
}

func NewWebhookHandler(db *sql.DB, queue *service.QueueService) *WebhookHandler {
	return &WebhookHandler{
		repo:      repository.NewTaskRepo(db),
		queue:     queue,
		rateLimit: repository.NewRateLimitRepo(db),
	}
}

// Trigger Webhook 触发
// @Summary 用 API Key 触发任务（无需登录）
// @Tags Webhook
// @Param api_key path string true "任务 API Key"
// @Accept json
// @Param body body object true "变量"
// @Success 202 {object} map[string]interface{}
// @Router /api/webhook/{api_key} [post]
func (h *WebhookHandler) Trigger(c *gin.Context) {
	// API Key 优先从 header 读取（防路径泄漏进日志）；兼容旧调用支持 URL path。
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		apiKey = c.Param("api_key")
	}
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 API Key"})
		return
	}
	// 集中式限流：每 api_key 60 次/分钟（多实例共享，DB 计数）。DB 故障时 fail-open。
	allowed, err := h.rateLimit.Allow("webhook:"+apiKey, time.Minute, 60)
	if err != nil {
		log.Printf("webhook: rate limit check failed: %v", err)
	} else if !allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
		return
	}
	task, err := h.repo.GetByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_key 无效"})
		return
	}
	if !task.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "任务已禁用"})
		return
	}
	if !h.ipAllowed(task, c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP 不在白名单"})
		return
	}
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是合法 JSON"})
		return
	}
	// 异步入队：请求立即返回 202，发送由后台 worker 池消费（含重试/崩溃接管）。
	jobID, err := h.queue.Enqueue(task.ID, req.Variables, "",
		service.Trigger{Type: "webhook", By: "webhook", IP: c.ClientIP()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "job_id": jobID})
}

func (h *WebhookHandler) ipAllowed(task *model.Task, c *gin.Context) bool {
	if task.AllowedIPsJSON == "" || task.AllowedIPsJSON == "[]" || task.AllowedIPsJSON == "null" {
		return true
	}
	var ips []string
	if err := json.Unmarshal([]byte(task.AllowedIPsJSON), &ips); err != nil {
		return false // 配置损坏 → 拒绝
	}
	if len(ips) == 0 {
		return true
	}
	remote := c.ClientIP()
	for _, allow := range ips {
		if ipMatches(allow, remote) {
			return true
		}
	}
	return false
}

func ipMatches(allow, remote string) bool {
	if strings.Contains(allow, "/") {
		_, ipnet, err := net.ParseCIDR(allow)
		if err != nil {
			return false
		}
		ip := net.ParseIP(remote)
		return ip != nil && ipnet.Contains(ip)
	}
	return allow == remote
}
