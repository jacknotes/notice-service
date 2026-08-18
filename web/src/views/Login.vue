<template>
  <div class="login-wrap">
    <div class="login-glow"></div>

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
        <el-form-item label="用户名" prop="username">
          <el-input
            v-model="form.username"
            placeholder="请输入用户名"
            :prefix-icon="User"
            autocomplete="username"
            clearable
          />
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="请输入密码"
            :prefix-icon="Lock"
            show-password
            autocomplete="current-password"
            @keyup.enter="onSubmit"
          />
        </el-form-item>

        <transition name="el-fade-in">
          <div v-if="error" class="error-box" role="alert">
            <el-icon><WarningFilled /></el-icon>
            <span>{{ error }}</span>
          </div>
        </transition>

        <el-button
          type="primary"
          class="submit-btn"
          :loading="loading"
          native-type="submit"
        >
          {{ loading ? '验证中…' : '登 录' }}
        </el-button>
      </el-form>

      <p class="foot mono">NOTICE-SERVICE / WEB CONSOLE</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { User, Lock, WarningFilled } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const error = ref('')

const form = reactive({ username: '', password: '' })

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function onSubmit() {
  error.value = ''
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await auth.login(form.username, form.password)
    router.push('/dashboard')
  } catch (e: any) {
    error.value = e?.response?.data?.error || '登录失败，请检查网络连接'
  } finally {
    loading.value = false
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

@media (max-width: 768px) {
  .login-card { padding: 34px 26px 28px; }
}
</style>
