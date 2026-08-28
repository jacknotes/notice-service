package router

import (
	"crypto/subtle"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"notice-service/internal/crypto"
	"notice-service/internal/handler"
	"notice-service/internal/metrics"
	"notice-service/internal/middleware"
	"notice-service/internal/scheduler"
	"notice-service/internal/service"
)

// Options 路由可选项（生产环境由 main 传入，测试用默认值）。
type Options struct {
	// TrustedProxies 反向代理 CIDR 列表；只有来自这些地址的 X-Forwarded-For /
	// X-Real-IP 头才被信任（Webhook IP 白名单与操作审计来源 IP 依赖）。
	// 默认值见 config：环回 + Docker 默认网桥网段 172.16.0.0/12。空 = 不信任任何代理头。
	TrustedProxies []string
	// SwaggerEnabled 是否暴露 /swagger API 文档。
	SwaggerEnabled bool
	// MaxBodyBytes 请求体大小上限（字节）。
	MaxBodyBytes int64
	// MetricsEnabled 是否暴露 /metrics（Prometheus）。默认关闭。
	MetricsEnabled bool
	// MetricsUser / MetricsPassword 同时非空时 /metrics 需 Basic Auth。
	MetricsUser     string
	MetricsPassword string
	// BuildVersion / InstanceID 注入到 /api/system/version 与节点心跳同源的版本口径。
	BuildVersion string
	InstanceID   string
}

func NewRouter(db *sql.DB, authSvc *service.AuthService, cipher *crypto.Cipher, sched *scheduler.Scheduler, queue *service.QueueService, opts ...Options) *gin.Engine {
	o := Options{SwaggerEnabled: true, MaxBodyBytes: 1 << 20}
	if len(opts) > 0 {
		if opts[0].SwaggerEnabled {
			o.SwaggerEnabled = true
		}
		if opts[0].MaxBodyBytes > 0 {
			o.MaxBodyBytes = opts[0].MaxBodyBytes
		}
		o.TrustedProxies = opts[0].TrustedProxies
		o.MetricsEnabled = opts[0].MetricsEnabled
		o.MetricsUser = opts[0].MetricsUser
		o.MetricsPassword = opts[0].MetricsPassword
		o.BuildVersion = opts[0].BuildVersion
		o.InstanceID = opts[0].InstanceID
	}

	// 不用 gin.Default()（它信任全部代理头）；显式 New + Recovery + 自研
	// 访问日志（对 /api/webhook/<api_key> 路径做脱敏，防 Key 泄漏进日志）。
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(securityHeaders()) // 全局安全响应头（覆盖 SPA 页面与全部 API）
	r.Use(accessLogger())

	// 可信代理：控制 X-Forwarded-For / X-Real-IP 的信任范围（默认信任环回 +
	// Docker 默认网桥网段，见 config 的 TrustedProxies 默认值）。
	if err := r.SetTrustedProxies(o.TrustedProxies); err != nil {
		log.Printf("router: invalid trusted proxies %v: %v", o.TrustedProxies, err)
	}

	authH := handler.NewAuthHandler(db, authSvc)
	channelH := handler.NewChannelHandler(db, cipher)
	templateH := handler.NewTemplateHandler(db)
	// 显式把 *scheduler.Scheduler 转成 service.Scheduler 接口：sched 为 nil 时保持
	// 真正的 nil 接口（避免 typed-nil 陷阱导致 TaskService 误判非空而调用 nil 指针）。
	var schedIf service.Scheduler
	if sched != nil {
		schedIf = sched
	}
	taskH := handler.NewTaskHandler(db, schedIf, queue)
	webhookH := handler.NewWebhookHandler(db, queue)
	categoryH := handler.NewCategoryHandler(db)
	dashH := handler.NewDashboardHandler(db)
	userH := handler.NewUserHandler(db)
	auditH := handler.NewAuditHandler(db)
	sysH := handler.NewSystemHandler(db, o.BuildVersion, o.InstanceID)

	r.GET("/api/health", handler.Health(db))
	if o.MetricsEnabled {
		m := r.Group("")
		if o.MetricsUser != "" && o.MetricsPassword != "" {
			m.Use(metricsBasicAuth(o.MetricsUser, o.MetricsPassword))
		}
		m.GET("/metrics", gin.WrapH(metrics.Handler()))
	}
	if o.SwaggerEnabled {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	api := r.Group("/api")
	api.Use(bodyLimit(o.MaxBodyBytes))
	api.POST("/auth/login", authH.Login)
	api.POST("/auth/forgot-password", authH.ForgotPassword) // 公开：一次性令牌自助重置密码
	api.POST("/auth/2fa/verify", authH.Verify2FA)           // 公开：登录第二步（持待验证令牌）

	auth := api.Group("")
	auth.Use(middleware.Auth(authSvc))
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/auth/me", authH.Me)
		auth.PUT("/auth/profile", authH.UpdateProfile) // 自助修改显示名/邮箱
		auth.POST("/auth/change-password", authH.ChangePassword)
		auth.POST("/auth/2fa/setup", authH.Setup2FA)
		auth.POST("/auth/2fa/enable", authH.Enable2FA)
		auth.POST("/auth/2fa/disable", authH.Disable2FA)

		// 读操作：所有登录用户可见全部共享数据
		auth.GET("/channels", channelH.List)
		auth.GET("/templates", templateH.List)
		auth.GET("/categories", categoryH.List)
		auth.GET("/categories/unused", categoryH.Unused)
		auth.POST("/templates/:id/preview", templateH.Preview)
		auth.GET("/tasks", taskH.List)
		auth.POST("/tasks/preview", taskH.Preview) // 任务发送预览（渲染，不发送）
		auth.GET("/tasks/:id/logs", taskH.Logs)
		auth.GET("/logs", taskH.LogsAll)     // 日志分页/筛选（后端下推）
		auth.GET("/logs/:id", taskH.LogByID) // 单条日志详情（完整内容）

		auth.GET("/dashboard/stats", dashH.Stats)
		auth.GET("/dashboard/trend", dashH.Trend)
		auth.GET("/dashboard/top-tasks", dashH.TopTasks)
		auth.GET("/dashboard/channel-stats", dashH.ChannelStats)

		auth.GET("/instances", sysH.Instances) // 后端节点健康列表（信号在线）
		auth.GET("/system/version", sysH.Version) // 当前服务版本（与心跳 version 同源）

		// 写操作与用户管理：仅管理员
		admin := auth.Group("")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/channels", channelH.Create)
			admin.PUT("/channels/:id", channelH.Update)
			admin.DELETE("/channels/:id", channelH.Delete)
			admin.POST("/channels/:id/test", channelH.Test)
			admin.POST("/channels/batch-delete", channelH.BatchDelete)

			admin.POST("/categories", categoryH.Create)
			admin.DELETE("/categories/:name", categoryH.Delete)

			admin.POST("/templates", templateH.Create)
			admin.PUT("/templates/:id", templateH.Update)
			admin.DELETE("/templates/:id", templateH.Delete)
			admin.POST("/templates/batch-delete", templateH.BatchDelete)

			admin.POST("/tasks", taskH.Create)
			admin.PUT("/tasks/:id", taskH.Update)
			admin.DELETE("/tasks/:id", taskH.Delete)
			admin.POST("/tasks/:id/toggle", taskH.Toggle)
			admin.POST("/tasks/:id/send", taskH.SendNow) // 立即发送（入队）
			admin.POST("/tasks/batch-delete", taskH.BatchDelete)
			admin.POST("/logs/:id/retry", taskH.RetryLog) // 重试失败日志（定向重发）
			admin.GET("/logs/export", taskH.ExportLogs)   // 发送日志 CSV 导出（仅管理员）

			admin.GET("/users", userH.List)
			admin.POST("/users", userH.Create)
			admin.PUT("/users/:id", userH.Update)
			admin.DELETE("/users/:id", userH.Delete)
			admin.POST("/users/:id/reset-token", userH.ResetToken) // 生成一次性重置令牌
			admin.POST("/users/batch-delete", userH.BatchDelete)
			admin.POST("/users/:id/2fa-enable", userH.ForceEnable2FA)   // 强制开启双因子认证
			admin.POST("/users/:id/2fa-disable", userH.ForceDisable2FA) // 强制关闭双因子认证
			admin.POST("/users/:id/disable", userH.Disable)             // 禁用用户（登录/令牌立即失效，可重新启用）
			admin.POST("/users/:id/enable", userH.Enable)               // 启用用户

			admin.GET("/audit", auditH.List) // 操作审计日志

			expH := handler.NewExportHandler(db, cipher, schedIf)
			admin.GET("/export", expH.Export)  // 导出渠道/模板/任务 JSON 备份（仅管理员）
			admin.POST("/import", expH.Import) // 导入渠道/模板/任务 JSON 备份（仅管理员）
		}
	}
	api.POST("/webhook/:api_key", webhookH.Trigger)

	return r
}

// accessLogger 访问日志：对 /api/webhook/<api_key> 路径脱敏（Key 不落日志）。
func accessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// 指标 path 标签用路由模板（c.FullPath()），避免 api_key 泄漏与路径基数爆炸；
		// NoRoute/404 时 FullPath 为空，统一归入 /<unmatched>。
		pathLabel := c.FullPath()
		if pathLabel == "" {
			pathLabel = "/<unmatched>"
		}
		metrics.HTTPRequests.WithLabelValues(strconv.Itoa(c.Writer.Status()), c.Request.Method, pathLabel).Inc()
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/webhook/") {
			path = "/api/webhook/<redacted>"
		}
		log.Printf("[http] %s %s %d %s %s",
			c.Request.Method, path, c.Writer.Status(),
			time.Since(start).Round(time.Millisecond), c.ClientIP())
	}
}

// metricsBasicAuth /metrics 的可选 Basic Auth（两字段都非空才要求）。
func metricsBasicAuth(user, pass string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, p, ok := c.Request.BasicAuth()
		okUser := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		okPass := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1
		if !ok || !okUser || !okPass {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// securityHeaders 基础安全响应头。
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 仅信任本服务资源 + 内联主题脚本 + Google Fonts；connect-src 'self'
		// 阻止跨站请求外带数据。
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data:; connect-src 'self'; frame-ancestors 'self'")
		c.Next()
	}
}

// bodyLimit 限制请求体大小，防超大请求拖垮内存。
func bodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Go 1.25 起 http.NewRequest(..., nil) 生成的请求 Body 为 nil；
		// MaxBytesReader 包装 nil 会得到内部 reader 为 nil 的 maxBytesReader，
		// 读取请求体时 panic（如空 body 的 webhook POST）。统一把 nil 转成
		// http.NoBody（读到即 io.EOF），既有非空请求行为不变。
		body := c.Request.Body
		if body == nil {
			body = http.NoBody
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, body, maxBytes)
		c.Next()
	}
}
