<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="title-row">
          <h1 class="grad-text">任务管理</h1>
          <el-tag v-if="!isAdmin" type="info" effect="plain" size="small">只读模式</el-tag>
        </div>
        <p class="sub">配置定时 / Webhook 投递任务，绑定渠道与模板</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          placeholder="搜索名称或渠道 / 模板…"
        />
        <el-button
          v-if="isAdmin"
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selectedRows.length"
          @click="batchDelete"
        >
          批量删除
        </el-button>
        <el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openCreate">新建任务</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="filteredTasks"
        style="width: 100%"
        empty-text="暂无任务，点击右上角「新建任务」开始"
        @selection-change="onSelectionChange"
      >
        <el-table-column v-if="isAdmin" type="selection" width="48" align="center" />
        <el-table-column prop="id" label="ID" width="64" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="名称" min-width="150">
          <template #default="{ row }">
            <span class="task-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column label="触发" width="170">
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

        <el-table-column label="接收地址" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono receivers-cell">{{ (row.receivers || []).join(', ') || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="96" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="togglingId === row.id"
              :disabled="!isAdmin"
              inline-prompt
              active-text="开"
              inactive-text="关"
              @change="(v: boolean) => toggleTask(row, v)"
            />
          </template>
        </el-table-column>

        <el-table-column label="操作" width="230" align="center" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.trigger_type === 'api'"
              link
              type="primary"
              size="small"
              @click="showApiKey(row)"
            >
              API Key
            </el-button>
            <el-button link type="primary" size="small" @click="goLogs(row)">日志</el-button>
            <template v-if="isAdmin">
              <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
              <el-button link type="danger" size="small" @click="removeTask(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- ── Create / Edit dialog ─────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? '编辑任务' : '新建任务'"
      width="620px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="给任务起个易记的名字" />
        </el-form-item>

        <div class="form-row">
          <el-form-item label="投递渠道" prop="channel_id" class="grow">
            <el-select v-model="form.channel_id" placeholder="选择渠道" style="width: 100%">
              <el-option
                v-for="ch in channels"
                :key="ch.id"
                :label="ch.name"
                :value="ch.id"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="通知模板" prop="template_id" class="grow">
            <el-select
              v-model="form.template_id"
              placeholder="选择模板"
              style="width: 100%"
              @change="onTemplateChange"
            >
              <el-option
                v-for="t in templates"
                :key="t.id"
                :label="t.name"
                :value="t.id"
              />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item label="触发方式" prop="trigger_type">
          <el-radio-group v-model="form.trigger_type">
            <el-radio-button value="cron">定时（Cron）</el-radio-button>
            <el-radio-button value="api">Webhook API</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="form.trigger_type === 'cron'" label="Cron 表达式" prop="cron_expr">
          <el-input v-model="form.cron_expr" placeholder="例如：0 */30 * * * *" class="mono" />
        </el-form-item>

        <el-form-item v-if="showReceivers" label="接收地址" prop="receivers">
          <el-input
            v-model="form.receivers"
            type="textarea"
            :rows="4"
            :placeholder="RECEIVER_PLACEHOLDER"
          />
          <div class="field-hint mono">{{ RECEIVER_HINT }}</div>
        </el-form-item>

        <div v-else-if="nonEmailChannel" class="receiver-note">
          当前渠道为 <b class="receiver-note-strong">{{ channelTypeLabel }}</b>，
          消息将发送到机器人 / token 绑定的目标
        </div>

        <el-form-item v-if="form.trigger_type === 'api'" label="IP 白名单（可选）">
          <el-input
            v-model="form.allowed_ips"
            type="textarea"
            :rows="3"
            placeholder="每行一个 IP 或 CIDR，留空表示不限制"
          />
        </el-form-item>

        <el-form-item v-if="selectedTemplateVariables.length" label="模板变量">
          <div class="tpl-vars-list">
            <div v-for="v in selectedTemplateVariables" :key="v.name" class="tpl-var-item">
              <div class="tpl-var-meta">
                <span class="tpl-var-name mono">{{ v.name }}</span>
                <span v-if="v.description" class="tpl-var-desc">{{ v.description }}</span>
              </div>
              <el-input
                :model-value="form.variables[v.name] ?? v.default ?? ''"
                :placeholder="v.default ? `默认：${v.default}` : '发送时替换为实际值'"
                @update:model-value="setVariable(v.name, $event)"
              />
            </div>
          </div>
        </el-form-item>

        <el-form-item label="状态">
          <div class="enabled-row">
            <el-switch v-model="form.enabled" inline-prompt active-text="启用" inactive-text="停用" />
            <span class="enabled-hint">停用后任务不再触发投递</span>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveTask">
            {{ form.id ? '保存' : '创建' }}
          </el-button>
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
      <p class="api-key-desc">POST /api/webhook/&lt;api_key&gt;（把 api_key 放在 URL 路径中，无需登录，无需请求头）。</p>
      <div class="api-key-box">
        <code class="mono api-key-value">{{ apiKeyValue || '—' }}</code>
        <el-button size="small" type="primary" :icon="CopyDocument" @click="copyApiKey">
          复制
        </el-button>
      </div>
      <p class="api-key-endpoint mono">POST /api/webhook/{{ apiKeyValue || '&lt;api_key&gt;' }}</p>
      <pre class="api-key-curl mono">curl -X POST https://your-host/api/webhook/{{ apiKeyValue || '&lt;api_key&gt;' }} \
  -H 'Content-Type: application/json' \
  -d '{"variables":{"name":"张三"}}'</pre>
      <template #footer>
        <el-button @click="apiKeyVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, CopyDocument, Search, Delete } from '@element-plus/icons-vue'
import { channelApi, taskApi, templateApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

interface TaskRow {
  id: number
  name: string
  channel_id: number
  template_id: number
  trigger_type: 'cron' | 'api'
  receivers: string[]
  cron_expr: string
  api_key?: string
  allowed_ips: string[]
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
    const chName = channels.value.find((c) => c.id === t.channel_id)?.name || ''
    const tplName = templates.value.find((p) => p.id === t.template_id)?.name || ''
    return (
      (t.name || '').toLowerCase().includes(kw) ||
      chName.toLowerCase().includes(kw) ||
      tplName.toLowerCase().includes(kw)
    )
  })
})

const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  channel_id: number | null
  template_id: number | null
  trigger_type: 'cron' | 'api'
  cron_expr: string
  receivers: string
  allowed_ips: string
  variables: Record<string, string>
  enabled: boolean
}>({
  id: 0,
  name: '',
  channel_id: null,
  template_id: null,
  trigger_type: 'cron',
  cron_expr: '',
  receivers: '',
  allowed_ips: '',
  variables: {},
  enabled: true,
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  channel_id: [{ required: true, message: '请选择投递渠道', trigger: 'change' }],
  template_id: [{ required: true, message: '请选择通知模板', trigger: 'change' }],
  trigger_type: [{ required: true, message: '请选择触发方式', trigger: 'change' }],
  cron_expr: [
    {
      validator: (_rule: any, value: string, cb: any) => {
        if (form.trigger_type === 'cron' && !value.trim()) cb(new Error('请输入 Cron 表达式'))
        else cb()
      },
      trigger: 'blur',
    },
  ],
  receivers: [
    {
      validator: (_rule: any, value: string, cb: any) => {
        if (showReceivers.value && !value.trim()) cb(new Error('请至少填写一个接收地址'))
        else cb()
      },
      trigger: 'blur',
    },
  ],
}

const RECEIVER_PLACEHOLDER = '每行一个接收地址，例如：\nuser@example.com\nalert@example.com'
const RECEIVER_HINT = '支持 {{变量}}，例如 {{email}} 会在发送时被替换'

/* ── Receiver field: only meaningful for the email channel ────────────
   渠道类型 → 显示名（与 Channels.vue 保持一致）。 */
const CHANNEL_TYPE_LABELS: Record<string, string> = {
  email: 'SMTP 邮件',
  wecom: '企业微信',
  dingtalk: '钉钉',
  feishu: '飞书',
  wechat: 'PushPlus',
}

const selectedChannel = computed(() =>
  channels.value.find((c) => c.id === form.channel_id)
)
const nonEmailChannel = computed(
  () => !!selectedChannel.value && selectedChannel.value.type !== 'email'
)
// 未选择渠道时默认展示接收地址；选定非邮件渠道时隐藏，仅展示渠道提示
const showReceivers = computed(
  () => !selectedChannel.value || selectedChannel.value.type === 'email'
)
const channelTypeLabel = computed(
  () =>
    (selectedChannel.value &&
      (CHANNEL_TYPE_LABELS[selectedChannel.value.type] || selectedChannel.value.type)) ||
    ''
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

/* ── API Key dialog ────────────────────────────────────────────────── */
const apiKeyVisible = ref(false)
const apiKeyValue = ref('')
const apiKeyTaskId = ref<number | null>(null)

function showApiKey(row: TaskRow) {
  apiKeyTaskId.value = row.id
  apiKeyValue.value = row.api_key || ''
  apiKeyVisible.value = true
}

async function copyApiKey() {
  const key = apiKeyValue.value
  if (!key) return
  try {
    await navigator.clipboard.writeText(key)
    ElMessage.success('API Key 已复制')
  } catch {
    // Clipboard API may be blocked (non-secure context) — select manually.
    ElMessage.warning('复制失败，请手动选择复制')
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
    ElMessage.error(errMsg(e, '任务列表加载失败'))
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
    ElMessage.error(errMsg(e, '渠道 / 模板列表加载失败'))
  }
}

/* ── Toggle ────────────────────────────────────────────────────────── */
async function toggleTask(row: TaskRow, enabled: boolean) {
  togglingId.value = row.id
  try {
    await taskApi.toggle(row.id, enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? '任务已启用' : '任务已停用')
  } catch (e: any) {
    ElMessage.error(errMsg(e, '状态切换失败'))
  } finally {
    togglingId.value = null
  }
}

/* ── Create / edit ─────────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.channel_id = null
  form.template_id = null
  form.trigger_type = 'cron'
  form.cron_expr = ''
  form.receivers = ''
  form.allowed_ips = ''
  form.variables = {}
  form.enabled = true
  dialogVisible.value = true
}

function openEdit(row: TaskRow) {
  form.id = row.id
  form.name = row.name
  form.channel_id = row.channel_id
  form.template_id = row.template_id
  form.trigger_type = row.trigger_type
  form.cron_expr = row.cron_expr || ''
  form.receivers = (row.receivers || []).join('\n')
  form.allowed_ips = (row.allowed_ips || []).join('\n')
  form.variables = row.variables ? { ...row.variables } : {}
  form.enabled = row.enabled
  dialogVisible.value = true
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
      channel_id: form.channel_id,
      template_id: form.template_id,
      trigger_type: form.trigger_type,
      cron_expr: form.trigger_type === 'cron' ? form.cron_expr.trim() : '',
      receivers: splitLines(form.receivers),
      allowed_ips: form.trigger_type === 'api' ? splitLines(form.allowed_ips) : [],
      variables: form.variables,
      enabled: form.enabled,
    }

    if (form.id) {
      await taskApi.update(form.id, payload)
      ElMessage.success('任务已更新')
    } else {
      const created = await taskApi.create(payload)
      ElMessage.success('任务已创建')
      // API 任务创建后后端自动生成 api_key，若有则提示
      if (form.trigger_type === 'api' && created?.api_key) {
        ElMessage.success('已生成 Webhook API Key，可在任务列表查看')
      }
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '保存失败'))
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
      `确定删除任务「${row.name}」吗？删除后不可恢复。`,
      '删除任务',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await taskApi.remove(row.id)
    ElMessage.success('任务已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

/* ── Batch delete ─────────────────────────────────────────────────── */
async function batchDelete() {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    await ElMessageBox.confirm(
      `确认删除选中的 ${rows.length} 项？`,
      '批量删除',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await taskApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(`已删除 ${rows.length} 个任务`)
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '批量删除失败'))
  }
}

onMounted(() => {
  load()
  loadOptions()
})
</script>

<style scoped>
.search-input { width: 220px; }

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
