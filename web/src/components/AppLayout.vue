<template>
  <div class="layout">
    <!-- ── Desktop sidebar ─────────────────────────────────────────── -->
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark">✦</span>
        <span class="brand-name grad-text">Notice</span>
      </div>

      <div class="status-pill" :class="{ 'is-offline': signal === 'offline' }">
        <span class="dot-live" :class="{ 'is-offline': signal === 'offline' }"></span>
        <span>{{ signal === 'online' ? '信号在线' : '信号离线' }}</span>
      </div>

      <el-menu router :default-active="route.path" class="side-menu">
        <el-menu-item v-for="item in visibleNavItems" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-foot">
        <p class="ver mono">Notice Service · v1</p>
      </div>
    </aside>

    <!-- ── Main column ─────────────────────────────────────────────── -->
    <div class="main">
      <header class="topbar">
        <div class="page-title">
          <h1>{{ pageTitle }}</h1>
          <span class="topbar-path mono">/{{ route.path.replace('/', '') }}</span>
        </div>

        <div class="topbar-right">
          <el-tooltip
            :content="theme === 'dark' ? '切换到白天模式' : '切换到夜晚模式'"
            placement="bottom"
            :show-after="320"
          >
            <button
              class="theme-btn"
              :class="{ 'is-light': theme === 'light' }"
              :aria-label="theme === 'dark' ? '切换到白天模式' : '切换到夜晚模式'"
              @click="toggleTheme"
            >
              <el-icon :size="18"><component :is="theme === 'dark' ? Sunny : Moon" /></el-icon>
            </button>
          </el-tooltip>

          <el-dropdown trigger="click" @command="onCommand">
            <div class="user-chip">
              <span class="avatar mono">{{ avatarLetter }}</span>
              <span class="user-name">{{ auth.user?.username || 'operator' }}</span>
              <el-icon class="chev"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>
                  <span class="role-tag">{{ auth.user?.role || 'admin' }}</span>
                </el-dropdown-item>
                <el-dropdown-item command="swagger">
                  <el-icon><Document /></el-icon>API 文档
                </el-dropdown-item>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon>个人设置
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main class="content">
        <router-view />
      </main>
    </div>

    <!-- ── Mobile bottom nav ───────────────────────────────────────── -->
    <nav class="bottom-nav">
      <button
        v-for="item in visibleNavItems"
        :key="item.path"
        class="bn-item"
        :class="{ 'is-active': route.path === item.path }"
        @click="router.push(item.path)"
      >
        <el-icon :size="18"><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import {
  Odometer, Connection, Document, AlarmClock, MessageBox,
  Setting, SwitchButton, User, ArrowDown, Sunny, Moon, List,
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { theme, toggleTheme } from '@/composables/useTheme'
import client from '@/api/client'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

interface NavItem {
  path: string
  label: string
  icon: Component
  adminOnly?: boolean
}

const navItems: NavItem[] = [
  { path: '/dashboard', label: '仪表盘', icon: Odometer },
  { path: '/channels', label: '渠道管理', icon: Connection },
  { path: '/templates', label: '模板管理', icon: Document },
  { path: '/tasks', label: '任务管理', icon: AlarmClock },
  { path: '/logs', label: '发送日志', icon: MessageBox },
  { path: '/audit', label: '操作审计', icon: List, adminOnly: true },
  { path: '/users', label: '用户管理', icon: User, adminOnly: true },
]

// 用户管理仅对 admin 可见
const visibleNavItems = computed(() =>
  navItems.filter((item) => !item.adminOnly || auth.user?.role === 'admin')
)

const pageTitle = computed<string>(
  () => (route.meta.title as string) || '信号中枢'
)
const avatarLetter = computed<string>(
  () => (auth.user?.username || 'N').slice(0, 1).toUpperCase()
)

/* ── 信号在线状态：轮询 /api/health，真实反映后端在线与否 ───────────── */
const signal = ref<'online' | 'offline'>('online')
const SIGNAL_POLL_MS = 10000

async function checkSignal() {
  try {
    await client.get('/health', { timeout: 5000 })
    signal.value = 'online'
  } catch {
    signal.value = 'offline'
  }
}

let signalTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  checkSignal()
  signalTimer = setInterval(checkSignal, SIGNAL_POLL_MS)
})
onBeforeUnmount(() => {
  if (signalTimer) clearInterval(signalTimer)
})

function onCommand(cmd: string) {
  if (cmd === 'swagger') window.open('/swagger/index.html', '_blank')
  if (cmd === 'settings') router.push('/settings')
  if (cmd === 'logout') onLogout()
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
.layout {
  --sbw: 220px;
  min-height: 100vh;
  display: flex;
}

/* ── Sidebar ─────────────────────────────────────────────────────── */
.sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  width: var(--sbw);
  z-index: var(--z-sticky);
  display: flex;
  flex-direction: column;
  padding: 24px 14px 18px;
  background: var(--sidebar-bg);
  border-right: 1px solid var(--border-faint);
  backdrop-filter: blur(14px);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 2px 10px 22px;
}
.brand-mark {
  font-size: 20px;
  color: var(--indigo-400);
  filter: drop-shadow(0 0 10px rgba(99, 102, 241, 0.6));
}
.brand-name {
  font-family: var(--font-display);
  font-size: 21px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  align-self: flex-start;
  margin: 0 10px 18px;
  padding: 5px 12px;
  border-radius: var(--radius-pill);
  border: 1px solid rgba(52, 211, 153, 0.22);
  background: rgba(52, 211, 153, 0.08);
  color: var(--emerald-400);
  font-size: var(--text-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}
/* 离线：红色主题 */
.status-pill.is-offline {
  border-color: rgba(248, 113, 113, 0.3);
  background: rgba(248, 113, 113, 0.1);
  color: #f87171;
}

.side-menu {
  flex: 1;
  border-right: none;
}
.side-menu :deep(.el-menu-item) {
  border-radius: var(--radius-sm);
  margin: 2px 4px;
  padding-left: 14px !important;
  height: 46px;
}
.side-menu :deep(.el-menu-item .el-icon) {
  font-size: 17px;
  margin-right: 10px;
}

.sidebar-foot {
  padding: 14px 6px 0;
  border-top: 1px solid var(--border-faint);
}
.ver {
  margin-top: 12px;
  text-align: center;
  color: var(--text-faint);
  font-size: 10px;
}

/* ── Main column ─────────────────────────────────────────────────── */
.main {
  flex: 1;
  margin-left: var(--sbw);
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: var(--z-sticky);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  height: var(--topbar-h);
  padding: 0 var(--page-pad);
  background: var(--topbar-bg);
  border-bottom: 1px solid var(--border-faint);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
}
.page-title { display: flex; align-items: baseline; gap: 12px; }
.page-title h1 { font-size: var(--text-lg); font-weight: 700; }
.topbar-path {
  color: var(--text-faint);
  font-size: var(--text-xs);
  text-transform: lowercase;
}

.topbar-right { display: flex; align-items: center; gap: 12px; }
.theme-btn {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border);
  background: var(--bg-card);
  color: var(--text-secondary);
  cursor: pointer;
  transition: color var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out),
              box-shadow var(--dur-fast) var(--ease-out),
              transform var(--dur-fast) var(--ease-out);
}
.theme-btn:hover {
  color: var(--amber-400);
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
  transform: translateY(-1px);
}
.theme-btn.is-light { color: var(--indigo-500); }
.theme-btn.is-light:hover { color: var(--indigo-500); }
.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px 6px 6px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border);
  background: var(--bg-card);
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-out),
              box-shadow var(--dur-fast) var(--ease-out);
}
.user-chip:hover {
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
}
.avatar {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background-image: var(--grad-primary);
  color: var(--text-on-grad);
  font-size: var(--text-sm);
  font-weight: 700;
}
.user-name {
  color: var(--text-primary);
  font-size: var(--text-sm);
  font-weight: 600;
}
.chev { color: var(--text-muted); font-size: 12px; }
.role-tag {
  color: var(--indigo-400);
  font-size: var(--text-xs);
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.content { flex: 1; min-width: 0; }

/* ── Mobile bottom nav ───────────────────────────────────────────── */
.bottom-nav {
  display: none;
}

/* ── Responsive ──────────────────────────────────────────────────── */
@media (max-width: 768px) {
  .layout { flex-direction: column; }
  .sidebar { display: none; }
  .main { margin-left: 0; }
  .topbar { padding: 0 var(--space-4); }
  .bottom-nav {
    position: fixed;
    inset: auto 0 0 0;
    z-index: var(--z-sticky);
    display: flex;
    justify-content: space-around;
    padding: 6px 4px calc(6px + env(safe-area-inset-bottom));
    background: var(--bottomnav-bg);
    border-top: 1px solid var(--border);
    backdrop-filter: blur(14px);
  }
  .bn-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 3px;
    flex: 1;
    padding: 6px 0;
    border: none;
    background: transparent;
    color: var(--text-muted);
    font-size: 11px;
    cursor: pointer;
    transition: color var(--dur-fast) var(--ease-out);
  }
  .bn-item.is-active {
    color: var(--indigo-400);
  }
  .content { padding-bottom: 64px; }
}
</style>
