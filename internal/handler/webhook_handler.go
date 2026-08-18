package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/service"
)

type WebhookHandler struct {
	repo *repository.TaskRepo
	ns   *service.NotificationService
}

func NewWebhookHandler(db *sql.DB, cipher *crypto.Cipher) *WebhookHandler {
	return &WebhookHandler{repo: repository.NewTaskRepo(db), ns: service.NewNotificationService(db, cipher)}
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	apiKey := c.Param("api_key")
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
	if err := h.ns.SendTask(task.ID, req.Variables); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	remote := clientIP(c)
	for _, allow := range ips {
		if ipMatches(allow, remote) {
			return true
		}
	}
	return false
}

// clientIP 解析客户端 IP。注意：X-Real-IP / X-Forwarded-For 头只有在服务部署于
// 可信反向代理（Nginx）之后才是可信的；若直接暴露公网，调用方可以伪造这些头绕过
// IP 白名单。生产环境务必通过 Nginx 反向代理暴露本服务。
func clientIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if fwd := c.GetHeader("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
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
