# Notice Service · 个人设置单面板整合设计

> 日期：2026-08-21
> 状态：已批准（brainstorming 会话，用户通过浏览器 Demo 逐版确认，最终选定方案 A）
> 修订：v1

## 1. 背景与目标

### 1.1 现状（已核实）

当前 `web/src/views/Settings.vue` 个人设置页由三张独立卡片组成：

1. **账户资料卡**：头像 + 显示名 + 角色，及 `el-descriptions` 展示 显示名 / 邮箱 / 用户名 / 角色 / 用户 ID（只读）
2. **编辑资料卡**：显示名 + 邮箱输入表单 + 保存按钮（对应 `PUT /api/auth/profile`）
3. **双因子认证卡**：状态标签 + 开启/关闭按钮 + 完整说明文字
4. **修改密码卡**：原密码 / 新密码 / 确认新密码 + 修改按钮

用户希望页面更聚焦、少卡片堆叠，将「编辑资料」「双因子认证」「修改密码」整合进**同一个面板**。

### 1.2 目标

- 个人设置页收敛为**单个面板、三段式布局**，自上而下：
  1. **账户资料**（显示名/邮箱行内编辑；用户名/角色/ID 只读）
  2. **双因子认证**（保留完整说明文字 + 状态标签 + 开启/关闭按钮）
  3. **修改密码**（原密码 / 新密码 / 确认新密码 + 按钮）
- 段与段之间用分割线分隔；头部保留头像 + 显示名 + 角色标签。
- 交互逻辑不变：行内编辑保存走 `PUT /auth/profile`；2FA 开启向导仍走弹窗；密码修改走 `POST /auth/change-password`。

### 1.3 非目标（YAGNI）

- 不改任何后端接口（`PUT /auth/profile`、2FA 系列、`change-password` 均已存在并验证）。
- 不做 2FA 折叠/懒加载，三段落平铺展示。
- 不改密码强度策略。
- 不做「用户名/角色可编辑」。

## 2. 总体方案

仅改 `web/src/views/Settings.vue`（模板结构 + 样式 + 少量脚本重组），无后端改动。

### 2.1 面板结构（模板）

```
<div class="page">
  <page-head>个人设置</page-head>

  <div class="card settings-card">          ← 单一主卡片
    <!-- 头部 -->
    <div class="profile-head"> 头像 / 显示名 / 角色 </div>

    <!-- 段 1：账户资料 -->
    <div class="info-rows">                  ← 原 el-descriptions 改为行式
      显示名  值  +  ✏️ (行内编辑)
      邮箱    值  +  ✏️ (行内编辑)
      用户名  （只读）
      角色    （只读）
      用户 ID （只读）
    </div>

    <div class="section-divider"></div>

    <!-- 段 2：双因子认证 -->
    <div class="section-head">双因子认证 / TWO-FACTOR AUTH</div>
    <p class="twofa-desc">（保留完整说明文字）</p>
    <div class="twofa-status">状态标签 + 开启/关闭按钮</div>
    <p class="twofa-tip">（已启用提示）</p>

    <div class="section-divider"></div>

    <!-- 段 3：修改密码 -->
    <div class="section-head">修改密码 / ROTATE CREDENTIALS</div>
    <el-form> 原密码 / 新密码 / 确认新密码 + 按钮 </el-form>
  </div>
</div>
```

- 删除独立的「编辑资料」「双因子认证」「修改密码」三张卡片及其卡片级样式。
- 删除原「编辑资料」卡片（`profile-edit-card` 及其表单），其能力由行内编辑承接。

### 2.2 行内编辑交互（显示名 / 邮箱）

- 每条可编辑行：右侧一个 ✏️ 图标按钮（`EditPen` 图标）。
- 点击 ✏️：该行值文本替换为 `el-input`（预填当前值，`maxlength` 100/190）+「保存 / 取消」两个小按钮；✏️ 隐藏。
- 保存：调用 `authApi.updateProfile(displayName, email)`。每次只编辑一行时，**被编辑字段提交新值，另一字段取当前值一并提交**（接口要求两字段同传）；成功后 `refresh2FA()` 同步 `auth.user` 与 localStorage，`ElMessage.success`，退出编辑态；失败 `ElMessage.error` 保留编辑态。
- 取消 / Esc：恢复值显示，不请求。
- 只读行（用户名/角色/ID）无图标。

### 2.3 双因子认证区块

- 保留现有说明段落、状态标签、开启/关闭按钮与「已启用」提示（仅移动位置与统一样式）。
- 开启向导（扫码 → 备用码 → 验证）、关闭校验弹窗**全部保持现状**，不做改动。

### 2.4 修改密码区块

- 表单字段、校验规则、提交逻辑与现有实现完全一致，仅移入主面板。

## 3. 数据流

```
保存资料 → PUT /api/auth/profile {display_name?, email?}
         → 200 → refresh2FA()（GET /api/auth/me → 更新 auth.user + localStorage）
开启 2FA → POST /api/auth/2fa/setup → 弹窗向导 → POST /api/auth/2fa/enable
关闭 2FA → POST /api/auth/2fa/disable（校验动态码）
改密码   → POST /api/auth/change-password → 成功登出跳登录
```

（全部接口已存在，无新增。）

## 4. 错误处理

- 行内编辑邮箱非法：前端校验（正则 + 长度）阻止提交；后端 400 错误透传 `e.response.data.error`。
- 2FA/改密码错误：沿用现有 `ElMessage.error` 展示后端返回的 `error` 字段。
- 未登录 401：由 axios 拦截器统一跳转登录页（现状）。

## 5. 样式

- 复用现有 `settings-card` 宽度与 `pwd-head`/`pwd-sub` 区块标题样式，新增：
  - `.info-rows`：行式列表（替代 `el-descriptions`），带行内编辑态样式
  - `.section-divider`：段间分割线
  - `.row-edit-btn`（✏️ 图标按钮）与编辑态 `.row-edit-active`
- 移动端单列自适应（现有卡片宽度内即可）。

## 6. 测试

- 后端无改动，`go test -p 1 ./...` 应保持全绿（回归确认）。
- 前端 `npm run build` 通过。
- 手工联调：
  1. 显示名/邮箱行内编辑保存 → `/auth/me` 与右上角头像同步更新
  2. 非法邮箱被拒
  3. 2FA 开启/关闭、改密码流程在单面板内正常
  4. 部署到 172.168.2.12 后按同样清单冒烟。

## 7. 涉及文件

- `web/src/views/Settings.vue`（唯一改动文件）
- 可能微调 `web/src/api/index.ts`（若需要补充类型；当前 `updateProfile` 已存在，预计不改）

## 8. 变更清单（验收）

- [ ] 个人设置页为单面板三段式
- [ ] 显示名/邮箱行内编辑可保存并同步
- [ ] 用户名/角色/ID 只读无图标
- [ ] 2FA 区块（含完整说明）位于面板中段
- [ ] 修改密码表单位于面板末段
- [ ] 后端测试全绿、前端构建通过、线上冒烟通过
