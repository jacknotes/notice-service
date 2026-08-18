# Notice Service 设计文档

> 日期：2026-07-17
> 状态：已批准
> 修订：2026-08-18（v2）— 支持分布式高可用、Webhook IP 白名单、前端专业视觉设计规格。详见文末「变更记录」。

## 1. 项目概述

Notice Service 是一个通知发送服务，用于给指定地址发送通知，帮助人员不会忘记事情。支持邮箱、企业微信、钉钉、飞书、个人微信（通过 PushPlus 桥接）等多种通知渠道，提供 PC/移动端 Web 管理界面，通过 Docker 部署在自有云服务器上。

### 1.1 核心需求

- 支持多种通知渠道：邮箱（SMTP）、企业微信、钉钉、飞书、个人微信（PushPlus）
- PC/移动端响应式 Web 管理界面，深色主题，专业视觉设计（非模板化）
- 支持 Cron 定时触发和 API/Webhook 外部触发
- Docker 部署，资源占用低（单实例 ~30-50MB 内存）
- 支持分布式多实例部署：高可用 + 故障转移，Cron 任务不重复执行
- 中型团队使用（几十到上百人）

### 1.2 技术选型

| 组件 | 技术方案 |
|------|---------|
| 后端语言 | Go |
| Web 框架 | Gin |
| 前端框架 | Vue3 + Element Plus |
| 数据库 | MySQL 5.7 |
| Cron 调度 | robfig/cron |
| 容器化 | Docker + docker-compose |
| 前端构建 | Vite |

## 2. 系统架构

### 2.1 整体结构

采用单体应用架构，前端编译后打包进 Go 镜像，Go 同时服务 API 和静态文件。支持**多实例高可用部署**：宿主机 Nginx 做负载均衡，2+ 个 notice-service 实例组成集群，共享同一 MySQL。

```
宿主机 Nginx (upstream 多实例 + /api/health 健康检查)
├── notice-service 实例 1 (端口 8080)
│   ├── Go 后端 (Gin)
│   │   ├── 认证模块（JWT 无状态）
│   │   ├── 通知管理（CRUD）
│   │   ├── 调度引擎（Cron 定时任务 + MySQL 租约锁防重复）
│   │   ├── 发送引擎（多渠道适配器）
│   │   └── API 网关（外部 Webhook 触发 + IP 白名单）
│   └── Vue3 前端（编译后的静态文件，Go http.FileServer 托管）
├── notice-service 实例 2 (端口 8081)
│   └── ...（同上，共享配置）
└── ...

外部依赖：
└── MySQL 5.7（独立 Docker 容器，实例间共享）
```

宿主机 Nginx 配置（多实例负载均衡 + 健康检查）：

```nginx
upstream notice {
    server 127.0.0.1:8080;
    server 127.0.0.1:8081;
}
server {
    listen 80;
    server_name notice.example.com;
    location / {
        proxy_pass http://notice;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

> Nginx 也可用 `check` 模块对 `/api/health` 做主动健康检查，失败的实例自动摘除，实现故障转移。

### 2.2 设计决策

- 前端打包进 Go 镜像：容器自包含，部署简单，前后端版本一致
- Go 内置 Cron 调度：无需 Redis 或外部调度组件，降低资源占用
- 通知渠道适配器模式：统一接口，便于扩展新渠道
- 用户已有 Nginx：Docker 容器不需要内置 Nginx，减少冗余
- 分布式 Cron 用 MySQL 租约锁：不引入新组件，满足低资源占用，同时支持多实例高可用
- JWT 无状态认证：多实例无需共享会话/粘性会话，天然支持负载均衡

## 3. 通知渠道设计

### 3.1 适配器接口

所有通知渠道实现统一接口：

```go
type Channel interface {
    // Send 发送通知消息
    Send(message *Message, receiver *Receiver) error
    // Type 返回渠道类型标识
    Type() string
    // ValidateConfig 验证渠道配置是否有效
    ValidateConfig(config map[string]string) error
    // TestConnection 测试渠道连接
    TestConnection(config map[string]string) error
}
```

### 3.2 内置渠道

| 渠道 | 类型标识 | 接入方式 | 配置项 |
|------|---------|---------|--------|
| 邮箱 | `email` | SMTP | host, port, username, password, from |
| 企业微信 | `wecom` | Webhook 机器人 / 应用消息 | webhook_url 或 (corp_id, agent_id, secret) |
| 钉钉 | `dingtalk` | Webhook 机器人 | webhook_url, secret（可选签名） |
| 飞书 | `feishu` | Webhook 机器人 | webhook_url |
| 个人微信 | `wechat` | PushPlus API | pushplus_token |

### 3.3 消息格式

通知内容支持 Markdown 格式。邮箱渠道将 Markdown 渲染为 HTML，即时通讯渠道自动降级为纯文本。

消息结构：

```go
type Message struct {
    Subject string            // 标题（邮箱使用）
    Content string            // Markdown 内容
    Extra   map[string]string // 渠道特定扩展字段
}

type Receiver struct {
    Address string // 接收地址（邮箱地址、用户ID等）
}
```

## 4. 数据模型

### 4.1 ER 关系

```
users 1──n channels
users 1──n templates
users 1──n tasks
channels 1──n tasks
templates 1──n tasks
tasks 1──n task_logs
```

### 4.2 表结构

**users - 用户表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO_INCREMENT | 主键 |
| username | VARCHAR(50) UNIQUE | 用户名 |
| password_hash | VARCHAR(255) | bcrypt 加密密码 |
| role | VARCHAR(20) | 角色：admin / user |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**channels - 通知渠道配置表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO_INCREMENT | 主键 |
| user_id | BIGINT FK | 所属用户 |
| type | VARCHAR(20) | 渠道类型：email/wecom/dingtalk/feishu/wechat |
| name | VARCHAR(100) | 渠道名称（用户自定义） |
| config_json | TEXT | AES 加密的渠道配置 JSON |
| enabled | TINYINT(1) | 是否启用 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**templates - 通知模板表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO_INCREMENT | 主键 |
| user_id | BIGINT FK | 所属用户 |
| name | VARCHAR(100) | 模板名称 |
| subject | VARCHAR(200) | 消息标题 |
| content_md | TEXT | Markdown 内容，支持变量占位符 |
| variables | JSON | 变量定义（名称、类型、描述、默认值） |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**tasks - 通知任务表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO_INCREMENT | 主键 |
| user_id | BIGINT FK | 所属用户 |
| name | VARCHAR(100) | 任务名称 |
| channel_id | BIGINT FK | 关联渠道 |
| template_id | BIGINT FK | 关联模板 |
| trigger_type | VARCHAR(10) | 触发方式：cron / api |
| receivers | JSON | 接收地址列表（如 `["a@x.com","{{to}}"]`），支持 `{{变量}}` 占位符 |
| cron_expr | VARCHAR(100) | Cron 表达式（trigger_type=cron 时使用） |
| api_key | VARCHAR(64) UNIQUE | API 触发密钥（trigger_type=api 时自动生成） |
| allowed_ips | VARCHAR(500) | IP 白名单，逗号分隔（支持 CIDR），空=不限制（可选） |
| locked_by | VARCHAR(64) | 分布式锁：当前持锁实例 UUID（cron 用） |
| locked_at | DATETIME | 分布式锁：持锁时间，租约 60s 过期自动接管 |
| enabled | TINYINT(1) | 是否启用 |
| last_run_at | DATETIME | 最后执行时间 |
| next_run_at | DATETIME | 下次执行时间 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**task_logs - 发送记录表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO_INCREMENT | 主键 |
| task_id | BIGINT FK | 关联任务 |
| channel_id | BIGINT FK | 关联渠道 |
| status | VARCHAR(20) | 状态：success / failed / retrying |
| request | TEXT | 发送请求内容 |
| response | TEXT | 渠道返回的响应 |
| error_msg | TEXT | 错误信息（失败时） |
| retry_count | INT | 已重试次数 |
| sent_at | DATETIME | 发送时间 |

## 5. API 设计

### 5.1 认证

```
POST   /api/auth/login          - 登录（返回 JWT Token）
POST   /api/auth/logout         - 登出
```

### 5.2 渠道管理

```
GET    /api/channels             - 获取渠道列表
POST   /api/channels             - 创建渠道
PUT    /api/channels/:id         - 更新渠道
DELETE /api/channels/:id         - 删除渠道
POST   /api/channels/:id/test    - 测试渠道连接
```

### 5.3 模板管理

```
GET    /api/templates            - 获取模板列表
POST   /api/templates            - 创建模板
PUT    /api/templates/:id        - 更新模板
DELETE /api/templates/:id        - 删除模板
POST   /api/templates/:id/preview - 预览模板（传入变量值）
```

### 5.4 任务管理

```
GET    /api/tasks                - 获取任务列表
POST   /api/tasks                - 创建任务
PUT    /api/tasks/:id            - 更新任务
DELETE /api/tasks/:id            - 删除任务
POST   /api/tasks/:id/toggle     - 启用/禁用任务
GET    /api/tasks/:id/logs       - 获取任务发送日志
```

### 5.5 外部触发

```
POST   /api/webhook/:api_key     - API 触发发送（无需 JWT 认证）
```

请求体：

```json
{
  "variables": {
    "username": "张三",
    "content": "明天10点开会"
  }
}
```

**接收地址解析**：接收地址存放在任务表的 `receivers`（JSON 数组）。每个接收地址可以是字面量（如 `zhangsan@example.com`），也可以含 `{{变量}}` 占位符——发送时用变量值替换。
- API 触发：变量来自请求体的 `variables`（覆盖模板默认值）
- Cron 触发：变量来自模板的默认变量值
- 发送时按接收地址逐条调用渠道 `Send(message, receiver)`

**IP 白名单（可选安全配置）**：当任务的 `allowed_ips` 非空时，Webhook 请求来源 IP（优先取 Nginx 设置的 `X-Real-IP`/`X-Forwarded-For`，回退到连接 IP）必须命中白名单（支持精确 IP 与 CIDR），否则返回 403。留空则不做限制。

### 5.6 健康检查（多实例 HA）

```
GET    /api/health               - 健康检查（返回 200 + 状态，供 Nginx 摘除故障实例）
```

### 5.7 仪表盘

```
GET    /api/dashboard/stats      - 统计数据（今日发送量、成功率、渠道状态等）
GET    /api/dashboard/trend      - 发送趋势（最近7天/30天）
```

## 6. 前端页面设计

### 6.1 页面列表

| 页面 | 路由 | 功能 |
|------|------|------|
| 登录页 | `/login` | 账号密码登录，简洁卡片式布局 |
| 仪表盘 | `/` / `/dashboard` | 今日发送量、成功率、渠道状态、发送趋势、最近记录 |
| 渠道管理 | `/channels` | 增删改查渠道配置，连接测试 |
| 模板管理 | `/templates` | Markdown 编辑器，变量插入，实时预览 |
| 任务管理 | `/tasks` | 创建/编辑任务，选择渠道+模板，配置 Cron 或 API 触发 |
| 发送日志 | `/logs` | 按时间/任务/状态筛选，查看每次发送详情 |
| 个人设置 | `/settings` | 修改密码 |

### 6.2 设计风格

**硬性要求：不采用模板化/通用 UI 样式，实现阶段必须加载并遵循 `frontend-design` skill 的专业前端设计流程产出视觉方案。**

- **主题**：深色主题为主（深邃蓝黑背景 #0f172a 系），支持浅色切换
- **主色**：靛蓝/紫色渐变（#6366f1 → #8b5cf6），配合强调色点缀
- **质感**：柔和渐变、玻璃拟态/细腻阴影、圆角层次、精致间距节奏，避免"看起来像模板"
- **微交互**：按钮 hover 动效、卡片 hover 浮起、加载骨架屏、发送成功/失败 toast 反馈
- **数据可视化**：仪表盘趋势折线图、成功率环形图、渠道状态卡片、最近记录
- **布局**：左侧固定导航栏 + 右侧内容区；响应式，移动端自动切换为底部导航栏
- **字体**：系统字体栈（PingFang SC, Inter, -apple-system）+ 等宽字体用于 Cron 表达式 / JSON 展示
- **落地方式**：先确定设计方向 → 建立设计系统（色板/字体/间距/组件规范）→ 再逐页面实现

## 7. 错误处理与可靠性

### 7.0 分布式调度（多实例防重复执行）

多实例部署下，每个实例的 Cron 调度器都会触发同一任务。为保证同一时刻只有一个实例执行某任务，采用 **MySQL 租约锁**：

1. 任务 Cron 触发时，实例执行一条**原子 UPDATE** 抢锁：
   ```sql
   UPDATE tasks SET locked_by = '<实例UUID>', locked_at = NOW()
   WHERE id = ? AND enabled = 1
     AND (locked_by IS NULL OR locked_at < NOW() - INTERVAL 60 SECOND)
   ```
2. 影响行数 = 1 → 抢锁成功，执行发送；影响行数 = 0 → 其他实例持锁中，跳过本次。
3. 执行完成后释放锁：`UPDATE tasks SET locked_by = NULL, locked_at = NULL WHERE id = ? AND locked_by = '<实例UUID>'`。
4. 实例崩溃未释放 → 租约 60s 后过期，其他实例自动接管（**故障转移**）。
5. 每次执行后更新 `last_run_at` / `next_run_at`。

> 租约锁直接复用 `tasks` 表两列，无需独立锁表或 Redis。锁持有期间需保证单次发送流程（含重试）在租约内完成；若单次发送耗时可能超过 60s，可定时续租或调大租约。

### 7.1 发送失败重试

- 每次发送失败自动重试 3 次
- 重试间隔递增：5s → 30s → 60s
- 重试全部失败后记录错误日志，任务继续按计划执行下一次调度
- task_logs 记录每次重试的完整请求和响应

### 7.2 渠道健康检查

- 渠道配置保存时执行连接测试
- 仪表盘展示各渠道在线/离线状态
- 连续失败 3 次标记为异常状态（不自动禁用，避免误判）

### 7.3 安全设计

- 用户密码 bcrypt 加密存储
- 渠道配置中的敏感信息（SMTP密码、Token）AES 加密存储
- API 触发使用唯一 api_key 鉴权，无需登录；可选 IP 白名单增强（`allowed_ips`，空则不限制）
- JWT Token 有效期 24 小时，无状态，多实例共享 `JWT_SECRET` 即可验证；支持手动登出（前端丢弃令牌）
- 所有 API 输入做参数校验，防止注入攻击

## 8. Docker 部署方案

### 8.1 docker-compose.yml

```yaml
version: '3'
services:
  notice-service-1:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=notice
      - DB_PASSWORD=your_password
      - DB_NAME=notice_service
      - JWT_SECRET=your_jwt_secret        # 多实例必须一致
      - ENCRYPT_KEY=your_32byte_encrypt_key
    depends_on:
      - mysql
    restart: unless-stopped

  notice-service-2:
    build: .
    ports:
      - "8081:8080"
    environment:
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=notice
      - DB_PASSWORD=your_password
      - DB_NAME=notice_service
      - JWT_SECRET=your_jwt_secret
      - ENCRYPT_KEY=your_32byte_encrypt_key
    depends_on:
      - mysql
    restart: unless-stopped

  mysql:
    image: mysql:5.7
    ports:
      - "3306:3306"
    environment:
      - MYSQL_ROOT_PASSWORD=root_password
      - MYSQL_DATABASE=notice_service
      - MYSQL_USER=notice
      - MYSQL_PASSWORD=your_password
    volumes:
      - mysql_data:/var/lib/mysql
    restart: unless-stopped

volumes:
  mysql_data:
```

### 8.2 Dockerfile（多阶段构建）

```
阶段1：构建前端
- node:18-alpine 基础镜像
- npm install && npm run build
- 输出 dist/ 目录

阶段2：构建后端
- golang:1.21-alpine 基础镜像
- COPY 前端 dist 到 static/ 目录
- go build -o notice-service

阶段3：运行镜像
- alpine:3.18 基础镜像
- 仅包含二进制文件 + 静态资源
- 最终镜像约 30-50MB
```

### 8.3 部署步骤

1. `git clone` 项目到服务器
2. 修改 `docker-compose.yml` 中的密码和密钥配置
3. `docker-compose up -d` 启动服务（自动起 2 个实例 + MySQL）
4. 在宿主机 Nginx 配置 `upstream` 负载均衡 + 健康检查（见 2.1）
5. 访问页面，使用默认管理员账号登录（首次启动自动创建）

> 多实例注意：`JWT_SECRET`、`ENCRYPT_KEY` 在所有实例间必须一致，否则登录令牌和渠道配置加密无法跨实例验证/解密。单实例部署可只起 1 个服务实例。

### 8.4 资源预估

- Docker 镜像大小：~30-50MB（每实例）
- 运行内存：~30-50MB / 实例；2 实例高可用约 60-100MB
- CPU：空闲时接近 0，发送时极低

## 9. 项目结构

```
notice-service/
├── cmd/
│   └── server/
│       └── main.go              # 入口
├── internal/
│   ├── config/                  # 配置加载
│   ├── model/                   # 数据模型
│   ├── repository/              # 数据库操作
│   ├── service/                 # 业务逻辑
│   ├── channel/                 # 通知渠道适配器
│   │   ├── channel.go           # Channel 接口定义
│   │   ├── email.go             # 邮箱实现
│   │   ├── wecom.go             # 企业微信实现
│   │   ├── dingtalk.go          # 钉钉实现
│   │   ├── feishu.go            # 飞书实现
│   │   └── wechat.go            # 个人微信(PushPlus)实现
│   ├── scheduler/               # Cron 调度引擎
│   ├── handler/                 # HTTP 处理器
│   ├── middleware/               # 中间件（JWT认证等）
│   └── router/                  # 路由注册
├── web/                         # Vue3 前端项目
│   ├── src/
│   │   ├── views/               # 页面组件
│   │   ├── components/          # 公共组件
│   │   ├── api/                 # API 请求封装
│   │   ├── router/              # 前端路由
│   │   └── stores/              # 状态管理
│   ├── package.json
│   └── vite.config.ts
├── migrations/                  # 数据库迁移脚本
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

## 10. 变更记录

| 版本 | 日期 | 变更 |
|------|------|------|
| v2 | 2026-08-18 | ① 支持分布式多实例高可用（Nginx 负载均衡 + MySQL 租约锁防重复执行，tasks 表新增 locked_by/locked_at）② Webhook 可选 IP 白名单（tasks.allowed_ips）③ 新增 /api/health 健康检查 ④ 前端视觉规格强化：实现阶段遵循 frontend-design skill 专业设计 ⑤ docker-compose 改为双实例 ⑥ 明确接收地址模型：tasks 表新增 receivers（JSON 数组，支持变量占位符），消除"接收地址来源"歧义 |
