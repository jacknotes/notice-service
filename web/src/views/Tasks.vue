<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="title-row">
          <h1 class="grad-text">{{ t('nav.tasks') }}</h1>
          <el-tag v-if="!isAdmin" type="info" effect="plain" size="small">{{ t('common.readOnlyMode') }}</el-tag>
        </div>
        <p class="sub">{{ t('tasks.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          :placeholder="t('tasks.searchPlaceholder')"
        />
        <el-button
          v-if="isAdmin"
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selectedRows.length"
          @click="batchDelete"
        >
          {{ t('common.batchDelete') }}
        </el-button>
        <el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openCreate">{{ t('tasks.createTitle') }}</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="paged"
        style="width: 100%"
        :empty-text="t('tasks.emptyTable')"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column v-if="isAdmin" type="selection" width="48" align="center" />
        <el-table-column prop="id" label="ID" width="64" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" :label="t('common.name')" min-width="150" sortable="custom">
          <template #default="{ row }">
            <span class="task-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('tasks.trigger')" width="170">
          <template #default="{ row }">
            <el-tag
              v-if="row.trigger_type === 'api'"
              :style="webhookTagStyle"
              effect="plain"
              size="small"
            >
              Webhook API
            </el-tag>
            <el-tag v-else effect="plain" size="small" type="primary">
              <span class="mono cron-tag">{{ row.cron_expr || '—' }}</span>
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('tasks.channels')" min-width="170">
          <template #default="{ row }">
            <span class="channels-cell">{{ channelNames(row) || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('tasks.receivers')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono receivers-cell">{{ (row.receivers || []).join(', ') || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.status')" width="96" align="center" sortable="custom" prop="enabled">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="togglingId === row.id"
              :disabled="!isAdmin"
              inline-prompt
              :active-text="t('tasks.on')"
              :inactive-text="t('tasks.off')"
              @change="(v: boolean) => toggleTask(row, v)"
            />
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="340" align="center" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="isAdmin && row.trigger_type === 'api'"
              link
              type="primary"
              size="small"
              @click="showApiKey(row)"
            >
              API Key
            </el-button>
            <el-button
              v-if="isAdmin"
              link
              type="warning"
              size="small"
              :loading="sendingId === row.id"
              @click="sendNow(row)"
            >
              {{ t('tasks.sendNow') }}
            </el-button>
            <el-button link type="primary" size="small" @click="goLogs(row)">{{ t('tasks.logsAction') }}</el-button>
            <template v-if="isAdmin">
              <el-button link type="primary" size="small" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
              <el-button link type="success" size="small" @click="duplicateTask(row)">{{ t('tasks.duplicateAction') }}</el-button>
              <el-button link type="danger" size="small" @click="removeTask(row)">{{ t('common.delete') }}</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div v-if="total > 0" class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @size-change="onPageSizeChange"
      />
    </div>

    <!-- ── Create / Edit dialog ─────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? t('tasks.editTitle') : t('tasks.createTitle')"
      width="620px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item :label="t('common.name')" prop="name">
          <el-input v-model="form.name" :placeholder="t('tasks.namePlaceholder')" />
        </el-form-item>

        <div class="form-row">
          <el-form-item :label="t('tasks.channels')" prop="channel_ids" class="grow">
            <el-select
              v-model="form.channel_ids"
              multiple
              collapse-tags
              collapse-tags-tooltip
              :placeholder="t('tasks.channelSelectPlaceholder')"
              style="width: 100%"
            >
              <el-option
                v-for="ch in channels"
                :key="ch.id"
                :label="ch.name"
                :value="ch.id"
              />
            </el-select>
          </el-form-item>

          <el-form-item :label="t('tasks.template')" prop="template_id" class="grow">
            <el-select
              v-model="form.template_id"
              :placeholder="t('tasks.templateSelectPlaceholder')"
              style="width: 100%"
              @change="onTemplateChange"
            >
              <el-option
                v-for="tpl in templates"
                :key="tpl.id"
                :label="tpl.name"
                :value="tpl.id"
              />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item :label="t('tasks.triggerType')" prop="trigger_type">
          <el-radio-group v-model="form.trigger_type">
            <el-radio-button value="cron">{{ t('tasks.triggerCron') }}</el-radio-button>
            <el-radio-button value="api">Webhook API</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="form.trigger_type === 'cron'" :label="t('tasks.cronExpr')" prop="cron_expr">
          <el-input v-model="form.cron_expr" :placeholder="t('tasks.cronExprPlaceholder')" class="mono" />
        </el-form-item>

        <el-form-item v-if="showReceivers" :label="t('tasks.receivers')" prop="receivers">
          <el-input
            v-model="form.receivers"
            type="textarea"
            :rows="4"
            :placeholder="t('tasks.receiversPlaceholder')"
          />
          <div class="field-hint mono">{{ t('tasks.receiverVarHint') }}</div>
        </el-form-item>

        <div v-else-if="nonEmailChannel" class="receiver-note">
          {{ t('tasks.nonEmailPrefix') }}<b class="receiver-note-strong">{{ channelTypeLabel }}</b>{{ t('tasks.nonEmailSuffix') }}
        </div>

        <el-form-item v-if="form.trigger_type === 'api'" :label="t('tasks.ipWhitelist')">
          <el-input
            v-model="form.allowed_ips"
            type="textarea"
            :rows="3"
            :placeholder="t('tasks.ipWhitelistPlaceholder')"
          />
        </el-form-item>

        <el-form-item v-if="form.trigger_type === 'api'" :label="t('tasks.hmacTitle')">
          <div class="signature-row">
            <el-switch v-model="form.require_signature" :aria-label="t('tasks.requireHmac')" />
            <span class="signature-label">{{ t('tasks.requireHmac') }}</span>
          </div>
          <el-alert
            v-if="form.require_signature"
            class="signature-alert"
            type="info"
            :closable="false"
            show-icon
          >
            <pre class="signature-pre mono">{{ t('tasks.hmacPre') }}</pre>
            <p class="signature-note">
              {{ t('tasks.hmacNote') }}
            </p>
          </el-alert>
        </el-form-item>

        <el-form-item v-if="selectedTemplateVariables.length" :label="t('tasks.templateVars')">
          <div class="tpl-vars-list">
            <div v-for="v in selectedTemplateVariables" :key="v.name" class="tpl-var-item">
              <div class="tpl-var-meta">
                <span class="tpl-var-name mono">{{ v.name }}</span>
                <span v-if="v.description" class="tpl-var-desc">{{ v.description }}</span>
              </div>
              <el-input
                :model-value="form.variables[v.name] ?? v.default ?? ''"
                :placeholder="v.default ? `${t('tasks.varDefaultPrefix')}${v.default}` : t('tasks.varRuntimeReplace')"
                @update:model-value="setVariable(v.name, $event)"
              />
            </div>
          </div>
        </el-form-item>

        <el-form-item :label="t('common.status')">
          <div class="enabled-row">
            <el-switch v-model="form.enabled" inline-prompt :active-text="t('common.enabled')" :inactive-text="t('common.disabled')" />
            <span class="enabled-hint">{{ t('tasks.enabledHint') }}</span>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <el-button :icon="View" :loading="previewing" @click="openPreview">
            {{ t('tasks.previewAction') }}
          </el-button>
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveTask">
            {{ form.id ? t('common.save') : t('common.create') }}
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── 发送预览 dialog：渲染最终效果（不落库、不发送） ───────────── -->
    <el-dialog
      v-model="previewVisible"
      :title="t('tasks.previewTitle')"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div v-loading="previewing" class="preview-panel">
        <template v-if="previewData">
          <div class="preview-block">
            <span class="preview-label">{{ t('tasks.subject') }}</span>
            <p class="preview-subject">{{ previewData.subject || '—' }}</p>
          </div>
          <div class="preview-block">
            <span class="preview-label">{{ t('tasks.content') }}</span>
            <div class="preview-md">
              <MarkdownPreview :content="previewData.content" />
            </div>
          </div>
          <div class="preview-block">
            <span class="preview-label">{{ t('tasks.receiversReplaced') }}</span>
            <div v-if="previewData.receivers.length" class="preview-receivers">
              <el-tag
                v-for="(r, i) in previewData.receivers"
                :key="i"
                effect="plain"
                size="small"
                class="receiver-tag"
              >
                {{ r }}
              </el-tag>
            </div>
            <span v-else class="preview-empty">—</span>
          </div>
        </template>
        <div v-else class="preview-hint">{{ t('tasks.previewHint') }}</div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="previewVisible = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- ── API Key dialog ───────────────────────────────────────────── -->
    <el-dialog
      v-model="apiKeyVisible"
      title="Webhook API Key"
      width="520px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <p class="api-key-desc">{{ t('tasks.apiKeyDesc') }}</p>
      <div class="api-key-box">
        <code class="mono api-key-value">{{ apiKeyValue || '—' }}</code>
        <el-button size="small" type="primary" :icon="CopyDocument" @click="copyCredential(apiKeyValue, 'tasks.apiKeyCopied')">
          {{ t('common.copy') }}
        </el-button>
      </div>
      <div class="api-key-box">
        <code class="mono api-key-value">{{ hmacValue || '—' }}</code>
        <el-button size="small" type="primary" :icon="CopyDocument" @click="copyCredential(hmacValue, 'common.copied')">
          {{ t('tasks.copySignatureBtn') }}
        </el-button>
      </div>
      <p class="api-key-desc">{{ t('tasks.signatureSecretHint') }}</p>
      <p class="api-key-endpoint mono">POST /api/webhook/{{ apiKeyValue || '&lt;api_key&gt;' }}</p>
      <pre class="api-key-curl mono">curl -X POST https://your-host/api/webhook/{{ apiKeyValue || '&lt;api_key&gt;' }} \
  -H 'Content-Type: application/json' \
  -d '{"variables":{"name":"张三"}}'</pre>
      <template #footer>
        <el-button @click="apiKeyVisible = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, CopyDocument, Search, Delete, View } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { channelApi, taskApi, templateApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useTablePaging } from '@/composables/useTablePaging'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

const { t } = useI18n()

interface TaskRow {
  id: number
  name: string
  channel_id: number
  channel_ids?: number[]
  template_id: number
  trigger_type: 'cron' | 'api'
  receivers: string[]
  cron_expr: string
  api_key?: string
  hmac_secret?: string
  allowed_ips: string[]
  require_signature?: boolean
  variables?: Record<string, string>
  enabled: boolean
  created_at?: string
  updated_at?: string
}

interface TemplateVar {
  name: string
  type?: string
  description?: string
  default?: string
}

const router = useRouter()

const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')

const loading = ref(false)
const tasks = ref<TaskRow[]>([])
const channels = ref<{ id: number; name: string; type: string }[]>([])
const templates = ref<{ id: number; name: string; variables: TemplateVar[] }[]>([])
const togglingId = ref<number | null>(null)
const sendingId = ref<number | null>(null)
const tableRef = ref<TableInstance>()
const selectedRows = ref<TaskRow[]>([])

function onSelectionChange(rows: TaskRow[]) {
  selectedRows.value = rows
}

const keyword = ref('')

// 按任务名称、或绑定的渠道 / 模板名称做客户端过滤
const filteredTasks = computed<TaskRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return tasks.value
  return tasks.value.filter((t) => {
    const ids = t.channel_ids?.length ? t.channel_ids : [t.channel_id]
    const chName = ids.map((id) => channels.value.find((c) => c.id === id)?.name || '').join(' ')
    const tplName = templates.value.find((p) => p.id === t.template_id)?.name || ''
    return (
      (t.name || '').toLowerCase().includes(kw) ||
      chName.toLowerCase().includes(kw) ||
      tplName.toLowerCase().includes(kw)
    )
  })
})

// 客户端排序 + 分页（整表数据在前端）
const { page, size, onSortChange, paged, total, onPageSizeChange } = useTablePaging<TaskRow>(filteredTasks)

const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  channel_ids: number[]
  template_id: number | null
  trigger_type: 'cron' | 'api'
  cron_expr: string
  receivers: string
  allowed_ips: string
  require_signature: boolean
  variables: Record<string, string>
  enabled: boolean
}>({
  id: 0,
  name: '',
  channel_ids: [],
  template_id: null,
  trigger_type: 'cron',
  cron_expr: '',
  receivers: '',
  allowed_ips: '',
  require_signature: false,
  variables: {},
  enabled: true,
})

// 校验消息随语言切换（与 Login 的 computed 规则同一约定）
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('tasks.nameRequired'), trigger: 'blur' }],
  channel_ids: [
    {
      validator: (_rule: any, value: number[], cb: any) => {
        if (!value || !value.length) cb(new Error(t('tasks.channelRequired')))
        else cb()
      },
      trigger: 'change',
    },
  ],
  template_id: [{ required: true, message: t('tasks.templateRequired'), trigger: 'change' }],
  trigger_type: [{ required: true, message: t('tasks.triggerRequired'), trigger: 'change' }],
  cron_expr: [
    {
      required: true,
      validator: (_rule: any, value: string, cb: any) => {
        if (form.trigger_type === 'cron' && !value.trim()) cb(new Error(t('tasks.cronRequired')))
        else cb()
      },
      trigger: 'blur',
    },
  ],
  receivers: [
    {
      required: true,
      validator: (_rule: any, value: string, cb: any) => {
        if (showReceivers.value && !value.trim()) cb(new Error(t('tasks.receiverRequired')))
        else cb()
      },
      trigger: 'blur',
    },
  ],
}))

/* ── Receiver field: only meaningful for the email channel ────────────
   渠道类型 → 显示名 key（与 Channels.vue 的 channels.type.* 保持一致）。 */
const TYPE_KEY: Record<string, string> = {
  email: 'channels.type.email',
  wecom: 'channels.type.wecom',
  dingtalk: 'channels.type.dingtalk',
  feishu: 'channels.type.feishu',
  wechat: 'channels.type.wechat',
}

const selectedChannels = computed(() =>
  channels.value.filter((c) => form.channel_ids.includes(c.id))
)
const anyEmailChannel = computed(() =>
  selectedChannels.value.some((c) => c.type === 'email')
)
// 仅当所选渠道均为非邮件渠道时才展示渠道提示；有邮件渠道（或未选）时展示接收地址
const nonEmailChannel = computed(
  () => selectedChannels.value.length > 0 && selectedChannels.value.every((c) => c.type !== 'email')
)
const showReceivers = computed(
  () => selectedChannels.value.length === 0 || anyEmailChannel.value
)
const channelTypeLabel = computed(() =>
  selectedChannels.value
    .map((c) => {
      const key = TYPE_KEY[c.type]
      return key ? t(key) : c.type
    })
    .join(t('tasks.listJoin'))
)

const selectedTemplate = computed(() =>
  templates.value.find((t) => t.id === form.template_id)
)
const selectedTemplateVariables = computed<TemplateVar[]>(
  () => selectedTemplate.value?.variables || []
)

const webhookTagStyle = {
  color: 'var(--violet-400)',
  borderColor: 'rgba(139, 92, 246, 0.4)',
  backgroundColor: 'rgba(139, 92, 246, 0.14)',
}

// 任务绑定的渠道名（多选时按语言分隔）
function channelNames(row: TaskRow): string {
  const ids = row.channel_ids?.length ? row.channel_ids : [row.channel_id]
  return ids
    .map((id) => channels.value.find((c) => c.id === id)?.name || '')
    .filter(Boolean)
    .join(t('tasks.listJoin'))
}

/* ── API Key dialog ────────────────────────────────────────────────── */
const apiKeyVisible = ref(false)
const apiKeyValue = ref('')
const hmacValue = ref('')
const apiKeyTaskId = ref<number | null>(null)

function showApiKey(row: TaskRow) {
  apiKeyTaskId.value = row.id
  apiKeyValue.value = row.api_key || ''
  hmacValue.value = row.hmac_secret || ''
  apiKeyVisible.value = true
}

async function copyCredential(value: string, okMsgKey: string) {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    ElMessage.success(t(okMsgKey))
  } catch {
    // Clipboard API may be blocked (non-secure context) — select manually.
    ElMessage.warning(t('common.copyFailed'))
  }
}

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    tasks.value = await taskApi.list()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('tasks.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadOptions() {
  try {
    const [chs, tpls] = await Promise.all([channelApi.list(), templateApi.list()])
    channels.value = chs || []
    templates.value = tpls || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('tasks.optionsLoadFailed')))
  }
}

/* ── Toggle ────────────────────────────────────────────────────────── */
async function toggleTask(row: TaskRow, enabled: boolean) {
  togglingId.value = row.id
  try {
    await taskApi.toggle(row.id, enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? t('tasks.enabledOk') : t('tasks.disabledOk'))
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('tasks.toggleFailed')))
  } finally {
    togglingId.value = null
  }
}

/* ── 立即发送 ─────────────────────────────────────────────────────── */
async function sendNow(row: TaskRow) {
  try {
    await ElMessageBox.confirm(
      t('tasks.sendNowConfirmMsg', { name: row.name, channels: channelNames(row) || t('tasks.channelWord') }),
      t('tasks.sendNowConfirmTitle'),
      { confirmButtonText: t('tasks.sendBtn'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  sendingId.value = row.id
  try {
    await taskApi.sendNow(row.id)
    ElMessage.success(t('tasks.queuedOk'))
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('tasks.sendFailed')))
  } finally {
    sendingId.value = null
  }
}

/* ── Create / edit ─────────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.channel_ids = []
  form.template_id = null
  form.trigger_type = 'cron'
  form.cron_expr = ''
  form.receivers = ''
  form.allowed_ips = ''
  form.require_signature = false
  form.variables = {}
  form.enabled = true
  dialogVisible.value = true
}

function openEdit(row: TaskRow) {
  form.id = row.id
  form.name = row.name
  form.channel_ids = row.channel_ids?.length ? [...row.channel_ids] : [row.channel_id]
  form.template_id = row.template_id
  form.trigger_type = row.trigger_type
  form.cron_expr = row.cron_expr || ''
  form.receivers = (row.receivers || []).join('\n')
  form.allowed_ips = (row.allowed_ips || []).join('\n')
  form.require_signature = row.require_signature ?? false
  form.variables = row.variables ? { ...row.variables } : {}
  form.enabled = row.enabled
  dialogVisible.value = true
}

// 复制任务：打开「新建任务」并预填源任务的全部配置（名称加副本后缀），
// id=0 走创建路径；api_key 由后端创建时重新生成。
function duplicateTask(row: TaskRow) {
  form.id = 0
  form.name = `${row.name}${t('common.copySuffix')}`
  form.channel_ids = row.channel_ids?.length ? [...row.channel_ids] : [row.channel_id]
  form.template_id = row.template_id
  form.trigger_type = row.trigger_type
  form.cron_expr = row.cron_expr || ''
  form.receivers = (row.receivers || []).join('\n')
  form.allowed_ips = (row.allowed_ips || []).join('\n')
  form.require_signature = row.require_signature ?? false
  form.variables = row.variables ? { ...row.variables } : {}
  form.enabled = row.enabled
  dialogVisible.value = true
}

/* ── 发送预览（任务编辑时查看最终效果，不落库、不发送） ─────────────── */
const previewVisible = ref(false)
const previewing = ref(false)
const previewData = ref<{ subject: string; content: string; receivers: string[] } | null>(null)

async function openPreview() {
  if (!form.template_id) {
    ElMessage.warning(t('tasks.templateFirst'))
    return
  }
  previewing.value = true
  previewData.value = null
  previewVisible.value = true
  try {
    const data = await taskApi.preview({
      template_id: form.template_id,
      variables: { ...form.variables },
      receivers: splitLines(form.receivers),
    })
    previewData.value = data || { subject: '', content: '', receivers: [] }
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('tasks.previewFailed')))
  } finally {
    previewing.value = false
  }
}

/* ── Template variables editor ──────────────────────────────────────── */
// 切换模板时清空变量：不同模板的变量互不通用
function onTemplateChange() {
  form.variables = {}
}

function setVariable(name: string, value: string) {
  form.variables[name] = value
}

function splitLines(s: string): string[] {
  return s
    .split('\n')
    .map((x) => x.trim())
    .filter(Boolean)
}

/* ── Save ──────────────────────────────────────────────────────────── */
async function saveTask() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const payload = {
      name: form.name,
      channel_ids: form.channel_ids,
      template_id: form.template_id,
      trigger_type: form.trigger_type,
      cron_expr: form.trigger_type === 'cron' ? form.cron_expr.trim() : '',
      receivers: splitLines(form.receivers),
      allowed_ips: form.trigger_type === 'api' ? splitLines(form.allowed_ips) : [],
      require_signature: form.trigger_type === 'api' ? form.require_signature : false,
      variables: form.variables,
      enabled: form.enabled,
    }

    let savedId = form.id
    if (form.id) {
      await taskApi.update(form.id, payload)
      ElMessage.success(t('tasks.updatedOk'))
    } else {
      const created = await taskApi.create(payload)
      savedId = created?.id ?? 0
      ElMessage.success(t('tasks.createdOk'))
    }
    dialogVisible.value = false
    await load()
    // api 任务：保存后自动弹出 API Key 便于复制（后端负责生成/保留）
    if (form.trigger_type === 'api' && savedId) {
      const fresh = tasks.value.find((t) => t.id === savedId)
      if (fresh?.api_key) {
        apiKeyValue.value = fresh.api_key
        hmacValue.value = fresh.hmac_secret || ''
        apiKeyTaskId.value = fresh.id
        apiKeyVisible.value = true
      }
    }
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.saveFailed')))
  } finally {
    saving.value = false
  }
}

/* ── Logs / Delete ─────────────────────────────────────────────────── */
function goLogs(row: TaskRow) {
  router.push({ path: '/logs', query: { task: row.id } })
}

async function removeTask(row: TaskRow) {
  try {
    await ElMessageBox.confirm(
      t('tasks.deleteConfirmMsg', { name: row.name }),
      t('tasks.deleteConfirmTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await taskApi.remove(row.id)
    ElMessage.success(t('tasks.deletedOk'))
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.deleteFailed')))
  }
}

/* ── Batch delete ─────────────────────────────────────────────────── */
async function batchDelete() {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    await ElMessageBox.confirm(
      t('common.batchDeleteConfirmMsg', { n: rows.length }),
      t('common.batchDelete'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await taskApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(t('tasks.batchDeletedOk', { n: rows.length }))
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  }
}

onMounted(() => {
  load()
  loadOptions()
})
</script>

<style scoped>
.search-input { width: 220px; }

.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
}

/* ── 发送预览 dialog ───────────────────────────────────────────────── */
.preview-panel {
  min-height: 180px;
}
.preview-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: var(--space-4);
}
.preview-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text-faint);
}
.preview-subject {
  color: var(--text-primary);
  font-size: var(--text-md);
  font-weight: 600;
  word-break: break-word;
}
.preview-md {
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(148, 163, 184, 0.05);
  max-height: 280px;
  overflow: auto;
}
.preview-receivers {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.receiver-tag {
  color: var(--sky-400) !important;
  border-color: rgba(56, 189, 248, 0.35) !important;
  background: rgba(56, 189, 248, 0.1) !important;
}
.preview-empty {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.preview-hint {
  display: grid;
  place-items: center;
  min-height: 140px;
  color: var(--text-faint);
  font-size: var(--text-sm);
}

.title-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.table-card {
  padding: 8px 14px 14px;
  overflow: hidden;
}

.id-cell {
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.task-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
}
.cron-tag {
  font-size: 11px;
  letter-spacing: 0.01em;
}
.receivers-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.channels-cell {
  color: var(--sky-400);
  font-size: var(--text-xs);
  font-weight: 500;
}

/* ── Dialog ────────────────────────────────────────────────────────── */
.form-row {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
}
.form-row .grow { flex: 1; min-width: 0; }

.field-hint {
  width: 100%;
  margin-top: 4px;
  color: var(--text-faint);
  font-size: 11px;
}

.receiver-note {
  width: 100%;
  margin-top: 2px;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  border: 1px dashed var(--border);
  background: rgba(148, 163, 184, 0.05);
  color: var(--text-muted);
  font-size: var(--text-xs);
  line-height: 1.7;
}
.receiver-note-strong {
  color: var(--text-secondary);
  font-weight: 600;
}

/* ── Template variables editor ──────────────────────────────────────── */
.tpl-vars-list {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.tpl-var-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.tpl-var-meta {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}
.tpl-var-name {
  color: var(--indigo-400);
  font-size: var(--text-xs);
  font-weight: 600;
}
.tpl-var-desc {
  color: var(--text-faint);
  font-size: var(--text-xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.enabled-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.enabled-hint {
  color: var(--text-muted);
  font-size: var(--text-xs);
}

/* ── HMAC 签名开关与调用示例 ─────────────────────────────────────── */
.signature-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.signature-label {
  color: var(--text-secondary);
  font-size: var(--text-sm);
}
.signature-alert {
  width: 100%;
  margin-top: var(--space-3);
}
.signature-pre {
  margin: 0 0 var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(11, 17, 32, 0.72);
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.7;
  white-space: pre;
  overflow-x: auto;
}
.signature-note {
  margin: 0;
  color: var(--text-muted);
  font-size: var(--text-xs);
  line-height: 1.7;
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }

/* ── API Key ───────────────────────────────────────────────────────── */
.api-key-desc {
  margin-bottom: var(--space-3);
  color: var(--text-secondary);
  font-size: var(--text-sm);
}
.api-key-box {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(11, 17, 32, 0.72);
}
.api-key-value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--violet-400);
  font-size: var(--text-sm);
}
.api-key-endpoint {
  margin-top: var(--space-3);
  color: var(--text-faint);
  font-size: 11px;
  letter-spacing: 0.02em;
}
.api-key-curl {
  margin-top: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(11, 17, 32, 0.72);
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.6;
  white-space: pre;
  overflow-x: auto;
}

@media (max-width: 480px) {
  .form-row { flex-direction: column; }
}
</style>
