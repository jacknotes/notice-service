package handler

import (
	"database/sql"
	"fmt"
	"strings"

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

// auditStr 格式化可选字符串：非 nil 输出值，nil 输出 "-"。
// 用于审计详情格式化，避免把 *string 指针以 %v/%s 打印成内存地址（如 0xc000…）。
func auditStr(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

// auditRef 生成「名称 (id=N)」形式的可读引用，用于审计详情；名称为空时退化为 "id=N"。
// 例如：删除用户 test1 (id=2)，使用户被删除后仍能看出操作对象是谁。
func auditRef(name string, id int64) string {
	if name == "" {
		return fmt.Sprintf("id=%d", id)
	}
	return fmt.Sprintf("%s (id=%d)", name, id)
}

// auditRefs 生成「名称1 (id=1)、名称2 (id=2)」形式的可读 ID 列表（批量操作审计用）。
func auditRefs(names []string, ids []int64) string {
	parts := make([]string, 0, len(ids))
	for i, id := range ids {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		parts = append(parts, auditRef(name, id))
	}
	return strings.Join(parts, "、")
}
