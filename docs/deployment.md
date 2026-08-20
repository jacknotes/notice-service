# Notice Service · 服务器部署指南

> 本文档基于 172.168.2.12 生产环境的实际部署过程整理，覆盖「从一台空服务器 clone 代码 → 构建 → 运行 → 反代 → 升级」的完整流程，并把部署中踩过的坑（Docker 版本、国内镜像、nginx include、密钥一致性等）单独列出。
>
> 适用目标：一台全新的 Linux 服务器，安装好 Docker 后按本文档即可跑起**双实例高可用**的通知服务（2 个应用实例 + 1 个 MySQL）。

---

## 0. 架构与端口约定

```
                 ┌────────────────────────────┐
   浏览器 ──▶ Nginx (80)                       │
                 │  upstream notice_service   │
                 │   server 127.0.0.1:8080    │   docker
                 │   server 127.0.0.1:8081    │──▶ notice-service-1 (8080)
                 └────────────────────────────┘        notice-service-2 (8081)
                                                        mysql (127.0.0.1:3306，仅本机)
```

- 应用：2 个实例（端口 8080 / 8081），通过 MySQL 租约锁 + 发送队列实现高可用与不重复投递
- MySQL：仅绑定 `127.0.0.1`，不对公网暴露
- 对外入口：Nginx 80 端口负载均衡；`/api/health` 供健康检查

---

## 1. 前置条件

| 依赖 | 版本要求 | 说明 |
|---|---|---|
| Linux | 任意主流发行版 | 本文以 Ubuntu 18.04 实测 |
| Docker | **19.03+ 即可** | Dockerfile 已不使用 `RUN --mount=type=cache`（旧版 BuildKit 不支持），低版本也能构建 |
| docker-compose | v2（`docker-compose` 独立二进制 或 `docker compose` 插件） | 二者其一可用即可 |
| git | 任意 | 拉取代码 |
| 网络 | 能访问 Docker 镜像源 | 国内网络见「4.2 国内镜像」 |

> 查看版本：
> ```bash
> docker --version          # Docker version 19.03.15 ...
> docker-compose --version  # Docker Compose version v2.39.2 ...
> ```

> ⚠️ 注意 `docker compose`（带空格，需要插件）和 `docker-compose`（独立二进制）是两种安装形态，本机装了哪个就用哪个命令。用 `docker-compose --version` 验证。

---

## 2. 克隆代码

```bash
# 建议放到 /opt 下，目录名固定为 notice-service，方便后续升级脚本
cd /opt
git clone https://github.com/jacknotes/notice-service.git
cd notice-service
```

> 若服务器需通过 SSH 拉取，将 URL 换成 `git@github.com:jacknotes/notice-service.git`（需在服务器上配置好 deploy key）。

---

## 3. 准备 `.env`（密钥配置）

复制示例并**务必修改全部密钥**：

```bash
cp .env.example .env
```

关键变量（`docker-compose.yml` 已用 `${VAR:?}` 校验，**缺失会直接报错拒绝启动**，杜绝弱默认裸跑）：

| 变量 | 必填 | 说明 |
|---|---|---|
| `DB_PASSWORD` | ✅ | 业务库密码（同时用于 MySQL 的 MYSQL_PASSWORD） |
| `MYSQL_ROOT_PASSWORD` | ✅ | MySQL root 密码 |
| `JWT_SECRET` | ✅ | JWT 签名密钥，**两个实例必须一致** |
| `ENCRYPT_KEY` | ✅ | 渠道配置 AES-256-GCM 密钥，**两个实例必须一致**（改它会导致已存渠道配置无法解密） |
| `ADMIN_USER` / `ADMIN_PASS` | ✅ | 首启创建的管理员账号（**不是默认的 admin123**，见「6.3」） |

用随机强密码生成（推荐在服务器上直接生成）：

```bash
openssl rand -hex 32   # JWT_SECRET（64 位十六进制）
openssl rand -hex 16   # ENCRYPT_KEY（32 位十六进制，正好 32 字节）
openssl rand -base64 15  # 密码类：含大小写/数字/符号，可作 DB_PASSWORD / ADMIN_PASS
```

> **密钥一致性的坑**：`JWT_SECRET` 与 `ENCRYPT_KEY` 是所有实例共享的。若多实例不一致，会出现「一台实例签发的 token 另一台验不过」「渠道配置解密失败」等问题。单 `.env` 文件被两个实例共同读取，天然保证一致。

---

## 4. 国内网络 / Docker Hub 不可达（重要）

国内服务器经常连不上 `registry-1.docker.io`（实测超时）。**不需要改服务器 Docker 配置**，只需在 `.env` 里指定镜像源前缀：

```bash
# .env 追加
IMAGE_PREFIX=docker.m.daocloud.io/library/          # 基础镜像：node / golang / alpine
MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:5.7  # MySQL 镜像
```

> 备选镜像源（实测可用）：`docker.1panel.live`、`dockerproxy.net`。不设置时默认走官方 Docker Hub。

---

## 5. 构建并启动

```bash
cd /opt/notice-service
export DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1   # 显式开启 BuildKit（构建更快）
docker-compose up -d --build
```

首次构建会：拉取基础镜像（node/golang/alpine/mysql）→ 构建前端 → 编译后端 → 启动 MySQL + 2 个应用实例。

> 构建耗时受网络影响（国内镜像一般 5~15 分钟）。**建议用 nohup 后台执行**避免 SSH 断开中断：
> ```bash
> nohup docker-compose up -d --build > deploy.log 2>&1 &
> tail -f deploy.log
> ```

启动后应看到 3 个容器：

```
NAME                              STATUS                      PORTS
notice-service-notice-service-1-1  Up (healthy)                0.0.0.0:8080->8080/tcp
notice-service-notice-service-2-1  Up (healthy)                0.0.0.0:8081->8080/tcp
notice-service-mysql-1             Up (healthy)                127.0.0.1:3306->3306/tcp
```

容器已配置 `restart: unless-stopped`，MySQL 数据卷 `notice-service_mysql_data` 持久化。

---

## 6. 验证

### 6.1 健康检查（LB / 容器健康检查都靠它）

```bash
curl -s http://127.0.0.1:8080/api/health
# {"status":"ok"}
```

> `/api/health` 会探测数据库连通性：**DB 不可达时返回 503**，负载均衡/容器健康检查据此摘除故障实例。

### 6.2 登录验证

```bash
curl -s -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"<你的ADMIN_PASS>\"}"
# 返回 {"token":"..."} 即成功
```

### 6.3 ⚠️ 首次登录密码

**默认管理员密码不是 admin123**，而是 `.env` 里设置的 `ADMIN_PASS`。因为系统密码强度规则要求 ≥12 位且含大小写字母、数字、特殊字符，`admin123` 本来就设置不上去。首次登录后请立即在「个人设置 → 修改密码」改成自己的密码。

> 登录失败限流：同一用户名连续失败 5 次会锁定 15 分钟（即使密码正确也会提示稍后再试），重启容器可立即解除。

---

## 7. Nginx 反向代理 + 负载均衡

配置模板见 `deploy/nginx.conf.example`。核心：

```nginx
upstream notice_service {
    server 127.0.0.1:8080;
    server 127.0.0.1:8081;
}

server {
    listen       80;
    server_name notice.hs.com;          # 改成你的域名（DNS 需解析到本机）
    location / {
        add_header backendIP $upstream_addr;   # 调试时显示由哪个实例处理
        proxy_redirect off;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Real-Port $remote_port;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://notice_service;
        error_page 500 502 503 504 /50x.html;
    }
}
```

### ⚠️ nginx 的坑：conf.d 未必被加载

部分服务器的 `nginx.conf` 并没有 `include /usr/local/nginx/conf/conf.d/*.conf;`（172.168.2.12 就是这样，主配置只 include 了 `mime.types`）。直接把文件丢进 conf.d 再 `nginx -s reload` **不会生效**。

判断方法（把本机 conf.d 里的 server_name 替换后 curl 看是否命中）：
```bash
nginx -T | grep -c 'conf.d'   # 0 = conf.d 未被加载
```

解决：在 `nginx.conf` 的 `http {}` 里加一行 **只 include 自己的那个文件**（避免加载 conf.d 里其它旧配置）：
```nginx
include /usr/local/nginx/conf/conf.d/notice-service.conf;
```
然后：
```bash
nginx -t && nginx -s reload   # 先校验再重载（优雅，不断连接）
```

### 关于 TRUSTED_PROXIES

Nginx 会把真实客户端 IP 通过 `X-Real-IP` / `X-Forwarded-For` 带给应用（Webhook 的 IP 白名单依赖它）。应用侧用 `TRUSTED_PROXIES` 声明可信代理来源：

- **宿主机 Nginx → 本机端口**（默认形态）：保持默认 `TRUSTED_PROXIES=127.0.0.1,::1` 即可
- **Nginx 在其它节点 / 容器网络**：改成反代所在网段，如 `10.0.0.0/8`

> 安全提示：不要把服务直接暴露公网而不经过可信反代——否则任何人可伪造 `X-Forwarded-For` 绕过 IP 白名单。

---

## 8. 升级部署（拉新代码重新构建）

```bash
cd /opt/notice-service
git pull                        # 拉取最新代码（.env 已 gitignore，不会被覆盖）
docker-compose up -d --build    # 重建镜像并滚动重启应用（MySQL 数据保留）
```

- 只会重建/重启应用容器，**MySQL 与数据卷不动**
- `.env` 不在 git 仓库里，`git pull` 不会覆盖你的密钥

---

## 9. 运维命令速查

```bash
docker-compose ps                # 查看容器状态
docker-compose logs -f notice-service-1   # 跟随应用日志
docker-compose down              # 停止（数据卷保留）
docker-compose down -v           # 停止并清空数据（危险，慎用）

# 离线重置任意用户密码（不启动/不影响运行中的服务；需能连数据库）
docker compose exec <service> ./notice-service reset-password --username admin
#   按提示交互输入新密码（需 ≥12 位，含大小写字母、数字、特殊字符）
```

---

## 10. 常见问题（部署中实际踩过的坑）

| 现象 | 原因 | 解决 |
|---|---|---|
| `dial tcp ... registry-1.docker.io ... connection timed out` | 连不上 Docker Hub（国内网络） | `.env` 设置 `IMAGE_PREFIX` / `MYSQL_IMAGE` 走国内镜像 |
| `Dockerfile parse error line N: Unknown flag: mount` | Docker ≤19.03 的旧 BuildKit 不支持 `RUN --mount=type=cache` | 当前 Dockerfile 已移除该语法，升级代码即可；若自改 Dockerfile 勿用 `--mount` |
| `docker compose` 提示 `'compose' is not a docker command` | 只装了独立二进制，没装插件 | 改用 `docker-compose` 命令 |
| 登录报「用户名或密码错误」 | 密码不是 admin123，是 `.env` 的 `ADMIN_PASS`；或密码被改过 | 用 `.env` 里的密码；被改过就重置 |
| 登录提示「失败次数过多」 | 连续输错 5 次触发 15 分钟限流 | 等 15 分钟或重启容器 |
| `/api/health` 返回 503 | 数据库不可达 | 检查 mysql 容器与 `DB_*` 配置 |
| 配置了 nginx 但不生效 | `nginx.conf` 没 include conf.d | `nginx -T` 确认，加 include 后 `nginx -t && nginx -s reload` |
| 渠道「解密失败」/ 列表拿不到配置 | `ENCRYPT_KEY` 与加密时不一致（多实例不一致 / 数据迁移未重加密） | 保证各实例 `ENCRYPT_KEY` 一致；迁移数据见「11」 |
| 前端接口返回 `401` 且跳登录 | JWT 过期（24h）或两实例 `JWT_SECRET` 不一致 | 重新登录；统一 `JWT_SECRET` |

---

## 11. 数据迁移到另一台服务器（重点：渠道加密）

渠道的敏感配置（SMTP 密码、Webhook token 等）是用 **`ENCRYPT_KEY` 做 AES-256-GCM 加密**后存进 `channels.config_json` 的。直接导出 SQL 再导入目标库**无法解密**，因为两端密钥不同。

两种安全做法：

1. **让目标服务器用相同的 `ENCRYPT_KEY`**（最简单）：把源服务器的 `ENCRYPT_KEY` 填进目标 `.env`，然后直接迁移 `channels` / `templates` / `tasks` 三张表（注意外键：先渠道/模板后任务；`api_key` 一并迁移可保住 Webhook URL）。
2. **解密后重加密**（目标密钥不同）：用源密钥解密 `config_json` → 用目标密钥重新加密 → 再导入。可用一个一次性脚本调用 `internal/crypto` 完成（172.168.2.12 迁移即采用此法）。

> 迁移时建议同步 `tasks.api_key`（否则 Webhook 触发 URL 会变），并把 `locked_by/locked_at` 清空（租约状态属运行时数据）。

---

## 12. 相关文档

- 部署配置模板：`deploy/nginx.conf.example`、`.env.example`、`config.example.yml`
- 功能与命令总览：`README.md`
- 架构说明：`docs/architecture/notice-service-architecture.html`
- 设计文档：`docs/superpowers/specs/2026-07-17-notification-service-design.md`
