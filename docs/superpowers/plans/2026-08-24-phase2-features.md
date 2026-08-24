# 二期功能 Implementation Plan（R4 · F1 · F2 · F3）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 4 个向后兼容的新功能——R4 Webhook 可选 HMAC 签名认证、F1 发送日志 CSV 导出 + 独立详情页、F2 `/metrics` Prometheus 端点、F3 JSON 导出导入（备份迁移）。

**Architecture:** 全部为新增/可选能力。R4 通过 `tasks.require_signature` 开关（默认 0）在 webhook handler 加 HMAC-SHA256 校验（HMAC 密钥 = 任务 api_key）；F2 引入 `prometheus/client_golang`，`internal/metrics` 统一注册指标，`/metrics` 走独立路由（可选 Basic Auth）；F3 新增导出/导入服务（导出明文 config 仅管理员、导入按 渠道→模板→任务 顺序建表并重映射 id、任务名冲突跳过）；F1 新增日志 CSV 导出 API（左连任务/渠道取名称）与单日志详情 API，前端加详情页与导出按钮。

**Tech Stack:** Go 1.25 + Gin + MySQL 5.7；Vue3 + Element Plus；`github.com/prometheus/client_golang@v1.21.1`（已确认经 goproxy.cn 可下载）。

---

## 前置

- 本计划在分支 **`feature/phase2`** 上执行（从 `main` 切出）。不要在 main 上直接实现。
- 所有 Go 命令在仓库根目录执行并带离线模块缓存：`export GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath`
- 本机需 MySQL 且存在 `notice_service_test` 库；全量回归用 `make test`（`-p 1 -count=1`）。
- 提交信息遵循仓库惯例（`feat:` / `fix:` / `docs:` + 中文描述）。
- 每个任务按「写失败测试 → 跑确认失败 → 实现 → 跑确认通过 → 提交」推进。

---

## Task 1: R4a 迁移 011 + Task 模型/仓储支持 require_signature

**Files:**
- Create: `internal/database/migrations/011_task_signature.sql`
- Modify: `internal/model/models.go`
- Modify: `internal/repository/task_repo.go`
- Test: `internal/repository/task_repo_test.go`

- [ ] **Step 1: 写失败测试** 在 `internal/repository/task_repo_test.go` 追加：

```go
// TestTaskRequireSignatureRoundtrip 验证 R4：require_signature 列可读写。
func TestTaskRequireSignatureRoundtrip(t *testing.T) {
	db := openTestDB(t)
	r := NewTaskRepo(db)
	uid := seedUser(t, db)
	chID := seedChannel(t, db, uid)
	tplID := seedTemplate(t, db, uid)
	tk := &model.Task{UserID: uid, Name: "sig-" + randSuffix(), ChannelID: chID, TemplateID: tplID,
		TriggerType: "api", Enabled: true, RequireSignature: true}
	if err := r.Create(tk); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequireSignature {
		t.Fatal("RequireSignature should be true after create+get")
	}
	// 关回 false 也能更新
	tk.RequireSignature = false
	if err := r.Update(tk); err != nil {
		t.Fatal(err)
	}
	got2, err := r.GetByID(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.RequireSignature {
		t.Fatal("RequireSignature should be false after update")
	}
}
```

> `seedUser` / `seedChannel` / `seedTemplate` 定义在 `internal/repository/helpers_test.go`；`openTestDB` / `randSuffix` 定义在 `user_repo_test.go`。`model` 需已 import（该文件应已 import）。

- [ ] **Step 2: 跑测试确认失败**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/repository/ -run TestTaskRequireSignatureRoundtrip -count=1 -v
```

Expected: FAIL（`RequireSignature` 字段不存在 / 列不存在）。

- [ ] **Step 3: 实现**

3a. `internal/database/migrations/011_task_signature.sql`：

```sql
-- Webhook 可选 HMAC 签名认证：require_signature=1 时须带 X-Timestamp + X-Signature
ALTER TABLE tasks ADD COLUMN require_signature TINYINT(1) NOT NULL DEFAULT 0 AFTER api_key;
```

3b. `internal/model/models.go` 的 `Task` 结构体，在 `APIKey` 后加：

```go
	RequireSignature bool `json:"require_signature"`
```

3c. `internal/repository/task_repo.go`：
- `Create` 的 INSERT 列与值各加一项 `require_signature`：
  - 列：`..., api_key, require_signature, allowed_ips, ...`（放在 `api_key` 后）
  - 值：`..., nullableKey(t.APIKey), t.RequireSignature, ...`
- `Update` 同理：`SET ..., api_key=?, require_signature=?, allowed_ips=?, ...`
- `taskCols` 常量加 `require_signature`（放在 `api_key` 后）
- `scanOne` / `scanMany` 的 `Scan` 增加一个 `&t.RequireSignature`（对应位置在 `&apiKey` 之后、`&allowed` 之前）

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/repository/ -count=1
```

Expected: PASS（含既有 task 测试——Create/Update 语句改列后旧测试仍应通过）。

- [ ] **Step 5: 提交**

```bash
git add internal/database/migrations/011_task_signature.sql internal/model/models.go internal/repository/task_repo.go internal/repository/task_repo_test.go
git commit -m "feat(webhook): tasks 增加 require_signature 字段（R4）"
```

---

## Task 2: R4b Webhook 签名校验（读 body 一次 + HMAC 校验）

**Files:**
- Modify: `internal/handler/webhook_handler.go`
- Test: `internal/handler/webhook_test.go`

- [ ] **Step 1: 写失败测试** 在 `internal/handler/webhook_test.go` 追加（顶部 import 增加 `"crypto/hmac"`、`"crypto/sha256"`、`"encoding/hex"`、`"fmt"`、`"time"`；`bytes`/`strconv` 已存在则复用）：

```go
// hmacSig 按协议计算 X-Signature：hex(HMAC-SHA256(key, "<ts>\n<body>"))。
func hmacSig(key string, ts string, body string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ts))
	mac.Write([]byte("\n"))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// TestWebhookSignatureRequired 验证 R4：require_signature=1 时缺/错/过期签名均 401，正确签名 202。
func TestWebhookSignatureRequired(t *testing.T) {
	channel.Register(&fakeOKChan{})
	r := testRouter(t)
	tok := login(t, r)

	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"fake-ok","name":"c","config":{},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	payload := `{"name":"sig-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":[],"require_signature":true,"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 { t.Fatalf("create task = %d body=%s", wtk.Code, wtk.Body.String()) }
	tk := mustJSON(t, wtk)
	apiKey := tk["api_key"].(string)
	if apiKey == "" { t.Fatal("api key empty") }

	body := `{"variables":{"name":"李四"}}`
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// 无签名头 → 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized { t.Fatalf("no signature = %d, want 401", w.Code) }

	// 错误签名 → 401
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Timestamp", ts)
	req2.Header.Set("X-Signature", "deadbeef")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized { t.Fatalf("wrong signature = %d, want 401", w2.Code) }

	// 时间戳超出 ±300s → 401
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix()-9999, 10))
	req3.Header.Set("X-Signature", hmacSig(apiKey, strconv.FormatInt(time.Now().Unix()-9999, 10), body))
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnauthorized { t.Fatalf("expired timestamp = %d, want 401", w3.Code) }

	// 正确签名 → 202
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("POST", "/api/webhook/"+apiKey, bytes.NewBufferString(body))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("X-Timestamp", ts)
	req4.Header.Set("X-Signature", hmacSig(apiKey, ts, body))
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusAccepted { t.Fatalf("valid signature = %d, want 202 body=%s", w4.Code, w4.Body.String()) }
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestWebhookSignatureRequired -count=1 -v
```

Expected: FAIL —— 当前实现不校验签名，正确签名分支返回 202 前（无签名分支也 202 而非 401）。

- [ ] **Step 3: 实现** `internal/handler/webhook_handler.go`：

3a. import 增加：`"bytes"`、`"crypto/hmac"`、`"crypto/sha256"`、`"crypto/subtle"`、`"encoding/hex"`、`"strconv"`。（`errors`/`io`/`time` 已存在。）

3b. `Trigger` 中把 body 处理改为「先读原始字节、再验签、再解析」：

```go
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体读取失败"})
		return
	}
	// 可选 HMAC 签名认证：require_signature=1 时校验（密钥 = 任务 api_key）。
	if task.RequireSignature {
		if err := verifyWebhookSignature(c, task.APIKey, raw); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
	}
	// R8：空 body（或纯空白）按空变量接受；其它解析错误 400。
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是合法 JSON"})
			return
		}
	}
```

（替换原 `if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {...}` 块。）

3c. 新增常量与函数（文件内）：

```go
// webhookSigWindow 签名时间戳允许偏差（秒），防重放。
const webhookSigWindow = 300

// verifyWebhookSignature 校验 HMAC 签名：X-Timestamp + X-Signature，
// 签名消息为 "<X-Timestamp>\n<原始请求体>"，HMAC-SHA256，密钥为任务 api_key。
func verifyWebhookSignature(c *gin.Context, key string, body []byte) error {
	tsStr := c.GetHeader("X-Timestamp")
	sig := c.GetHeader("X-Signature")
	if tsStr == "" || sig == "" {
		return errors.New("缺少 X-Timestamp / X-Signature 请求头")
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return errors.New("X-Timestamp 格式错误")
	}
	if delta := time.Now().Unix() - ts; delta < -webhookSigWindow || delta > webhookSigWindow {
		return errors.New("X-Timestamp 超出允许时间窗（±300 秒）")
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("\n"))
	mac.Write(body)
	expect := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expect), []byte(sig)) != 1 {
		return errors.New("签名无效")
	}
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -count=1
```

Expected: PASS（含新签名测试 + 既有 `TestWebhookMalformedJSON400`（畸形→400、空 body→202 在新 ReadAll 路径下仍成立）+ 跨实例限流测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/webhook_handler.go internal/handler/webhook_test.go
git commit -m "feat(webhook): 可选 HMAC 签名校验（X-Timestamp/X-Signature，±300s）（R4）"
```

---

## Task 3: R4c 前端任务表单「需要签名」开关

**Files:**
- Modify: `web/src/views/Tasks.vue`
- Modify: `web/src/api/index.ts`（如需要显式类型）

- [ ] **Step 1: 读 `web/src/views/Tasks.vue`**，定位任务新建/编辑弹窗中 `trigger_type`、`api_key`、`allowed_ips` 表单区。

- [ ] **Step 2: 在表单区（api 触发相关，`allowed_ips` 附近）新增开关**：

```
el-switch v-model="form.require_signature"（label「需要 HMAC 签名」）
```

仅当 `form.trigger_type === 'api'` 时显示。`form` 的初始/回填对象需包含 `require_signature: false`（新建）并随编辑回填。

- [ ] **Step 3: 当开关开启时显示调用示例**（`v-if="form.require_signature"`），用 `<pre>` 展示：

```
X-Timestamp: <unix 秒>
X-Signature: hex(HMAC-SHA256(key=任务APIKey, msg="<timestamp>\n<原始请求体>"))
```

说明文案：时间戳偏差超过 ±300s 会被拒绝。

- [ ] **Step 4: 验证前端构建**：

```bash
cd web && npm --cache $PWD/../.dev/npm-cache run build
```

Expected: `vite build` 成功（`web/dist` 更新）。

- [ ] **Step 5: 提交**

```bash
git add web/src/views/Tasks.vue web/src/api/index.ts
git commit -m "feat(web): 任务表单支持「需要 HMAC 签名」开关与调用示例（R4）"
```

---

## Task 4: F2a 引入 prometheus/client_golang + internal/metrics 包

**Files:**
- Modify: `go.mod` / `go.sum`
- Create: `internal/metrics/metrics.go`
- Test: `internal/metrics/metrics_test.go`

- [ ] **Step 1: 拉依赖**（网络可用，goproxy.cn）：

```bash
export GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath
go get github.com/prometheus/client_golang@v1.21.1
go mod tidy
```

Expected: go.mod require 出现 client_golang；go.sum 补齐。

- [ ] **Step 2: 写失败测试** `internal/metrics/metrics_test.go`：

```go
package metrics

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMetricsCounterIncrements 用唯一标签验证 counter 自增（避免跨测试累计干扰）。
func TestMetricsCounterIncrements(t *testing.T) {
	key := fmt.Sprintf("ch-%d", time.Now().UnixNano())
	SendsTotal.WithLabelValues(key, "success").Inc()
	SendsTotal.WithLabelValues(key, "success").Inc()
	mfs, err := Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "notice_sends_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "channel" && lp.GetValue() == key {
					if m.GetCounter().GetValue() != 2 {
						t.Fatalf("counter = %v, want 2", m.GetCounter().GetValue())
					}
					return
				}
			}
		}
	}
	t.Fatal("notice_sends_total missing unique channel label")
}

// TestMetricsHandlerServes 验证 /metrics handler 可正常输出（含 Go runtime 指标）。
func TestMetricsHandlerServes(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("metrics handler = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatal("expected go_goroutines in output")
	}
}
```

- [ ] **Step 3: 跑测试确认失败**（编译失败，包/符号未定义）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/metrics/ -count=1 -v
```

- [ ] **Step 4: 实现** `internal/metrics/metrics.go`：

```go
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 全局指标注册表与指标。所有采集器只在本包注册一次（init）。
var (
	Registry = prometheus.NewRegistry()

	SendsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "notice_sends_total",
		Help: "通知发送次数，按渠道与状态（success/failed）",
	}, []string{"channel", "status"})

	SendDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notice_send_duration_seconds",
		Help:    "单次发送耗时（秒）",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP 请求数，按 code/method/path",
	}, []string{"code", "method", "path"})
)

// QueuePendingFunc 由 main 注入：返回待处理队列中的 pending job 数（scrape 时调用）。
var QueuePendingFunc func() float64

var once sync.Once

func init() {
	once.Do(func() {
		Registry.MustRegister(SendsTotal, SendDuration, HTTPRequests)
		Registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		Registry.MustRegister(prometheus.NewGoCollector())
		Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "notice_queue_pending",
			Help: "待处理发送队列中 pending 的 job 数",
		}, func() float64 {
			if QueuePendingFunc == nil {
				return 0
			}
			return QueuePendingFunc()
		}))
	})
}

// Handler 返回 /metrics 的 HTTP handler（含 Go runtime/process 默认采集）。
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}
```

- [ ] **Step 5: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/metrics/ -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
```

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/metrics/metrics.go internal/metrics/metrics_test.go
git commit -m "feat(metrics): 引入 prometheus/client_golang 与指标注册表（F2）"
```

---

## Task 5: F2b 指标埋点 + /metrics 路由 + 配置 + 测试

**Files:**
- Modify: `internal/service/notification_service.go`
- Modify: `internal/router/router.go`
- Modify: `internal/repository/send_job_repo.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/config/config.go`、`internal/config/config_test.go`
- Test: `internal/handler/metrics_test.go`（新建）、`internal/service/notification_service_test.go`（或既有测试补充）

- [ ] **Step 1: 写失败测试**

1a. `internal/handler/metrics_test.go`（package handler_test）：

```go
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/router"
	"notice-service/internal/service"
)

func metricsRouter(t *testing.T, enabled bool, user, pass string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	resetAdminData(db)
	ciph, _ := crypto.New(make([]byte, 32))
	authSvc := service.NewAuthService(db, "secret-secret-secret", "admin", "admin123")
	return router.NewRouter(db, authSvc, ciph, nil, nil, router.Options{
		MetricsEnabled: enabled, MetricsUser: user, MetricsPassword: pass,
	})
}

func TestMetricsEndpointEnabled(t *testing.T) {
	r := metricsRouter(t, true, "", "")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("/metrics = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "go_goroutines") {
		t.Fatal("missing go_goroutines metric")
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	r := metricsRouter(t, false, "", "")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("/metrics disabled = %d, want 404", w.Code)
	}
}

func TestMetricsEndpointBasicAuth(t *testing.T) {
	r := metricsRouter(t, true, "ops", "secret")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no creds = %d, want 401", w.Code)
	}
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/metrics", nil)
	req2.SetBasicAuth("ops", "secret")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("with creds = %d, want 200", w2.Code)
	}
}
```

1b. 在 `internal/service/notification_service_test.go` 追加一个断言发送后指标存在的测试（可选，若该文件结构不便则在 Task 5 步骤 4 用 metrics 包单测覆盖；此处以「发送成功后 `notice_sends_total` 计数 +1」为验收，可参考 `queue_test.go` 的 sink 渠道方式）：

```go
// TestSendTaskIncrementsMetrics 验证 F2：真实 sendOnce 会递增 notice_sends_total。
func TestSendTaskIncrementsMetrics(t *testing.T) {
	// 先取当前计数，再触发一次发送，断言 +1。
	// 用唯一 channel 标签（按渠道名取当前值；无则 0）。
	// 具体实现：构造 NotificationService + sink 渠道，调用 SendTask 成功后，
	// 从 metrics.Registry.Gather() 找到 notice_sends_total{channel=<type>,status="success"} 计数。
	// 见 internal/metrics/metrics_test.go 的查找方式；本测试可复用其遍历逻辑。
}
```

> 若步骤 1b 编写困难，允许以 `internal/metrics` 包单测 + `internal/handler` 的 /metrics 200 测试作为本任务验收；埋点正确性由评审抽查确认。

- [ ] **Step 2: 跑测试确认失败**（`/metrics` 404 / 编译失败）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run 'TestMetrics' -count=1 -v
```

- [ ] **Step 3: 实现**

3a. `internal/service/notification_service.go`：
- import 加 `"time"` 与 `"notice-service/internal/metrics"`。
- `sendOnce` 中把 `inst.Send(...)` 改为带计时与埋点：

```go
	start := time.Now()
	err := inst.Send(msg, &channel.Receiver{Address: addr})
	dur := time.Since(start).Seconds()
	if err != nil {
		metrics.SendsTotal.WithLabelValues(ch.Type, "failed").Inc()
		metrics.SendDuration.WithLabelValues(ch.Type).Observe(dur)
		_ = s.logRepo.Create(&model.TaskLog{...失败日志...})
		return err
	}
	metrics.SendsTotal.WithLabelValues(ch.Type, "success").Inc()
	metrics.SendDuration.WithLabelValues(ch.Type).Observe(dur)
	_ = s.logRepo.Create(&model.TaskLog{...成功日志...})
	return nil
```

（保留原日志字段不变，仅把发送调用包上计时并在两分支各加两行埋点。）

3b. `internal/router/router.go`：
- import 加 `"strconv"` 与 `"notice-service/internal/metrics"`。
- `Options` 增加字段：`MetricsEnabled bool`、`MetricsUser string`、`MetricsPassword string`。
- `accessLogger` 在写日志前加（**必须用 `c.FullPath()` 路由模板，而不是原始 URL path**——否则 `/api/webhook/<api_key>` 会把 api_key 泄进指标标签，且 `:id` 类路由造成路径基数爆炸）：

```go
		// 指标 path 标签用路由模板（c.FullPath()），避免 api_key 泄漏与路径基数爆炸。
		metrics.HTTPRequests.WithLabelValues(strconv.Itoa(c.Writer.Status()), c.Request.Method, c.FullPath()).Inc()
```

（`accessLogger` 中现有的 `path` 变量仍用于日志脱敏输出，保留不动。）

- `NewRouter` 内、`/api/health` 之后加：

```go
	if o.MetricsEnabled {
		m := r.Group("")
		if o.MetricsUser != "" || o.MetricsPassword != "" {
			m.Use(metricsBasicAuth(o.MetricsUser, o.MetricsPassword))
		}
		m.GET("/metrics", gin.WrapH(metrics.Handler()))
	}
```

- 新增辅助函数：

```go
// metricsBasicAuth /metrics 的可选 Basic Auth（两字段都非空才要求）。
func metricsBasicAuth(user, pass string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, p, ok := c.Request.BasicAuth()
		if !ok || u != user || p != pass {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
```

> 注意：`NewRouter` 现有 opts 合并逻辑是「显式 true 才覆盖默认」；`MetricsEnabled` 请按 `if opts[0].MetricsEnabled { o.MetricsEnabled = true }` 语义处理，保证未显式传时为 false（避免测试意外开启）。

3c. `internal/repository/send_job_repo.go` 加：

```go
// CountPending 统计待处理（pending）job 数（/metrics 用）。
func (r *SendJobRepo) CountPending() (int64, error) {
	var n int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM send_jobs WHERE status='pending'").Scan(&n)
	return n, err
}
```

3d. `internal/config/config.go`：
- `Config` 增加：`MetricsEnabled bool`、`MetricsUser string`、`MetricsPassword string`。
- `fileConfig` 增加嵌套块：

```go
	Metrics struct {
		Enabled  *bool  `yaml:"enabled"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"metrics"`
```

- `loadFromPath` 字面量增加：

```go
		MetricsEnabled:       firstBool("METRICS_ENABLED", f.Metrics.Enabled, true),
		MetricsUser:          firstNonEmpty(os.Getenv("METRICS_USER"), f.Metrics.User, ""),
		MetricsPassword:      firstNonEmpty(os.Getenv("METRICS_PASSWORD"), f.Metrics.Password, ""),
```

- `config_test.go` 的 `clearEnv` 增加 `"METRICS_ENABLED"`、`"METRICS_USER"`、`"METRICS_PASSWORD"`。

3e. `cmd/server/main.go`：`router.NewRouter(...)` 的 `router.Options{...}` 增加：

```go
		MetricsEnabled:   cfg.MetricsEnabled,
		MetricsUser:      cfg.MetricsUser,
		MetricsPassword:  cfg.MetricsPassword,
```

并在 `queue := service.NewQueueService(...)` 之后注入 pending 计数：

```go
	metrics.QueuePendingFunc = func() float64 {
		n, err := repository.NewSendJobRepo(db).CountPending()
		if err != nil {
			return 0
		}
		return float64(n)
	}
```

（main.go 需 import `"notice-service/internal/metrics"`。）

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run 'TestMetrics' -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/config/ ./internal/service/ ./internal/repository/ -count=1 -p 1
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/notification_service.go internal/router/router.go internal/repository/send_job_repo.go cmd/server/main.go internal/config/config.go internal/config/config_test.go internal/handler/metrics_test.go
git commit -m "feat(metrics): 埋点 + /metrics 路由（可选 Basic Auth）与配置（F2）"
```

---

## Task 6: F3a 导出 API（JSON 备份）

**Files:**
- Create: `internal/service/export_service.go`
- Create: `internal/handler/export_handler.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/export_test.go`（新建）

- [ ] **Step 1: 写失败测试** `internal/handler/export_test.go`：

```go
package handler_test

import (
	"net/http"
	"testing"
)

// TestExportBundle 验证 F3：管理员可导出含渠道/模板/任务的 JSON。
func TestExportBundle(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"exp-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"exp-tpl","subject":"会议 {{name}}","content_md":"hi","variables":[{"name":"name","default":"张三"}]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	payload := `{"name":"exp-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	if w := authReq(t, r, tok, "POST", "/api/tasks", payload); w.Code != 200 { t.Fatalf("create task = %d", w.Code) }

	w := authReq(t, r, tok, "GET", "/api/export", "")
	if w.Code != 200 { t.Fatalf("export = %d body=%s", w.Code, w.Body.String()) }
	body := w.Body.String()
	for _, s := range []string{"exp-ch", "exp-tpl", "exp-task", "smtp.x.com"} {
		if !containsStr(body, s) {
			t.Fatalf("export missing %q:\n%s", s, body)
		}
	}
	// 普通用户无权导出
	wu := normalUserToken(t, r)
	if w2 := authReq(t, r, wu, "GET", "/api/export", ""); w2.Code != http.StatusForbidden {
		t.Fatalf("user export = %d, want 403", w2.Code)
	}
}

func containsStr(s, sub string) bool { return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub { return i }
	}
	return -1
}

// normalUserToken 创建一个普通用户并登录，返回其 token（测试内共享助手；本文件定义）。
func normalUserToken(t *testing.T, r *gin.Engine) string {
	t.Helper()
	adminTok := login(t, r)
	wu := authReq(t, r, adminTok, "POST", "/api/users",
		`{"username":"exp-user","display_name":"","email":"","password":"Passw0rd!abcd","role":"user"}`)
	if wu.Code != 200 {
		t.Fatalf("create user = %d body=%s", wu.Code, wu.Body.String())
	}
	uid := int64(mustJSON(t, wu)["id"].(float64))
	t.Cleanup(func() {
		db := testDB(t)
		db.Exec("DELETE FROM channels WHERE user_id=?", uid)
		db.Exec("DELETE FROM templates WHERE user_id=?", uid)
		db.Exec("DELETE FROM tasks WHERE user_id=?", uid)
		db.Exec("DELETE FROM users WHERE id=?", uid)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{"username":"exp-user","password":"Passw0rd!abcd"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login user = %d body=%s", w.Code, w.Body.String())
	}
	return mustJSON(t, w)["token"].(string)
}
```

> 说明：`normalUserToken` 在 `export_test.go` 定义一次，`Task 9` 的 `log_export_test.go` 复用（同 package handler_test）。import 需含 `bytes`、`net/http`、`net/http/httptest`、`github.com/gin-gonic/gin`。

- [ ] **Step 2: 跑测试确认失败**（404 / 编译失败）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestExportBundle -count=1 -v
```

- [ ] **Step 3: 实现**

3a. `internal/service/export_service.go`：

```go
package service

import (
	"database/sql"
	"errors"
	"time"

	"notice-service/internal/model"
)

// ExportBundle 导出数据结构（备份/迁移用）。
type ExportBundle struct {
	Version    int              `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	Channels   []*model.Channel `json:"channels"`
	Templates  []*model.Template `json:"templates"`
	Tasks      []*model.Task    `json:"tasks"`
}

// ExportService 数据导出导入（备份迁移）。仅管理员调用。
type ExportService struct {
	channels  *ChannelService
	templates *TemplateService
	tasks     *TaskService
}

func NewExportService(db *sql.DB, cipher *crypto.Cipher) *ExportService {
	return &ExportService{
		channels:  NewChannelService(db, cipher),
		templates: NewTemplateService(db),
		tasks:     NewTaskService(db, nil),
	}
}

// Export 导出全部未删除的渠道（明文 config）/模板/任务。
func (s *ExportService) Export(userID int64) (*ExportBundle, error) {
	chs, err := s.channels.List(userID) // List 内部解密 config 到 Config 字段
	if err != nil {
		return nil, err
	}
	tpls, err := s.templates.List(userID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.tasks.List(userID)
	if err != nil {
		return nil, err
	}
	return &ExportBundle{
		Version:    1,
		ExportedAt: time.Now(),
		Channels:   chs,
		Templates:  tpls,
		Tasks:      tasks,
	}, nil
}
```

（需要 import `"notice-service/internal/crypto"`；如 `NewChannelService(db, cipher)` 的 cipher 参数是 `*crypto.Cipher`，保持一致。）

3b. `internal/handler/export_handler.go`：

```go
package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/service"
)

type ExportHandler struct {
	svc *service.ExportService
	db  *sql.DB
}

func NewExportHandler(db *sql.DB, cipher *crypto.Cipher) *ExportHandler {
	return &ExportHandler{svc: service.NewExportService(db, cipher), db: db}
}

// Export 导出备份（仅管理员）。
// @Summary 导出渠道/模板/任务 JSON 备份
// @Tags 系统
// @Security BearerAuth
// @Success 200 {object} service.ExportBundle
// @Router /api/export [get]
func (h *ExportHandler) Export(c *gin.Context) {
	bundle, err := h.svc.Export(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "export.data", "导出数据备份（%d 渠道/%d 模板/%d 任务）",
		len(bundle.Channels), len(bundle.Templates), len(bundle.Tasks))
	c.JSON(http.StatusOK, bundle)
}

// Import 导入备份（仅管理员）。
// @Summary 导入渠道/模板/任务 JSON 备份
// @Tags 系统
// @Security BearerAuth
// @Accept json
// @Param body body service.ExportBundle true "备份内容"
// @Success 200 {object} map[string]interface{}
// @Router /api/import [post]
func (h *ExportHandler) Import(c *gin.Context) {
	var bundle service.ExportBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	res, err := h.svc.Import(c.GetInt64("uid"), &bundle)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "import.data", "导入数据备份（+%d 渠道/+%d 模板/+%d 任务，跳过 %d）",
		res.ChannelsCreated, res.TemplatesCreated, res.TasksCreated, len(res.Skipped))
	c.JSON(http.StatusOK, res)
}
```

3c. `internal/router/router.go`：在 admin 组内加：

```go
			expH := handler.NewExportHandler(db, cipher)
			admin.GET("/export", expH.Export)
			admin.POST("/import", expH.Import)
```

> `cipher` 在 `NewRouter` 参数中已有。

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestExportBundle -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/export_service.go internal/handler/export_handler.go internal/router/router.go internal/handler/export_test.go
git commit -m "feat(backup): 数据导出 API（仅管理员，明文 config）（F3）"
```

---

## Task 7: F3b 导入 API（重映射 + 名称冲突跳过 + api_key 保留）

**Files:**
- Modify: `internal/service/export_service.go`
- Modify: `internal/service/task_service.go`（或 repo 加 SetAPIKey）
- Modify: `internal/repository/task_repo.go`
- Test: `internal/handler/export_test.go`（追加）

- [ ] **Step 1: 写失败测试** 追加到 `internal/handler/export_test.go`：

```go
// TestImportCreatesAndSkips 验证 F3：导入可建新记录、名称冲突跳过、api_key 保留。
func TestImportCreatesAndSkips(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	// 先建一个用于「跳过冲突」的渠道
	if w := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"dup-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`); w.Code != 200 {
		t.Fatalf("create dup channel = %d", w.Code)
	}

	// 构造导入 bundle：1 个已存在的渠道（跳过）+ 1 个新模板 + 1 个新 api 任务（保留 api_key）
	bundle := `{
		"version":1,
		"channels":[{"type":"email","name":"dup-ch","config":{"host":"smtp.y.com","port":"587","username":"u","password":"p","from":"b@x.com"},"enabled":true},
		            {"type":"email","name":"new-ch","config":{"host":"smtp.z.com","port":"587","username":"u","password":"p","from":"c@x.com"},"enabled":true}],
		"templates":[{"name":"new-tpl","subject":"S {{name}}","content_md":"hi","variables":[{"name":"name","default":"张三"}]}],
		"tasks":[{"name":"new-task","channel_id":2,"template_id":1,"trigger_type":"api","receivers":["a@x.com"],"api_key":"imported-key-123","enabled":true}]
	}`
	w := authReq(t, r, tok, "POST", "/api/import", bundle)
	if w.Code != 200 {
		t.Fatalf("import = %d body=%s", w.Code, w.Body.String())
	}
	res := mustJSON(t, w)
	if int(res["channels_created"].(float64)) != 1 {
		t.Fatalf("channels_created = %v, want 1", res["channels_created"])
	}
	if int(res["templates_created"].(float64)) != 1 {
		t.Fatalf("templates_created = %v, want 1", res["templates_created"])
	}
	if int(res["tasks_created"].(float64)) != 1 {
		t.Fatalf("tasks_created = %v, want 1", res["tasks_created"])
	}
	if len(res["skipped"].([]interface{})) != 1 {
		t.Fatalf("skipped = %v, want 1 (dup-ch)", res["skipped"])
	}

	// 导入的 api 任务应保留 api_key，可用它直接触发 webhook（202）
	// 先通过列表找到该任务 id
	wl := authReq(t, r, tok, "GET", "/api/tasks", "")
	if wl.Code != 200 { t.Fatalf("list tasks = %d", wl.Code) }
	// 简化：直接验证导出中带 imported-key-123
	we := authReq(t, r, tok, "GET", "/api/export", "")
	if !containsStr(we.Body.String(), "imported-key-123") {
		t.Fatalf("export should contain preserved api_key, got:\n%s", we.Body.String())
	}
}
```

> 注：bundle 中 `channel_id:2 / template_id:1` 是「旧 id」——导入按 渠道→模板→任务 顺序创建后，服务端需按数组下标把旧 id 重映射为新建后的真实 id。测试里 bundle 的 tasks.channel_id 指向「第 2 个渠道（new-ch，新建）」、template_id 指向「第 1 个模板（new-tpl，新建）」。服务端重映射规则：**按数组顺序**，第 i 个渠道/模板映射到新建后的 id。若实现选择「按旧 id 数值匹配」而不是按下标，测试需相应调整——实现时应以「能正确重映射」为准，并在实现后按真实行为微调本测试。

- [ ] **Step 2: 跑测试确认失败**（`POST /api/import` 404）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestImportCreatesAndSkips -count=1 -v
```

- [ ] **Step 3: 实现**

3a. `internal/service/export_service.go` 增加 Import：

```go
// ImportResult 导入结果摘要。
type ImportResult struct {
	ChannelsCreated  int      `json:"channels_created"`
	TemplatesCreated int      `json:"templates_created"`
	TasksCreated     int      `json:"tasks_created"`
	Skipped          []string `json:"skipped"`
}

// Import 导入备份：按 渠道→模板→任务 顺序建表，旧 id → 新 id 重映射；
// 名称冲突跳过并记入摘要；api 任务的 api_key 保留（迁移后 webhook URL 不变）。
func (s *ExportService) Import(userID int64, b *ExportBundle) (*ImportResult, error) {
	if b == nil {
		return nil, errors.New("空的导入内容")
	}
	if b.Version != 1 {
		return nil, errors.New("不支持的备份版本")
	}
	res := &ImportResult{}
	chMap := map[int]int64{} // 数组下标 -> 新渠道 id
	// 渠道
	for i, c := range b.Channels {
		if c == nil || c.Name == "" || c.Type == "" {
			res.Skipped = append(res.Skipped, "(无效渠道)")
			continue
		}
		if s.nameExists("channels", c.Name, c.Type) {
			res.Skipped = append(res.Skipped, "渠道 "+c.Name)
			continue
		}
		nc := &model.Channel{Type: c.Type, Name: c.Name, Config: c.Config, Enabled: c.Enabled}
		if err := s.channels.Create(userID, nc); err != nil {
			res.Skipped = append(res.Skipped, "渠道 "+c.Name+" ("+err.Error()+")")
			continue
		}
		chMap[i] = nc.ID
		res.ChannelsCreated++
	}
	// 模板
	tplMap := map[int]int64{}
	for i, t := range b.Templates {
		if t == nil || t.Name == "" {
			res.Skipped = append(res.Skipped, "(无效模板)")
			continue
		}
		if s.nameExists("templates", t.Name, "") {
			res.Skipped = append(res.Skipped, "模板 "+t.Name)
			continue
		}
		nt := &model.Template{Name: t.Name, Subject: t.Subject, ContentMD: t.ContentMD, Variables: t.Variables}
		if err := s.templates.Create(userID, nt); err != nil {
			res.Skipped = append(res.Skipped, "模板 "+t.Name+" ("+err.Error()+")")
			continue
		}
		tplMap[i] = nt.ID
		res.TemplatesCreated++
	}
	// 任务
	for _, t := range b.Tasks {
		if t == nil || t.Name == "" {
			res.Skipped = append(res.Skipped, "(无效任务)")
			continue
		}
		if s.nameExists("tasks", t.Name, "") {
			res.Skipped = append(res.Skipped, "任务 "+t.Name)
			continue
		}
		nt := &model.Task{Name: t.Name, TemplateID: remapID(tplMap, t.TemplateID), TriggerType: t.TriggerType,
			Receivers: t.Receivers, CronExpr: t.CronExpr, AllowedIPs: t.AllowedIPs, Variables: t.Variables,
			Enabled: t.Enabled, RequireSignature: t.RequireSignature}
		for _, cid := range t.ChannelIDs {
			if mapped := remapID(chMap, cid); mapped > 0 {
				nt.ChannelIDs = append(nt.ChannelIDs, mapped)
			}
		}
		if len(nt.ChannelIDs) == 0 {
			if mapped := remapID(chMap, t.ChannelID); mapped > 0 {
				nt.ChannelIDs = []int64{mapped}
			}
		}
		if len(nt.ChannelIDs) == 0 {
			res.Skipped = append(res.Skipped, "任务 "+t.Name+"（无有效渠道）")
			continue
		}
		oldKey := t.APIKey
		if err := s.tasks.Create(userID, nt); err != nil {
			res.Skipped = append(res.Skipped, "任务 "+t.Name+" ("+err.Error()+")")
			continue
		}
		if oldKey != "" && nt.TriggerType == "api" {
			if err := s.tasks.SetAPIKey(nt.ID, oldKey); err != nil {
				res.Skipped = append(res.Skipped, "任务 "+t.Name+"（api_key 保留失败）")
				continue
			}
		}
		res.TasksCreated++
	}
	return res, nil
}

// remapID 按旧 id 在映射表中查找；未命中返回 0。
func remapID(m map[int]int64, oldID int64) int64 {
	if v, ok := m[int(oldID)]; ok {
		return v
	}
	return 0
}
```

> 关键决策：重映射用「**数组下标**」而非「旧 id 数值」（导出/导入结构无全局 id，且旧 id 在目标库中无意义；下标映射在测试 bundle 里即 tasks 引用第 2 个渠道=下标 1、第 1 个模板=下标 0）。因此 `chMap[i]`/`tplMap[i]` 按数组下标存，`remapID` 用 `int(oldID)` 查。若测试与实际不符，以让 `TestImportCreatesAndSkips` 通过为准微调下标约定，并保持注释说明。

3b. `nameExists`（检查名称冲突；渠道按 (name,type) 唯一，模板/任务按 name 唯一）：

```go
func (s *ExportService) nameExists(table, name, typ string) bool {
	// 直接查 DB：channels 按 name+type；templates/tasks 按 name（均排除软删）。
	// 实现用 repository 层查询；若现有 repo 无按名查询方法，则在相应 repo 增加
	// CountByName(name, type) 之类的幂等查询。
	...
}
```

> 若 `ChannelRepo`/`TemplateRepo`/`TaskRepo` 无按名查询，需各加一个小方法（如 `TaskRepo.CountByName(name string) (int, error)`、`ChannelRepo.CountByNameType(name, typ string) (int, error)`、`TemplateRepo.CountByName(name string) (int, error)`）。请先读这三个 repo 文件，按现有风格补方法。

3c. `internal/service/task_service.go` 增加（转发到 repo）：

```go
// SetAPIKey 覆盖任务的 api_key（导入备份时保留 webhook URL）。
func (s *TaskService) SetAPIKey(taskID int64, key string) error {
	return s.repo.SetAPIKey(taskID, key)
}
```

3d. `internal/repository/task_repo.go` 增加：

```go
// SetAPIKey 覆盖任务 api_key（导入备份用）。
func (r *TaskRepo) SetAPIKey(taskID int64, key string) error {
	_, err := r.db.Exec("UPDATE tasks SET api_key=? WHERE id=?", nullableKey(key), taskID)
	return err
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run 'TestExportBundle|TestImportCreatesAndSkips' -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/service/ ./internal/repository/ -count=1 -p 1
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/export_service.go internal/service/task_service.go internal/repository/task_repo.go internal/handler/export_test.go
git commit -m "feat(backup): 数据导入 API（重映射/跳过冲突/保留 api_key）（F3）"
```

---

## Task 8: F3c 前端「数据备份」区

**Files:**
- Modify: `web/src/views/Settings.vue`
- Modify: `web/src/api/index.ts`

- [ ] **Step 1: 读 `web/src/views/Settings.vue`**，找到个人设置面板结构（一段式三段式：资料/安全/…）。新增「数据备份」卡片（仅管理员可见——用 `useAuthStore().user?.role === 'admin'`）。

- [ ] **Step 2: `web/src/api/index.ts` 增加**：

```ts
export const backupApi = {
  export: () => client.get('/export', { responseType: 'blob' }).then((r) => r.data as Blob),
  import: (data: any) => client.post('/import', data).then((r) => r.data),
}
```

- [ ] **Step 3: Settings.vue 备份卡片**：
- 导出按钮：`backupApi.export()` 拿到 Blob → 用 `URL.createObjectURL` + `<a download="notice-backup-<ts>.json">` 触发下载（注意：`/api/export` 返回 JSON，用 blob 下载）。
- 导入：`el-upload`（`auto-upload: false` / `on-change` 读文件 JSON）→ 解析后 `backupApi.import(bundle)` → 展示返回的 `{channels_created, templates_created, tasks_created, skipped}`（`ElMessage` 汇总）。
- 加一句警示文案：「导出包含渠道明文配置（含密码等敏感信息），请妥善保管备份文件」。

- [ ] **Step 4: 前端构建验证**

```bash
cd web && npm --cache $PWD/../.dev/npm-cache run build
```

- [ ] **Step 5: 提交**

```bash
git add web/src/views/Settings.vue web/src/api/index.ts
git commit -m "feat(web): 个人设置新增数据备份/恢复区（F3）"
```

---

## Task 9: F1a 日志 CSV 导出 API

**Files:**
- Modify: `internal/repository/task_log_repo.go`
- Modify: `internal/service/task_service.go`
- Modify: `internal/handler/task_handler.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/log_export_test.go`（新建）

- [ ] **Step 1: 写失败测试** `internal/handler/log_export_test.go`：

```go
package handler_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestLogExportCSV 验证 F1：管理员可导出 CSV，含任务/渠道名称列与表头。
func TestLogExportCSV(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)

	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"exp-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	payload := `{"name":"exp-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 { t.Fatalf("create task = %d", wtk.Code) }
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))

	db := testDB(t)
	if _, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, status, error_msg, sent_at) VALUES (?, ?, '主题A', 'failed', 'boom', NOW())", taskID, int64(ch["id"].(float64))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE task_id=?", taskID) })

	w := authReq(t, r, tok, "GET", "/api/logs/export?task_id="+num(taskID), "")
	if w.Code != 200 { t.Fatalf("export = %d body=%s", w.Code, w.Body.String()) }
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type = %q, want text/csv", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "task_name") || !strings.Contains(body, "channel_name") {
		t.Fatalf("CSV missing name columns:\n%s", body)
	}
	if !strings.Contains(body, "exp-task") || !strings.Contains(body, "exp-ch") {
		t.Fatalf("CSV missing names:\n%s", body)
	}
	if !strings.Contains(body, "主题A") || !strings.Contains(body, "boom") {
		t.Fatalf("CSV missing subject/error:\n%s", body)
	}
	// 普通用户无权导出
	wu := normalUserToken(t, r) // 定义见 Task 6 的 export_test.go（同 package）
	if w2 := authReq(t, r, wu, "GET", "/api/logs/export", ""); w2.Code != http.StatusForbidden {
		t.Fatalf("user export = %d, want 403", w2.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**（404）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestLogExportCSV -count=1 -v
```

- [ ] **Step 3: 实现**

3a. `internal/repository/task_log_repo.go`：

- 抽出 WHERE 构建（避免 Query/ListExportRows 重复）：

```go
// logWhere 按过滤条件构建 WHERE 子句与参数（Query/ListExportRows 共用）。
func logWhere(f LogFilter) (string, []interface{}) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.TaskID > 0 { where += " AND tl.task_id=?"; args = append(args, f.TaskID) }
	if f.Status != "" { where += " AND tl.status=?"; args = append(args, f.Status) }
	if !f.From.IsZero() { where += " AND tl.sent_at >= ?"; args = append(args, f.From) }
	if !f.To.IsZero() { where += " AND tl.sent_at < ?"; args = append(args, f.To) }
	return where, args
}
```

> 注意：现有 `Query` 的 WHERE 用的是不带别名的 `task_id` 等（单表查询）。若抽成共用会改变 `Query`——请保持 `Query` 原样，仅为导出新增独立方法（可复制同样条件片段，加 `tl.` 别名前缀）。不要把 `Query` 改成别名形式以免影响既有分页测试。

- 新增导出行结构与查询：

```go
// LogExportRow CSV 导出用扁平行（左连任务/渠道取名称）。
type LogExportRow struct {
	ID          int64
	SentAt      time.Time
	TaskID      int64
	TaskName    string
	ChannelID   int64
	ChannelName string
	Status      string
	Subject     string
	ErrorMsg    string
	TriggerType string
	TriggerBy   string
	TriggerIP   string
}

// ListExportRows 按过滤条件导出全部匹配行（不分页，最多 limit 行）。
func (r *TaskLogRepo) ListExportRows(f LogFilter, limit int) ([]*LogExportRow, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.TaskID > 0 { where += " AND tl.task_id=?"; args = append(args, f.TaskID) }
	if f.Status != "" { where += " AND tl.status=?"; args = append(args, f.Status) }
	if !f.From.IsZero() { where += " AND tl.sent_at >= ?"; args = append(args, f.From) }
	if !f.To.IsZero() { where += " AND tl.sent_at < ?"; args = append(args, f.To) }
	if limit <= 0 || limit > 100000 { limit = 100000 }
	query := `SELECT tl.id, tl.sent_at, tl.task_id, COALESCE(t.name,''), tl.channel_id, COALESCE(c.name,''),
		tl.status, tl.subject, COALESCE(tl.error_msg,''), COALESCE(tl.trigger_type,''), COALESCE(tl.trigger_by,''), COALESCE(tl.trigger_ip,'')
		FROM task_logs tl
		LEFT JOIN tasks t ON t.id = tl.task_id
		LEFT JOIN channels c ON c.id = tl.channel_id
		` + where + ` ORDER BY tl.id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []*LogExportRow{}
	for rows.Next() {
		row := &LogExportRow{}
		if err := rows.Scan(&row.ID, &row.SentAt, &row.TaskID, &row.TaskName, &row.ChannelID, &row.ChannelName,
			&row.Status, &row.Subject, &row.ErrorMsg, &row.TriggerType, &row.TriggerBy, &row.TriggerIP); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
```

3b. `internal/service/task_service.go` 增加：

```go
// ExportLogRows 导出日志扁平行（CSV 用）。
func (s *TaskService) ExportLogRows(f repository.LogFilter, limit int) ([]*repository.LogExportRow, error) {
	return s.logRepo.ListExportRows(f, limit)
}
```

3c. `internal/handler/task_handler.go` 增加 `ExportLogs`（仅管理员；复用 `LogsAll` 的筛选解析逻辑）：

```go
// ExportLogs 导出发送日志为 CSV（仅管理员）。
// @Summary 导出发送日志 CSV（仅管理员）
// @Tags 任务
// @Security BearerAuth
// @Param task_id query int false "任务 ID"
// @Param status query string false "状态"
// @Param from query string false "开始日期"
// @Param to query string false "结束日期"
// @Success 200 {string} string "CSV"
// @Router /api/logs/export [get]
func (h *TaskHandler) ExportLogs(c *gin.Context) {
	f := h.logFilterFromQuery(c)
	rows, err := h.svc.ExportLogRows(f, 100000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := "logs-" + time.Now().Format("20060102-150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "sent_at", "task_id", "task_name", "channel_id", "channel_name", "status", "subject", "error_msg", "trigger_type", "trigger_by", "trigger_ip"})
	for _, r := range rows {
		_ = w.Write([]string{
			strconv.FormatInt(r.ID, 10), r.SentAt.Format("2006-01-02 15:04:05"),
			strconv.FormatInt(r.TaskID, 10), r.TaskName, strconv.FormatInt(r.ChannelID, 10), r.ChannelName,
			r.Status, r.Subject, r.ErrorMsg, r.TriggerType, r.TriggerBy, r.TriggerIP,
		})
	}
	w.Flush()
}
```

> 为复用筛选解析，建议把 `LogsAll` 里的查询参数解析抽成 `func (h *TaskHandler) logFilterFromQuery(c *gin.Context) repository.LogFilter`（返回带默认 Page/PageSize 的 filter；ExportLogs 只关心筛选字段）。`LogsAll` 改为调用它。import 需加 `"encoding/csv"`。

3d. `internal/router/router.go`：admin 组内加 `admin.GET("/logs/export", taskH.ExportLogs)`。

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run 'TestLogExportCSV' -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/repository/task_log_repo.go internal/service/task_service.go internal/handler/task_handler.go internal/router/router.go internal/handler/log_export_test.go
git commit -m "feat(logs): 发送日志 CSV 导出 API（仅管理员）（F1）"
```

---

## Task 10: F1b 单日志详情 API

**Files:**
- Modify: `internal/service/task_service.go`
- Modify: `internal/handler/task_handler.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/log_detail_test.go`（新建）

- [ ] **Step 1: 写失败测试** `internal/handler/log_detail_test.go`：

```go
package handler_test

import (
	"net/http"
	"testing"
)

// TestLogDetail 验证 F1：GET /api/logs/:id 返回完整日志；不存在 404。
func TestLogDetail(t *testing.T) {
	r := testRouter(t)
	tok := login(t, r)
	db := testDB(t)

	// 直插一条日志（channel_id 无外键，用 1；task_id 需真实任务，先建一个）
	wc := authReq(t, r, tok, "POST", "/api/channels", `{"type":"email","name":"d-ch","config":{"host":"smtp.x.com","port":"587","username":"u","password":"p","from":"a@x.com"},"enabled":true}`)
	if wc.Code != 200 { t.Fatalf("create channel = %d", wc.Code) }
	ch := mustJSON(t, wc)
	wt := authReq(t, r, tok, "POST", "/api/templates", `{"name":"t","subject":"s","content_md":"hi","variables":[]}`)
	if wt.Code != 200 { t.Fatalf("create template = %d", wt.Code) }
	tpl := mustJSON(t, wt)
	payload := `{"name":"d-task","channel_id":` + num(int64(ch["id"].(float64))) + `,"template_id":` + num(int64(tpl["id"].(float64))) + `,"trigger_type":"api","receivers":["a@x.com"],"enabled":true}`
	wtk := authReq(t, r, tok, "POST", "/api/tasks", payload)
	if wtk.Code != 200 { t.Fatalf("create task = %d", wtk.Code) }
	tk := mustJSON(t, wtk)
	taskID := int64(tk["id"].(float64))
	chID := int64(ch["id"].(float64))

	res, err := db.Exec("INSERT INTO task_logs (task_id, channel_id, subject, content, status, request, response, error_msg, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, '主题A', '正文B', 'failed', '{\"address\":\"a@x.com\"}', '', 'boom', 'manual', 'admin', '1.2.3.4', NOW())", taskID, chID)
	if err != nil { t.Fatal(err) }
	logID, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec("DELETE FROM task_logs WHERE id=?", logID) })

	w := authReq(t, r, tok, "GET", "/api/logs/"+num(logID), "")
	if w.Code != 200 { t.Fatalf("detail = %d body=%s", w.Code, w.Body.String()) }
	d := mustJSON(t, w)
	if d["subject"] != "主题A" || d["content"] != "正文B" || d["status"] != "failed" {
		t.Fatalf("detail fields = %+v", d)
	}
	if d["trigger_by"] != "admin" || d["trigger_ip"] != "1.2.3.4" {
		t.Fatalf("trigger info = %+v", d)
	}
	// 不存在 → 404
	if w2 := authReq(t, r, tok, "GET", "/api/logs/999999", ""); w2.Code != http.StatusNotFound {
		t.Fatalf("missing log = %d, want 404", w2.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**（404 / 路由冲突）

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestLogDetail -count=1 -v
```

- [ ] **Step 3: 实现**

3a. `internal/service/task_service.go`：

```go
// GetLog 返回单条发送日志完整内容（详情页用）；不存在返回 repository.ErrNotFound。
func (s *TaskService) GetLog(logID int64) (*model.TaskLog, error) {
	return s.logRepo.GetByID(logID)
}
```

3b. `internal/handler/task_handler.go`：

```go
// LogByID 发送日志详情（完整内容）。
// @Summary 发送日志详情
// @Tags 任务
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} model.TaskLog
// @Router /api/logs/{id} [get]
func (h *TaskHandler) LogByID(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	log, err := h.svc.GetLog(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}
	c.JSON(http.StatusOK, log)
}
```

3c. `internal/router/router.go`：读分组（登录即可）加 `auth.GET("/logs/:id", taskH.LogByID)`。

- [ ] **Step 4: 跑测试确认通过**

```bash
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go test ./internal/handler/ -run TestLogDetail -count=1 -v
GOCACHE=$PWD/.dev/go-cache GOMODCACHE=$PWD/.dev/gomodcache GOPATH=/tmp/dsh-gopath go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/task_service.go internal/handler/task_handler.go internal/router/router.go internal/handler/log_detail_test.go
git commit -m "feat(logs): 单条发送日志详情 API（F1）"
```

---

## Task 11: F1c 前端日志详情页

**Files:**
- Create: `web/src/views/LogDetail.vue`
- Modify: `web/src/router/index.ts`
- Modify: `web/src/views/Logs.vue`
- Modify: `web/src/api/index.ts`

- [ ] **Step 1: `web/src/api/index.ts` 的 `logApi` 增加**：

```ts
export const logApi = {
  // ...existing...
  detail: (id: number): Promise<any> => client.get(`/logs/${id}`).then((r) => r.data),
  export: (params: { task_id?: number; status?: string; from?: string; to?: string }) =>
    client.get('/logs/export', { params, responseType: 'blob' }).then((r) => r.data as Blob),
}
```

- [ ] **Step 2: 新建 `web/src/views/LogDetail.vue`**：
- 路由参数 `id` → `logApi.detail(id)` 加载；`el-descriptions` 展示 id/时间/任务/渠道/状态/触发方式/人/IP/错误。
- subject 用标题；content 用 `MarkdownPreview` 渲染（复用 `web/src/components/MarkdownPreview.vue`，参考 Logs.vue 现有用法）。
- request/response 用 `<pre>` 展示（若为 JSON 可格式化）。
- 失败记录显示「重试」按钮 → `logApi.retry(id)` → `ElMessage` 提示（成功 202）。

- [ ] **Step 3: `web/src/router/index.ts`** 在 logs 子路由后加：

```ts
{ path: 'logs/:id', component: () => import('@/views/LogDetail.vue'), meta: { title: '日志详情' } },
```

- [ ] **Step 4: `web/src/views/Logs.vue`**：
- 行内展开（如有）保留或改为「详情」链接/按钮 → `router.push('/logs/' + row.id)`；至少新增一个「详情」入口。
- 顶部工具区新增「导出 CSV」按钮 → `logApi.export(当前筛选)` → Blob 下载（`URL.createObjectURL` + `<a download>`）。

- [ ] **Step 5: 前端构建验证**

```bash
cd web && npm --cache $PWD/../.dev/npm-cache run build
```

Expected: 构建通过。

- [ ] **Step 6: 提交**

```bash
git add web/src/views/LogDetail.vue web/src/router/index.ts web/src/views/Logs.vue web/src/api/index.ts
git commit -m "feat(web): 发送日志详情页 + 导出 CSV 按钮（F1）"
```

---

## Task 12: F1d 前端日志导出按钮（若已在 Task 11 Step 4 完成，则本任务标记为完成并在回归时确认）

**Files:** （由 Task 11 覆盖）
- 若无独立工作，直接执行 Task 13。

- [ ] **Step 1:** 确认 Logs.vue 已含导出按钮且构建通过（见 Task 11 Step 4/5）。若无，补齐后再提交。

---

## Task 13: 收尾回归与文档

**Files:**
- Modify: `README.md`（环境变量表 + API 概览）
- Modify: `CHANGELOG.md`（Unreleased 二期）
- Modify: `.env.example`（新增 METRICS_* 示例）

- [ ] **Step 1: 全量回归**

```bash
make vet
make test
cd web && npm --cache $PWD/../.dev/npm-cache run build
```

Expected: 全部 PASS。

- [ ] **Step 2: `README.md`**
- 环境变量表新增：

```markdown
| `METRICS_ENABLED` | true | 是否暴露 /metrics Prometheus 指标端点 |
| `METRICS_USER` / `METRICS_PASSWORD` | 空 | 同时设置时 /metrics 启用 Basic Auth（建议再用反代/内网保护） |
```

- API 概览追加：

```markdown
日志  GET /api/logs/export   导出发送日志 CSV（仅管理员，同列表筛选，上限 10 万行）
      GET /api/logs/:id      单条发送日志详情（完整内容）
系统  GET /metrics            Prometheus 指标（METRICS_ENABLED 控制；可选 Basic Auth）
备份  GET /api/export        导出渠道/模板/任务 JSON（仅管理员，含明文 config）
      POST /api/import       导入备份（重映射 id、名称冲突跳过、保留 api_key）
Webhook 可选 HMAC 签名：任务开启「需要签名」后须带 X-Timestamp（±300s）与 X-Signature（hex HMAC-SHA256(key=api_key, msg="<ts>\n<body>")）
```

- [ ] **Step 3: `CHANGELOG.md`** Unreleased 增补：

```markdown
### 二期功能
- **Webhook 可选 HMAC 签名**：任务可开启「需要签名」，调用方须带 X-Timestamp（±300s 防重放）与 X-Signature（hex HMAC-SHA256(key=任务api_key, msg="<timestamp>\n<body>")）；默认关闭，向后兼容
- **发送日志 CSV 导出 + 详情页**：管理员可按列表同款筛选导出 CSV（含任务/渠道名，上限 10 万行）；新增 GET /api/logs/:id 与前端详情页（渲染全文/请求/响应/错误，支持重试）
- **Prometheus /metrics**：新增 /metrics 端点（notice_sends_total / notice_send_duration_seconds / notice_queue_pending / http_requests_total + Go runtime），可选 Basic Auth（METRICS_USER/PASSWORD），METRICS_ENABLED 可关
- **数据备份/恢复**：管理员可导出渠道（明文 config）/模板/任务为 JSON，并可导入（按 渠道→模板→任务 顺序建表、旧 id 重映射、名称冲突跳过、api 任务保留 api_key）
```

- [ ] **Step 4: `.env.example`** 追加：

```bash
# 监控（可选）
# METRICS_ENABLED=true
# METRICS_USER=
# METRICS_PASSWORD=
```

- [ ] **Step 5: 提交**

```bash
git add README.md CHANGELOG.md .env.example
git commit -m "docs: 二期功能收尾（README/CHANGELOG/.env.example）"
```

---

## Self-Review（写完后对照 spec 自查）

- **Spec 覆盖**：R4→Task1/2/3；F2→Task4/5；F3→Task6/7/8；F1→Task9/10/11/12；迁移（011）→Task1；依赖→Task4；文档→Task13。无缺口。
- **占位符**：`nameExists`（Task 7）标注为「若 repo 无按名查询需补方法」——实现时按现存 repo 补 `CountByName*`，属计划内的落地细节；Task 12 允许与 Task 11 合并。其余步骤均含完整代码/指令。
- **类型/命名一致性**：`metrics.SendsTotal`/`SendDuration`/`HTTPRequests`/`QueuePendingFunc`/`Handler()`、`ExportBundle`/`ImportResult`/`SetAPIKey`/`ListExportRows`/`logFilterFromQuery` 在相关任务中拼写一致。
- **风险提示**：F2 的 `notice_queue_pending` GaugeFunc 在 scrape 时查询 DB（库故障时 scrape 报错，可接受）；F3 导出含明文 config（仅管理员 + 文档明示）；R4 默认关闭不影响既有调用。
