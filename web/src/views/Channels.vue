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
        <el-dropdown
          v-if="isAdmin"
          trigger="hover"
          :disabled="!selectedRows.length"
          @command="onBatchCommand"
        >
          <el-button type="primary" :disabled="!selectedRows.length">
            {{ t('common.batchOps') }}
            <el-icon class="el-icon--right"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="enable" :icon="CircleCheck">{{ t('common.batchEnable') }}</el-dropdown-item>
              <el-dropdown-item command="disable" :icon="CircleClose">{{ t('common.batchDisable') }}</el-dropdown-item>
              <el-dropdown-item command="category" :icon="CollectionTag">{{ t('common.batchChangeCategory') }}</el-dropdown-item>
              <el-dropdown-item divided command="delete" :icon="Delete" class="danger-dropdown-item">
                {{ t('common.batchDelete') }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
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
        :row-class-name="rowClassName"
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

        <el-table-column :label="t('channels.createdAt')" min-width="150" sortable="custom" prop="created_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.status')" width="110" align="center" sortable="custom" prop="enabled">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="togglingId === row.id"
              :disabled="!isAdmin"
              inline-prompt
              :active-text="t('tasks.on')"
              :inactive-text="t('tasks.off')"
              @change="(v: boolean) => toggleChannel(row, v)"
            />
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

    <!-- ── 批量变更分类 dialog ─────────────────────────────────────── -->
    <el-dialog
      v-model="batchCategoryVisible"
      :title="t('common.batchCategoryTitle')"
      width="440px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <p class="batch-desc">{{ t('common.batchCategoryConfirmMsg', { n: selectedRows.length, name: batchCategory || '' }) }}</p>
      <el-select v-model="batchCategory" filterable :placeholder="t('common.selectCategory')" style="width: 100%">
        <el-option v-for="cg in categories" :key="cg.name" :label="cg.name" :value="cg.name" />
      </el-select>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="batchCategoryVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="batching" @click="doBatchCategory">{{ t('common.confirm') }}</el-button>
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
import { Plus, Edit, Delete, Promotion, Search, ArrowDown, CircleCheck, CircleClose, CollectionTag } from '@element-plus/icons-vue'
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

/* ── 外部跳转闪烁定位（发送日志 → 渠道管理 → ?edit=<id>） ────────────
   目标行短暂闪烁两次后恢复默认样式，不常驻高亮。 */
const highlightId = ref<number | null>(null)
const flashRowId = ref<number | null>(null)

// 行 class：目标行在闪烁期间附加 flash 样式（动画结束后由下方定时器清除）
function rowClassName({ row }: { row: ChannelRow }) {
  return flashRowId.value === row.id ? 'flash-row' : ''
}

function highlightById(id: number) {
  const row = channels.value.find((c) => c.id === id)
  if (!row) return
  // 行可能跨页：切到含该行的页再 setCurrentRow 滚动定位
  const idx = channels.value.findIndex((c) => c.id === id)
  const targetPage = Math.floor(idx / size.value) + 1
  if (targetPage !== page.value) page.value = targetPage
  nextTick(() => {
    tableRef.value?.setCurrentRow(row)
    flashRowId.value = id
    // 动画约 0.9s（闪烁两次），结束后清除 class 恢复正常显示
    window.setTimeout(() => {
      flashRowId.value = null
    }, 1000)
  })
}

onMounted(() => {
  load()
  loadCategories()
  const id = Number(route.query.edit)
  if (Number.isInteger(id) && id > 0) highlightId.value = id
})

// 渠道数据就绪后执行闪烁（数据可能晚于 onMounted 返回）
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

/* ── 状态开关（单条） ─────────────────────────────────────────────── */
const togglingId = ref<number | null>(null)

async function toggleChannel(row: ChannelRow, enabled: boolean) {
  togglingId.value = row.id
  try {
    await channelApi.batchToggle([row.id], enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? t('channels.enabledOk') : t('channels.disabledOk'))
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('channels.toggleFailed')))
  } finally {
    togglingId.value = null
  }
}

/* ── 批量操作（操作下拉） ─────────────────────────────────────────── */
const batching = ref(false)
const batchCategoryVisible = ref(false)
const batchCategory = ref('')

function onBatchCommand(cmd: string) {
  if (!selectedRows.value.length) return
  if (cmd === 'enable') return doBatchToggle(true)
  if (cmd === 'disable') return doBatchToggle(false)
  if (cmd === 'category') {
    // 带出原有值：若所有选中渠道分类一致则默认选中该分类，否则留空
    const cats = selectedRows.value.map((r) => r.category || 'default')
    batchCategory.value = cats.every((c) => c === cats[0]) ? cats[0] : ''
    batchCategoryVisible.value = true
    return
  }
  if (cmd === 'delete') return batchDelete()
}

async function doBatchToggle(enabled: boolean) {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    await ElMessageBox.confirm(
      enabled ? t('common.batchEnableConfirmMsg', { n: rows.length }) : t('common.batchDisableConfirmMsg', { n: rows.length }),
      t('common.batchToggleTitle'),
      { confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  batching.value = true
  try {
    await channelApi.batchToggle(rows.map((r) => r.id), enabled)
    ElMessage.success(t('common.batchToggleOk'))
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  } finally {
    batching.value = false
  }
}

async function doBatchCategory() {
  if (!batchCategory.value) {
    ElMessage.warning(t('common.selectCategory'))
    return
  }
  batching.value = true
  try {
    await channelApi.batchCategory(selectedRows.value.map((r) => r.id), batchCategory.value)
    ElMessage.success(t('common.batchCategoryOk'))
    batchCategoryVisible.value = false
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  } finally {
    batching.value = false
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

/* 外部跳转定位闪烁：目标行闪烁两次后恢复默认（约 0.9s） */
:deep(.flash-row > td.el-table__cell) {
  animation: flash-target 0.45s ease-in-out 2;
}
@keyframes flash-target {
  0%, 100% { background-color: transparent; }
  50% { background-color: rgba(129, 140, 248, 0.28); }
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

/* ── 批量操作下拉与弹窗 ──────────────────────────────────────────── */
.danger-dropdown-item {
  color: var(--rose-400) !important;
}
.batch-desc {
  margin: 0 0 var(--space-3);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  line-height: 1.7;
}

@media (max-width: 480px) {
  .form-row { flex-direction: column; }
  .form-row .shrink .el-select { width: 100% !important; }
}
</style>
