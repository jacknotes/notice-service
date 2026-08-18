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
          <h3>{{ auth.user?.username || 'operator' }}</h3>
          <span class="role-tag mono">{{ (auth.user?.role || 'admin').toUpperCase() }}</span>
        </div>
      </div>

      <el-descriptions :column="1" border class="desc">
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

      <div class="actions-line">
        <el-button type="danger" plain :icon="SwitchButton" @click="onLogout">
          退出登录
        </el-button>
      </div>
    </div>

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
            placeholder="至少 6 位"
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
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { EditPen, Key, Lock, SwitchButton } from '@element-plus/icons-vue'
import { authApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

const avatarLetter = computed<string>(
  () => (auth.user?.username || 'N').slice(0, 1).toUpperCase()
)

const pwdFormRef = ref<FormInstance>()
const pwdLoading = ref(false)
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })

const pwdRules: FormRules = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '新密码至少 6 位', trigger: 'blur' },
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

async function onLogout() {
  try {
    await ElMessageBox.confirm('确认退出当前会话？', '退出登录', {
      confirmButtonText: '退出',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  auth.logout()
  router.push('/login')
}
</script>

<style scoped>
.settings-card {
  max-width: 560px;
  padding: var(--space-6);
}

.profile-head {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  margin-bottom: var(--space-5);
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
</style>
