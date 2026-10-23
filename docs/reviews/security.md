# 安全视角审查 — notice-service v2.0.0..HEAD

- 审查范围：`git diff v2.0.0..HEAD`（52 提交 / 100 文件 / +4751 −396）
- 审查对象：Go(Gin) 后端 + Vue3 前端自托管通知发送服务
- 审查方式：逐文件 diff 走查 + 源码全文交叉验证 + 可执行验证（`go build ./...`、`go vet ./...`、`go test` 纯包、渲染/配置行为的临时探针程序，探针已删除，未改动任何业务代码）
- 严重度定义：`blocker` = 直接导致认证/授权或凭据失守，须发布前修；`high` = 在现实前提（管理员操作、抓包、DB 只读泄漏）下可被利用；`medium` = 需组合条件或影响有限；`low` = 加固项/纵深防御

> 说明：仓库内无可用 MySQL 实例（`.dev/mysql-run/mysqld.sock` 不存在，127.0.0.1:3306 上无响应），因此依赖 DB 的 `go test ./internal/scheduler`、`./internal/repository`、`./internal/handler` 未能执行；不依赖 DB 的 `internal/totp`、`internal/render`、`internal/config`、`internal/channel` 全部通过。下文凡标注「依据 MySQL 文档语义」的结论均未在本机实测，已单独说明。

---

## 0. 变更面概览（安全相关）

| 区域 | 关键提交 | 安全相关净效果 |
|---|---|---|
| 认证/2FA | `c1164cf`、`0ba5916`、`c7b021c` | TOTP 防重放、密钥加密落库、登录防枚举、重置令牌哈希化、2FA pending 令牌隔离；同时引入 1 个 blocker（见 S-1）与 2 个 high（S-2、S-3） |
| Webhook 签名 | `231c5d6` | 签名窗口去重 + 非 admin 不下发 `hmac_secret`；去重实现有容量缺陷（S-6） |
| 密钥与启动 | `b11aa30` | `STRICT_SECRETS` 拒绝弱密钥启动、Swagger 默认关闭；但弱密钥判定漏掉 `.env.example` 占位值（S-4） |
| SMTP | `582a9bd` | 按服务器宣告扩展协商 CRAM-MD5/LOGIN/PLAIN；引入明文降级与 CRAM-MD5 降级风险（S-5） |
| 队列 | `972a616` | 认领 CAS + 续租心跳 + 崩溃计入 attempts，正确性显著提升；`MarkFailed` 终态更新丢 CAS（S-7，low） |
| 部署 | `7465812`、`12c6f06`、`05af1b6` | 容器非 root、MySQL 8.0、3306 仅本机、健康检查去命令行明文；`publish.sh` 有 `.env` 解析风险（S-9） |
| 前端 | `8652790`、`9d172be`、`380ebfa`、`5b9be09`、`bec8595` | 会话窗口标记、401 去重、`redirect` 仅站内；登出未调用服务端接口（S-8） |

**确认得到修复（本视角复核通过）**：2FA pending 令牌不可用于业务 API（`internal/middleware/auth.go:24-29`）；角色/禁用状态每请求回查 DB（`auth.go:30-36`）；`Verify2FA` 增补 `!u.Enabled`（`auth_service.go:376`）；TOTP 同一时间步不可复用（`auth_service.go:440-447`）；重置令牌仅存 SHA-256（`user_service.go:66`、`auth_service.go:519`）；`ORDER BY` 全部走白名单（`audit_repo.go:118-129`、`task_log_repo.go:163-180`）；LIKE 通配符转义（`audit_repo.go:83-86`）；CSV 公式注入转义（`task_handler.go:419-431`）；邮件头 CRLF 净化与收件地址校验（`email.go:208-282`）；Swagger 默认关闭且 `Options` 零值不再被 if-true 吞掉（`router.go:47-53`、`config.go:180`）；CI 权限收窄为 `contents: read`（`ci.yml:3-5`）。

---

## 1. 发现清单

**归属说明**（帮助 captain 判断「本批变更引入」还是「存量未修」）：

| 归属 | 发现 |
|---|---|
| 本批变更**新引入** | S-1（`c1164cf` 引入加密列时漏了 `ForceEnable2FA`）、S-3（`c1164cf` 的升级路径）、S-4（`b11aa30` 的严格模式）、S-5（`582a9bd` 的认证协商）、S-6（`231c5d6` 的重放缓）、S-7（`972a616` 的 CAS 收尾）、S-9（`05af1b6`/`12c6f06` 新增脚本） |
| 存量问题，但本次**改动覆盖了对应文件/逻辑却未修复**（或只修了一半） | S-2（`c7b021c` 新增用户列表的单用户 2FA 启停入口，把同一零确认路径又多开了一个 UI 入口）、S-8（会话层被 `8652790`/`5b9be09` 重写）、S-11（`render.go` 本批大改，HTML 转义仍未补）、S-13（compose 本批改过）、S-14（本批**新增**截断逻辑，但截到 100 仍超列宽 50，属「修了但没修对」）、S-18（`c1164cf` 新增的防重放实现本身） |
| 存量、本次未触及（低优先，记录备查） | S-10、S-12、S-17、S-19、S-20、S-21、S-22 |

**编号说明**：S-17~S-20 为交叉核实过程中新增的发现；S-21/S-22 由原 S-15/S-16 改号而来（避免与新增编号冲突），内容不变。阅读时按严重度而非编号排序即可。

### S-1 管理员强制开启 2FA 时密钥仍走明文列，AES-GCM 加密对「被强制开启」的用户完全失效 — blocker

- **文件:行号**：`internal/service/user_service.go:309`（`SetTOTP(userID, "", secret, string(b))`），配合 `internal/repository/user_repo.go:211-216`
- **攻击前提与利用路径**：
  1. 管理员通过 `POST /api/users/{id}/2fa-enable`（`router.go:188`）或批量 `POST /api/users/batch-2fa-enable`（`router.go:186`）为用户开启 2FA；
  2. 该路径直接调用 `UserService.ForceEnable2FA`，写库时第一个实参（`secretEnc`）硬编码为空串 → 落到 `totp_secret` 明文列，`totp_secret_enc` 保持 NULL；
  3. 攻击者取得 DB 只读权限（备份泄露、只读账号、SQL 注入、误导出 dump）后直接读取 `users.totp_secret` 即可获得全部被强制开启 2FA 用户的 TOTP 种子，离线生成任意时刻动态码；
  4. 结合密码（可从撞库/钓鱼/`login.failed` 审计之外的途径获得，或管理员批量改密后统一初始口令），即可完整绕过 2FA 登录。
- **影响**：`c1164cf` 的核心安全目标（「DB 泄漏不再直接等于 2FA 失守」）在这一条最常用的运维路径上不成立；被强制开启 2FA 的账号（通常是普通用户/运营账号，且管理员批量为全员开启是常见合规操作）在**初始写入**时就是明文，且首次登录的加密升级也不会清掉明文（见 S-3），因此这类账号从开启到停用全程可被 DB 泄漏直接攻破。反而「用户自助开启」的账号从写入起就是加密的，安全属性出现倒挂。
- **验证**：`user_repo.SetTOTP` 的三个参数为 `(secretEnc, secretPlain, recoveryCodesJSON)`；`ForceEnable2FA` 传 `("", secret, ...)`，`storeTOTPSecret`（`auth_service.go:290-299`）则传 `(enc, "", ...)`。两条路径的写入列不同，可静态确认。
- **修复建议**：把 `UserService` 也注入 `crypto.Cipher`（或复用 `AuthService.storeTOTPSecret` 的加密逻辑），`ForceEnable2FA` 改为加密写入；`DisableTOTP` 同步清理 `totp_secret_enc`（见 S-3）。另建议在 `SetTOTP` 层直接拒绝 `secretEnc == "" && secretPlain != ""` 的调用（生产路径 fail-closed），把「明文降级」限制在测试可显式构造的构造器里。

### S-2 管理员对任意用户强制关闭 2FA 不需要任何二次确认，单个被盗的管理员会话即可批量摘除全员 2FA — high

- **文件:行号**：`internal/service/user_service.go:319-325`（`ForceDisable2FA`）、`internal/service/user_service.go:415-427`（`BatchForceDisable2FA`）、`internal/handler/user_handler.go:412-427`、路由 `internal/router/router.go:187`、`internal/router/router.go:189`
- **攻击前提与利用路径**：
  1. 攻击者获得任一管理员 JWT（XSS 之外的常见途径：管理员设备失窃、令牌被日志/代理记录、钓鱼）；
  2. 调用 `POST /api/users/batch-2fa-disable` 传全部用户 ID → 服务端仅校验 `operator.Role == "admin"`，无 TOTP/密码再确认，也无「是否内置 admin」保护（`BatchForceEnable2FA` 的注释明确写了「允许对内置 admin 账号操作」，`ForceDisable2FA` 同样无 `isDefaultAdmin` 拦截）；
  3. 2FA 全站关闭后，攻击者用已窃取的弱口令或密码喷洒即可长期驻留。
- **影响**：2FA 作为「账号失陷后的最后一道防线」，其关闭路径的强度低于开启路径（用户自助关闭需要 TOTP/备用码，`auth_service.go:344-364`）。管理员侧反而零验证，属于安全模型的不对称缺陷。批量接口使单次请求即可覆盖全站，且审计只留一条 `user.batch_2fa_disable`（`user_handler.go:310`）。
- **修复建议**：`ForceDisable2FA` / `BatchForceDisable2FA` 要求管理员提交自身 TOTP 或当前密码（若管理员自身未启用 2FA 则至少要求密码），并对内置 admin 账号拒绝强制关闭（与删除/禁用一致）；批量接口限制单次 ID 数量并逐条写审计。

### S-3 加密升级路径不清明文列：`ForceEnable2FA` 后的用户首次登录会把种子同时写进明文列与密文列并永久保留 — high

- **文件:行号**：`internal/repository/user_repo.go:225-231`（`SetTOTPSecretEnc` 只写 `totp_secret_enc`，**不清 `totp_secret`**）、`internal/service/auth_service.go:404-411`（验证成功后调用该升级）、`internal/repository/user_repo.go:240-245`（`DisableTOTP` 只置空明文列，不清密文列）
- **攻击前提与利用路径**：
  1. 管理员用 `POST /api/users/{id}/2fa-enable` 为用户开启 2FA（S-1：种子写入**明文列**，密文列为 NULL）；
  2. 该用户首次用认证器动态码登录成功 → `Verify2FA` 走 `!encrypted && s.cipher != nil` 分支（`auth_service.go:404-411`）→ 调用 `SetTOTPSecretEnc` 写入密文；
  3. 关键缺陷：`SetTOTPSecretEnc` 的 SQL 是 `UPDATE users SET totp_secret_enc=? WHERE id=?`，**明文列原封不动**。于是该行**同时**保存同一份种子的明文与密文，且此后任何一次登录都不会再清理明文列；
  4. 攻击者取得 DB 只读权限后直接读 `users.totp_secret` 即得种子——加密对「被管理员强制开启 2FA 的账号」永久失效（这类账号恰恰是批量合规开启的主力人群）；
  5. 附加缺陷：`DisableTOTP` 只置空明文列、不清密文列，已停用账号的种子材料长期驻留 DB，扩大泄漏面并违反数据最小化。
- **影响**：与 S-1 同源但独立成立——即便修好 S-1（让 `ForceEnable2FA` 直接写密文），只要「明文→密文升级」路径不清明文列，历史数据的升级也只是「多存一份」而非「消除明文」。当前实现下 `c1164cf` 的注释「历史明文数据首次验证成功后自动升级为密文」在数据层面是不成立的。
- **修复建议**：`SetTOTPSecretEnc` 改为 `UPDATE users SET totp_secret_enc=?, totp_secret=NULL WHERE id=?`（升级即消除明文）；`DisableTOTP` 同时置空 `totp_secret_enc` 与 `totp_last_counter`；把「两列至多一列非空」做成写入层不变量，并补一条集成测试断言升级后 `totp_secret IS NULL`。

### S-4 `STRICT_SECRETS=1` 的弱密钥判定漏掉 `.env.example` 的占位值，文档推荐流程下「严格模式」形同虚设 — high（**与产品 P3 同源**）

> **交叉核实结论：绕过成立，产品 P3 的结论正确。** 我已独立复核产品审查员给出的全部事实点，并补做了定量探针与 JWT 自签实测（见下），结论一致；本条在严重度上进一步定为 **high**（产品侧若定为 medium，建议合成时按 high 收敛，理由是可直接导致管理员身份伪造）。

- **文件:行号**：`internal/config/config.go:102-117`（`WeakSecretWarnings`）、`cmd/server/main.go:120-128`（strict 判定）、`.env.example:7`、`.env.example:8`、`.env.example:10`、`docker-compose.yml:34`、`docker-compose.yml:63`、`docker-compose.quickstart.yml:29`
- **产品侧事实点逐条复核**：
  - ① 精确相等判断 —— **确认**。`config.go:104`（JWT：`""`/`change-me`/`change_me`）、`config.go:107`（ENCRYPT：`""`/`0123456789abcdef0123456789abcdef`）、`config.go:110`（ADMIN_PASS：`""`/`admin123`/`change-me`）、`config.go:113`（JWT 长度 <32）。**四处全部为 `==` 字符串比较，无前缀/正则/熵判断。**
  - ② `.env.example:7-10` 的占位值不在集合内且长度 ≥32 —— **确认**。`JWT_SECRET`/`ENCRYPT_KEY` 均为 `please_change_me_to_32_bytes_random_hex`（39 字符），`ADMIN_PASS` 为 `please_change_me_to_a_strong_password`（36 字符）。
  - ③ `WeakSecretWarnings()` 返回空 → `log.Fatalf` 永不进入 —— **确认**。
- **定量探针结果**（临时程序，已删除，未改动业务代码）：

  | 构造的 Config | `len(WeakSecretWarnings())` |
  |---|---|
  | `.env.example` 三个原值 | **0** |
  | `.env.example` 原值 + 仅把 ADMIN_PASS 改强 | **0** |
  | JWT/ENCRYPT 用强随机 + `ADMIN_PASS=admin123` | 1（仅 ADMIN_PASS） |
  | 完全不设置（二进制默认 `change-me`/`""`/`admin123`） | 4 |
  | `JWT_SECRET` = 32 个 `a` | **0** |
  | `JWT_SECRET` = `please_change_me_...`（单字符重复 39） | **0** |

- **攻击前提与利用路径（具体化）**：
  - **前提**：目标按官方文档部署（`README.md:86`/`README.md:212` 的 `cp .env.example .env`，compose 默认注入 `STRICT_SECRETS=1`）且未改 `JWT_SECRET`。攻击者**无需任何凭据**——占位密钥就在公开仓库里。
  - **路径 A：JWT 伪造 → 直接管理员**。`JWT_SECRET` 是公开固定串，攻击者本地用 `HS256` 自签 `{"uid":1,"role":"admin"}` 即可。我已实测：用 `please_change_me_to_32_bytes_random_hex` 签发的令牌，经服务端 `VerifyToken` 等价逻辑校验 **通过（`err == nil`, `uid=1`, `role=admin`）**。中间件 `auth.go:30-44` 只在 `uid` 有效且用户启用时放行——`uid=1` 恰是 `BootstrapAdmin` 创建的首个账号（`auth_service.go:186`），因此**签名伪造的令牌直接获得全站管理员权限**，无需知道任何口令。
  - **路径 B：解密渠道配置**。`ENCRYPT_KEY` 同为公开串，经 `padKey`（`main.go:417-423`）截断为 `please_change_me_to_32_bytes_ran`；拿到 DB 备份/只读权限后可用该密钥解开全部渠道密文，取得 SMTP 授权码、机器人 webhook token、PushPlus token，进而以组织身份对外发信。
  - **路径 C：默认管理员口令**。`ADMIN_PASS` 若保留占位值即公开口令，可直接登录 `admin`。注意 `BootstrapAdmin`（`auth_service.go:176-195`）**完全不校验口令强度**，只在账号已存在时跳过——即首次启动就会把弱口令写进库。
- **影响**：三条路径任一成立即「完全失守」；且失败模式是**静默通过**（无告警、无日志、启动成功），运维在 compose 里看到 `STRICT_SECRETS=1` 会误以为已被保护。这是本批「安全默认」最危险的一处：**给出虚假的安全保证比没有保护更糟**。
- **附加发现（同一函数内，独立成立）**：
  - **`padKey` 截断碰撞**：`padKey` 对 ≥32 字节的密钥直接 `s[:32]`。实测 `please_change_me_to_32_bytes_random_hex` 与 `please_change_me_to_32_bytes_random_HEX` 的 `padKey` 结果**完全相同**（`"please_change_me_to_32_bytes_ran"`）。这意味着仅改大小写/尾部字符会被静默视为同一密钥——多实例轮换密钥时极易误判「已换」而实际未换，且 `ENCRYPT_KEY` 的实际熵被压到 32 字符以内，长度检查完全无法反映真实强度。
  - **`ADMIN_PASS` 强度检查几乎不存在**：实测 `ADMIN_PASS` 为 `x`/`1`/`a`/`admin`/`password`/`please_change_me` 时 `WeakSecretWarnings()` **均返回 0**。`validatePassword`（`password.go:20-50`，要求 12 位 + 四类字符）只用于「用户创建/管理员改密/自助改密」，**独独漏掉 bootstrap 管理员**——最强的口令策略恰好没有约束权限最高的账号。
  - **`ADMIN_USER` 可配但 `isDefaultAdmin` 硬编码 `"admin"`**：`config.go:165` 允许 `ADMIN_USER` 改为任意值，而 `isDefaultAdmin`（`user_service.go:29-31`）写死 `u.Username == "admin"`。若部署时把 `ADMIN_USER` 设为 `root`，则该账号**失去全部「内置 admin 保护」**：可被其他管理员删除/禁用/改密（`user_service.go:135`、`161`、`261`、`441`），也可被 `GenerateResetToken` 重置（`user_service.go:59`）——原本「防锁死」的设计目标被静默绕过。
  - **空值 fail-open**：实测 `config.yml` 写 `admin: pass: ""` 时 `firstNonEmpty` 回落为 `admin123`，即「留空」不等于「无口令」而是**回落到默认弱口令**，并且此时 `WeakSecretWarnings` 只报 ADMIN_PASS 一条（JWT/ENCRYPT 若已设强值则不报），运维很可能只看到一条无关紧要的告警而忽略。
- **修复建议**（按性价比排序）：
  1. **`.env.example` 把三个密钥值留空**（`JWT_SECRET=`），让 compose 的 `${VAR:?}` 守卫在启动前直接拦住——这是成本最低、最不容易被绕过的修法；
  2. `WeakSecretWarnings` 增加**前缀黑名单**：命中 `please_change_me`、`change_`、`change-`、`your_`、`example`、`placeholder` 即告警；
  3. 增加**低熵检测**：对 `JWT_SECRET`/`ENCRYPT_KEY`/`ADMIN_PASS` 计算字符多样性（如去重字符数 ≥ 12）或 Shannon 熵下限，可同时挡住「32 个 a」这类通过长度检查的弱值；
  4. `BootstrapAdmin` 复用 `validatePassword`（或至少在 `STRICT_SECRETS=1` 时强制），把最强策略也覆盖到权限最高的账号；
  5. `padKey` 改为对超长密钥做 SHA-256 摘要（而非截断），消除碰撞并保留全部熵；
  6. `isDefaultAdmin` 改为接收 `adminUser` 配置或按 `ID == 1 && CreatedAt == 最早` 判定，使「内置管理员」语义与配置一致。

### S-5 SMTP 认证协商引入明文降级与 CRAM-MD5 优先，凭据在特定配置下可被中间人截获或被动降级 — high

- **文件:行号**：`internal/channel/email.go:104-132`（`authByExtensions`）、`internal/channel/email.go:116-120`（`case advertised["CRAM-MD5"]` / `case advertised["LOGIN"]`）、`internal/channel/email.go:84-87`
- **攻击前提与利用路径**：
  1. **LOGIN 在 TLS 前就被选中**：`authByExtensions` 的分支顺序只依据服务器宣告的 `AUTH` 扩展，**没有**先判断 `secure`。若渠道配置 `allow_insecure=true`（内网明文中继，官方支持的场景）且服务器宣告 `LOGIN`，则用户名/口令以 base64（等价明文）走网络——这是设计内的取舍，但分支里没有任何日志或拒绝，运维无法感知；
  2. **CRAM-MD5 降级为 HMAC-MD5**：`cramMD5Auth`（`email.go:135-148`）使用 `crypto/md5` 作为 HMAC 摘要，攻击者若能在 TLS 建立前注入（例如 `allow_insecure` 明文连接 + 中间人扮演 SMTP 服务器并宣告 `CRAM-MD5`），可对已知挑战做离线字典攻击（MD5-HMAC 无迭代、无盐），比 bcrypt 类口令哈希弱得多；
  3. **明文连接上宣告 `CRAM-MD5` 时不再检查 `allow_insecure`**：`case advertised["CRAM-MD5"]` 直接认证，绕过了 `PLAIN` 分支才有的 `secure || allow_insecure` 守卫。虽然 CRAM-MD5 不上线口令，但响应可被重放（服务器挑战随机性依赖对端，恶意服务器可固定挑战 → 重放响应即完成认证，进而以受害者身份发信）；
  4. **未宣告 `AUTH` 时在 TLS 上回落 `PlainAuth`**（`email.go:127-131`）：若服务器宣告了 `AUTH` 但内容为空串，`advertised` 为空 map，落到「未宣告」分支，行为与 PLAIN 相同——这条路径本身在 TLS 上安全，但日志缺失使配置漂移不可见。
- **影响**：邮箱授权码是长期凭据（多数邮箱服务商不支持单独吊销），一旦泄露可长期冒名发信、被用于钓鱼（借助受害组织域名信誉）。`allow_insecure` 是显式配置，但当前实现让「显式接受明文」的范围从「PLAIN 口令」扩大到了「任何认证方式的握手细节」。
- **修复建议**：`authByExtensions` 增加参数顺序守卫——`secure == false` 时只允许 `CRAM-MD5`，`LOGIN`/`PLAIN` 必须 `allow_insecure=true` 且每次发送打一条 warning 级日志；`cramMD5Auth` 换成 `crypto/sha256` 实现 CRAM-SHA256（若服务器支持）或至少在对端宣告 `CRAM-MD5` 但连接非 TLS 时拒绝；对「宣告 AUTH 但列表为空」显式返回错误而不是回落 PLAIN。

### S-6 Webhook 防重放缓存在容量打满时整体清空，攻击者可主动制造清空窗口后重放 — medium

- **文件:行号**：`internal/handler/webhook_handler.go:156-189`（`replayCache` / `seen` / `newReplayCache(4096)`）、`internal/handler/webhook_handler.go:215-218`（去重调用）
- **攻击前提与利用路径**：
  1. 攻击者已能构造合法签名（即已掌握 `hmac_secret`，或本身是该 webhook 的合法调用方但想重复触发）；
  2. 攻击者在 5 分钟签名窗口内，用**不同的** `X-Timestamp`/不同 body 组合向同一 `api_key` 发起约 4096 次合法签名请求（每次签名不同 → 缓存 key 不同），把 `entries` 撑到 `max`；
  3. `seen()` 在 `len(entries) > max` 时先删过期项，若仍超限则 `c.entries = make(map[string]time.Time)` —— **整表重建**，此前记录的全部「已见签名」一次性丢失；
  4. 攻击者重放任意一个仍在时间窗内的旧签名请求 → 去重命中失败 → 任务被重复触发。
- **影响**：`231c5d6` 声称的「窗口内防重放」在主动攻击下可被清除。多实例部署下本就有「重放需落在同一实例」的缺口（注释已承认），叠加整体清空后该防护接近装饰性。实际危害取决于任务副作用（重复发信/短信/群机器人消息），属可用性 + 业务完整性风险。
- **修复建议**：容量打满时不要整体清空，改为**按 TTL 分桶淘汰最旧**（`container/heap` 或环形缓冲）或对 `len(entries) >= max` 直接返回「拒绝」（fail-closed，宁可拒绝新签名也不放行重放）；更彻底的做法是把去重键写入 DB/Redis（`INSERT ... ON DUPLICATE KEY` 天然原子，多实例也生效），与限流同源。

### S-7 `MarkFailed` 的终态更新丢失 CAS 守卫，接管后的旧实例仍可改写 job 状态 — low

- **文件:行号**：`internal/repository/send_job_repo.go:146-168`（`MarkFailed`），关键在 `send_job_repo.go:158-165` 两条后续 `UPDATE ... WHERE id=?` 没有 `status='claimed' AND claimed_by=?` 条件
- **攻击前提与利用路径**：
  1. 实例 A 认领 job 后发送超时（`ClaimTTL=120s`），`RecoverStale` 把 job 放回 pending 并由实例 B 重新认领（`send_job_repo.go:188-205`）；
  2. 实例 A 此时返回失败，`MarkFailed` 的第一条 `UPDATE ... LAST_INSERT_ID(attempts+1) ... WHERE id=? AND status='claimed' AND claimed_by=?` 因 CAS 不命中返回 `n != 1` → 函数提前 `return false, nil`，**这条路径是安全的**；
  3. 但若 CAS 命中（A 仍持有）后、第二条 `UPDATE ... SET status='failed'` 执行前，B 恰好完成接管（时序窗口在两条语句之间），A 会把**B 正在处理的 job** 直接置为 `failed` 并清空 `claimed_by`；B 随后 `MarkDone` 的 CAS 不命中，发送结果被丢弃但任务已发出。
- **影响**：极端竞态下出现「消息已发但 job 记为 failed」或「job 被错误终止」，影响重试语义与运维判断；无越权/凭据风险，故定 low。`process()` 侧（`queue.go:246-262`）已用 `completed` 标记规避了任务时间戳误更新，但 repo 层的两条语句仍是分开的。
- **修复建议**：把 `MarkFailed` 的终态更新合并为单条带 CAS 的 `UPDATE ... SET status=IF(attempts+1 >= ?, 'failed', 'pending'), next_retry_at=... WHERE id=? AND status='claimed' AND claimed_by=?`，或把两条语句放进同一事务并对 `send_jobs` 行加锁（`SELECT ... FOR UPDATE`）。

### S-8 前端登出不调用服务端吊销接口，被盗令牌在 24h TTL 内继续有效 — medium

- **文件:行号**：`web/src/components/AppLayout.vue:328-340`（`onLogout` 只做 `auth.logout()` + 路由跳转）、`web/src/stores/auth.ts:27-31`（`logout` 仅清本地）、`web/src/api/index.ts:25-39`（`authApi` 未定义 `logout`）、对比后端已提供 `POST /api/auth/logout` → `RevokeAllSessions`（`auth_handler.go:153-161`）
- **攻击前提与利用路径**：
  1. 用户在公共/共享设备上点击「退出登录」；
  2. 前端只清 `localStorage`/`sessionStorage`/会话 Cookie（`session.ts:99-104`），**不请求** `/api/auth/logout`；
  3. 该用户令牌若在此前已被窃取（例如浏览器同步、恶意扩展、代理日志、屏幕录制），在服务端 `session_revoked_at` 基线未推进的情况下，令牌直到 24h 过期都可用；
  4. 攻击者可在受害者「已登出」后继续以受害者身份读取全部共享数据（渠道/模板/任务/日志/审计）并执行管理员操作（若受害者为管理员）。
- **影响**：服务端已实现的「登出即全端吊销」能力（`middleware/auth.go:37-41` 的 `SessionRevokedAt` 校验）在前端主路径上从未被触发，安全能力闲置；改密路径（`Settings.vue:526`）同样只清本地，但改密会推进 `session_revoked_at`（`user_repo.go:84-88`），因此该路径实际安全。
- **范围说明**：该缺口在 `v2.0.0` 即存在（当时 `auth.ts` 的 `logout` 同样只清 `localStorage`），本批 `8652790`/`5b9be09` 重写了会话存储层（新增 `utils/session.ts`）但未顺带补上服务端调用，属于「改动正好覆盖该文件却未修复」的情形，故仍列入本次发现。
- **修复建议**：`authApi` 增加 `logout: () => client.post('/auth/logout')`，`onLogout` 中 `await authApi.logout().catch(() => {})` 后再清本地（失败也要清，但保留日志）；或在 401 拦截器与 `beforeunload` 之外，统一在「主动登出」入口调用服务端接口。

### S-9 `publish.sh` 从 `.env` 解析 `IMAGE_PREFIX` 的方式对含空格/引号/`=` 的值不安全 — low

- **文件:行号**：`deploy/publish.sh:29-34`（`.env` 解析与 `--build-arg` 拼接）
- **攻击前提与利用路径**：`grep -E '^IMAGE_PREFIX=' .env | tail -1 | cut -d= -f2-` 不做引号剥离、不校验内容，随后作为 `--build-arg IMAGE_PREFIX="${IMAGE_PREFIX}"` 传入 `docker build`。若 `.env` 中该值被写成多行注入形式（`IMAGE_PREFIX=x` 后接恶意行）或含 shell 元字符，`--build-arg` 会把它原样带入 Dockerfile 的 `FROM ${IMAGE_PREFIX}alpine:3.21`；配合 `IMAGE_PREFIX` 可控的 `.env`（例如从他人处拷贝的部署包），可让构建阶段拉取攻击者控制的同名基础镜像。
- **影响**：需要攻击者已能写 `.env`（即已具备部署机写权限），故定 low；但 `publish.sh` 是**发布链路**，其产物会带 `latest` tag 推到 Docker Hub（`publish.sh:61-65`），一次污染影响所有拉取 `latest` 的下游部署，影响半径远大于触发条件。
- **修复建议**：用白名单校验 `IMAGE_PREFIX`（正则 `^[a-z0-9./-]*/$|^$`）；`set -euo pipefail` 已启用，建议再加 `--build-arg` 前的显式拒绝；更稳妥的是不读 `.env`，只接受显式环境变量。

### S-10 任务导入/导出把 `hmac_secret` 与 `api_key` 明文往返，导入接口可写入任意签名密钥 — medium

- **文件:行号**：`internal/service/export_service.go:157-173`（导入时 `SetAPIKey`/`SetHMACSecret` 原样回填）、`internal/service/task_service.go:311-319`、`internal/handler/export_handler.go:44-70`、路由 `router.go:196-197`
- **攻击前提与利用路径**：
  1. 导入接口仅要求 `role == "admin"`，`ExportBundle` 完全由请求体决定（`export_handler.go:56-62`），`Tasks[].APIKey` / `Tasks[].HMACSecret` 直接来自 JSON；
  2. 攻击者（或被诱导的管理员）提交一份备份，把某任务的 `hmac_secret` 设为攻击者已知值、`require_signature` 置 true；
  3. 由于 `SetAPIKey`/`SetHMACSecret` 直接 `UPDATE ... WHERE id=?` 且不校验调用者是否原属主（`task_repo.go:224-232`），攻击者即可用已知密钥为任意任务生成合法签名，在签名窗口内触发该任务；
  4. 反向：管理员导出的备份 JSON 含全部 `hmac_secret`/`api_key` 明文（`export_service.go:37-74` 的 `List(userID, true)` + 任务 `fill`），备份文件一旦落入对象存储/工单附件即等于全部 webhook 凭据泄露。
- **影响**：凭据在「备份」这一非敏感语境中被明文扩散；导入侧的写入能力使 webhook 签名从「独立密钥」退化为「管理员可任意设定」，削弱了 `231c5d6` 建立的凭据分离价值。属管理员权限内的操作，故 medium。
- **修复建议**：导出时对 `api_key`/`hmac_secret` 做脱敏（或要求二次确认 + 单独的高敏感导出端点）；导入时**忽略**外部提供的凭据字段，改为导入后重新生成并在响应中一次性返回；若确需保留（迁移场景），要求显式 `preserve_credentials=true` 并写高危审计。

### S-11 渲染管线不转义 HTML，HTML/邮件渠道正文可注入任意标签（含 `javascript:` 链接）— medium

- **文件:行号**：`internal/render/render.go:24-29`（`ToHTML` 直接 `markdown.ToHTML`，未启用 `html.SkipHTML`）、`internal/render/render.go:14-22`（`RenderVariables` 无转义直接替换）、`internal/channel/email.go:218`、`internal/channel/wechat.go:76-77`
- **攻击前提与利用路径**：
  1. 模板 `content_md` 由管理员编写，但变量值来自 **webhook 调用方**（`webhook_handler.go:81-107` 的 `variables`）或任务级变量；
  2. `RenderVariables` 把 `{{name}}` 替换为变量原值，随后 `ToHTML` 直接解析 Markdown —— 探针实测 `<script>alert(1)</script>`、`<img src=x onerror=alert(2)>`、`[x](javascript:alert(1))`、`<a href="javascript:alert(1)">` **全部原样出现在输出 HTML 中**（`<p><a href="javascript:alert(document.domain)">x</a></p>`）；
  3. 该 HTML 被 `ToHTMLEmail` 包进邮件骨架后经 SMTP 投递，也被 PushPlus `html` 模板使用（`wechat.go:76-77`）；
  4. 邮件客户端多数会过滤脚本，但**链接协议**（`javascript:`）在部分 Webmail/移动客户端与「在浏览器中查看」场景可执行；PushPlus 的 HTML 模板在浏览器内渲染时同理。前端预览 `MarkdownPreview.vue:29-31` 已用 DOMPurify 兜底，但**发送链路没有**。
- **影响**：webhook 调用方（可能只是拿到 `api_key` 的第三方系统）可向组织内部收件人投递带任意标签/恶意链接的「官方通知」，形成钓鱼跳板；结合 `img` 标签可做打开追踪。因收件端过滤程度不一，定 medium。
- **修复建议**：`ToHTML` 增加 `parser.CommonExtensions | parser.SkipHTML`（gomarkdown 会丢弃原始 HTML），并对 `RenderVariables` 的替换值做 HTML 转义（或对 `ToHTML` 输入先转义、只允许 Markdown 语法）；对 `javascript:`/`data:`/`vbscript:` 协议在渲染后做一次 URL 白名单过滤；`ToPlainText` 同步做标签剥离（当前会把 `<script>` 原样保留进短信文本）。

### S-12 邮件主题/收件人变量替换在 `sanitizeHeader` 之前不做长度与字符约束，可放大为超大头部 — low

- **文件:行号**：`internal/channel/email.go:244-261`（`buildMailFrom` / `sanitizeHeader`）、`internal/service/notification_service.go:97`（`render.RenderMessage` 产出 subject）
- **攻击前提与利用路径**：webhook 变量可注入超长主题（仅受 `MaxBodyBytes=1MB` 限制，`router.go:47`）；`sanitizeHeader` 只删 CR/LF，不限制长度，SMTP `DATA` 阶段会写出一个超长 `Subject:` 头。部分 MTA 对超长头部会截断或拒收，造成发送失败并触发重试（`queue.go` 的 job 级重试），形成低强度资源消耗。
- **影响**：CRLF 注入已被正确阻断（这点做得好），残留的是长度与 RFC 2047 编码缺失（非 ASCII 主题未做 encoded-word，可能被部分客户端误判），影响为可用性/兼容性，故 low。
- **修复建议**：对 `Subject` 做 rune 长度上限（如 200，与 `tasks.subject VARCHAR(200)` 对齐）并按 RFC 2047 编码非 ASCII 主题；收件地址长度上限同样补齐。

### S-13 可信代理默认放宽到 `172.16.0.0/12`，Webhook IP 白名单与审计 IP 可被同网段容器伪造 — medium

- **文件:行号**：`docker-compose.yml:35`、`docker-compose.yml:64`、`docker-compose.quickstart.yml:30`、`.env.example:17`、`internal/config/config.go:178`（二进制默认仅 `127.0.0.1,::1`，compose 覆盖为含网桥网段）
- **攻击前提与利用路径**：
  1. compose 默认把 `TRUSTED_PROXIES` 设为 `127.0.0.1,::1,172.16.0.0/12`，注释说明是为「宿主机 Nginx → 容器」场景取真实 IP；
  2. 若部署形态是**端口直连**（`docker-compose.yml:22` 的 `8080:8080` 映射，注释已提示「若纯直连建议改窄」），同一 Docker 网桥上的任意其它容器（`172.16.0.0/12` 内）即被视为可信代理；
  3. 攻击者在该网段内起一个容器，向 `notice-service:8080` 发请求并携带 `X-Forwarded-For: <白名单 IP>`，gin 的 `validateHeader`（`gin@v1.12.0/gin.go:483-502`）会从右往左找到第一个不可信 IP —— 由于来源本身可信，最终采信的正是攻击者伪造的值；
  4. 结果：`webhook_handler.go:118-136` 的 `ipAllowed` 被绕过（可对配置了 `allowed_ips` 的任务发起触发），审计日志（`audit.go:38`）与发送日志的 `trigger_ip` 被污染，事后取证失效。
- **影响**：需要攻击者能在 Docker 网桥内起容器（多租户宿主机、CI runner、被拿下的邻接容器），前提不算苛刻但非公网直接可达；`allowed_ips` 是任务级的安全边界，绕过即越权触发通知，故 medium。
- **修复建议**：compose 中把默认值收紧为 `127.0.0.1,::1`，把「加网桥网段」写成注释里的可选步骤；或在应用侧检测「配置了可信代理但未配置反代」时打 warning；更稳妥的是改用 `TRUSTED_PROXIES` 显式必填（`:?` 守卫），不给默认值。

### S-14 审计日志 `username` 列宽 50 与代码截断 100 不一致，超长用户名写入失败会静默丢弃审计 — low

- **文件:行号**：`internal/handler/audit.go:44-55`（截断到 100 rune）、`internal/database/migrations/005_audit_logs.sql:5`（`username VARCHAR(50)`）
- **攻击前提与利用路径**：`login.failed` 的 `username` 来自请求体（`auth_handler.go:45`）。攻击者提交 51–100 字符的用户名 → 截断到 100 后仍超列宽 → MySQL 严格模式下 `INSERT` 报 `Data too long` → `auditActor` 的 `_ =` 吞掉错误 → **该次登录失败尝试完全不落审计**。攻击者可借此在审计日志中制造爆破「盲区」（虽然限流仍生效，但事后无法回溯来源与尝试量）。
- **影响**：审计完整性缺口，且代码注释明确写了「防止未认证流量用超长用户名刷写审计表」——目标达成了一半（防刷写），但引入了静默丢弃。属 low（不影响认证本身）。
- **修复建议**：截断长度与列宽对齐（50），或把列改为 `VARCHAR(100)`；`auditActor` 对写失败至少 `log.Printf` 一次，便于发现列宽漂移。

### S-17 `ADMIN_USER` 可配置但 `isDefaultAdmin` 硬编码 `"admin"`，「内置管理员保护」可被部署配置静默解除 — high

- **文件:行号**：`internal/service/user_service.go:29-31`（`return u != nil && u.Username == "admin"`）、`internal/config/config.go:165`（`AdminUser: firstNonEmpty(os.Getenv("ADMIN_USER"), ...)`）、调用点 `user_service.go:59`、`135`、`161`、`188`、`210`、`241`、`261`、`441`、`444`
- **攻击前提与利用路径**：
  1. 部署方按「安全加固」直觉把 `ADMIN_USER` 从 `admin` 改为 `root`/`opsadmin`（`ADMIN_USER` 本就是文档化的可配项）；
  2. `isDefaultAdmin` 仍然只认 `username == "admin"`，于是真正的高权限账号**不是**「内置 admin」；
  3. 任一**普通管理员**（role=admin 但非内置）即可对该账号执行：删除（`user_service.go:135` 的 `target.Role == "admin" && !isDefaultAdmin(operator)` 守卫——此处 `isDefaultAdmin(operator)` 对普通管理员为 false，但目标也不是「内置 admin」，故仅受「普通管理员不能删除管理员」限制；若攻击者本身是内置 `admin` 则完全无限制）、禁用、**重置密码**（`user_service.go:261` 的 `isDefaultAdmin(target)` 不成立 → 允许重置）、生成重置令牌（`user_service.go:59` 不成立 → 允许）；
  4. 攻击者用普通管理员账号（或社工到的管理员会话）重置该高权限账号密码 → 接管。
- **影响**：`isDefaultAdmin` 的全部保护（不可删除、不可禁用、不可改密、不可生成重置令牌、角色不可改）——也就是「防止把系统管理员锁死/被接管」这套设计——在**非默认 `ADMIN_USER` 的部署里全部失效**，而这类改名恰恰是安全加固的常见做法。属配置与代码语义脱节导致的授权边界漂移。
- **修复建议**：`isDefaultAdmin` 不应硬编码用户名，改为由 `Config.AdminUser` 注入（`UserService` 增加 `adminUser` 字段并在构造时传入），或用「`ID == 1`（bootstrap 首建）+ 角色为 admin」判定；补一条测试：`ADMIN_USER=root` 时 `root` 账号仍不可被删除/禁用/改密。

### S-18 2FA 防重放是「读快照→无条件写」，并发下同一动态码可被消费两次 — medium

- **文件:行号**：`internal/service/auth_service.go:375`（`GetByID` 取 `u.TOTPLastCounter` 快照）、`auth_service.go:440-447`（比较后调用 `SetTOTPLastCounter`）、`internal/repository/user_repo.go:218-223`（`UPDATE users SET totp_last_counter=? WHERE id=?`，**无 CAS 条件**）
- **攻击前提与利用路径**：
  1. 攻击者已截获一个有效的 6 位动态码 + 一个有效的 pending token（两者可同时获得：pending token 在登录第一步响应体里返回，动态码可被钓鱼/肩窥/或从受害者设备同步）；
  2. 在同一个 30 秒时间步内，**并发**发出 N 个 `POST /api/auth/2fa/verify` 请求；
  3. 每个请求都先 `GetByID` 读到相同的 `lastCounter`（快照），都通过 `counter > lastCounter` 检查，然后各自 `UPDATE ... SET totp_last_counter=counter`——后写覆盖先写，**N 个请求全部成功**；
  4. 结果：一个一次性动态码签发了 N 个有效 JWT。若攻击者只想「抢在受害者之前用掉」则无需并发；若想**延长驻留**，并发可把单次动态码放大成多个会话。
- **影响**：`c1164cf` 的防重放目标（「同一时间步的验证码只能消费一次」）在并发下不成立。与 S-6 同属「防护存在但可被绕过」。注意 `SetTOTPLastCounter` 的注释写的是「更新最近成功验证的时间步计数器」，语义上应是一次性消费（compare-and-set），实现却是无条件赋值。
- **修复建议**：把校验与写入合并为一条原子 CAS：`UPDATE users SET totp_last_counter=? WHERE id=? AND totp_last_counter < ?`，按 `RowsAffected() == 1` 判定是否消费成功（失败即拒绝本次登录）；或包进事务并 `SELECT ... FOR UPDATE`。这与队列层 `Claim`/`MarkDone` 已经采用的 CAS 模式一致，属同类修复。

### S-19 渠道配置不校验 URL scheme，管理员可配置 `file://`/任意内网地址，构成 SSRF（含云元数据）— medium

- **文件:行号**：`internal/channel/webhook.go:12`（`var webhookClient = &http.Client{Timeout: 15 * time.Second}`，无 Transport/无 scheme 限制）、`internal/channel/wechat.go:43-45`（`pushplus_url` 可任意覆盖 `endpoint`）、`internal/service/channel_service.go:58-72`（`Create` **只校验类型存在**，不调用 `ValidateConfig`）、`channel_service.go:74-94`（`Update` 同样不调用）
- **攻击前提与利用路径**：
  1. 渠道 `Create`/`Update` 完全不执行 `ValidateConfig`（只有 `Test` 与 `Send` 路径会），因此 `webhook_url` / `pushplus_url` 可以写入**任意字符串**，包括 `http://169.254.169.254/latest/meta-data/`、`http://10.0.0.5:6379/`、`file:///etc/passwd`；
  2. 之后任一绑定该渠道的任务发送时，`postJSON`/`PostForm` 直接请求该地址；响应体会被读入（`io.LimitReader(..., 64*1024)`）并**写进 `task_logs.response` / 错误信息**（`notification_service.go:160-173` 落库 `err.Error()`）；
  3. 有日志读权限的用户（所有登录用户均可 `GET /api/logs`、`GET /api/logs/:id`）即可读取该响应 → 云主机元数据/内网服务响应外带。
- **影响**：需要管理员权限才能配置渠道，故非「未授权 SSRF」；但管理员配置「webhook 地址」时通常不会意识到自己在给服务端开一个任意 URL 请求器，且**响应内容会落进所有登录用户可读的日志**，形成「管理员配置 → 普通用户外带」的权限跨越。`file://` 在 Go 的 `http.Client` 下会报错（`unsupported protocol scheme`），不构成本地文件读取，但错误信息里会带上路径。
- **修复建议**：`ChannelService.Create/Update` 调用 `ValidateConfig`；在 `postJSON`/`sendPushPlus` 前做 URL 校验——仅允许 `https`（或显式白名单域名），拒绝环回/私有/链路本地地址（`net.ParseIP(...).IsPrivate()/IsLoopback()/IsLinkLocalUnicast()`）；`task_logs` 的 `response` 字段对非 2xx 响应体做截断或脱敏。

### S-20 登录 `TrimSpace(password)` 与改密/创建路径不 trim 不对称，含首尾空格的强口令会被静默截断 — low

- **文件:行号**：`internal/service/auth_service.go:216-217`（`Login` 对 `username` 与 **`password`** 都 `TrimSpace`）、对比 `auth_service.go:490`（`ChangePassword` 用 `oldPass` 原值）、`auth_service.go:515`（`ResetPassword` 用 `newPass` 原值，仅 `validatePassword`）、`user_service.go:87-108`（`Create` 用原值）
- **攻击前提与利用路径**：
  1. 管理员/用户创建账号时口令为 `" Passw0rd!2026 "`（首尾各一个空格，`validatePassword` 的 12 位与四类字符检查均通过）；
  2. 该口令以**含空格**的形式 bcrypt 后入库；
  3. 用户登录时输入 `" Passw0rd!2026 "`，`Login` 先 `TrimSpace` → 变成 `"Passw0rd!2026"` → 与库中哈希不匹配 → **永久无法登录**（用户会以为密码错误）；
  4. 反向：若用户创建时口令为 `"Passw0rd!2026"` 而后在别处用 `"Passw0rd!2026 "` 登录，则 trim 后反而能成功——两个不同的口令字符串在登录侧被视为等价，口令空间被压缩。
- **影响**：可用性缺陷为主（合法用户被锁在门外），安全性上是「口令归一化策略不一致」——同一系统内两个组件对同一秘密采用不同规范化规则，属 CWE-20 类输入处理不一致，low。
- **修复建议**：统一策略——要么全链路都不 trim（并在 `validatePassword` 里拒绝首尾空白），要么全链路都 trim（`ChangePassword`/`ResetPassword`/`Create` 同步）；推荐前者，并在 UI 上提示。

### S-21 CSP 允许 `script-src 'unsafe-inline'`，配合前端 `v-html` 存在 XSS 放大面 — low

- **文件:行号**：`internal/router/router.go:251-253`（CSP）、`web/src/components/MarkdownPreview.vue:29-31`（DOMPurify 兜底，已正确）、`web/src/utils/session.ts:23-25`（会话 Cookie 无 `HttpOnly`/`Secure`）
- **攻击前提与利用路径**：
  1. CSP 的 `script-src` 含 `'unsafe-inline'`（注释解释为「内联主题脚本」），这使 CSP 无法阻断内联脚本注入；
  2. 当前唯一的 `v-html` 出口（`MarkdownPreview.vue`）已过 DOMPurify，因此**暂无直接利用链**；但任何未来新增的 `v-html`、或第三方组件内的 `innerHTML`，都会因 CSP 无兜底而直接成为 XSS；
  3. `notice_session` Cookie（`session.ts:23`）是纯前端标记，不含凭据，故未设 `HttpOnly` 合理；但令牌存 `localStorage`，任意 XSS 即可外带。
- **影响**：属纵深防御缺失，当前无已知利用链，定 low。CSP 的 `connect-src 'self'` 确实限制了外带（这点做得好），但 `img-src 'self' data:` 也阻止了图片外带，攻击者仍可用 `location.href` 跳转外带。
- **修复建议**：把内联脚本改为带 nonce/hash 的 `script-src 'self' 'nonce-...'`；或至少拆分「Swagger 页面」与「应用页面」的 CSP；长期考虑把令牌迁到 `HttpOnly` Cookie + CSRF token（当前纯 Bearer 模式对 CSRF 天然免疫，这是优点，迁移需权衡）。

### S-22 Swagger 在测试/自定义路由构造下仍默认开启，若被误用于生产暴露完整 API 面 — low

- **文件:行号**：`internal/router/router.go:45-53`
- **攻击前提与利用路径**：`NewRouter` 的默认 `Options{SwaggerEnabled: true}` 仅在「调用方不传 Options」时生效；生产 `main.go:254-262` 显式传入 `cfg.SwaggerEnabled`（默认 false，`config.go:180`），因此生产安全。但任何新增的调用方（脚本、内部工具、测试之外的 main 变体）忘记传 Options 就会默认暴露 `/swagger`，其中含全部路由、参数与示例（`auth_handler.go:31` 的示例仍是 `admin/admin123`）。
- **影响**：当前无生产暴露；属「默认值方向不对」的加固项，low。
- **修复建议**：默认改为 `false`，测试显式开启（与注释里「测试方便开启 Swagger」的目标可通过测试代码设置 `Options` 达成）。

---

## 2. 复核确认的正面改进（不重复计入发现）

- **TOTP 防重放实现正确**：`MatchingCounter` 在 ±1 步窗口内命中并返回 counter（`totp.go:75-92`），`validateTOTPWithReplay` 要求 `counter > lastCounter` 且写库失败即拒绝（`auth_service.go:440-447`），时间步单调推进，标准重放（同一码二次使用）被正确阻断；`Enable2FA`/`Disable2FA` 有意不消耗计数器（避免「启用后 30s 内登录被误拒」），该取舍合理。
- **登录防枚举**：账号不存在与账号禁用两条路径都跑等价 bcrypt（`auth_service.go:227-242`），`dummyBcryptHash` 为公开口令哈希（非机密），时序差被抹平；`loginBucket` 用 `username|ip` 复合桶（`auth_service.go:211-213`）避免「知道用户名即可锁死受害者」的 DoS。
- **2FA pending 令牌隔离**：`middleware/auth.go:24-29` 拒绝 `claims.TwoFA` 令牌访问业务 API，这是本次最关键的一处修复（此前 2FA 可被跳过）。
- **CAS 与续租**：`Claim` 逐条 `WHERE status='pending'` 条件更新（`send_job_repo.go:100-109`），`RenewClaim`/`MarkDone` 均带 `claimed_by` 守卫（`send_job_repo.go:119-135`），`RecoverStale` 把崩溃计入 attempts（`send_job_repo.go:188-205`），消除了「毒消息无限重投」与「重复发送」两类经典缺陷。
- **密钥处理**：`randomToken`/`generateAPIKey`/`randomHex` 在 `crypto/rand` 失败时 panic 而非回退（`user_service.go:76-82`、`task_service.go:468-475`、`config.go:325-333`）——这是正确的 fail-fast。
- **部署加固**：容器非 root（`Dockerfile:37-39`）、MySQL 端口仅绑 `127.0.0.1`（`docker-compose.yml:78-79`）、健康检查口令走环境变量（`docker-compose.yml:88-91`）、`.dockerignore` 排除 `.env`/`*.key`/`config.yml`（`.dockerignore:19-25`）、`.gitignore` 覆盖 `.env*` 变体与密钥文件（`.gitignore:16-33`）。
- **迁移锁**：`Migrate` 改用专用连接 + 显式判断 `GET_LOCK` 返回值（`db.go:45-60`），修复了「连接池导致锁不生效」的严重缺陷。
- **前端开放重定向**：`Login.vue:164-168` 仅接受以 `/` 开头且非 `//` 的 `redirect`，正确阻断协议相对 URL。

---

## 3. 该视角 3-5 条最高优先级结论

1. **【blocker，S-1】管理员强制开启 2FA 的路径把 TOTP 种子明文落库**（`user_service.go:309`），使「TOTP 密钥加密存储」这一本次核心安全目标在最常用的批量运维路径上完全失效，且与用户自助开启的加密路径形成安全属性倒挂。**发布前必修**：`ForceEnable2FA` 注入 cipher 走加密写入，并在 `SetTOTP` 层拒绝「明文非空且密文为空」的生产调用。
2. **【high，S-4】`STRICT_SECRETS=1` 对官方文档的 `cp .env.example .env` 流程完全无效**：占位值 `please_change_me_to_32_bytes_random_hex` 不在弱密钥黑名单里，实测 0 条告警，服务带公开固定的 JWT/AES 密钥正常启动。这比「没有严格模式」更危险，因为它给出虚假的安全感。**发布前必修**：黑名单补占位前缀 + `.env.example` 相关值留空让 compose `:?` 拦住。
3. **【high，S-3】「明文→密文」升级路径只写密文不清明文**（`SetTOTPSecretEnc`），使被管理员强制开启 2FA 的账号在首次登录后**同时**持有明文与密文种子，加密对该人群永久失效；`DisableTOTP` 也不清密文列。修法只需在两条 SQL 里补 `totp_secret=NULL` / `totp_secret_enc=NULL`，成本极低、收益直接。
4. **【high，S-2】强制关闭 2FA 零二次确认且可批量覆盖全站（含内置 admin）**：单个被盗管理员会话即可一次性摘除所有账号的第二因子。应要求管理员提交自身 TOTP/密码，并对内置 admin 拒绝强制关闭。
5. **【high，S-5】SMTP 认证协商把「明文」的范围从 PLAIN 扩展到全部方式**：`CRAM-MD5` 分支跳过 `allow_insecure` 检查、`LOGIN` 分支不区分是否 TLS、`CRAM-MD5` 用 HMAC-MD5 可离线字典攻击。邮箱授权码是长期凭据，泄露后果持久，建议在非 TLS 连接上只允许 CRAM-MD5 且加 warning 日志、把 PLAIN/LOGIN 显式绑定到 `allow_insecure=true`。

> 补充（medium，不进前五但建议同期处理）：Webhook 重放缓存在容量打满时整体清空（S-6，攻击者可主动制造清空窗口后重放）；前端登出完全不调用服务端吊销接口（S-8），使 `middleware/auth.go:37-41` 的会话吊销能力在主路径上从未生效——后者补一行 `authApi.logout()` 即可；`ADMIN_USER` 改名会让「内置管理员保护」整体失效（S-17）；2FA 防重放在并发下可被消费两次（S-18）。

---

## 3.1 交叉核实回执（针对 captain 转来的产品 P3 及「同类放行」排查）

**核实对象**：`STRICT_SECRETS=1` 是否被 `.env.example` 占位值绕过。
**结论：成立。与产品 P3 同源**（详见 S-4，该条已标注同源标记）。产品审查员给出的三个事实点逐条复核后**全部确认**，我另补了定量探针、JWT 自签实测与 5 项同类缺陷。

**1) 绕过是否真实成立** —— 成立。`config.go:104/107/110/113` 四处均为 `==` 精确比较，`.env.example:7-10` 的占位值不在任何集合内且长度 39/36 ≥ 32。探针实测 `WeakSecretWarnings()` 对 `.env.example` 三值返回 **0 条**，`main.go:120` 的 `if len(warnings) > 0` 直接短路，`log.Fatalf` 永不进入。

**2) 影响评估（前提 → 达成）**：

| 路径 | 攻击者前提 | 可达成 |
|---|---|---|
| A. 伪造 JWT | **无需任何凭据**（占位 `JWT_SECRET` 在公开仓库）；仅需能访问服务端口 | 自签 `uid=1, role=admin` 令牌 → **全站管理员**。已实测服务端验签通过（`err==nil`）。`uid=1` 即 bootstrap 首建账号 |
| B. 解密渠道配置 | 需 DB 读权限/备份文件 | 用公开 `ENCRYPT_KEY` 解开全部渠道密文 → SMTP 授权码、机器人 token、PushPlus token → 以组织身份对外发信/钓鱼 |
| C. 登录默认管理员 | 仅需网络可达 | 若 `ADMIN_PASS` 保留占位值即公开口令，直接登录 `admin` |

**3) 其它「看起来安全、实际放行」的校验（本次 52 提交引入或覆盖）** —— 共 5 项，均已单列：

| 编号 | 放行点 | 实测/证据 |
|---|---|---|
| S-4 附加 | `padKey` 对 ≥32 字节密钥直接 `s[:32]` 截断 | 实测 `..._hex` 与 `..._HEX` 的 `padKey` 结果**完全相同** → 密钥轮换静默失效、熵被压到 32 字符 |
| S-4 附加 | `ADMIN_PASS` 强度检查 | 实测 `x`/`1`/`a`/`admin`/`password`/`please_change_me` 全部 **warnings=0**；`validatePassword` 未覆盖 bootstrap 管理员 |
| S-17 | `ADMIN_USER` 可配但 `isDefaultAdmin` 硬编码 `"admin"` | 改名部署后内置管理员可被删除/禁用/改密/生成重置令牌 |
| S-18 | `SetTOTPLastCounter` 无 CAS（读快照后无条件写） | 并发提交同一动态码 → 全部通过 → 一次性码签发多个 JWT |
| S-6 | `replayCache` 容量打满时 `c.entries = make(...)` 整体清空 | 主动灌 4096 条合法签名即可清空去重表，随后重放旧签名 |
| S-19 | `ChannelService.Create/Update` **不调用** `ValidateConfig` | `webhook_url`/`pushplus_url` 可存任意 URL（含 `169.254.169.254`），响应落进所有登录用户可读的日志 |
| S-20 | `Login` trim 口令、其它路径不 trim | 含首尾空格的强口令永久无法登录 |

> 说明：`forgot-password` 与 webhook 限流在 DB 故障时 **fail-open**（`auth_handler.go:238`、`webhook_handler.go:62-63`），这是**有意为之**且注释明确（登录/触发本身依赖 DB），不计为缺陷；但若 DB 故障期间攻击者能持续请求，限流实际为零，建议在 fail-open 时至少打一条 error 级日志以便告警。

**4) 修复方向** —— 见 S-4 的「修复建议（按性价比排序）」6 条，核心两条：**`.env.example` 密钥值留空**（让 compose `${VAR:?}` 前置拦截）+ **前缀黑名单与低熵检测**（同时挡住「32 个 a」与 `please_change_me*`）。

---

## 4. 验证记录

| 命令 | 结果 | 说明 |
|---|---|---|
| `go build ./...` | 通过（exit 0） | 全仓库编译无错 |
| `go vet ./...` | 通过（exit 0） | 无静态检查告警 |
| `go test ./internal/totp/... ./internal/render/... ./internal/config/... ./internal/channel/...` | 全部 `ok` | 不依赖 DB 的包全绿 |
| `go test ./internal/scheduler/...` | 失败 | 仅因本机无 MySQL（`connect failed: invalid connection`），非代码缺陷；`TestLeaseExclusiveAndExpiry`、`TestSchedulerLeasePath` 需要 DB |
| 探针：`WeakSecretWarnings` × `.env.example` 三值 | `warnings=0` | 直接证明 S-4（探针已删除） |
| 探针：`WeakSecretWarnings` × `ADMIN_PASS` ∈ {`x`,`1`,`a`,`admin`,`password`,`please_change_me`} | 全部 `warnings=0` | 证明 S-4 附加发现：bootstrap 管理员口令无任何强度约束 |
| 探针：`padKey("..._hex")` vs `padKey("..._HEX")` | 输出相同 `"please_change_me_to_32_bytes_ran"` | 证明 S-4 附加发现：截断碰撞 |
| 探针：用公开占位 `JWT_SECRET` 自签 `{uid:1,role:admin}` 并走服务端验签等价逻辑 | 验签**通过**（`err==nil`） | 证明 S-4 路径 A：无需凭据即可伪造管理员令牌 |
| 探针：`config.yml` 写 `admin.pass: ""` | `AdminPass="admin123"` | 证明 S-4 附加发现：留空回落默认弱口令（fail-open） |
| 探针：webhook `X-Timestamp` 取 int64 极值（`0`/`MinInt64`/`MaxInt64`） | 全部被时间窗拒绝 | **排除**一种猜测：窗口判断无整数溢出绕过 |
| 探针：`render.ToHTMLEmail` × `<script>` / `onerror` / `javascript:` | 原样输出 | 直接证明 S-11（探针已删除） |
| `go mod why -m go.mongodb.org/mongo-driver/v2` | 由 `gin` 间接引入 | 依赖链正常，`go.mod` 无新增直接依赖（`v2.0.0..HEAD` 未改 `go.mod`/`go.sum`） |
| `gofmt -l ./cmd ./internal` | 大量文件被列出 | 已定位原因：Windows 工作区检出为 CRLF（`internal/channel/clawbot.go` 实测 45 处 CRLF），`gofmt -d` 的差异全部是行尾符，无格式/语法问题；`.gitattributes` 已保证入库为 LF，故**非缺陷**，仅记录 |

**未能验证**：所有依赖 MySQL 的行为（`users.Update` 的 `IF(password_hash=?, ...)` 赋值顺序语义、`RecoverStale` 的并发竞态、`SetTOTPLastCounter` 的原子性）。其中 `users.Update` 的吊销逻辑**依赖 MySQL「单表 UPDATE 的赋值从左到右求值，右侧读取已更新的值」这一语义**——若该语义在某版本/某配置下不成立（例如 `ONLY_FULL_GROUP_BY` 无关，但与 `sql_mode`/优化器无关的文档承诺），管理员重置密码后的旧 JWT 将不会失效。建议在 CI（已有 MySQL 8.0 service）补一条集成测试：改密后断言 `session_revoked_at` 非空且旧令牌被 `middleware.Auth` 拒绝。
