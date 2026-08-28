<template>
  <div class="page">
    <div class="page-head">
      <div>
        <router-link class="back-link" to="/logs">
          <span class="back-arrow">←</span> {{ t('logs.backToList') }}
        </router-link>
        <h1 class="grad-text">{{ log?.subject || t('logs.fallbackTitle', { id }) }}</h1>
        <p class="sub">{{ t('logs.detailSubtitle') }}</p>
      </div>
      <div class="actions">
        <el-button
          v-if="log?.status === 'failed'"
          type="danger"
          :loading="retrying"
          @click="retryLog"
        >
          {{ t('logs.sendRetryTitle') }}
        </el-button>
      </div>
    </div>

    <div v-loading="loading" class="card detail-card">
      <template v-if="log">
        <el-descriptions :column="2" border class="meta-desc">
          <el-descriptions-item label="ID">
            <span class="mono">#{{ log.id }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('common.time')">
            <span class="mono">{{ fmtTime(log.sent_at) }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.taskCol')">
            <span class="name-cell">{{ taskName }}</span>
            <span class="mono name-id">#{{ log.task_id }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.channelCol')">
            <span class="name-cell">{{ channelName }}</span>
            <span class="mono name-id">#{{ log.channel_id }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.category')">
            <el-tag effect="plain" size="small" class="category-tag">{{ log.category || 'default' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('common.status')">
            <el-tag
              :type="log.status === 'success' ? 'success' : 'danger'"
              effect="light"
              size="small"
            >
              {{ log.status === 'success' ? t('common.success') : t('common.failed') }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.retryCountLabel')">
            <span class="mono">{{ log.retry_count ?? 0 }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.triggerTypeLabel')">
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
          <el-descriptions-item :label="t('logs.triggerByLabel')">
            <span class="mono">{{ log.trigger_by || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.triggerIpLabel')">
            <span class="mono">{{ log.trigger_ip || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('logs.errorMsgLabel')" :span="2">
            <pre v-if="log.error_msg" class="desc-error mono">{{ log.error_msg }}</pre>
            <span v-else class="faint">—</span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="log.content" class="detail-block">
          <span class="detail-label">{{ t('logs.contentLabel') }}</span>
          <div class="content-panel">
            <MarkdownPreview :content="log.content" />
          </div>
        </div>

        <div v-if="log.request" class="detail-block">
          <span class="detail-label">{{ t('logs.requestLabel') }}</span>
          <pre class="detail-code mono">{{ prettyJson(log.request) }}</pre>
        </div>

        <div v-if="log.response" class="detail-block">
          <span class="detail-label">{{ t('logs.responseLabel') }}</span>
          <pre class="detail-code mono">{{ prettyJson(log.response) }}</pre>
        </div>
      </template>

      <el-empty v-else-if="!loading" :description="t('logs.notFound')" class="detail-empty" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { channelApi, logApi, taskApi } from '@/api'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

const { t } = useI18n()

interface LogDetail {
  id: number
  task_id: number
  channel_id: number
  category?: string
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
  return tasks.value.find((x) => x.id === log.value!.task_id)?.name || t('logs.taskFallback', { id: log.value!.task_id })
})
const channelName = computed(() => {
  if (!log.value) return ''
  return channels.value.find((x) => x.id === log.value!.channel_id)?.name || t('logs.channelFallback', { id: log.value!.channel_id })
})

// 触发方式 → 本地化标签 / 标签配色（与列表页一致）
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
      ElMessage.error(e?.response?.data?.error || t('logs.loadFailedDetail'))
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
    ElMessage.success(t('logs.retriedOk'))
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || t('logs.retryFailed'))
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
.category-tag {
  color: var(--indigo-400) !important;
  border-color: rgba(129, 140, 248, 0.4) !important;
  background: rgba(129, 140, 248, 0.12) !important;
  overflow: hidden;
  text-overflow: ellipsis;
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
