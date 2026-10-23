# 性能视角审查：`v2.0.0..HEAD`（52 commits / 100 files / +4751 −396）

审查范围：`git diff v2.0.0..HEAD`。工作区 `F:\project\notice-service`（Go 1.25.3 + Gin / Vue3 + Element Plus）。
本文件只记录**性能**结论，不修改任何业务代码。安全/产品视角的问题交由对应审查员，本文仅在机制上重叠时标注。

## 验证方式与环境

| 项 | 结果 |
| --- | --- |
| `go build ./...` | 通过（exit 0） |
| `go vet ./...` | 通过（exit 0） |
| 无 DB 依赖的测试包 `go test ./internal/render ./internal/channel ./internal/totp ./internal/config` | 全部 `ok` |
| `go test ./internal/scheduler` | 仅 `TestSchedulerLeasePath` 失败（`invalid connection`，本机无 MySQL，属环境缺失，非代码问题） |
| 既有基准测试 | 全仓 `*_test.go` 中**没有** `func Benchmark`，故无现成基准可用 |
| 量化手段 | 为定位 `Next()` 开销，临时在仓内建了一个 `main` 包（`.perfprobe/`）直接调用 `scheduler.NewLunarParser().Parse(...).Next(...)` 并读 `runtime.MemStats`；测量后**已删除**（`git status` 只剩 `.agent-teams/` 与本文件所在的 `docs/reviews/`，无业务代码改动） |

实测（Windows / go1.25.3，单次 `Next()`，含 GC 前分配统计）：

| 表达式 | 单次 `Next()` 耗时 | 分配 | 结果 |
| --- | --- | --- | --- |
| `*/5 * * * *`（标准 5 段） | ~2 µs | 0 B | 命中 |
| `0 9 15 9 * 2026`（年份字段） | ~2 µs | 0 B | 命中 |
| `@lunar monthly 15 09:00` | 1.0–1.5 ms | 114 KB / 2.9k objs | 命中 |
| `@lunar monthly 1,2,…,15 09:00` | 18.4 ms | 1.7 MB | 命中 |
| `@lunar term 春节 09:00`（节日表） | 1.6 ms | 21 KB | 命中 |
| `@lunar term 清明 09:00`（节气） | 36–68 ms | 10–20 MB | 命中 |
| `@lunar term 立春,清明 09:00` | ~125 ms | ~34 MB | 命中 |
| `@lunar term 立春,清明,夏至,冬至 09:00` | ~185 ms | ~54 MB | 命中 |
| `@lunar term 中秋 09:00`（前端内置快捷项） | **336 ms** | **92 MB** | **零值（永不触发）** |
| `@lunar term 不存在 09:00` | 345 ms | 92 MB | 零值 |

---

## P1（high）农历 `Next()` 在调度热路径上同步执行，单次 36–336 ms、10–92 MB，且 `@lunar term 中秋` 每次跑满 8 年 × 370 天

- **位置**
  - `internal/scheduler/lunar_schedule.go:26-41`（`Next` 最多扫 `lunarMaxLookaheadYears=8` 年）
  - `internal/scheduler/lunar_schedule.go:44-76`（`candidatesForYear`）
  - `internal/scheduler/lunar_schedule.go:80-96`（`lunarDayInYear`，第 81 行每个（月,日）组合都 `calendar.NewLunarYear(year)` 新建一次）
  - `internal/scheduler/lunar_schedule.go:115-142`（`termCandidatesInYear`，第 127 行 370 次逐日循环，第 128/136 行每天调两次 `GetLunar()`）
  - `internal/scheduler/scheduler.go:74-105`（`makeJob`）
  - `internal/scheduler/scheduler.go:110-119`（`NextRun`）
  - `internal/service/queue.go:333-352`（`updateSchedule` / `RefreshNextRun`）
  - `cmd/server/main.go:195-206`（启动自愈循环）
  - `internal/service/task_service.go:120-129`、`:186-192`（保存后立即重算）
- **问题机制**
  1. `robfig/cron` 的 run loop 是**单 goroutine**，每次触发后在同一条循环里同步执行 `e.Next = e.Schedule.Next(now)`（`cron/v3@v3.0.1/cron.go:275`，`c.startJob` 在 `:273`；已核对源码）。`LunarSchedule.Next` 的 36–336 ms 直接阻塞**所有任务**的派发：一个 `@lunar term 清明` 任务到点重算时，同一分钟内其它任务被推迟数十毫秒。
  2. `term` 分支每算一次 `Next` 都要从当年 1 月 1 日起逐日推进（370 次 × `GetLunar()`/`Next(1)`），且**对 8 年窗口每年重复一遍**；`monthly`/`yearly` 分支则在每个（年×月×日）组合上重建 `calendar.NewLunarYear`。这是 20–92 MB 分配的直接来源。
  3. 命中不了的名字（`中秋`、任意错别字）走完全部 8 年扫描才返回零值：实测 **336 ms / 92 MB**。
  4. 前端内置快捷项就是 `@lunar term 中秋 09:00`（`web/src/views/Tasks.vue:812`），而节日表键是 `中秋节`（`internal/scheduler/lunar_schedule.go:106`），节气表里也没有「中秋」（lunar-go `JIE_QI` 只有 24 节气名）。于是：**用户点一次快捷按钮得到的是永不触发的任务，且每次保存/启动都为它烧 336 ms + 92 MB**（`lunar_parser.go:23` 的注释同样写成「中秋」，进一步误导用户）。
  5. 启动路径叠加：`main.go:195-206` 对每个 cron 任务先 `RefreshNextRun`（1 次 `Next`）再 `RegisterTask`（`cron.AddFunc` 内部再 1 次 `Next`）；100 个节气任务 ≈ 2×100×68 ms ≈ **13.6 s 启动延迟**，其中 `cron.AddFunc` 那次还发生在 run loop 内。
- **触发条件**
  - 任务使用 `@lunar term <节气名>`（含 `立春,清明,夏至,冬至` 这类列表），或 `@lunar monthly` 配长日列表；
  - 每次 cron 到点、每次任务保存（Create/Update/Toggle 重新启用）、每次进程启动；
  - `@lunar term 中秋` / 任意未知名：每次保存/启动都是最坏路径。
- **修复建议**
  1. `lunarDayInYear` 的 `calendar.NewLunarYear(year)` 提到 `candidatesForYear` 的年份循环外，按年构造一次复用；`termCandidatesInYear` 复用同一个 `Solar`/`Lunar` 对象，去掉第 128 与 136 行的重复 `GetLunar()`。
  2. `Next` 结果缓存：`LunarSchedule` 记录 `(lastAfterYear, next)`，同一年内重复查询直接命中（robfig 对同一 entry 的连续 `Next` 调用具有单调性，缓存安全）。
  3. 对未知节气/节日名在**解析期**拒绝（`parseLunarSchedule` 校验 `lunarFestivals` + 24 节气名白名单），既消除 336 ms 最坏路径，也修掉「中秋永不触发」。
  4. 把 `Next` 的年度扫描上限从「8 年 × 370 天」改为「按节气表直接算候选日」（lunar-go 提供 `GetJieQiTable()`，24 个节气一年只算一次），避免逐日推进。
  5. 前端快捷项与注释统一改为 `中秋节`（`web/src/views/Tasks.vue:812`、`internal/scheduler/lunar_parser.go:23`）。
  6. 长期：把 `Next` 计算移出 cron run loop（注册时预计算、或用独立 goroutine + 缓存表），避免任何单任务解析阻塞全体派发。

## P2（high）队列吞吐被硬编码为 `Workers / PollInterval`（默认 4 job/s），且 worker 忙时 tick 被合并丢弃

- **位置**
  - `internal/service/queue.go:162-181`（`workerLoop`：`time.NewTicker(q.cfg.PollInterval)` + 每次 `Claim(q.instanceID, 1)`）
  - `internal/service/queue.go:183-267`（`process` 同步完成整次发送）
  - `internal/repository/send_job_repo.go:76-117`（`Claim` 的 `limit`）
  - `internal/config/config.go:168-172`（`QUEUE_WORKERS=4`、`QUEUE_POLL_MS=1000`）
- **问题机制**
  - 每个 worker 每个 tick 只认领 **1** 个 job，默认 4 worker × 1 s = **上限 4 job/s**，与发送耗时无关；Webhook 60 次/分钟/Key 的限流是**每 Key**的，多个 Key 并发打进来时会立刻堆积。
  - worker 在 `process` 里同步跑完整个发送（多收件人 SMTP 可长达 `smtpOpTimeout=30 s`），期间 `time.Ticker` 的 channel 容量为 1：30 s 的发送会**合并掉 29 个 tick**，等价于该 worker 这 30 s 只产出 1 个 job。4 个 worker 全忙时，实际吞吐退化到「4 job / 发送耗时」。
  - `Claim` 本身是 3 条 SQL：`SELECT id`（`:78`）→ 逐 id `UPDATE ... WHERE status='pending'`（`:100`）→ `GetByID`（`:110`）。limit=1 时每个 job 固定 3 次往返，空队列时也是每秒 4 条 `SELECT`（4 实例部署 = 16 条/s 空转）。
- **触发条件**：Webhook 突发（多 Key 并发）、单任务多收件人/多渠道路由、SMTP 或第三方 webhook 变慢（`internal/channel/webhook.go:12` 15 s 超时）、实例数少（单副本 4 worker）。
- **修复建议**
  1. `Claim` 批量：一次 `SELECT id ... LIMIT workers`（甚至 `workers*4`），循环 `UPDATE` 认领后把整批交给 worker 池（或直接把 `UPDATE ... WHERE status='pending' ORDER BY id LIMIT n` 后回查），把「1 job/tick」改为「1 批/tick」。
  2. 用「发送完成即立即再取」替代纯 tick 驱动：`process` 返回后立刻再 `Claim`，tick 只作兜底（避免忙时丢 tick）。
  3. `PollInterval` 降到 100–200 ms（空队列时 `SELECT` 走 `idx_jobs_status` 索引，代价可控），或改成 `Claim` 阻塞式通知（DB 不可用时用本地信号量）。
  4. 给 `Claim` 的 `SELECT` 加覆盖索引（见 P4），并把 `GetByID` 合并进同一条 `UPDATE ... RETURNING` 语义（MySQL 无 RETURNING，可改为认领后按 id 列表一次性 `WHERE id IN (...)` 回查，1 条代替 N 条）。

## P3（high）CSV 导出：前端把超时放宽到 300 s，但服务端 `WriteTimeout` 只有 20 s，且 10 万行全量物化在内存

- **位置**
  - `web/src/api/index.ts:107-109`（`client.get('/logs/export', { responseType: 'blob', timeout: 300000 })`）
  - `internal/handler/task_handler.go:444-467`（`ExportLogs`：`ExportLogRows(f, 100000)` 后逐行 `w.Write`）
  - `internal/repository/task_log_repo.go:341-391`（`ListExportRows`：`limit` 上限 100000，结果全部 append 进 `[]*LogExportRow`）
  - `cmd/server/main.go:302-309`（`WriteTimeout: 20 * time.Second`）
- **问题机制**
  - 本次变更把客户端超时从 15 s 放宽到 300 s，但 HTTP 服务器写超时仍为 20 s。导出耗时 = `SELECT` 全量行（含 `content`/`request`/`response` TEXT）+ 组 CSV；超过 20 s 时 `net/http` 会中断响应，浏览器侧表现为**下载被截断或网络错误**，而不是一个可读的「超时」提示——放宽的客户端超时完全无效。
  - `ListExportRows` 把最多 10 万行、每行含多个 TEXT 字段整体读进内存再逐行写出：按每行 1–2 KB 估算即 **100–200 MB** 峰值，且 `content` 是渲染后的正文，单行可达数十 KB。导出与业务请求共用 20 个连接与同一进程堆。
  - `ORDER BY tl.id ASC LIMIT ?` 无上限保护语义（`limit<=0 || limit>100000 → 100000`），默认路径就是 10 万行。
- **触发条件**：管理员在日志页点「导出 CSV」，且筛选区间覆盖大量历史日志（默认近一个月，90 天保留期内的高频任务足够触发）。
- **修复建议**
  1. 服务端流式写：`rows, _ := db.Query(...)` 后边 `rows.Next()` 边 `csv.Writer.Write`，不要先物化切片；同时 `Flush` 分段（每 500 行 flush 一次）让字节持续出网，避免 20 s 写超时。
  2. 导出接口单独放宽 `WriteTimeout`（`http.Server` 是全局的，可用 `http.ResponseController.SetWriteDeadline` 在 handler 内覆盖）。
  3. 把行数上限降到 1–2 万，或改为异步导出（生成文件 + 下载令牌），避免一次请求打满内存。
  4. 前端提示与实际能力对齐：要么服务端支持长导出，要么把 `timeout` 改回 15–30 s 并明确提示「请缩小时间范围」。

## P4（medium）`send_jobs` 认领查询 `ORDER BY id` 触发 filesort，缺 `(status, id)` 索引；完成清理缺 `updated_at` 索引

- **位置**
  - `internal/repository/send_job_repo.go:76-80`（`WHERE status='pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW()) ORDER BY id LIMIT ?`）
  - `internal/database/migrations/002_send_jobs.sql`（仅有 `KEY idx_jobs_status (status, next_retry_at)`、`idx_jobs_created (created_at)`）
  - `internal/repository/send_job_repo.go:210-225`（`CleanupDoneOlderThan`：`WHERE status IN ('done','failed') AND updated_at < ... LIMIT 1000` 循环）
- **问题机制**
  - `idx_jobs_status` 只能过滤 `status`，`ORDER BY id` 必须对全部 pending 行做 filesort；`next_retry_at IS NULL OR ...` 的 OR 条件进一步阻止索引范围收敛。P2 的 tick 驱动下这条语句每秒执行 4 次（4 副本 ×4 worker = 16 次/s），pending 积压（第三方故障、SMTP 慢）时排序集同步膨胀。
  - `CleanupDoneOlderThan` 的过滤列 `updated_at` 没有任何索引，`LIMIT 1000` 的循环删除在「老行稀疏」时要反复全表扫描；`done/failed` 行占表绝大多数。
- **触发条件**：队列积压（>1 万 pending）；`send_jobs` 长期运行（30 天保留，done/failed 行堆积到百万级）。
- **修复建议**
  1. 新增 `KEY idx_jobs_status_id (status, id)`，把认领改成 `ORDER BY id` 走索引；或改成 `ORDER BY next_retry_at, id` 并让索引 `(status, next_retry_at, id)` 覆盖。
  2. 给 `send_jobs` 加 `KEY idx_jobs_updated (updated_at)`（或 `(status, updated_at)`）供清理使用。
  3. 清理改为按 `id` 分页删除（`WHERE id <= ? AND status IN (...) LIMIT 1000`），避免无索引扫描。

## P5（medium）审计日志：新增排序字段 + `LIKE '%kw%'` → 全表扫描 + filesort

- **位置**
  - `internal/repository/audit_repo.go:83-93`（`WHERE (username LIKE ? OR ip LIKE ? OR detail LIKE ?)`，通配符前置）
  - `internal/repository/audit_repo.go:121-131`（白名单排序后 `ORDER BY <col> <dir> LIMIT ? OFFSET ?`）
  - `internal/handler/audit_handler.go:71-77`（本次新增 `sort_by`/`sort_order` 透传）
  - `internal/database/migrations/005_audit_logs.sql`（只有 `KEY idx_audit_created (created_at)`）
- **问题机制**：本次让 `username/ip/action/module/created_at` 都可排序，但这些列都没有索引；`ORDER BY username|ip|action|module` 需要对过滤后的集合做 filesort。叠加 `%kw%` 前置通配（无法走索引）后，一次「带关键词 + 按用户名排序」的查询等于**全表扫描 + 全量排序**。`audit_logs` 保留 180 天，登录失败也会写入（`internal/handler/auth_handler.go:45` 的 `login.failed`），量级不小。
- **触发条件**：管理员在审计页输入关键词（`@keyup.enter`）并按任意非 `created_at` 列排序；分页越深 OFFSET 越大。
- **修复建议**
  1. 给 `audit_logs` 加 `(action, created_at)`、`(module, created_at)`、`(created_at, id)` 复合索引，至少让「按模块/动作 + 时间」组合可用索引。
  2. 关键词搜索改为 `detail LIKE 'kw%'`（前缀）或引入 `FULLTEXT`/外部检索；`username`、`ip` 用等值/前缀匹配。
  3. 深分页改 keyset（`WHERE (created_at, id) < (?, ?)`）而不是 `OFFSET`。

## P6（medium）发送日志列表：新增排序字段无索引，且每页固定 2 条 SQL（COUNT + 页查询，JOIN tasks）

- **位置**
  - `internal/repository/task_log_repo.go:184-237`（`Query`：`:207` 先 `COUNT(*)`，`:225-227` 再取页数据，均 `LEFT JOIN tasks`）
  - `internal/repository/task_log_repo.go:164-180`（`sortColumn`：`status`、`retry_count`、`category` 均非索引列）
  - `internal/database/migrations/001_init.sql`（`task_logs` 仅 `idx_logs_task (task_id)`、`idx_logs_sent_at (sent_at)`）
- **问题机制**：本次把排序能力扩到 `status/retry_count/channel_id/category`。`status`、`retry_count`、`t.category` 无索引，按这些列排序时对命中行做 filesort；`category` 排序还作用在 JOIN 后的 `tasks.category` 上，JOIN 顺序不可控。每条列表请求固定 2 条语句，`COUNT(*)` 与页查询各扫一遍区间（`COUNT` 无法用覆盖索引，因为 `task_logs` 有多个 TEXT 列）。
- **触发条件**：日志页切换排序列（`web/src/views/Logs.vue:322-332` 每次 `onSortChange` 立即 `loadLogs()`）、翻页、快速改筛选（虽有 `loadSeq` 防覆盖，但请求仍全部发出）。
- **修复建议**
  1. 排序白名单只保留有索引的列（`id`、`sent_at`、`task_id`），其余列在 UI 上禁用或改为「仅当前页排序」。
  2. 加 `(status, sent_at)`、`(channel_id, sent_at)`、`(sent_at, id)` 复合索引；`COUNT(*)` 用近似值或缓存（列表页无需精确总数时用 `SQL_CALC_FOUND_ROWS` 替代方案已废弃，建议加「约 N 条」+ 缓存 30 s）。
  3. 把 `tasks.category` 反范式到 `task_logs`（写日志时落 `category` 快照），去掉 JOIN，`category` 排序即可走索引。

## P7（medium）批量启用/删除任务先 `repo.List()` 全表加载

- **位置**
  - `internal/service/task_service.go:234-260`（`BatchToggle`：`:239` `all, err := s.repo.List()`）
  - `internal/service/task_service.go:209-231`（`BatchDelete`：`:214` 同样 `List()`）
  - `internal/repository/task_repo.go:90-92`（`List()` 无分页、无列裁剪，含 `variables`/`receivers`/`channel_ids` 等 JSON 列）
- **问题机制**：为了判断「选中的 id 里哪些是 cron 任务」，把**全部任务**（含 `variables`/`receivers`/`allowed_ips`/`channel_ids` 等 JSON 列的原始字符串）读进内存建 map；批量操作本应只处理选中的几十条。任务数上千时，一次「批量禁用 3 条」= 一次全表 SELECT（返回上千行 × 22 列）+ 上千次对象分配 + 全量 map 构建；多实例部署下这个代价被放大到每个管理员操作。
- **触发条件**：任务数 > 500；管理员在任务页做任意批量操作（启停/改分类/改渠道/删除）。
- **修复建议**
  1. 新增 `TaskRepo.ListCronByIDs(ids)`：`SELECT id, cron_expr FROM tasks WHERE id IN (...) AND trigger_type='cron' AND deleted_at IS NULL`，只取需要的两列。
  2. `BatchToggle` 先落库再按需注册（或直接复用 `Toggle` 的单条路径，N 很小）。
  3. `List()` 增加列裁剪版本（列表接口不需要 `variables`/`allowed_ips` 全文）。

## P8（medium）邮件渠道每个收件人新建一次 SMTP 连接（TCP + TLS + AUTH），多收件人线性放大

- **位置**
  - `internal/service/notification_service.go:108-149`（`sendToChannels`：`:136-140` 对 `receivers` 逐个 `sendOnce`）
  - `internal/channel/email.go:204-236`（`Send` 内 `:212` `dialAndAuth(e.config)`，`:216` `defer client.Close()`）
  - `internal/channel/email.go:47-102`（`dialAndAuth`：`:51` 10 s dial、`:56` TLS 握手、`:97` AUTH 协商）
- **问题机制**：一次任务有 K 个收件人就有 K 次 `dial → (TLS) → EHLO → AUTH → MAIL/RCPT/DATA → QUIT`，连接完全不复用。50 个收件人 ≈ 50 次 TLS 握手 + 50 次认证，单次发送可达数十秒；这段时间占着 1 个 DB 连接池之外的 goroutine，并直接决定 P2 的 worker 占用时长。本次新增的续租心跳（`internal/service/queue.go:183-216`）正是为这种长发送兜底——说明该成本已被感知，但根因（无连接复用）未处理。
- **触发条件**：任务绑定邮件渠道且接收地址较多（≥10）；SMTP 服务器 RTT 较高或需要 TLS 握手（465/STARTTLS）。
- **修复建议**
  1. 同一渠道（同 host/port/账号）一次连接内 `MAIL/RCPT/DATA` 多收件人，最后 `QUIT`；或至少在同一次 `SendTask` 内复用连接（`smtp.Client` 支持多次 `Mail/Rcpt`）。
  2. 加连接池（按渠道 ID 缓存 `*smtp.Client`，带空闲超时与健康检查），并明确重连策略。
  3. 单收件人失败不应中断其余收件人（当前已如此），但要把「已成功列表」持久化，避免 job 级重试把整批重发（见 P14）。

## P9（medium）`QUEUE_CLAIM_TTL` 无校验：配成 0/负数时 `time.NewTicker(0)` 在 goroutine 内 panic，直接崩进程

- **位置**
  - `internal/service/queue.go:187-204`（`process` 内 `:192` `t := time.NewTicker(q.cfg.ClaimTTL / 3)`）
  - `internal/service/queue.go:270-288`（`recoverLoop` 对 `interval < time.Second` 有兜底，形成不一致）
  - `internal/config/config.go:172`（`QUEUE_CLAIM_TTL` 直接 `time.Duration(firstInt(...)) * time.Second`，无下限校验）
- **问题机制**：`time.NewTicker` 对非正周期 panic，而该 ticker 建在 `go func()` 里——**goroutine 内 panic 不会被 `process` 的 `recover` 捕获**，整个进程崩溃。`recoverLoop` 特意做了 `if interval < time.Second` 兜底，说明作者已意识到该风险，但 `process` 漏了。同类风险：`QUEUE_CLAIM_TTL=1` 时每 333 ms 一次 `RenewClaim`，长发送会产生每秒 3 次写放大。
- **触发条件**：运维把 `QUEUE_CLAIM_TTL` 设为 `0`（或负值/拼写错误导致 `firstInt` 回退到 0 的配置），且队列收到第一个 job 时。
- **修复建议**
  1. `config.Load` 对 `QUEUE_CLAIM_TTL` 做下限钳制（如 `<30s → 30s`），`QUEUE_WORKERS`、`QUEUE_POLL_MS`、`QUEUE_MAX_ATTEMPTS` 同理。
  2. `process` 里对 `q.cfg.ClaimTTL/3` 取 `max(..., time.Second)`，或干脆在 `NewQueueService` 归一化一次。
  3. 给 renew goroutine 加 `defer recover()`（兜底不应依赖配置正确）。

## P10（low）前端任务页：整表拉取 + 每次按键全量过滤/排序，比较函数内重复创建 `Intl.Collator`

- **位置**
  - `web/src/views/Tasks.vue:701-721`（`filteredTasks`：每行做 `channels.find` / `templates.find`）
  - `web/src/views/Tasks.vue:725-735`（`useTablePaging` 传入派生 getter）
  - `web/src/views/Tasks.vue:861-873`（`channelNames`/`templateName` 逐行 `find`）
  - `web/src/composables/useTablePaging.ts:34-44`（`sorted`：每次 `[...rows].sort(...)` 全量拷贝排序）
  - `web/src/composables/useTablePaging.ts:63-70`（`compareValues`：`:69` 每次比较都 `String(...).localeCompare(..., 'zh-Hans-CN')`）
- **问题机制**：`keyword` 是 `v-model` 双向绑定，每敲一个字符都会让 `filteredTasks` 产出新数组 → `sorted` 重排全表 → `paged` 重切片。比较函数里没有复用 `Intl.Collator`，而 `localeCompare(..., 'zh-Hans-CN')` 每次调用都会解析 locale。实测 4000 行排序：`localeCompare` 11.1 ms，复用 `Intl.Collator` 6.7 ms（约 1.65×）。同时 `channelNames` 对每行每渠道做线性 `find`，是 O(行 × 渠道)。
- **触发条件**：任务数 > 1000 时在搜索框连续输入；或按渠道/模板列排序。
- **修复建议**
  1. 模块级 `const collator = new Intl.Collator('zh-Hans-CN')`，`compareValues` 用它。
  2. 预建 `channelMap = new Map(channels.map(c => [c.id, c.name]))`、`templateMap`，`channelNames`/`templateName` 改 O(1) 查表。
  3. 搜索输入加 200–300 ms 防抖（`watch(keyword, debounce(...))`），或至少把 `filteredTasks` 的过滤与排序拆开（排序只在 `sortKey` 变化时执行）。

## P11（low）审计页新增排序但缺请求序号竞态保护（其它列表页已加）

- **位置**
  - `web/src/views/Audit.vue:185-195`（`onSortChange` 直接 `load()`）
  - `web/src/views/Audit.vue:336-360`（`load` 无 `loadSeq`）
  - 对照实现：`web/src/views/Logs.vue:538-576`、`web/src/views/Dashboard.vue:261-291`
- **问题机制**：本次给审计页加了后端排序，`onSortChange` 与筛选/翻页/刷新都会触发 `load()`，但没有任何「只接受最新响应」的守卫。快速点两次表头或边输入边翻页时，先发的请求可能后到并覆盖 `items`/`total`，导致列表与当前排序/筛选永久不一致（用户看到错序数据，且会误以为后端排序失效而反复点击，进一步放大请求量）。Logs/Dashboard 在本次已修，审计页漏改，属同一类缺陷的不一致。
- **触发条件**：慢网络/后端 `audit_logs` 慢查询（见 P5）时快速切换排序或筛选。
- **修复建议**：把 `Logs.vue` 的 `loadSeq` 模式复制到 `Audit.vue`（`const seq = ++loadSeq`，写入前 `if (seq !== loadSeq) return`，`finally` 里同样判断再清 `loading`）。

## P12（low）仪表盘每次加载 5 条聚合 SQL 且无缓存，环形图每次 `load` 渲染两遍

- **位置**
  - `web/src/views/Dashboard.vue:264-291`（`Promise.all` 4 个接口）
  - `internal/service/dashboard_service.go:51-65`（`StatsRange` = `CountByRange` + `CountDistinctByRange` 2 条）
  - `internal/repository/task_log_repo.go:241-269`（`CountByDay` 的 `GROUP BY DATE_FORMAT(sent_at,...)`、`COUNT(DISTINCT ...)`）
  - `web/src/views/Dashboard.vue:282`（`renderDonut()`）+ `:303`（`watch(() => stats, renderDonut, { deep: true })`）
- **问题机制**：一次仪表盘打开 = 5 条对 `task_logs` 区间的聚合查询（2+1+1+1），默认近 7 天，90 天保留期内高频任务下每次都要扫区间；没有短 TTL 缓存，切页/刷新/改日期全部重算。`COUNT(DISTINCT task_id)`、`COUNT(DISTINCT channel_id)` 各扫一遍同一区间（可合并进 `CountByRange` 的一条语句）。前端 `load()` 里显式 `renderDonut()` 之后，`Object.assign(stats, ...)` 又会触发 `deep` watch 再渲染一次，等于每次加载 `setOption` 两遍（echarts 全量重排 + 动画重启）。
- **触发条件**：默认落地页即 `/dashboard`（`web/src/router/index.ts:12`）；频繁切换 7/14/30 天快捷项。
- **修复建议**
  1. 合并 `StatsRange` 为 1 条 SQL（`COUNT(*)`、`SUM(status=...)`、`COUNT(DISTINCT task_id)`、`COUNT(DISTINCT channel_id)` 同句完成）。
  2. 后端对 `(from,to)` 维度加 10–30 s 内存缓存（或 `singleflight` 合并并发请求），仪表盘是只读统计，容忍轻微陈旧。
  3. 去掉 `deep` watch 或去掉显式 `renderDonut()`（二者留一），并给 `renderDonut` 加 `notMerge: false` 复用实例。

## P13（low）模板编辑页实时预览：每次按键全量 `marked.parse` + `DOMPurify.sanitize`

- **位置**
  - `web/src/views/Templates.vue:412-419`（`previewMarkdown` computed 依赖 `form.content_md`）
  - `web/src/components/MarkdownPreview.vue:29-31`（`html = computed(() => DOMPurify.sanitize(marked.parse(...)))`）
- **问题机制**：`form.content_md` 是 `v-model` 文本域，每敲一个字符都会让 `previewMarkdown` 变化，进而触发一次完整 Markdown 解析 + DOM 清洗 + `v-html` 重设 innerHTML。模板正文通常几百到几千字，长模板下连续输入会明显掉帧；`DOMPurify` 对整段 HTML 的遍历是主要成本。
- **触发条件**：编辑较长模板正文（>2 KB）时连续输入/粘贴大段内容。
- **修复建议**：对 `previewMarkdown` 的输入做 150–300 ms 防抖（`watch` + `setTimeout`，或 `@input` 防抖后写入一个 `debouncedContent` ref）；或输入时只显示源码、失焦/点击「预览」再渲染。

## P14（low）job 级重试会重发已成功的渠道/收件人（放大系数 = MaxAttempts × 目标数）

- **位置**
  - `internal/service/notification_service.go:105-149`（注释已声明「重试会重发成功的渠道，这是 job 粒度重试的已知取舍」）
  - `internal/repository/send_job_repo.go:146-167`（失败回 `pending` + 退避，最多 `MaxAttempts=3`）
- **问题机制**：一个任务绑 3 个渠道、10 个收件人，第 1 个渠道失败 → 整 job 重试 → 已成功的渠道/收件人被重复投递，最坏 3 次 × 13 个目标 = 39 次发送，其中 36 次是重复。对接收方是骚扰，对服务是 3 倍的第三方调用配额与 SMTP 连接成本。
- **触发条件**：多渠道/多收件人任务中任一目标失败（第三方限流、单个邮箱地址退信）。
- **修复建议**：把「发送单元」从 job 粒度下沉到（渠道 × 收件人）粒度：`send_jobs` 拆子任务，或成功目标写 `task_logs` 后重试时跳过已成功组合（按 `(task_id, channel_id, address)` 查最近一次成功）。短期至少把「渠道停用/模板停用」这类永久失败与「网络失败」区分开，避免无意义重试。

## P15（low）迁移 `GET_LOCK` 期间持有专用连接最长 120 s，慢 ALTER 会让其它实例启动串行阻塞

- **位置**
  - `internal/database/db.go:45-103`（`:54` `GET_LOCK('notice_migrate', 120)`，`:60` release，`:89-97` 按 `;` 拆分逐条执行）
  - `internal/database/migrations/019_users_username_active_unique_fix.sql`（生成列 + 删/建唯一索引，需重建表）
  - `internal/database/migrations/018_tasks_cron_expr_width.sql`（`MODIFY COLUMN` 需重建表）
- **问题机制**：加锁到释放之间执行**全部**待应用迁移，且每条语句用同一连接串行执行。`019`/`018` 这类 `ALTER` 在中等规模表上可跑数十秒；此期间其它副本的启动流程会阻塞在 `GET_LOCK` 上（最长 120 s），超过即 `log.Fatalf` 退出——滚动发布时表现为「新 Pod 启动失败重试」。锁本身是必要的（防重复 ALTER），问题在「持锁粒度过大 + 启动路径无重试」。
- **触发条件**：多副本同时滚动升级且本次发布含 DDL 迁移；表行数较大。
- **修复建议**
  1. 迁移按文件粒度加锁/释放（每个文件独立 `GET_LOCK`），缩短单次持锁时长。
  2. 拿不到锁时不要 `Fatal`，改为指数退避重试（如 3 次 × 30 s）再退出。
  3. 把 DDL 迁移与启动解耦（独立 `migrate` 命令/Job），应用启动只校验 `schema_migrations` 是否最新。

---

## 附：既有问题（v2.0.0 已存在，本次范围未修改，供合并报告参考）

1. **首屏体积**：本地 `web/dist` 实测 `index-*.js` 1225 KB、`Dashboard-*.js` 1023 KB（整包 `import * as echarts`，且默认落地页就是 `/dashboard`）、`index-*.css` 367 KB（`main.ts` 全量 `app.use(ElementPlus)`）。首屏约 2.25 MB JS + 367 KB CSS。建议 echarts 按需引入（`echarts/core` + 折线/饼图）与 Element Plus 按需自动导入；本次未改动，故不计入正式发现。
2. **`middleware.Auth` 每请求回查用户**（`internal/middleware/auth.go:32-41`）：每个业务请求 1 条 `SELECT users`。本次只加了 `claims.TwoFA` 判断，未改变该开销；高 QPS 下建议 5–10 s 本地缓存（配合 `session_revoked_at` 失效广播）。
3. **`/categories/unused` 的 N+1**（`internal/repository/category_repo.go:129-149`）：每个分类 1 条含 3 个子查询的 SQL；分类页每次加载（`web/src/views/Categories.vue:272-304`，同时并发 4 个列表接口）都会打一遍。建议改成 1 条 `UNION ALL` + `GROUP BY`。

---

## 该视角最高优先级结论（3–5 条）

1. **农历 `Next()` 是本次最大的性能地雷**：`@lunar term` 单次 36–336 ms、10–92 MB，而它在 robfig **单 goroutine** run loop 里同步执行，会阻塞同一时刻所有任务的派发；启动与每次保存还会重复计算。更糟的是前端内置快捷项 `@lunar term 中秋` 与节日表键 `中秋节` 不匹配，**该任务永不触发，且每次保存/启动都跑满 8 年 × 370 天的最坏路径（336 ms / 92 MB）**。→ 先修名字与解析期校验，再做 `NewLunarYear` 复用 + `Next` 缓存 + 移出 run loop。
2. **发送队列吞吐被硬编码为 `Workers/PollInterval`（默认 4 job/s）**，且 worker 忙时 tick 合并丢弃、每个 job 还要 3 条 SQL 认领。Webhook 突发或 SMTP 变慢时队列只会越堆越深（`send_jobs` 无 `(status,id)` 索引又让认领退化为 filesort）。→ 批量认领 + 完成即再取 + 索引，是让这套队列能承载真实负载的前提。
3. **CSV 导出的「300 s 超时」是无效修复**：服务端 `WriteTimeout=20 s` 会先掐断连接，同时 `ListExportRows` 把最多 10 万行（含 TEXT 正文）整体物化进内存。→ 改流式写 + handler 级 `SetWriteDeadline` + 降低行数上限，否则大导出既会失败又会造成内存尖峰。
4. **列表类接口普遍是「先 COUNT 再分页」+ 新增排序列无索引**：审计日志（`LIKE '%kw%'` + 新排序）、发送日志（`status/retry_count/category` 排序、JOIN `tasks`）都会退化为全表扫描 + filesort。→ 补复合索引、去掉 JOIN（`category` 反范式）、排序白名单只保留有索引列。
5. **前端已加竞态保护，但只加了一半**：Logs/Dashboard 有 `loadSeq`，审计页（本次新增排序）没有；任务页仍是「整表拉取 + 每次按键全量重排」，且比较函数每次新建 `Intl.Collator`（实测慢 1.65×）。→ 复制 `loadSeq` 模式、预建查找表 + 复用 `Collator` + 输入防抖，成本极低、收益直接。
