package config

import (
	"os"
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
	cfg := LoadFile(t.TempDir() + "/nope.yml")
	if cfg.DBHost != "127.0.0.1" || cfg.Port != "8080" || cfg.QueueWorkers != 4 {
		t.Errorf("missing file should fall back to defaults, got %s:%s workers=%d", cfg.DBHost, cfg.Port, cfg.QueueWorkers)
	}
}
