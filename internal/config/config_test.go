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
	// 弱场景：默认 JWT/加密密钥 + JWT 长度不足 → 3 条告警
	// （ADMIN_PASS 已移出 WeakSecretWarnings，由 AdminPassWarnings 单独判定）
	weak := &Config{JWTSecret: "change-me", EncryptKey: "0123456789abcdef0123456789abcdef", AdminPass: "admin123"}
	if n := len(weak.WeakSecretWarnings()); n != 3 {
		t.Errorf("weak secrets should produce 3 warnings, got %d: %v", n, weak.WeakSecretWarnings())
	}
	strong := &Config{JWTSecret: "random-secret-1234567890-random-secret-long", EncryptKey: "abcdef0123456789abcdef0123456789", AdminPass: "Str0ng!Passphrase#2026"}
	if n := len(strong.WeakSecretWarnings()); n != 0 {
		t.Errorf("strong secrets should produce 0 warnings, got %d: %v", n, strong.WeakSecretWarnings())
	}
	// AdminPass 缺省（空）由 AdminPassWarnings 告警，WeakSecretWarnings 不涉及
	emptyPass := &Config{JWTSecret: "random-secret-1234567890-random-secret-long", EncryptKey: "abcdef0123456789abcdef0123456789"}
	if n := len(emptyPass.WeakSecretWarnings()); n != 0 {
		t.Errorf("WeakSecretWarnings 不应因空 ADMIN_PASS 告警，got %d", n)
	}
	if n := len(emptyPass.AdminPassWarnings()); n != 1 {
		t.Errorf("empty admin pass should produce 1 AdminPassWarning, got %d", n)
	}
}

// TestWeakSecretWarningsPlaceholders 回归 H-1：.env.example 的占位值长度 ≥32，
// 精确相等判断完全漏过（STRICT_SECRETS=1 形同虚设）。占位前缀必须告警。
func TestWeakSecretWarningsPlaceholders(t *testing.T) {
	const placeholder = "please_change_me_to_32_bytes_random_hex"
	cases := []struct {
		name string
		cfg  Config
	}{
		{"env.example 三个原值", Config{JWTSecret: placeholder, EncryptKey: placeholder, AdminPass: "please_change_me_to_a_strong_password"}},
		{"仅 JWT 占位", Config{JWTSecret: placeholder, EncryptKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90", AdminPass: "Str0ng!Passphrase#2026"}},
		{"仅 ENCRYPT 占位", Config{JWTSecret: "9f8e7d6c5b4a39281706f5e4d3c2b1a0", EncryptKey: placeholder, AdminPass: "Str0ng!Passphrase#2026"}},
		{"32 个 a 低熵", Config{JWTSecret: strings.Repeat("a", 32), EncryptKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90", AdminPass: "Str0ng!Passphrase#2026"}},
		{"ENCRYPT 低熵", Config{JWTSecret: "9f8e7d6c5b4a39281706f5e4d3c2b1a0", EncryptKey: strings.Repeat("ab", 16), AdminPass: "Str0ng!Passphrase#2026"}},
		{"占位前缀 change_me", Config{JWTSecret: "change_me_please_1234567890abcdef", EncryptKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90", AdminPass: "Str0ng!Passphrase#2026"}},
		{"占位前缀 your_", Config{JWTSecret: "your_jwt_secret_here_0123456789abcdef", EncryptKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90", AdminPass: "Str0ng!Passphrase#2026"}},
		{"占位前缀 example", Config{JWTSecret: "9f8e7d6c5b4a39281706f5e4d3c2b1a0", EncryptKey: "example_encrypt_key_0123456789ab", AdminPass: "Str0ng!Passphrase#2026"}},
		{"占位前缀 placeholder", Config{JWTSecret: "placeholder_secret_0123456789abcdef", EncryptKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90", AdminPass: "Str0ng!Passphrase#2026"}},
	}
	for _, tc := range cases {
		cfg := tc.cfg
		if w := cfg.WeakSecretWarnings(); len(w) == 0 {
			t.Errorf("%s: 占位/低熵密钥必须产生告警，实际 0 条", tc.name)
		}
	}
}

// TestAdminPassWarnings 回归 H-1c：ADMIN_PASS 走 BootstrapAdmin 建号，
// 没有强度校验，常见弱口令必须告警。
//
// 注意：判定入口是 AdminPassWarnings（而非 WeakSecretWarnings）——ADMIN_PASS
// 只在首次建号时生效，admin 已存在后与当前登录口令无关（可能是历史遗留值），
// 不应参与每次启动的弱密钥阻断。
func TestAdminPassWarnings(t *testing.T) {
	for _, pass := range []string{"x", "1", "a", "admin", "password", "admin123", "please_change_me"} {
		cfg := &Config{AdminPass: pass}
		if w := cfg.AdminPassWarnings(); len(w) == 0 {
			t.Errorf("ADMIN_PASS=%q 必须产生告警", pass)
		}
	}
}

// TestWeakSecretWarningsExcludesAdminPass 回归生产事故：WeakSecretWarnings
// 不再包含 ADMIN_PASS。它只校验每次启动都真正生效的 JWT_SECRET / ENCRYPT_KEY。
func TestWeakSecretWarningsExcludesAdminPass(t *testing.T) {
	strongJWT, strongKey := "9f8e7d6c5b4a39281706f5e4d3c2b1a0", "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	// 即使 ADMIN_PASS 是最弱的 admin123，WeakSecretWarnings 也应返回空
	cfg := &Config{JWTSecret: strongJWT, EncryptKey: strongKey, AdminPass: "admin123"}
	if w := cfg.WeakSecretWarnings(); len(w) != 0 {
		t.Errorf("WeakSecretWarnings 不应包含 ADMIN_PASS 告警，got %v", w)
	}
}

// TestAdminPassWarningsStrongNotFlagged 回归生产事故：ADMIN_PASS 曾被低熵判定
// （去重字符数 < 12）误判为弱口令，导致 16 位四类字符的强口令在
// STRICT_SECRETS=1 下拒绝启动、容器崩溃循环。
//
// 口令的字符集天然窄（字母+数字+符号），16 位强口令去重字符数常只有 8-11，
// 因此口令必须用「长度 + 字符类别」判定，不能用密钥用的去重字符数指标。
func TestAdminPassWarningsStrongNotFlagged(t *testing.T) {
	strongPasses := []string{
		"Abc12345def6789X", // 16 位：大小写+数字，去重字符数 < 12（旧逻辑会误判）
		"Passw0rdPassw0rd", // 16 位：大小写+数字，去重字符数 10
		"Notice2026abcDEF", // 16 位：大小写+数字，去重字符数 11
		"MySecret12345678", // 16 位：大小写+数字
		"Abcd1234!@#$efgh", // 16 位：四类字符
		"Str0ng!Passphrase#2026",
	}
	for _, pass := range strongPasses {
		cfg := &Config{AdminPass: pass}
		if w := cfg.AdminPassWarnings(); len(w) != 0 {
			t.Errorf("强口令 ADMIN_PASS=%q 不应告警（低熵指标不适用于人类口令），got %v", pass, w)
		}
	}

	// 反证：同样长度但字符类别不足的口令必须仍告警
	weakByClass := []string{
		"abcdefgh12345678", // 16 位仅两类（小写+数字）
		"aaaaaaaaaaaa1234", // 16 位仅两类且重复
		"ABCDEFGH12345678", // 16 位仅两类（大写+数字）
		"Short1!",          // 长度不足
	}
	for _, pass := range weakByClass {
		cfg := &Config{AdminPass: pass}
		if w := cfg.AdminPassWarnings(); len(w) == 0 {
			t.Errorf("弱口令 ADMIN_PASS=%q 必须告警", pass)
		}
	}
}

// TestEnvExampleSecretsBlank 回归 H-1：.env.example 的 JWT_SECRET/ENCRYPT_KEY
// 必须留空，让 docker compose 的 ${VAR:?} 在启动前拦截，而不是留一个「长度
// 达标但公开」的占位值骗过 STRICT_SECRETS。
func TestEnvExampleSecretsBlank(t *testing.T) {
	data, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("读取 .env.example 失败: %v", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	for _, k := range []string{"JWT_SECRET", "ENCRYPT_KEY"} {
		v, ok := values[k]
		if !ok {
			t.Fatalf(".env.example 缺少 %s", k)
		}
		if v != "" {
			t.Errorf(".env.example 的 %s 必须留空（compose :? 守卫拦截），实际 %q", k, v)
		}
	}
	// 反向校验：若运维把示例里的占位值原样填进环境，WeakSecretWarnings 仍须告警。
	if w := (&Config{JWTSecret: "please_change_me_to_32_bytes_random_hex",
		EncryptKey: "please_change_me_to_32_bytes_random_hex",
		AdminPass:  "please_change_me_to_a_strong_password"}).WeakSecretWarnings(); len(w) == 0 {
		t.Error(".env.example 历史占位值必须仍被识别为弱密钥")
	}
}

// TestQueueConfigClamp 回归 M-2：QUEUE_CLAIM_TTL=0/负数会让
// time.NewTicker(ClaimTTL/3) 在 goroutine 内 panic 崩进程，加载期必须钳制。
func TestQueueConfigClamp(t *testing.T) {
	clearEnv(t)
	for _, tc := range []struct {
		ttl     string
		want    time.Duration
		workers string
		poll    string
		wantW   int
		wantP   int
	}{
		{"0", 30 * time.Second, "0", "0", 1, 50},
		{"-5", 30 * time.Second, "-3", "-1", 1, 50},
		{"1", 30 * time.Second, "4", "1000", 4, 1000},
		{"999999", 3600 * time.Second, "1000", "99999999", 64, 60000},
	} {
		t.Setenv("QUEUE_CLAIM_TTL", tc.ttl)
		t.Setenv("QUEUE_WORKERS", tc.workers)
		t.Setenv("QUEUE_POLL_MS", tc.poll)
		cfg := LoadFile(t.TempDir() + "/nope.yml")
		if cfg.QueueClaimTTL != tc.want {
			t.Errorf("QUEUE_CLAIM_TTL=%s → %v, want %v", tc.ttl, cfg.QueueClaimTTL, tc.want)
		}
		if cfg.QueueWorkers != tc.wantW {
			t.Errorf("QUEUE_WORKERS=%s → %d, want %d", tc.workers, cfg.QueueWorkers, tc.wantW)
		}
		if cfg.QueuePollMS != tc.wantP {
			t.Errorf("QUEUE_POLL_MS=%s → %d, want %d", tc.poll, cfg.QueuePollMS, tc.wantP)
		}
	}
	// 配置文件通道同样钳制（先清掉上一段设置的环境变量，确保走文件通道）
	clearEnv(t)
	dir := t.TempDir()
	yml := dir + "/config.yml"
	if err := os.WriteFile(yml, []byte("queue:\n  claim_ttl: 0\n  workers: 0\n  poll_ms: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadFile(yml)
	if cfg.QueueClaimTTL != 30*time.Second || cfg.QueueWorkers != 1 || cfg.QueuePollMS != 50 {
		t.Errorf("config.yml 非法队列参数未钳制: ttl=%v workers=%d poll=%d", cfg.QueueClaimTTL, cfg.QueueWorkers, cfg.QueuePollMS)
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
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "127.0.0.1" || cfg.TrustedProxies[1] != "::1" {
		t.Errorf("TrustedProxies default = %v, want [127.0.0.1 ::1]", cfg.TrustedProxies)
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
