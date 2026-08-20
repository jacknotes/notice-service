# Notice Service · 运维修复批次设计（Webhook Key / Admin 重置 / 设置页居中 / 用户名显示）

> 日期：2026-08-20
> 状态：已批准（brainstorming 会话，用户已逐节确认）
> 修订：v1

## 1. 背景与目标

### 1.1 现状问题（已核实）

| # | 问题 | 严重度 | 证据 |
|---|------|--------|------|
| 1 | 任务从「定时(cron)」改为「Webhook API」后，`api_key` 为空，Webhook 不生效 | 高 | `internal/repository/task_repo.go:32-39` UPDATE 语句**不含 `api_key` 列**；`internal/service/task_service.go:81-104` `Update()` 不生成 Key |
| 2 | 隐含隐患：webhook 处理器 `GetByAPIKey` **不校验 `trigger_type`**，cron 任务若残留 Key 仍可被 Webhook 触发 | 中 | `internal/handler/webhook_handler.go:78-82` 仅查 `api_key` 匹配且未删除 |
| 3 | 唯一 admin 忘记密码后无人能生成重置令牌（现有重置机制依赖另一管理员），无离线重置手段 | 高 | `internal/service/auth_service.go:74-93` `BootstrapAdmin` 仅在用户不存在时创建；`cmd/server/main.go` 无任何子命令 |
| 4 | 个人设置页卡片靠左（`max-width: 560px` 未居中），内容也靠左 | 低 | `web/src/views/Settings.vue:178-181` |
| 5 | 用户管理列表用户名被 `show-overflow-tooltip` 截断为省略号，悬停才显示全 | 低 | `web/src/views/Users.vue:42` |

### 1.2 目标

- **问题 1**：任务切到 Webhook 触发时自动生成唯一 `api_key` 并落库；`api→api` 编辑保留原 Key；切回 cron 清空 Key。
- **问题 3**：提供服务端离线重置命令，唯一 admin 忘记密码也能恢复；同时在 README 写清全部重置场景。
- **问题 4**：个人设置页卡片与内容整体居中。
- **问题 5**：用户管理列表用户名完整显示（自动换行，不截断）。

### 1.3 非目标（YAGNI）

- 不做「API Key 手动轮换」独立接口（当前需求下切换即生成即可）。
- 不做 webhook 处理器对 `trigger_type` 的运行时校验改动——通过「切回 cron 清空 Key」从数据源头保证一致性（见 2.1）。
- 不改密码强度策略（沿用现有 ≥12 位 + 大小写 + 数字 + 特殊字符）。

## 2. 总体方案

四个互相独立的修复，可分别合入：

### 2.1 Webhook API Key 生命周期修复（问题 1、2）

**`internal/service/task_service.go` → `Update()`**

```
读出现有任务 ex
if in.TriggerType == "api":
    if ex.APIKey 非空 → in.APIKey = ex.APIKey   // api→api 编辑保留原 Key
    else              → in.APIKey = generateAPIKey() // cron→api 切换生成新 Key
else (in.TriggerType == "cron"):
    in.APIKey = ""                                // api→cron 清空，旧 URL 立即失效
```

**`internal/repository/task_repo.go` → `Update()`**

UPDATE 语句补充 `api_key=?` 列写入，使数据库与触发方式保持一致：

```sql
UPDATE tasks SET name=?, channel_id=?, channel_ids=?, template_id=?, trigger_type=?, receivers=?, cron_expr=?, api_key=?, allowed_ips=?, variables=?, enabled=? WHERE id=? AND user_id=?
```

**行为矩阵**

| 场景 | api_key 结果 |
|------|-------------|
| cron → api | 生成新唯一 Key |
| api → api（改名/改渠道等） | 保留原 Key（URL 不断） |
| api → cron | 清空 Key（旧 URL 失效，同时封堵隐患 2） |
| Create(api) | 现有逻辑不变，创建即生成 |

**前端（`web/src/views/Tasks.vue`）**

- `saveTask` 成功后若任务为 `api` 类型，自动打开「API Key」弹窗并提示复制（创建与编辑均触发）。
- payload 不携带 `api_key`，后端全权负责。

**测试**

- `internal/service/task_service_test.go`：cron→api 生成 Key 且落库、api→api 保留 Key、api→cron 清空 Key。
- `internal/handler/webhook_test.go`：定时任务（无 Key）经 webhook 请求返回 404；含 Key 的 api 任务可正常 202。

### 2.2 Admin 离线重置 CLI（问题 3）

**`cmd/server/main.go`**：入口处增加子命令分支，`os.Args[1] == "reset-password"` 时进入重置流程并退出，**不启动 HTTP 服务**；无参数照常启动服务（完全向后兼容）。

命令形态：

```
./notice-service reset-password [--username admin] [--new-password '...']
```

流程：

1. `config.Load()` 复用配置（DSN 等），`database.Open` 连接；**不跑 Migrate**（表已存在，避免副作用）。
2. `--username` 默认取 `ADMIN_USER`（默认 `admin`）。
3. 密码来源：`--new-password` 参数 > 交互式终端提示（`golang.org/x/term` 隐藏回显，避免入 shell 历史）。
4. 复用 `internal/service/password.go` 的强度校验（≥12 位 + 大小写 + 数字 + 特殊字符），不达标直接报错退出。
5. bcrypt 哈希后 `UPDATE users SET password_hash=?, updated_at=NOW() WHERE username=?`；**用户不存在报错退出**，不静默成功。
6. 成功打印「已重置用户 <username> 的密码」，退出码 0。

**安全性**

- 需服务器 shell + 数据库访问权限（自托管合理）。
- 不改运行期配置/环境变量，不存在「每次启动都重置」风险。
- 支持任意用户名；密码强度与 Web 端一致。

**文档（`README.md`）**

新增「密码重置」章节，写清三种场景：

1. 多管理员：用户管理 → 生成一次性令牌 → 登录页「忘记密码」自助重置。
2. 唯一 admin：服务器上运行 CLI 离线重置命令。
3. CLI 用法示例与注意事项（shell 历史、交互式输入）。

**测试**

- 新增 `cmd/server/reset_password_test.go`：成功重置后旧密码失效新密码可登录、强度不足拒绝、用户不存在报错、`--new-password` 与交互提示两条路径（交互用注入 reader 模拟）。

### 2.3 个人设置页居中（问题 4）

**`web/src/views/Settings.vue`（仅模板/样式）**

- `.settings-card` 增加 `margin-inline: auto;`（卡片整体水平居中）。
- `.profile-head`：`justify-content: center; text-align: center;`（头像 + 用户名 + 角色徽标居中）。
- `.el-descriptions`：`align="center"` + `label-align="center"`（信息区内容居中）。
- `.actions-line`：`justify-content: center;`（按钮居中）。
- 修改密码表单 `.el-form-item`：`max-width` + `margin-inline: auto`（输入框成列居中，label 保持在上方）。

**视觉预期**：页面卡片整体居中，卡片内头像/信息/按钮/表单均居中；深浅主题不变。

### 2.4 用户管理用户名完整显示（问题 5）

**`web/src/views/Users.vue`（仅模板/样式）**

- 用户名列移除 `show-overflow-tooltip`。
- `min-width` 180 → 220；`.user-name` 增加 `white-space: normal; word-break: break-all;`，长用户名单元格内换行完整显示。
- 保留 `el-tooltip` 悬停兜底（双保险）；「（我）」标记 `display: inline-block` 保留对齐。

**视觉预期**：长用户名直接在页面完整可见，不截断、不省略。

## 3. 数据流

- 2.1：前端保存 → `PUT /api/tasks/:id` → `TaskService.Update()` 计算/生成 Key → `TaskRepo.Update()` 落库 → 列表刷新显示「API Key」按钮 → 点击弹窗复制。切回 cron 时 Key 清空，旧 URL 即 404。
- 2.2：运维在服务器执行 `reset-password` → 连接 DB → 校验强度 → bcrypt 更新 → 退出。与运行中服务无交互（可在线执行，亦可停机执行）。

## 4. 错误处理

- 2.1：Key 生成失败（`rand` 出错）沿用现有 `generateAPIKey` 行为（忽略错误、UUID 保底）；DB 更新失败由 `Update()` 原有错误路径返回。
- 2.2：DB 连接失败/用户不存在/强度不足 → 明确报错并非零退出；交互输入两次不一致 → 提示重输。
- 2.3/2.4：纯展示，无运行时错误路径。

## 5. 测试策略

| 层级 | 覆盖 |
|------|------|
| Go 单测（service） | Key 生命周期三场景 + 落库 |
| Go 集成测试（handler） | webhook 对「定时无 Key」返回 404、api 任务 202 |
| Go 单测（cmd） | CLI 重置成功/强度不足/用户不存在/双密码来源 |
| 前端手动验证 | 设置页居中；用户名换行完整显示；保存 api 任务后自动弹 Key |

## 6. 影响面

- 后端：`internal/service/task_service.go`、`internal/repository/task_repo.go`、`cmd/server/main.go`、`internal/service/password.go`（复用，可能需导出校验函数）。
- 前端：`web/src/views/Tasks.vue`、`web/src/views/Settings.vue`、`web/src/views/Users.vue`。
- 文档：`README.md`。
- 数据库：无 schema 变更（`api_key` 列已存在）。
