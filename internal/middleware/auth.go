package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notice-service/internal/service"
)

func Auth(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		claims, err := svc.VerifyToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		// 每次请求回查用户当前状态与角色：被禁用（软删除）的用户其已签发令牌立即失效；
		// 角色也以 DB 为准——提权/降级在下一个请求即生效，而不是等 JWT 自然过期。
		u, err := svc.User(claims.UserID)
		if err != nil || !u.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "账号已被禁用"})
			return
		}
		c.Set("uid", u.ID)
		c.Set("role", u.Role)
		c.Set("username", u.Username)
		c.Next()
	}
}
