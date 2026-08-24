# 安全风险加固设计（一期）

> 日期：2026-08-24 · 状态：**已评审通过** · 范围：7 项风险修复（R1/R2/R3/R5/R6/R8/R12）

## 1. 背景与目标

当前项目已完成 1.x 迭代（多渠道通知、模板、任务、2FA、审计、多实例高可用），
代码审查发现若干安全与可靠性风险点。本设计做**一轮安全/风险加固**：全部为
行为等价修复（不改接口语义、不改 JWT 格式、不改部署形态），多实例一致性优先。

目标：消除以下 7 个已确认风险点：

| 编号 | 风险 | 等级 |
|------|------|------|
| R1 | 被降级的管理员已签发 token 在 24h 内仍拥有 admin 权限（角色写死在 JWT 里，中间件只回查"未禁用"不回查"当前角色"） | 高 |
| R2 | 登录限流与 Webhook 限流均为内存态、各实例独立计数，多实例可绕过（2 实例 = 2 倍额度） | 高 |
| R3 | 未配置 `ENCRYPT_KEY` 时自动生成随机密钥写本地文件；容器/多实例未持久化时重启丢失 → 历史渠道密文不可解 | 高 |
| R5 | Webhook 限流器 `keyRateLimiter` 的 map（windowAt/hits）按 api_key 只增不减，内存缓慢泄漏 | 中 |
| R6 | 优雅退出时 `queue.Stop()` 无超时，worker 卡在发送时进程挂住不退 | 中 |
| R8 | Webhook 请求体解析错误被忽略（`_ = c.ShouldBindJSON`），畸形 JSON 被当作空变量接受 | 中 |
| R12 | 静态资源用相对路径 `./web/dist`，非固定 CWD 启动（如 systemd）时前端 404 | 中 |

> R5 不单独实现：随 R2 将限流迁到 MySQL 后，内存态 map 整体删除，泄漏自然消失。

## 2. 总体改动面

| 项 | 涉及模块 | 性质 |
|----|---------|------|
| R1 | `internal/middleware/auth.go`、`internal/service/auth_service.go` | 角色每次请求从 DB 读取 |
| R2 | 新增 `internal/repository/rate_limit_repo.go`、`internal/database/migrations/010_rate_limits.sql`；改 `internal/handler/webhook_handler.go`、`internal/service/login_limiter.go`、auth 服务 | 新增 1 张表，登录+Webhook 限流迁 MySQL |
| R3 | `internal/config/config.go`、`cmd/server/main.go`、`docker-compose.yml` | 可配置密钥文件路径 + 启动硬校验 |
| R6 | `internal/service/queue.go`、`cmd/server/main.go` | 队列排空加超时兜底 |
| R8 | `internal/handler/webhook_handler.go` | 坏请求体返回 400 |
| R12 | `cmd/server/main.go`、`internal/config/config.go` | `STATIC_DIR` 可配置 + 探测兜底 |

## 3. R1 · 角色即时生效

**现状**：`Auth` 中间件校验 JWT 后把 `claims.Role` 写入 `c.Set("role", ...)`；
`AdminOnly` 中间件读取该值。token 一旦签发，角色冻结 24h。

**方案**：角色每次请求从 DB 读取。

- 把现有 `AuthService.UserActive(uid)`（每请求已执行，用于"禁用立即生效"）升级为
  `UserForAuth(uid)`，一次查出 `enabled + role + username`。
- 中间件逻辑：查不到 / `enabled=false` → 401；否则
  `c.Set("role", dbUser.Role)`、`c.Set("username", dbUser.Username)`——角色与显示名
  不再取自信令。
- 效果：提权、降级、改名，下一次请求立即生效，与"禁用立即生效"同一口径。
- **不做角色版本号/黑名单**：那是给无法每请求查库的高吞吐场景用的；本服务每请求
  本就有一次 DB 查询，合并即可，加版本机制纯属复杂度。

**边界**：内置 `admin` 的保护在 DB 层，天然覆盖；`AdminOnly` 中间件无需改动
（它读的 `role` 现在来自 DB）。

## 4. R2 · 集中式限流（MySQL）

**目标**：登录限流（每用户名 5 次失败/锁定 15 分钟）与 Webhook 限流
（每 api_key 60 次/分钟）迁到 MySQL，多实例共享计数。

### 4.1 新表 `rate_limits`（迁移 `010_rate_limits.sql`）

```sql
CREATE TABLE IF NOT EXISTS rate_limits (
  bucket       VARCHAR(128) NOT NULL,   -- 'login:<user>' 或 'webhook:<api_key>'
  window_start BIGINT      NOT NULL DEFAULT 0,  -- 固定窗口起始 unix 秒；login 恒为 0
  count        INT         NOT NULL DEFAULT 0,  -- 窗口内次数（login 为连续失败次数）
  locked_until DATETIME    NULL,                -- login 锁定到期时间；webhook 恒为 NULL
  updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (bucket, window_start)
);
```

### 4.2 算法（原子、MySQL 5.7 兼容、无锁）

- **Webhook（固定窗口计数）**：bucket=`webhook:<key>`，`window_start` = 当前分钟截断。
  `INSERT ... ON DUPLICATE KEY UPDATE count = count + 1` → `SELECT count` → `count <= 60` 放行。
  窗口滚动 = 换一行（主键含 window_start）；并发下最多略超（fail-safe，绝不小放行）。
- **登录（连续失败 + 锁定）**：bucket=`login:<user>`，`window_start=0`。
  - `checkLocked`：`SELECT locked_until`，未过期 → 拒绝；
  - `recordFailure`：`INSERT ... ON DUPLICATE KEY UPDATE count = count + 1`，达到上限时
    `UPDATE ... SET locked_until = NOW() + 锁定窗口`；
  - `reset`：登录成功后 `DELETE` 该 bucket。
- **DB 故障策略**：限流记录错误并**放行（fail-open）**。登录本就依赖 DB（库挂了登录
  自然失败）；Webhook 在库故障窗口内不限额，属可接受取舍，日志记录。

### 4.3 清理

复用现有 `cleanerLoop`（每日）删除过期窗口行（如 `updated_at` 超过 24h），防止表无限
膨胀。**这同时解决 R5**：内存 map 删除，DB 行有清理兜底。

### 4.4 改动点

- 新增 `internal/repository/rate_limit_repo.go`：`Allow / CheckLocked / RecordFailure / Reset / Cleanup`。
- `webhook_handler.go`：删除 `keyRateLimiter`，改调 repo。
- `login_limiter.go` / auth 服务：删除内存 `loginLimiter`，改调 repo（语义不变）。

### 4.5 性能

每次 Webhook 触发多 2 条 SQL；对通知服务量级（每 key 60 次/分钟上限）完全可接受。
未来量大可加内存缓存，本期不做。

## 5. R3 · ENCRYPT_KEY 密钥持久化兜底

核心原则：**宁可启动失败，不要静默丢数据**。

- 新增可配置密钥文件路径 `ENCRYPT_KEY_FILE`（config.yml `encrypt_key_file`），
  默认 `.notice-encrypt.key`，向后兼容。
- **启动硬校验**：若未设 `ENCRYPT_KEY` 且密钥文件不存在，但数据库已存在**加密的
  渠道配置行**（说明是重启而非首启）→ 直接 `log.Fatalf` 并给出可操作提示
  （"请设置 ENCRYPT_KEY 或恢复密钥文件，否则历史渠道配置无法解密"），
  绝不静默生成新密钥。
  - 检测口径：`channels.config_json` 存的是**整段 AES 密文**（`encryptConfig`
    对整个配置 JSON 加密，见 `internal/service/channel_service.go`），故
    判定为"存在加密数据"的条件是：
    `SELECT COUNT(*) FROM channels WHERE config_json IS NOT NULL AND config_json != '' AND deleted_at IS NULL`，计数 > 0 即成立。
- 首启（无任何密文）且无密钥 → 维持现状生成并落盘，但输出醒目告警提示持久化。
- `docker-compose.yml`：给密钥文件挂命名卷（如 `encrypt-key:/app`），
  配合 .env 已要求的 `ENCRYPT_KEY`。

## 6. R6 · 优雅退出超时兜底

- 给 `QueueService` 增加 `StopWithTimeout(d)`：关闭 `stopCh` 后用
  `sync.WaitGroup` + 计时器等待 worker 退出，超时记录"队列排空超时，强制退出"并返回。
- `main.go` 退出流程：`srv.Shutdown` → `queue.StopWithTimeout(15s)`。
- 顺带核查各渠道 `Send` 是否都有硬超时（若有缺失一并补上，保证 worker 不会永久卡死）。

## 7. R8 · Webhook 畸形 JSON 返回 400

`webhook_handler.go` 中 `_ = c.ShouldBindJSON(&req)` 改为检查错误：
`io.EOF`（空 body，允许，按空变量处理）放行，其它解析错误返回
`400 {"error":"请求体不是合法 JSON"}`。

## 8. R12 · 静态资源路径健壮化

- 新增配置 `STATIC_DIR`（config.yml `static_dir`），默认 `./web/dist`。
- 启动时按优先级探测：`STATIC_DIR` → `./web/dist` → 可执行文件同目录 `web/dist`。
- 都找不到时：记录告警日志但服务照常启动（API 可用、SPA 404），引导设置 `STATIC_DIR`。
- Docker 内 WORKDIR 固定，不受影响；主要解决 systemd/其它 CWD 启动场景。

## 9. 测试策略（全部 Go 侧）

沿用现有 repo/service/handler 测试模式（独立测试库 `notice_service_test`）：

- **R1**：auth 中间件——签发 token 后降级角色，断言下一请求按新角色返回 403/200；
  禁用仍即时 401。
- **R2**：`rate_limit_repo` 单测——Allow 计数/超限/窗口滚动/Reset；RecordFailure 到上限
  触发锁定、锁定期内拒绝、过期自动解锁；Cleanup 删旧行。webhook handler 测试改走 repo。
- **R3**：config 测试——`ENCRYPT_KEY_FILE` 生效；"存在密文但无密钥"路径返回错误
  （用可注入探针测试而非真退出）。
- **R6**：queue 测试——worker 阻塞时 `StopWithTimeout` 超时返回。
- **R8**：webhook handler 测试——坏 JSON → 400，空 body → 202。
- **R12**：config/启动测试——`STATIC_DIR` 解析与目录不存在告警路径。

最终 `make vet && make test && make build` 全绿。

## 10. 兼容性

- JWT 格式、token 时效、所有 API 契约**不变**；旧 token 继续有效（角色改以 DB 为准）。
- 配置项全部新增且带默认值，旧 config.yml / 纯环境变量部署无需改。
- 限流语义不变（5 次/15 分钟、60 次/分钟），只是计数从内存换到 DB。

## 11. 数据迁移

新增 `internal/database/migrations/010_rate_limits.sql`（仅 `rate_limits` 一张表，
幂等 `CREATE TABLE IF NOT EXISTS`），沿用现有 schema_migrations + GET_LOCK 机制自动应用。

## 12. 实施顺序（一份实现计划，按依赖排）

1. R1 角色即时生效（中间件/服务 + 测试）
2. R2 集中式限流（迁移表 → repo → 登录/Webhook 接入 + 测试）
3. R3 密钥持久化兜底（config + 启动校验 + compose 卷）
4. R6 优雅退出超时（queue + main）
5. R8 畸形 JSON 400（webhook handler）
6. R12 静态目录（config + main）
7. 全量回归：`make vet && make test && make build` + 更新 CHANGELOG/README

## 13. 本期不做（已明确排除）

- R4 Webhook HMAC 签名认证（偏向新功能，放下期）
- R9 前端自动化测试（工程质量，非本轮安全加固）
- 限流内存缓存加速、i18n、多租户等远期项
