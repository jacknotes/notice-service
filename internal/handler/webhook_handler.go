package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

// keyRateLimiter 按 key 的固定窗口限流（内存态，单实例计数）。
type keyRateLimiter struct {
	mu       sync.Mutex
	hits     map[string]int
	windowAt map[string]time.Time
	limit    int
	window   time.Duration
}

func newKeyRateLimiter(limit int, window time.Duration) *keyRateLimiter {
	return &keyRateLimiter{
		hits:     map[string]int{},
		windowAt: map[string]time.Time{},
		limit:    limit,
		window:   window,
	}
}

func (l *keyRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if start, ok := l.windowAt[key]; !ok || now.Sub(start) >= l.window {
		l.windowAt[key] = now
		l.hits[key] = 1
		return true
	}
	l.hits[key]++
	return l.hits[key] <= l.limit
}

type WebhookHandler struct {
	repo    *repository.TaskRepo
	queue   *service.QueueService
	limiter *keyRateLimiter
}

func NewWebhookHandler(db *sql.DB, queue *service.QueueService) *WebhookHandler {
	return &WebhookHandler{
		repo:    repository.NewTaskRepo(db),
		queue:   queue,
		limiter: newKeyRateLimiter(60, time.Minute), // 每 api_key 每分钟 60 次
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
	if !h.limiter.allow(apiKey) {
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
	_ = c.ShouldBindJSON(&req)
	// 异步入队：请求立即返回 202，发送由后台 worker 池消费（含重试/崩溃接管）。
	// 触发来源：webhook，触发 IP 取可信反代判定后的客户端地址。
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
