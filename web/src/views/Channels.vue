<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="title-row">
          <h1 class="grad-text">{{ t('nav.channels') }}</h1>
          <el-tag v-if="!isAdmin" type="info" effect="plain" size="small">{{ t('channels.readOnlyMode') }}</el-tag>
        </div>
        <p class="sub">{{ t('channels.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          :placeholder="t('channels.searchPlaceholder')"
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
        <el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openCreate">
          {{ t('channels.createTitle') }}
        </el-button>
      </div>
    </div>

    <div class="filter-row">
      <el-select
        v-model="categoryFilter"
        class="filter-select"
        clearable
        :placeholder="t('channels.allCategories')"
      >
        <el-option v-for="cg in categories" :key="cg.name" :label="cg.name" :value="cg.name" />
      </el-select>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="paged"
        style="width: 100%"
        highlight-current-row
        :empty-text="t('channels.emptyTable')"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column v-if="isAdmin" type="selection" width="48" align="center" />
        <el-table-column prop="id" label="ID" width="72" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" :label="t('common.name')" min-width="160" sortable="custom">
          <template #default="{ row }">
            <span class="ch-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="type" :label="t('common.type')" width="150" sortable="custom">
          <template #default="{ row }">
            <el-tag :style="typeTagStyle(row.type)" effect="plain" size="small">
              {{ typeLabel(row.type) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('channels.category')" width="130">
          <template #default="{ row }">
            <el-tag effect="plain" size="small" class="category-tag">{{ row.category || 'default' }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.status')" width="110" align="center" sortable="custom" prop="enabled">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" effect="light" size="small">
              {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('channels.createdAt')" min-width="150" sortable="custom" prop="created_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="250" align="center" fixed="right">
          <template #default="{ row }">
            <template v-if="isAdmin">
              <el-button link type="primary" size="small" :loading="testingId === row.id" @click="testChannel(row)">
                {{ t('channels.testAction') }}
              </el-button>
              <el-button link type="primary" size="small" @click="openEdit(row)">
                {{ t('common.edit') }}
              </el-button>
              <el-button link type="success" size="small" @click="duplicateChannel(row)">
                {{ t('channels.duplicateAction') }}
              </el-button>
              <el-button link type="danger" size="small" @click="removeChannel(row)">
                {{ t('common.delete') }}
              </el-button>
            </template>
            <span v-else class="text-muted">—</span>
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

    <!-- ── Create / Edit dialog ────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? t('channels.editTitle') : t('channels.createTitle')"
      width="520px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-row">
          <el-form-item :label="t('common.name')" prop="name" class="grow">
            <el-input v-model="form.name" :placeholder="t('channels.namePlaceholder')" />
          </el-form-item>

          <el-form-item :label="t('common.type')" prop="type" class="shrink">
            <el-select v-model="form.type" :placeholder="t('channels.selectType')" style="width: 160px" @change="onTypeChange">
              <el-option v-for="o in typeOptions" :key="o.value" :label="t(o.labelKey)" :value="o.value" />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item :label="t('channels.category')" prop="category">
          <el-select
            v-model="form.category"
            filterable
            :placeholder="t('channels.categoryPlaceholder')"
            style="width: 100%"
          >
            <el-option v-for="cg in categories" :key="cg.name" :label="cg.name" :value="cg.name" />
          </el-select>
        </el-form-item>

        <template v-for="field in currentFields" :key="field.key">
          <el-form-item :label="t(field.labelKey)" :prop="`config.${field.key}`">
            <el-input
              v-model="form.config[field.key]"
              :type="field.type"
              :placeholder="t(field.placeholderKey)"
              :show-password="field.type === 'password'"
              clearable
            />
          </el-form-item>
        </template>

        <el-form-item :label="t('common.status')">
          <div class="enabled-row">
            <el-switch
              v-model="form.enabled"
              inline-prompt
              :active-text="t('common.enabled')"
              :inactive-text="t('common.disabled')"
            />
            <span class="enabled-hint">{{ t('channels.enabledHint') }}</span>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <el-button :loading="testing" :icon="Promotion" @click="testForm">
            {{ t('channels.testConnection') }}
          </el-button>
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveChannel">
            {{ form.id ? t('common.save') : t('common.create') }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, Edit, Delete, Promotion, Search } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import client from '@/api/client'
import { channelApi, categoryApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useTablePaging } from '@/composables/useTablePaging'

const { t } = useI18n()

interface ChannelRow {
  id: number
  type: string
  name: string
  category?: string
  config: Record<string, string>
  enabled: boolean
  created_at?: string
}

interface ConfigField {
  key: string
  labelKey: string
  placeholderKey: string
  type: 'text' | 'password'
}

const typeOptions = [
  { value: 'email', labelKey: 'channels.type.email' },
  { value: 'wecom', labelKey: 'channels.type.wecom' },
  { value: 'dingtalk', labelKey: 'channels.type.dingtalk' },
  { value: 'feishu', labelKey: 'channels.type.feishu' },
  { value: 'wechat', labelKey: 'channels.type.wechat' },
]

const typeMeta: Record<string, { key: string; color: string }> = {
  email: { key: 'channels.type.email', color: '#818cf8' },
  wecom: { key: 'channels.type.wecom', color: '#38bdf8' },
  dingtalk: { key: 'channels.type.dingtalk', color: '#8b5cf6' },
  feishu: { key: 'channels.type.feishu', color: '#34d399' },
  wechat: { key: 'channels.type.wechat', color: '#fbbf24' },
}

const configFields: Record<string, ConfigField[]> = {
  email: [
    { key: 'host', labelKey: 'channels.fieldHost', placeholderKey: 'channels.phHost', type: 'text' },
    { key: 'port', labelKey: 'channels.fieldPort', placeholderKey: 'channels.phPort', type: 'text' },
    { key: 'username', labelKey: 'channels.fieldUsername', placeholderKey: 'channels.phEmailAccount', type: 'text' },
    { key: 'password', labelKey: 'channels.fieldPassword', placeholderKey: 'channels.phSmtpSecret', type: 'password' },
    { key: 'from', labelKey: 'channels.fieldFrom', placeholderKey: 'channels.phFrom', type: 'text' },
  ],
  wecom: [
    { key: 'webhook_url', labelKey: 'channels.fieldWebhook', placeholderKey: 'channels.phWebhookWecom', type: 'text' },
  ],
  dingtalk: [
    { key: 'webhook_url', labelKey: 'channels.fieldWebhook', placeholderKey: 'channels.phWebhookDingtalk', type: 'text' },
    { key: 'secret', labelKey: 'channels.fieldSecret', placeholderKey: 'channels.phSecret', type: 'text' },
  ],
  feishu: [
    { key: 'webhook_url', labelKey: 'channels.fieldWebhook', placeholderKey: 'channels.phWebhookFeishu', type: 'text' },
  ],
  wechat: [
    { key: 'pushplus_token', labelKey: 'channels.fieldPpToken', placeholderKey: 'channels.phPpToken', type: 'password' },
    { key: 'pushplus_topic', labelKey: 'channels.fieldPpTopic', placeholderKey: 'channels.phPpTopic', type: 'text' },
  ],
}

const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')
const route = useRoute()

const loading = ref(false)
const channels = ref<ChannelRow[]>([])
const testingId = ref<number | null>(null)
const tableRef = ref<TableInstance>()
const selectedRows = ref<ChannelRow[]>([])

function onSelectionChange(rows: ChannelRow[]) {
  selectedRows.value = rows
}

const keyword = ref('')

// 分类筛选（客户端）
const categoryFilter = ref<string>('')

// 共享分类池（渠道/模板/任务统一引用）
const categories = ref<{ id: number; name: string }[]>([])

// 按名称、类型（原始值 / 本地化标签）或分类做客户端过滤
const filteredChannels = computed<ChannelRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  return channels.value.filter((c) => {
    if (categoryFilter.value && (c.category || 'default') !== categoryFilter.value) return false
    if (!kw) return true
    return (
      (c.name || '').toLowerCase().includes(kw) ||
      (c.type || '').toLowerCase().includes(kw) ||
      (typeLabel(c.type) || '').toLowerCase().includes(kw)
    )
  })
})

// 客户端排序 + 分页（整表数据在前端）
const { page, size, onSortChange, paged, total, onPageSizeChange } = useTablePaging<ChannelRow>(filteredChannels)

const dialogVisible = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  type: string
  category: string
  enabled: boolean
  config: Record<string, string>
}>({
  id: 0,
  name: '',
  type: 'email',
  category: 'default',
  enabled: true,
  config: {},
})

// 校验消息随语言切换（与 Login 的 computed 规则同一约定）
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('channels.nameRequired'), trigger: 'blur' }],
  type: [{ required: true, message: t('channels.typeRequired'), trigger: 'change' }],
}))

const currentFields = computed<ConfigField[]>(() => configFields[form.type] || [])

function typeLabel(type: string) {
  const m = typeMeta[type]
  return m ? t(m.key) : type
}

function typeTagStyle(type: string) {
  const c = typeMeta[type]?.color || '#94a3b8'
  return { color: c, borderColor: `${c}55`, backgroundColor: `${c}1a` }
}

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    channels.value = await channelApi.list()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('channels.loadFailed')))
  } finally {
    loading.value = false
  }
}

// 共享分类池（渠道/模板/任务统一引用）：只在「分类管理」一处创建
async function loadCategories() {
  try {
    categories.value = (await categoryApi.list()) || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('channels.loadFailed')))
  }
}

/* ── 外部跳转高亮（发送日志 → 渠道管理 → ?edit=<id>） ──────────────── */
const highlightId = ref<number | null>(null)

function highlightById(id: number) {
  const row = channels.value.find((c) => c.id === id)
  if (!row) return
  highlightId.value = id
  // 行可能跨页：切到含该行的页再 setCurrentRow
  const idx = channels.value.findIndex((c) => c.id === id)
  const targetPage = Math.floor(idx / size.value) + 1
  if (targetPage !== page.value) page.value = targetPage
  nextTick(() => tableRef.value?.setCurrentRow(row))
}

onMounted(() => {
  load()
  loadCategories()
  const id = Number(route.query.edit)
  if (Number.isInteger(id) && id > 0) highlightId.value = id
})

// 渠道数据就绪后执行高亮（数据可能晚于 onMounted 返回）
watch(
  () => channels.value,
  (list) => {
    if (highlightId.value && list.length) {
      highlightById(highlightId.value)
      highlightId.value = null
    }
  },
  { immediate: true }
)

/* ── Create / edit ───────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.type = 'email'
  form.category = 'default'
  form.enabled = true
  form.config = {}
  dialogVisible.value = true
}

function openEdit(row: ChannelRow) {
  form.id = row.id
  form.name = row.name
  form.type = row.type
  form.category = row.category || 'default'
  form.enabled = row.enabled
  form.config = { ...(row.config || {}) }
  dialogVisible.value = true
}

// 复制渠道：打开「新建渠道」并预填源渠道配置（名称加副本后缀），id=0 走创建路径。
function duplicateChannel(row: ChannelRow) {
  form.id = 0
  form.name = `${row.name}${t('channels.copySuffix')}`
  form.type = row.type
  form.category = row.category || 'default'
  form.enabled = row.enabled
  form.config = { ...(row.config || {}) }
  dialogVisible.value = true
}

function onTypeChange() {
  form.config = {}
  formRef.value?.clearValidate()
}

async function saveChannel() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const payload = { type: form.type, name: form.name, category: form.category || 'default', config: form.config, enabled: form.enabled }
    if (form.id) {
      await channelApi.update(form.id, payload)
      ElMessage.success(t('channels.updatedOk'))
    } else {
      await channelApi.create(payload)
      ElMessage.success(t('channels.createdOk'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.saveFailed')))
  } finally {
    saving.value = false
  }
}

/* ── Test ────────────────────────────────────────────────────────── */
async function testChannel(row: ChannelRow) {
  testingId.value = row.id
  try {
    await channelApi.test(row.id)
    ElMessage.success(t('channels.testPassNamed', { name: row.name }))
  } catch (e: any) {
    ElMessage.error(t('channels.testFail', { msg: errMsg(e, t('channels.checkConfig')) }))
  } finally {
    testingId.value = null
  }
}

async function testForm() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  testing.value = true
  try {
    await client.post('/channels/0/test', { type: form.type, config: form.config })
    ElMessage.success(t('channels.testPass'))
  } catch (e: any) {
    ElMessage.error(t('channels.testFail', { msg: errMsg(e, t('channels.checkConfig')) }))
  } finally {
    testing.value = false
  }
}

/* ── Delete ──────────────────────────────────────────────────────── */
async function removeChannel(row: ChannelRow) {
  try {
    await ElMessageBox.confirm(
      t('channels.deleteConfirmMsg', { name: row.name }),
      t('channels.deleteConfirmTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await channelApi.remove(row.id)
    ElMessage.success(t('channels.deletedOk'))
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
      t('channels.batchDeleteConfirmMsg', { n: rows.length }),
      t('common.batchDelete'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await channelApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(t('channels.batchDeletedOk', { n: rows.length }))
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  }
}

</script>

<style scoped>
.search-input { width: 220px; }

.filter-row {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  margin-bottom: var(--space-3);
  flex-wrap: wrap;
}
.filter-select { width: 200px; }

.category-tag {
  color: var(--indigo-400) !important;
  border-color: rgba(129, 140, 248, 0.4) !important;
  background: rgba(129, 140, 248, 0.12) !important;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
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
.ch-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
}
.time-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}

/* ── Dialog ──────────────────────────────────────────────────────── */
.form-row {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
}
.form-row .grow { flex: 1; min-width: 0; }
.form-row .shrink { flex: 0 0 auto; }

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

@media (max-width: 480px) {
  .form-row { flex-direction: column; }
  .form-row .shrink .el-select { width: 100% !important; }
}
</style>
