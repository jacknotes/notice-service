# 生产故障复盘：H-1 弱口令检查误判导致容器崩溃循环

> 发生时间：2026-10-23 17:38–17:40（Asia/Shanghai）
> 影响范围：生产环境 `tencent.markli.cn:/opt/notice-service`，服务不可用约 2 分钟
> 严重度：**P0**（生产不可用，崩溃循环）
> 引入版本：`v2.0.0-61-ga3bc300`（本批 H-1 修复）
> 状态：**已修复**（本文件对应修复）

---

## 1. 故障现象

按计划把生产从 `v2.0.0-48-g135ab47` 升级到 `v2.0.0-61-ga3bc300` 后，
容器立即进入重启循环：

```
notice-service-notice-service-1  jacknotes/notice-service:v2.0.0-61-ga3bc300  Restarting (1)
```

日志反复输出：

```
[警告] ADMIN_PASS 为默认/弱值，默认管理员可被直接登录，请设置强密码
STRICT_SECRETS=1 且检测到弱密钥配置，拒绝启动。请设置强随机 JWT_SECRET / ENCRYPT_KEY / ADMIN_PASS 后重试
```

旧版本 `v2.0.0-48-g135ab47` 用同一份 `.env` 可正常启动。

---

## 2. 根因

### 2.1 直接原因：新增的「低熵」判定误判人类口令

H-1 修复给 `WeakSecretWarnings()` 加了三层判定，其中第三层是：

```go
// 去重字符数 < 12 判为低熵
func lowEntropySecret(s string) bool { ... return len(distinct) < 12 }
```

该规则被同时应用到了 `JWT_SECRET`、`ENCRYPT_KEY` **和 `ADMIN_PASS`**。

生产 `.env` 的 `ADMIN_PASS` 特征：长度 16、去重字符数 **9**、小写+数字两类字符。

→ `lowEntropySecret()=true` → 告警 → `STRICT_SECRETS=1` 拒绝启动 → 崩溃循环。

### 2.2 为什么这条规则本身是错的

「去重字符数」只适用于**机器生成**的随机串（hex/base64 字符集大，64 字符 hex
去重数几乎必然 ≥12）。**人类口令的字符集天然窄**——只由大小写字母、数字、符号
构成（最多 90 余个可打印字符），16 位强口令的去重字符数通常只有 8–11。

用一个与口令强度无关的指标判定口令，必然误伤。

### 2.3 更深层原因：`ADMIN_PASS` 根本不该参与「每次启动」的阻断

`ADMIN_PASS` **只在 `BootstrapAdmin()` 首次建号时使用一次**：

```go
func (s *AuthService) BootstrapAdmin() error {
    if _, err := s.users.GetByUsername(s.adminUser); err == nil {
        return nil          // ← admin 已存在，直接返回，完全不使用 adminPass
    }
    // 只有 admin 不存在时才用 s.adminPass 建号
}
```

生产库中 admin 建于 `2026-08-28`，**`updated_at=2026-09-22`**（运维后来改过密码）。
实测用 `.env` 里的 `ADMIN_PASS` 登录返回 **401** —— 它已是历史遗留值。

拿一个「本次启动根本不会生效」的历史配置去拒绝启动，逻辑上不成立。

---

## 3. 修复

### 3.1 口令改用结构判定（不用去重字符数）

`internal/config/config.go`：

```go
// weakAdminPass 判断人类口令强度：长度 < 12 或字符类别 < 3 类即判弱。
// 类别 = 大写 / 小写 / 数字 / 符号。
// 不用去重字符数：口令字符集天然窄，16 位强口令去重数常为 8-11。
func weakAdminPass(s string) bool { ... }
```

`lowEntropySecret` 的注释补充明确约束：**只用于机器生成的密钥，不要用于人类口令**。

### 3.2 `ADMIN_PASS` 移出启动期弱密钥阻断

- `WeakSecretWarnings()` **只保留** `JWT_SECRET` / `ENCRYPT_KEY`——它们每次启动都
  真正生效（签发/校验 JWT、加解密渠道配置）。
- 新增 `AdminPassWarnings()` 单独判定口令强度。

### 3.3 只在「真的会建号」时校验

`cmd/server/main.go` 新增 `checkAdminPassOnBootstrap()`，在 DB 就绪后执行：

```go
if _, err := users.GetByUsername(cfg.AdminUser); err == nil {
    return nil                    // admin 已存在：ADMIN_PASS 本次不生效，跳过
} else if !errors.Is(err, repository.ErrNotFound) {
    return fmt.Errorf("校验管理员口令失败: %w", err)
}
// admin 不存在：本次启动会用它建号，弱口令 = 部署后门
if strict { return fmt.Errorf("...拒绝启动...") }
```

行为矩阵：

| admin 状态 | ADMIN_PASS | STRICT_SECRETS | 结果 |
|---|---|---|---|
| 已存在 | 弱（历史遗留） | 1 | **正常启动**（本次修复点） |
| 已存在 | 弱 | 0 | 正常启动 |
| 不存在 | 弱 | 1 | **拒绝启动**（保留原安全目标） |
| 不存在 | 弱 | 0 | 告警后继续 |
| 不存在 | 强 | 1 | 正常启动并建号 |

---

## 4. 验证

### 4.1 单元测试（`internal/config/config_test.go`）

| 测试 | 断言 |
|---|---|
| `TestAdminPassWarnings` | `x/1/a/admin/password/admin123/please_change_me` 全部告警 |
| `TestAdminPassWarningsStrongNotFlagged` | 6 个「长度够但去重字符数 <12」的强口令**不告警**；4 个类别不足的**仍告警** |
| `TestWeakSecretWarningsExcludesAdminPass` | `ADMIN_PASS=admin123` 时 `WeakSecretWarnings()` 返回空 |
| `TestWeakSecretWarnings` | 告警数由 4 改为 3（ADMIN_PASS 已移出） |

**反向验证**：把 `config.go` 还原成修复前版本后，
`TestAdminPassWarningsStrongNotFlagged` **FAIL 3 项**
（`Passw0rdPassw0rd` 被误判、两个类别不足的口令未被拦截）。恢复后 PASS。

### 4.2 启动行为实测（独立库 `notice_probe_gate`，四个分支）

```
B1 空库 + 弱口令 + STRICT=1
   EXIT=1
   [警告] ADMIN_PASS 为默认/弱值（长度 <12 或字符类别 <3）…（将用于首次创建管理员 "admin"）
   STRICT_SECRETS=1 且首次创建管理员 "admin" 的 ADMIN_PASS 为弱口令，拒绝启动。  ✅

B2 空库 + 强口令 Abcd1234!@#$efgh + STRICT=1
   listening on :8099，admin 已创建（id=1）                                ✅

B3 admin 已存在 + 弱口令 + STRICT=1
   listening on :8099（不阻断）                                            ✅  ← 修复点

B4 首次建号 + 弱口令 + STRICT=0
   告警后继续启动                                                          ✅
```

### 4.3 生产复现验证

用生产 `.env`（admin 已存在）启动新二进制：

```
STRICT_SECRETS=1 ... ADMIN_PASS=<生产历史值> → notice-service listening on :8099
```

不再崩溃。`go build ./...` / `go vet ./...` / `go test -p 1 ./... -count=1` 全绿。

---

## 5. 审查漏判反思

`docs/fixes/review.md` 曾把「低熵阈值误伤」列为**低风险**，理由是「hex 密钥误报率
1.7%，`openssl rand -hex 32` 下为 0」。这个结论**只算了密钥，漏算了口令**。

`TestWeakSecretWarningsAdminPass` 当时只测 `x/1/a/admin/password/admin123` 这类
**精确弱词**（长度都 <12，走的是另一条分支），因此**没有覆盖到「长度足够但字符集
小」的真实口令**——测试全绿掩盖了缺陷。

教训：
1. **熵指标不能跨类型复用**——密钥（机器生成）与口令（人类选择）的分布完全不同。
2. **判定要绑定「实际生效范围」**——先问「这个配置本次启动真的会用吗」，再决定
   要不要据此阻断启动。
3. **测试要覆盖真实输入的分布**，而不只是黑名单里的样例。

---

## 6. 处置记录

| 时间 | 动作 |
|---|---|
| 17:37 | 构建 `v2.0.0-61-ga3bc300` 成功 |
| 17:38 | 更新 compose 镜像 tag，`up -d` → 容器进入重启循环 |
| 17:38–17:40 | 诊断：`ADMIN_PASS` 低熵误判 |
| 17:40 | **应急**：`.env` 设 `STRICT_SECRETS=0`（原 `.env` 备份到 `/tmp/.env.bak-*`）→ 服务恢复 |
| 17:49–18:05 | 修复代码 + 补测试 + 四分支实测 + 全量回归 |
| 待办 | 重新构建镜像 → 部署 → 恢复 `STRICT_SECRETS=1` |

**应急措施的性质**：`STRICT_SECRETS=0` 只是让服务起来，**并未降低实际安全性**
——因为生产 `JWT_SECRET`（64 字符 hex）与 `ENCRYPT_KEY`（32 字符）都是强随机值，
`ADMIN_PASS` 那条告警本就是误判。但仍应尽快恢复 `=1` 以保留对 JWT/ENCRYPT 的强制检查。

---

## 7. 后续建议

1. **`ADMIN_PASS` 与当前口令的语义割裂应消除**：`.env` 的值在 admin 建号后即失效，
   容易让运维误以为「改 .env 就是改密码」。建议启动时若检测到
   `ADMIN_PASS` 为弱值且 admin 已存在，输出一条**信息性**日志说明「该配置已不生效，
   改密码请用界面或 reset-password 子命令」。
2. **CHANGELOG「升级注意」需补充**：H-1 条目应写明「本次收紧可能因 `ADMIN_PASS`
   触发启动失败」，并给出排查路径（本故障正是升级踩中）。
3. **CI 增加「口令强度判定」属性测试**：用随机生成的真实口令分布（长度 12–24、
   1–4 类字符）验证判定准确性，而非仅黑名单样例。
