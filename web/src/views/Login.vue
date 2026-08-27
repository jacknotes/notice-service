<template>
  <div class="login-wrap">
    <div class="login-glow"></div>

    <div class="corner-actions">
      <el-dropdown trigger="click" @command="(cmd: any) => setLocale(cmd as SupportedLocale)">
        <button class="corner-toggle" :aria-label="t('appShell.switchLang')">
          <el-icon :size="18"><Switch /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="zh-CN">{{ t('appShell.languageZh') }}</el-dropdown-item>
            <el-dropdown-item command="en-US">{{ t('appShell.languageEn') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <button
        class="corner-toggle"
        :aria-label="theme === 'dark' ? t('common.switchToDay') : t('common.switchToNight')"
        @click="toggleTheme"
      >
        <el-icon :size="18"><component :is="theme === 'dark' ? Sunny : Moon" /></el-icon>
      </button>
    </div>

    <div class="login-card reveal">
      <div class="brand">
        <span class="brand-mark">✦</span>
        <span class="brand-name grad-text">Notice</span>
      </div>
      <p class="tagline mono">SIGNAL RELAY · CONTROL ROOM</p>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        size="large"
        @submit.prevent="onSubmit"
      >
        <el-form-item v-if="step === 'password'" :label="t('login.username')" prop="username">
          <el-input
            v-model="form.username"
            :placeholder="t('login.usernamePlaceholder')"
            :prefix-icon="User"
            autocomplete="username"
            clearable
          />
        </el-form-item>

        <el-form-item v-if="step === 'password'" :label="t('login.password')" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            :placeholder="t('login.passwordPlaceholder')"
            :prefix-icon="Lock"
            show-password
            autocomplete="current-password"
            @keyup.enter="onSubmit"
          />
        </el-form-item>

        <template v-if="step === '2fa'">
          <el-form-item :label="t('login.code')" prop="code">
            <el-input
              v-model="form.code"
              :placeholder="t('login.codePlaceholder')"
              :prefix-icon="Key"
              maxlength="16"
              class="mono code-input"
              @keyup.enter="onVerify2FA"
            />
            <div class="code-hint">
              {{ t('login.codeHint') }}
            </div>
          </el-form-item>
          <el-button link type="primary" size="small" class="back-login" @click="backToPassword">
            {{ t('login.backToPassword') }}
          </el-button>
        </template>

        <transition name="el-fade-in">
          <div v-if="error" class="error-box" role="alert">
            <el-icon><WarningFilled /></el-icon>
            <span>{{ error }}</span>
          </div>
        </transition>

        <el-button
          v-if="step === 'password'"
          type="primary"
          class="submit-btn"
          :loading="loading"
          native-type="submit"
        >
          {{ loading ? t('login.verifying') : t('login.signIn') }}
        </el-button>
        <el-button
          v-else
          type="primary"
          class="submit-btn"
          :loading="loading"
          @click="onVerify2FA"
        >
          {{ loading ? t('login.verifying') : t('login.verify') }}
        </el-button>

        <div v-if="step === 'password'" class="forgot-row">
          <el-button link type="primary" size="small" @click="openForgot">{{ t('login.forgot') }}</el-button>
        </div>
      </el-form>

      <p class="foot mono">NOTICE-SERVICE / WEB CONSOLE</p>
    </div>

    <!-- 忘记密码：用管理员生成的一次性令牌自助重置 -->
    <el-dialog v-model="forgotVisible" :title="t('login.resetPasswordTitle')" width="440px" :close-on-click-modal="false">
      <el-form ref="forgotFormRef" :model="forgotForm" :rules="forgotRules" label-position="top">
        <p class="forgot-hint">
          {{ t('login.forgotHint') }}
        </p>
        <el-form-item :label="t('login.username')" prop="username">
          <el-input v-model="forgotForm.username" :placeholder="t('login.usernamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('login.resetToken')" prop="token">
          <el-input v-model="forgotForm.token" :placeholder="t('login.resetTokenPlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('login.newPassword')" prop="newPassword">
          <el-input
            v-model="forgotForm.newPassword"
            type="password"
            show-password
            :placeholder="t('login.newPasswordPlaceholder')"
          />
        </el-form-item>
        <el-form-item :label="t('login.confirmPassword')" prop="confirm">
          <el-input v-model="forgotForm.confirm" type="password" show-password :placeholder="t('login.confirmPasswordPlaceholder')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="forgotVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="forgotLoading" @click="submitForgot">{{ t('login.resetPasswordTitle') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { User, Lock, WarningFilled, Sunny, Moon, Key, Switch } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { theme, toggleTheme } from '@/composables/useTheme'
import { setLocale, type SupportedLocale } from '@/i18n/locale'
import { authApi } from '@/api'

const router = useRouter()
const auth = useAuthStore()

const { t } = useI18n()

const formRef = ref<FormInstance>()
const loading = ref(false)
const error = ref('')

// 两步登录：password（账号密码）→ 2fa（动态验证码/备用码）
const step = ref<'password' | '2fa'>('password')
const pendingToken = ref('')

const form = reactive({ username: '', password: '', code: '' })

// 校验规则用 computed：运行时切换 locale 后 t() 重求值，规则文案即时生效（Tasks 11–18 同此范式）
const rules = computed<FormRules>(() => ({
  username: [{ required: true, message: t('login.usernamePlaceholder'), trigger: 'blur' }],
  password: [{ required: true, message: t('login.passwordPlaceholder'), trigger: 'blur' }],
  code: [{ required: true, message: t('login.codeRequired'), trigger: 'blur' }],
}))

async function onSubmit() {
  if (loading.value) return
  error.value = ''
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const res = await auth.login(form.username.trim(), form.password.trim())
    if (res.requires_2fa) {
      pendingToken.value = res.pending_token || ''
      step.value = '2fa'
      form.code = ''
      formRef.value?.clearValidate()
    } else if (res.token) {
      auth.completeLogin(res as any)
      router.push('/dashboard')
    } else {
      error.value = t('login.loginResponseError')
    }
  } catch (e: any) {
    error.value = e?.response?.data?.error || t('login.loginNetworkError')
  } finally {
    loading.value = false
  }
}

// 第二步：校验动态码/备用码，换取完整登录令牌
async function onVerify2FA() {
  if (loading.value) return
  error.value = ''
  if (!form.code.trim()) {
    error.value = t('login.codeRequired')
    return
  }
  loading.value = true
  try {
    const data = await authApi.verify2FA(pendingToken.value, form.code.trim())
    auth.completeLogin(data)
    router.push('/dashboard')
  } catch (e: any) {
    error.value = e?.response?.data?.error || t('login.codeIncorrect')
  } finally {
    loading.value = false
  }
}

function backToPassword() {
  step.value = 'password'
  pendingToken.value = ''
  form.code = ''
  error.value = ''
}

/* ── 忘记密码（方案A：一次性令牌自助重置） ───────────────────────────── */
const PASSWORD_RE = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[^A-Za-z0-9]).{12,}$/
const forgotVisible = ref(false)
const forgotLoading = ref(false)
const forgotFormRef = ref<FormInstance>()
const forgotForm = reactive({ username: '', token: '', newPassword: '', confirm: '' })
const forgotRules = computed<FormRules>(() => ({
  username: [{ required: true, message: t('login.usernamePlaceholder'), trigger: 'blur' }],
  token: [{ required: true, message: t('login.tokenRequired'), trigger: 'blur' }],
  newPassword: [
    { required: true, message: t('login.newPasswordPlaceholder'), trigger: 'blur' },
    {
      validator: (_r, v, cb) => {
        if (v && !PASSWORD_RE.test(v)) cb(new Error(t('login.passwordRule')))
        else cb()
      },
      trigger: 'blur',
    },
  ],
  confirm: [
    { required: true, message: t('login.confirmPasswordPlaceholder'), trigger: 'blur' },
    {
      validator: (_r, v, cb) => {
        if (v !== forgotForm.newPassword) cb(new Error(t('login.confirmMismatch')))
        else cb()
      },
      trigger: 'blur',
    },
  ],
}))

function openForgot() {
  forgotForm.username = ''
  forgotForm.token = ''
  forgotForm.newPassword = ''
  forgotForm.confirm = ''
  forgotFormRef.value?.clearValidate()
  forgotVisible.value = true
}

async function submitForgot() {
  const valid = await forgotFormRef.value?.validate().catch(() => false)
  if (!valid) return
  forgotLoading.value = true
  try {
    await authApi.forgotPassword(forgotForm.username.trim(), forgotForm.token.trim(), forgotForm.newPassword)
    ElMessage.success(t('login.passwordResetOk'))
    forgotVisible.value = false
    form.username = forgotForm.username.trim()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || t('login.resetFailed'))
  } finally {
    forgotLoading.value = false
  }
}
</script>

<style scoped>
.login-wrap {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: var(--space-6);
  overflow: hidden;
}

/* atmospheric glow behind the card */
.login-glow {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    radial-gradient(560px 400px at 50% 38%, rgba(99, 102, 241, 0.22), transparent 68%),
    radial-gradient(420px 320px at 82% 74%, rgba(139, 92, 246, 0.16), transparent 68%),
    radial-gradient(320px 240px at 12% 82%, rgba(52, 211, 153, 0.07), transparent 64%);
  filter: blur(2px);
}

/* top-right corner actions（语言切换 + 日夜切换）— pinned together */
.corner-actions {
  position: absolute;
  top: var(--space-5);
  right: var(--space-5);
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
}
.corner-toggle {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border);
  background: var(--bg-card);
  color: var(--text-secondary);
  cursor: pointer;
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  transition: color var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out),
              box-shadow var(--dur-fast) var(--ease-out),
              transform var(--dur-fast) var(--ease-out);
}
.corner-toggle:hover {
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
  transform: translateY(-1px);
}
/* DOM：语言按钮被 el-dropdown 包裹，日夜按钮是容器直接子级 —— 用结构区分 hover 色 */
.el-dropdown .corner-toggle:hover {
  color: var(--indigo-400); /* 语言切换 */
}
.corner-actions > .corner-toggle:hover {
  color: var(--amber-400); /* 日/夜切换，与原 theme-toggle 一致 */
}

/* soften the glow in daylight mode */
[data-theme='light'] .login-glow {
  opacity: 0.5;
  filter: blur(3px);
}

.login-card {
  position: relative;
  width: min(400px, 100%);
  padding: 42px 40px 34px;
  background: var(--bg-glass);
  backdrop-filter: blur(20px) saturate(1.3);
  -webkit-backdrop-filter: blur(20px) saturate(1.3);
  border: 1px solid var(--border);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-float), var(--shadow-glow), var(--shadow-inset);
}

.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-bottom: 6px;
}
.brand-mark {
  font-size: 26px;
  color: var(--indigo-400);
  filter: drop-shadow(0 0 12px rgba(99, 102, 241, 0.7));
}
.brand-name {
  font-family: var(--font-display);
  font-size: 30px;
  font-weight: 700;
  letter-spacing: 0.05em;
}

.tagline {
  text-align: center;
  color: var(--text-faint);
  font-size: 11px;
  letter-spacing: 0.32em;
  margin-bottom: 30px;
}

.error-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 16px;
  border-radius: var(--radius-sm);
  border: 1px solid rgba(248, 113, 113, 0.32);
  background: rgba(248, 113, 113, 0.1);
  color: var(--rose-400);
  font-size: var(--text-sm);
}

.submit-btn {
  width: 100%;
  height: 46px;
  margin-top: 6px;
  font-size: var(--text-md);
  letter-spacing: 0.3em;
  text-indent: 0.3em;
}

.foot {
  margin-top: 26px;
  text-align: center;
  color: var(--text-faint);
  font-size: 10px;
  letter-spacing: 0.24em;
}

.forgot-row {
  margin-top: 12px;
  display: flex;
  justify-content: center;
}

/* ── 2FA 验证码 ────────────────────────────────────────────────────── */
.code-input {
  letter-spacing: 0.3em;
  text-align: center;
  font-size: var(--text-lg);
}
.code-hint {
  width: 100%;
  margin-top: 4px;
  color: var(--text-faint);
  font-size: 11px;
  line-height: 1.6;
}
.back-login {
  margin: -4px 0 8px;
}

.forgot-hint {
  margin: 0 0 6px;
  color: var(--text-secondary);
  font-size: var(--text-xs);
  line-height: 1.6;
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }

@media (max-width: 768px) {
  .login-card { padding: 34px 26px 28px; }
}
</style>
