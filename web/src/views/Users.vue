<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">{{ t('nav.users') }}</h1>
        <p class="sub">{{ t('users.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-button
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selectedRows.length"
          @click="batchDelete"
        >
          {{ t('common.batchDelete') }}
        </el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('users.createTitle') }}</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="paged"
        style="width: 100%"
        :empty-text="t('users.emptyTable')"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column
          type="selection"
          width="48"
          align="center"
          :selectable="isSelectableRow"
        />
        <el-table-column prop="id" label="ID" width="72" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="username" :label="t('users.usernameCol')" min-width="170" sortable="custom">
          <template #default="{ row }">
            <el-tooltip :content="row.username" placement="top" :show-after="320">
              <span class="user-name">{{ row.username }}</span>
            </el-tooltip>
            <span v-if="row.id === auth.user?.id" class="self-tag mono">{{ t('users.me') }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('users.displayNameCol')" min-width="130" sortable="custom" prop="display_name">
          <template #default="{ row }">
            <span class="profile-cell">{{ row.display_name || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('users.emailCol')" min-width="180" show-overflow-tooltip sortable="custom" prop="email">
          <template #default="{ row }">
            <span class="mono profile-cell">{{ row.email || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="role" :label="t('users.roleCol')" width="110" align="center" sortable="custom">
          <template #default="{ row }">
            <el-tag :style="roleTagStyle(row.role)" effect="plain" size="small">
              {{ row.role === 'admin' ? t('appShell.roleAdmin') : t('appShell.roleUser') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="2FA" width="86" align="center" sortable="custom" prop="totp_enabled">
          <template #default="{ row }">
            <el-tag :type="row.totp_enabled ? 'success' : 'info'" effect="light" size="small">
              {{ row.totp_enabled ? t('users.totpOn') : t('users.totpOff') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.status')" width="86" align="center" sortable="custom" prop="enabled">
          <template #default="{ row }">
            <el-tag :type="row.enabled === false ? 'danger' : 'success'" effect="light" size="small">
              {{ row.enabled === false ? t('users.statusDisabled') : t('users.statusNormal') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('users.createdAtCol')" min-width="170" sortable="custom" prop="created_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="300" align="center" fixed="right">
          <template #default="{ row }">
            <el-tooltip
              :disabled="row.id !== auth.user?.id"
              :content="t('users.selfEditHint')"
            >
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  :disabled="row.id === auth.user?.id"
                  @click="openEdit(row)"
                >
                  {{ t('common.edit') }}
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip
              :disabled="!isProtectedAdmin(row)"
              :content="t('users.adminResetForbidden')"
            >
              <span>
                <el-button
                  link
                  type="warning"
                  size="small"
                  :disabled="isProtectedAdmin(row)"
                  @click="generateResetToken(row)"
                >
                  {{ t('users.resetPasswordAction') }}
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip
              :disabled="canToggleEnabled(row)"
              :content="toggleEnabledHint(row)"
            >
              <span>
                <el-button
                  link
                  :type="row.enabled === false ? 'success' : 'warning'"
                  size="small"
                  :disabled="!canToggleEnabled(row)"
                  @click="toggleEnabled(row)"
                >
                  {{ row.enabled === false ? t('users.enableAction') : t('users.disableAction') }}
                </el-button>
              </span>
            </el-tooltip>
            <el-dropdown trigger="click" @command="(cmd: string) => on2FACommand(cmd, row)">
              <el-button link type="primary" size="small">
                2FA<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="enable">{{ t('users.force2faMenuOn') }}</el-dropdown-item>
                  <el-dropdown-item command="disable" :disabled="!row.totp_enabled">
                    {{ t('users.force2faMenuOff') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-tooltip
              :disabled="canDelete(row)"
              :content="deleteHint(row)"
            >
              <span>
                <el-button
                  link
                  type="danger"
                  size="small"
                  :disabled="!canDelete(row)"
                  @click="removeUser(row)"
                >
                  {{ t('common.delete') }}
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div v-if="total > 0" class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @size-change="onPageSizeChange"
      />
    </div>

    <!-- ── 重置密码：生成一次性令牌，线下交给用户自助重置 ─────────────── -->
    <el-dialog
      v-model="resetTokenVisible"
      :title="t('users.resetTokenTitle')"
      width="480px"
      :close-on-click-modal="false"
    >
      <p class="token-hint">
        {{ t('users.resetTokenHint', { user: resetTokenUser }) }}
      </p>
      <div class="token-box">
        <code class="mono token-value">{{ resetTokenValue || '—' }}</code>
        <el-button size="small" type="primary" :icon="CopyDocument" @click="copyResetToken">
          {{ t('common.copy') }}
        </el-button>
      </div>
      <p class="token-expire mono">{{ t('users.tokenExpires', { time: resetTokenExpires }) }}</p>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="resetTokenVisible = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── Create user dialog ────────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="t('users.createTitle')"
      width="460px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item :label="t('users.usernameField')" prop="username">
          <el-input v-model="form.username" :placeholder="t('users.usernamePlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('users.displayNameField')" prop="display_name">
          <el-input v-model="form.display_name" :placeholder="t('users.displayNamePlaceholderOpt')" />
        </el-form-item>

        <el-form-item :label="t('users.emailField')" prop="email">
          <el-input v-model="form.email" :placeholder="t('users.emailPlaceholderOpt')" />
        </el-form-item>

        <el-form-item :label="t('users.passwordField')" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="t('users.passwordPlaceholder')"
          />
        </el-form-item>

        <el-form-item :label="t('users.roleCol')" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option :label="t('appShell.roleAdmin')" value="admin" />
            <el-option :label="t('appShell.roleUser')" value="user" />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveUser">{{ t('common.create') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── Edit user dialog ────────────────────────────────────────── -->
    <el-dialog
      v-model="editVisible"
      :title="t('users.editTitle')"
      width="460px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-position="top">
        <el-form-item :label="t('users.usernameField')">
          <el-input :model-value="editingUser?.username || ''" disabled />
        </el-form-item>

        <el-form-item :label="t('users.displayNameField')" prop="display_name">
          <el-input v-model="editForm.display_name" :placeholder="t('users.displayNamePlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('users.emailField')" prop="email">
          <el-input v-model="editForm.email" :placeholder="t('users.emailPlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('users.roleCol')" prop="role">
          <el-select
            v-model="editForm.role"
            style="width: 100%"
            :disabled="isProtectedAdmin(editingUser)"
          >
            <el-option :label="t('appShell.roleAdmin')" value="admin" />
            <el-option :label="t('appShell.roleUser')" value="user" />
          </el-select>
          <div v-if="isProtectedAdmin(editingUser)" class="edit-hint">{{ t('users.protectedRoleHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('users.newPasswordField')" prop="password">
          <el-input
            v-model="editForm.password"
            type="password"
            show-password
            :placeholder="t('users.newPasswordPlaceholder')"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="editVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="editSaving" @click="saveEdit">{{ t('common.save') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── 管理员强制开启 2FA：生成密钥与备用码，线下转交用户 ──────────── -->
    <el-dialog
      v-model="force2FAVisible"
      :title="t('users.force2faMenuOn')"
      width="540px"
      top="6vh"
      :close-on-click-modal="false"
    >
      <p class="token-hint">
        {{ t('users.force2faConfirmOnMsg', { name: force2FAUser }) }}
      </p>
      <div v-if="force2FAData" class="force-body">
        <div class="force-block">
          <span class="force-label">{{ t('users.scanBind') }}</span>
          <div class="qr-box">
            <img v-if="force2FAQr" :src="force2FAQr" :alt="t('users.qrAlt')" />
          </div>
        </div>
        <div class="force-block">
          <span class="force-label">{{ t('users.secretLabel') }}</span>
          <div class="secret-box">
            <code class="mono secret-value">{{ force2FAData.secret }}</code>
            <el-button size="small" :icon="CopyDocument" @click="copyText(force2FAData.secret)">
              {{ t('common.copy') }}
            </el-button>
          </div>
        </div>
        <div class="force-block">
          <span class="force-label">{{ t('users.recoveryCodesLabel') }}</span>
          <div class="codes-box">
            <code v-for="c in force2FAData.recovery_codes" :key="c" class="mono code-item">{{ c }}</code>
          </div>
          <el-button size="small" :icon="CopyDocument" @click="copyText(force2FAData.recovery_codes.join('\n'))">
            {{ t('users.copyAllRecovery') }}
          </el-button>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="force2FAVisible = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, Delete, CopyDocument, ArrowDown } from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useI18n } from 'vue-i18n'
import { userApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useTablePaging } from '@/composables/useTablePaging'

const { t } = useI18n()

interface UserRow {
  id: number
  username: string
  display_name?: string
  email?: string
  role: 'admin' | 'user'
  enabled?: boolean
  totp_enabled?: boolean
  created_at?: string
  updated_at?: string
}

const auth = useAuthStore()

const loading = ref(false)
const users = ref<UserRow[]>([])
const tableRef = ref<TableInstance>()
const selectedRows = ref<UserRow[]>([])

// 客户端排序 + 分页（整表数据在前端）
const { page, size, onSortChange, paged, total, onPageSizeChange } = useTablePaging<UserRow>(users)

function onSelectionChange(rows: UserRow[]) {
  selectedRows.value = rows
}

// 可勾选（批量删除）的行 = 当前操作者可删除的行
function isSelectableRow(row: UserRow) {
  return canDelete(row)
}

// 当前操作者是否为内置 admin（username=admin）账号
const isBuiltinAdmin = () => auth.user?.username === 'admin'

// 删除权限：不能删自己；内置 admin 账号任何人不可删；
// 管理员账号只有内置 admin 能删；普通用户任何管理员可删。
function canDelete(row: UserRow) {
  if (row.id === auth.user?.id) return false
  if (row.username === 'admin') return false
  if (row.role === 'admin') return isBuiltinAdmin()
  return true
}

// 禁用/启用权限：与删除一致（不能动自己 / 内置 admin / 普通管理员不能动管理员账号）
function canToggleEnabled(row: UserRow) {
  if (row.id === auth.user?.id) return false
  if (row.username === 'admin') return false
  if (row.role === 'admin') return isBuiltinAdmin()
  return true
}

function deleteHint(row: UserRow) {
  if (row.id === auth.user?.id) return t('users.deleteHintSelf')
  if (row.username === 'admin') return t('users.deleteHintBuiltin')
  if (row.role === 'admin' && !isBuiltinAdmin()) return t('users.deleteHintAdminOnly')
  return ''
}

function toggleEnabledHint(row: UserRow) {
  if (row.id === auth.user?.id) return t('users.toggleHintSelf')
  if (row.username === 'admin') return t('users.toggleHintBuiltin')
  if (row.role === 'admin' && !isBuiltinAdmin()) return t('users.toggleHintAdminOnly')
  return ''
}

// 内置 admin 账号（username='admin'，bootstrap 默认管理员）：
// 角色不可改、密码不可由管理员重置（恢复走离线 CLI）。
function isProtectedAdmin(row: UserRow | null) {
  return row?.username === 'admin'
}

const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{ username: string; display_name: string; email: string; password: string; role: 'admin' | 'user' }>({
  username: '',
  display_name: '',
  email: '',
  password: '',
  role: 'user',
})

// 密码强度：至少 12 位，且含大写、小写、数字、特殊字符（与后端一致）
const PASSWORD_RE = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[^A-Za-z0-9]).{12,}$/
// 邮箱格式（与后端一致；为空视为合法）
const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/

function emailRule() {
  return [
    {
      validator: (_rule: unknown, value: string, callback: (e?: Error) => void) => {
        if (value && !EMAIL_RE.test(value)) callback(new Error(t('users.emailInvalid')))
        else callback()
      },
      trigger: 'blur',
    },
  ]
}

function passwordValid(pw: string) {
  return PASSWORD_RE.test(pw)
}

// passwordRule 生成密码校验规则；required=false 时留空视为合法（编辑场景）。
function passwordRule(required: boolean) {
  return [
    ...(required ? [{ required: true, message: t('users.passwordRequired'), trigger: 'blur' }] : []),
    {
      validator: (_rule: unknown, value: string, callback: (e?: Error) => void) => {
        if (value && !passwordValid(value)) callback(new Error(t('users.passwordRule')))
        else callback()
      },
      trigger: 'blur',
    },
  ]
}

// 校验消息随语言切换（与 Login 的 computed 规则同一约定）
const rules = computed<FormRules>(() => ({
  username: [{ required: true, message: t('users.usernameRequired'), trigger: 'blur' }],
  email: emailRule(),
  password: passwordRule(true),
  role: [{ required: true, message: t('users.roleRequired'), trigger: 'change' }],
}))

/* ── Edit state ────────────────────────────────────────────────────── */
const editVisible = ref(false)
const editSaving = ref(false)
const editFormRef = ref<FormInstance>()
const editingUser = ref<UserRow | null>(null)

const editForm = reactive<{ display_name: string; email: string; role: 'admin' | 'user'; password: string }>({
  display_name: '',
  email: '',
  role: 'user',
  password: '',
})

const editRules = computed<FormRules>(() => ({
  email: emailRule(),
  role: [{ required: true, message: t('users.roleRequired'), trigger: 'change' }],
  // 密码可选：留空表示不修改；填写时需符合强度规则
  password: passwordRule(false),
}))

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

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    users.value = (await userApi.list()) || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.loadFailed')))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.username = ''
  form.display_name = ''
  form.email = ''
  form.password = ''
  form.role = 'user'
  formRef.value?.clearValidate()
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
      display_name: form.display_name.trim(),
      email: form.email.trim(),
      password: form.password,
      role: form.role,
    })
    ElMessage.success(t('users.createdOk'))
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.createFailed')))
  } finally {
    saving.value = false
  }
}

/* ── Edit ───────────────────────────────────────────────────────────── */
function openEdit(row: UserRow) {
  editingUser.value = row
  editForm.display_name = row.display_name || ''
  editForm.email = row.email || ''
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

  // 只提交发生变更的字段：内置 admin 账号角色不可改，密码留空不修改
  const d: { role?: string; password?: string; display_name?: string; email?: string } = {}
  if (!isProtectedAdmin(row) && editForm.role !== row.role) d.role = editForm.role
  if (editForm.password) d.password = editForm.password
  if (editForm.display_name.trim() !== (row.display_name || '')) d.display_name = editForm.display_name.trim()
  if (editForm.email.trim() !== (row.email || '')) d.email = editForm.email.trim()

  editSaving.value = true
  try {
    await userApi.update(row.id, d)
    ElMessage.success(t('users.updatedOk'))
    editVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.updateFailed')))
  } finally {
    editSaving.value = false
  }
}

/* ── 管理员强制 2FA ────────────────────────────────────────────────── */
const force2FAVisible = ref(false)
const force2FAUser = ref('')
const force2FAData = ref<{ secret: string; otpauth_url: string; recovery_codes: string[] } | null>(null)
const force2FAQr = ref('')

function on2FACommand(cmd: string, row: UserRow) {
  if (cmd === 'enable') forceEnable2FA(row)
  else if (cmd === 'disable') forceDisable2FA(row)
}

async function forceEnable2FA(row: UserRow) {
  try {
    await ElMessageBox.confirm(
      t('users.force2faConfirmOnMsg', { name: row.username }),
      t('users.force2faConfirmOnTitle'),
      { confirmButtonText: t('users.force2faOnBtn'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    const data = await userApi.forceEnable2FA(row.id)
    force2FAData.value = data
    force2FAUser.value = row.username
    force2FAQr.value = await QRCode.toDataURL(data.otpauth_url, { width: 180, margin: 1 })
    force2FAVisible.value = true
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.force2faOnFailed')))
  }
}

async function forceDisable2FA(row: UserRow) {
  try {
    await ElMessageBox.confirm(
      t('users.force2faConfirmOffMsg', { name: row.username }),
      t('users.force2faConfirmOffTitle'),
      { confirmButtonText: t('users.force2faOffBtn'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await userApi.forceDisable2FA(row.id)
    ElMessage.success(t('users.force2faOffOk', { name: row.username }))
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.force2faOffFailed')))
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.warning(t('common.copyFailed'))
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
    ElMessage.error(errMsg(e, t('users.resetTokenFailed')))
  }
}

async function copyResetToken() {
  if (!resetTokenValue.value) return
  try {
    await navigator.clipboard.writeText(resetTokenValue.value)
    ElMessage.success(t('users.tokenCopiedOk'))
  } catch {
    ElMessage.warning(t('common.copyFailed'))
  }
}

/* ── 禁用 / 启用 ─────────────────────────────────────────────────── */
async function toggleEnabled(row: UserRow) {
  const disabling = row.enabled !== false
  try {
    await ElMessageBox.confirm(
      disabling
        ? t('users.disableConfirmMsg', { name: row.username })
        : t('users.enableConfirmMsg', { name: row.username }),
      disabling ? t('users.disableConfirmTitle') : t('users.enableConfirmTitle'),
      { confirmButtonText: disabling ? t('users.disableAction') : t('users.enableAction'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    if (disabling) await userApi.disable(row.id)
    else await userApi.enable(row.id)
    ElMessage.success(disabling ? t('users.disabledOk') : t('users.enabledOk'))
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('users.opFailed')))
  }
}

/* ── Delete ────────────────────────────────────────────────────────── */
async function removeUser(row: UserRow) {
  try {
    await ElMessageBox.confirm(
      t('users.deleteConfirmMsg', { name: row.username }),
      t('users.deleteConfirmTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await userApi.remove(row.id)
    ElMessage.success(t('users.deletedOk'))
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.deleteFailed')))
  }
}

/* ── Batch delete ─────────────────────────────────────────────────── */
async function batchDelete() {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    await ElMessageBox.confirm(
      t('common.batchDeleteConfirmMsg', { n: rows.length }),
      t('common.batchDelete'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await userApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(t('users.batchDeletedOk', { n: rows.length }))
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  }
}

onMounted(load)
</script>

<style scoped>
.table-card {
  padding: 8px 14px 14px;
  overflow: hidden;
}

.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
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
  white-space: normal;   /* 允许换行 */
  word-break: break-all; /* 长用户名完整显示 */
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
.profile-cell {
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

/* ── 强制开启 2FA 弹窗 ─────────────────────────────────────────────── */
.force-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  margin-top: var(--space-3);
}
.force-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.force-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--text-faint);
}
.qr-box {
  align-self: flex-start;
  padding: 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: #fff;
}
.qr-box img {
  width: 180px;
  height: 180px;
  display: block;
}
.secret-box {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(11, 17, 32, 0.72);
}
.secret-value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--violet-400);
  font-size: var(--text-sm);
}
.codes-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 6px;
}
.code-item {
  padding: 6px 10px;
  border: 1px dashed var(--border);
  border-radius: var(--radius-xs);
  background: rgba(148, 163, 184, 0.06);
  color: var(--sky-400);
  font-size: var(--text-xs);
  letter-spacing: 0.06em;
  text-align: center;
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
