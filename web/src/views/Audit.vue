<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">{{ t('nav.audit') }}</h1>
        <p class="sub">{{ t('audit.subtitle', { days: RETENTION_DAYS }) }}</p>
      </div>
    </div>

    <div class="filters">
      <div class="filter-item">
        <span class="filter-label">{{ t('audit.keywordField') }}</span>
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          :placeholder="t('audit.searchPlaceholder')"
          @keyup.enter="applyKeyword"
          @clear="applyKeyword"
        />
      </div>

      <div class="filter-item">
        <span class="filter-label">{{ t('audit.moduleField') }}</span>
        <el-select v-model="moduleFilter" clearable :placeholder="t('audit.allModules')" style="width: 140px" @change="applyKeyword">
          <el-option
            v-for="m in moduleOptions"
            :key="m.value"
            :label="t(m.labelKey)"
            :value="m.value"
          />
        </el-select>
      </div>

      <div class="filter-item">
        <span class="filter-label">{{ t('audit.actionField') }}</span>
        <el-select v-model="actionFilter" clearable :placeholder="t('audit.allActions')" style="width: 180px" @change="applyKeyword">
          <el-option
            v-for="a in actionOptions"
            :key="a.value"
            :label="t(a.labelKey)"
            :value="a.value"
          />
        </el-select>
      </div>

      <div class="filter-item date-filter">
        <span class="filter-label">{{ t('common.date') }}</span>
        <div class="date-quick">
          <el-button
            v-for="p in quickPresets"
            :key="p.key"
            size="small"
            :type="quickPreset === p.key ? 'primary' : 'default'"
            plain
            @click="applyQuickPreset(p)"
          >
            {{ t(p.labelKey) }}
          </el-button>
        </div>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          :range-separator="t('common.to')"
          :start-placeholder="t('common.startDate')"
          :end-placeholder="t('common.endDate')"
          value-format="YYYY-MM-DD"
          :clearable="true"
          style="width: 240px"
          @change="onDateRangeChange"
        />
      </div>

      <div class="filter-meta mono">{{ t('audit.recordCount', { n: total }) }}</div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        :data="items"
        style="width: 100%"
        :empty-text="t('audit.emptyTable')"
      >
        <el-table-column prop="id" label="ID" width="80" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.time')" min-width="170">
          <template #default="{ row }">
            <span class="mono time-cell">{{ fmtTime(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('audit.userCol')" min-width="130">
          <template #default="{ row }">
            <span class="user-cell">{{ row.username || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('audit.ipCol')" min-width="120">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.ip || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="200">
          <template #default="{ row }">
            <div class="action-cell">
              <el-tag :style="moduleTagStyle(row.module)" effect="plain" size="small" class="module-tag">
                {{ moduleLabel(row.module) }}
              </el-tag>
              <el-tag :style="actionTagStyle(row.action)" effect="plain" size="small">
                {{ actionLabel(row.action) }}
              </el-tag>
            </div>
            <span class="mono action-raw">{{ row.action }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('audit.detailCol')" min-width="320" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="detail-cell">{{ row.detail || '—' }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div v-if="total > 0" class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @current-change="load"
        @size-change="onPageSizeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { auditApi } from '@/api'

const { t } = useI18n()

interface AuditRow {
  id: number
  user_id: number
  username: string
  ip: string
  action: string
  module: string
  detail: string
  created_at: string
}

const loading = ref(false)
const items = ref<AuditRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const keyword = ref('')
const actionFilter = ref('')
const moduleFilter = ref('')

const RETENTION_DAYS = 180 // 与后端 AUDIT_RETENTION_DAYS 默认一致

/* ── 模块分类（后端按 action 前缀推导） ─────────────────────────────── */
interface FilterOption {
  value: string
  labelKey: string
}

const moduleOptions: FilterOption[] = [
  { value: 'auth', labelKey: 'audit.module.auth' },
  { value: 'channel', labelKey: 'audit.module.channel' },
  { value: 'template', labelKey: 'audit.module.template' },
  { value: 'task', labelKey: 'audit.module.task' },
  { value: 'log', labelKey: 'audit.module.log' },
  { value: 'user', labelKey: 'audit.module.user' },
  { value: 'other', labelKey: 'audit.module.other' },
]

const MODULE_KEY: Record<string, string> = Object.fromEntries(
  moduleOptions.map((o) => [o.value, o.labelKey])
)

function moduleLabel(module: string) {
  const key = MODULE_KEY[module]
  return (module && key ? t(key) : module) || '—'
}

function moduleTagStyle(module: string) {
  const c =
    module === 'auth' ? '#8b5cf6'
    : module === 'channel' ? '#818cf8'
    : module === 'template' ? '#38bdf8'
    : module === 'task' ? '#f59e0b'
    : module === 'log' ? '#34d399'
    : module === 'user' ? '#f472b6'
    : '#94a3b8'
  return { color: c, borderColor: `${c}55`, backgroundColor: `${c}1a` }
}

/* ── 操作类型选项（后端 action 值 → 本地化标签） ─────────────────────── */
const ACTION_KEY: Record<string, string> = {
  'login.success': 'audit.action.loginSuccess',
  'login.failed': 'audit.action.loginFailed',
  'login.step1': 'audit.action.loginStep1',
  logout: 'audit.action.logout',
  'auth.2fa_setup': 'audit.action.twoFaSetup',
  'auth.2fa_enable': 'audit.action.twoFaEnable',
  'auth.2fa_disable': 'audit.action.twoFaDisable',
  'channel.create': 'audit.action.channelCreate',
  'channel.update': 'audit.action.channelUpdate',
  'channel.delete': 'audit.action.channelDelete',
  'channel.batch_delete': 'audit.action.channelBatchDelete',
  'channel.test': 'audit.action.channelTest',
  'template.create': 'audit.action.templateCreate',
  'template.update': 'audit.action.templateUpdate',
  'template.delete': 'audit.action.templateDelete',
  'template.batch_delete': 'audit.action.templateBatchDelete',
  'task.create': 'audit.action.taskCreate',
  'task.update': 'audit.action.taskUpdate',
  'task.delete': 'audit.action.taskDelete',
  'task.batch_delete': 'audit.action.taskBatchDelete',
  'task.toggle': 'audit.action.taskToggle',
  'task.send_now': 'audit.action.taskSendNow',
  'log.retry': 'audit.action.logRetry',
  'user.create': 'audit.action.userCreate',
  'user.update': 'audit.action.userUpdate',
  'user.delete': 'audit.action.userDelete',
  'user.batch_delete': 'audit.action.userBatchDelete',
  'user.reset_token': 'audit.action.userResetToken',
  'user.disable': 'audit.action.userDisable',
  'user.enable': 'audit.action.userEnable',
  'user.2fa_force_enable': 'audit.action.userTwoFaForceEnable',
  'user.2fa_force_disable': 'audit.action.userTwoFaForceDisable',
}

const actionOptions: FilterOption[] = Object.entries(ACTION_KEY).map(([value, labelKey]) => ({ value, labelKey }))

function actionLabel(action: string) {
  const key = ACTION_KEY[action]
  return (action && key ? t(key) : action)
}

function actionTagStyle(action: string) {
  if (action.includes('login.failed') || action.includes('delete')) {
    return { color: 'var(--rose-400)', borderColor: 'rgba(248,113,113,0.4)', backgroundColor: 'rgba(248,113,113,0.12)' }
  }
  if (action.startsWith('login') || action.includes('2fa')) {
    return { color: 'var(--violet-400)', borderColor: 'rgba(139,92,246,0.4)', backgroundColor: 'rgba(139,92,246,0.12)' }
  }
  return { color: 'var(--sky-400)', borderColor: 'rgba(56,189,248,0.4)', backgroundColor: 'rgba(56,189,248,0.12)' }
}

/* ── 日期范围（与发送日志一致） ─────────────────────────────────────── */
const quickPresets = [
  { key: 'today', labelKey: 'common.today', days: 0 },
  { key: 'week', labelKey: 'common.lastWeek', days: 7 },
  { key: 'month', labelKey: 'common.lastMonth', days: 30 },
]
const quickPreset = ref('month')
const dateRange = ref<[string, string] | null>(null)

function fmtDate(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function applyQuickPreset(p: { key: string; days: number }) {
  quickPreset.value = p.key
  const now = new Date()
  const todayEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
  if (p.days === 0) {
    dateRange.value = [fmtDate(now), fmtDate(todayEnd)]
  } else {
    const start = new Date(todayEnd.getTime() - p.days * 86400000)
    dateRange.value = [fmtDate(start), fmtDate(todayEnd)]
  }
  applyKeyword()
}

applyQuickPreset(quickPresets[2])

function onDateRangeChange() {
  if (!dateRange.value) {
    applyQuickPreset(quickPresets[2])
    return
  }
  quickPreset.value = ''
  applyKeyword()
}

function fmtTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

/* ── 加载 ──────────────────────────────────────────────────────────── */
async function load() {
  loading.value = true
  try {
    const params: {
      keyword?: string; action?: string; module?: string; from?: string; to?: string
      page: number; page_size: number
    } = { page: page.value, page_size: pageSize.value }
    if (keyword.value.trim()) params.keyword = keyword.value.trim()
    if (actionFilter.value) params.action = actionFilter.value
    if (moduleFilter.value) params.module = moduleFilter.value
    if (dateRange.value) {
      params.from = dateRange.value[0]
      params.to = dateRange.value[1]
    }
    const data = await auditApi.list(params)
    items.value = (data?.items || []) as AuditRow[]
    total.value = data?.total || 0
  } catch (e: any) {
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function applyKeyword() {
  page.value = 1
  load()
}

function onPageSizeChange() {
  page.value = 1
  load()
}

onMounted(load)
</script>

<style scoped>
.search-input { width: 220px; }

.table-card {
  padding: 8px 14px 14px;
  overflow: hidden;
}

.filters {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  flex-wrap: wrap;
  margin-bottom: var(--space-4);
}
.filter-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.filter-label {
  color: var(--text-secondary);
  font-size: var(--text-sm);
  white-space: nowrap;
}
.filter-meta {
  margin-left: auto;
  color: var(--text-faint);
  font-size: 11px;
  letter-spacing: 0.04em;
}

.date-filter {
  flex-wrap: wrap;
  row-gap: var(--space-2);
}
.date-quick {
  display: flex;
  align-items: center;
  gap: 6px;
}
.date-quick .el-button { margin: 0; }

.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
}

.id-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.time-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.user-cell {
  color: var(--text-primary);
  font-size: var(--text-sm);
  font-weight: 500;
}
.action-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
.module-tag {
  color: var(--text-secondary) !important;
}
.action-raw {
  margin-left: 2px;
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.detail-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
</style>
