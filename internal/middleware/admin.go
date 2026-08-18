package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminOnly 仅允许管理员继续访问；其它角色返回 403。
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权操作"})
			return
		}
		c.Next()
	}
}
