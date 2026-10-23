---
name: "notice-service-deploy"
description: "Deploy/upgrade notice-service to the Tencent Cloud server (root@tencent.markli.cn:/opt/notice-service) via SSH from WSL: pull latest code, build the Docker image with the git-describe tag, swap the image tag in docker-compose.quickstart.yml, restart without touching MySQL or .env, then verify health and auto-rollback on failure. Captures the hard-won gotchas: multi-layer quote mangling through PowerShell→wsl→ssh, STRICT_SECRETS crash loops, ENCRYPT_KEY/decrypt-key mismatches, stale portproxy rules, and prod-vs-historical ADMIN_PASS. Invoke whenever asked to deploy, upgrade, roll back, or check the health of notice-service on the server."
---

# notice-service 部署到服务器（notice-service-deploy）

把本仓库的新版本部署到生产服务器 `root@tencent.markli.cn:/opt/notice-service` 的**完整可执行流程**，以及所有踩过的坑。

> **核心纪律**：只构建镜像 + 换 `image:` 一行 + 重启 notice-service 容器。**绝不动** `.env`、MySQL 容器、`mysql_data` 数据卷、`docker-compose.yml`（多实例版）。

---

## 1. 何时使用

- 「部署到服务器」「升级线上」「重新构建镜像」
- 「回滚到上一版」「线上挂了看看什么情况」
- 「查一下线上跑的是哪个版本」

不适用：本地开发（用 `make dev`）、发布到 Docker Hub（用 `deploy/publish.sh`）。

---

## 2. 一键命令（首选）

```powershell
# 完整部署（拉代码 → 构建 → 换 tag → 重启 → 验证 → 失败自动回滚）
wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh

# 只做健康检查，不改任何东西
wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh --check

# 回滚到部署前记录的 tag
wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh --rollback

# 跳过构建，仅用现有镜像重启
wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh --skip-build
```

脚本内置 7 步：前置检查 → 记录回滚点 → 拉代码 → 构建镜像 → 验证镜像 → 换 tag 重启 → 健康验证（失败自动回滚）。

---

## 3. 环境事实（写死的事实，改前先确认）

| 项 | 值 | 备注 |
|---|---|---|
| 服务器 | `root@tencent.markli.cn` | SSH 免密已配置 |
| 项目目录 | `/opt/notice-service` | |
| 生产 compose | `docker-compose.quickstart.yml` | **单实例**，8081:8080 |
| 多实例 compose | `docker-compose.yml` | **不要动**，非当前生产形态 |
| 对外端口 | `8081` | 宿主机 → 容器 8080 |
| 数据库 | 容器 `notice-service-mysql-1`，MySQL **5.7** | `.env` 的 `MYSQL_IMAGE` 指定 |
| 加密密钥 | `/opt/notice-service/.notice-encrypt.key` 或 `.env` 的 `ENCRYPT_KEY` | **必须与库内密文一致** |
| 镜像仓库 | `jacknotes/notice-service:<tag>` | tag = `git describe --tags --always` |
| 构建加速 | `.env` 的 `IMAGE_PREFIX=docker.m.daocloud.io/library/` | 国内网络必需 |

镜像 tag 格式（来自 `deploy/publish.sh` 与 `Makefile`）：

```
v2.0.0-63-g45264f2     ← v2.0.0 + 63 个提交 + g<short-sha>
```

---

## 4. 手动流程（脚本不可用时的等价步骤）

> ⚠️ **不要**在 PowerShell 里手敲带嵌套引号的 `wsl -e bash -lc '...'`。PowerShell 会吞掉内层双引号，导致
> `unexpected EOF while looking for matching ')'`。**必须**把脚本写到文件再执行（见 §5）。

### 4.1 拉代码

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && git fetch origin && git merge --ff-only origin/main && git describe --tags --always'
```

`-dirty` 后缀是正常的（compose 的 image tag 改动未提交）。构建时用 `sed 's/-dirty//'` 去掉。

### 4.2 构建镜像

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && VERSION=$(git describe --tags --always | sed "s/-dirty//") && docker build \
  --build-arg BUILD_VERSION=$VERSION \
  --build-arg IMAGE_PREFIX=docker.m.daocloud.io/library/ \
  --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t jacknotes/notice-service:$VERSION \
  -t jacknotes/notice-service:latest .'
```

耗时约 1–3 分钟（含前端 `npm ci` + `vue-tsc` + `vite build` + Go 编译 + swag）。

### 4.3 验证镜像（**必做**，防止构建出空壳）

```bash
# 版本号是否真的注入
docker run --rm --entrypoint /app/notice-service jacknotes/notice-service:$VERSION --version
#   → notice-service v2.0.0-63-g45264f2

# 前端产物是否存在
docker run --rm --entrypoint sh jacknotes/notice-service:$VERSION -c 'ls /app/web/dist/index.html'

# 前端是否含本次修复（以 B-1 为例）
docker run --rm --entrypoint sh jacknotes/notice-service:$VERSION -c \
  'grep -o "term [^ ]*中秋[^ ]*" /app/web/dist/assets/Tasks-*.js | sort -u'
#   → term 中秋节    （若是 term 中秋 说明 dist 是旧的）
```

### 4.4 换 tag 并重启

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && \
  sed -i "s#image: jacknotes/notice-service:.*#image: jacknotes/notice-service:v2.0.0-63-g45264f2#" docker-compose.quickstart.yml && \
  grep -n "image: jacknotes" docker-compose.quickstart.yml && \
  docker compose -f docker-compose.quickstart.yml up -d'
```

`up -d` 只重建 notice-service 容器；MySQL 会显示 `Container notice-service-mysql-1 Running`（**未重建**），这是正确行为。

### 4.5 验证

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && \
  docker compose -f docker-compose.quickstart.yml ps --format "{{.Name}} {{.Image}} {{.Status}}" && \
  curl -s -o /dev/null -w "health=%{http_code}\n" http://127.0.0.1:8081/api/health && \
  docker compose -f docker-compose.quickstart.yml logs --tail=20 notice-service'
```

通过标准：
- 状态 `Up (healthy)`，**不是** `Restarting`
- `health=200`
- 日志有 `notice-service listening on :8080 (build <version>)`
- 日志**无** `Fatal` / `panic` / `拒绝启动`

### 4.6 回滚

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && \
  sed -i "s#image: jacknotes/notice-service:.*#image: jacknotes/notice-service:<旧tag>#" docker-compose.quickstart.yml && \
  docker compose -f docker-compose.quickstart.yml up -d'
```

旧镜像不会被自动清理，回滚只需换 tag。查看可用 tag：

```bash
ssh root@tencent.markli.cn 'docker images jacknotes/notice-service --format "{{.Tag}} {{.CreatedSince}}"'
```

---

## 5. 引号传递（最容易踩的坑）

`PowerShell → wsl.exe → bash → ssh → 远端 bash` 是**五层**，嵌套引号几乎必坏。

**错误示范**（PowerShell 吞引号）：

```powershell
wsl -e bash -lc 'TOK=$(curl ... | python3 -c "import sys,json; ...")'   # ← 报 unexpected EOF
```

**正确做法 A：写成 .sh 文件，只传路径**

```powershell
wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh
```

**正确做法 B：本地文件经 stdin 投递**

```powershell
wsl -e bash -lc 'tr -d "\r" < /mnt/c/Users/0799/AppData/Local/Temp/x.sh | ssh root@tencent.markli.cn "bash -s"'
```

**正确做法 C：远端用 heredoc**

```powershell
wsl -e bash -lc 'ssh root@tencent.markli.cn "bash -s" < ./script.sh'
```

> 另一个坑：从 Windows 写的 `.sh` 是 **CRLF** 行尾，bash 会报 `$'\r': command not found`。
> 必须 `sed -i 's/\r$//' file.sh` 或 `tr -d '\r'`。

---

## 6. 已知故障与处置（全部真实发生过）

### 6.1 🔴 `STRICT_SECRETS=1` 导致崩溃循环

**现象**：容器 `Restarting (1)`，日志反复 `拒绝启动`。

**根因**（2026-10-23 实际事故）：
- 旧版 H-1 用「去重字符数 < 12」判 `ADMIN_PASS`，人类口令天然字符集窄 → 误判
- 且 `ADMIN_PASS` 只在 `BootstrapAdmin` 首次建号时生效，admin 已存在后与登录无关

**已修复**（`v2.0.0-63`）：`WeakSecretWarnings` 只查 `JWT_SECRET`/`ENCRYPT_KEY`；
新增 `checkAdminPassOnBootstrap`，admin 已存在则跳过口令校验。

**若在旧镜像上遇到**：

```bash
# 应急：临时关闭 strict（不影响实际安全性，JWT/ENCRYPT 本就是强随机值）
ssh root@tencent.markli.cn 'cd /opt/notice-service && cp -a .env /tmp/.env.bak-$(date +%s) && sed -i "s/^STRICT_SECRETS=.*/STRICT_SECRETS=0/" .env && docker compose -f docker-compose.quickstart.yml up -d'
```

升级到 `v2.0.0-63+` 后再恢复 `STRICT_SECRETS=1`。

### 6.2 🔴 `ENCRYPT_KEY` 不匹配 → 渠道配置解密失败

**现象**：能启动但渠道列表报「配置解密失败」，或日志出现
`未设置 ENCRYPT_KEY 且未找到密钥文件，已自动生成新密钥`。

**根因**：`.env` 的 `ENCRYPT_KEY` 与库里密文所用的密钥不一致；或容器重建后
`.notice-encrypt.key` 没挂进容器（容器内路径不是宿主机的）。

**诊断**：

```bash
ssh root@tencent.markli.cn 'cd /opt/notice-service && grep -E "^ENCRYPT_KEY=" .env | cut -d= -f2- | wc -c'
# 32 字符（32 字节密钥）或 64 字符（hex 编码）为正常
```

**注意**：`.notice-encrypt.key` 在 `.gitignore` 里，**不在容器镜像内**。若走密钥文件路径，
必须 volume 挂载；当前生产用 `.env` 的 `ENCRYPT_KEY`，所以不受影响。

**关键**：部署时**不要**给 `docker compose` 传 `ENCRYPT_KEY` 环境变量覆盖 `.env` 的值。

### 6.3 🟡 镜像 tag 写错 → 拉到不存在的镜像

`quickstart` 用 `image:` 而非 `build:`，tag 必须**已经存在于本机**。若 tag 拼错，
`up -d` 会尝试 pull 并失败。构建后先 `docker images | grep <tag>` 确认。

### 6.4 🟡 `-dirty` 版本号

`git describe` 带 `-dirty` 说明工作区有未提交改动（通常是 compose 的 image tag 本身）。
**构建时用 `sed 's/-dirty//'` 去掉**，否则镜像 tag 会变成 `v2.0.0-63-g45264f2-dirty`，
下次部署对不上。

### 6.5 🟡 磁盘空间不足

构建镜像会临时占用数 GB（旧镜像层不会自动删）。检查：

```bash
ssh root@tencent.markli.cn 'df -h / && docker system df'
```

**实测现状**（2026-10-23）：根分区 69G / 已用 55G / 剩 12G（84%），
其中 build cache 占 **9.6G**、可回收镜像 10.4G。

清理（按安全度排序）：

```bash
# ① 最安全：只清构建缓存，不影响任何镜像与回滚能力
ssh root@tencent.markli.cn 'docker builder prune -f'

# ② 安全：只删悬空镜像（<none>:<none>）
ssh root@tencent.markli.cn 'docker image prune -f'

# ③ 有选择地删旧版本（保留最近 3 个用于回滚）
ssh root@tencent.markli.cn 'docker images jacknotes/notice-service --format "{{.Tag}}" | grep "^v" | sort -V | head -n -3 | xargs -r -I{} docker rmi jacknotes/notice-service:{}'

# ④ 危险：删所有未使用镜像，回滚能力丢失，勿在无备份时使用
# docker system prune -a
```

> 每次部署都会新增一个镜像层（约 122MB）且 `latest` 与版本 tag 指向同一 image id，
> 所以 `docker images` 里会出现两条同 ID 记录——这不算重复占用。

### 6.6 🟡 WSL 本地 vs 远程混淆

`wsl -e bash -lc '...'` 默认在**本地 WSL** 执行。要操作服务器必须显式 `ssh`。
典型错误：本地跑了 `docker exec`（本地没这个容器）却以为是远程。

### 6.7 🟢 宿主机 portproxy 陈旧规则

若宿主机曾配 `netsh interface portproxy` 指向旧 WSL IP（WSL2 重启后 IP 会变），
`localhost:3306` 等转发会失效。检查与清理：

```powershell
netsh interface portproxy show all
# 删除失效规则
netsh interface portproxy delete v4tov4 listenport=3306 listenaddress=0.0.0.0
```

注意：VMware 的 `vmnat` 也会占用端口（如 3306 转发到 VM），与 WSL 转发冲突。

---

## 7. 部署前检查清单

- [ ] 本地代码已提交并推送（`git status` 干净，`git log origin/main..HEAD` 为空）
- [ ] `go build ./... && go vet ./... && go test -p 1 ./... -count=1` 本地全绿
- [ ] 版本号符合预期（`git describe --tags --always`）
- [ ] `.env` 的 `JWT_SECRET` / `ENCRYPT_KEY` 长度 ≥32 且**从未变更**（变了会解不开旧渠道配置）
- [ ] 磁盘余量 >5G（`df -h /opt`）
- [ ] 已记录回滚点（当前 commit + 当前 image tag）

## 8. 部署后检查清单

- [ ] `docker compose ps` → notice-service `Up (healthy)`
- [ ] `curl http://127.0.0.1:8081/api/health` → `200` `{"status":"ok"}`
- [ ] 日志有 `listening on :8080 (build <version>)`，版本号与预期一致
- [ ] 日志无 `Fatal` / `panic` / `拒绝启动` / `自动生成新密钥`
- [ ] MySQL 容器仍是 `Up N weeks`（**未重建**）
- [ ] 页面能登录、渠道列表能正常解密显示

---

## 9. 快速诊断命令

```bash
# 线上跑什么版本
ssh root@tencent.markli.cn 'cd /opt/notice-service && git rev-parse --short HEAD && git describe --tags --always && docker compose -f docker-compose.quickstart.yml ps --format "{{.Name}} {{.Image}} {{.Status}}"'

# 最近日志（只看关键行）
ssh root@tencent.markli.cn 'cd /opt/notice-service && docker compose -f docker-compose.quickstart.yml logs --tail=50 notice-service 2>&1 | grep -E "警告|listening|Fatal|拒绝|panic"'

# 容器为什么重启
ssh root@tencent.markli.cn 'docker inspect notice-service-notice-service-1 --format "{{.State.Status}} exit={{.State.ExitCode}} err={{.State.Error}}"'

# 对比 compose 当前 tag 与运行中镜像
ssh root@tencent.markli.cn 'cd /opt/notice-service && grep "image: jacknotes" docker-compose.quickstart.yml && docker compose -f docker-compose.quickstart.yml ps --format "{{.Image}}" | grep notice-service'
```

---

## 10. 不要做的事

| ❌ 禁止 | 原因 |
|---|---|
| `docker compose down -v` | `-v` 删 `mysql_data` 卷 = **数据全丢** |
| 改 `.env` 的 `ENCRYPT_KEY` / `JWT_SECRET` | 前者解不开历史渠道配置；后者使全部登录态失效 |
| 在服务器上 `git reset --hard` 丢掉 compose 改动 | 会把 image tag 改回旧值 |
| 用 `docker-compose.yml`（多实例版）部署 | 当前生产是单实例 `quickstart`，端口/实例数都不同 |
| 直接 `docker run` 手工起容器 | 绕过 compose 的 env/依赖/健康检查 |
| 部署完不验证就离开 | 崩溃循环可能持续到磁盘/监控报警才发现 |
