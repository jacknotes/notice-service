package main

import (
	"crypto/sha256"
	"log"
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
)

func main() {
	cfg := config.Load()

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
		Workers:           cfg.QueueWorkers,
		PollInterval:      time.Duration(cfg.QueuePollMS) * time.Millisecond,
		MaxAttempts:       cfg.QueueMaxAttempts,
		RetryBackoff:      cfg.QueueRetryBackoff,
		ClaimTTL:          cfg.QueueClaimTTL,
		LogRetentionDays:  cfg.LogRetentionDays,
		JobRetentionDays:  cfg.QueueJobRetentionDays,
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
	gin.SetMode(gin.ReleaseMode)

	log.Printf("notice-service listening on :%s (instance %s)", cfg.Port, cfg.InstanceID)
	if err := engine.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// padKey 保证 32 字节：过长截断，过短用 SHA-256 摘要补齐。
func padKey(s string) string {
	if len(s) >= 32 {
		return s[:32]
	}
	sum := sha256.Sum256([]byte(s))
	return string(sum[:])
}
