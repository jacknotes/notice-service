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
        <span class="hint">修改密码功能将在后续版本开放</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

const avatarLetter = computed<string>(
  () => (auth.user?.username || 'N').slice(0, 1).toUpperCase()
)

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
</style>
