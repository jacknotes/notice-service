package handler

import (
	"database/sql"
	"fmt"

	"github.com/gin-gonic/gin"

	"notice-service/internal/repository"
)

// auditCtx 记录当前登录用户的操作到 audit_logs（失败静默，不阻断业务）。
func auditCtx(c *gin.Context, db *sql.DB, action, detail string) {
	uid := c.GetInt64("uid")
	var username string
	if db != nil {
		_ = db.QueryRow("SELECT username FROM users WHERE id=? AND deleted_at IS NULL", uid).Scan(&username)
	}
	_ = repository.NewAuditRepo(db).Create(&repository.AuditLog{
		UserID: uid, Username: username, Action: action, Detail: detail,
	})
}

// auditActor 记录指定操作者的操作（登录/登出等无中间件上下文场景）。
func auditActor(db *sql.DB, uid int64, username, action, detail string) {
	_ = repository.NewAuditRepo(db).Create(&repository.AuditLog{
		UserID: uid, Username: username, Action: action, Detail: detail,
	})
}

// auditf 是 auditCtx 的格式化便捷版。
func auditf(c *gin.Context, db *sql.DB, action, format string, args ...interface{}) {
	auditCtx(c, db, action, fmt.Sprintf(format, args...))
}
