# 修复正确性审查报告（t8）—— 7 个立即条目闭环 + 回归风险

> 审查者：fix-reviewer（t8）
> 审查对象：统一报告 `docs/reviews/v2.0.0-post-review.md` §4.1 的 7 个立即条目
> 修复说明：`docs/fixes/security-fixes.md`（B-2/H-1/H-2/M-2）、`docs/fixes/product-fixes.md`（B-1/H-5/M-12）
> 验证报告：`docs/fixes/wsl-verification.md`（t7，串行口径）
> 审查方法：逐条读码核对我方修改 + grep 全部 `totp_secret` 写入点 + 与验证报告探针结论逐项对照。
> **结论：`pass`**（7 项逐条达标；两个 blocker 全部闭环；无阻断性问题）。

---

## 一、结论

| 维度 | 判定 | 说明 |
|---|---|---|
| 修点与报告一一对应 | ✅ | 7 项逐一读码核对，无「修了没修对」项 |
| B-2 三处修点 + 无遗漏明文路径 | ✅ | ① 加密写入 ② 升级清明文 ③ 停用清密文 全到位；grep 确认生产路径无遗漏明文落库入口 |
| H-2 真正阻止明文降级 | ✅ | 不是「只加日志」：非 TLS 未 `allow_insecure` 时 LOGIN/PLAIN 直接**拒绝**（读码确认），而非仅告警放行 |
| B-1 解析期拦截短名 | ✅ | `parseLunarSchedule` 的 `term` 分支逐项白名单校验，未知短名 Parse **报错**；UI 快捷项与注释已改全称（不止 UI 层） |
| 回归风险清单 | ✅ | fail-closed 误伤 / 黑名单误拒 / 白名单兼容性 均已逐条评估（见第四节） |
| 测试质量 | ✅ | 新增回归测试均能捕获原缺陷（反向验证 + 断言非恒真），见第三节 |
| 未验证/推断项处理 | ✅ | 端到端 SMTP 探针用本地假服务器实测，探针结论与源码一致；标注「无法验证」/估算项合理，未把推断写成已验证 |
| 范围控制 | ✅ | 业务改动全部落在 7 项 {B-1,B-2,H-1,H-2,H-5,M-2,M-12}+ 必要接线内；唯一超出的是**测试基建**（见 §5） |

契约 verify（WSL）：`go build ./...` ✅ / `go vet ./...` ✅（EXIT=0，本次审查重新执行）。

---

## 二、逐条核对表（7 项 × 是否达标 × 证据）

### 1. 【B-2】TOTP 加密闭环 —— ✅ 达标

**三处修点逐一确认（§2.2 报告强调「修其一不足以闭环」）：**

| 报告要求的修点 | 实施 | 证据 |
|---|---|---|
| ① `ForceEnable2FA` 加密写入 | 完成 | `internal/service/user_service.go:305-334`：`cipher==nil` 时 fail-closed（`:307`）→ `EncryptString`（`:316`）→ `SetTOTP(userID, secretEnc, "", ...)`（`:327`）。`UserService` 注入 `cipher` 并加 `SetCipher`（`:38,:46`） |
| ② 升级路径清明文列 | 完成 | `internal/repository/user_repo.go:248-252`：`SetTOTPSecretEnc` SQL 改为 `UPDATE users SET totp_secret_enc=?, totp_secret=NULL WHERE ...` |
| ③ `DisableTOTP` 清密文列 | 完成 | `internal/repository/user_repo.go:263-268`：同时置 `totp_secret=NULL, totp_secret_enc=NULL, totp_enabled=0, totp_recovery_codes=NULL, totp_last_counter=0` |

**grep 全仓 `totp_secret` 写入点（本项审查重点），确认无遗漏明文写入路径：**

| 写入点 | 位置 | 判定 |
|---|---|---|
| `setTOTP`（唯一落库种子 SQL） | `user_repo.go:231-236` | 密文走 `secretEnc`、明文列 `setTOTP` 分支 |
| `SetTOTP`（生产路径） | `user_repo.go:218-222` | fail-closed：`secretEnc=="" && secretPlain!=""` → `ErrPlaintextTOTPSecret`，不写任何列 |
| `SetTOTPPlaintextLegacy`（显式构造历史明文） | `user_repo.go:227-229` | 仅测试/迁移调用，生产代码**无调用点**（grep 全仓只有 `.env`/测试用到） |
| `SetTOTPSecretEnc` | `user_repo.go:248` | 只写密文列，同时清明文列 |
| `DisableTOTP` | `user_repo.go:265` | 两列全清 |
| `SetTOTPLastCounter` / `SetTOTPRecoveryCodes` | `user_repo.go:239 / 271` | 只写计数器 / 备用码列，不触碰种子 |
| `storeTOTPSecret`（自助 Setup2FA） | `auth_service.go:292-302` | 加密失败即报错、**不再回退明文**；`cipher==nil` 仅测试场景显式走 `SetTOTPPlaintextLegacy` |

→ **生产路径每一处种子写入都以密文或不写的形式落库；明文列只能由显式构造入口生成。B-2 三处 + fail-closed 完整闭环。**

- 明文路径调用方复核：`auth_service.go` 自助路径加密（`:301`）、升级路径 `SetTOTPSecretEnc`（`:338,:410`）、`ForceDisable2FA`（`user_service.go:337-342`）与 `resetPassword`（`cmd/server/reset_password.go:38`）都走 `DisableTOTP`（清两列），无遗漏。
- 验证报告 §2.2 的「反向验证 FAIL 3 项」与源码一致：旧代码 `ForceEnable2FA` 明文落库、`SetTOTPSecretEnc` 不清明文、`DisableTOTP` 清密文 → 断言在旧代码必 FAIL。新增测试**非恒真**。

### 2. 【H-1】`STRICT_SECRETS` 失效 —— ✅ 达标

- `.env.example:9-10`：`JWT_SECRET=` / `ENCRYPT_KEY=` **留空**（`:29` 后是 `STRICT_SECRETS=1`）；compose 的 `${JWT_SECRET:?}` / `${ENCRYPT_KEY:?}`（`docker-compose.yml:30-31,59-60`）启动前拦截。全文件无真实密钥。
- 三层判定（`internal/config/config.go:153-176`）：① 精确默认值 → ② 占位前缀 `weakSecretPrefixes`（`please_change_me`/`change_me`/`your_`/`example`/`placeholder`/`sample`/`dummy`/`test_secret`….）+ 低熵 `lowEntropySecret`（去重字符数 <12）→ ③ 长度 <32。`ADMIN_PASS` 额外比对 `weakAdminPasswords` + 占位前缀 + 低熵。
- **关键验证**：`.env.example` 三个原占位值 → 3 条告警（含 `ADMIN_PASS=please_change_me_to_a_strong_password` 前缀命中），`STRICT_SECRETS=1` → 拒绝启动。探针（wsl-verification §2.3）实测 3/0 告警对照成立。
- 遗留补丁项（报告 §2.3 附加项）`padKey` SHA-256 / `BootstrapAdmin` 复用 `validatePassword` / `config.yml` 空口令：明确登记「未做」，属独立条目（无 scoped 业务改动），**不构成本次目标 failure**。

### 3. 【B-1】农历 `term` 节日名静默失效 —— ✅ 达标（含 M-8 合并）

- **解析期拦截（关键判据）**：`internal/scheduler/lunar_parser.go:160-168` 的 `term` 分支对每个逗号项调用 `validateTermName`，未知名 **Parse 阶段报错**并列出可用值。「不是只在 UI 改全称」——后端解析器同时拦截，`scheduler.NextRun`（`scheduler.go:110`）、任务 `validate`（`task_service.go`）都走 `NewLunarParser().Parse`，故前端/API/任务校验三条路径都生效。
- 白名单：`lunarJieQiNames`（24 节气，与 lunar-go `JIE_QI` 完全一致、顺序一致，已比对库源码 `Lunar.go:12`）+ `lunarFestivals`（9 个节日**全称**）。短名 `中秋`/`元宵`/`端午`/`春节,中秋` 全部报错。
- 快捷项：`web/src/views/Tasks.vue:814`→`term 中秋节`；其余快捷项合法。注释 `lunar_parser.go:17,23`→全称。
- 验证报告 §2.1：33 名全部有效 / `中秋` 等报错；"反向验证" 旧代码 `term 中秋` Parse 成功 + `Next()` 零值 → 断言可证伪。
- 回归测试真实性：`TestLunarTermRejectsShortFestivalName` 断言报错且错误含「中秋节」；`TestLunarTermAcceptsFullFestivalName` 精确断言日期；`TestLunarTermUnknownNameFailsFast` 断言解析 <50ms。均非恒真。

### 4. 【H-2】SMTP 认证明文降级 —— ✅ 达标

- **不是「只加日志」**：`pickAuthMechanism(secure, allowInsecure, advertised)`（`email.go:169-198`）是确定性决策。`secure==false` 且未 `allow_insecure` 时 LOGIN/PLAIN **直接 `return error`**（`:187-189`），非仅告警放行。
- `secure==true` → PLAIN > LOGIN > CRAM-MD5（不再因服务器宣告顺序自动选中 CRAM-MD5，消除 HMAC-MD5 优先）。
- 非 TLS + `allow_insecure=true` 使用 LOGIN/PLAIN 时打 warning 日志（`email.go:144`）→ 明文可见。
- 「宣告 AUTH 但列表为空」显式报错（`email.go:129`），不回落 PLAIN。
- 验证报告 §2.4：本地假 SMTP 端到端——非 TLS 宣告全机制 + `allow_insecure=true` → 实选 CRAM-MD5；仅 LOGIN/PLAIN + `true` → LOGIN；`false` → 连接期拒绝；裸 AUTH → 显式报错。探针结论与源码一致。

### 5. 【H-5】备份导入丢模板 `enabled` —— ✅ 达标

- `internal/service/export_service.go:126-129` 补 `Enabled: t.Enabled`；`Template.Enabled` 的 json tag 为 `enabled`（`model/models.go`），JSON 往返保真。
- 回归测试两条：roundtrip（导出 1 启用 + 1 停用 → JSON → 软删 → Import → 逐个断言）与直接 `SELECT enabled` 断言落库值（确认作用于 `template_repo.Create` 的显式 INSERT）。反向验证：临时移除修复后两条均 FAIL（product-fixer 已证），**非恒真**。

### 6. 【M-2】`QUEUE_CLAIM_TTL` 无校验 panic —— ✅ 达标

- 配置层钳制（`config.go:227-231` + `clampInt` `:272-283`）：`QUEUE_CLAIM_TTL`→[30,3600]、`QUEUE_WORKERS`→[1,64]、`QUEUE_POLL_MS`→[50,60000]、`QUEUE_MAX_ATTEMPTS`→[1,20]；越界打告警日志（不静默改写）。
- 运行期兜底（`queue.go`）：`pollInterval`/`renewInterval`/`recoverInterval`/`safeClaimTTL`（`:91-131`）非正回退；renew goroutine 加 `defer recover()`（`:236-241`）。
- 语义保真：`QUEUE_WORKERS=0` 运行期语义保持不变（`TestStaleRecoveryReclaims` 依赖「不起 worker 只跑恢复」），钳制只在加载层。
- 验证：`TestQueueConfigClamp` / `TestQueueZeroClaimTTLDoesNotPanic`（真实启动 0/-1s 不 panic、恰发送 1 次）/ `TestQueueIntervalFallbacks` 全 PASS。

### 7. 【M-12】CHANGELOG `[Unreleased]` 为空 —— ✅ 达标

- `CHANGELOG.md:7-79`：`[Unreleased]` 非空，四节（安全加固/新增/变更/升级注意）；「升级注意」单列 8 条，覆盖 STRICT_SECRETS、登录态窗口级、MySQL 8.0、排版变化、农历节日名必须全称、swagger/metrics 默认关闭、镜像 tag、备份导入复查——与报告 §2.20 的 7 项对应。

---

## 三、测试质量核对

| 测试 | 能否捕获原缺陷 | 依据 |
|---|---|---|
| `TestForceEnable2FAEncryptsSecret` / `TestVerify2FAUpgradesLegacyPlaintext` / `TestForceDisable2FAClearsEncryptedSecret` / `TestSetTOTPFailClosed` / `TestSetTOTPSecretEncClearsPlaintext` / `TestDisableTOTPClearsBothColumns` | ✅ | wsl-verifier 在 pre-fix 副本上实测 **FAIL 3 项**（明文 "4VNT6…" 落库 / 升级后明文明存 / 停用后密文残留）；断言用 `sql.NullString`/落库值区分 NULL，非恒真 |
| `TestWeakSecretWarningsPlaceholders` / `TestWeakSecretWarningsAdminPass` | ✅ | pre-fix 副本上全部 FAIL；断言针对 `please_change_me…`（历史占位值长度达标、精确相等漏过） |
| `TestEnvExampleSecretsBlank` | ✅ | 直接 `os.ReadFile("../../.env.example")` 解析断言两密钥空串；非内存构造 |
| `TestPickAuthMechanismInsecure` | ✅ | pre-fix 上 `secure==false` 会选到 LOGIN；断言 FAIL |
| `TestLunarTermRejectsShortFestivalName` | ✅ | pre-fix `term 中秋` Parse 成功 + `Next()` 零值 → 断言必 FAIL |
| `TestExportImportTemplateEnabledRoundtrip` / `TestImportTemplateEnabledWrittenToDB` | ✅ | 临时移除修复后两条 FAIL |
| `TestQueueZeroClaimTTLDoesNotPanic` | ✅ | 原代码 `time.NewTicker(0)` goroutine panic 崩进程 → 断言必 FAIL |

**探针结论与源码一致性**：wsl-verification §2.1–§2.4 的探针（真实 DB 读列 / 假 SMTP / 农历 Next / WeakSecretWarnings）与我读码的调用路径、SQL、分支结果一致，无「把推断写成已验证」的情况。

---

## 四、回归风险清单

### 4.1 `SetTOTP` fail-closed 误伤评估 —— **低风险（不阻断）**

- **判定**：`secretEnc=="" && secretPlain!=""` 报错拦截。仅影响「试图明文落库」的调用。仓库内全部生产调用（`auth_service.go storeTOTPSecret`、`user_service.go ForceEnable2FA`）在修复后都走密文或显式 `SetTOTPPlaintextLegacy`，无生产路径会被误伤。
- **潜在误伤点**：若未来新增一处 `SetTOTP(..."" , plain,...)` 而忘记注入 cipher，将被拒绝（这正是 fail-closed 的目的，方向安全）；测试已有的 `SetTOTPPlaintextLegacy` 显式入口可覆盖「构造历史明文」场景。
- 与报告 §2.2 修复建议 4「把明文降级限制在测试可显式构造的构造器里」一致。

### 4.2 H-1 前缀黑名单误拒强密钥 —— **低风险（保守误判，方向安全）**

- **现象（验证报告 §4.1）**：`JWTSecret="exampleNotARealSecret…"`、`"your-company-secret…"` 会告警（前缀 `example`/`your-` 命中）。`lowEntropySecret` 对某些 32 字符类型误报率理论约 1.7%，`openssl rand -hex 32`（64 字符）下为 0。
- **影响**：仅导致 `STRICT_SECRETS=1` 下拒绝启动（提示换密钥），不会带弱/公开密钥上线。方向安全。
- **后续改进建议（不在本期 inScope）**：用「精确匹配历史占位值全集 + 熵判定」替代纯前缀匹配，可去掉 `example`/`your_` 等易误伤前缀。已登记，非必须。
- **注意**：`.env.example` 的 `DB_PASSWORD`/`MYSQL_ROOT_PASSWORD` 仍为占位值 `please_change_me`，不在 WeakSecretWarnings 判定内（compose `${DB_PASSWORD:?}` 只要求非空）。属既有配置、非 7 项交付范围，造成的 DB 口令暴露限于容器网络，**低**；建议后续把 DB 口令也纳入「必改」核实（报告 H-1 未列此项）。

### 4.3 B-1 白名单兼容性 —— **低风险（刻意行为收紧，已告知）**

- **判定**：短名（`中秋`/`元宵`/`端午`）此前「保存成功但 `Next()` 恒零」，属静默失效。修复后**保存即报错**——这是报告 B-1 要求的刻意破坏性变更。
- **存量兼容性**：现有 DB `cron_expr`（生产库 4 行 `term 春节/端午节/中秋节/清明`）全部为全称/节气名，白名单内，**不受影响**（wsl-verification §4.3 实测）。短名任务本就不触发，报错是纠正而非回归。
- **文档同步**：CHANGELOG 升级注意第 5 条已写明（`@lunar term 中秋` → 保存即报错、如何改）。`lunarHint` 本地化文案仍未列出节日全称（`zh-CN.json:422` 只写 `<节气>`），属 t6 已登记「未在本条范围」，报错文案会指引全称，**低**。
- **白名单覆盖**：`lunarJieQiNames` 与 `calendar.JIE_QI` 一致；`lunarFestivals` 9 项 = 既有表键，未缩小合法范围。

### 4.4 B-2 升级触发覆盖不全（次要观察）——**低**

- `Verify2FA` 中**动态码验证成功**才触发明文→密文升级（`auth_service.go:408-414`）；**备用码登录成功**的 `else` 分支（`:396-405`）**不触发升级**。历史明文 + enabled 用户若始终只用备用码登录，明文列直到某次动态码登录才被清理。
- **影响**：非新增暴露（明文列本就在），仅延迟清理；`Enable2FA` 路径（重新设置时）两种码都能触发升级。**低**，建议后续：备用码登录成功后同样调用 `SetTOTPSecretEnc` 升级。

### 4.5 农历性能改动的语义等价性 —— **低（已实测比对，未固化为 CI 全量）**

- `termCandidatesInYear` 复用 `lun` 对象并 `lun.Next(1)` 推进，与旧逐日 `solar.GetLunar()` 等价；`lunarDayInYear` 改用「月初一儒略日 + 偏移」求阳历日。
- 验证报告 §2.1：**74028 组**（1901–2099×12×31）+ **160 组** `Next()` 比对 mismatches=0。**但这两组全量等价性仅在验证期探针中，未写入永久测试套件**（套件只有 20 个既有农历用例）。建议后续把「农历全量日网格等价」固化为 CI 防回归（对应报告 §4.4）。**低**——白名单已把 `term` 名字限制为已知集合，monthly/yearly 由既有用例覆盖。

### 4.6 M-2 钳制范围 —— **低**

- 仅对配置层 `QUEUE_WORKERS` 桩下限为 [1,64]。直接以 `QueueConfig{Workers:0}` 构造并 `Start()` 的调用方（`cmd/server/main.go:163-174` 用 `cfg.QueueWorkers`，已钳制 ≥1）运行期语义不变；`testRouter` 用 `Workers:0` 且不调用 `Start()`，无 `time.NewTicker(0)` 执行。运行期兜底函数已覆盖。**无新增风险**。

---

## 五、范围控制评估

**结论：业务改动全部落在 7 项 + 必要接线内，无顺手重构。** 逐文件归属：

| 文件 | 归属条目 |
|---|---|
| `.env.example` | H-1②（t9） |
| `internal/config/config.go` / `config_test.go` | H-1 + M-2 |
| `internal/channel/email.go` / `email_test.go` | H-2 |
| `internal/repository/user_repo.go` | B-2（②③ + fail-closed） |
| `internal/service/user_service.go` / `auth_service.go` | B-2（① + storeTOTPSecret no-fallback） |
| `internal/handler/user_handler.go` / `router/router.go` | B-2① 注入 cipher 的**必要接线**（非顺手重构） |
| `internal/service/queue.go` | M-2 |
| `internal/service/export_service.go` / `export_service_test.go` | H-5 |
| `internal/scheduler/lunar_parser.go` / `lunar_schedule.go` / `lunar_test.go` | B-1 + 性能联动（同一修点） |
| `web/src/views/Tasks.vue` | B-1（快捷项） |
| `CHANGELOG.md` | M-12 |
| `internal/service/helpers_test.go` / `user_2fa_force_test.go` | B-2 测试辅助（`testCipher` / `SetCipher`） |
| `internal/service/user_2fa_encryption_test.go` / `repository/user_totp_repo_test.go` / `queue_ttl_test.go` | 各条新增回归 |
| **`internal/testutil/lock.go` + 7 个 `main_test.go`（t5 收尾）** | **唯一超出原 inScope** |

**对测试基建的处理（透明评估）**：
- 功能：用 MySQL `GET_LOCK` 命名锁把共用 `notice_service_test` 库的 7 个测试包串行化，消除基线 HEAD 上**并行口径 5/5 失败**（captain 已在干净 worktree 复现）。它**不参与任何业务逻辑**，未被生产代码引用（`go list -deps ./cmd/server | grep -c testutil → 0`，wsl-verification §4.4 复核）。
- 这是让契约 verify（`go test ./internal/...`）在默认并行口径下可重复通过的必要测试基建修复，**非业务重构**。虽不在 7 项 inScope 内，但属「纯测试期串行锁」，不引入生产行为变化。
- 唯一维护成本：今后新增写 `notice_service_test` 库的测试包须照抄 `TestMain`（已写入 security-fixes.md 维护约定）。
- **建议**：captain 可将其明确登记为「测试基建豁免」的修订 inScope 项以闭环 git 审计；不作为阻断。

---

## 六、遗留可改进项（非阻断，均已在 §4 登记）

1. H-1：`example`/`your_`/`placeholder` 等前缀对强密钥的保守误拒（fails closed，建议后续精确化）。
2. B-2：`Verify2FA` 备用码登录路径不触发明文→密文升级（建议后续补）。
3. B-1：`lunarHint` 本地化文案列出节日全称（t6 已登记未做，报错提示已兜底覆盖）。
4. B-1 性能：74028/160 组等价性比对未固化为 CI（建议后续并入）。
5. `.env.example` `DB_PASSWORD` 仍为占位值且不在 WeakSecretWarnings。

以上均不影响本次交付目标的达成；`go build` / `go vet` exit 0，串行全量回归 exit 0。