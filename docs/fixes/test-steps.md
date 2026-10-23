# 修复项手工验收测试步骤（顺序执行）

> 对应 7 个「立即」条目：B-1 / B-2 / H-1 / H-2 / H-5 / M-2 / M-12。
> 每一步给出**操作 → 期望 → 判定依据**。带 🔴 的是核心验收项（两个 blocker + 两个 high）。
> 环境：WSL 中已运行的服务 `http://localhost:8080`（`admin` / `admin123`），MariaDB 库 `notice_service`。

---

## ⚡ 最快方式：一键脚本（推荐）

**不要**在 PowerShell 里手敲嵌套引号的命令——PowerShell 会吞掉嵌套双引号，导致 `unexpected EOF`。
所有步骤已封装成脚本，直接跑：

```powershell
# 全部 7 项，一条命令
wsl -e bash /mnt/f/project/notice-service/docs/fixes/run-all-tests.sh
```

单项：

```powershell
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-b1.sh       # B-1 农历短名
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-b2.sh       # B-2 TOTP 加密
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h1.sh       # H-1 弱密钥
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h2.sh       # H-2 SMTP
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h5.sh       # H-5 导入保真
wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-m2-m12.sh   # M-2 + M-12
```

每个脚本末尾都会打印 `PASS` / `FAIL` 断言结果，并自动清理探针数据。

**2026-10-23 实测结果：7/7 全部 PASS。**

---

---

## 0. 前置：确认服务在跑

```bash
wsl -e bash -lc 'curl -s -o /dev/null -w "health=%{http_code}\n" http://127.0.0.1:8080/api/health'
```

**期望**：`health=200`。

若未运行，先启动（注意**不要**设置 `ENCRYPT_KEY`，让它读 `.notice-encrypt.key`，否则已有 9 个渠道无法解密）：

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service && nohup env -u ENCRYPT_KEY -u JWT_SECRET -u STRICT_SECRETS PORT=8080 /tmp/notice-server > /tmp/notice.log 2>&1 & sleep 5; curl -s http://127.0.0.1:8080/api/health'
```

浏览器打开 **http://localhost:8080**（必须用 `localhost`；WSL2 NAT 模式下 `127.0.0.1` 从 Windows 侧不通）。

> 下文命令里的 `$TOK` 是管理员令牌。手动执行时可先运行：
> ```bash
> TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login \
>   -H "Content-Type: application/json" \
>   -d '{"username":"admin","password":"admin123"}' \
>   | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")
> echo "$TOK"
> ```

---

## 1. 🔴 B-1 农历节日短名必须在解析期被拒绝（blocker）

修复前 `term 中秋` 保存成功但永不触发；修复后保存即报错。

### 1.1 后端拒绝短名（命令行）

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X POST http://127.0.0.1:8080/api/tasks -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
  -d "{\"name\":\"t_short\",\"channel_id\":3,\"template_id\":17,\"trigger_type\":\"cron\",\"cron_expr\":\"@lunar term 中秋 09:00\",\"receivers\":[\"a@x.com\"],\"enabled\":true}" \
  -w "\nHTTP=%{http_code}\n"'
```

**期望**：
- `HTTP=400`
- 错误含 `未知的节气/节日名 "中秋"` 与 `可用节日：…中秋节…`

**修复前对比**：`HTTP=200`、`next_run_at=null`（静默失效）。

### 1.2 后端接受全称

把 `cron_expr` 换成 `@lunar term 中秋节 09:00`，其余不变。

**期望**：`HTTP=200`，且 `next_run_at` 是**具体日期**（如 `2027-09-15T09:00:00+08:00`），不是 null。

> 记下返回的 `id`，1.4 要删除。

### 1.3 界面快捷项（浏览器）

1. 打开 http://localhost:8080 → 登录 → **任务管理** → 新建任务
2. 找到「农历快捷」按钮组，点 **「中秋 9点」**
3. 看表达式输入框

**期望**：填入 `@lunar term 中秋节 09:00`（**全称**，带「节」字）。
修复前（旧 `web/dist`）是 `@lunar term 中秋 09:00`。

4. 直接保存 → **期望成功**

### 1.4 清理探针任务

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X DELETE http://127.0.0.1:8080/api/tasks/<1.2 的 id> -H "Authorization: Bearer $TOK" -w "HTTP=%{http_code}\n"'
```

---

## 2. 🔴 B-2 TOTP 种子必须加密落库（blocker）

核心断言：`totp_secret`（明文列）为 **NULL**，`totp_secret_enc`（密文列）**非空**。

### 2.1 自助开启 2FA（浏览器，最真实）

1. 右上角头像 → **个人设置**
2. 找到「双因子认证」段 → 点「开启」
3. 用手机认证器 App 扫码（或手动输入密钥）
4. 输入 App 上的 6 位动态码 → 启用

**期望**：提示启用成功。

> 没有手机 App 用命令行方式（2.2）。

### 2.2 命令行方式（无需手机）

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service
TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
SECRET=$(curl -s -X POST http://127.0.0.1:8080/api/auth/2fa/setup -H "Authorization: Bearer $TOK" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"secret\"])")
echo "secret=$SECRET"
CODE=$(python3 docs/fixes/totp.py "$SECRET")
echo "code=$CODE"
curl -s -X POST http://127.0.0.1:8080/api/auth/2fa/enable -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" -d "{\"code\":\"$CODE\"}" -w "\nHTTP=%{http_code}\n"'
```

**期望**：`{"ok":true}`、`HTTP=200`。

### 2.3 直接读库验证加密（关键证据）

```bash
wsl -e bash -lc 'mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT username, totp_enabled,
       totp_secret IS NULL         AS 明文列为NULL,
       CHAR_LENGTH(totp_secret_enc) AS 密文长度
FROM notice_service.users WHERE username=\"admin\";" 2>/dev/null'
```

**期望**：
```
username  totp_enabled  明文列为NULL  密文长度
admin     1             1            80
```

- `明文列为NULL = 1` ← **B-2 的核心**（修复前是 0，明文种子直接落库）
- `密文长度 = 80` ← AES-GCM 密文非空

### 2.4 强制开启路径（管理员批量入口，另一个破口）

1. 界面 → **用户管理** → 选一个非 admin 用户 → 操作列 → **「强制开启 2FA」**
2. 弹窗显示密钥 + 8 个备用码 + 二维码

**期望**：弹窗正常；再对该用户跑 2.3 的 SQL（`username` 换成该用户），同样得到 `明文列为NULL=1`、`密文长度=80`。

> 修复前这条路径写明文（`ForceEnable2FA` 传空串），是「管理员强制开启反而更弱」的破口。

### 2.5 关闭 2FA 必须清空两列

个人设置 → 关闭 2FA → 输入动态码确认。然后：

```bash
wsl -e bash -lc 'mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT username, totp_enabled,
       totp_secret IS NULL AS p, totp_secret_enc IS NULL AS e,
       totp_last_counter, totp_recovery_codes IS NULL AS r
FROM notice_service.users WHERE username=\"admin\";" 2>/dev/null'
```

**期望**：`totp_enabled=0`、`p=1`、`e=1`、`totp_last_counter=0`、`r=1`。
（修复前 `e=0`——停用后密文种子永久驻留。）

### 2.6 收尾

若 2.4 给别的用户开了 2FA，记得强制关闭，避免影响后续登录：

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X POST http://127.0.0.1:8080/api/users/<用户id>/2fa-disable -H "Authorization: Bearer $TOK" -w "HTTP=%{http_code}\n"'
```

---

## 3. 🔴 H-1 弱密钥必须拒绝启动

### 3.1 用 `.env.example` 的历史占位值启动 → 必须被拒

```bash
wsl -e bash -lc 'STRICT_SECRETS=1 \
  JWT_SECRET=please_change_me_to_32_bytes_random_hex \
  ENCRYPT_KEY=please_change_me_to_32_bytes_random_hex \
  ADMIN_PASS=admin123 \
  PORT=8081 /tmp/notice-server > /tmp/h1.log 2>&1
echo "EXIT=$?"; tail -5 /tmp/h1.log'
```

**期望**：
- `EXIT=1`（**拒绝启动**）
- 3 条告警：`JWT_SECRET 为公开占位值或低熵密钥`、`ENCRYPT_KEY 为公开占位值或低熵密钥`、`ADMIN_PASS 为默认/弱值`
- 最后一行 `STRICT_SECRETS=1 且检测到弱密钥配置，拒绝启动`

**修复前对比**：这组值**长度 ≥32 且不在精确相等黑名单** → **0 条告警** → 正常启动（形同虚设）。

### 3.2 强随机密钥必须放行（验证不误伤）

```bash
wsl -e bash -lc 'STRICT_SECRETS=1 \
  JWT_SECRET=$(openssl rand -hex 32) \
  ENCRYPT_KEY=$(openssl rand -hex 32) \
  ADMIN_PASS="Str0ng!Passphrase#2026" \
  DB_PASSWORD=notice123 PORT=8081 timeout 6 /tmp/notice-server 2>&1 | grep -E "警告|listening" | head -5'
```

**期望**：**无弱密钥告警**，直接 `notice-service listening on :8081`。

> ⚠️ 这里故意覆盖 `ENCRYPT_KEY`，仅验证启动放行；`timeout 6` 会杀掉它，不影响 8080 主服务。

### 3.3 弱 `ADMIN_PASS` 单独也要拦

```bash
wsl -e bash -lc 'STRICT_SECRETS=1 \
  JWT_SECRET=$(openssl rand -hex 32) ENCRYPT_KEY=$(openssl rand -hex 32) \
  ADMIN_PASS=password DB_PASSWORD=notice123 PORT=8081 /tmp/notice-server 2>&1 | grep -E "ADMIN_PASS|拒绝启动"'
```

**期望**：`ADMIN_PASS 为常见弱口令…` + `拒绝启动`。

### 3.4 检查 `.env.example` 本身

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service && grep -nE "^(JWT_SECRET|ENCRYPT_KEY)=" .env.example'
```

**期望**：`JWT_SECRET=` / `ENCRYPT_KEY=`（**值留空**），配合 compose 的 `${VAR:?}` 启动前拦截。

---

## 4. 🔴 H-2 SMTP 认证不得明文降级

需要假 SMTP 服务器（仓库已备 `docs/fixes/fake_smtp.py`）。

### 4.1 启动两个假服务器

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service
nohup python3 docs/fixes/fake_smtp.py 2525 "CRAM-MD5 LOGIN PLAIN" > /tmp/smtp2525.log 2>&1 &
nohup python3 docs/fixes/fake_smtp.py 2526 "LOGIN PLAIN" > /tmp/smtp2526.log 2>&1 &
sleep 2; ss -ltn | grep -E "2525|2526"'
```

- **2525**：宣告全部三种机制
- **2526**：只宣告 LOGIN/PLAIN（没有 CRAM-MD5）

### 4.2 场景 A：非 TLS 未显式放行 → 连接期拒绝

界面 → 渠道管理 → 新建 → 类型 `email`：

| 字段 | 值 |
|---|---|
| host | `127.0.0.1` |
| port | `2525` |
| username | `u@x.com` |
| password | `s3cr3t-pass` |
| from | `u@x.com` |

点「测试连接」。

**期望**：报错 `SMTP 服务器不支持 STARTTLS，拒绝明文传输邮箱密码…`
（第一道守卫：非 TLS 未 `allow_insecure` 时**根本不发起认证**。）

命令行等价：

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X POST http://127.0.0.1:8080/api/channels/0/test -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
 -d "{\"type\":\"email\",\"config\":{\"host\":\"127.0.0.1\",\"port\":\"2525\",\"username\":\"u@x.com\",\"password\":\"s3cr3t-pass\",\"from\":\"u@x.com\"}}" -w "\nHTTP=%{http_code}\n"'
```

### 4.3 场景 B：显式 `allow_insecure=true` + 三机制都宣告 → **必须选 CRAM-MD5**

⚠️ 界面表单**没有** `allow_insecure` 字段，必须用命令行加这个键：

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X POST http://127.0.0.1:8080/api/channels/0/test -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
 -d "{\"type\":\"email\",\"config\":{\"host\":\"127.0.0.1\",\"port\":\"2525\",\"username\":\"u@x.com\",\"password\":\"s3cr3t-pass\",\"from\":\"u@x.com\",\"allow_insecure\":\"true\"}}" -w "\nHTTP=%{http_code}\n"
sleep 1; echo "--- 服务器实际收到的认证机制 ---"; cat /tmp/smtp2525.log'
```

**期望**：
```
{"ok":true}
HTTP=200
--- 服务器实际收到的认证机制 ---
MECH=CRAM-MD5
CRAM_USER=u@x.com
CRAM_DIGEST=3bd080447d39567dc176d895b4bf317a
```

**关键判定**：
- `MECH=CRAM-MD5` ← 三种都宣告，客户端**仍选 CRAM-MD5**（口令不上网）
- **没有** `LOGIN_PASSWORD_B64=` 行 ← 口令没以 base64 明文传输
- `CRAM_DIGEST` 是 HMAC 摘要，**不是**口令 `s3cr3t-pass`

**修复前对比**：旧实现按服务器宣告顺序走，服务器把 `LOGIN` 排前面就选 `LOGIN`（明文口令上网）。

### 4.4 场景 C：只宣告 LOGIN/PLAIN + 未放行 → **必须拒绝且不认证**

```bash
wsl -e bash -lc 'TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
curl -s -X POST http://127.0.0.1:8080/api/channels/0/test -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
 -d "{\"type\":\"email\",\"config\":{\"host\":\"127.0.0.1\",\"port\":\"2526\",\"username\":\"u@x.com\",\"password\":\"s3cr3t-pass\",\"from\":\"u@x.com\"}}" -w "\nHTTP=%{http_code}\n"
echo "--- 2526 日志（应无 MECH 行 = 从未认证）---"; cat /tmp/smtp2526.log'
```

**期望**：`HTTP=400` 报错；`/tmp/smtp2526.log` 里**只有** `LISTEN …`，**没有** `MECH=` 行。

### 4.5 场景 D：只宣告 LOGIN/PLAIN + 显式放行 → 允许但打 warning

`port` 保持 `2526`，加上 `"allow_insecure":"true"`。

**期望**：
- `HTTP=200`
- 假服务器日志出现 `MECH=LOGIN`、`LOGIN_PASSWORD_B64=czNjcjN0LXBhc3M=`（= `s3cr3t-pass`，说明确实明文了）
- **服务端日志出现 warning**：

```bash
wsl -e bash -lc 'grep "非 TLS 连接使用" /tmp/notice.log | tail -2'
```

期望 `[警告] channel/email: 非 TLS 连接使用 LOGIN 认证（allow_insecure=true）…`

### 4.6 清理

```bash
wsl -e bash -lc 'pkill -f fake_smtp.py; sleep 1; ss -ltn | grep -E "2525|2526" || echo "fake SMTP stopped"'
```

---

## 5. 🔴 H-5 备份导入必须保真模板 `enabled`

**你的库正好是 H-5 的受害者现场**：17 个模板里 **16 个停用**，而 29 个任务全部指向停用模板 → 任务静默停发。

### 5.1 先看当前状态（问题现场）

```bash
wsl -e bash -lc 'mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT enabled, COUNT(*) AS 数量 FROM notice_service.templates WHERE deleted_at IS NULL GROUP BY enabled;
SELECT COUNT(*) AS 指向停用模板的任务数 FROM notice_service.tasks t
  JOIN notice_service.templates tpl ON tpl.id=t.template_id
 WHERE t.deleted_at IS NULL AND tpl.enabled=0;" 2>/dev/null'
```

**期望**：`enabled=0` 有 16 个、`enabled=1` 有 1 个；指向停用模板的任务数 = 29。

### 5.2 界面走一遍完整导出/导入

1. 界面 → **系统设置** → 拉到「数据备份」段
2. 点 **「导出备份」** → 得到 JSON
3. 打开 JSON 搜 `"enabled"`，确认模板对象里**有**该字段且值为 `true`/`false`（不是缺失）

**期望**：模板都带 `enabled` 字段，与库内状态一致。

### 5.3 命令行精确验证（1 启用 + 1 停用 → 导入后一致）

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service
TOK=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)[\"token\"])")
# 1) 导出
curl -s -H "Authorization: Bearer $TOK" http://127.0.0.1:8080/api/export -o /tmp/bk.json -w "export HTTP=%{http_code}\n"
# 2) 造「1 启用 + 1 停用」的最小 bundle
python3 -c "
import json,time
b=json.load(open(\"/tmp/bk.json\"))
tp=b[\"templates\"]
on=[t for t in tp if t[\"enabled\"]][0]
off=[t for t in tp if not t[\"enabled\"]][0]
s=str(int(time.time()))
on[\"name\"]=\"RVW_ON_\"+s; off[\"name\"]=\"RVW_OFF_\"+s
json.dump({\"version\":1,\"exported_at\":b[\"exported_at\"],\"channels\":[],\"templates\":[on,off],\"tasks\":[]},open(\"/tmp/probe.json\",\"w\"),ensure_ascii=False)
print(\"bundle:\", [(t[\"name\"],t[\"enabled\"]) for t in [on,off]])
"
# 3) 导入
curl -s -X POST http://127.0.0.1:8080/api/import -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" --data-binary @/tmp/probe.json -w "\nimport HTTP=%{http_code}\n"
# 4) 读库断言
mysql -unotice -pnotice123 -h127.0.0.1 -e "SELECT name, enabled FROM notice_service.templates WHERE name LIKE \"RVW_%\" ORDER BY name;" 2>/dev/null'
```

**期望**：
```
{"channels_created":0,"templates_created":2,"tasks_created":0,"skipped":null}
import HTTP=200
name                 enabled
RVW_OFF_<时间戳>      0
RVW_ON_<时间戳>       1
```

**关键判定**：`RVW_ON` 导入后 `enabled=1`。
**修复前**：两个都会是 `0`（Go 零值 `false` 直接落库，绕过列默认值 1）。

### 5.4 清理探针

```bash
wsl -e bash -lc 'mysql -unotice -pnotice123 -h127.0.0.1 -e "DELETE FROM notice_service.templates WHERE name LIKE \"RVW_%\";" 2>/dev/null
rm -f /tmp/bk.json /tmp/probe.json
mysql -unotice -pnotice123 -h127.0.0.1 -e "SELECT COUNT(*) AS 剩余模板 FROM notice_service.templates WHERE deleted_at IS NULL;" 2>/dev/null'
```

**期望**：剩余模板 = 17（回到原始数量）。

### 5.5 顺带体验 M-7 后果（可选，说明 H-5 为什么是 high）

1. 任务管理 → 任一任务 → 「立即发送」
2. 看发送日志

**期望**：日志出现 `模板「xxx」已停用` 的 failed 记录 —— 这就是「导入丢 enabled」导致的静默停发。

> 想真正恢复发送：模板管理 → 批量选中要用的模板 → 批量启用。

---

## 6. M-2 `QUEUE_CLAIM_TTL=0` 不得崩进程

### 6.1 用 0 启动（修复前会 panic 崩进程）

```bash
wsl -e bash -lc 'QUEUE_CLAIM_TTL=0 QUEUE_WORKERS=0 PORT=8082 timeout 6 /tmp/notice-server 2>&1 | grep -E "警告|listening|panic" | head -8'
```

**期望**：
```
[警告] QUEUE_WORKERS=0 低于下限，已钳制为 1
[警告] QUEUE_CLAIM_TTL=0 低于下限，已钳制为 30
notice-service listening on :8082 …
```
**没有 panic、没有崩溃**。

**修复前**：`time.NewTicker(0)` 在 renew goroutine 内 panic → 整个进程崩溃。

### 6.2 负数同样

把 `QUEUE_CLAIM_TTL=0` 换成 `-5`，期望同样 `已钳制为 30` + 正常监听。

### 6.3 正常值不被改写

```bash
wsl -e bash -lc 'QUEUE_CLAIM_TTL=120 QUEUE_WORKERS=4 PORT=8082 timeout 5 /tmp/notice-server 2>&1 | grep -E "QUEUE_|listening" | head -5'
```

**期望**：**没有** `QUEUE_*` 钳制告警（120 与 4 都在合法区间），正常监听。

---

## 7. M-12 CHANGELOG `[Unreleased]` 非空

### 7.1 检查结构与条数

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service
echo "=== [Unreleased] 到 [2.0.0] 之间的行数 ==="
awk "/^## \[Unreleased\]/{f=1;next} /^## \[2\.0\.0\]/{f=0} f" CHANGELOG.md | wc -l
echo "=== 小节标题 ==="
awk "/^## \[Unreleased\]/{f=1;next} /^## \[2\.0\.0\]/{f=0} f" CHANGELOG.md | grep "^### "
echo "=== 升级注意条数 ==="
awk "/^### 升级注意/{f=1;next} /^## \[2\.0\.0\]/{f=0} f" CHANGELOG.md | grep -cE "^[0-9]+\."'
```

**期望**：
- 行数 ≈ 73（>0）
- 四个小节：`### 安全加固`、`### 新增`、`### 变更`、`### 升级注意`
- 升级注意 **8** 条

### 7.2 确认关键条目存在

```bash
wsl -e bash -lc 'cd /mnt/f/project/notice-service && grep -nE "农历节日名必须用全称|STRICT_SECRETS|登录态语义变化|备份导入" CHANGELOG.md'
```

**期望**：命中「农历节日名必须用全称」（对应 B-1）、`STRICT_SECRETS` 启动失败风险（H-1）、登录态、备份导入复查（H-5）。

---

## 8. 全部跑完后的一键回归（可选，最严格）

```bash
wsl -e bash -lc 'export PATH=$PATH:/usr/local/go/bin; cd /mnt/f/project/notice-service && go build ./... && go vet ./... && go test -p 1 ./... -count=1 2>&1 | tail -25'
```

**期望**：`go build` / `go vet` exit 0；所有包 `ok`（串行口径 `-p 1`，与 CI 一致）。

> ⚠️ 不要用并行口径 `go test ./internal/...` 对照——该仓库测试共用同一 MySQL 库，并行会互删种子数据（既有基线问题，已由测试期命名锁缓解）。

---

## 附：本次已实测通过的结论（2026-10-23）

| 条目 | 实测方式 | 结果 |
|---|---|---|
| B-1 | 真实 HTTP：`term 中秋` / `term 中秋节` | 400 报错含可用值 / 200 + `next_run_at=2027-09-15` ✅ |
| B-2 | 真实 API + 读库 | `totp_secret IS NULL=1`、`enc_len=80`、`totp_enabled=1` ✅ |
| H-1 | `STRICT_SECRETS=1` + 占位值启动 | `EXIT=1` + 3 条告警 + `拒绝启动` ✅ |
| H-2 | 假 SMTP 端到端 | 三机制都宣告 → 实选 `CRAM-MD5`，无明文口令；仅 LOGIN/PLAIN 未放行 → 400 且从未认证；放行 → `LOGIN` + warning ✅ |
| H-5 | 真实导出/导入 | `RVW_ON=1` / `RVW_OFF=0` 保真 ✅ |
| M-2 | `QUEUE_CLAIM_TTL=0` 启动 | 钳制 30s，无 panic ✅ |
| M-12 | `awk` 统计 | 4 小节 + 升级注意 8 条 ✅ |

**辅助脚本**（本次新增，可保留供复测）：
- `docs/fixes/fake_smtp.py` — 最小假 SMTP 服务器，打印客户端实际使用的认证机制
- `docs/fixes/totp.py` — 由 Base32 密钥生成 6 位动态码（无需手机 App）
