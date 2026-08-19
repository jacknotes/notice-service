<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">用户管理</h1>
        <p class="sub">管理系统账号：创建普通用户与管理员，分配登录权限</p>
      </div>
      <div class="actions">
        <el-button
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selectedRows.length"
          @click="batchDelete"
        >
          批量删除
        </el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建用户</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="users"
        style="width: 100%"
        empty-text="暂无用户，点击右上角「新建用户」开始"
        @selection-change="onSelectionChange"
      >
        <el-table-column
          type="selection"
          width="48"
          align="center"
          :selectable="isSelectableRow"
        />
        <el-table-column prop="id" label="ID" width="72" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="username" label="用户名" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="user-name">{{ row.username }}</span>
            <span v-if="row.id === auth.user?.id" class="self-tag mono">（我）</span>
          </template>
        </el-table-column>

        <el-table-column prop="role" label="角色" width="140" align="center">
          <template #default="{ row }">
            <el-tag :style="roleTagStyle(row.role)" effect="plain" size="small">
              {{ row.role === 'admin' ? '管理员' : '普通用户' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="创建时间" min-width="170">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="210" align="center" fixed="right">
          <template #default="{ row }">
            <el-tooltip
              :disabled="row.id !== auth.user?.id"
              content="请用个人设置修改"
            >
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  :disabled="row.id === auth.user?.id"
                  @click="openEdit(row)"
                >
                  编辑
                </el-button>
              </span>
            </el-tooltip>
            <el-button link type="warning" size="small" @click="generateResetToken(row)">
              重置密码
            </el-button>
            <el-tooltip
              :disabled="canDelete(row)"
              :content="row.id === auth.user?.id ? '不能删除当前登录账号' : '管理员账号不可删除'"
            >
              <span>
                <el-button
                  link
                  type="danger"
                  size="small"
                  :disabled="!canDelete(row)"
                  @click="removeUser(row)"
                >
                  删除
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- ── 重置密码：生成一次性令牌，线下交给用户自助重置 ─────────────── -->
    <el-dialog
      v-model="resetTokenVisible"
      title="重置密码（一次性令牌）"
      width="480px"
      :close-on-click-modal="false"
    >
      <p class="token-hint">
        已为用户 <b class="token-user">{{ resetTokenUser }}</b> 生成一次性重置令牌。
        请线下转交给该用户，其在登录页「忘记密码」输入用户名 + 令牌 + 新密码即可自助重置。
      </p>
      <div class="token-box">
        <code class="mono token-value">{{ resetTokenValue || '—' }}</code>
        <el-button size="small" type="primary" :icon="CopyDocument" @click="copyResetToken">
          复制
        </el-button>
      </div>
      <p class="token-expire mono">令牌有效期至 {{ resetTokenExpires }}，使用一次后即失效</p>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="resetTokenVisible = false">关闭</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── Create user dialog ────────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      title="新建用户"
      width="460px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="登录用户名" />
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="至少 12 位，含大小写字母、数字、特殊字符"
          />
        </el-form-item>

        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="管理员" value="admin" />
            <el-option label="普通用户" value="user" />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveUser">创建</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── Edit user dialog ────────────────────────────────────────── -->
    <el-dialog
      v-model="editVisible"
      title="编辑用户"
      width="460px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-position="top">
        <el-form-item label="用户名">
          <el-input :model-value="editingUser?.username || ''" disabled />
        </el-form-item>

        <el-form-item label="角色" prop="role">
          <el-select
            v-model="editForm.role"
            style="width: 100%"
            :disabled="editingUser?.role === 'admin'"
          >
            <el-option label="管理员" value="admin" />
            <el-option label="普通用户" value="user" />
          </el-select>
          <div v-if="editingUser?.role === 'admin'" class="edit-hint">管理员角色不可修改</div>
        </el-form-item>

        <el-form-item label="新密码" prop="password">
          <el-input
            v-model="editForm.password"
            type="password"
            show-password
            placeholder="留空则不修改；填写需至少 12 位，含大小写字母、数字、特殊字符"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="editVisible = false">取消</el-button>
          <el-button type="primary" :loading="editSaving" @click="saveEdit">保存</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, Delete, CopyDocument } from '@element-plus/icons-vue'
import { userApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

interface UserRow {
  id: number
  username: string
  role: 'admin' | 'user'
  created_at?: string
  updated_at?: string
}

const auth = useAuthStore()

const loading = ref(false)
const users = ref<UserRow[]>([])
const tableRef = ref<TableInstance>()
const selectedRows = ref<UserRow[]>([])

function onSelectionChange(rows: UserRow[]) {
  selectedRows.value = rows
}

// 管理员账号、以及当前登录账号本身不可勾选（不能批量删除）
function isSelectableRow(row: UserRow) {
  return row.role !== 'admin' && row.id !== auth.user?.id
}

const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{ username: string; password: string; role: 'admin' | 'user' }>({
  username: '',
  password: '',
  role: 'user',
})

// 密码强度：至少 12 位，且含大写、小写、数字、特殊字符（与后端一致）
const PASSWORD_RE = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[^A-Za-z0-9]).{12,}$/
const PASSWORD_MSG = '密码至少 12 位，且需包含大小写字母、数字、特殊字符'

function passwordValid(pw: string) {
  return PASSWORD_RE.test(pw)
}

// passwordRule 生成密码校验规则；required=false 时留空视为合法（编辑场景）。
function passwordRule(required: boolean) {
  return [
    ...(required ? [{ required: true, message: '请输入密码', trigger: 'blur' }] : []),
    {
      validator: (_rule: unknown, value: string, callback: (e?: Error) => void) => {
        if (value && !passwordValid(value)) callback(new Error(PASSWORD_MSG))
        else callback()
      },
      trigger: 'blur',
    },
  ]
}

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: passwordRule(true),
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
}

/* ── Edit state ────────────────────────────────────────────────────── */
const editVisible = ref(false)
const editSaving = ref(false)
const editFormRef = ref<FormInstance>()
const editingUser = ref<UserRow | null>(null)

const editForm = reactive<{ role: 'admin' | 'user'; password: string }>({
  role: 'user',
  password: '',
})

const editRules: FormRules = {
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
  // 密码可选：留空表示不修改；填写时需符合强度规则
  password: passwordRule(false),
}

// 角色标签：admin → violet，user → blue（与 Signal Relay 色板一致）
function roleTagStyle(role: string) {
  if (role === 'admin') {
    return {
      color: 'var(--violet-400)',
      borderColor: 'rgba(139, 92, 246, 0.4)',
      backgroundColor: 'rgba(139, 92, 246, 0.14)',
    }
  }
  return {
    color: 'var(--sky-400)',
    borderColor: 'rgba(56, 189, 248, 0.4)',
    backgroundColor: 'rgba(56, 189, 248, 0.14)',
  }
}

// 管理员账号、以及当前登录账号本身不可删除
function canDelete(row: UserRow) {
  return row.role !== 'admin' && row.id !== auth.user?.id
}

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    users.value = (await userApi.list()) || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, '用户列表加载失败'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.username = ''
  form.password = ''
  form.role = 'user'
  dialogVisible.value = true
}

/* ── Create ────────────────────────────────────────────────────────── */
async function saveUser() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    await userApi.create({
      username: form.username.trim(),
      password: form.password,
      role: form.role,
    })
    ElMessage.success('用户已创建')
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '创建失败'))
  } finally {
    saving.value = false
  }
}

/* ── Edit ───────────────────────────────────────────────────────────── */
function openEdit(row: UserRow) {
  editingUser.value = row
  editForm.role = row.role
  editForm.password = ''
  editFormRef.value?.clearValidate()
  editVisible.value = true
}

async function saveEdit() {
  const valid = await editFormRef.value?.validate().catch(() => false)
  if (!valid) return

  const row = editingUser.value
  if (!row) return

  // 只提交发生变更的字段：管理员角色不可改，密码留空不修改
  const d: { role?: string; password?: string } = {}
  if (row.role !== 'admin' && editForm.role !== row.role) d.role = editForm.role
  if (editForm.password) d.password = editForm.password

  editSaving.value = true
  try {
    await userApi.update(row.id, d)
    ElMessage.success('用户已更新')
    editVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '更新失败'))
  } finally {
    editSaving.value = false
  }
}

/* ── 重置密码：生成一次性令牌 ───────────────────────────────────────── */
const resetTokenVisible = ref(false)
const resetTokenValue = ref('')
const resetTokenExpires = ref('')
const resetTokenUser = ref('')

async function generateResetToken(row: UserRow) {
  try {
    const data = await userApi.resetToken(row.id)
    resetTokenValue.value = data.token || ''
    resetTokenExpires.value = data.expires_at || ''
    resetTokenUser.value = row.username
    resetTokenVisible.value = true
  } catch (e: any) {
    ElMessage.error(errMsg(e, '生成重置令牌失败'))
  }
}

async function copyResetToken() {
  if (!resetTokenValue.value) return
  try {
    await navigator.clipboard.writeText(resetTokenValue.value)
    ElMessage.success('令牌已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选择复制')
  }
}

/* ── Delete ────────────────────────────────────────────────────────── */
async function removeUser(row: UserRow) {
  try {
    await ElMessageBox.confirm(
      `确定删除用户「${row.username}」吗？删除后该账号将无法登录。`,
      '删除用户',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await userApi.remove(row.id)
    ElMessage.success('用户已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

/* ── Batch delete ─────────────────────────────────────────────────── */
async function batchDelete() {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    await ElMessageBox.confirm(
      `确认删除选中的 ${rows.length} 项？`,
      '批量删除',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await userApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(`已删除 ${rows.length} 个用户`)
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '批量删除失败'))
  }
}

onMounted(load)
</script>

<style scoped>
.table-card {
  padding: 8px 14px 14px;
  overflow: hidden;
}

.id-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.user-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
  line-height: 1.6;
  display: inline-block;
  vertical-align: middle;
}
.self-tag {
  margin-left: 6px;
  color: var(--text-faint);
  font-size: 11px;
}
.time-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}

.edit-hint {
  margin-top: 4px;
  color: var(--text-faint);
  font-size: 11px;
}

/* ── 重置令牌弹窗 ──────────────────────────────────────────────────── */
.token-hint {
  margin: 0 0 12px;
  color: var(--text-secondary);
  font-size: var(--text-xs);
  line-height: 1.7;
}
.token-user {
  color: var(--indigo-400);
  font-weight: 600;
}
.token-box {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(11, 17, 32, 0.72);
}
.token-value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--violet-400);
  font-size: var(--text-sm);
}
.token-expire {
  margin-top: var(--space-3);
  color: var(--text-faint);
  font-size: 11px;
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }
</style>
