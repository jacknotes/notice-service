# 产品/体验类「立即」条目修复记录（B-1 / H-5 / M-12）

对应统一报告：[`docs/reviews/v2.0.0-post-review.md`](../reviews/v2.0.0-post-review.md) §4.1「立即（发布前必须完成）」中的
第 3、5、7 项，以及 §2.1 / §2.7 / §2.16（M-8 与 B-1 同一修点）/ §2.20。

修复范围仅限以下文件（不改动本条之外的业务逻辑）：

- `internal/scheduler/lunar_parser.go`、`internal/scheduler/lunar_schedule.go`、`internal/scheduler/lunar_test.go`
- `internal/service/export_service.go`、`internal/service/export_service_test.go`
- `web/src/views/Tasks.vue`
- `CHANGELOG.md`

---

## B-1 农历 `term` 节日名静默失效（blocker，报告 §2.1，性能 P1 同根因）

### 问题

`term` 的名字既不在 `lunarFestivals`（键为**全称**「中秋节」「元宵节」「端午节」…）也不是 24 节气之一时，
`parseLunarSchedule` 只校验「名字非空 + `HH:MM` 合法」，于是：

1. **解析成功、保存返回 200**（`task_service.go:420` 的预检只判断 `Parse` 是否报错）；
2. `Next()` 走 `termCandidatesInYear` 的**节气回退分支**，逐日扫描 8 年 × 370 天仍匹配不到 → 返回**零值**；
3. 任务永不触发，列表把空 `next_run_at` 渲染成中性的 `—`，与「api 任务 / 已停用任务」视觉一致 → **零可观测信号**；
4. 性能代价：实测单次 `Next()` 对未知名字 **336 ms / 92 MB**，且 `robfig/cron` run loop 是单 goroutine，
   会阻塞所有任务派发（报告 §2.1 表格）。

前端快捷项正是 `@lunar term 中秋 09:00`（短名），注释示例也写着 `春节,中秋` —— **用户按产品自身引导操作即漏发通知**。
列表写法 `term 春节,中秋` 更糟：春节命中、中秋静默丢失，属「部分生效」，比全零更难发现。

### 改法

1. **解析期白名单校验**（一处修复同时消灭最坏路径与静默失效）
   - `lunar_schedule.go`：新增 `lunarJieQiNames`（24 节气静态表）、`lunarJieQiSet`（O(1) 查找集，由前者派生避免两处不同步）、
     `isKnownTermName`、`knownTermNamesHint`（生成可用值提示）、`validateTermName`。
   - `lunar_parser.go`：`term` 分支对**每个**逗号项调用 `validateTermName`，未知名直接返回错误并列出可用值。
     - 错误文案：`未知的节气/节日名 "中秋"（农历节日请用全称，如「中秋节」而非「中秋」）；可用节日：七夕、中秋节、元宵节、除夕…；可用节气：冬至、小寒…`
2. **前端快捷项改全称**：`Tasks.vue` 的 `lunarPresets` 中 `@lunar term 中秋 09:00` → `@lunar term 中秋节 09:00`；
   同文件其余快捷项（`monthly 1` / `monthly 15` / `yearly 1 1` / `term 清明`）经核对均合法，无需改动。
3. **注释示例改全称**：`lunar_parser.go` 文件头注释中 `春节,中秋` → `春节,中秋节`，并补充「节日名必须用全称」说明；
   `term` 示例行改为 `@lunar term 清明 09:00 / 中秋节 09:00`。
4. **M-8（§2.16）合并修复**：白名单校验 + 可用值提示即 M-8 的修复建议（「输入非法节日名时保存即报错并列出可用名字」），
   同一修点，不额外改动。

### 文件:行号

| 文件 | 位置 | 改动 |
|---|---|---|
| `internal/scheduler/lunar_schedule.go` | `:114-128`（`lunarFestivals`） | 补注「全称为合法值，短名在解析期被拒」 |
| `internal/scheduler/lunar_schedule.go` | `:130-177`（`lunarJieQiNames` / `lunarJieQiSet` / `isKnownTermName` / `knownTermNamesHint` / `validateTermName`） | 白名单与错误提示 |
| `internal/scheduler/lunar_parser.go` | 文件头注释 `:13-24` | 示例改全称 + 说明 |
| `internal/scheduler/lunar_parser.go` | `:160-168`（`parseLunarSchedule` 的 `term` 分支） | 逐项 `validateTermName`，未知名报错 |
| `web/src/views/Tasks.vue` | `:807-815`（`lunarPresets`，中秋项在 `:814`） | `term 中秋` → `term 中秋节` |

### 验证证据

```
$ go test ./internal/scheduler/ -run "Lunar" -v
--- PASS: TestLunarTermRejectsShortFestivalName     # term 中秋/元宵/端午/不存在/春节,中秋 全部报错，且错误含「中秋节」
--- PASS: TestLunarTermAcceptsFullFestivalName      # term 中秋节/元宵节/端午节/春节 Next() 命中真实农历日期
--- PASS: TestLunarTermUnknownNameFailsFast         # 未知名解析耗时 < 50ms（旧路径 Next() 336ms）
--- PASS: TestLunarTermMidAutumn / TestLunarTermSpringFestival / TestLunarTermMultiList  # 既有用例全绿
ok  	notice-service/internal/scheduler	0.073s
```

---

## B-1 性能联动：`Next()` 分配与最坏路径

### 问题

报告 §2.1 / §2.2 修复建议第 5 条：

- `lunarDayInYear` 内 `calendar.NewLunarYear(year)` 对**每个（月,日）组合**新建一次，而 `NewLunarYear` 要算
  整年节气（26 项）与合朔表（16 项）；
- `termCandidatesInYear` 每天调用**两次** `GetLunar()`（`solar.GetLunar()` 各建一次 `LunarYear`），370 天 × 2 次；
- 未知名字会跑满 8 年 × 370 天才返回零值。

### 改法

1. **`NewLunarYear` 提到年份循环外复用**：`candidatesForYear` 按年构造一次 `*calendar.LunarYear`，传给
   `lunarDayInYear` / `termCandidatesInYear`（签名改为接收 `ly`）。`term` 分支进一步惰性构造——纯节气任务不构造。
2. **`termCandidatesInYear` 去掉重复 `GetLunar()`**：循环外取一次 `lun := solar.GetLunar()`，循环内复用 `lun`，
   推进用 `next := lun.Next(1)`（其内部就是 `solar.NextDay(1).GetLunar()`，与旧写法等价但只建一次）。
3. **`lunarDayInYear` 省掉 `NewLunarFromYmd`**：直接由「该月初一的儒略日 + 偏移」求阳历日
   （`calendar.NewSolarFromJulianDay(lm.GetFirstJulianDay() + float64(lunarDay-1))`），与 `NewLunar` 内部算法一致，
   但不再构造 `LunarYear`/`Lunar`（后者同样会重算整年节气表）。
4. **未知名不再跑满最坏路径**：白名单校验在解析期完成，`Next()` 里不存在「注定匹配不到」的名字。

### 文件:行号

| 文件 | 位置 | 改动 |
|---|---|---|
| `internal/scheduler/lunar_schedule.go` | `:47-88`（`candidatesForYear`） | 按年构造一次 `LunarYear` 并传入；`term` 分支惰性构造 |
| `internal/scheduler/lunar_schedule.go` | `:91-112`（`lunarDayInYear`） | 签名改收 `ly`；用儒略日偏移代替 `NewLunarFromYmd` |
| `internal/scheduler/lunar_schedule.go` | `:179-211`（`termCandidatesInYear`） | 签名改收 `ly`；循环内复用同一个 `Lunar` 对象 |

### 验证证据

**实测基准**（同一台机器 i5-6500，`go test -bench`，临时基准文件已删除）：

| 场景 | 改前（v2.0.0..HEAD 原实现） | 改后 | 变化 |
|---|---|---|---|
| `@lunar term 中秋 09:00`（**未知名**）`Next()` | **441 ms / 92.3 MB / 2.35M allocs** | **解析期即报错**（不再进入 `Next()`） | 最坏路径消失 |
| `@lunar term 清明 09:00`（节气）`Next()` | 36.8 ms / 10.2 MB / 260k allocs | 14.0 ms / 3.43 MB / 87k allocs | **−62% 耗时 / −66% 内存 / −66% 分配** |
| `@lunar term 中秋节 09:00`（节日）`Next()` | 33.2 µs / 9.7 KB / 266 allocs | 0.38 µs / 96 B / 3 allocs | **−99% 耗时 / −99% 分配** |
| `@lunar monthly 15 09:00` | 1.85 ms / 114 KB / 2879 allocs | 2.07 µs / 1.3 KB / 17 allocs | **−99% 耗时 / −99% 分配** |

（未知名场景改前需跑满 8 年 × 370 天；改后名字在 `Parse` 阶段就被白名单拒绝，一次 `Next()` 都不会发生。）

```
$ go test ./internal/scheduler/...        # 全部既有农历用例（monthly/yearly/term/last/列表/区间）结果与改前一致
ok  	notice-service/internal/scheduler	1.620s
```

行为等价性由既有 20 个农历用例覆盖（`TestLunarMonthly15`、`TestLunarYearlyLastChuxi`、`TestLunarTermQingming`、
`TestLunarTermMultiList` 等，均断言精确到日的触发时间），未出现任何回归。

> 补充说明：`calendar.NewLunarYear` 内部有单元素 `CACHE_YEAR`（只缓存最近一年），因此「按年复用」的真实收益
> 主要来自**同一 `LunarYear` 内多次 `GetMonth`/月份查询**与**去掉 `NewLunarFromYmd` 的重复整年计算**，
> 以及节气分支去掉每日两次 `GetLunar()`——上表数字即为合并效果。

---

## H-5 备份导入丢失模板 `enabled` 状态（high，报告 §2.7）

### 问题

`internal/service/export_service.go` 的 `Import` 构造 `model.Template` 时未带 `Enabled`：

```go
nt := &model.Template{Name: t.Name, Subject: t.Subject, ContentMD: t.ContentMD, Variables: t.Variables}
```

而同文件构造渠道（`:106`）与任务（`:142`）都带了 `Enabled`。`template_repo.Create` 的 INSERT **显式写** `enabled` 列
（`INSERT INTO templates (user_id, name, category, enabled, subject, content_md, variables) ...`），
因此 Go 零值 `false` 直接落库，**不经过**列默认值 `NOT NULL DEFAULT 1`（迁移 016 在此路径下不生效）。

后果：**所有导入的模板一律 `enabled=0`（停用）**，与导出前的真实状态无关——不是「状态漂移」而是「状态归零」。
叠加本批新增的「模板停用时禁止发送」（`9a21d0a`，对应正文 M-7），迁移到新环境后**所有任务静默停发**，
而任务列表仍显示「启用」、`next_run_at` 照常计算，导入摘要只显示 `templates_created: N`，无任何告警。

### 改法

`export_service.go` 的模板导入分支补 `Enabled: t.Enabled`（一行修复）：

```go
nt := &model.Template{Name: t.Name, Subject: t.Subject, ContentMD: t.ContentMD,
    Variables: t.Variables, Enabled: t.Enabled}
```

并补回归测试（新增文件 `internal/service/export_service_test.go`）：

- `TestExportImportTemplateEnabledRoundtrip`：导出含「1 启用 + 1 停用」模板 → 走 JSON 序列化/反序列化 →
  删除原记录（模拟导入到新环境）→ `Import` → 逐个断言 `enabled` 一致；
- `TestImportTemplateEnabledWrittenToDB`：直接 `SELECT enabled FROM templates` 断言落库值，
  确保修复作用于 `template_repo` 的显式 INSERT 而非仅内存对象。

### 文件:行号

| 文件 | 位置 | 改动 |
|---|---|---|
| `internal/service/export_service.go` | `Import` 的模板分支（`:123-131`） | 补 `Enabled: t.Enabled` + 说明注释 |
| `internal/service/export_service_test.go` | 新增 | 两条状态保真回归测试 |

### 验证证据

**修复前**（临时移除 `Enabled: t.Enabled` 后运行）：

```
--- FAIL: TestExportImportTemplateEnabledRoundtrip (0.04s)
    export_service_test.go:117: imported template "exp_on_..." enabled = false, want true（H-5 回归：Enabled 未被带入）
--- FAIL: TestImportTemplateEnabledWrittenToDB (0.04s)
    export_service_test.go:153: templates.enabled in DB = 0, want 1（导入丢失启用状态）
FAIL
```

**修复后**：

```
$ go test ./internal/service/ -run "Export|Import" -v
--- PASS: TestExportImportTemplateEnabledRoundtrip (0.06s)
--- PASS: TestImportTemplateEnabledWrittenToDB (0.03s)
ok  	notice-service/internal/service	0.098s
```

---

## M-12 CHANGELOG `[Unreleased]` 为空（medium，报告 §2.20）

### 问题

`CHANGELOG.md:7` 的 `## [Unreleased]` 下为空，第 9 行直接是 `## [2.0.0]`。`v2.0.0..HEAD` 的 52 个提交
（`STRICT_SECRETS`、MySQL 8.0、登录态窗口化、五渠道排版统一、农历增强、PushPlus ClawBot 等）**零记录**。
自托管用户判断「要不要升级、升级要注意什么」完全依赖 CHANGELOG，空白意味着只能盲升；
其中至少「`STRICT_SECRETS` 拒绝启动」与「登录态窗口化」会让用户升级后**立刻遇到启动失败或行为变化**。

### 改法

补写 `[Unreleased]` 全节，分 `安全加固` / `新增` / `变更` / **`升级注意`**（单列一节）四部分：

- 安全加固：TOTP 加密与防重放、`STRICT_SECRETS`、Swagger 默认关闭、登录防枚举与复合桶锁定、
  重置令牌哈希化、Webhook 防重放、SMTP 认证协商、审计 IP 与心跳清理；
- 新增：农历 cron 增强（列表/区间/`last`/`term` 节日全称/年份字段）、PushPlus ClawBot 子渠道、五渠道排版统一、
  管理列表派生字段排序、任务页交互、`deploy/publish.sh`；
- 变更：登录态改窗口级、默认 MySQL 8.0 与非 root 容器、cron 保存前置校验、`next_run_at` 统一计算、
  队列 CAS 与续租、调度去重键用 DB 时钟、`@lunar` 时刻越界拒绝、**`@lunar term` 节日名白名单校验**、前端 `vue-tsc`；
- **升级注意**（8 条）：可能启动失败（`STRICT_SECRETS`）、登录态语义变化、MySQL 8.0 与老引擎、
  已发消息排版会变、农历节日名必须用全称、`/swagger` 与 `/metrics` 默认关闭、镜像 tag 升级方式、
  备份导入后请复查模板 `enabled`（与本次 H-5 修复对应）。

### 文件:行号

| 文件 | 位置 | 改动 |
|---|---|---|
| `CHANGELOG.md` | `:7-9`（`## [Unreleased]` 与 `## [2.0.0]` 之间） | 新增 4 个小节，含「升级注意」 |

---

## 回归测试清单（已纳入测试套件）

| 测试 | 文件 | 断言 |
|---|---|---|
| `TestLunarTermRejectsShortFestivalName` | `internal/scheduler/lunar_test.go` | `term 中秋` / `元宵` / `端午` / `不存在` / `春节,中秋` **必须报错**，且错误含可用值 |
| `TestLunarTermAcceptsFullFestivalName` | `internal/scheduler/lunar_test.go` | `term 中秋节` / `元宵节` / `端午节` / `春节` **必须有效**且命中真实农历日期 |
| `TestLunarTermUnknownNameFailsFast` | `internal/scheduler/lunar_test.go` | 未知名解析耗时 < 50 ms（不进 8 年 × 370 天路径） |
| `TestExportImportTemplateEnabledRoundtrip` | `internal/service/export_service_test.go` | 导出 1 启用 + 1 停用 → 导入后逐个状态一致 |
| `TestImportTemplateEnabledWrittenToDB` | `internal/service/export_service_test.go` | 导入后 `templates.enabled` 落库值为 1 |

对应报告 §4.4「建议纳入 CI 的防回归项」中的两条：**农历解析白名单测试**、**导出/导入状态保真测试**。

---

## 验证命令与结果

```bash
export PATH=$PATH:/usr/local/go/bin
cd /mnt/f/project/notice-service
go build ./...                                              # exit 0
go vet ./...                                                # exit 0
go test ./internal/scheduler/... ./internal/service/...      # ok
cd web && npx vue-tsc --noEmit                               # exit 0
```

前端改动仅为 `lunarPresets` 数组内的字符串字面量，`labelKey` 与结构不变，`vue-tsc --noEmit` 通过。

> **测试环境注意**：`internal/service` 的测试共用本机 MySQL 库 `notice_service_test`（`helpers_test.go` 直连
> `127.0.0.1:3306`），并行运行多个 `go test ./internal/service/...` 进程会互相删除/软删对方的种子数据。
> 本次验证时曾出现 `TestUserServiceDisableEnable` / `TestUserServiceDefaultAdminProtected` 因**另一进程同时跑测试**
> 而报「内置 admin 查不到」，单独重跑即通过（与本条改动无关）。串行验证时全绿。

---

## 未在本条范围内（已在报告中登记，留给后续批次）

- **M-9**（§2.17）登录态窗口化的 README 升级小节提示、被 `clearSession()` 登出时的用户提示 —— 属报告 §4.2 第 18 项
  「与 M-12 同一批次」，但本条 in-scope 路径不含 `README.md` / `web/src/utils/session.ts`，故仅在 CHANGELOG 中记录行为变化。
- **M-10 / M-11**（§2.18 / §2.19）`next_run_at` 空值的警示渲染与「无下次触发」保存警告 —— 报告 §4.2 第 16 项
  「失效可见性三件套」，涉及 `task_service.go` 响应体与列表渲染，超出本条范围。
- **`web/src/locales/*.json` 的 `lunarHint` 补充节日全称**（报告 §2.1 第 6 点提到的 `zh-CN.json:422`）——
  不在本条 in-scope 路径内；快捷项按钮文案（`lunarMidAutumn`）保持不变即可，因表达式已改为全称。
