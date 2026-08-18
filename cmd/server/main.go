package main

import (
	"crypto/sha256"
	"log"
	"strings"

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

	// 调度器：加载已启用 cron 任务，用本实例 UUID 作租约锁身份
	sched := scheduler.New(func(taskID int64) {
		_ = ns.SendTask(taskID, nil)
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

	engine := router.NewRouter(db, authSvc, ciph, sched)
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
