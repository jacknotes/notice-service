package handler

import (
	"strings"
	"database/sql"
	"fmt"

	"github.com/gin-gonic/gin"

	"notice-service/internal/repository"
)

// auditModule 由 action 前缀推导模块（前端按模块分组/筛选）：
// action 形如 "channel.create"、"task.send_now"；无前缀的特殊 action 单独映射。
func auditModule(action string) string {
	// 认证域：登录/登出/2FA
	if action == "logout" || strings.HasPrefix(action, "login.") {
		return "auth"
	}
	for i := 0; i < len(action); i++ {
		if action[i] == '.' {
			return action[:i]
		}
	}
	return "other"
}

// auditCtx 记录当前登录用户的操作到 audit_logs（失败静默，不阻断业务）。
// 自动带上请求来源 IP 与模块分类。
func auditCtx(c *gin.Context, db *sql.DB, action, detail string) {
	uid := c.GetInt64("uid")
	var username string
	if db != nil {
		_ = db.QueryRow("SELECT username FROM users WHERE id=? AND deleted_at IS NULL", uid).Scan(&username)
	}
	_ = repository.NewAuditRepo(db).Create(&repository.AuditLog{
		UserID: uid, Username: username, IP: c.ClientIP(),
		Action: action, Module: auditModule(action), Detail: detail,
	})
}

// auditActor 记录指定操作者的操作（登录/登出等无中间件上下文场景），ip 为请求来源。
func auditActor(db *sql.DB, uid int64, username, ip, action, detail string) {
	_ = repository.NewAuditRepo(db).Create(&repository.AuditLog{
		UserID: uid, Username: username, IP: ip,
		Action: action, Module: auditModule(action), Detail: detail,
	})
}

// auditf 是 auditCtx 的格式化便捷版。
func auditf(c *gin.Context, db *sql.DB, action, format string, args ...interface{}) {
	auditCtx(c, db, action, fmt.Sprintf(format, args...))
}
