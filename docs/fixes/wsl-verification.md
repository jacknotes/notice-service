# WSL 环境验证报告（t7）—— 7 个立即条目 + 全量回归

> 验证时间：2026-10-23 14:18–14:45（Asia/Shanghai）
> 验证者：wsl-verifier（t7）
> **本次验证为串行执行**，详见「运行方式」。
> 结论速览：**7/7 条目已验证通过**；全量回归 Go 侧全绿；前端类型检查通过；前端 vitest 有 3 条**基线性**失败（已证与本次改动无关）。

---

## 0. 运行方式与工作区冻结

- **串行性**：报告中所有 `go test` 均**单独串行**运行（契约 verify 与 CI 口径 `go test -p 1 ./... -count=1` 或单包 `-count=1`，杜绝跨包并行）；验证全程没有与我以外的任何进程并发跑测试。
- **已知并发伪失败基线**：`internal/service`、`internal/repository`、`internal/handler` 等包共用本机 `notice_service_test` 库（DSN 硬编码于各 `*_test.go`），`go test ./internal/...` 默认并行跑包时互相删对方的 `users/admin/channels/send_jobs` 行，产生与业务代码无关的随机失败（如「内置 admin 查不到」）。**本报告所有通过项均为串行/加锁口径下的结果**，未把并发伪失败误判为代码缺陷。
- **临时探针**：全部放在 WSL `/tmp` 端（`/tmp/verify-probe`、`/tmp/prefix-tree`）或 Windows `%TEMP%\notice-verify-probe`、`%TEMP%\notice-probe-prefix`（仓库外一次性目录，通过 `replace` 指向仓库只读副本），**未落仓库**；报告末尾附「探针清理声明」。业务代码零改动（`git status` 14:45 复核，唯一新增为 t5 的测试基建 `internal/testutil/lock.go` + 7 个 `main_test.go`，见 §4.4）。
- **工作区快照**：验证开始时（14:18）业务侧 19 个 M + 4 个新测试文件；验证结束时对 31 个业务文件逐一 SHA256 对照，14:18 快照中已存在的文件**内容零变化**，快照后仅新增 t5 收尾阶段（13:55–13:58）写入的测试基建文件（`internal/testutil/lock.go` + 7 个 `main_test.go`）。**整个验证期间无他人测试并发**（captain 已确认并收回误领；14:18 后多次 `ps` 复核无 `go test`/`.test` 进程）。

---

## 1. 环境快照

| 项 | 值 |
|---|---|
| 平台 | WSL2 Ubuntu 26.04（`5.10.102.1-microsoft-standard-WSL2`） |
| Go | go1.25.0 linux/amd64（`/usr/local/go/bin`，`export PATH=$PATH:/usr/local/go/bin`） |
| Node | v22.22.1（CI 位 v20 / 基线 v22；另 v24.16.0 用于 vitest 对照，见 §4.6） |
| MariaDB | 11.8.6-MariaDB，`127.0.0.1:3306`，用户 `notice/notice123`，库 `notice_service_test` |
| 工作区 | `/mnt/f/project/notice-service`（F:\\project\\notice-service，Git `a3a914c`） |
| 生产二进制 | `go list -deps ./cmd/server \| grep -c testutil` → **0**（testutil 未被生产代码引用） |

---

## 2. 逐条验证矩阵（对应报告 §4.1 的 7 个立即条目）

### 2.1 【B-1】农历 `term` 节日名白名单（blocker）—— ✅ 通过

**验收断言**：`term 中秋` 必须 Parse **报错**；`term 中秋节` 必须 Parse 成功且 `Next()` 非零；`term 元宵` 报错 / `term 元宵节` 有效。

**探针实测输出（`scheduler.NewLunarParser`，直接复现报告要求）：**

```
"中秋"    REJECT  未知的节气/节日名 "中秋"（农历节日请用全称，如「中秋节」而非「中秋」）；可用节日：七夕、中秋节、元宵节…
"元宵"    REJECT  （同，报错）
"端午"    REJECT  （同）
"春节,中秋" REJECT  未知的节气/节日名 "中秋"…   ← 列表整体拒绝，杜绝「部分生效」
"中秋节"  ACCEPT  next=2026-09-25 09:00 zero=false
"元宵节"  ACCEPT  next=2026-03-03 09:00 zero=false
"端午节"  ACCEPT  next=2026-06-19 09:00 zero=false
"春节"    ACCEPT  next=2026-02-17 09:00 zero=false
"清明"    ACCEPT  next=2026-04-05 09:00 zero=false（节气）
"七夕"    ACCEPT  next=2026-08-19 09:00 zero=false（表键为「七夕」而非「七夕节」——白名单正确）
```

**白名单覆盖性**：24 节气 + 9 个节日全称（春节/元宵节/龙头节/端午节/七夕/中秋节/重阳节/腊八节/除夕）共 33 个名字，Parse + Next() 全部有效，**失败 0**。lunar-go `calendar.JIE_QI` 的 24 项与 `lunarJieQiNames` 逐项一致（顺序亦一致，已比对库源码 `Lunar.go:12`）；`JIE_QI_IN_USE` 里多出的 6 个英文键（`DA_XUE`/`DONG_ZHI`/`XIAO_HAN`/`DA_HAN`/`LI_CHUN`/`YU_SHUI`/`JING_ZHE`）经 `convertJieQi` 映射回对应中文名，不构成额外合法输入（用户输入英文键仍会被白名单拒绝，符合预期）。

**行为等价性（性能改法未改语义，关键）**：
- `lunarDayInYear` 儒略日新算法 vs 旧 `NewLunarFromYmd`：**1901–2099 年 × 12 月 × 31 日 = 74028 组全量比对，mismatches=0**；
- 用 lunar-go 原算法（`NewLunarFromYmd` / 逐日 `GetCurrentJieQi`）重写参考实现，对 33 个 `term` 名字 × 4 个基准年、7 个 `monthly/yearly` 表达式 × 4 年共 **160 组 `Next()` 比对，mismatches=0**。

**可证伪性（防恒真断言，关键）**：把 HEAD 版 `lunar_parser.go`/`lunar_schedule.go` 还原到 `/tmp` 副本后，同一探针实测旧代码 `term 中秋` **Parse 成功且 `Next()` 为零值**——即新增 `TestLunarTermRejectsShortFestivalName` 在旧代码上必然 FAIL（已在 prefix 副本实测 FAIL）。证明断言能捕获缺陷。

### 2.2 【B-2】TOTP 密钥加密闭环（blocker）—— ✅ 通过

**直接读库断言（`SELECT totp_secret, totp_secret_enc, totp_enabled … FROM users WHERE id=?`，走真实 service/repository 层）：**

```
① ForceEnable2FA（注入 cipher）：totp_secret.Valid=false  ← NULL ✓   enc.len=80（非空）✓   codes=8 ✓   uri=true ✓
   ✓ 明文列 NULL 且密文列非空；密文能用同一 cipher 解回原密钥；totp_enabled=true
② SetTOTPSecretEnc（手工造明文行 totp_secret='LEGACY-SEED' 后调用）：totp_secret.Valid=false（明文列被清空）✓  enc="UPGRADED-ENC"
③ SetTOTP(enc="", plain="PLAINTEXT-SEED") → err=ErrPlaintextTOTPSecret ✓（fail-closed 拒绝明文降级）
④ DisableTOTP：secret.Valid=false enc.Valid=false enabled=false counter=0 recovery.Valid=false ✓（两列均清空）
⑤ 未注入 cipher 的 ForceEnable2FA → 报错「服务器未配置加密密钥…」且两列均未写入 ✓（fail-closed）
```

**可证伪性**：HEAD 版 `user_service.go`/`user_repo.go` 还原到 `/tmp` 副本后，同一组断言：
- `①` 旧代码 **totp_secret 非 NULL（明文 "4VNT6D7SHUEVSXAIBESBZXRTIORKLEJU"）、enc 列 NULL** → 断言 FAIL；
- `②` 旧代码升级后**明文列仍为 "LEGACY-SEED"**（未清）→ FAIL；
- `③` 旧代码 `DisableTOTP` 后**密文列仍为 "E"**（未清）→ FAIL。
共 3 处可捕获缺陷，证明新增回归测试非恒真。

**新增回归测试（串行逐个跑）**：`TestSetTOTPFailClosed`/`TestSetTOTPSecretEncClearsPlaintext`/`TestDisableTOTPClearsBothColumns`（repository，0.02–0.03s PASS）+ `TestForceEnable2FAEncryptsSecret`/`TestForceEnable2FAWithoutCipherFailsClosed`/`TestVerify2FAUpgradesLegacyPlaintext`/`TestForceDisable2FAClearsEncryptedSecret`/`TestUserServiceCipherInjection`/`TestForce2FA`（service，全部 PASS）。

**明文写入路径排查**：grep 全仓 `totp_secret` 的所有写入点，仅 `user_repo.go` 的 `setTOTP`（密文走 `secretEnc`、`SetTOTPPlaintextLegacy` 显式构造历史明文）与 `SetTOTPSecretEnc`/`DisableTOTP`（清列）；`auth_service.go` 的 `storeTOTPSecret` 加密失败即报错、`cipher==nil` 时显式走 `SetTOTPPlaintextLegacy`。生产路径无遗漏的明文落库入口。

### 2.3 【H-1】STRICT_SECRETS 弱密钥识别 + `.env.example` 留空—— ✅ 通过

**探针实测：**

```
WeakSecretWarnings（修复后）:
  .env.example 原值三件套(please_change_me…/请改密码) → 3 条告警 ✓
  JWT 32 个 a（低熵）                       → 1 条告警 ✓
  ADMIN_PASS=x / admin / password           → 各 1 条告警 ✓
  强随机密钥(9f8e…/a1b2…/Str0ng!Passphrase#2026) → 0 告警 ✓（不误拒正常强值）
```

**`.env.example`**：`JWT_SECRET=` `/ENCRYPT_KEY=` 确为**留空**（`TestEnvExampleSecretsBlank` PASS），配合 compose `${VAR:?}` 在启动前拦截；`ADMIN_USER`/`ADMIN_PASS` 保留占位值——而 `WeakSecretWarnings` 现能把这些占位值判为弱（占位前缀黑名单命中）。`ADMIN_PASS=please_change_me_to_a_strong_password` → 告警（前缀 `please_change_me` 命中）。

**新增测试**：`TestWeakSecretWarningsPlaceholders`/`TestWeakSecretWarningsAdminPass`/`TestEnvExampleSecretsBlank` 全部 PASS；在 pre-fix 副本上这些测试全部 FAIL（证明可证伪）。

### 2.4 【H-2】SMTP 认证降级—— ✅ 通过

**可证伪性 + 新测试**：`TestPickAuthMechanismSecure`/`TestPickAuthMechanismInsecure`/`TestCramMD5NextNoLeak` PASS；在 pre-fix 代码上 insecure 分支会兜错（`secure==false` 时选到 LOGIN）。

**端到端探针（本地假 SMTP 服务器，验证后删除）**：

```
非TLS + 服务器宣告 CRAM-MD5 LOGIN PLAIN + allow_insecure=true  → 实际选 CRAM-MD5，发送成功
非TLS + 服务器宣告 LOGIN PLAIN        + allow_insecure=true  → 实际选 LOGIN，发送成功（带 warning 日志）
非TLS + 服务器宣告 CRAM-MD5 LOGIN PLAIN + allow_insecure=false → 连接期被拒（STARTTLS 守卫），不发送
非TLS + 服务器宣告裸 AUTH（无机制）     + allow_insecure=true  → 显式报错，不回落 PLAIN
```

→ **`secure==false` 时，LOGIN/PLAIN 必须 `allow_insecure==true` 才允许，否则拒绝**；非 TLS 未显式 allow_insecure 时走 STARTTLS 守卫直接拒绝。空 AUTH 列表显式报错、不回落 PLAIN。

### 2.5 【M-2】QUEUE_CLAIM_TTL 无校验 → 钳制/兜底—— ✅ 通过

**探针实测（配置加载，非法值不 panic 且被钳制）：**

```
QUEUE_CLAIM_TTL = 0 / -1 / -5 / 1   → 30s（钳制到下限，打告警日志）
QUEUE_CLAIM_TTL = 120               → 2m0s（正常保留）
QUEUE_CLAIM_TTL = 999999            → 3600s（钳制到上限）
QUEUE_WORKERS = 0 / -3              → 1；QUEUE_POLL_MS = 0 / -1 → 50ms
QUEUE_WORKERS = 1000                → 64；QUEUE_POLL_MS = 1000 → 1000ms
```

**运行期兜底**：renew goroutine `defer recover()` + `renewInterval()`/`pollInterval()`/`recoverInterval()`/`safeClaimTTL()`；`TestQueueZeroClaimTTLDoesNotPanic`（ClaimTTL=0 与 -1s 真实启动队列、job 置 done 且恰发送 1 次）与 `TestQueueIntervalFallbacks` PASS。

### 2.6 【H-5】备份导入丢模板 enabled—— ✅ 通过

**新测试**：`TestExportImportTemplateEnabledRoundtrip`（导出 1 启用 + 1 停用 → JSON 往返 → 软删 → Import → 逐个断言状态一致）与 `TestImportTemplateEnabledWrittenToDB`（直接 `SELECT enabled` 断言落库值）均 PASS；在 pre-fix 代码上这两条测试 FAIL（product-fixer 已证，t6 交付），证明可证伪。

### 2.7 【M-12】CHANGELOG [Unreleased] + 升级注意—— ✅ 通过

- `CHANGELOG.md` `## [Unreleased]`（L7，非空）到 `## [2.0.0]`（L81）之间共 73 行，含「安全加固/新增/变更/升级注意」四节（`### 安全加固` L12、`### 新增` L29、`### 变更` L46、`### 升级注意` L62）；
- 「升级注意」单列 8 条（含`STRICT_SECRETS` 启动失败风险、登录态窗口级、MySQL 8.0、渲染变化、**农历节日名必须用全称**、swagger/metrics 默认关闭、镜像 tag、备份导入复查）；明确覆盖 7 项立即背景。

---

## 3. 完整回归

| 项 | 命令 | 结果 |
|---|---|---|
| 全量 build | `go build ./...` | ✅ exit 0 |
| 全量 vet | `go vet ./...` | ✅ exit 0 |
| 全量测试（串行 ×2） | `go test -p 1 ./... -count=1` | ✅ 两次均 exit 0，**18 个包全 ok**（handler 18.5s / service 10.3s / repository 1.7s / scheduler 1.5s 等）；另跑 `go test ./... -count=1`（默认并行，验证命名锁）→ exit 0，3 次连续全绿 |
| 前端类型检查 | `cd web && npx vue-tsc --noEmit` | ✅ exit 0 |
| 前端单元测试 | `cd web && npm test` | ⚠️ 3 failed / 62 passed（**基线问题**，见 §4.6） |

新增回归测试单独串行跑全部 PASS：repository TOTP 3 条、service 2FA+导出+队列 9 条、scheduler lunar 3 条、config 弱密钥+钳制 5 条、channel SMTP 3 条。

---

## 4. 发现的问题、偏差与测试基建

### 4.1 【H-1 已知误拒】前缀黑名单 + 低熵阈值会误伤「合法但恰好命中前缀/恰好低熵」的强密钥

**现象（探针实测）**：
- `WeakSecretWarnings` 对 `JWTSecret="exampleNotARealSecret0123456789abcdef"`（32+ 字符、高熵）返回 **1 条告警**（前缀命中 `example`）；
- 对 `"your-company-secret-0123456789abcdef"` 同样告警（前缀命中 `your-`）；
- `lowEntropySecret`（去重字符数 < 12）对真正的 32 字符 hex（如 `openssl rand -hex 16`）误报率**精确计算 ≈ 1.70%**（64 字符 hex / 32 base62 为 0）。

**是否构成阻断**：**不阻断**。它是「保守误判」——把合法强密钥误判成弱密钥只会在 `STRICT_SECRETS=1` 下**拒绝启动**（会提示运维换密钥，不会带着弱密钥上线），方向安全；且 CI 的 `config_test.go` 也按此设计。误报率在 README 推荐生成方式（`openssl rand -hex 32`＝64 字符 hex）下为 0。

**建议（供 t8 记录，不必在本条修复）**：后续把「是否等于已知占位值」判定 + 熵阈值组合，替代纯前缀匹配（e.g. 先精确匹配 `.env.example`/README 历史占位值全集，再做熵判断），可去掉 `example`/`your_` 等易误伤前缀。

### 4.2 【B-1 兼容性】白名单会拒绝「此前能保存但静默失效」的短名——这是刻意的行为收紧

- 旧版 `term 中秋`/`元宵`/`端午` 保存成功但 `Next()` 恒零（任务永不触发）；修复后**保存即报错**。这符合报告 B-1 要求（解析期拒绝），属**刻意破坏性变更**，已在 CHANGELOG 升级注意写明（`农历节日名必须用全称`）。
- 现有 DB 中 `term 春节`/`端午节`/`中秋节`/`清明`（生产库 tasks 表）均为全称/节气名，**不受影响**。

### 4.3 【生产库无遗留短名】核实

`SELECT cron_expr FROM notice_service.tasks WHERE cron_expr LIKE '%term%'` → 4 行：`term 春节`/`term 端午节`/`term 中秋节`/`term 清明`，全部在白名单内；test 库 0 行。无升级后立即失配的存量任务。

### 4.4 【测试基建】`internal/testutil/lock.go` + 7 个 `main_test.go`（t5 收尾新增）

**锁确实生效（实测对照，同一台机器、同一库、均先 `go clean -testcache`）**：

| 口径 | 基线（pre-fix 副本，无锁，HEAD 源码+HEAD 测试） | 修复后（有锁） |
|---|---|---|
| `go test ./internal/... -count=1`（默认并行）× 3 次 | **3/3 失败**（TestUserServiceDisableEnable / TestUserBatchResetPassword / TestRoleChangeTakesEffectImmediately / TestNotificationServiceSendOnceWithoutReceivers / TestImportCreatesAndSkips / TestExportBundle…） | **3/3 全绿** |
| `go test -p 1 ./... -count=1`（串行）× 2 次 | — | 两次均 exit 0 |

实现说明：`testutil.Acquire()` 用 MySQL 命名锁 `GET_LOCK('notice_service_test_serial', 600)` 把共用同一测试库的包串行化（连接级锁、固定 `MaxOpenConns=1`、同连接 RELEASE_LOCK；取锁失败时放行让用例报真实连接错误，避免把「环境缺失」伪装成锁超时）。

**副作用（如实记录）**：命名锁使不等于 `-p 1` 的并行口径耗时从 ~20s 升到 ~30s（repository 20s/scheduler 21s/service 31s——各包在**起跑等待锁**），`-p 1` 串行口径 ~103–110s。锁只影响测试并行度，不改变任何业务行为。

**范围/纯净性**：`internal/testutil` 未被任何生产代码引用（`go list -deps ./cmd/server | grep -c testutil` → **0**；全仓 `grep -rl "internal/testutil" --include=*.go` 仅命中 7 个 `*_test.go` 的 `TestMain`，其余命中全在 `.dev/gomodcache` 第三方依赖里，与本项目无关）；`go vet`/`go build` 全绿。

### 4.5 与报告的偏差

- 报告 §4.1 的 7 项均已落入代码并被验证，**无「修了没修对」项**（逐条对应见 §2.1–2.7）；
- 未探测到新的验证失败项；`go build`/`go vet`/全量测试均无回归；
- 唯一与报告口径不同的点：报告 §4.4 建议「TOTP 加密不变量测试需 MySQL service（CI 已有 MySQL 8.0）」，本机用 MariaDB 11.8 实测通过——两者在 `UPDATE ... SET a=?, b=NULL`、`sql.NullString` 语义上无差异，但 CI 仍建议保留 MySQL 8.0 跑一次（MariaDB 与 MySQL 对 `GET_LOCK` 名称长度上限均为 64 字符，`notice_service_test_serial` 24 字符，无风险）。

### 4.6 前端 vitest：3 条基线性失败（与本次改动无关）

**入口差异（已定位）**：在 WSL 里 `npx vitest` 与 `npm test` 走的不是同一个二进制——
- `npx --no-install vitest --version` → `vitest/2.1.9 **win32-x64 node-v20.20.2**`（WSL 的 npx 在 `/mnt/f` 上优先解析了 Windows 侧的 `node_modules/.bin/vitest.CMD`，于是用 Windows node v20 跑）；
- `./node_modules/.bin/vitest --version` → `vitest/2.1.9 linux-x64 node-v22.22.1`（正确入口）。
Windows node v20 下 jsdom 30 / undici 8 的 `webidl.util.markAsUncloneable` 不存在 → **8 个套件全部加载失败（0 collect，8 errors）**。这是**调用入口 + 本机 node 版本**问题，不是业务回归。

**正确入口（`npm test` → linux node v22.22.1）**：8 个套件 **62 passed / 3 failed**，失败集中在 `src/api/*`：
1. `client.test.ts > 响应 401 且不在登录页时跳转 /login` —— 期望 `location.href` 为 `/login`，实际 `http://localhost/dashboard`（jsdom 对 `window.location` 赋值的语义）；
2. `index.test.ts > logApi.export 带 responseType=blob` —— 断言 `mockClient.get` 被以 `('/logs/export', { responseType: 'blob' })` 调用，实际参数不含该配置；
3. `index.test.ts > backupApi.export 下载 JSON` —— 同型断言不匹配。

**基线对照（归因证据）**：把 `web/` 源码（含这 3 个测试文件原样、不含本次任何改动）拷到 `/tmp` pre-fix 副本跑 `vitest run` → **完全相同的 3 条失败、62 passed**。且这 3 条位于 `src/api/{client,index}.test.ts`，与本次改动的文件（`Tasks.vue`、后端 TOTP/SMTP/queue/config/export/lunar）**零交集**。

**结论**：`npm test` 的 3 failed 为**基线既有问题**（jsdom 环境下 `window.location`/`Blob` 断言与 axios mock 形态不匹配），**与本次 7 条修复无关**；`vue-tsc --noEmit` exit 0，前端类型构建正常。

---

## 5. 结论

| 维度 | 判定 |
|---|---|
| 7 个立即条目 | **全部通过**（B-1 ✓ / B-2 ✓ / H-1 ✓ / H-2 ✓ / M-2 ✓ / H-5 ✓ / M-12 ✓），每个均给出可复现命令 + 实际输出 + 断言 |
| 两个 blocker 验收核心 | B-2：`totp_secret IS NULL` + `totp_secret_enc` 非空实测 ✓，且**反向验证**在旧代码 FAIL；B-1：`term 中秋` 报错 + `term 中秋节` 有效实测 ✓，且**反向验证**在旧代码零值 |
| 全量回归（Go） | `go build`/`go vet`/`go test -p 1 ./... -count=1` 均 exit 0，18 包全 ok |
| 前端 | 类型检查 ✓；vitest 3 条失败为基线环境问题（与本次无关） |
| 测试基建 | 命名锁生效（并行口径从基线 3/3 失败 → 全绿），不污染生产二进制 |
| 范围 | 验证本身零业务改动；`internal/`、`web/src/` 的变更全部为 t5/t6/t9 交付（含测试基建），见第 6 节 |

---

## 6. 范围核对（供 t8 / captain）

`git status` 业务侧最终清单（14:45，SHA256 与 14:18 快照一致，仅新增 t5 测试基建）：

- **本次改动来源核对**：19 个 M + `.env.example` + 两个 fix 文档均属 t5/t6/t9 交付；新增测试文件 `internal/repository/user_totp_repo_test.go`、`internal/service/{user_2fa_encryption_test,...,queue_ttl_test,export_service_test}.go` 属 t5/t6 声明的 inScope。
- **测试基建新增**（t5 收尾声明，未被列入 t5 初始 inScope，但属「纯测试基建、不参与生产逻辑」）：`internal/testutil/lock.go`、`cmd/server/main_test.go`、`internal/{database,handler,integration,repository,scheduler,service}/main_test.go`。**这是本次唯一超出原 inScope 清单的改动**，请 t8 按「测试基建豁免」评估（生产依赖为零、仅 TestMain 取命名锁）。
- **不在 t5/t6 原 inScope 但实际被改并与修复强相关的文件**（均已被 captain 第二/三次 revision 明确扩进 inScope，此处仅登记）：`internal/handler/user_handler.go`、`internal/router/router.go`（B-2 注入 cipher 的必要接线）、`internal/service/helpers_test.go`（testCipher 辅助）、`internal/service/user_2fa_force_test.go`（补 SetCipher）——**均属 B-2 实现的一部分**，非顺手重构。
- 仓库内**未遗留任何临时探针**；探针均在仓库外（/tmp 或 %TEMP%），git 不含 `.perfprobe` 等残留。

---

## 7. 探针清理声明

- 探针源码与输出全部位于**仓库外**：WSL `/tmp/verify-probe`（config/lunar/totp/smtp 四组探针）、`/tmp/prefix-tree`（pre-fix Go 对照副本）、`/tmp/prefix-web`（pre-fix 前端对照副本）、Windows `%TEMP%\notice-verify-probe`、`%TEMP%\notice-probe-prefix`。
- **已在验证结束后全部删除**（`rm -rf /tmp/verify-probe /tmp/prefix-tree /tmp/prefix-web` 与 `%TEMP%` 下两个目录，并清理 `/tmp/base_par_*.log`、`/tmp/fix_par_*.log`、`/tmp/serial_*.log`、`/tmp/v1.log`、`/tmp/v2.log`）。删除后复核：两个 `%TEMP%` 探针目录 `Test-Path` → False，`/tmp` 下已无本任务产物。
- **仓库内零探针残留**：`git status --untracked-files=all`（清理后）业务侧仅 t5/t6/t9 的 19 个 M + 12 个新增文件（含 7 个 `main_test.go` 与 `internal/testutil/lock.go`），无 `.perfprobe`/`probe`/`tmp` 类文件。
- 本报告引用的所有探针输出均在写入前已抄录为正文内容，删除探针不影响证据可复核性（关键数字与原文已内联在上述各节）。