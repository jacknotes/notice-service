package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/handler"
	"notice-service/internal/middleware"
	"notice-service/internal/scheduler"
	"notice-service/internal/service"
)

func NewRouter(db *sql.DB, authSvc *service.AuthService, cipher *crypto.Cipher, sched *scheduler.Scheduler) *gin.Engine {
	r := gin.Default()

	authH := handler.NewAuthHandler(authSvc)
	channelH := handler.NewChannelHandler(db, cipher)
	templateH := handler.NewTemplateHandler(db)
	taskH := handler.NewTaskHandler(db, sched)
	webhookH := handler.NewWebhookHandler(db, cipher)
	dashH := handler.NewDashboardHandler(db)
	userH := handler.NewUserHandler(db)

	r.GET("/api/health", handler.Health)

	api := r.Group("/api")
	api.POST("/auth/login", authH.Login)

	auth := api.Group("")
	auth.Use(middleware.Auth(authSvc))
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/auth/me", authH.Me)
		auth.POST("/auth/change-password", authH.ChangePassword)

		// 读操作：所有登录用户可见全部共享数据
		auth.GET("/channels", channelH.List)
		auth.GET("/templates", templateH.List)
		auth.POST("/templates/:id/preview", templateH.Preview)
		auth.GET("/tasks", taskH.List)
		auth.GET("/tasks/:id/logs", taskH.Logs)

		auth.GET("/dashboard/stats", dashH.Stats)
		auth.GET("/dashboard/trend", dashH.Trend)

		// 写操作与用户管理：仅管理员
		admin := auth.Group("")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/channels", channelH.Create)
			admin.PUT("/channels/:id", channelH.Update)
			admin.DELETE("/channels/:id", channelH.Delete)
			admin.POST("/channels/:id/test", channelH.Test)
			admin.POST("/channels/batch-delete", channelH.BatchDelete)

			admin.POST("/templates", templateH.Create)
			admin.PUT("/templates/:id", templateH.Update)
			admin.DELETE("/templates/:id", templateH.Delete)
			admin.POST("/templates/batch-delete", templateH.BatchDelete)

			admin.POST("/tasks", taskH.Create)
			admin.PUT("/tasks/:id", taskH.Update)
			admin.DELETE("/tasks/:id", taskH.Delete)
			admin.POST("/tasks/:id/toggle", taskH.Toggle)
			admin.POST("/tasks/batch-delete", taskH.BatchDelete)

			admin.GET("/users", userH.List)
			admin.POST("/users", userH.Create)
			admin.PUT("/users/:id", userH.Update)
			admin.DELETE("/users/:id", userH.Delete)
			admin.POST("/users/batch-delete", userH.BatchDelete)
		}
	}
	api.POST("/webhook/:api_key", webhookH.Trigger)

	return r
}
