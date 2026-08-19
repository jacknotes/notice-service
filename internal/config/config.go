package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	EncryptKey string
	Port       string
	InstanceID string
	AdminUser  string
	AdminPass  string

	QueueWorkers          int
	QueuePollMS           int
	QueueMaxAttempts      int
	QueueRetryBackoff     []time.Duration
	QueueClaimTTL         time.Duration
	LogRetentionDays      int
	QueueJobRetentionDays int
}

func Load() *Config {
	loadDotEnv(".env") // 可选配置文件；已存在的环境变量优先，不会被覆盖
	return &Config{
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "notice"),
		DBPassword: getEnv("DB_PASSWORD", "notice123"),
		DBName:     getEnv("DB_NAME", "notice_service"),
		JWTSecret:  getEnv("JWT_SECRET", "change-me"),
		EncryptKey: resolveEncryptKey(),
		Port:       getEnv("PORT", "8080"),
		InstanceID: getEnv("INSTANCE_ID", uuid.NewString()),
		AdminUser:  getEnv("ADMIN_USER", "admin"),
		AdminPass:  getEnv("ADMIN_PASS", "admin123"),

		QueueWorkers:          getEnvInt("QUEUE_WORKERS", 4),
		QueuePollMS:           getEnvInt("QUEUE_POLL_MS", 1000),
		QueueMaxAttempts:      getEnvInt("QUEUE_MAX_ATTEMPTS", 3),
		QueueRetryBackoff:     parseDurations(getEnv("QUEUE_RETRY_BACKOFF", "5s,30s,60s"), []time.Duration{5 * time.Second, 30 * time.Second, 60 * time.Second}),
		QueueClaimTTL:         time.Duration(getEnvInt("QUEUE_CLAIM_TTL", 120)) * time.Second,
		LogRetentionDays:      getEnvInt("LOG_RETENTION_DAYS", 90),
		QueueJobRetentionDays: getEnvInt("QUEUE_JOB_RETENTION_DAYS", 30),
	}
}

// keyFile 未显式配置 ENCRYPT_KEY 时用于持久化密钥，保证重启后能解密已存的渠道配置。
const keyFile = ".notice-encrypt.key"

// resolveEncryptKey 优先用环境变量；否则读取本地密钥文件，不存在则生成并持久化。
func resolveEncryptKey() string {
	if v := os.Getenv("ENCRYPT_KEY"); v != "" {
		return v
	}
	if b, err := os.ReadFile(keyFile); err == nil && len(b) >= 32 {
		return string(b[:32])
	}
	k := randomHex(16)
	_ = os.WriteFile(keyFile, []byte(k), 0o600)
	return k
}

func (c *Config) DSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" +
		c.DBName + "?parseTime=true&charset=utf8mb4&loc=Local"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// parseDurations 解析逗号分隔的 duration 列表；任一解析失败或为空时回退到 def。
func parseDurations(s string, def []time.Duration) []time.Duration {
	if strings.TrimSpace(s) == "" {
		return def
	}
	parts := strings.Split(s, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, p := range parts {
		d, err := time.ParseDuration(strings.TrimSpace(p))
		if err != nil {
			return def
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// loadDotEnv 读取简单的 KEY=VALUE 配置文件（每行一条，支持 # 注释与引号）。
// 仅当对应环境变量未设置时才写入，保证显式环境变量优先。
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" {
			continue
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}
