# 二期功能设计（R4 Webhook HMAC · F1 日志导出/详情页 · F2 /metrics · F3 JSON 导入导出）

> 日期：2026-08-24 · 状态：**已评审通过** · 范围：4 个独立小功能，全部后端可独立交付，F1/F3 附带最小前端

## 1. 背景与目标

安全加固一期（R1/R2/R3/R5/R6/R8/R12）已合并。本二期做上一期「已明确排除 / 计划中」的 4 项：

| 编号 | 功能 | 说明 |
|------|------|------|
| R4 | Webhook HMAC 签名认证 | 在 api_key 之上加可选 HMAC 签名 + 时间戳防重放 |
| F1 | 发送日志导出 CSV + 独立详情页 | CHANGELOG「计划中」项；导出做备份/审计 |
| F2 | `/metrics` Prometheus 端点 | 依据已有《监控告警接入设计》M1 里程碑落地 |
| F3 | 任务/模板/渠道 JSON 导出导入 | 备份迁移场景 |

原则：**全部向后兼容、可独立启用**；新增配置/字段带默认值；F2 引入的新依赖已确认可下载。

## 2. R4 · Webhook HMAC 签名认证

### 2.1 数据与配置

- 迁移 `011_task_signature.sql`：`ALTER TABLE tasks ADD COLUMN require_signature TINYINT(1) NOT NULL DEFAULT 0 AFTER api_key;`
- 默认关闭：既有 api 任务无需改动即可继续用纯 api_key 调用。

### 2.2 签名协议

- HMAC 密钥 = 任务自身的 `api_key`（调用方与服务端已共享的秘密，不新增字段）。
- 请求头：
  - `X-Timestamp`：unix 秒（服务端本地时间，允许 ±300s 偏差，防重放）。
  - `X-Signature`：`hex( HMAC-SHA256( key=api_key, msg = "<timestamp>\n<raw request body>" ) )`，小写十六进制。
- 校验流程（仅 `require_signature=1` 时）：
  1. 缺 `X-Timestamp` 或 `X-Signature` → 400。
  2. `|now - ts| > 300s` → 401（时间戳过期）。
  3. 常量时间比较签名（`crypto/subtle.ConstantTimeCompare`）→ 失败 401「签名无效」。
  4. 通过后正常继续。
- 请求体处理：`io.ReadAll(c.Request.Body)` 一次读取（≤1MB 由现有 bodyLimit 限制），同一份字节既用于 HMAC 又用于 `json.Unmarshal`（不再走 `c.ShouldBindJSON`，避免重复读 body）。
- 触发顺序：限流 → `GetByAPIKey` → 签名校验（若开启）→ 启用 → IP 白名单 → 解析 body → 入队。

### 2.3 前端

任务表单新增「需要签名」开关；开启后展示调用示例（两个请求头 + 签名算法说明）。

### 2.4 兼容性

默认 `require_signature=0`，零影响；开启后旧调用（无签名头）被 401 拒绝，属预期行为。

## 3. F1 · 发送日志导出 CSV + 独立详情页

### 3.1 后端

- `GET /api/logs/export`（**仅管理员**）：与 `GET /api/logs` 同款筛选（task_id / 状态 / 日期区间 / 触发方式等），**不分页**返回全部匹配行，上限 10 万行；响应 `text/csv` + `Content-Disposition: attachment`，文件名含时间戳。
  - 列：`id, sent_at, task_id, task_name, channel_id, channel_name, status, subject, error_msg, trigger_type, trigger_by, trigger_ip`（不含 content/request/response 大字段，防文件膨胀）。
  - 用 `encoding/csv` 处理转义；任务/渠道名左连接查询（名称对象删除后仍可读）。
- `GET /api/logs/:id`（登录即可）：返回单条完整日志（含 content/request/response/触发信息），供详情页使用；不存在 404。

### 3.2 前端

- 新增路由 `/logs/:id` 详情页：渲染 subject、content（Markdown 预览复用 `MarkdownPreview`）、request/response/error、触发方式/人/IP、时间；提供「重试」按钮（失败记录）。
- 列表页行内展开改为「详情」入口跳转。

## 4. F2 · `/metrics` Prometheus 端点

### 4.1 依赖

- 新增 `github.com/prometheus/client_golang`（版本 v1.21.1，已确认可经 goproxy.cn 下载；`go mod tidy` 补齐间接依赖并提交 go.mod/go.sum）。

### 4.2 指标

`internal/metrics/metrics.go` 统一注册：

| 指标 | 类型 | 来源 |
|------|------|------|
| `notice_sends_total{channel,status}` | CounterVec | `NotificationService.sendOnce`（success/failed 各计一次） |
| `notice_send_duration_seconds{channel}` | Histogram | sendOnce 计时 |
| `notice_queue_pending` | Gauge (GaugeFunc) | scrape 时 `SELECT COUNT(*) FROM send_jobs WHERE status IN ('pending')` |
| `http_requests_total{code,method,path}` | CounterVec | 接入现有 `accessLogger` 中间件 |
| Go runtime / process | 默认采集器 | `promhttp.Handler()` 自带 |

### 4.3 路由与安全

- `GET /metrics`：受 `METRICS_ENABLED`（默认 true）控制；可选 Basic Auth（`METRICS_USER` 与 `METRICS_PASSWORD` 都非空才启用）。
- 文档强调：不要将 `/metrics` 直接暴露公网，走内网 / 反代 allowlist（复用 `TRUSTED_PROXIES` 思路的部署约束）。

### 4.4 测试

- metrics 包单测（counter 自增、registry 可用）。
- handler 测试：`METRICS_ENABLED` 开 → `/metrics` 200 且含 `notice_sends_total`；关 → 404；Basic Auth 启用 → 无凭据 401、正确凭据 200。

## 5. F3 · JSON 导出导入（备份迁移）

### 5.1 后端

- `GET /api/export`（**仅管理员**）→ `{ version: 1, exported_at, channels:[...], templates:[...], tasks:[...] }`。
  - channels 含**明文 config**（解密后导出；敏感，文档明示），templates 含变量定义，tasks 含 channel_ids/template_id/receivers/cron/api_key/allowed_ips/variables。
- `POST /api/import`（**仅管理员**，复用 1MB bodyLimit）：
  - 校验结构与 `version`；
  - 按 渠道→模板→任务 顺序建表，旧 id → 新 id 重映射（channel_ids / template_id）；
  - 渠道 config 用当前 `ENCRYPT_KEY` 重新加密落库；
  - 名称冲突：跳过并记入摘要；
  - 返回 `{ channels_created, templates_created, tasks_created, skipped:[...] }`。

### 5.2 前端（最小）

管理端「个人设置 → 数据备份」区：导出按钮（下载 JSON）+ 文件选择上传导入 + 结果提示。

## 6. 测试、迁移、兼容性与实施顺序

**迁移**：仅 `011_task_signature.sql` 一张（R4）。F2 依赖 go.mod/go.sum 更新。

**兼容性**：所有 API 均为新增（`/api/logs/export`、`/api/logs/:id`、`/metrics`、`/api/export`、`/api/import`）；无既有接口语义变更；`require_signature` 默认 0；`METRICS_ENABLED` 默认 true、Basic Auth 默认关。

**测试**：后端沿用现有 repo/service/handler 模式 + 真实 `notice_service_test` 库；F1/F3 补 handler 全链路测试；F2 补 metrics 单测 + handler 测试；前端 `npm run build` 必须通过（CI 也跑 frontend-build）。

**实施顺序**（一份实现计划，按依赖排）：
1. R4（迁移 → 签名校验 → 前端开关）
2. F2（依赖 → metrics 包 → 路由 → 测试）
3. F3（导出/导入服务 + handler + 前端备份区）
4. F1（日志导出 API + 详情 API → 前端详情页）
5. 全量回归：`make vet && make test` + 前端构建 + CHANGELOG/README/.env.example 更新

**本期不做**：R9 前端自动化测试、i18n、多租户、限流内存缓存加速（仍属远期）。
