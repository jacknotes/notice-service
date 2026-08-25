<template>
  <div class="page">
    <div class="page-head">
      <div>
        <router-link class="back-link" to="/logs">
          <span class="back-arrow">←</span> 返回日志列表
        </router-link>
        <h1 class="grad-text">{{ log?.subject || `发送日志 #${id}` }}</h1>
        <p class="sub">单条投递记录详情</p>
      </div>
      <div class="actions">
        <el-button
          v-if="log?.status === 'failed'"
          type="danger"
          :loading="retrying"
          @click="retryLog"
        >
          重试发送
        </el-button>
      </div>
    </div>

    <div v-loading="loading" class="card detail-card">
      <template v-if="log">
        <el-descriptions :column="2" border class="meta-desc">
          <el-descriptions-item label="ID">
            <span class="mono">#{{ log.id }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="时间">
            <span class="mono">{{ fmtTime(log.sent_at) }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="任务">
            <span class="name-cell">{{ taskName }}</span>
            <span class="mono name-id">#{{ log.task_id }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="渠道">
            <span class="name-cell">{{ channelName }}</span>
            <span class="mono name-id">#{{ log.channel_id }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag
              :type="log.status === 'success' ? 'success' : 'danger'"
              effect="light"
              size="small"
            >
              {{ log.status === 'success' ? '成功' : '失败' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="重试次数">
            <span class="mono">{{ log.retry_count ?? 0 }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="触发方式">
            <el-tag
              v-if="log.trigger_type"
              :style="triggerTagStyle(log.trigger_type)"
              effect="plain"
              size="small"
            >
              {{ triggerLabel(log.trigger_type) }}
            </el-tag>
            <span v-else class="faint">—</span>
          </el-descriptions-item>
          <el-descriptions-item label="触发人">
            <span class="mono">{{ log.trigger_by || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="触发 IP">
            <span class="mono">{{ log.trigger_ip || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="错误信息" :span="2">
            <pre v-if="log.error_msg" class="desc-error mono">{{ log.error_msg }}</pre>
            <span v-else class="faint">—</span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="log.content" class="detail-block">
          <span class="detail-label">发送内容</span>
          <div class="content-panel">
            <MarkdownPreview :content="log.content" />
          </div>
        </div>

        <div v-if="log.request" class="detail-block">
          <span class="detail-label">请求</span>
          <pre class="detail-code mono">{{ prettyJson(log.request) }}</pre>
        </div>

        <div v-if="log.response" class="detail-block">
          <span class="detail-label">响应</span>
          <pre class="detail-code mono">{{ prettyJson(log.response) }}</pre>
        </div>
      </template>

      <el-empty v-else-if="!loading" description="日志不存在" class="detail-empty" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { channelApi, logApi, taskApi } from '@/api'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

interface LogDetail {
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

const id = computed(() => Number(route.params.id))

const loading = ref(false)
const log = ref<LogDetail | null>(null)
const retrying = ref(false)

const tasks = ref<{ id: number; name: string }[]>([])
const channels = ref<{ id: number; name: string }[]>([])

const taskName = computed(() => {
  if (!log.value) return ''
  return tasks.value.find((t) => t.id === log.value!.task_id)?.name || `任务 #${log.value!.task_id}`
})
const channelName = computed(() => {
  if (!log.value) return ''
  return channels.value.find((c) => c.id === log.value!.channel_id)?.name || `渠道 #${log.value!.channel_id}`
})

// 触发方式 → 中文标签 / 标签配色（与列表页一致）
const TRIGGER_META: Record<string, { label: string; color: string }> = {
  cron: { label: '定时', color: '#38bdf8' },
  webhook: { label: 'Webhook', color: '#8b5cf6' },
  manual: { label: '手动', color: '#fbbf24' },
  retry: { label: '重试', color: '#f87171' },
}
function triggerLabel(t?: string) {
  return (t && TRIGGER_META[t]?.label) || t || '—'
}
function triggerTagStyle(t?: string) {
  const c = (t && TRIGGER_META[t]?.color) || '#94a3b8'
  return { color: c, borderColor: `${c}55`, backgroundColor: `${c}1a` }
}

// ISO → 本地 "YYYY-MM-DD HH:mm:ss"；零值/非法显示 "—"
function fmtTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// request/response 若是 JSON 字符串则美化缩进，否则保留原样
function prettyJson(raw?: string) {
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

async function loadDetail() {
  loading.value = true
  try {
    log.value = await logApi.detail(id.value)
  } catch (e: any) {
    if (e?.response?.status === 404) {
      // 日志不存在（404）→ 空状态
      log.value = null
    } else {
      // 其它错误（网络/500/参数错误）→ 提示 + 空状态，避免误报「日志不存在」
      ElMessage.error(e?.response?.data?.error || '加载失败，请稍后再试')
      log.value = null
    }
  } finally {
    loading.value = false
  }
}

async function retryLog() {
  retrying.value = true
  try {
    await logApi.retry(id.value)
    ElMessage.success('已重试')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || '重试失败')
  } finally {
    retrying.value = false
  }
}

// 任务/渠道名称解析（失败不影响详情展示）
async function loadMeta() {
  try {
    const [ts, chs] = await Promise.all([taskApi.list(), channelApi.list()])
    tasks.value = ts || []
    channels.value = chs || []
  } catch {
    /* ignore */
  }
}

onMounted(() => {
  loadDetail()
  loadMeta()
})

// 路由参数变化（组件复用场景）时重新加载，避免展示陈旧日志
watch(
  () => route.params.id,
  () => {
    loadDetail()
    loadMeta()
  },
)
</script>

<style scoped>
.back-link {
  display: inline-block;
  color: var(--text-faint);
  font-size: var(--text-xs);
  letter-spacing: 0.05em;
  text-decoration: none;
  margin-bottom: var(--space-2);
  transition: color var(--dur-base) var(--ease-out);
}
.back-link:hover {
  color: var(--indigo-400);
}
.back-arrow {
  font-family: var(--font-mono);
}

.detail-card {
  padding: var(--space-5) var(--space-6);
  min-height: 240px;
}

.meta-desc {
  margin-bottom: var(--space-5);
}
.meta-desc :deep(.el-descriptions__label) {
  color: var(--text-faint);
  font-size: var(--text-xs);
  width: 96px;
}
.meta-desc :deep(.el-descriptions__content) {
  color: var(--text-primary);
  font-size: var(--text-sm);
}

.name-cell {
  color: var(--text-primary);
  font-size: var(--text-sm);
  margin-right: 8px;
}
.name-id {
  color: var(--indigo-400);
  font-size: var(--text-xs);
}
.faint {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.desc-error {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--rose-400);
  font-size: 12px;
  line-height: 1.65;
}

.detail-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  margin-bottom: var(--space-5);
}
.detail-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text-faint);
}
.content-panel {
  padding: var(--space-4);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(148, 163, 184, 0.05);
}
.detail-code {
  margin: 0;
  max-height: 420px;
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

.detail-empty {
  padding: var(--space-8) 0;
}
</style>
