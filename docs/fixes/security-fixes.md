# 安全类「立即」条目修复记录（B-2 / H-1 / H-2 / M-2）

对应统一报告：[`docs/reviews/v2.0.0-post-review.md`](../reviews/v2.0.0-post-review.md) §4.1「立即（发布前必须完成）」
第 1、2、4 项，以及 §2.2（B-2）、§2.3（H-1）、§2.4（H-2）、§2.10（M-2）。

修复范围仅限以下文件（不改动本条之外的业务逻辑）：

- `internal/service/user_service.go`、`internal/service/auth_service.go`、`internal/repository/user_repo.go`
- `internal/config/config.go`、`internal/config/config_test.go`
- `internal/channel/email.go`、`internal/channel/email_test.go`
- `internal/service/queue.go`
- `.env.example`
- 连带的最小接线改动（为把 cipher 注入 UserService，B-2① 必需）：
  `internal/handler/user_handler.go`、`internal/router/router.go`
- 新增测试：`internal/repository/user_totp_repo_test.go`、`internal/service/user_2fa_encryption_test.go`、
  `internal/service/queue_ttl_test.go`；`internal/service/helpers_test.go`（新增 `testCipher` 辅助）、
  `internal/service/user_2fa_force_test.go`（既有测试补 SetCipher）
- 纯测试基建（消除「共用同一测试库」的跨包并发竞争，不涉及业务逻辑）：
  `internal/testutil/lock.go` 与 7 个包的 `main_test.go`（见文末「附带修复」）

---

## B-2 TOTP 密钥加密不完整（blocker，报告 §2.2，S-1 + S-3 合并）

### 问题

「TOTP 种子不得以明文长期驻留 DB」这一本次核心安全目标有**两个独立破口**，修其一不足以闭环：

1. **`ForceEnable2FA` 明文落库**：`internal/service/user_service.go:309`（修复前）把空串作为
   `secretEnc` 传给 `SetTOTP` → 种子落到 `totp_secret` 明文列，`totp_secret_enc` 保持 NULL。
   管理员**批量强制开启**（最常见的合规运维操作）反而是明文，越是被组织统一管理的账号保护越弱。
2. **升级路径不清明文列**：`SetTOTPSecretEnc` 只写 `totp_secret_enc`，`totp_secret` 原封不动 →
   历史明文数据首次验证成功升级后，同一份种子**明文与密文并存**且永不清理。
   `DisableTOTP` 同样只清明文列，不清密文列 → 已停用账号的种子材料长期驻留 DB（违反数据最小化）。

### 改法

**① 加密写入（`internal/service/user_service.go`）**

- `UserService` 新增 `cipher *crypto.Cipher` 字段（`:38`）与 `SetCipher`（`:46`），
  与 `AuthService.SetCipher` 同源密钥（`cmd/server/main.go:149-155` 的 `ciph`）。
- `ForceEnable2FA`（`:305`）改为：先生成密钥 → `s.cipher.EncryptString(secret)`（`:317`）→
  `SetTOTP(userID, secretEnc, "", recoveryJSON)`（`:327`）。
- **fail-closed**：`cipher == nil` 时直接报错（`:307`，`服务器未配置加密密钥，无法安全写入 2FA 密钥`），
  加密失败同样报错——**不做明文降级**（报告要求「测试可显式构造例外」，见 ③）。
- 接线：`handler.NewUserHandler(db, cipher)`（`internal/handler/user_handler.go:23`）→
  `router.NewRouter` 传入 `cipher`（`internal/router/router.go:89`）。
- `AuthService.storeTOTPSecret`（`internal/service/auth_service.go:292`）同步收紧：原先加密失败会
  **回退明文列**（注释自称「可用性优先」），这条回退本身就是 B-2 的同型破口，改为加密失败即报错
  （`:299`）；仅当 `cipher == nil`（单元测试/离线工具）才显式走 `SetTOTPPlaintextLegacy`（`:294`）。

**② 升级/停用清列（`internal/repository/user_repo.go`）**

- `SetTOTPSecretEnc`（`:248`）：SQL 改为
  `UPDATE users SET totp_secret_enc=?, totp_secret=NULL WHERE ...`（`:250`）——升级即消除明文。
- `DisableTOTP`（`:263`）：SQL 改为
  `UPDATE users SET totp_secret=NULL, totp_secret_enc=NULL, totp_enabled=0, totp_recovery_codes=NULL, totp_last_counter=0 WHERE ...`（`:265`）——
  两列种子、备用码、防重放计数器一次清空。

**③ 写入层 fail-closed（`internal/repository/user_repo.go`）**

- 新增 `ErrPlaintextTOTPSecret`（`:209`）。`SetTOTP`（`:218`）在
  `secretEnc == "" && secretPlain != ""` 时直接返回该错误（`:220`），不执行 SQL。
- 历史明文只能由显式的 `SetTOTPPlaintextLegacy`（`:227`）构造（测试/迁移用），
  生产代码路径无法再「忘了加密」而静默退化成明文。
- 由此形成写入层不变量：**两列至多一列非空**（密文写入时明文列写 NULL，反之亦然）。

### 文件:行号

| 文件 | 行 | 内容 |
|---|---|---|
| `internal/service/user_service.go` | 38, 46 | `cipher` 字段 + `SetCipher` |
| `internal/service/user_service.go` | 305-334 | `ForceEnable2FA` 加密写入 + fail-closed |
| `internal/service/auth_service.go` | 292-302 | `storeTOTPSecret` 去掉明文回退 |
| `internal/repository/user_repo.go` | 208-236 | `ErrPlaintextTOTPSecret` / `SetTOTP` fail-closed / `SetTOTPPlaintextLegacy` |
| `internal/repository/user_repo.go` | 248-252 | `SetTOTPSecretEnc` 同时清 `totp_secret` |
| `internal/repository/user_repo.go` | 263-268 | `DisableTOTP` 同时清 `totp_secret_enc`（+ 计数器） || `internal/handler/user_handler.go` | 23 | `NewUserHandler(db, cipher)` |
| `internal/router/router.go` | 89 | 传入 `cipher` |

### 验证证据

新增回归测试（MySQL 实库，`notice_service_test`）：

| 测试 | 断言 |
|---|---|
| `internal/service/user_2fa_encryption_test.go:30` `TestForceEnable2FAEncryptsSecret` | 强制开启后 `totp_secret IS NULL`、`totp_secret_enc` 非空且 ≠ 明文、可用同一 cipher 解回原种子；端到端用该种子完成 2FA 登录 |
| `internal/service/user_2fa_encryption_test.go:78` `TestForceEnable2FAWithoutCipherFailsClosed` | 未注入 cipher → 报错且**两列均未写入** |
| `internal/service/user_2fa_encryption_test.go:94` `TestVerify2FAUpgradesLegacyPlaintext` | 历史明文首次验证成功后密文列写入、明文列变 NULL、密文可解回原种子 |
| `internal/service/user_2fa_encryption_test.go:143` `TestForceDisable2FAClearsEncryptedSecret` | 强制关闭后两列种子均为 NULL |
| `internal/repository/user_totp_repo_test.go:31` `TestSetTOTPFailClosed` | 明文降级返回 `ErrPlaintextTOTPSecret` 且**不写任何列**；`SetTOTPPlaintextLegacy` 仍可用；密文写入后明文列 NULL |
| `internal/repository/user_totp_repo_test.go:68` `TestSetTOTPSecretEncClearsPlaintext` | 升级后 `totp_secret IS NULL` |
| `internal/repository/user_totp_repo_test.go:90` `TestDisableTOTPClearsBothColumns` | 停用后两列 NULL + `totp_enabled=0` + 计数器归零 + 备用码清空 |

```
$ go test ./internal/repository/ -run TOTP -v
--- PASS: TestSetTOTPFailClosed (0.03s)
--- PASS: TestSetTOTPSecretEncClearsPlaintext (0.02s)
--- PASS: TestDisableTOTPClearsBothColumns (0.03s)
ok  notice-service/internal/repository

$ go test ./internal/service/ -run "ForceEnable|ForceDisable|Verify2FAUpgrades|CipherInjection" -v
--- PASS: TestForceEnable2FAEncryptsSecret (0.26s)
--- PASS: TestForceEnable2FAWithoutCipherFailsClosed (0.02s)
--- PASS: TestVerify2FAUpgradesLegacyPlaintext (0.23s)
--- PASS: TestForceDisable2FAClearsEncryptedSecret (0.04s)
--- PASS: TestUserServiceCipherInjection (0.00s)
ok  notice-service/internal/service
```

既有用例 `TestForce2FA`（`internal/service/user_2fa_force_test.go:33`）已补 `SetCipher`，
仍覆盖「强制开启 → 备用码登录 → 强制关闭 → 直接登录」全链路，handler 侧
`TestForce2FAEndpoints` / `TestUserBatch2FA` 通过（见文末测试记录）。

---

## H-1 `STRICT_SECRETS=1` 对 `.env.example` 占位值完全无效（high，报告 §2.3，S-4 + P3 合并）

### 问题

`WeakSecretWarnings` 只做**精确 `==` 比较**，而 `.env.example:7-10` 的占位值
`please_change_me_to_32_bytes_random_hex`（39 字符）/ `please_change_me_to_a_strong_password`（36 字符）
不在集合内且长度 ≥32 → **0 条告警**，`main.go:120` 的 `len(warnings) > 0` 直接短路，
`STRICT_SECRETS=1` 下 `log.Fatalf` 永不触发。运维在 compose 里看到 `STRICT_SECRETS=1` 会误以为已被保护，
而公开仓库里的固定密钥足以自签管理员 JWT（`HS256`）或解密全部渠道配置。

### 改法

**① 三层弱密钥判定（`internal/config/config.go`）**

- 新增 `weakSecretPrefixes`（`:102`）：`please_change_me` / `please-change-me` / `pleasechangeme` /
  `change_me` / `change-me` / `changeme` / `your_` / `your-` / `example` / `placeholder` /
  `sample` / `dummy` / `test_secret` / `test-secret`（小写比较，前缀匹配）。
- 新增 `isPlaceholderSecret`（`:120`）。
- 新增 `lowEntropySecret`（`:135`）：去重字符数 < 12 → 低熵（挡住「32 个 a」「abab…」这类长度达标但可预测的密钥）。
- `WeakSecretWarnings`（`:153`）改为三层判定：① 精确默认值 → ② 占位前缀/低熵 → ③ 长度不足 32；
  `ADMIN_PASS` 额外比对 `weakAdminPasswords`（`:112`，含 `x` / `1` / `a` / `admin` / `password` / `admin123` …）。
- 保留原有 4 条告警语义（`change-me` + 示例 ENCRYPT_KEY + `admin123` + JWT 长度不足仍为 4 条），
  既有 `TestWeakSecretWarnings` 无需改动即通过。

**② `.env.example` 密钥留空（`.env.example:9-10`）**

`JWT_SECRET=` / `ENCRYPT_KEY=` 留空，让 `docker-compose.yml:30-31,59-60` 与
`docker-compose.quickstart.yml:25-26` 的 `${VAR:?}` 守卫在**启动前**直接报错拦截，
而不是留一个「长度达标但公开」的占位值骗过 strict 检查。注释里写明这一意图。

**③ `ADMIN_PASS` 纳入黑名单**

`ADMIN_PASS` 走 `BootstrapAdmin` 直接建号、**无任何强度校验**，因此弱口令表 + 占位前缀 + 低熵三路都必须命中
（`x`/`1`/`a`/`admin`/`password` 全部告警，见下方测试）。

### 文件:行号

| 文件 | 行 | 内容 |
|---|---|---|
| `internal/config/config.go` | 102-118 | `weakSecretPrefixes` / `weakAdminPasswords` |
| `internal/config/config.go` | 120-150 | `isPlaceholderSecret` / `lowEntropySecret` |
| `internal/config/config.go` | 153-176 | `WeakSecretWarnings` 三层判定（JWT/ENCRYPT/ADMIN_PASS） |
| `.env.example` | 9-10 | `JWT_SECRET=` / `ENCRYPT_KEY=` 留空 |

### 验证证据

| 测试 | 断言 |
|---|---|
| `internal/config/config_test.go:123` `TestWeakSecretWarningsPlaceholders` | `.env.example` 三个原值、单点占位（JWT/ENCRYPT/ADMIN_PASS 各一）、32 个 `a`、`abab…`、`change_me_`/`your_`/`example_`/`placeholder_` 前缀共 11 组，**全部非空告警** |
| `internal/config/config_test.go:150` `TestWeakSecretWarningsAdminPass` | `x`/`1`/`a`/`admin`/`password`/`admin123`/`please_change_me` 全部告警 |
| `internal/config/config_test.go:163` `TestEnvExampleSecretsBlank` | 解析 `.env.example` 断言 `JWT_SECRET`/`ENCRYPT_KEY` 为空串；且历史占位值仍被识别为弱密钥 |
| `internal/config/config_test.go:199` `TestQueueConfigClamp` | 见 M-2 |

```
$ go test ./internal/config/ -run "Weak|EnvExample|QueueConfigClamp" -v
--- PASS: TestWeakSecretWarnings (0.00s)
--- PASS: TestWeakSecretWarningsPlaceholders (0.00s)
--- PASS: TestWeakSecretWarningsAdminPass (0.00s)
--- PASS: TestEnvExampleSecretsBlank (0.00s)
--- PASS: TestQueueConfigClamp (0.01s)
ok  notice-service/internal/config
```

---

## H-2 SMTP 认证协商引入明文降级与 CRAM-MD5 优先（high，报告 §2.4）

### 问题

`582a9bd` 引入认证协商后（`internal/channel/email.go`）：

1. `authByExtensions` 的分支顺序**只看服务器宣告的 AUTH 扩展、不看 `secure`**。`secure == false`
   且服务器宣告 `LOGIN` 时，用户名/口令以 base64（等价明文）走网络，**无日志、无拒绝**。
2. **CRAM-MD5 优先**：`crypto/md5` 的 HMAC-MD5 无迭代无盐，非 TLS 下可被离线字典攻击。
3. CRAM-MD5 分支**跳过** `allow_insecure` 守卫（该守卫只挂在 PLAIN 分支上）。
4. 「宣告 AUTH 但列表为空」时 `advertised` 为空 map，落到「未宣告」分支 → 非 TLS 下报错、TLS 下回落 PLAIN，**日志缺失**。

### 改法（`internal/channel/email.go`）

- 新增**纯函数** `pickAuthMechanism(secure, allowInsecure bool, advertised map[string]bool)`（`:169`），
  决策不再依赖服务器宣告顺序：
  - `secure == true` → `PLAIN > LOGIN > CRAM-MD5`（TLS 内明文口令无风险，MD5 反而是弱点，因此**不再优先 CRAM-MD5**）；
  - `secure == false` → **只允许 `CRAM-MD5`**（口令不上线）；`LOGIN`/`PLAIN` 必须 `allow_insecure == true`
    才放行，否则返回错误（`SMTP 服务器仅支持 LOGIN/PLAIN 且连接非 TLS，拒绝明文传输邮箱密码…`）。
- `authByExtensions`（`:115`）改为调用该纯函数，并在非 TLS 使用 LOGIN/PLAIN 时打 **warning 级日志**
  （`:144`，含机制名与 host），让「明文传凭据」在运维侧可见。
- 「宣告 AUTH 但机制列表为空」显式报错（`:129`），**不再回落 PLAIN**。
- 未宣告 AUTH 的路径：TLS 上回落 PLAIN 时补日志（配置漂移可见），非 TLS 仍拒绝。
- 保留 `cramMD5Auth` 实现（部分内网中继只支持它），但只在「非 TLS 且必须不暴露口令」或
  「TLS 且无 PLAIN/LOGIN」时使用；不再因为服务器把 `CRAM-MD5` 排前面就自动选中。

### 文件:行号

| 文件 | 行 | 内容 |
|---|---|---|
| `internal/channel/email.go` | 115-152 | `authByExtensions`：调用纯函数 + warning 日志 + 空 AUTH 列表显式报错 |
| `internal/channel/email.go` | 155-165 | 机制常量 `mechCRAMMD5` / `mechLogin` / `mechPlain` |
| `internal/channel/email.go` | 169-198 | `pickAuthMechanism`：secure/allow_insecure 驱动的确定性选择 |

### 验证证据

| 测试 | 断言 |
|---|---|
| `internal/channel/email_test.go:68` `TestPickAuthMechanismSecure` | TLS 全支持时选 PLAIN（**不选** CRAM-MD5）；无 PLAIN 选 LOGIN；仅 CRAM-MD5 才选它 |
| `internal/channel/email_test.go:86` `TestPickAuthMechanismInsecure` | 非 TLS 且服务器把 LOGIN 排前 → **仍选 CRAM-MD5**；未 allow_insecure 时 LOGIN/PLAIN 一律拒绝；allow_insecure=true 时允许且 LOGIN 优先；空/不支持机制集合报错 |
| `internal/channel/email_test.go:121` `TestCramMD5NextNoLeak` | CRAM-MD5 响应为 `用户名 HMAC` 格式且**不含明文口令** |

```
$ go test ./internal/channel/ -run "PickAuth|CramMD5" -v
--- PASS: TestPickAuthMechanismSecure (0.00s)
--- PASS: TestPickAuthMechanismInsecure (0.00s)
--- PASS: TestCramMD5NextNoLeak (0.00s)
ok  notice-service/internal/channel
```

> 说明：`authByExtensions` 的端到端分支需要真实 SMTP 服务器（`internal/integration/email_real_test.go`
> 依赖外部凭据），因此把**决策逻辑抽成纯函数**后用表驱动测试覆盖，避免回归只能靠人工看代码。

---

## M-2 `QUEUE_CLAIM_TTL` 无校验导致 `time.NewTicker(0)` 崩进程（medium，报告 §2.10）

### 问题

`QUEUE_CLAIM_TTL=0`（或负数）时，`internal/service/queue.go:192`（修复前）的
`time.NewTicker(q.cfg.ClaimTTL / 3)` 建在 **renew goroutine 内**——`time.NewTicker` 对非正周期 panic，
而 goroutine 的 panic **不会被 `process` 的 `recover` 捕获**，直接崩掉整个进程。
`recoverLoop` 特意做了 `interval < time.Second` 兜底，说明作者已意识到该风险，但 `process` 漏了。

### 改法

**① 配置加载期钳制（`internal/config/config.go`）**

新增 `clampInt`（`:272`，越界时打告警日志，避免静默改写）：

- `QUEUE_WORKERS` → `[1, 64]`（`:227`）
- `QUEUE_POLL_MS` → `[50, 60000]`（`:228`）
- `QUEUE_MAX_ATTEMPTS` → `[1, 20]`（`:229`）
- `QUEUE_CLAIM_TTL` → `[30, 3600]` 秒（`:231`）

**② 运行期兜底 + recover（`internal/service/queue.go`）**

- 新增 `pollInterval`（`:91`）、`renewInterval`（`:101`，`ClaimTTL/3` 且 ≥1s）、
  `recoverInterval`（`:112`，`ClaimTTL/4` 且 ≥1s）、`safeClaimTTL`（`:125`，非正值回退 120s，
  防止 `claimed_at < NOW() - INTERVAL 0 SECOND` 把刚认领的 job 当陈旧而重复发送）。
- renew goroutine 内加 `defer recover()`（`:239` 记录 `queue: renew goroutine panic for job %d`），
  并改用 `q.renewInterval()`；`workerLoop` 改用 `q.pollInterval()`（`:207`）；
  `recoverLoop` 改用 `q.recoverInterval()` + `q.safeClaimTTL()`。
- 说明：`Start()` 中 `QUEUE_WORKERS=0` 的**运行期语义保持不变**（`TestStaleRecoveryReclaims`
  等测试依赖「不起 worker，只跑恢复循环」），钳制只在配置加载层做。

### 文件:行号

| 文件 | 行 | 内容 |
|---|---|---|
| `internal/config/config.go` | 227-231 | 队列参数钳制 |
| `internal/config/config.go` | 272-283 | `clampInt` 实现（含越界告警） |
| `internal/service/queue.go` | 91-131 | `pollInterval` / `renewInterval` / `recoverInterval` / `safeClaimTTL` |
| `internal/service/queue.go` | 207 | worker 轮询周期兜底 |
| `internal/service/queue.go` | 236-241 | renew goroutine `defer recover()` |
| `internal/service/queue.go` | 320-336 | `recoverLoop` 使用兜底周期与 TTL |

### 验证证据

| 测试 | 断言 |
|---|---|
| `internal/config/config_test.go:199` `TestQueueConfigClamp` | `QUEUE_CLAIM_TTL` = 0 / -5 / 1 / 999999 → 30s / 30s / 30s / 3600s；`QUEUE_WORKERS` 0 / -3 / 1000 → 1 / 1 / 64；`QUEUE_POLL_MS` 0 / -1 / 99999999 → 50 / 50 / 60000；**config.yml 通道同样钳制** |
| `internal/service/queue_ttl_test.go:37` `TestQueueZeroClaimTTLDoesNotPanic` | `ClaimTTL` = 0 与 -1s 时 `Start()` + `process()` **不 panic**，job 正常置 done 且恰好发送 1 次 |
| `internal/service/queue_ttl_test.go:85` `TestQueueIntervalFallbacks` | 非正 TTL 的续租/恢复周期均 ≥1s；轮询周期 >0；`safeClaimTTL()` >0 |

```
$ go test ./internal/config/ -run QueueConfigClamp -v
--- PASS: TestQueueConfigClamp (0.01s)
$ go test ./internal/service/ -run "QueueZero|QueueInterval" -v
--- PASS: TestQueueZeroClaimTTLDoesNotPanic (0.17s)
    --- PASS: TestQueueZeroClaimTTLDoesNotPanic/0s (0.11s)
    --- PASS: TestQueueZeroClaimTTLDoesNotPanic/-1s (0.07s)
--- PASS: TestQueueIntervalFallbacks (0.00s)
ok  notice-service/internal/service
```

---

## 验证记录（WSL，Go 1.25）

```
$ wsl -e bash -lc 'export PATH=$PATH:/usr/local/go/bin; cd /mnt/f/project/notice-service && go build ./...'
exit 0

$ wsl -e bash -lc 'export PATH=$PATH:/usr/local/go/bin; cd /mnt/f/project/notice-service && go vet ./...'
exit 0

$ wsl -e bash -lc 'export PATH=$PATH:/usr/local/go/bin; cd /mnt/f/project/notice-service && go test ./internal/...'
ok  	notice-service/internal/channel	0.017s
ok  	notice-service/internal/config	0.042s
ok  	notice-service/internal/crypto	0.004s
ok  	notice-service/internal/database	0.057s
ok  	notice-service/internal/handler	18.216s
ok  	notice-service/internal/integration	0.297s
ok  	notice-service/internal/metrics	0.013s
ok  	notice-service/internal/model	0.006s
ok  	notice-service/internal/render	0.007s
ok  	notice-service/internal/repository	1.700s
ok  	notice-service/internal/scheduler	1.801s
ok  	notice-service/internal/service	9.991s
ok  	notice-service/internal/totp	0.005s
exit 0   （连续 3 次 `go clean -testcache` + 全量运行均 exit 0）

$ wsl -e bash -lc 'export PATH=$PATH:/usr/local/go/bin; cd /mnt/f/project/notice-service && go test -p 1 ./... -count=1'
ok  	notice-service/cmd/server	0.807s
ok  	notice-service/internal/channel	0.014s
ok  	notice-service/internal/config	0.050s
ok  	notice-service/internal/crypto	0.005s
ok  	notice-service/internal/database	0.039s
ok  	notice-service/internal/handler	21.473s
ok  	notice-service/internal/integration	0.242s
ok  	notice-service/internal/metrics	0.015s
ok  	notice-service/internal/model	0.005s
ok  	notice-service/internal/render	0.005s
ok  	notice-service/internal/repository	1.643s
ok  	notice-service/internal/scheduler	1.633s
ok  	notice-service/internal/service	11.052s
ok  	notice-service/internal/totp	0.005s
exit 0
```

> **验收口径（captain 裁定后纠正）**：仓库 CI（`.github/workflows/ci.yml`）与 `Makefile:45` 的实际口径是
> `go test -p 1 ./... -count=1`（串行），本表以此为准；`go test ./internal/...`（并行）为附加证据。
> captain 已在干净 HEAD worktree 独立复现「并行口径 3/3 失败、串行口径 exit 0」，确认根因是测试基建而非本次改动。

### 附带修复：并发测试的既有不稳定（测试基建，非业务逻辑）

`go test ./internal/...`（**不带 `-p 1`**）在本仓库**基线 HEAD 上同样不稳定**：
`cmd/server`、`internal/database`、`internal/handler`、`internal/integration`、
`internal/repository`、`internal/scheduler`、`internal/service` 七个包的测试**共用同一个
MySQL 库** `notice_service_test`（DSN 硬编码在 `handler_test.go:24`、`channel_service_test.go:18`、
`user_repo_test.go:15`、`cmd/server/reset_password_test.go:18` 等），并发执行时互相删对方的
`users` / `admin` / `channels` / `send_jobs` 行。

实测对照（同一台机器、同一数据库、均先 `go clean -testcache`）：

| 分支 | `go test ./internal/...`（并发，契约里的 verify 命令） |
|---|---|
| 基线 HEAD（`git worktree` 干净检出，未含本次任何改动） | **5/5 次失败**（`TestImportCreatesAndSkips`、`TestExportBundle`、`TestUserServiceDisableEnable` 等随机失败） |
| 本次改动后（未加测试锁） | 同样 5/5 次失败（同一批既有用例） |
| 本次改动后（**加了测试锁**） | **3/3 次通过** |

典型失败信息（基线同样出现，非本次引入）：

```
--- FAIL: TestUserServiceDisableEnable
    user_service_test.go:200: disabling builtin admin should fail, got 用户不存在
--- FAIL: TestExportBundle
    export_test.go:40: export = 500 body={"error":"渠道 \"c\" 配置解密失败，已中止导出…"}
--- FAIL: TestUserBatchToggle
    user_batch_test.go:51: batch enable = 401 body={"error":"账号已被禁用"}
```

因此新增了**纯测试期**的跨包串行锁（不参与任何业务逻辑，不被生产代码引用）：

- `internal/testutil/lock.go`：`Acquire()` 用 MySQL `GET_LOCK('notice_service_test_serial', 600)`
  取命名锁，返回释放函数；取锁失败（如本地无 MySQL）时**直接放行**，让各用例自身给出
  明确的连接错误，而不是把「环境缺失」伪装成锁超时。
- 七个包的 `TestMain`（`cmd/server/main_test.go`、`internal/{database,handler,integration,repository,scheduler,service}/main_test.go`）：
  `release := testutil.Acquire(); code := m.Run(); release(); os.Exit(code)`。

这把「共用同一个库」的包串行化，使 `go test ./internal/...` 稳定通过；
包内用例本来就是顺序执行（仓库无 `t.Parallel()`），无需改动任何既有测试。

**代价与维护约定（重要）**

1. **耗时**：7 个共用库的包从并行变为串行，`go test ./internal/...`（并行口径）总耗时
   从约 **20 s** 增至约 **30 s**（实测：加锁前 20–25 s，加锁后 27–30 s）。仓库标准口径
   `go test -p 1 ./... -count=1`（Makefile:45 / CI）本来就是串行，**耗时不受影响**。
2. **新增包必须同步加 TestMain**：今后任何**新写入 `notice_service_test` 库**的测试包，
   都必须照抄同样的 `TestMain`（`testutil.Acquire()` → `m.Run()` → `release()` → `os.Exit`），
   否则该包不受锁保护，会重现并发互删种子数据的随机失败。
   判断方法：包内 `*_test.go` 出现 `notice_service_test` 或 `sql.Open("mysql", ...)` 即需加锁。
3. **锁只在测试期生效**：`internal/testutil` 不被任何生产代码引用，`go build` 产物与运行时行为零影响；
   取锁失败（无 MySQL）时放行，不会把环境缺失伪装成超时。
4. **彻底解法（建议后续独立条目）**：给每个包分配独立测试库（`notice_service_test_handler` 等）
   或改用容器化一次性实例，即可去掉这把锁、恢复并行提速。

---

## 未做（明确超出本条范围，留给对应条目）

- **H-1 附加项**：`padKey` 截断改 SHA-256（报告 §2.3 H-1a）、`BootstrapAdmin` 复用 `validatePassword`（H-1b）、
  `config.yml` 的 `admin.pass: ""` 回落默认口令（H-1c）——属 `cmd/server/main.go` 与 `auth_service.go` 的独立改动。
- **M-4** `SetTOTPLastCounter` 改原子 CAS（报告 §2.12）——独立条目。
- **H-6** 强制关 2FA 二次验证（报告 §2.8）——独立条目。
