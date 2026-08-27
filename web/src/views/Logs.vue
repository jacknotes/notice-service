<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">{{ t('nav.logs') }}</h1>
        <p class="sub">{{ t('logs.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-button
          v-if="isAdmin"
          :icon="Download"
          :loading="exporting"
          @click="exportCsv"
        >
          {{ t('logs.exportCsv') }}
        </el-button>
      </div>
    </div>

    <div class="filters">
      <div class="filter-item">
        <span class="filter-label">{{ t('logs.taskCol') }}</span>
        <el-select
          v-model="taskFilter"
          clearable
          :placeholder="t('logs.allTasks')"
          style="width: 220px"
        >
          <el-option
            v-for="tk in tasks"
            :key="tk.id"
            :label="tk.name"
            :value="tk.id"
          />
        </el-select>
      </div>

      <div class="filter-item">
        <span class="filter-label">{{ t('common.status') }}</span>
        <el-select v-model="statusFilter" clearable :placeholder="t('logs.allStatus')" style="width: 150px">
          <el-option :label="t('common.success')" value="success" />
          <el-option :label="t('common.failed')" value="failed" />
        </el-select>
      </div>

      <div class="filter-item">
        <span class="filter-label">{{ t('logs.keywordLabel') }}</span>
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          :placeholder="t('logs.searchPlaceholder')"
        />
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

      <div class="filter-meta mono">
        {{ t('logs.recordCount', { n: filteredLogs.length }) }}
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        v-if="logs.length > 0"
        :data="filteredLogs"
        style="width: 100%"
        :empty-text="t('logs.emptyFiltered')"
        @sort-change="onSortChange"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="log-detail">
              <div v-if="row.trigger_type || row.trigger_by || row.trigger_ip" class="detail-block detail-trigger">
                <span class="detail-label">{{ t('logs.triggerSource') }}</span>
                <div class="trigger-line">
                  <el-tag
                    v-if="row.trigger_type"
                    :style="triggerTagStyle(row.trigger_type)"
                    effect="plain"
                    size="small"
                  >
                    {{ triggerLabel(row.trigger_type) }}
                  </el-tag>
                  <span class="mono detail-value-text">{{ t('logs.byPerson') }}{{ row.trigger_by || '—' }}</span>
                  <span class="mono detail-value-text">{{ t('logs.byIp') }}{{ row.trigger_ip || '—' }}</span>
                </div>
              </div>

              <div v-if="row.subject" class="detail-block">
                <span class="detail-label">{{ t('logs.subjectCol') }}</span>
                <p class="detail-subject">{{ row.subject }}</p>
              </div>

              <div v-if="row.content" class="detail-block">
                <span class="detail-label">{{ t('logs.contentLabel') }}</span>
                <pre class="detail-code mono">{{ row.content }}</pre>
              </div>

              <div v-if="row.request" class="detail-block">
                <span class="detail-label">{{ t('logs.requestLabel') }}</span>
                <pre class="detail-code mono">{{ row.request }}</pre>
              </div>

              <div v-if="row.response" class="detail-block">
                <span class="detail-label">{{ t('logs.responseLabel') }}</span>
                <pre class="detail-code mono">{{ row.response }}</pre>
              </div>

              <div v-if="row.error_msg" class="detail-block">
                <span class="detail-label">{{ t('logs.errorMsgLabel') }}</span>
                <pre class="detail-code mono detail-error">{{ row.error_msg }}</pre>
              </div>

              <div v-if="row.retry_count" class="detail-block">
                <span class="detail-label">{{ t('logs.retryCountLabel') }}</span>
                <span class="mono detail-value-text">{{ row.retry_count }}</span>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="id" label="ID" width="72" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('logs.subjectCol')" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.subject" class="subject-cell">{{ row.subject }}</span>
            <span v-else class="ok-cell">—</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('logs.taskCol')" min-width="160" sortable="custom" prop="task_id">
          <template #default="{ row }">
            <span class="task-name-cell">{{ taskName(row.task_id) }}</span>
            <span class="mono task-id-cell">#{{ row.task_id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('logs.channelCol')" min-width="150" sortable="custom" prop="channel_id">
          <template #default="{ row }">
            <span class="task-name-cell">{{ channelName(row.channel_id) }}</span>
            <span class="mono task-id-cell">#{{ row.channel_id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.status')" width="100" align="center" sortable="custom" prop="status">
          <template #default="{ row }">
            <el-tag :type="row.status === 'success' ? 'success' : 'danger'" effect="light" size="small">
              {{ row.status === 'success' ? t('common.success') : t('common.failed') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.time')" min-width="170" sortable="custom" prop="sent_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ fmtTime(row.sent_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="140" align="center">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="goDetail(row)">{{ t('logs.detailAction') }}</el-button>
            <el-button
              v-if="row.status === 'failed'"
              link
              type="danger"
              size="small"
              :loading="retryingId === row.id"
              @click="retryLog(row)"
            >
              {{ t('logs.retryAction') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty
        v-else-if="!loading && tasksLoaded"
        :description="emptyDescription"
        class="logs-empty"
      />
    </div>

    <div v-if="total > 0" class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @current-change="loadLogs"
        @size-change="onPageSizeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, Search } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { channelApi, logApi, taskApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()

interface LogRow {
  id: number
  task_id: number
  channel_id: number
  subject?: string
  content?: string
  status: 'success' | 'failed'
  request?: string
  response?: string
  error_msg?: string
  retry_count?: number
  trigger_type?: string
  trigger_by?: string
  trigger_ip?: string
  sent_at?: string
}

const route = useRoute()
const router = useRouter()

const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')

const loading = ref(false)
const tasksLoaded = ref(false)
const tasks = ref<{ id: number; name: string }[]>([])
const channels = ref<{ id: number; name: string }[]>([])
const logs = ref<LogRow[]>([])

const taskFilter = ref<number | undefined>(undefined)
const statusFilter = ref<'success' | 'failed' | ''>('')
const keyword = ref('')

/* ── 后端排序 ──────────────────────────────────────────────────────── */
const sortBy = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('desc')

// el-table sort-change：仅在后端白名单列上生效（列上标 sortable="custom"）。
function onSortChange({ prop, order }: { prop: string; order: string | null }) {
  if (!order) {
    sortBy.value = ''
    sortOrder.value = 'desc'
  } else {
    sortBy.value = prop
    sortOrder.value = order === 'ascending' ? 'asc' : 'desc'
  }
  page.value = 1
  loadLogs()
}

// 触发方式 → 本地化标签 / 标签配色
const TRIGGER_KEY: Record<string, string> = {
  cron: 'logs.triggerType.cron',
  webhook: 'logs.triggerType.webhook',
  manual: 'logs.triggerType.manual',
  retry: 'logs.triggerType.retry',
}
const TRIGGER_COLOR: Record<string, string> = {
  cron: '#38bdf8',
  webhook: '#8b5cf6',
  manual: '#fbbf24',
  retry: '#f87171',
}
function triggerLabel(type?: string) {
  if (!type) return '—'
  const key = TRIGGER_KEY[type]
  return key ? t(key) : type
}
function triggerTagStyle(type?: string) {
  const c = (type && TRIGGER_COLOR[type]) || '#94a3b8'
  return { color: c, borderColor: `${c}55`, backgroundColor: `${c}1a` }
}

/* ── 日期范围 ──────────────────────────────────────────────────────────
   默认展示最近一个月（不展示全部）；快捷按钮 + 自定义日期范围；
   最大跨度 1 年。dateRange 为 ['YYYY-MM-DD', 'YYYY-MM-DD')，结束日期排他。 */
const quickPresets = [
  { key: 'today', labelKey: 'common.today', days: 0 },
  { key: 'week', labelKey: 'common.lastWeek', days: 7 },
  { key: 'month', labelKey: 'common.lastMonth', days: 30 },
]
const MAX_RANGE_DAYS = 366 // 1 年（含闰年）
const quickPreset = ref('month')
const dateRange = ref<[string, string] | null>(null)

function fmtDate(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function applyQuickPreset(p: { key: string; labelKey: string; days: number }) {
  quickPreset.value = p.key
  const now = new Date()
  const todayEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1) // 结束排他
  if (p.days === 0) {
    dateRange.value = [fmtDate(now), fmtDate(todayEnd)] // 今天全天
  } else {
    const start = new Date(todayEnd.getTime() - p.days * 86400000)
    dateRange.value = [fmtDate(start), fmtDate(todayEnd)]
  }
}

// 默认最近一个月
applyQuickPreset(quickPresets[2])

function onDateRangeChange() {
  if (!dateRange.value) {
    // 清空 → 回退到最近一个月（不展示全部）
    applyQuickPreset(quickPresets[2])
    return
  }
  quickPreset.value = ''
  // 跨度上限 1 年：超出则把结束日期收窄到开始日期 + 1 年
  const [s, e] = dateRange.value
  const spanDays = (new Date(e).getTime() - new Date(s).getTime()) / 86400000
  if (spanDays > MAX_RANGE_DAYS) {
    const capped = new Date(new Date(s).getTime() + MAX_RANGE_DAYS * 86400000)
    dateRange.value = [s, fmtDate(capped)]
    ElMessage.warning(t('logs.rangeCapWarn'))
  }
}

const taskName = (id: number) => tasks.value.find((x) => x.id === id)?.name || t('logs.taskFallback', { id })
const channelName = (id: number) => channels.value.find((x) => x.id === id)?.name || t('logs.channelFallback', { id })

// 把 ISO 时间格式化为本地 "YYYY-MM-DD HH:mm:ss"；零值/非法显示 "—"
function fmtTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// 任务/状态/日期已由后端过滤，前端仅做关键词二次过滤（针对当前页）
const filteredLogs = computed<LogRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return logs.value
  return logs.value.filter((l) => {
    const hit =
      taskName(l.task_id).toLowerCase().includes(kw) ||
      channelName(l.channel_id).toLowerCase().includes(kw) ||
      (l.subject || '').toLowerCase().includes(kw) ||
      (l.content || '').toLowerCase().includes(kw) ||
      (l.error_msg || '').toLowerCase().includes(kw) ||
      (l.trigger_by || '').toLowerCase().includes(kw) ||
      (l.trigger_ip || '').toLowerCase().includes(kw) ||
      triggerLabel(l.trigger_type).toLowerCase().includes(kw)
    return hit
  })
})

const emptyDescription = computed(() => {
  if (taskFilter.value !== undefined || statusFilter.value || keyword.value.trim() || dateRange.value)
    return t('logs.emptyFiltered')
  return t('logs.emptyAll')
})

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

/* ── 后端分页加载 ──────────────────────────────────────────────────── */
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

/* ── 重试失败日志（定向重发该条） ───────────────────────────────────── */
const retryingId = ref<number | null>(null)

async function retryLog(row: LogRow) {
  try {
    await ElMessageBox.confirm(
      t('logs.sendRetryMsg', { task: taskName(row.task_id), channel: channelName(row.channel_id) }),
      t('logs.sendRetryTitle'),
      { confirmButtonText: t('logs.retryBtn'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  retryingId.value = row.id
  try {
    await logApi.retry(row.id)
    ElMessage.success(t('logs.queuedOk'))
    await loadLogs()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('logs.retryFailed')))
  } finally {
    retryingId.value = null
  }
}

/* ── 导出 CSV（仅管理员；筛选条件与列表一致） ───────────────────────── */
const exporting = ref(false)

function goDetail(row: LogRow) {
  router.push('/logs/' + row.id)
}

async function exportCsv() {
  if (exporting.value) return
  exporting.value = true
  try {
    const params: { task_id?: number; status?: string; from?: string; to?: string } = {}
    if (taskFilter.value !== undefined) params.task_id = taskFilter.value
    if (statusFilter.value) params.status = statusFilter.value
    if (dateRange.value) {
      params.from = dateRange.value[0]
      params.to = dateRange.value[1]
    }
    const blob = await logApi.export(params)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `logs-${fmtDate(new Date())}.csv`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    ElMessage.success(t('logs.exportedOk'))
  } catch (e: any) {
    // blob 响应下错误也是 Blob，先尝试解析其中的 {error}
    const raw = e?.response?.data
    if (raw instanceof Blob) {
      try {
        const parsed = JSON.parse(await raw.text())
        ElMessage.error(parsed?.error || t('logs.exportFailed'))
        return
      } catch {
        /* 非 JSON 错误体，走默认文案 */
      }
    }
    ElMessage.error(errMsg(e, t('logs.exportFailed')))
  } finally {
    exporting.value = false
  }
}

async function loadLogs() {
  loading.value = true
  try {
    const params: {
      task_id?: number; status?: string; from?: string; to?: string
      page: number; page_size: number; sort_by?: string; sort_order?: 'asc' | 'desc'
    } = {
      page: page.value,
      page_size: pageSize.value,
    }
    if (taskFilter.value !== undefined) params.task_id = taskFilter.value
    if (statusFilter.value) params.status = statusFilter.value
    if (dateRange.value) {
      params.from = dateRange.value[0]
      params.to = dateRange.value[1]
    }
    if (sortBy.value) {
      params.sort_by = sortBy.value
      params.sort_order = sortOrder.value
    }
    const data = await logApi.query(params)
    logs.value = (data?.items || []) as LogRow[]
    total.value = data?.total || 0
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('logs.loadFailed')))
  } finally {
    loading.value = false
    tasksLoaded.value = true
  }
}

async function loadMeta() {
  try {
    const [list, chList] = await Promise.all([taskApi.list(), channelApi.list()])
    tasks.value = list || []
    channels.value = chList || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('logs.metaLoadFailed')))
  }
}

function onPageSizeChange() {
  page.value = 1
  loadLogs()
}

// 任务/状态/日期变化时回到第一页并重新查询
watch([taskFilter, statusFilter, dateRange], () => {
  page.value = 1
  loadLogs()
})

// 从「任务管理」跳转过来时按任务预筛选
watch(
  () => route.query.task,
  (val) => {
    if (val !== undefined && val !== null && val !== '') {
      taskFilter.value = Number(val)
    }
  },
  { immediate: true }
)

onMounted(() => {
  loadMeta()
  loadLogs()
})
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

.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
}

/* ── 日期筛选 ───────────────────────────────────────────────────────── */
.date-filter {
  flex-wrap: wrap;
  row-gap: var(--space-2);
}
.date-quick {
  display: flex;
  align-items: center;
  gap: 6px;
}
.date-quick .el-button {
  margin: 0;
}

.id-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.subject-cell {
  color: var(--text-primary);
  font-size: var(--text-sm);
  font-weight: 500;
}
.task-id-cell {
  color: var(--indigo-400);
  font-size: var(--text-xs);
}
.task-name-cell {
  color: var(--text-primary);
  font-size: var(--text-sm);
  margin-right: 8px;
}
.ok-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.time-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}

.logs-empty {
  padding: var(--space-8) 0;
}

/* ── Expandable detail panel ───────────────────────────────────────── */
.table-card :deep(.el-table__expanded-cell) {
  padding: 0;
}
.log-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
  background: rgba(148, 163, 184, 0.04);
  border-left: 2px solid var(--indigo-500);
}
.detail-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.detail-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text-faint);
}
.trigger-line {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}
.detail-value-text {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.detail-subject {
  color: var(--text-primary);
  font-size: var(--text-sm);
  font-weight: 600;
  line-height: 1.6;
  word-break: break-word;
}
.detail-code {
  margin: 0;
  max-height: 220px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(148, 163, 184, 0.06);
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.65;
}
.detail-code.detail-error {
  color: var(--rose-400);
  border-color: rgba(248, 113, 113, 0.3);
  background: rgba(248, 113, 113, 0.06);
}
</style>
