# Changelog

本项目遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。所有显著变更都会记录在本文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [Unreleased]

### 已实现
### 安全加固（一期）
- **角色即时生效**：登录 token 中的角色不再可信，每次请求从 DB 读取——管理员提权/降级在下一个请求即生效（此前被降级者旧 token 24h 内仍可调管理接口）
- **集中式限流**：登录（5 次/15 分钟）与 Webhook（每 key 60 次/分钟）限流从内存迁到 MySQL（新表 rate_limits），多实例共享计数，堵住多实例绕过
- **密钥持久化兜底**：未提供 ENCRYPT_KEY 且密钥文件缺失、但库里已有加密渠道配置时，启动直接失败并提示（不再静默生成新密钥导致历史配置不可解）；首启自动生成密钥时输出持久化告警
- **优雅退出超时**：队列排空加 15s 超时兜底 + SMTP 会话整体 30s 超时，worker 卡住时进程可退出
- **Webhook 畸形 JSON** 返回 400（空 body 仍按空变量接受）
- **静态目录可配置**：新增 STATIC_DIR，非固定工作目录启动不再前端 404
- **优化：审计详情可读性**——操作审计详情由裸 ID 改为「名称 (id=N)」，如
  `删除用户 test1 (id=2)`、`删除渠道 邮件 (id=3)`、`删除任务 每日报表 (id=5)`；
  对象被删除后仍能看出操作的是谁/什么；批量操作逐项列出名称与 ID
- **新增：用户禁用/启用**——用户管理新增「状态」列与「禁用/启用」按钮
  （`POST /api/users/:id/disable|enable`）：禁用后登录与已签发令牌立即失效，数据保留可重新启用；
  内置 admin 不可禁用，普通管理员只能操作普通用户
- **新增：删除/禁用权限分级**——内置 admin 可删除/禁用/启用其它管理员与普通用户；普通管理员只能
  删除/禁用/启用普通用户；内置 admin 账号与当前登录账号不可删除/禁用
- **新增：侧边栏收缩/展开**——桌面端侧边栏可折叠为图标栏（顶栏按钮切换，状态记忆到 localStorage，
  移动端使用底部导航不受影响）
- **修复：审计关键词支持来源 IP**——操作审计的搜索框现可按来源 IP 检索（关键词匹配 用户名/IP/详情）
- **修复：审计详情不再显示指针地址**——`user.update` 详情由 `role=0xc000… display_name=<nil>` 改为
  可读值（如 `role=admin display_name=新名字 email=a@b.com`），排查其余审计详情均无类似问题
- **修复：用户角色管理**——管理员可随时调整任意用户角色（普通用户↔管理员，含提升后再降级，
  但至少保留一个管理员）；内置 `admin` 账号（username=`admin`）角色不可改、密码不可由管理员重置
  （含「重置密码」令牌接口），恢复走离线 `reset-password` CLI
- **修复：容器部署来源 IP**——应用以 docker-compose 容器运行、由宿主机 Nginx 反代时，
  操作审计/发送日志/Webhook 的来源 IP 取真实客户端 IP（`TRUSTED_PROXIES` 默认加入
  Docker 默认网桥网段 `172.16.0.0/12`，不再记成网桥网关如 `172.18.0.1`）
- **角色显示统一**：个人设置与右上角顶栏的角色显示为「管理员 / 普通用户」，与用户管理一致
- **开源协议**：新增 `LICENSE`（MIT, Copyright (c) 2026 jacknotes），README 补充许可证章节
- **审计日志增强**：记录来源 IP 与模块分类（auth/channel/template/task/log/user），
  前端「操作审计」新增「模块」筛选与「来源 IP」列，详情更完整
- **管理员强制 2FA**：用户管理新增「2FA」下拉，可强制开启（重新生成密钥+备用码，
  弹窗展示二维码/密钥/备用码供线下转交）或强制关闭（用户丢失手机/备用码时的恢复路径）
- **用户资料**：新建/编辑用户支持显示名、邮箱（列表新增「显示名/邮箱/2FA 状态」列，
  邮箱格式校验与后端一致）
- **仪表盘日期选择框**：改为与发送日志完全一致的结构（「日期」标签 + 快捷按钮 +
  240px 可清空日期选择框），不再伸缩变形
- **多后端节点健康**：新增实例心跳表与 `GET /api/instances`（每实例 5s 上报，
  超 15s 判离线）；侧边栏「信号在线」显示健康节点数，点击弹出节点列表
  （节点 ID/地址/版本/启动时间/最后心跳/健康状态）
- **双因子认证（2FA/TOTP）**：用户可在「个人设置」开启双因子认证（扫描二维码 / 手动密钥，含 8 个一次性备用码，仅显示一次）；登录改为两步（密码 → 动态码/备用码），备用码登录自动消费；关闭需校验动态码。全部基于标准库实现，无需第三方服务
- **操作审计日志页面**：新增「操作审计」（仅管理员）菜单，可查看登录/登出、2FA 变更、用户/渠道/模板/任务的增删改、立即发送、日志重试等记录，支持关键词/操作/日期筛选 + 分页（后端下推）
- **发送日志补充触发信息**：新增「触发方式 / 触发人 / 触发 IP」列（Webhook=webhook+来源IP、定时=scheduler、立即发送=操作人+IP、重试=操作人+IP），展开详情亦可查看
- **表格列表排序 + 分页**：任务/模板/渠道/用户列表新增列头升降序排序（客户端）与分页；发送日志新增后端排序（sort_by/sort_order 下推 SQL，白名单防注入）
- **渠道/模板/任务「复制」**：列表操作列新增「复制」按钮，一键预填「新建」弹窗（名称加「（副本）」），改完即可用
- **模板预览使用当前值**：编辑模板点「使用当前值预览」改为渲染当前未保存的表单值（含新增/未保存模板），不再回退已保存值
- **任务发送预览**：任务新建/编辑弹窗新增「预览」，渲染最终标题/正文/接收地址（模板 + 变量替换，不落库、不发送）
- **仪表盘日期选择框**：宽度调整为与发送日志一致（240px）
- 路由/接口：新增 `GET /api/audit`、`GET /api/instances`、`POST /api/users/{id}/2fa-enable|2fa-disable`、`POST /api/tasks/preview`、`POST /api/auth/2fa/{setup,enable,disable}`、`POST /api/auth/2fa/verify`；`/api/auth/me` 返回完整用户信息（含 totp_enabled）；`/api/logs` 支持 sort_by/sort_order
- 以下为历史已实现项：
- 修复：渠道停用后不再参与投递——立即发送 / Webhook（API Key）触发时跳过停用渠道并落一条「已停用」失败日志，与前端「停用后该渠道不再参与投递」提示一致（此前停用渠道仍会收到消息）
- 修复：用户被禁用（删除）后，其已签发的登录令牌立即失效（此前 24h 内仍可访问后台/API）；Auth 中间件每次请求回查用户状态
- 加固：SendTask 增加任务级 enabled 校验（纵深防御，保证任何直接调用发送管线的路径都不会向停用任务投递）
- **生产加固**：优雅退出（SIGINT/SIGTERM → 停服 → 排空队列 → 关库）、HTTP 读/写/空闲超时防慢连接、/api/health 含 DB 探测（不可达返回 503，供 LB/容器健康检查摘除实例）
- **安全加固**：Webhook IP 白名单改用可信代理判定（新增 `TRUSTED_PROXIES`，默认信任环回；不再无条件信任 X-Forwarded-For/X-Real-IP）、访问日志对 `/api/webhook/<api_key>` 路径脱敏、邮件头注入防护（CR/LF 清洗 + 收件地址校验）、SMTP 强制 TLS（默认拒绝明文凭据，内网中继可 `allow_insecure=true`）、TLS 最低版本 TLS1.2、请求体大小上限、全局安全响应头（CSP/X-Frame-Options/nosniff/Referrer-Policy）
- **运维**：审计日志自动清理（`AUDIT_RETENTION_DAYS`，默认 180 天）、DB 连接 DSN 超时 + ConnMaxLifetime、Dockerfile 增加健康检查、docker-compose 关键密钥缺失即报错（不再弱默认裸跑）+ 容器健康检查 + `stop_grace_period` + MySQL 仅绑定 127.0.0.1、可关闭 Swagger（`SWAGGER_ENABLED`）
- 移除顶层冗余 `migrations/`（迁移以内嵌 `internal/database/migrations` 为准）
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
- 构建：Dockerfile 支持 GOPROXY / npm 镜像 + BuildKit 缓存，构建不再卡住
- 配置：支持 config.yml（优先级 环境变量 > 配置文件 > 默认），`--config` 参数，附 config.example.yml
- 文档：新增 Swagger 页面（swaggo 注解自动生成，`/swagger/index.html`）
- 日志：失败记录支持「重试」——定向重发该条（同渠道+接收人，用已渲染内容），异步单次尝试，历史保留
- 仪表盘：数据丰富版——日期筛选（默认近 7 天）+ 6 统计卡 + 状态环形图 + 趋势含失败 + TOP 任务 + 渠道分布
- 修复：仪表盘日期快捷按钮选中态文字可读（plain-primary 按钮去掉深色渐变）
- 修复：仪表盘状态分布图中心不再显示 `[object Object]`，改为真实成功率
- 新增：右上角头像下拉「API 文档」入口（打开 Swagger 页）
- 修复：Vite dev 代理 `/swagger`，:5173 下也能打开 API 文档
- 构建：`make build` 依赖 `swagger` 并静态编译；新增 `make dev-backend`/`prod-backend`；Gin 模式支持 `GIN_MODE` 环境变量
- 文档：README 补充 Makefile 使用说明
- 修复：Swagger 去掉写死的 host/basePath，Try-it-out 随当前域名自适应（解决跨域与双 `/api`）
- 调整：个人设置页移除「退出登录」，仅保留右上角退出
- 新增：左上角「信号在线」改为真实状态——每 10s 轮询 `/api/health`，离线显示红色「信号离线」

### 计划中
- 用户自助注册（当前仅管理员创建）
- 邮箱验证（注册/改密邮件验证）
- 发送日志独立详情页（当前为表格行内展开）

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
