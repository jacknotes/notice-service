package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health 健康检查：除进程存活外，同时探测数据库连通性。
// 供负载均衡 / 容器健康检查 / 前端「信号在线」使用：DB 不可达时返回 503，
// 便于 LB 主动摘除故障实例。
func Health(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "db": "down"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
