# Notice Service · 发送日志增加分类（筛选 + 列 + 详情 + CSV 导出）

> 日期：2026-08-28
> 状态：已批准（brainstorming 会话；关键决策「分类语义」用户选定：跟随任务当前分类）
> 修订：v1

## 1. 背景与目标

### 1.1 现状

| # | 需求 | 现状 | 证据 |
|---|------|------|------|
| 1 | 发送日志筛选支持分类 | 筛选仅有 任务/状态/关键词/日期 | `web/src/views/Logs.vue` filters 区 |
| 2 | 发送日志列表展示分类列 | 列仅有 ID/主题/任务/渠道/状态/时间/操作 | `web/src/views/Logs.vue` 表格 |
| 3 | 分类是渠道/模板/任务共享池 | `task_logs` 表无 category 字段，日志分类只能经任务间接得到 | `internal/database/migrations/015_categories.sql`、`internal/model/models.go` |

### 1.2 目标

- 发送日志页：筛选区增加「分类」下拉（共享分类池），表格增加「分类」列（支持后端排序）。
- 日志详情页：描述区展示分类。
- CSV 导出：增加 `category` 列，且与列表共用同一套筛选（含分类）。
- 语义：**日志的分类 = 所属任务的当前分类**（读取时 JOIN，不落库、不改表结构）。

### 1.3 非目标（YAGNI）

- 不给 `task_logs` 加 category 列、不做数据迁移（用户已选定「跟随任务当前分类」语义）。
- 仪表盘不做按分类的统计维度（新增分析维度，另起迭代）。
- 任务详情日志（`/tasks/:id/logs`）不加分类（上下文已是单任务）。
- 审计日志、备份导入导出不涉及（备份分类已由 `EnsureExists` 覆盖）。

## 2. 关键决策：分类语义

| 方案 | 说明 | 取舍 |
|------|------|------|
| **A. 跟随任务当前分类（已选）** | 查询时 `LEFT JOIN tasks` 取 `t.category` | 无迁移/回填；分类改名、删除保护自动生效（rename 事务已同步 tasks）；日志永远有有效分类，无孤儿数据。代价：任务改分类后历史日志归入新分类 |
| B. 发送时快照（加列） | `task_logs` 加 category 列，发送时写入 | 历史语义精确；但需迁移回填 + 4 处写入路径改动，且分类改名若不同步 task_logs 会产生孤儿分类名，同步则重命名需全表更新日志表 |

选 A 的核心理由：分类在系统中是「实时引用」语义（渠道/模板/任务均如此，rename 同步、引用中禁删），日志跟随任务保持一致，避免悬空分类名破坏筛选下拉。

## 3. 总体方案

### 3.1 后端 — 模型与数据访问

**`internal/model/models.go`**
- `TaskLog` 增加 `Category string \`json:"category"\``（仅读路径填充；`Create` 不写该字段）。

**`internal/repository/task_log_repo.go`**
- `LogFilter` 增加 `Category string`。
- `Query` 与 `GetByID` 改写为：
  ```sql
  SELECT tl.id, tl.task_id, ..., tl.sent_at, COALESCE(t.category,'default') AS category
  FROM task_logs tl
  LEFT JOIN tasks t ON t.id = tl.task_id
  ```
  - 筛选：`AND t.category=?`（`LogFilter.Category != ""` 时）；`Query` 的 `COUNT(*)` 计数查询同样需要带该 JOIN，否则分类筛选时列名解析报错。
  - 排序白名单新增 `category → t.category`；JOIN 后所有列限定 `tl.`/`t.` 前缀（`id` 在两表均有，否则歧义报错）。
  - `Query`/`GetByID` 使用独立的带 category 的 SELECT 清单与扫描函数；`taskLogCols` 常量与 `scanLogs` 保持原样，继续服务 `ListByTask`/`Recent`/`Create` 等不 JOIN 的路径（这些路径的 `Category` 留空）。
- `ListExportRows`（已有 `LEFT JOIN tasks t`）SELECT 增加 `COALESCE(t.category,'default')`，`LogExportRow` 增加 `Category string`。

### 3.2 后端 — Handler / Service

- `internal/handler/task_handler.go` `logFilterFromQuery` 解析 `category` 查询参数（trim 后透传）→ `LogsAll` 与 `ExportLogs` 共用，列表与导出筛选自动一致。
- `ExportLogs` CSV 表头与每行输出增加 `category`（经 `csvSafe`）。
- Service 层 `QueryLogs`/`GetLog`/`ExportLogRows` 签名不变，透传 filter。

### 3.3 前端

**`web/src/views/Logs.vue`**
- 筛选区：任务下拉后新增「分类」`el-select`（`categoryApi.list()`，clearable，placeholder「全部分类」，选项 label/value = 分类名；与 Tasks/Channels/Templates 页同款交互）。
- 表格：渠道列后新增「分类」列，`el-tag effect="plain" size="small"`，样式复用 Tasks.vue 的 category-tag；`sortable="custom"` `prop="category"` 走后端排序。
- `categoryFilter` 纳入 watch（变化回第一页重查）、`loadLogs`/`exportCsv` 的参数透传、关键词二次过滤（分类名参与匹配）。
- `LogRow` 接口增加 `category?: string`。

**`web/src/views/LogDetail.vue`**
- 描述区（任务/渠道行之后）增加分类行，`el-tag` plain 展示，`LogDetail` 接口增加 `category?: string`。

**i18n（`web/src/locales/zh-CN.json` / `en-US.json`）**
- `logs` 段新增 `category`（分类 / Category）、`allCategories`（全部分类 / All Categories）。

### 3.4 Swagger 注解

- `LogsAll`/`ExportLogs` 注解补 `@Param category` 说明。

## 4. 错误处理

- 无新增错误路径；任务缺失（仅软删除，理论上 JOIN 不到）时 `COALESCE` 兜底 `default`。
- 分类筛选值来自共享分类池下拉，后端仅做字符串等值匹配，无注入面（走参数占位符）。

## 5. 测试

- **repo**：`Query` 分类筛选命中/不命中、JOIN 后按 `t.category` 排序、默认 `id` 倒序分页稳定；`GetByID` 返回 category。
- **handler**：`category` 参数解析进入 filter；导出 CSV 表头含 `category` 列且行值正确（沿用 `log_export_test.go` / `export_test.go` 模式）。
- **前端**：`api/index.test.ts` 补 `logApi.query`/`export` 透传 `category` 参数断言。
