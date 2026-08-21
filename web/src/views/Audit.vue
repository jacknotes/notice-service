<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">操作审计</h1>
        <p class="sub">追踪管理员的操作记录：谁在什么时候做了什么（保留 {{ RETENTION_DAYS }} 天）</p>
      </div>
    </div>

    <div class="filters">
      <div class="filter-item">
        <span class="filter-label">关键词</span>
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          placeholder="搜索用户 / 详情…"
          @keyup.enter="applyKeyword"
          @clear="applyKeyword"
        />
      </div>

      <div class="filter-item">
        <span class="filter-label">操作</span>
        <el-select v-model="actionFilter" clearable placeholder="全部操作" style="width: 180px" @change="applyKeyword">
          <el-option
            v-for="a in actionOptions"
            :key="a.value"
            :label="a.label"
            :value="a.value"
          />
        </el-select>
      </div>

      <div class="filter-item date-filter">
        <span class="filter-label">日期</span>
        <div class="date-quick">
          <el-button
            v-for="p in quickPresets"
            :key="p.key"
            size="small"
            :type="quickPreset === p.key ? 'primary' : 'default'"
            plain
            @click="applyQuickPreset(p)"
          >
            {{ p.label }}
          </el-button>
        </div>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
          :clearable="true"
          style="width: 240px"
          @change="onDateRangeChange"
        />
      </div>

      <div class="filter-meta mono">{{ total }} 条记录</div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        :data="items"
        style="width: 100%"
        empty-text="暂无审计记录"
      >
        <el-table-column prop="id" label="ID" width="80" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column label="时间" min-width="170">
          <template #default="{ row }">
            <span class="mono time-cell">{{ fmtTime(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="用户" min-width="140">
          <template #default="{ row }">
            <span class="user-cell">{{ row.username || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-tag :style="actionTagStyle(row.action)" effect="plain" size="small">
              {{ actionLabel(row.action) }}
            </el-tag>
            <span class="mono action-raw">{{ row.action }}</span>
          </template>
        </el-table-column>

        <el-table-column label="详情" min-width="320" show-overflow-tooltip>
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
import { auditApi } from '@/api'

interface AuditRow {
  id: number
  user_id: number
  username: string
  action: string
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

const RETENTION_DAYS = 180 // 与后端 AUDIT_RETENTION_DAYS 默认一致

/* ── 操作类型选项（后端 action 值 → 中文） ──────────────────────────── */
const ACTION_LABELS: Record<string, string> = {
  'login.success': '登录成功',
  'login.failed': '登录失败',
  'login.step1': '登录(待2FA)',
  logout: '登出',
  'auth.2fa_setup': '生成2FA密钥',
  'auth.2fa_enable': '启用2FA',
  'auth.2fa_disable': '关闭2FA',
  'channel.create': '新建渠道',
  'channel.update': '更新渠道',
  'channel.delete': '删除渠道',
  'channel.batch_delete': '批量删渠道',
  'channel.test': '测试渠道',
  'template.create': '新建模板',
  'template.update': '更新模板',
  'template.delete': '删除模板',
  'template.batch_delete': '批量删模板',
  'task.create': '新建任务',
  'task.update': '更新任务',
  'task.delete': '删除任务',
  'task.batch_delete': '批量删任务',
  'task.toggle': '启停任务',
  'task.send_now': '立即发送',
  'log.retry': '日志重试',
  'user.create': '新建用户',
  'user.update': '更新用户',
  'user.delete': '删除用户',
  'user.batch_delete': '批量删用户',
  'user.reset_token': '生成重置令牌',
}

const actionOptions = Object.entries(ACTION_LABELS).map(([value, label]) => ({ value, label }))

function actionLabel(action: string) {
  return ACTION_LABELS[action] || action
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
  { key: 'today', label: '今天', days: 0 },
  { key: 'week', label: '最近一周', days: 7 },
  { key: 'month', label: '最近一个月', days: 30 },
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
      keyword?: string; action?: string; from?: string; to?: string
      page: number; page_size: number
    } = { page: page.value, page_size: pageSize.value }
    if (keyword.value.trim()) params.keyword = keyword.value.trim()
    if (actionFilter.value) params.action = actionFilter.value
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
.action-raw {
  margin-left: 6px;
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.detail-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
</style>
