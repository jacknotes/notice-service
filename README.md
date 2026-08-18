# Notice Service · 通知发送服务

自托管通知发送服务：按你配置的规则，把提醒消息准时发给指定的人。支持邮箱、企业微信、钉钉、飞书、个人微信（PushPlus）等多种渠道，提供深色主题的 Web 管理界面，可多实例高可用部署。

## 功能特性

- **多渠道通知**：邮箱 (SMTP)、企业微信、钉钉（支持加签）、飞书、个人微信 (PushPlus)，统一适配器接口，易扩展
- **模板 + 变量**：Markdown 模板，`{{变量}}` 占位符，实时预览
- **任务触发**：Cron 定时触发 或 Webhook API 触发（唯一 api_key，可选 IP 白名单）
- **分布式高可用**：多实例部署时基于 MySQL 租约锁保证 Cron 任务不重复执行，单实例宕机自动接管
- **可靠性**：发送失败自动重试 3 次（间隔 5s→30s→60s），完整发送日志
- **安全**：JWT 无状态认证、bcrypt 密码、渠道敏感配置 AES-256-GCM 加密
- **仪表盘**：今日发送量、成功率、近 7/30 天发送趋势
- **前端**：Vue3 + Element Plus 深色主题（"Signal Relay" 设计），PC/移动端响应式
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

## Docker 部署（多实例高可用）

```bash
# 1. 复制并修改配置
cp .env.example .env   # 修改密码和密钥（JWT_SECRET、ENCRYPT_KEY 多实例必须一致）

# 2. 启动（自动构建镜像，起 2 个服务实例 + MySQL 5.7）
docker compose up -d

# 3. 宿主机 Nginx 负载均衡 + 健康检查
#    upstream notice { server 127.0.0.1:8080; server 127.0.0.1:8081; }
#    location / { proxy_pass http://notice; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; }
#    /api/health 可用于健康检查（主动摘除故障实例）
```

> 首次启动自动创建默认管理员：`admin` / `admin123`，请尽快修改。

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | 127.0.0.1 / 3306 / notice / notice123 / notice_service | 数据库连接 |
| `JWT_SECRET` | change-me | JWT 签名密钥（多实例必须一致） |
| `ENCRYPT_KEY` | 随机生成 | 渠道配置 AES 密钥（多实例必须一致） |
| `PORT` | 8080 | HTTP 端口 |
| `INSTANCE_ID` | 自动 UUID | 实例标识（租约锁所有权） |
| `ADMIN_USER` / `ADMIN_PASS` | admin / admin123 | 默认管理员（首启创建） |

## API 概览

```
POST  /api/auth/login           登录（返回 JWT）
GET   /api/health               健康检查
渠道  GET/POST /api/channels  PUT/DELETE /api/channels/:id  POST /api/channels/:id/test
模板  GET/POST /api/templates  PUT/DELETE /api/templates/:id  POST /api/templates/:id/preview
任务  GET/POST /api/tasks  PUT/DELETE /api/tasks/:id  POST /api/tasks/:id/toggle  GET /api/tasks/:id/logs
外部  POST /api/webhook/:api_key   （无需登录，可选 IP 白名单）
仪表盘 GET /api/dashboard/stats   GET /api/dashboard/trend
```

Webhook 触发示例：

```bash
curl -X POST https://your-host/api/webhook/<api_key> \
  -H 'Content-Type: application/json' \
  -d '{"variables":{"name":"张三","time":"10:00"}}'
```

## 项目结构

```
cmd/server/           # 入口
internal/
  config/ model/ crypto/ database/ repository/ service/ channel/ render/
  scheduler/          # Cron + MySQL 租约锁
  handler/ middleware/ router/
web/                  # Vue3 前端
migrations/           # 数据库迁移 SQL
Dockerfile  docker-compose.yml  .env.example
```

## 文档

- 设计文档：`docs/superpowers/specs/2026-07-17-notification-service-design.md`
- 实现计划：`docs/superpowers/plans/2026-08-18-notice-service-implementation.md`
