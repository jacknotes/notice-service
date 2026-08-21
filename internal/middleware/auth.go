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
		// 每次请求回查用户状态：被禁用（软删除）的用户其已签发令牌立即失效，
		// 而不是等到 24h 令牌自然过期（与「停用渠道不再参与投递」同口径）。
		if !svc.UserActive(claims.UserID) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "账号已被禁用"})
			return
		}
		c.Set("uid", claims.UserID)
		c.Set("role", claims.Role)
		c.Set("username", svc.GetUsername(claims.UserID))
		c.Next()
	}
}
