<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">发送日志</h1>
        <p class="sub">查看所有任务的投递记录与失败回执</p>
      </div>
    </div>

    <div class="filters">
      <div class="filter-item">
        <span class="filter-label">任务</span>
        <el-select
          v-model="taskFilter"
          clearable
          placeholder="全部任务"
          style="width: 220px"
        >
          <el-option
            v-for="t in tasks"
            :key="t.id"
            :label="t.name"
            :value="t.id"
          />
        </el-select>
      </div>

      <div class="filter-item">
        <span class="filter-label">状态</span>
        <el-select v-model="statusFilter" clearable placeholder="全部状态" style="width: 150px">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
      </div>

      <div class="filter-meta mono">
        {{ filteredLogs.length }} 条记录
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        v-if="logs.length > 0"
        :data="filteredLogs"
        style="width: 100%"
        empty-text="没有符合条件的日志，试试调整筛选条件"
      >
        <el-table-column prop="id" label="ID" width="72" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column label="任务" min-width="160">
          <template #default="{ row }">
            <span class="task-name-cell">{{ taskName(row.task_id) }}</span>
            <span class="mono task-id-cell">#{{ row.task_id }}</span>
          </template>
        </el-table-column>

        <el-table-column label="渠道" min-width="160">
          <template #default="{ row }">
            <span class="task-name-cell">{{ channelName(row.channel_id) }}</span>
            <span class="mono task-id-cell">#{{ row.channel_id }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'success' ? 'success' : 'danger'" effect="light" size="small">
              {{ row.status === 'success' ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="重试" width="80" align="center">
          <template #default="{ row }">
            <span class="mono retry-cell">{{ row.retry_count ?? 0 }}</span>
          </template>
        </el-table-column>

        <el-table-column label="错误信息" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.error_msg" class="err-cell">{{ row.error_msg }}</span>
            <span v-else class="ok-cell">—</span>
          </template>
        </el-table-column>

        <el-table-column label="时间" min-width="170">
          <template #default="{ row }">
            <span class="mono time-cell">{{ fmtTime(row.sent_at) }}</span>
          </template>
        </el-table-column>
      </el-table>

      <el-empty
        v-else-if="!loading && tasksLoaded"
        :description="emptyDescription"
        class="logs-empty"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { channelApi, taskApi } from '@/api'

interface LogRow {
  id: number
  task_id: number
  channel_id: number
  status: 'success' | 'failed'
  request?: string
  response?: string
  error_msg?: string
  retry_count?: number
  sent_at?: string
}

const route = useRoute()

const loading = ref(false)
const tasksLoaded = ref(false)
const tasks = ref<{ id: number; name: string }[]>([])
const channels = ref<{ id: number; name: string }[]>([])
const logs = ref<LogRow[]>([])

const taskFilter = ref<number | undefined>(undefined)
const statusFilter = ref<'success' | 'failed' | ''>('')

const taskName = (id: number) => tasks.value.find((t) => t.id === id)?.name || `任务 #${id}`
const channelName = (id: number) => channels.value.find((c) => c.id === id)?.name || `渠道 #${id}`

// 把 ISO 时间格式化为本地 "YYYY-MM-DD HH:mm:ss"；零值/非法显示 "—"
function fmtTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const filteredLogs = computed<LogRow[]>(() =>
  logs.value.filter((l) => {
    if (taskFilter.value !== undefined && l.task_id !== taskFilter.value) return false
    if (statusFilter.value && l.status !== statusFilter.value) return false
    return true
  })
)

const emptyDescription = computed(() => {
  if (taskFilter.value !== undefined || statusFilter.value) return '没有符合条件的日志，试试调整筛选条件'
  return '暂无发送日志，任务触发投递后这里会实时记录'
})

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  tasksLoaded.value = false
  try {
    const [list, chList] = await Promise.all([taskApi.list(), channelApi.list()])
    tasks.value = list || []
    channels.value = chList || []

    const groups = await Promise.all(
      (list || []).map((t: { id: number }) =>
        taskApi.logs(t.id).catch(() => [])
      )
    )
    logs.value = groups.flat() as LogRow[]
    tasksLoaded.value = true
  } catch (e: any) {
    ElMessage.error(errMsg(e, '日志加载失败'))
  } finally {
    loading.value = false
  }
}

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

onMounted(load)
</script>

<style scoped>
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

.id-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
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
.retry-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.err-cell {
  color: var(--rose-400);
  font-size: var(--text-xs);
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
</style>
