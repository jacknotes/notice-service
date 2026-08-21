<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">个人设置</h1>
        <p class="sub">查看当前账号信息与会话</p>
      </div>
    </div>

    <div class="card settings-card">
      <div class="profile-head">
        <span class="avatar mono">{{ avatarLetter }}</span>
        <div>
          <h3>{{ displayName || 'operator' }}</h3>
          <span class="role-tag mono">{{ (auth.user?.role || 'admin').toUpperCase() }}</span>
        </div>
      </div>

      <el-descriptions :column="1" border class="desc" align="center" label-align="center">
        <el-descriptions-item label="显示名">
          <span class="mono desc-value">{{ auth.user?.display_name?.trim() || '—' }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="邮箱">
          <span class="mono desc-value">{{ auth.user?.email?.trim() || '—' }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="用户名">
          <span class="mono desc-value">{{ auth.user?.username || '—' }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="角色">
          <span class="mono desc-value">{{ auth.user?.role || '—' }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="用户 ID">
          <span class="mono desc-value">#{{ auth.user?.id ?? '—' }}</span>
        </el-descriptions-item>
      </el-descriptions>
    </div>

    <div class="card settings-card twofa-card">
      <div class="pwd-head">
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
    </div>

    <!-- ── 开启 2FA 向导：扫码 → 保存备用码 → 验证启用 ──────────────── -->
    <el-dialog
      v-model="setupVisible"
      title="开启双因子认证"
      width="540px"
      top="6vh"
      :close-on-click-modal="false"
    >
      <div v-if="setupData" class="setup-body">
        <div class="setup-step">
          <span class="setup-num">1</span>
          <div class="setup-content">
            <p class="setup-title">用认证器 App 扫描二维码（或手动输入密钥）</p>
            <div class="qr-box">
              <img v-if="qrDataUrl" :src="qrDataUrl" alt="双因子认证二维码" />
            </div>
            <div class="secret-box">
              <code class="mono secret-value">{{ setupData.secret }}</code>
              <el-button size="small" :icon="CopyDocument" @click="copyText(setupData.secret)">
                复制密钥
              </el-button>
            </div>
          </div>
        </div>

        <div class="setup-step">
          <span class="setup-num">2</span>
          <div class="setup-content">
            <p class="setup-title">保存好以下一次性备用码（仅本次显示，手机丢失时用于登录）</p>
            <div class="codes-box">
              <code v-for="c in setupData.recovery_codes" :key="c" class="mono code-item">{{ c }}</code>
            </div>
            <el-button size="small" :icon="CopyDocument" @click="copyText(setupData.recovery_codes.join('\n'))">
              复制全部备用码
            </el-button>
          </div>
        </div>

        <div class="setup-step">
          <span class="setup-num">3</span>
          <div class="setup-content">
            <p class="setup-title">输入认证器中的 6 位动态码以启用</p>
            <div class="verify-row">
              <el-input
                v-model="setupCode"
                placeholder="6 位动态码"
                class="mono code-input"
                maxlength="8"
                @keyup.enter="enable2FA"
              />
              <el-button type="primary" :loading="enabling" @click="enable2FA">启用</el-button>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="setupVisible = false">暂不开启</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── 关闭 2FA：需校验动态码/备用码 ─────────────────────────────── -->
    <el-dialog
      v-model="disableVisible"
      title="关闭双因子认证"
      width="420px"
      :close-on-click-modal="false"
    >
      <p class="disable-hint">请输入当前认证器动态码（或一次性备用码）以确认关闭。</p>
      <el-input
        v-model="disableCode"
        placeholder="6 位动态码或备用码"
        class="mono code-input"
        maxlength="16"
        @keyup.enter="disable2FA"
      />
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="disableVisible = false">取消</el-button>
          <el-button type="danger" :loading="disabling" @click="disable2FA">确认关闭</el-button>
        </div>
      </template>
    </el-dialog>

    <div class="card settings-card password-card">
      <div class="pwd-head">
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { EditPen, Key, Lock, CopyDocument } from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { authApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

// 显示名：优先 display_name，未设置时回退到用户名。
const displayName = computed<string>(
  () => auth.user?.display_name?.trim() || auth.user?.username || ''
)
const avatarLetter = computed<string>(
  () => (displayName.value || 'N').slice(0, 1).toUpperCase()
)

/* ── 双因子认证（TOTP） ────────────────────────────────────────────── */
const totpEnabled = ref(false)
const settingUp = ref(false)

const setupVisible = ref(false)
const setupData = ref<{ secret: string; otpauth_url: string; recovery_codes: string[] } | null>(null)
const qrDataUrl = ref('')
const setupCode = ref('')
const enabling = ref(false)

const disableVisible = ref(false)
const disableCode = ref('')
const disabling = ref(false)

// 从 /auth/me 拉取最新资料（显示名/邮箱/角色/2FA 状态），并同步回
// auth store 与 localStorage（localStorage 中的 user 可能过期或资料已更新）。
async function refresh2FA() {
  try {
    const me = await authApi.me()
    totpEnabled.value = !!me.totp_enabled
    if (auth.user) {
      auth.user.username = me.username
      auth.user.display_name = me.display_name
      auth.user.email = me.email
      auth.user.role = me.role
      auth.user.totp_enabled = !!me.totp_enabled
      localStorage.setItem('user', JSON.stringify(auth.user))
    }
  } catch {
    /* 忽略：会话过期由拦截器处理 */
  }
}

async function openSetup() {
  settingUp.value = true
  try {
    const data = await authApi.setup2FA()
    setupData.value = data
    setupCode.value = ''
    qrDataUrl.value = await QRCode.toDataURL(data.otpauth_url, { width: 180, margin: 1 })
    setupVisible.value = true
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '生成密钥失败')
  } finally {
    settingUp.value = false
  }
}

async function enable2FA() {
  if (!setupCode.value.trim()) {
    ElMessage.warning('请输入 6 位动态码')
    return
  }
  enabling.value = true
  try {
    await authApi.enable2FA(setupCode.value.trim())
    ElMessage.success('双因子认证已开启')
    setupVisible.value = false
    setupData.value = null
    qrDataUrl.value = ''
    await refresh2FA()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '启用失败，请检查验证码')
  } finally {
    enabling.value = false
  }
}

async function disable2FA() {
  if (!disableCode.value.trim()) {
    ElMessage.warning('请输入动态码或备用码')
    return
  }
  disabling.value = true
  try {
    await authApi.disable2FA(disableCode.value.trim())
    ElMessage.success('双因子认证已关闭')
    disableVisible.value = false
    disableCode.value = ''
    await refresh2FA()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '关闭失败，请检查验证码')
  } finally {
    disabling.value = false
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选择复制')
  }
}

/* ── 修改密码 ──────────────────────────────────────────────────────── */
const pwdFormRef = ref<FormInstance>()
const pwdLoading = ref(false)
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })

// 密码强度：至少 12 位，且含大写、小写、数字、特殊字符（与后端一致）
const PASSWORD_RE = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[^A-Za-z0-9]).{12,}$/

const pwdRules: FormRules = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value && !PASSWORD_RE.test(value))
          callback(new Error('密码至少 12 位，且需包含大小写字母、数字、特殊字符'))
        else callback()
      },
      trigger: 'blur',
    },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value !== pwdForm.newPassword) callback(new Error('两次输入的密码不一致'))
        else callback()
      },
      trigger: 'blur',
    },
  ],
}

async function onChangePassword() {
  if (pwdLoading.value) return
  const valid = await pwdFormRef.value?.validate().catch(() => false)
  if (!valid) return

  pwdLoading.value = true
  try {
    await authApi.changePassword(pwdForm.oldPassword, pwdForm.newPassword)
    ElMessage.success('密码已修改，请重新登录')
    auth.logout()
    router.push('/login')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '修改失败，请检查原密码')
  } finally {
    pwdLoading.value = false
  }
}

onMounted(refresh2FA)
</script>

<style scoped>
.settings-card {
  max-width: 560px;
  margin-inline: auto; /* 卡片整体水平居中 */
  padding: var(--space-6);
}

.profile-head {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
  text-align: center;
}
.avatar {
  display: grid;
  place-items: center;
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background-image: var(--grad-primary);
  color: var(--text-on-grad);
  font-size: var(--text-xl);
  font-weight: 700;
  box-shadow: var(--shadow-glow);
}
.profile-head h3 {
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
}
.role-tag {
  display: inline-block;
  margin-top: 4px;
  color: var(--indigo-400);
  font-size: 11px;
  letter-spacing: 0.14em;
}

.desc-value {
  color: var(--text-primary);
  font-size: var(--text-sm);
}

.actions-line {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-4);
  margin-top: var(--space-6);
}
.hint {
  color: var(--text-faint);
  font-size: var(--text-xs);
}

.password-card {
  margin-top: var(--space-6);
}
.pwd-head {
  margin-bottom: var(--space-5);
}
.pwd-head h3 {
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
}
.pwd-sub {
  margin-top: 3px;
  display: block;
  color: var(--text-faint);
  font-size: 10px;
  letter-spacing: 0.22em;
}

/* ── 双因子认证卡片 ────────────────────────────────────────────────── */
.twofa-card {
  margin-top: var(--space-6);
}
.twofa-desc {
  margin: 0 0 var(--space-4);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  line-height: 1.8;
}
.twofa-status {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}
.twofa-tip {
  margin-top: var(--space-4);
  color: var(--emerald-400);
  font-size: var(--text-xs);
}

.setup-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.setup-step {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
}
.setup-num {
  flex: 0 0 24px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--grad-primary);
  color: var(--text-on-grad);
  font-size: var(--text-xs);
  font-weight: 700;
}
.setup-content {
  flex: 1;
  min-width: 0;
}
.setup-title {
  margin: 0 0 10px;
  color: var(--text-secondary);
  font-size: var(--text-xs);
  line-height: 1.6;
}
.qr-box {
  display: flex;
  justify-content: center;
  padding: 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: #fff;
  margin-bottom: 10px;
}
.qr-box img {
  width: 180px;
  height: 180px;
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
  margin-bottom: 10px;
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
.verify-row {
  display: flex;
  gap: var(--space-3);
}
.code-input {
  letter-spacing: 0.25em;
}
.disable-hint {
  margin: 0 0 var(--space-3);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  line-height: 1.7;
}

/* 修改密码表单输入框成列居中（label 仍在上方） */
.password-card :deep(.el-form-item) {
  max-width: 340px;
  margin-inline: auto;
}
</style>
