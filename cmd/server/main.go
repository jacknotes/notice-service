package main

// @title Notice Service API
// @version 1.0.0
// @description 自托管通知发送服务 API（邮箱/企微/钉钉/飞书/PushPlus 多渠道投递）。
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/config"
	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/repository"
	"notice-service/internal/router"
	"notice-service/internal/scheduler"
	"notice-service/internal/service"

	_ "notice-service/docs/swagger" // 注册 Swagger 文档（swag init 生成）
)

func main() {
	cfg := config.Load()

	// Gin 模式：GIN_MODE 环境变量优先（debug/release/test），默认 release。
	// 必须在创建 engine（router.NewRouter）之前设置，否则启动横幅/请求日志按错误模式输出。
	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	// reset-password 子命令：唯一 admin 忘记密码时离线重置，不启动 HTTP 服务。
	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		os.Exit(runResetPasswordCmd(cfg))
	}

	// 弱默认密钥告警：防止以默认/示例密钥裸跑
	for _, w := range cfg.WeakSecretWarnings() {
		log.Printf("[警告] %s", w)
	}

	db, err := database.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ciph, err := crypto.New([]byte(padKey(cfg.EncryptKey)))
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}

	authSvc := service.NewAuthService(db, cfg.JWTSecret, cfg.AdminUser, cfg.AdminPass)
	if err := authSvc.BootstrapAdmin(); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	ns := service.NewNotificationService(db, ciph)

	// 发送队列：入队即落库，worker 池后台消费（重试/崩溃接管/清理都在这层）
	qcfg := service.QueueConfig{
		Workers:          cfg.QueueWorkers,
		PollInterval:     time.Duration(cfg.QueuePollMS) * time.Millisecond,
		MaxAttempts:      cfg.QueueMaxAttempts,
		RetryBackoff:     cfg.QueueRetryBackoff,
		ClaimTTL:         cfg.QueueClaimTTL,
		LogRetentionDays: cfg.LogRetentionDays,
		JobRetentionDays: cfg.QueueJobRetentionDays,
	}
	queue := service.NewQueueService(db, ns, qcfg, cfg.InstanceID)
	queue.Start()
	defer queue.Stop()

	// 调度器：cron 到点只做快速入队（毫秒级），带 dedupe key 防极端竞态重复
	sched := scheduler.New(func(taskID int64, dedupeKey string) {
		if _, err := queue.Enqueue(taskID, nil, dedupeKey); err != nil {
			log.Printf("scheduler: enqueue task %d failed: %v", taskID, err)
		}
	}, repository.NewTaskRepo(db), cfg.InstanceID)
	sched.Start()
	tasks, err := repository.NewTaskRepo(db).ListEnabledCron()
	if err != nil {
		log.Fatalf("load cron tasks: %v", err)
	}
	for _, t := range tasks {
		sched.RegisterTask(t.ID, t.CronExpr)
	}
	defer sched.Stop()

	engine := router.NewRouter(db, authSvc, ciph, sched, queue)
	engine.Static("/assets", "./web/dist/assets")
	engine.StaticFile("/", "./web/dist/index.html")
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		if c.Request.Method == "GET" {
			c.File("./web/dist/index.html")
			return
		}
		c.JSON(404, gin.H{"error": "not found"})
	})

	log.Printf("notice-service listening on :%s (instance %s)", cfg.Port, cfg.InstanceID)
	if err := engine.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// runResetPasswordCmd 处理 reset-password 子命令，返回进程退出码。
func runResetPasswordCmd(cfg *config.Config) int {
	username := cfg.AdminUser
	newPassword := ""
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.StringVar(&username, "username", cfg.AdminUser, "要重置的用户名（默认 ADMIN_USER）")
	fs.StringVar(&newPassword, "new-password", "", "新密码（缺省时交互式输入，不回显）")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return 2
	}
	if newPassword == "" {
		pw, err := promptNewPassword(os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取密码失败:", err)
			return 2
		}
		newPassword = pw
	}
	db, err := database.Open(cfg.DSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, "数据库连接失败:", err)
		return 1
	}
	defer db.Close()
	if err := resetPassword(db, username, newPassword); err != nil {
		fmt.Fprintln(os.Stderr, "重置失败:", err)
		return 1
	}
	fmt.Printf("已重置用户 %s 的密码\n", username)
	return 0
}

// padKey 保证 32 字节：过长截断，过短用 SHA-256 摘要补齐。
func padKey(s string) string {
	if len(s) >= 32 {
		return s[:32]
	}
	sum := sha256.Sum256([]byte(s))
	return string(sum[:])
}
