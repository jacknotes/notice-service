# Notice Service · 异步发送队列与可靠性加固设计

> 日期：2026-08-19
> 状态：已批准（brainstorming 会话，用户已逐节确认）
> 修订：v1

## 1. 背景与目标

### 1.1 现状问题（已核实）

| # | 问题 | 严重度 | 证据 |
|---|------|--------|------|
| 1 | Webhook 触发是**同步阻塞**发送：`webhook_handler` 同步调 `SendTask`，`sendWithRetry` 单接收者最多 sleep 5s→30s→60s 约 **95s**，多个接收者线性叠加 | 高 | `internal/handler/webhook_handler.go:46`、`internal/service/notification_service.go:104-126` |
| 2 | 前端 axios 超时 15s：客户端早已超时，服务端仍在后台跑 | 高 | `web/src/api/client.ts` |
| 3 | Cron 用 `SkipIfStillRunning`：一次超长失败会跳过后续多次计划执行 | 中 | `internal/scheduler/scheduler.go:27-30` |
| 4 | 任务级 60s 租约锁**无续期**：任务执行超过 60s 时另一实例可抢锁**双发** | 中 | `internal/repository/task_repo.go:11` |
| 5 | `task_logs` **无保留策略**，无限增长 | 中 | 无任何清理逻辑（已 grep 确认） |
| 6 | `UpdateSchedule`（更新 `last_run_at`/`next_run_at`）**定义了但从未被调用** | 中 | `internal/repository/task_repo.go:178`，无调用点 |
| 7 | 单次失败重试在 worker 内 `time.Sleep`：worker 被单个坏任务阻塞 95s | 中 | `notification_service.go:108-111` |

### 1.2 目标

- Webhook 触发**立即返回**，不再阻塞/超时
- 发送任务**落库持久化**：进程崩溃不丢单（至多延迟）
- **多副本正确性**：同一 job 只被发送一次；实例崩溃后被其它实例接管
- Cron 不再因长任务漏跑
- `task_logs` 与已完成 job 可配置保留期，自动清理
- 零新基础设施依赖（仅 MySQL），符合自托管轻量定位

### 1.3 非目标（YAGNI）

- 不引入 Redis / RabbitMQ 等真消息队列
- 不做 job 优先级、延迟调度、死信队列、调用方幂等键
- 不改变 channel 测试按钮语义（保持同步，它测的是配置连通性）

## 2. 总体方案

新增一张 `send_jobs` 队列表 + 每实例一个 worker 池。入队（毫秒级）与消费（慢，后台）分离：

```
webhook / cron ──Enqueue(taskID, vars)──▶ send_jobs 表（落库即持久化）
                                                    ▲
worker 池（每实例，默认 4 goroutine）───────────────┘
   认领 → 发送（复用现有 SendTask 渲染/发送/写日志管线）→ 标记结果 → 循环
```

「异步不阻塞 + 落库持久 + 多副本不重复 + 崩溃接管 + 日志清理」用一个表 + 一个 worker 池统一解决。

## 3. 数据模型：`send_jobs` 表

```sql
CREATE TABLE IF NOT EXISTS send_jobs (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id       BIGINT NOT NULL,
    vars_json     JSON,                    -- webhook 请求变量快照（cron 为 null）
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
                  -- pending → claimed → (done | failed)；可重试的失败回到 pending
    claimed_by    VARCHAR(64),
    claimed_at    DATETIME,
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME,
    last_error    TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    sent_at       DATETIME,
    dedupe_key    VARCHAR(128),            -- cron 幂等键；webhook 不填
    KEY idx_jobs_status (status, next_retry_at),
    KEY idx_jobs_created (created_at),
    UNIQUE KEY uk_jobs_dedupe (dedupe_key),
    CONSTRAINT fk_jobs_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 3.1 设计决策

- **粒度**：一个 job = 一次「任务发送」（含全部接收者），不是每接收者一行。部分接收者失败 → 整体按队列重试。
- **内容策略**：job 只存 `task_id + vars_json`，消费时**重新加载任务/渠道/模板并渲染**（cron 场景要最新内容；webhook 请求变量已快照）。任务在消费前被删 → job 级联删除；渠道/模板被删 → 以明确错误标记失败。
- **级联**：`tasks` 软删除不会触发 FK 级联（软删除是 UPDATE），因此**入队前校验任务存在且启用**；job 的 task_id 指向任务表物理行。
- **迁移机制**：`database.Migrate` 从嵌入单个 `001_init.sql` 改为 `//go:embed migrations/*.sql` 按文件名顺序执行，追加 `002_send_jobs.sql`。

## 4. 入队路径

### 4.1 Webhook（同步 → 异步）

`webhook_handler.Trigger` 由「同步 `SendTask`」改为「`Enqueue(taskID, vars)` + 立即返回 `202 Accepted {ok:true, job_id}`」。

保留现有校验顺序：api_key 有效 → 任务启用 → IP 白名单 → 入队。

### 4.2 Cron（快速入队）

scheduler 回调由「`SendTask`」改为「`Enqueue`（毫秒级返回）」。

收益：
- 60s 任务级租约锁现在只保护**入队**这一毫秒级临界区 → 崩溃接管窗口大幅缩短
- `SkipIfStillRunning` 不再因 95s 长任务漏跑
- 入队时顺带更新 `tasks.last_run_at = NOW()`，并把死字段 `next_run_at` 用 `cron.ParseStandard` 算出来写回（修复问题 6）

## 5. 消费者与多副本正确性

### 5.1 Worker 池

每实例 `QUEUE_WORKERS` 个 goroutine（默认 4），循环执行：认领 → 发送 → 标记结果。

### 5.2 认领（原子条件 UPDATE）

```sql
UPDATE send_jobs j
JOIN (SELECT id FROM send_jobs
      WHERE status='pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
      ORDER BY id LIMIT ?) t ON j.id = t.id
SET j.status='claimed', j.claimed_by=?, j.claimed_at=NOW()
WHERE j.status='pending'
```

- MySQL 行锁保证同一行只有**一个**实例的 UPDATE 生效（受影响行数 = 1 才算抢到）→ 多副本天然不重复
- 每个 worker 认领一个 batch（默认 1）并立即执行，执行完再认领下一个

### 5.3 发送

worker 调用现有 `SendTask(taskID, vars)` 管线（加载 → 渲染 → 按接收者发送 → 写 `task_logs`）。

- 任务在入队后被禁用/删除 → 跳过发送，标记 `done`（尊重「停用 = 停止发送」的直觉；已删除任务靠 FK 级联移除，正常不会走到这里）
- 发送成功但任务已不存在 → 标记 done（内容已发出）

### 5.4 结果处理

- 成功 → `status='done'`, `sent_at=NOW()`
- 失败 → `attempts+1`
  - `attempts < QUEUE_MAX_ATTEMPTS` → `status='pending'`, `next_retry_at = now + backoff[attempts-1]`（5s/30s/60s）
  - 否则 → `status='failed'`, `last_error=...`

**重试上移为队列级重试**：现有 `SendTask` 内部的 sleep 重试（5s/30s/60s）移除，重试由队列调度。worker 不再被单个坏任务阻塞 95s（4 个 worker 不会被一个坏任务占满）。需同步修改现有重试相关代码与测试。

### 5.5 陈旧认领恢复（替代「租约续期」）

```sql
UPDATE send_jobs SET status='pending', claimed_by=NULL, claimed_at=NULL
WHERE status='claimed' AND claimed_at < NOW() - INTERVAL ? SECOND AND attempts < ?
```

- 实例崩溃 → `claimed_at` 过旧（`QUEUE_CLAIM_TTL` 默认 120s）→ 其它实例自动接管
- **这直接吸收「租约自动续期」问题**：不再需要给任务锁续期，崩溃接管由队列层保证

### 5.6 幂等（Cron 双入队防护）

- cron 入队时填 `dedupe_key = task_id + ':' + 计划触发时刻(Unix)`，靠 `UNIQUE KEY uk_jobs_dedupe` 兜底
- 即使租约锁出现极端竞态（实例 A 入队后崩溃 → 锁过期 → 实例 B 再次触发），也不会重复入队
- webhook 不填 dedupe_key（MySQL UNIQUE 允许多个 NULL）

## 6. 数据清理

每实例一个清理协程（每天一次；幂等删除跨实例安全，无需选举唯一清理者）：

- `task_logs`：`DELETE ... WHERE sent_at < NOW() - INTERVAL {LOG_RETENTION_DAYS} DAY LIMIT 1000` 循环，直到影响行数 < 1000
- `send_jobs`：`DELETE ... WHERE status IN ('done','failed') AND updated_at < NOW() - INTERVAL {QUEUE_JOB_RETENTION_DAYS} DAY LIMIT 1000` 循环

## 7. 配置项

| 变量 | 默认 | 说明 |
|------|------|------|
| `QUEUE_WORKERS` | 4 | 每实例 worker 数 |
| `QUEUE_POLL_MS` | 1000 | 认领轮询间隔 |
| `QUEUE_MAX_ATTEMPTS` | 3 | 队列级最大尝试次数（含首次） |
| `QUEUE_RETRY_BACKOFF` | `5s,30s,60s` | 重试间隔（逗号分隔，长度须 ≥ MAX_ATTEMPTS-1） |
| `QUEUE_CLAIM_TTL` | 120 | 认领后多久算陈旧（秒） |
| `LOG_RETENTION_DAYS` | 90 | 发送日志保留天数 |
| `QUEUE_JOB_RETENTION_DAYS` | 30 | 已完成 job 保留天数 |

全部带默认值，零配置即可跑。`QUEUE_RETRY_BACKOFF` 需解析逗号分隔的 duration 列表。

## 8. API 变更

| 变更 | 说明 |
|------|------|
| `POST /api/webhook/:api_key` | 返回码改为 **202**，body 增加 `job_id` |
| 其余 API | 无变更 |

## 9. 测试策略

**单元测试**
- 认领原子性：多 goroutine 并发认领同一批 job，每个 job 恰好被认领一次
- 陈旧恢复：`claimed_at` 过旧的行被重新认领（接管）
- 重试退避：失败后 `next_retry_at` 按 backoff 推进，到期前不可认领
- 清理幂等：并发重复清理不报错、结果一致
- dedupe：相同 `dedupe_key` 重复入队只产生一行

**集成测试**
- enqueue → worker 消费 → 本地 sink 收到（复用现有 `internal/integration` 框架）
- webhook 触发返回 202 且异步完成

**多实例模拟**
- 两个 worker/实例抢同一批 job → 最终每个 job 恰好发送一次

**回归**
- 现有 12 个包测试全绿；重试相关测试迁移到队列级语义

## 10. 涉及文件

| 文件 | 变更 |
|------|------|
| `internal/database/migrations/002_send_jobs.sql` | 新增 |
| `internal/database/db.go` | 迁移改为按文件顺序执行 |
| `internal/repository/send_job_repo.go` | 新增：入队/认领/标记/陈旧恢复/清理 |
| `internal/service/queue_service.go` | 新增：worker 池 + 清理协程 |
| `internal/service/notification_service.go` | `SendTask` 移除 sleep 重试；新增 `Enqueue` |
| `internal/service/task_service.go` | cron 变更时同步调度器（不直接受影响） |
| `internal/handler/webhook_handler.go` | 改为入队 + 返回 202 |
| `cmd/server/main.go` | 启动/停止队列服务 |
| `internal/config/config.go` | 新增 7 个配置项 |
| `internal/repository/task_repo.go` | `UpdateSchedule` 接入；入队时更新 `last_run_at`/`next_run_at` |
| `README.md` / `CHANGELOG.md` | 文档与变更记录 |

## 11. 风险与缓解

| 风险 | 缓解 |
|------|------|
| MySQL 作为队列的吞吐上限 | 自托管中型团队量级（分钟级 ~ 低并发 webhook），单表轮询绰绰有余；`idx_jobs_status` 覆盖认领查询 |
| 多副本同时清理 | 幂等 DELETE + LIMIT 循环，行锁防冲突 |
| 认领后 worker 被 kill -9 | 陈旧恢复兜底（claimed_at 过期即接管） |
| 现有重试逻辑改动引入回归 | 测试迁移 + 全量回归 |
| `tasks` 软删除不级联到 send_jobs | 入队前校验存在/启用；worker 消费时再校验一次 |

## 12. 变更记录

- 2026-08-19 v1：初始设计（异步发送队列 + 多副本正确性 + 数据清理）。
