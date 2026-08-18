# Changelog

本项目遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。所有显著变更都会记录在本文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [Unreleased]

### 计划中
- 用户管理（多用户注册/禁用/角色管理）
- 忘记密码 / 邮箱验证
- 发送日志详情页
- Webhook 异步确认（202 排队语义）
- 租约自动续期（超长任务防双发）

## [1.0.0] - 2026-08-18

首个正式发布：完整的自托管通知发送服务，支持多实例高可用部署。

### 🚀 主要功能

- **多渠道通知**：统一 `Channel` 适配器接口，内置 5 种渠道
  - 邮箱（SMTP，自动 STARTTLS）
  - 企业微信（Webhook 机器人）
  - 钉钉（Webhook + HMAC-SHA256 加签）
  - 飞书（Webhook 机器人）
  - 个人微信（PushPlus）
- **模板系统**：Markdown 模板 + `{{变量}}` 占位符，变量默认值 + 覆盖，服务端与前端双预览
- **任务管理**：Cron 定时触发 + Webhook API 触发（唯一 `api_key`，可选 IP 白名单，支持 CIDR）
- **发送可靠性**：失败自动重试 3 次（间隔 5s/30s/60s），完整 `task_logs` 发送日志
- **分布式高可用**：多实例部署时基于 MySQL 租约锁保证 Cron 任务不重复执行，实例崩溃 60s 自动接管
- **仪表盘**：今日发送量/成功率/失败数、近 7/30 天发送趋势（ECharts）
- **认证安全**：JWT 无状态认证（24h）、bcrypt 密码、渠道敏感配置 AES-256-GCM 加密、修改密码
- **前端**：Vue3 + Element Plus "Signal Relay" 深色主题，PC/移动端响应式（侧边栏 ↔ 底部导航）

### ✨ 新增

- Go (Gin) 后端骨架：配置加载、数据模型、AES 加密、数据库连接与幂等迁移
- 5 张数据表：`users` / `channels` / `templates` / `tasks` / `task_logs`
- JWT 认证服务 + Gin 中间件，默认管理员自动创建（容错多实例并发建号竞态）
- 通知发送引擎（渲染 → 逐接收者发送 → 重试 → 日志）
- Cron 调度器：5 字段标准表达式、`SkipIfStillRunning` + `Recover` 链、租约锁集成
- HTTP 层：认证/健康检查/仪表盘/渠道/模板/任务/Webhook handler + 路由
- 服务入口：静态文件托管 + SPA fallback（不吞 `/api` 404）
- 前端脚手架（Vite + Vue3 + Pinia + vue-router + Element Plus + ECharts）
- 前端设计系统（tokens.css 单一来源 + 全局样式）
- API 客户端（JWT 拦截器 + 401 自动登出）、认证 store、路由守卫
- 7 个页面：登录 / 仪表盘 / 渠道管理 / 模板管理 / 任务管理 / 发送日志 / 个人设置
- Docker 多阶段构建（node:20 → golang:1.25 → alpine:3.18，镜像 ~30-50MB）
- docker-compose 双实例高可用 + MySQL 5.7
- GitHub Actions CI（后端测试 + 前端构建 + Docker 镜像构建）
- Nginx 反向代理 + 负载均衡配置模板

### 🐛 修复

- Webhook 渠道（飞书/PushPlus）静默吞错：`checkWebhookResp` 同时识别 `errcode` 与 `code`
- 邮箱渠道连接测试缺 STARTTLS/超时导致的误判
- Cron 调度器 `WithSeconds()` 拒绝 5 字段表达式导致的静默不调度
- 租约实例 ID 硬编码导致的多实例锁误释放
- 通知服务默认 Instancer 缺 cipher 导致的 nil panic
- SPA fallback 吞掉 `/api/*` 404
- Webhook IP 白名单 malformed JSON fail-open
- 登录页 Enter 双重提交
- localStorage JSON 损坏导致应用挂起
- 删除任务/渠道/模板被外键（1451）阻塞 → `ON DELETE CASCADE`
- Webhook 使用说明错误（应为 `POST /api/webhook/:api_key`）
- 邮件渠道 Markdown 未渲染为 HTML
- 列表接口返回 `null` 而非 `[]`

### 📦 依赖

- Go 1.25、Gin、go-sql-driver/mysql、golang-jwt/v5、robfig/cron/v3、gomarkdown、google/uuid、golang.org/x/crypto
- Vue 3.4、Element Plus、Vite 5、Pinia、vue-router、ECharts、marked、axios

### 安全提示（部署前必读）

- 生产环境务必通过 `docker compose --env-file` 覆盖默认密钥：`JWT_SECRET`、`ENCRYPT_KEY`（多实例必须一致）、`ADMIN_PASS`、`MYSQL_ROOT_PASSWORD`
- 首次登录后立即修改默认管理员密码 `admin/admin123`
- 服务必须置于可信 Nginx 反向代理之后（`X-Real-IP` / `X-Forwarded-For` 才可信，Webhook IP 白名单依赖此）
- Google Fonts 在中国大陆不可达时前端自动降级为系统字体（不影响功能）

[Unreleased]: https://github.com/jacknotes/notice-service/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/jacknotes/notice-service/releases/tag/v1.0.0
