# Changelog

本项目遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。所有显著变更都会记录在本文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [Unreleased]

### 已实现
- 异步持久化发送队列：Webhook 触发立即返回 202，发送由后台 worker 池消费
- 多副本正确性：MySQL 原子认领保证不重复；陈旧认领自动接管（替代任务锁续期）
- 队列级重试（5s/30s/60s），取代发送内部 sleep 重试
- task_logs / send_jobs 保留期自动清理（LOG_RETENTION_DAYS / QUEUE_JOB_RETENTION_DAYS）
- cron 任务 last_run_at / next_run_at 现已写入
- 任务支持多投递渠道（向全部所选渠道扇出发送），channel_id 兼容旧数据
- 密码强度规则：至少 12 位，须含大小写字母、数字、特殊字符（创建/重置/改密一致）
- 迁移改为 schema_migrations 跟踪 + GET_LOCK 串行（支持非幂等迁移语句）
- 新建任务表单必填字段统一红色 * 标识
- 发送日志支持日期范围筛选（今天/最近一周/最近一个月快捷方式，默认最近一个月，跨度上限 1 年）
- 用户管理用户名显示修复（换行/溢出处理 + 悬停显示全名）
- 忘记密码（方案A）：管理员生成一次性重置令牌（15 分钟有效、用完即焚），用户登录页自助设新密码
- 登录失败限流：同一用户名连续失败 5 次锁定 15 分钟（防暴力破解）
- Webhook API Key 支持 Authorization 头（兼容 URL path），每 key 限流 60 次/分钟
- 仪表盘趋势改为单条 GROUP BY（消除 N 次 COUNT 查询）
- 批量删除改为单条 UPDATE ... WHERE id IN
- 任务支持「立即发送」按钮（入队，不依赖 cron 到点）；列表新增「投递渠道」列
- 发送日志改为后端分页 + 任务/状态/日期筛选下推 DB
- 管理员操作审计日志（登录/登出、用户/渠道/模板/任务的增删改）
- 弱默认密钥启动告警（JWT_SECRET / ENCRYPT_KEY）
- 修复：定时任务改为 Webhook API 后自动生成 API Key；api→api 编辑保留原 Key；切回定时清空 Key（旧 URL 立即失效）
- 修复：cron 任务 api_key 改以 NULL 落库，消除多定时任务撞唯一键的问题
- 新增：`reset-password` 离线重置命令（唯一 admin 忘记密码时可恢复），并补充 README 密码重置文档
- 样式：个人设置页卡片与内容居中；用户管理用户名自动换行完整显示

### 计划中
- 用户管理（多用户注册/禁用/角色管理）
- 忘记密码 / 邮箱验证
- 发送日志详情页

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

## [1.1.0] - 2026-08-18

### ✨ 新增
- 前端白天/夜晚主题切换（localStorage 记忆，默认深色）
- 用户管理：admin/user 双角色，可创建用户，管理员不可删除，普通用户无权管理
- 接收地址仅邮件渠道展示/必填；Webhook 渠道无接收地址也能发送
- 模板变量：任务级变量（优先级：请求 > 任务 > 模板默认值），任务表单可视化编辑
- 发送日志展示消息标题与内容（task_logs 新增 subject/content 列）
- 任务表单：非邮件渠道时提示"接收地址不生效"
- 支持 `.env` 配置文件（环境变量优先）
- Makefile：生命周期管理（build/run/dev/test/docker 等）
- 邮箱渠道支持 465 隐式 TLS（SMTPS）
- PushPlus 渠道支持 markdown 模板

### 🐛 修复
- PushPlus 连接测试模板值 text→txt（code=600）
- ENCRYPT_KEY 未设置时持久化到本地文件，重启不再导致渠道配置解密失败
- 登录用户名/密码自动去空格
- 日志 sent_at 零值导致时间/仪表盘错误
- 测试改用独立测试库 notice_service_test，不再影响真实数据
- 邮箱渠道 STARTTLS/超时、Webhook 渠道错误检测等

### ✅ 真实渠道联调
- 126 邮箱、PushPlus、钉钉、飞书、企业微信 5 渠道全部真实发送验证通过

## [1.2.0] - 2026-08-18

### ✨ 新增
- 逻辑删除（软删）：users/channels/templates/tasks 支持回滚，列表自动过滤已删数据
- 用户管理完整增删改查：改角色 / 重置密码；管理员可降级但至少保留一个，管理员不可删除
- 批量删除：用户/渠道/模板/任务四页支持复选框多选 + 批量软删
- 权限模型：所有写操作仅 admin；普通用户只读（可查看全部共享配置，界面只读模式）
- PushPlus 群组发送：渠道支持可选"群组编码"（topic 参数）
- 用户/渠道/模板/任务/日志页面搜索过滤
- 侧边栏精简：个人设置/退出登录统一到右上角菜单

### 🐛 修复
- 管理员误提升后无法降级 → 允许降级（保留至少一个管理员）
- 任务列表跨用户查询时 api_key 为 NULL 的扫描问题

[1.2.0]: https://github.com/jacknotes/notice-service/releases/tag/v1.2.0

[1.1.0]: https://github.com/jacknotes/notice-service/releases/tag/v1.1.0
