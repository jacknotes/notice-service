package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("INSTANCE_ID", "")
	cfg := Load()
	if cfg.DBHost != "127.0.0.1" {
		t.Errorf("DBHost default = %q, want 127.0.0.1", cfg.DBHost)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default = %q, want 8080", cfg.Port)
	}
	if cfg.EncryptKey == "" {
		t.Error("EncryptKey should default to a non-empty generated key")
	}
	if cfg.InstanceID == "" {
		t.Error("InstanceID should default to a generated uuid")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("PORT", "9090")
	cfg := Load()
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %q, want db.example.com", cfg.DBHost)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}

func TestParseDurations(t *testing.T) {
	d := parseDurations("1s,30s,60s", nil)
	if len(d) != 3 || d[0] != time.Second || d[1] != 30*time.Second || d[2] != time.Minute {
		t.Errorf("parseDurations = %v", d)
	}
	fallback := []time.Duration{time.Second}
	if d2 := parseDurations("bad", fallback); len(d2) != 1 || d2[0] != time.Second {
		t.Errorf("bad input should fall back, got %v", d2)
	}
	if d3 := parseDurations("", fallback); len(d3) != 1 {
		t.Errorf("empty input should fall back, got %v", d3)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("Q_X", "7")
	if n := getEnvInt("Q_X", 1); n != 7 {
		t.Errorf("getEnvInt = %d, want 7", n)
	}
	if n := getEnvInt("Q_UNSET", 3); n != 3 {
		t.Errorf("getEnvInt default = %d, want 3", n)
	}
	t.Setenv("Q_BAD", "abc")
	if n := getEnvInt("Q_BAD", 3); n != 3 {
		t.Errorf("getEnvInt bad = %d, want 3", n)
	}
}

func TestLoadQueueDefaults(t *testing.T) {
	t.Setenv("QUEUE_WORKERS", "")
	t.Setenv("QUEUE_POLL_MS", "")
	t.Setenv("QUEUE_MAX_ATTEMPTS", "")
	t.Setenv("QUEUE_RETRY_BACKOFF", "")
	t.Setenv("QUEUE_CLAIM_TTL", "")
	t.Setenv("LOG_RETENTION_DAYS", "")
	t.Setenv("QUEUE_JOB_RETENTION_DAYS", "")
	cfg := Load()
	if cfg.QueueWorkers != 4 || cfg.QueuePollMS != 1000 || cfg.QueueMaxAttempts != 3 {
		t.Errorf("queue numeric defaults = %d/%d/%d", cfg.QueueWorkers, cfg.QueuePollMS, cfg.QueueMaxAttempts)
	}
	if len(cfg.QueueRetryBackoff) != 3 || cfg.QueueRetryBackoff[0] != 5*time.Second {
		t.Errorf("backoff default = %v", cfg.QueueRetryBackoff)
	}
	if cfg.QueueClaimTTL != 120*time.Second {
		t.Errorf("claim ttl default = %v", cfg.QueueClaimTTL)
	}
	if cfg.LogRetentionDays != 90 || cfg.QueueJobRetentionDays != 30 {
		t.Errorf("retention defaults = %d/%d", cfg.LogRetentionDays, cfg.QueueJobRetentionDays)
	}
}

func TestLoadQueueFromEnv(t *testing.T) {
	t.Setenv("QUEUE_WORKERS", "8")
	t.Setenv("QUEUE_RETRY_BACKOFF", "1s,2s,3s")
	cfg := Load()
	if cfg.QueueWorkers != 8 {
		t.Errorf("QueueWorkers = %d, want 8", cfg.QueueWorkers)
	}
	if len(cfg.QueueRetryBackoff) != 3 || cfg.QueueRetryBackoff[2] != 3*time.Second {
		t.Errorf("QueueRetryBackoff = %v", cfg.QueueRetryBackoff)
	}
}

func TestWeakSecretWarnings(t *testing.T) {
	weak := &Config{JWTSecret: "change-me", EncryptKey: "0123456789abcdef0123456789abcdef"}
	if n := len(weak.WeakSecretWarnings()); n != 2 {
		t.Errorf("weak secrets should produce 2 warnings, got %d", n)
	}
	strong := &Config{JWTSecret: "random-secret-1234567890", EncryptKey: "abcdef0123456789abcdef0123456789"}
	if n := len(strong.WeakSecretWarnings()); n != 0 {
		t.Errorf("strong secrets should produce 0 warnings, got %d: %v", n, strong.WeakSecretWarnings())
	}
}

func TestLoadFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yml := dir + "/config.yml"
	if err := os.WriteFile(yml, []byte(`
server:
  port: "9090"
database:
  host: dbhost
  port: "3307"
  user: dbuser
  password: dbpass
  name: dbname
jwt_secret: file-secret
encrypt_key: 0123456789abcdef0123456789abcdef
admin:
  user: fileadmin
  pass: filepass
queue:
  workers: 6
  poll_ms: 500
log_retention_days: 45
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadFile(yml)
	if cfg.Port != "9090" {
		t.Errorf("port = %q want 9090", cfg.Port)
	}
	if cfg.DBHost != "dbhost" || cfg.DBPort != "3307" {
		t.Errorf("db = %s:%s want dbhost:3307", cfg.DBHost, cfg.DBPort)
	}
	if cfg.DBUser != "dbuser" || cfg.DBPassword != "dbpass" || cfg.DBName != "dbname" {
		t.Errorf("db creds = %s/%s/%s", cfg.DBUser, cfg.DBPassword, cfg.DBName)
	}
	if cfg.JWTSecret != "file-secret" {
		t.Errorf("jwt = %q want file-secret", cfg.JWTSecret)
	}
	if cfg.AdminUser != "fileadmin" || cfg.AdminPass != "filepass" {
		t.Errorf("admin = %s/%s", cfg.AdminUser, cfg.AdminPass)
	}
	if cfg.QueueWorkers != 6 {
		t.Errorf("workers = %d want 6", cfg.QueueWorkers)
	}
	if cfg.QueuePollMS != 500 {
		t.Errorf("poll_ms = %d want 500", cfg.QueuePollMS)
	}
	if cfg.LogRetentionDays != 45 {
		t.Errorf("log_retention_days = %d want 45", cfg.LogRetentionDays)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yml := dir + "/config.yml"
	if err := os.WriteFile(yml, []byte("database:\n  host: filehost\nserver:\n  port: \"8080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_HOST", "envhost")
	t.Setenv("PORT", "7070")
	cfg := LoadFile(yml)
	if cfg.DBHost != "envhost" {
		t.Errorf("DBHost = %q want envhost (env overrides file)", cfg.DBHost)
	}
	if cfg.Port != "7070" {
		t.Errorf("Port = %q want 7070 (env overrides file)", cfg.Port)
	}
}

func TestLoadFileMissingFallsBackToDefaults(t *testing.T) {
	clearEnv(t)
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if cfg.DBHost != "127.0.0.1" || cfg.Port != "8080" || cfg.QueueWorkers != 4 {
		t.Errorf("missing file should fall back to defaults, got %s:%s workers=%d", cfg.DBHost, cfg.Port, cfg.QueueWorkers)
	}
}

// clearEnv 清空会影响配置解析的环境变量，保证测试与宿主 shell 环境隔离。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"JWT_SECRET", "ENCRYPT_KEY", "PORT", "INSTANCE_ID",
		"ADMIN_USER", "ADMIN_PASS",
		"QUEUE_WORKERS", "QUEUE_POLL_MS", "QUEUE_MAX_ATTEMPTS", "QUEUE_RETRY_BACKOFF",
		"QUEUE_CLAIM_TTL", "LOG_RETENTION_DAYS", "QUEUE_JOB_RETENTION_DAYS", "AUDIT_RETENTION_DAYS",
		"TRUSTED_PROXIES", "SWAGGER_ENABLED",
		"METRICS_ENABLED", "METRICS_USER", "METRICS_PASSWORD",
		"ENCRYPT_KEY_FILE", "STATIC_DIR",
	} {
		t.Setenv(k, "")
	}
}

func TestTrustedProxiesAndSwaggerDefaults(t *testing.T) {
	clearEnv(t)
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	// 默认信任环回（宿主 Nginx 反代形态）+ Docker 默认网桥网段（容器反代形态）；
	// Swagger/Metrics 端点默认关闭（安全基线），需要时经 SWAGGER_ENABLED/METRICS_ENABLED 显式开启。
	if len(cfg.TrustedProxies) != 3 || cfg.TrustedProxies[0] != "127.0.0.1" || cfg.TrustedProxies[2] != "172.16.0.0/12" {
		t.Errorf("TrustedProxies default = %v, want [127.0.0.1 ::1 172.16.0.0/12]", cfg.TrustedProxies)
	}
	if cfg.SwaggerEnabled {
		t.Error("SwaggerEnabled should default false")
	}
	if cfg.MetricsEnabled {
		t.Error("MetricsEnabled should default false")
	}
}

func TestTrustedProxiesFromEnvAndFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yml := dir + "/config.yml"
	if err := os.WriteFile(yml, []byte("trusted_proxies: \"10.0.0.0/8, 172.16.0.1\"\nswagger_enabled: false\naudit_retention_days: 90\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadFile(yml)
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.0/8" || cfg.TrustedProxies[1] != "172.16.0.1" {
		t.Errorf("TrustedProxies from file = %v", cfg.TrustedProxies)
	}
	if cfg.SwaggerEnabled {
		t.Error("swagger_enabled=false should disable swagger")
	}
	if cfg.AuditRetentionDays != 90 {
		t.Errorf("AuditRetentionDays = %d, want 90", cfg.AuditRetentionDays)
	}
	// 环境变量优先于文件
	t.Setenv("TRUSTED_PROXIES", "192.168.0.0/16")
	t.Setenv("SWAGGER_ENABLED", "1")
	cfg2 := LoadFile(yml)
	if len(cfg2.TrustedProxies) != 1 || cfg2.TrustedProxies[0] != "192.168.0.0/16" {
		t.Errorf("TrustedProxies env override = %v", cfg2.TrustedProxies)
	}
	if !cfg2.SwaggerEnabled {
		t.Error("SWAGGER_ENABLED=1 should override file false")
	}
}

func TestDSNHasTimeouts(t *testing.T) {
	c := &Config{DBHost: "h", DBPort: "3306", DBUser: "u", DBPassword: "p", DBName: "n"}
	dsn := c.DSN()
	if !strings.Contains(dsn, "timeout=5s") || !strings.Contains(dsn, "readTimeout=10s") || !strings.Contains(dsn, "writeTimeout=10s") {
		t.Errorf("DSN should include connection timeouts: %s", dsn)
	}
}

func TestEncryptKeyFileAndAutoFlag(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENCRYPT_KEY", "")
	t.Setenv("ENCRYPT_KEY_FILE", t.TempDir()+"/my.key")
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if !cfg.EncryptKeyAutoGenerated {
		t.Error("no key anywhere -> should be auto-generated")
	}
	if cfg.EncryptKeyFile == "" {
		t.Error("EncryptKeyFile should default/resolve")
	}
	t.Setenv("ENCRYPT_KEY", "0123456789abcdef0123456789abcdef")
	cfg2 := LoadFile(t.TempDir() + "/nope.yml")
	if cfg2.EncryptKeyAutoGenerated {
		t.Error("explicit ENCRYPT_KEY -> not auto-generated")
	}
}

func TestStaticDirDefault(t *testing.T) {
	clearEnv(t)
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if cfg.StaticDir != "./web/dist" {
		t.Errorf("StaticDir default = %q, want ./web/dist", cfg.StaticDir)
	}
	t.Setenv("STATIC_DIR", "/srv/static")
	cfg2 := LoadFile(t.TempDir() + "/nope.yml")
	if cfg2.StaticDir != "/srv/static" {
		t.Errorf("STATIC_DIR override = %q", cfg2.StaticDir)
	}
}
