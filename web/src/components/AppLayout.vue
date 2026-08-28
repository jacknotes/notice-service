<template>
  <div class="layout" :style="{ '--sbw': collapsed ? '64px' : '220px' }">
    <!-- ── Desktop sidebar ─────────────────────────────────────────── -->
    <aside class="sidebar" :class="{ 'is-collapsed': collapsed }">
      <div class="brand">
        <span class="brand-mark">✦</span>
        <span class="brand-name grad-text">Notice</span>
      </div>

      <button
        class="status-pill"
        :class="{ 'is-offline': signal === 'offline', 'is-partial': signal === 'partial' }"
        @click="nodesVisible = true"
      >
        <span class="dot-live" :class="{ 'is-offline': signal === 'offline', 'is-partial': signal === 'partial' }"></span>
        <span>{{ signalLabel }}</span>
      </button>

      <el-menu router :default-active="route.path" class="side-menu">
        <el-menu-item v-for="item in visibleNavItems" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <span v-if="!collapsed">{{ t(item.labelKey) }}</span>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-foot">
        <p class="ver mono">Notice Service<template v-if="buildVersion"> · {{ buildVersion }}</template></p>
      </div>
    </aside>

    <!-- ── Main column ─────────────────────────────────────────────── -->
    <div class="main">
      <header class="topbar">
        <div class="topbar-left">
          <button
            class="collapse-btn"
            :class="{ 'is-collapsed': collapsed }"
            :aria-label="collapsed ? t('appShell.expandSidebar') : t('appShell.collapseSidebar')"
            @click="toggleCollapsed"
          >
            <el-icon :size="17"><component :is="collapsed ? Expand : Fold" /></el-icon>
          </button>
          <div class="page-title">
            <h1>{{ pageTitle }}</h1>
            <span class="topbar-path mono">/{{ route.path.replace('/', '') }}</span>
          </div>
        </div>

        <div class="topbar-right">
          <el-tooltip
            :content="theme === 'dark' ? t('common.switchToDay') : t('common.switchToNight')"
            placement="bottom"
            :show-after="320"
          >
            <button
              class="theme-btn"
              :class="{ 'is-light': theme === 'light' }"
              :aria-label="theme === 'dark' ? t('common.switchToDay') : t('common.switchToNight')"
              @click="toggleTheme"
            >
              <el-icon :size="18"><component :is="theme === 'dark' ? Sunny : Moon" /></el-icon>
            </button>
          </el-tooltip>

          <el-dropdown trigger="click" @command="(cmd: any) => setLocale(cmd as SupportedLocale)">
            <button class="theme-btn lang-btn" :aria-label="t('appShell.switchLang')">
              <el-icon :size="18"><Switch /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="zh-CN">{{ t('appShell.languageZh') }}</el-dropdown-item>
                <el-dropdown-item command="en-US">{{ t('appShell.languageEn') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <el-dropdown trigger="click" @command="onCommand">
            <div class="user-chip">
              <span class="avatar mono">{{ avatarLetter }}</span>
              <span class="user-name">{{ displayName || 'operator' }}</span>
              <el-icon class="chev"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>
                  <span class="role-tag">{{ roleLabel }}</span>
                </el-dropdown-item>
                <el-dropdown-item command="swagger">
                  <el-icon><Document /></el-icon>{{ t('appShell.apiDocs') }}
                </el-dropdown-item>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon>{{ t('appShell.settings') }}
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                  <el-icon><SwitchButton /></el-icon>{{ t('appShell.logout') }}
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

    <!-- ── 后端节点健康弹窗（多实例信号在线） ─────────────────────── -->
    <el-dialog
      v-model="nodesVisible"
      :title="t('appShell.nodesTitle')"
      width="680px"
      top="8vh"
      :close-on-click-modal="true"
    >
      <p class="nodes-hint">{{ t('appShell.nodesHint', { sec: NODE_TIMEOUT_SEC }) }}</p>
      <div v-loading="nodesLoading" class="nodes-table">
        <el-table :data="nodes" style="width: 100%" :empty-text="t('appShell.nodesEmpty')">
          <el-table-column :label="t('appShell.nodeId')" min-width="180">
            <template #default="{ row }">
              <span class="mono node-id">{{ shortID(row.instance_id) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('appShell.nodeAddr')" min-width="150">
            <template #default="{ row }">
              <span class="mono node-addr">{{ row.host }}:{{ row.port }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('appShell.nodeVersion')" width="90">
            <template #default="{ row }">
              <span class="mono node-ver">{{ row.version || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('appShell.nodeStarted')" min-width="160">
            <template #default="{ row }">
              <span class="mono node-time">{{ fmtTime(row.started_at) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('appShell.nodeHeartbeat')" min-width="160">
            <template #default="{ row }">
              <span class="mono node-time">{{ fmtTime(row.last_seen_at) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('appShell.nodeStatus')" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="row.healthy ? 'success' : 'danger'" effect="light" size="small">
                {{ row.healthy ? t('appShell.healthy') : t('appShell.offline') }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <template #footer>
        <div class="nodes-footer">
          <span class="nodes-sum mono">{{ t('appShell.nodesSum', { healthy: healthyCount, total: nodes.length, sec: NODE_TIMEOUT_SEC }) }}</span>
          <div>
            <el-button size="small" :loading="nodesLoading" @click="loadNodes">{{ t('common.refresh') }}</el-button>
            <el-button size="small" @click="nodesVisible = false">{{ t('common.close') }}</el-button>
          </div>
        </div>
      </template>
    </el-dialog>

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
        <span>{{ t(item.labelKey) }}</span>
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
  Setting, SwitchButton, User, ArrowDown, Sunny, Moon, List, Fold, Expand, Switch,
} from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { setLocale, type SupportedLocale } from '@/i18n/locale'
import { useAuthStore } from '@/stores/auth'
import { theme, toggleTheme } from '@/composables/useTheme'
import { systemApi } from '@/api'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()

// 侧边栏收缩/展开（桌面端），状态持久化到 localStorage
const collapsed = ref<boolean>(localStorage.getItem('notice.sidebar.collapsed') === '1')
function toggleCollapsed() {
  collapsed.value = !collapsed.value
  localStorage.setItem('notice.sidebar.collapsed', collapsed.value ? '1' : '0')
}

interface NavItem {
  path: string
  labelKey: string
  icon: Component
  adminOnly?: boolean
}

const navItems: NavItem[] = [
  { path: '/dashboard', labelKey: 'nav.dashboard', icon: Odometer },
  { path: '/channels', labelKey: 'nav.channels', icon: Connection },
  { path: '/templates', labelKey: 'nav.templates', icon: Document },
  { path: '/tasks', labelKey: 'nav.tasks', icon: AlarmClock },
  { path: '/logs', labelKey: 'nav.logs', icon: MessageBox },
  { path: '/audit', labelKey: 'nav.audit', icon: List, adminOnly: true },
  { path: '/users', labelKey: 'nav.users', icon: User, adminOnly: true },
]

// 用户管理仅对 admin 可见
const visibleNavItems = computed(() =>
  navItems.filter((item) => !item.adminOnly || auth.user?.role === 'admin')
)

const pageTitle = computed<string>(
  () => t((route.meta.titleKey as string) || 'appShell.defaultTitle')
)
// 显示名：优先 display_name，未设置时回退到用户名。
const displayName = computed<string>(
  () => auth.user?.display_name?.trim() || auth.user?.username || ''
)
const avatarLetter = computed<string>(
  () => (displayName.value || 'N').slice(0, 1).toUpperCase()
)
// 角色显示文案：与「用户管理 / 个人设置」保持一致（admin → 管理员，其余 → 普通用户）。
const roleLabel = computed<string>(() => {
  const role = auth.user?.role
  if (!role) return '—'
  return role === 'admin' ? t('appShell.roleAdmin') : t('appShell.roleUser')
})

/* ── 信号在线：多后端节点健康（点击查看节点列表） ───────────────────── */
interface BackendNode {
  instance_id: string
  host: string
  port: string
  version: string
  started_at: string
  last_seen_at: string
  healthy: boolean
}

const signal = ref<'online' | 'partial' | 'offline'>('online')
const nodes = ref<BackendNode[]>([])
const healthyCount = ref(0)
const nodesLoading = ref(false)
const nodesVisible = ref(false)
const SIGNAL_POLL_MS = 10000
const NODE_TIMEOUT_SEC = 15 // 与后端 heartbeatHealthyWindow 一致

const signalLabel = computed(() => {
  if (signal.value === 'online') return t('appShell.signalOnline', { n: healthyCount.value })
  if (signal.value === 'partial') return t('appShell.signalPartial', { healthy: healthyCount.value, total: nodes.value.length })
  return t('appShell.signalOffline')
})

async function loadNodes() {
  nodesLoading.value = true
  try {
    const data = await systemApi.instances()
    nodes.value = data?.instances || []
    healthyCount.value = data?.healthy || 0
    const total = data?.total || 0
    if (healthyCount.value === 0) signal.value = 'offline'
    else if (healthyCount.value < total) signal.value = 'partial'
    else signal.value = 'online'
  } catch {
    // 后端不可达：无法确认节点，按离线处理
    signal.value = 'offline'
  } finally {
    nodesLoading.value = false
  }
}

function shortID(id: string) {
  if (!id) return '—'
  return id.length > 8 ? id.slice(0, 8) + '…' : id
}

function fmtTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

let signalTimer: ReturnType<typeof setInterval> | null = null
// 侧边栏底部版本：登录态下取当前实例构建版本（与节点弹窗/设置页同源）
const buildVersion = ref('')
onMounted(async () => {
  try {
    buildVersion.value = (await systemApi.version()).version
  } catch {
    /* 获取失败保持纯品牌文案 */
  }
})

onMounted(() => {
  loadNodes()
  signalTimer = setInterval(loadNodes, SIGNAL_POLL_MS)
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
    await ElMessageBox.confirm(t('appShell.logoutConfirmMsg'), t('appShell.logoutConfirmTitle'), {
      confirmButtonText: t('appShell.logoutOk'),
      cancelButtonText: t('common.cancel'),
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
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
}
.status-pill:hover {
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
}
/* 离线：红色主题 */
.status-pill.is-offline {
  border-color: rgba(248, 113, 113, 0.3);
  background: rgba(248, 113, 113, 0.1);
  color: #f87171;
}
/* 部分离线：琥珀色主题 */
.status-pill.is-partial {
  border-color: rgba(251, 191, 36, 0.35);
  background: rgba(251, 191, 36, 0.1);
  color: #fbbf24;
}

/* ── 节点健康弹窗 ──────────────────────────────────────────────────── */
.nodes-hint {
  margin: 0 0 var(--space-3);
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.nodes-table {
  margin-bottom: var(--space-3);
}
.node-id {
  color: var(--indigo-400);
  font-size: var(--text-xs);
}
.node-addr {
  color: var(--text-primary);
  font-size: var(--text-xs);
}
.node-ver {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.node-time {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.nodes-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.nodes-sum {
  color: var(--text-faint);
  font-size: 11px;
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

/* ── 侧边栏折叠态：收窄为图标栏 ─────────────────────────────────────── */
.sidebar {
  transition: width var(--dur-fast) var(--ease-out);
}
.sidebar.is-collapsed {
  padding-left: 8px;
  padding-right: 8px;
}
.sidebar.is-collapsed .brand {
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
}
.sidebar.is-collapsed .brand-name,
.sidebar.is-collapsed .status-pill span,
.sidebar.is-collapsed .ver {
  display: none;
}
.sidebar.is-collapsed .status-pill {
  justify-content: center;
  padding: 5px 10px;
}
/* 折叠态：菜单项改为图标居中（flex）。必须把左右 padding 都清零——
   element-plus 菜单项默认 padding-right 为 var(--el-menu-base-level-padding)（20px），
   只清 padding-left 会造成内容区不对称、图标偏离中心 */
.sidebar.is-collapsed .side-menu :deep(.el-menu-item) {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 !important;
  margin-left: 0;
  margin-right: 0;
}
.sidebar.is-collapsed .side-menu :deep(.el-menu-item .el-icon) {
  margin-right: 0;
}

/* ── Main column ─────────────────────────────────────────────────── */
.main {
  flex: 1;
  margin-left: var(--sbw);
  transition: margin-left var(--dur-fast) var(--ease-out);
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.collapse-btn {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--bg-card);
  color: var(--text-secondary);
  cursor: pointer;
  transition: color var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out),
              box-shadow var(--dur-fast) var(--ease-out);
}
.collapse-btn:hover {
  color: var(--indigo-400);
  border-color: var(--border-accent);
  box-shadow: var(--shadow-glow);
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
/* 语言切换按钮：沿用 theme-btn 外观，hover 跟随主题色（indigo）而非日/月切换的 amber */
.lang-btn:hover {
  color: var(--indigo-400);
}
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
  .collapse-btn { display: none; } /* 移动端使用底部导航，无需折叠按钮 */
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
