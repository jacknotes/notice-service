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

	r.GET("/api/health", handler.Health)

	api := r.Group("/api")
	api.POST("/auth/login", authH.Login)

	auth := api.Group("")
	auth.Use(middleware.Auth(authSvc))
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/auth/me", authH.Me)
		auth.POST("/auth/change-password", authH.ChangePassword)

		auth.GET("/channels", channelH.List)
		auth.POST("/channels", channelH.Create)
		auth.PUT("/channels/:id", channelH.Update)
		auth.DELETE("/channels/:id", channelH.Delete)
		auth.POST("/channels/:id/test", channelH.Test)

		auth.GET("/templates", templateH.List)
		auth.POST("/templates", templateH.Create)
		auth.PUT("/templates/:id", templateH.Update)
		auth.DELETE("/templates/:id", templateH.Delete)
		auth.POST("/templates/:id/preview", templateH.Preview)

		auth.GET("/tasks", taskH.List)
		auth.POST("/tasks", taskH.Create)
		auth.PUT("/tasks/:id", taskH.Update)
		auth.DELETE("/tasks/:id", taskH.Delete)
		auth.POST("/tasks/:id/toggle", taskH.Toggle)
		auth.GET("/tasks/:id/logs", taskH.Logs)

		auth.GET("/dashboard/stats", dashH.Stats)
		auth.GET("/dashboard/trend", dashH.Trend)
	}
	api.POST("/webhook/:api_key", webhookH.Trigger)

	return r
}
