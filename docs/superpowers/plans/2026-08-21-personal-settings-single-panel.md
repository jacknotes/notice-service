# 个人设置单面板整合 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将个人设置页重构为单面板三段式（账户资料 / 双因子认证 / 修改密码），显示名与邮箱改为行内编辑。

**Architecture:** 仅修改 `web/src/views/Settings.vue` 一个文件。模板收敛为单一 `settings-card` 内的三段布局；显示名/邮箱用「值 + ✏️」行替换原 `el-descriptions` 与独立编辑卡片，行内编辑保存复用已存在的 `PUT /api/auth/profile`。2FA 与修改密码的交互逻辑、弹窗全部保持原样，仅移动位置。后端零改动。

**Tech Stack:** Vue 3 (`<script setup>`) + Element Plus（el-input / el-button / el-icon / el-form）+ Pinia。无前端测试框架，验证手段为 `npm run build` + 手工冒烟；后端用 `go test -p 1 ./...` 回归确认无影响。

**Spec:** `docs/superpowers/specs/2026-08-21-personal-settings-single-panel-design.md`

---

## 文件结构

- 唯一改动文件：`web/src/views/Settings.vue`
  - `<template>`：三段式单面板（含行内编辑行）
  - `<script setup>`：新增行内编辑状态/函数；删除原 `profileForm`/`profileRules`/`saveProfile`；`refresh2FA` 去掉对旧表单的同步
  - `<style scoped>`：新增 `.info-rows/.info-row/.info-label/.info-value/.info-input/.row-edit-btn/.section-divider`；删除 `.desc/.desc-value/.profile-edit-card` 等死代码

---

### Task 1: 重构模板为单面板三段式

**Files:**
- Modify: `web/src/views/Settings.vue` 模板区（第 1–250 行内：`<div class="card settings-card">` 起至第一个 `</div>` 前）

- [ ] **Step 1: 替换第一个资料卡 + 编辑资料卡 + 2FA 卡 + 密码卡为单一面板**

将现有模板中从 `<div class="card settings-card">`（第 10 行）到修改密码卡结束 `</div>`（第 248 行）之间的内容，替换为下面整个块（**保留**第 107 行之后的 2FA 开启/关闭弹窗与密码卡后的结构不变）：

```html
    <div class="card settings-card">
      <div class="profile-head">
        <span class="avatar mono">{{ avatarLetter }}</span>
        <div>
          <h3>{{ displayName || 'operator' }}</h3>
          <span class="role-tag mono">{{ (auth.user?.role || 'admin').toUpperCase() }}</span>
        </div>
      </div>

      <!-- ── 段 1：账户资料（显示名/邮箱行内编辑） ─────────────────── -->
      <div class="info-rows">
        <div class="info-row" :class="{ 'is-editing': editingField === 'display_name' }">
          <span class="info-label">显示名</span>
          <template v-if="editingField === 'display_name'">
            <el-input
              v-model="editDraft"
              class="info-input"
              size="small"
              maxlength="100"
              autofocus
              placeholder="显示名/昵称（可选）"
              @keyup.enter="saveField"
              @keyup.esc="cancelEdit"
            />
            <el-button type="primary" size="small" :loading="savingField" @click="saveField">保存</el-button>
            <el-button size="small" @click="cancelEdit">取消</el-button>
          </template>
          <template v-else>
            <span class="info-value">{{ auth.user?.display_name?.trim() || '—' }}</span>
            <button class="row-edit-btn" title="编辑显示名" @click="startEdit('display_name')">
              <el-icon :size="13"><EditPen /></el-icon>
            </button>
          </template>
        </div>

        <div class="info-row" :class="{ 'is-editing': editingField === 'email' }">
          <span class="info-label">邮箱</span>
          <template v-if="editingField === 'email'">
            <el-input
              v-model="editDraft"
              class="info-input"
              size="small"
              maxlength="190"
              autofocus
              placeholder="用于接收通知的邮箱（可选）"
              @keyup.enter="saveField"
              @keyup.esc="cancelEdit"
            />
            <el-button type="primary" size="small" :loading="savingField" @click="saveField">保存</el-button>
            <el-button size="small" @click="cancelEdit">取消</el-button>
          </template>
          <template v-else>
            <span class="info-value">{{ auth.user?.email?.trim() || '—' }}</span>
            <button class="row-edit-btn" title="编辑邮箱" @click="startEdit('email')">
              <el-icon :size="13"><EditPen /></el-icon>
            </button>
          </template>
        </div>

        <div class="info-row">
          <span class="info-label">用户名</span>
          <span class="info-value mono">{{ auth.user?.username || '—' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">角色</span>
          <span class="info-value mono">{{ auth.user?.role || '—' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">用户 ID</span>
          <span class="info-value mono">#{{ auth.user?.id ?? '—' }}</span>
        </div>
      </div>

      <div class="section-divider"></div>

      <!-- ── 段 2：双因子认证 ─────────────────────────────────────── -->
      <div class="section-head">
        <h3>双因子认证</h3>
        <span class="pwd-sub mono">TWO-FACTOR AUTH</span>
      </div>
      <p class="twofa-desc">
        双因子认证（TOTP）在密码之外额外要求认证器中的 6 位动态码，即使密码泄露也无法直接登录。
        推荐使用 Google Authenticator / Microsoft Authenticator / 1Password 扫码绑定。
      </p>
      <div class="twofa-status">
        <el-tag :type="totpEnabled ? 'success' : 'info'" effect="light" size="large">
          {{ totpEnabled ? '已开启' : '未开启' }}
        </el-tag>
        <el-button
          v-if="!totpEnabled"
          type="primary"
          :icon="Key"
          :loading="settingUp"
          @click="openSetup"
        >
          开启双因子认证
        </el-button>
        <el-button v-else type="danger" plain :icon="Key" @click="disableVisible = true">
          关闭双因子认证
        </el-button>
      </div>
      <p v-if="totpEnabled" class="twofa-tip mono">
        已启用：登录时输入密码后需再输入认证器动态码（或一次性备用码）
      </p>

      <div class="section-divider"></div>

      <!-- ── 段 3：修改密码 ───────────────────────────────────────── -->
      <div class="section-head">
        <h3>修改密码</h3>
        <span class="pwd-sub mono">ROTATE CREDENTIALS</span>
      </div>
      <el-form
        ref="pwdFormRef"
        :model="pwdForm"
        :rules="pwdRules"
        label-position="top"
        size="large"
        @submit.prevent="onChangePassword"
      >
        <el-form-item label="原密码" prop="oldPassword">
          <el-input
            v-model="pwdForm.oldPassword"
            type="password"
            placeholder="请输入当前密码"
            :prefix-icon="Lock"
            show-password
            autocomplete="current-password"
          />
        </el-form-item>
        <el-form-item label="新密码" prop="newPassword">
          <el-input
            v-model="pwdForm.newPassword"
            type="password"
            placeholder="至少 12 位，含大小写字母、数字、特殊字符"
            :prefix-icon="Key"
            show-password
            autocomplete="new-password"
          />
        </el-form-item>
        <el-form-item label="确认新密码" prop="confirmPassword">
          <el-input
            v-model="pwdForm.confirmPassword"
            type="password"
            placeholder="再次输入新密码"
            :prefix-icon="Key"
            show-password
            autocomplete="new-password"
          />
        </el-form-item>

        <div class="actions-line">
          <el-button type="primary" :loading="pwdLoading" native-type="submit" :icon="EditPen">
            {{ pwdLoading ? '提交中…' : '修改密码' }}
          </el-button>
          <span class="hint">修改成功后需使用新密码重新登录</span>
        </div>
      </el-form>
    </div>
```

- [ ] **Step 2: 校验模板结构**

打开文件确认：`<div class="card settings-card">` 仅出现一次；其内部按「资料 → 分割线 → 2FA → 分割线 → 密码」排列；2FA 开启向导弹窗（`setupVisible`）、关闭弹窗（`disableVisible`）仍在 `settings-card` 之后。

---

### Task 2: 更新脚本（行内编辑逻辑）

**Files:**
- Modify: `web/src/views/Settings.vue` `<script setup>`（第 252–463 行）

- [ ] **Step 1: 更新图标导入**

将第 257 行：
```ts
import { EditPen, Key, Lock, CopyDocument, User, Message } from '@element-plus/icons-vue'
```
改为（移除不再使用的 `User, Message`）：
```ts
import { EditPen, Key, Lock, CopyDocument } from '@element-plus/icons-vue'
```

- [ ] **Step 2: 用行内编辑逻辑替换旧表单逻辑**

将第 273–310 行（从 `/* ── 编辑资料（自助修改显示名/邮箱） ──*/` 到 `saveProfile` 函数结束）整体替换为：

```ts
/* ── 行内编辑显示名/邮箱（保存复用 PUT /auth/profile） ────────────── */
const editingField = ref<'display_name' | 'email' | null>(null)
const editDraft = ref('')
const savingField = ref(false)

const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/

function startEdit(field: 'display_name' | 'email') {
  editingField.value = field
  editDraft.value = (field === 'display_name' ? auth.user?.display_name : auth.user?.email) || ''
}

function cancelEdit() {
  editingField.value = null
  editDraft.value = ''
}

async function saveField() {
  if (!editingField.value || savingField.value) return
  const field = editingField.value
  const value = editDraft.value.trim()
  if (field === 'email' && value && !EMAIL_RE.test(value)) {
    ElMessage.error('邮箱格式不正确')
    return
  }
  savingField.value = true
  try {
    await authApi.updateProfile(
      field === 'display_name' ? value : (auth.user?.display_name || ''),
      field === 'email' ? value : (auth.user?.email || '')
    )
    ElMessage.success('资料已更新')
    cancelEdit()
    await refresh2FA() // 同步 auth store 与 localStorage，右上角头像即时生效
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '更新失败，请重试')
  } finally {
    savingField.value = false
  }
}
```

- [ ] **Step 3: `refresh2FA` 去掉对旧表单的同步**

将第 339–341 行：
```ts
      // 同步编辑表单（进入页面或保存后都取服务端最新值）
      profileForm.display_name = me.display_name || ''
      profileForm.email = me.email || ''
```
删除（`profileForm` 已不存在）。

- [ ] **Step 4: 确认脚本无残留引用**

用 grep 确认整个文件不再出现 `profileForm`、`profileRules`、`saveProfile`、`profileSaving`、`profileFormRef`：
```bash
grep -nE "profileForm|profileRules|saveProfile|profileSaving|profileFormRef" web/src/views/Settings.vue
```
预期：无输出（或仅注释提及）。

---

### Task 3: 更新样式

**Files:**
- Modify: `web/src/views/Settings.vue` `<style scoped>`（第 465 行起）

- [ ] **Step 1: 新增行内编辑与分割线样式**

在 `.settings-card { ... }` 块之后插入：

```css
/* ── 账户资料行式列表（含行内编辑） ─────────────────────────────── */
.info-rows {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.info-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 10px 14px;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border-faint);
  min-height: 42px;
}
.info-row:last-child { border-bottom: none; }
.info-label {
  flex: 0 0 76px;
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.info-value {
  flex: 1;
  min-width: 0;
  color: var(--text-primary);
  font-size: var(--text-sm);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.info-input {
  flex: 1;
  max-width: 280px;
}
.row-edit-btn {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--indigo-400);
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
}
.row-edit-btn:hover {
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
}

/* ── 段间分割线 ──────────────────────────────────────────────────── */
.section-divider {
  height: 1px;
  background: var(--border-faint);
  margin: var(--space-5) 0;
}
.section-head {
  display: flex;
  align-items: baseline;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}
.section-head h3 {
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
}
```

- [ ] **Step 2: 删除死代码样式**

删除以下样式块（原 `el-descriptions` 与独立编辑卡使用，现已移除）：
- `.desc-value { ... }`（约第 450 行附近）
- `.profile-edit-card` 相关规则（若有）
- 若原 `.profile-head h3` 等仍被使用则保留（模板头部仍在用）。

- [ ] **Step 3: 调整密码表单居中等既有规则**

确认 `.password-card :deep(.el-form-item)` 规则（原仅作用于密码卡）改为作用于段 3 表单——将选择器 `.password-card` 改为 `.settings-card`：
```css
/* 修改密码表单输入框成列居中（label 仍在上方） */
.settings-card :deep(.el-form-item) {
  max-width: 340px;
  margin-inline: auto;
}
```
（删除原 `.password-card { margin-top: var(--space-6); }` 等卡片间距规则，因为它们不再有对应卡片。）

---

### Task 4: 构建验证 + 后端回归

**Files:** 无（验证）

- [ ] **Step 1: 前端构建**

```bash
cd /home/jack/trae/notice-service/web && npm --cache /home/jack/trae/notice-service/.dev/npm-cache run build
```
预期：`✓ built in ...`（成功）。如有 TS/模板错误，回到 Task 1–3 修复。

- [ ] **Step 2: 后端回归（确认零影响）**

```bash
export GOCACHE=/home/jack/trae/notice-service/.dev/go-cache GOMODCACHE=/home/jack/trae/notice-service/.dev/gomodcache GOPATH=/tmp/dsh-gopath
cd /home/jack/trae/notice-service && go test -p 1 ./... -count=1
```
预期：全部 `ok`。

- [ ] **Step 3: 手工冒烟（本地 8080 + vite 5173）**

用浏览器打开本地开发页，验证：
1. 个人设置为单面板三段式；显示名/邮箱行有 ✏️
2. 点 ✏️ → 行内输入 + 保存/取消；保存后右上角头像与 `/auth/me` 同步
3. 非法邮箱被拒（前端提示 + 后端 400）
4. 2FA 开启/关闭弹窗正常
5. 修改密码表单正常
6. 移动端宽度下布局不错位

---

### Task 5: 提交 + 部署

- [ ] **Step 1: 提交**

```bash
cd /home/jack/trae/notice-service
git add web/src/views/Settings.vue
git commit -m "feat(ui): 个人设置整合为单面板三段式，显示名/邮箱行内编辑"
```

- [ ] **Step 2: 推送**

```bash
GIT_SSH_COMMAND='ssh -F /dev/null -o BatchMode=yes' git push origin main
```

- [ ] **Step 3: 部署 172.168.2.12**

```bash
# 使用 .dev/sshpass.py + REMOTE_SSH_PASS=homsom
# ① cd /opt/notice-service && git pull --ff-only
# ② export DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 && setsid nohup docker-compose up -d --build </dev/null > /tmp/notice-deploy.log 2>&1 &
# ③ 轮询容器 healthy；curl :8080/:8081 /api/health
# ④ docker exec 确认 dist 中 Settings 包含「编辑资料」被替换后的新结构标记（如 "info-rows"）
```

---

## Self-Review

**Spec 覆盖：** Task 1 覆盖「单面板三段式 + 行内编辑 + 用户名/角色/ID 只读无图标」；Task 2 覆盖保存逻辑与同步；Task 3 覆盖样式；Task 4 覆盖验收清单的构建/回归；Task 5 覆盖部署。2FA 完整说明文字保留（Task 1 段 2 含完整 `.twofa-desc`）。

**占位符扫描：** 无 TBD/TODO；所有代码块完整。

**类型一致性：** 脚本中 `editingField`/`editDraft`/`savingField`/`startEdit`/`cancelEdit`/`saveField` 与模板引用一一对应；`authApi.updateProfile(display_name, email)` 签名与 `web/src/api/index.ts` 现有定义一致。
