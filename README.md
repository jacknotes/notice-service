# Notice Service · 通知发送服务

自托管通知发送服务：按你配置的规则，把提醒消息准时发给指定的人。支持邮箱、企业微信、钉钉、飞书、个人微信（PushPlus）等多种渠道，提供深色主题的 Web 管理界面，可多实例高可用部署。

## 功能特性

- **多渠道通知**：邮箱 (SMTP)、企业微信、钉钉（支持加签）、飞书、个人微信 (PushPlus)，统一适配器接口，易扩展
- **模板 + 变量**：Markdown 模板，`{{变量}}` 占位符，实时预览
- **任务触发**：Cron 定时触发 或 Webhook API 触发（唯一 api_key，可选 IP 白名单）
- **分布式高可用**：多实例部署时基于 MySQL 租约锁保证 Cron 任务不重复执行，单实例宕机自动接管
- **可靠性**：异步持久化发送队列（Webhook 立即返回 202），失败自动重试 3 次（5s→30s→60s），多副本原子认领不重复 + 崩溃自动接管，完整发送日志与保留期自动清理
- **安全**：JWT 无状态认证、bcrypt 密码、渠道敏感配置 AES-256-GCM 加密、**双因子认证（TOTP + 一次性备用码）**、登录失败限流
- **操作审计**：管理员操作审计日志（含来源 IP 与模块分类），Web 端「操作审计」页可按模块/操作/关键词/日期筛选 + 分页查看
- **仪表盘**：今日发送量、成功率、近 7/30 天发送趋势；发送日志含触发方式 / 触发人 / 触发 IP
- **多实例可观测**：各后端实例心跳上报，「信号在线」侧边栏显示健康节点数，点击可查看各节点状态（地址/版本/启动时间/最后心跳）
- **前端**：Vue3 + Element Plus 深色主题（"Signal Relay" 设计），PC/移动端响应式；列表支持排序与分页，渠道/模板/任务支持「复制」，模板/任务编辑实时预览，用户管理支持显示名/邮箱与管理员强制 2FA
- **部署**：Docker 多阶段构建，单实例镜像约 30-50MB

## 技术栈

| 组件 | 方案 |
|------|------|
| 后端 | Go + Gin + database/sql + MySQL |
| Cron 调度 | robfig/cron + MySQL 租约锁 |
| 前端 | Vue3 + Element Plus + Vite + ECharts |
| 认证 | JWT (golang-jwt) + bcrypt |
| 部署 | Docker 多阶段 + docker-compose |

## 快速开始（本地开发）

依赖：Go 1.25+、Node 20+、MySQL 5.7+（兼容 MariaDB）。

```bash
# 1. 准备数据库（本地 MariaDB/MySQL），创建库与用户：
#    CREATE DATABASE notice_service CHARACTER SET utf8mb4;
#    CREATE USER 'notice'@'%' IDENTIFIED BY 'notice123';
#    GRANT ALL ON notice_service.* TO 'notice'@'%';

# 2. 启动后端（自动建表 + 创建默认管理员 admin/admin123）
export DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=notice DB_PASSWORD=notice123 DB_NAME=notice_service
export JWT_SECRET=your-secret ENCRYPT_KEY=0123456789abcdef0123456789abcdef
go run ./cmd/server

# 3. 启动前端（开发模式，/api 代理到 :8080）
cd web && npm install && npm run dev
# 访问 http://127.0.0.1:5173
```

> 更简单的方式：直接 `make dev`（见下方「Makefile 使用说明」），一键启动后端 :8080 + 前端 :5173。

## Makefile 使用说明

所有命令见 `make help`。按场景：

### 首次准备
```bash
make deps    # 一键安装：npm install（前端）+ go mod download（后端）
```

### 本地开发（推荐日常用）
```bash
make dev     # 编译后端 :8080（release）+ 前端 Vite :5173（热更新；/api、/swagger 自动代理到 :8080）
```
访问 **http://localhost:5173**。前提：本机 MySQL（`.dev/mysql-run`）在跑，且 :8080 未被占用（有旧进程先 `kill <pid>`）。

### 只跑后端 / 只跑前端
```bash
make run             # 编译并启动后端 :8080（默认 release）
make dev-backend     # 后端 :8080，GIN_MODE=debug（多请求日志，便于排查）
make prod-backend    # 后端 :8080，GIN_MODE=release
make frontend-dev    # 仅前端 Vite :5173（代理到 127.0.0.1:8080）
make frontend-build  # 构建前端产物到 web/dist
make frontend-install# 仅安装前端依赖
```

### 构建与检查
```bash
make build    # 先生成 Swagger 文档，再静态编译后端到 .dev/notice-service（CGO_ENABLED=0）
make swagger  # 只重新生成 Swagger 文档（改过 handler 注解后执行）
make test     # 全部 Go 测试（需本地 notice_service_test 测试库）
make vet      # 静态检查
make fmt      # 格式化 Go 代码
```

### Docker（前后端合一镜像）
```bash
make docker-build   # docker build -t notice-service .（多阶段：前端→后端→运行时，镜像内含前端与 Swagger）
make docker-up      # docker compose up -d（2 实例 + MySQL 5.7 高可用）
make docker-down    # 停止
make docker-logs    # 跟随日志
```

### 运维 / 清理
```bash
make db-clean FORCE=1   # 危险：清空 notice_service 与 notice_service_test 全部数据
make clean              # 删除 .dev/notice-service、web/dist、web/node_modules、docs/swagger
```

## Docker 部署（多实例高可用）

```bash
# 1. 复制并修改配置（必须改：DB_PASSWORD / MYSQL_ROOT_PASSWORD / JWT_SECRET /
#    ENCRYPT_KEY（多实例必须一致）/ ADMIN_PASS；未设置时 docker compose 会直接报错）
cp .env.example .env
vi .env

# 2. 启动（自动构建镜像，起 2 个服务实例 + MySQL 5.7）
docker compose up -d

# 3. 宿主机 Nginx 负载均衡 + 健康检查
#    upstream notice { server 127.0.0.1:8080; server 127.0.0.1:8081; }
#    location / { proxy_pass http://notice; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; }
#    /api/health 可用于健康检查（主动摘除故障实例，DB 不可达时返回 503）
```

> 首次启动自动创建默认管理员：`admin` / `admin123`，请尽快修改。

### 国内网络 / 无法访问 Docker Hub 时

服务构建需拉取基础镜像（node/golang/alpine/mysql）。若服务器访问不到 Docker Hub，在 `.env` 设置镜像源即可（不改服务器 Docker 配置）：

```bash
IMAGE_PREFIX=docker.m.daocloud.io/library/   # 基础镜像前缀（node/golang/alpine）
MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:5.7
```

### 反向代理与 IP 白名单

Webhook 的 IP 白名单依赖 `X-Real-IP` / `X-Forwarded-For` 头，但这些头**只有来自可信反向代理才是可信的**。
服务通过 `TRUSTED_PROXIES`（默认 `127.0.0.1,::1`）声明可信代理来源：

- 典型部署「宿主 Nginx → 本机端口」：保持默认即可（Nginx 从 127.0.0.1 连入）。
- Nginx 在其它节点 / 容器网络内：把 `TRUSTED_PROXIES` 设为反代所在网段，如 `10.0.0.0/8`。
- 不要直接把服务暴露公网却不设反向代理——此时任何人可伪造 `X-Forwarded-For` 绕过 IP 白名单。

## 环境变量

> 除环境变量外，也支持 `config.yml` 配置文件（参考 `config.example.yml`）。优先级：环境变量 > config.yml > 默认值。启动时可用 `--config <path>` 或 `CONFIG_FILE` 环境变量指定配置文件路径（默认 `./config.yml`）。

| 变量 | 默认 | 说明 |
|------|------|------|
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | 127.0.0.1 / 3306 / notice / notice123 / notice_service | 数据库连接 |
| `JWT_SECRET` | change-me | JWT 签名密钥（多实例必须一致） |
| `ENCRYPT_KEY` | 随机生成 | 渠道配置 AES 密钥（多实例必须一致） |
| `PORT` | 8080 | HTTP 端口 |
| `INSTANCE_ID` | 自动 UUID | 实例标识（租约锁所有权） |
| `ADMIN_USER` / `ADMIN_PASS` | admin / admin123 | 默认管理员（首启创建） |
| `QUEUE_WORKERS` | 4 | 每实例发送 worker 数 |
| `QUEUE_POLL_MS` | 1000 | 队列认领轮询间隔（毫秒） |
| `QUEUE_MAX_ATTEMPTS` | 3 | 队列级最大尝试次数（含首次） |
| `QUEUE_RETRY_BACKOFF` | 5s,30s,60s | 重试间隔（逗号分隔） |
| `QUEUE_CLAIM_TTL` | 120 | 认领后多久算陈旧（秒），超时由其它实例接管 |
| `LOG_RETENTION_DAYS` | 90 | 发送日志保留天数 |
| `QUEUE_JOB_RETENTION_DAYS` | 30 | 已完成 job 保留天数 |
| `AUDIT_RETENTION_DAYS` | 180 | 审计日志保留天数（超出自动清理） |
| `TRUSTED_PROXIES` | 127.0.0.1,::1 | 可信反向代理 CIDR（逗号分隔），控制 X-Forwarded-For / X-Real-IP 是否可信 |
| `SWAGGER_ENABLED` | true | 是否暴露 `/swagger` API 文档 |

## 密码重置

忘记密码时按场景选择：

1. **多管理员 / 普通用户**：任一管理员登录后，在「用户管理」对该用户点「重置密码」，生成一次性令牌（15 分钟有效、用完即焚），线下转交；该用户到登录页点「忘记密码」，输入用户名 + 令牌 + 新密码即可自助重置。

2. **唯一 admin 忘记密码（离线重置）**：在服务器上运行（不启动服务，不影响运行中的实例）：

   ```bash
   # 需能连上数据库；密码至少 12 位，含大小写字母、数字、特殊字符
   ./notice-service reset-password --username admin --new-password 'NewPass1234!'
   ```

   - 不带 `--new-password` 时进入交互式输入，密码不回显、不落 shell 历史，更安全。
   - `--username` 默认取 `ADMIN_USER`（默认 `admin`）；也支持重置任意普通用户。
   - 使用 `Docker` 部署时：`docker compose exec <service> ./notice-service reset-password --username admin`（交互输入）。

3. **日常改密**：登录后在「个人设置 → 修改密码」（需原密码）。

## API 概览

```
认证   POST /api/auth/login   账号密码登录（已开启 2FA 时返回待验证令牌）
      POST /api/auth/2fa/verify   双因子验证（登录第二步，换取 JWT）
      POST /api/auth/2fa/setup    生成 TOTP 密钥与备用码（个人设置）
      POST /api/auth/2fa/enable   启用双因子认证
      POST /api/auth/2fa/disable  关闭双因子认证（需校验动态码/备用码）
      POST /api/auth/forgot-password   一次性令牌自助重置密码
渠道  GET/POST /api/channels  PUT/DELETE /api/channels/:id  POST /api/channels/:id/test
模板  GET/POST /api/templates  PUT/DELETE /api/templates/:id  POST /api/templates/:id/preview
用户  GET/POST /api/users  PUT/DELETE /api/users/:id  POST /api/users/:id/reset-token
      POST /api/users/:id/2fa-enable  管理员强制开启双因子认证（返回密钥+备用码）
      POST /api/users/:id/2fa-disable 管理员强制关闭双因子认证
任务  GET/POST /api/tasks  PUT/DELETE /api/tasks/:id  POST /api/tasks/:id/toggle
      POST /api/tasks/preview   任务发送预览（渲染标题/正文/接收地址）
      GET /api/tasks/:id/logs
日志  GET /api/logs   分页/筛选 + 排序（sort_by/sort_order，含触发方式/人/IP）
      POST /api/logs/:id/retry  重试失败日志
审计  GET /api/audit   操作审计日志（仅管理员，模块/操作/关键词/日期筛选 + 分页）
系统  GET /api/instances  后端节点健康列表（多实例「信号在线」）
外部  POST /api/webhook/:api_key   （无需登录，可选 IP 白名单；异步入队，返回 202）
仪表盘 GET /api/dashboard/stats   GET /api/dashboard/trend   GET /api/dashboard/top-tasks   GET /api/dashboard/channel-stats
```

Webhook 触发示例：

```bash
curl -X POST https://your-host/api/webhook/<api_key> \
  -H 'Content-Type: application/json' \
  -d '{"variables":{"name":"张三","time":"10:00"}}'
```

### 双因子认证（2FA）

登录「个人设置 → 双因子认证」扫码（或手动输入密钥）绑定认证器 App（Google/Microsoft Authenticator、1Password 等），
同时会生成 8 个一次性备用码（仅显示一次，请妥善保存，手机丢失时用于登录）。开启后登录流程变为两步：
密码验证通过后，再输入认证器动态码（或备用码）换取登录令牌。关闭 2FA 时需再次校验动态码/备用码。

管理员可在「用户管理 → 2FA」对任意用户**强制开启**（重新生成密钥与备用码，弹窗展示二维码/密钥/备用码，
线下转交该用户绑定）或**强制关闭**（用户丢失手机与备用码时的恢复路径）。

### 多实例健康（信号在线）

多实例部署时，每个后端实例每 5 秒向 `instance_heartbeats` 表上报心跳。侧边栏「信号在线」显示
健康节点数（全部健康=绿、部分离线=琥珀、全部离线=红），点击弹出节点列表（节点 ID/地址/版本/
启动时间/最后心跳/健康状态）；超过 15 秒未上报的节点判定为离线。实例优雅退出时会移除自身心跳。

## 项目结构

```
cmd/server/           # 入口（含优雅退出/HTTP 超时）
internal/
  config/ model/ crypto/ database/ repository/ service/ channel/ render/
  scheduler/          # Cron + MySQL 租约锁
  handler/ middleware/ router/
web/                  # Vue3 前端
internal/database/migrations/  # 数据库迁移 SQL（go:embed 内嵌，启动自动应用）
Dockerfile  docker-compose.yml  .env.example
```

## 文档

- **服务器部署指南**：`docs/deployment.md`（从空服务器 clone 代码 → 构建 → 运行 → Nginx 反代 → 升级 → 排错，含国内镜像与常见坑）
- 设计文档：`docs/superpowers/specs/2026-07-17-notification-service-design.md`
- 实现计划：`docs/superpowers/plans/2026-08-18-notice-service-implementation.md`
