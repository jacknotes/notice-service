<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="title-row">
          <h1 class="grad-text">{{ t('nav.templates') }}</h1>
          <el-tag v-if="!isAdmin" type="info" effect="plain" size="small">{{ t('common.readOnlyMode') }}</el-tag>
        </div>
        <p class="sub">{{ t('templates.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          :placeholder="t('templates.searchPlaceholder')"
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
        <el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openCreate">{{ t('templates.createTitle') }}</el-button>
      </div>
    </div>

    <div class="filter-row">
      <el-select
        v-model="categoryFilter"
        class="filter-select"
        clearable
        :placeholder="t('templates.allCategories')"
      >
        <el-option v-for="cg in categories" :key="cg.name" :label="cg.name" :value="cg.name" />
      </el-select>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="paged"
        style="width: 100%"
        :empty-text="t('templates.emptyTable')"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column v-if="isAdmin" type="selection" width="48" align="center" />
        <el-table-column prop="id" label="ID" width="72" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" :label="t('common.name')" min-width="170" sortable="custom" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="tpl-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('templates.category')" width="130" sortable="custom" prop="category">
          <template #default="{ row }">
            <el-tag effect="plain" size="small" class="category-tag">{{ row.category || 'default' }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="subject" :label="t('templates.subject')" min-width="220" show-overflow-tooltip sortable="custom">
          <template #default="{ row }">
            <span class="subject-cell">{{ row.subject || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('templates.variables')" width="140" align="center" sortable="custom" prop="variables_count">
          <template #default="{ row }">
            <span v-if="(row.variables || []).length" class="mono var-count">{{ t('templates.varCount', { n: row.variables.length }) }}</span>
            <span v-else class="text-muted">—</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('templates.updatedAt')" min-width="150" sortable="custom" prop="updated_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.updated_at || '—' }}</span>
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
              @change="(v: boolean) => toggleTemplate(row, v)"
            />
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="230" align="center" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="previewTemplate(row)">{{ t('templates.previewAction') }}</el-button>
            <template v-if="isAdmin">
              <el-button link type="primary" size="small" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
              <el-button link type="success" size="small" @click="duplicateTemplate(row)">{{ t('templates.duplicateAction') }}</el-button>
              <el-button link type="danger" size="small" @click="removeTemplate(row)">{{ t('common.delete') }}</el-button>
            </template>
            <span v-if="!isAdmin" class="text-muted"></span>
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
      :title="form.id ? t('templates.editTitle') : t('templates.createTitle')"
      width="900px"
      top="4vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div class="tpl-split">
        <!-- Left: form -->
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="tpl-form">
          <el-form-item :label="t('common.name')" prop="name">
            <el-input v-model="form.name" :placeholder="t('templates.namePlaceholder')" />
          </el-form-item>

          <el-form-item :label="t('templates.category')" prop="category">
            <el-select
              v-model="form.category"
              filterable
              :placeholder="t('templates.categoryPlaceholder')"
              style="width: 100%"
            >
              <el-option v-for="cg in categories" :key="cg.name" :label="cg.name" :value="cg.name" />
            </el-select>
          </el-form-item>

          <el-form-item :label="t('templates.subject')" prop="subject">
            <el-input v-model="form.subject" :placeholder="t('templates.subjectPlaceholder')" />
            <div class="field-hint mono">{{ t('templates.varHint') }}</div>
          </el-form-item>

          <el-form-item :label="t('templates.content')" prop="content_md">
            <el-input
              v-model="form.content_md"
              type="textarea"
              :rows="10"
              :placeholder="t('templates.contentPlaceholder')"
            />
          </el-form-item>

          <el-form-item :label="t('templates.variables')">
            <div class="var-editor">
              <div
                v-for="(v, i) in form.variables"
                :key="i"
                class="var-row"
              >
                <el-input
                  v-model="v.name"
                  :placeholder="t('templates.varNamePlaceholder')"
                  class="grow mono"
                />
                <el-input
                  v-model="v.default"
                  :placeholder="t('templates.varDefaultPlaceholder')"
                  class="grow-2"
                  @input="onVarEdit"
                />
                <el-button
                  :icon="Delete"
                  class="var-del"
                  text
                  @click="removeVar(i)"
                />
              </div>
              <el-button class="add-var" text :icon="Plus" @click="addVar">{{ t('templates.addVar') }}</el-button>
            </div>
          </el-form-item>
        </el-form>

        <!-- Right: live preview -->
        <div class="tpl-preview">
          <div class="preview-head">
            <div>
              <h3>{{ t('templates.livePreview') }}</h3>
              <span class="preview-sub mono">LIVE RENDER</span>
            </div>
            <el-button
              size="small"
              :icon="View"
              :loading="previewing"
              @click="useServerPreview"
            >
              {{ t('templates.renderWithValues') }}
            </el-button>
          </div>
          <div v-loading="previewing" class="preview-body">
            <MarkdownPreview :content="previewMarkdown" />
            <div v-if="!previewMarkdown" class="preview-empty">{{ t('templates.previewEmpty') }}</div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveTemplate">
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

    <!-- ── 只读预览 dialog ─────────────────────────────────────────── -->
    <el-dialog
      v-model="readonlyPreviewVisible"
      :title="t('templates.previewTitle')"
      width="680px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div v-if="readonlyPreview" class="readonly-preview">
        <div class="preview-block">
          <span class="preview-label">{{ t('tasks.subject') }}</span>
          <p class="readonly-subject">{{ readonlyPreview.subject || '—' }}</p>
        </div>
        <div class="preview-block">
          <span class="preview-label">{{ t('tasks.content') }}</span>
          <div class="preview-md">
            <MarkdownPreview :content="readonlyPreview.content" />
          </div>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="readonlyPreviewVisible = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, Delete, View, Search, ArrowDown, CircleCheck, CircleClose, CollectionTag } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { templateApi, categoryApi } from '@/api'
import MarkdownPreview from '@/components/MarkdownPreview.vue'
import { useAuthStore } from '@/stores/auth'
import { useTablePaging } from '@/composables/useTablePaging'

const { t } = useI18n()

interface TemplateVar { name: string; default: string }
interface TemplateRow {
  id: number
  name: string
  category?: string
  enabled?: boolean
  subject: string
  content_md: string
  variables: TemplateVar[]
  created_at?: string
  updated_at?: string
}

// 含 {{ 的占位/提示文案改存 locale（templates.subjectPlaceholder 等），经 vue-i18n 转义后输出。

const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')

const loading = ref(false)
const templates = ref<TemplateRow[]>([])
const tableRef = ref<TableInstance>()
const selectedRows = ref<TemplateRow[]>([])

function onSelectionChange(rows: TemplateRow[]) {
  selectedRows.value = rows
}

const keyword = ref('')

// 分类筛选（客户端）
const categoryFilter = ref<string>('')

// 共享分类池（渠道/模板/任务统一引用）：只在「分类管理」一处创建
const categories = ref<{ id: number; name: string }[]>([])

// 按名称、标题或分类做客户端过滤
const filteredTemplates = computed<TemplateRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  return templates.value.filter((t) => {
    if (categoryFilter.value && (t.category || 'default') !== categoryFilter.value) return false
    if (!kw) return true
    return (
      (t.name || '').toLowerCase().includes(kw) ||
      (t.subject || '').toLowerCase().includes(kw)
    )
  })
})

// 客户端排序 + 分页（整表数据在前端）。变量列按变量个数排序。
const { page, size, onSortChange, paged, total, onPageSizeChange } = useTablePaging<TemplateRow>(
  filteredTemplates,
  20,
  { variables_count: (t) => (t.variables || []).length },
)

const dialogVisible = ref(false)
const saving = ref(false)
const previewing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  category: string
  subject: string
  content_md: string
  variables: TemplateVar[]
}>({
  id: 0,
  name: '',
  category: 'default',
  subject: '',
  content_md: '',
  variables: [],
})

// 校验消息随语言切换（与 Login 的 computed 规则同一约定）
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('templates.nameRequired'), trigger: 'blur' }],
  subject: [{ required: true, message: t('templates.subjectRequired'), trigger: 'blur' }],
  content_md: [{ required: true, message: t('templates.contentRequired'), trigger: 'blur' }],
}))

/* ── Preview state ─────────────────────────────────────────────────── */
// Server-rendered preview (variables substituted) overrides the live one.
const serverPreview = ref<{ subject: string; content: string } | null>(null)

const previewSubject = computed(() => serverPreview.value?.subject ?? form.subject)
const previewContent = computed(() => serverPreview.value?.content ?? form.content_md)
const previewMarkdown = computed(() => {
  const subject = previewSubject.value.trim()
  const content = previewContent.value
  if (!subject && !content) return ''
  return subject ? `## ${subject}\n\n${content}` : content
})

// Live preview tracks the form; any edit clears the server-rendered snapshot.
watch(
  [() => form.subject, () => form.content_md],
  () => { serverPreview.value = null }
)

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    templates.value = await templateApi.list()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('templates.loadFailed')))
  } finally {
    loading.value = false
  }
}

// 共享分类池（渠道/模板/任务统一引用）：只在「分类管理」一处创建
async function loadCategories() {
  try {
    categories.value = (await categoryApi.list()) || []
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('templates.loadFailed')))
  }
}

/* ── Create / edit ─────────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.category = 'default'
  form.subject = ''
  form.content_md = ''
  form.variables = []
  serverPreview.value = null
  dialogVisible.value = true
}

function openEdit(row: TemplateRow) {
  form.id = row.id
  form.name = row.name
  form.category = row.category || 'default'
  form.subject = row.subject
  form.content_md = row.content_md
  form.variables = (row.variables || []).map((v) => ({ name: v.name, default: v.default ?? '' }))
  serverPreview.value = null
  dialogVisible.value = true
}

// 复制模板：打开「新建模板」并预填源模板内容（名称加副本后缀），id=0 走创建路径。
function duplicateTemplate(row: TemplateRow) {
  form.id = 0
  form.name = `${row.name}${t('common.copySuffix')}`
  form.category = row.category || 'default'
  form.subject = row.subject
  form.content_md = row.content_md
  form.variables = (row.variables || []).map((v) => ({ name: v.name, default: v.default ?? '' }))
  serverPreview.value = null
  dialogVisible.value = true
}

function addVar() {
  form.variables.push({ name: '', default: '' })
}

function removeVar(i: number) {
  form.variables.splice(i, 1)
}

function onVarEdit() {
  serverPreview.value = null
}

/* ── Save ──────────────────────────────────────────────────────────── */
async function saveTemplate() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const vars = form.variables
      .filter((v) => v.name.trim())
      .map((v) => ({ name: v.name.trim(), type: 'string', description: '', default: v.default }))
    const payload = { name: form.name, category: form.category || 'default', subject: form.subject, content_md: form.content_md, variables: vars }

    if (form.id) {
      await templateApi.update(form.id, payload)
      ElMessage.success(t('templates.updatedOk'))
    } else {
      await templateApi.create(payload)
      ElMessage.success(t('templates.createdOk'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.saveFailed')))
  } finally {
    saving.value = false
  }
}

/* ── Server preview ────────────────────────────────────────────────── */
// 使用「当前表单值」（未保存的新值）渲染，不再回退已保存值；新模板（id=0）也可预览。
async function useServerPreview() {
  const vars: Record<string, string> = {}
  for (const v of form.variables) {
    if (v.name.trim()) vars[v.name.trim()] = v.default
  }
  previewing.value = true
  try {
    const res = await templateApi.preview(form.id, {
      subject: form.subject,
      content_md: form.content_md,
      variables: vars,
    })
    serverPreview.value = { subject: res?.subject ?? form.subject, content: res?.content ?? form.content_md }
    ElMessage.success(t('templates.renderOk'))
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('templates.previewFailed')))
  } finally {
    previewing.value = false
  }
}

/* ── 只读预览（操作列按钮，所有登录用户可用） ─────────────────────── */
const readonlyPreviewVisible = ref(false)
const readonlyPreview = ref<{ subject: string; content: string } | null>(null)

async function previewTemplate(row: TemplateRow) {
  readonlyPreview.value = { subject: row.subject, content: row.content_md }
  readonlyPreviewVisible.value = true
}

/* ── Delete ────────────────────────────────────────────────────────── */
async function removeTemplate(row: TemplateRow) {
  try {
    await ElMessageBox.confirm(
      t('templates.deleteConfirmMsg', { name: row.name }),
      t('templates.deleteConfirmTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await templateApi.remove(row.id)
    ElMessage.success(t('templates.deletedOk'))
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
    await templateApi.batchRemove(rows.map((r) => r.id))
    ElMessage.success(t('templates.batchDeletedOk', { n: rows.length }))
    tableRef.value?.clearSelection()
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.batchDeleteFailed')))
  }
}

/* ── 状态开关（单条） ─────────────────────────────────────────────── */
const togglingId = ref<number | null>(null)

async function toggleTemplate(row: TemplateRow, enabled: boolean) {
  togglingId.value = row.id
  try {
    await templateApi.batchToggle([row.id], enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? t('templates.enabledOk') : t('templates.disabledOk'))
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('templates.toggleFailed')))
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
    // 带出原有值：若所有选中模板分类一致则默认选中该分类，否则留空
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
    await templateApi.batchToggle(rows.map((r) => r.id), enabled)
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
    await templateApi.batchCategory(selectedRows.value.map((r) => r.id), batchCategory.value)
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

onMounted(() => {
  load()
  loadCategories()
})
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
.tpl-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
}
.subject-cell {
  color: var(--text-secondary);
  font-size: var(--text-sm);
}
.var-count {
  color: var(--violet-400);
  font-size: var(--text-xs);
}
.time-cell {
  color: var(--text-secondary);
  font-size: var(--text-xs);
}

/* ── Dialog layout ─────────────────────────────────────────────────── */
.tpl-split {
  display: grid;
  grid-template-columns: 1.15fr 1fr;
  gap: var(--space-5);
  align-items: start;
}

.field-hint {
  width: 100%;
  margin-top: 4px;
  color: var(--text-faint);
  font-size: 11px;
}

/* ── Variable editor ───────────────────────────────────────────────── */
.var-editor {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.var-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.var-row .grow { flex: 1; min-width: 0; }
.var-row .grow-2 { flex: 1.6; min-width: 0; }
.var-del {
  color: var(--rose-400);
  flex: 0 0 auto;
}
.var-del:hover { color: var(--rose-500); background: rgba(248, 113, 113, 0.1); }
.add-var {
  align-self: flex-start;
  color: var(--indigo-400);
}

/* ── Preview panel ─────────────────────────────────────────────────── */
.tpl-preview {
  position: sticky;
  top: 8px;
  display: flex;
  flex-direction: column;
  min-height: 320px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: rgba(11, 17, 32, 0.55);
  overflow: hidden;
}
.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-faint);
  background: rgba(148, 163, 184, 0.05);
}
.preview-head h3 {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
}
.preview-sub {
  display: block;
  margin-top: 2px;
  color: var(--text-faint);
  font-size: 10px;
  letter-spacing: 0.22em;
}
.preview-body {
  flex: 1;
  padding: 16px 18px;
  min-height: 260px;
  max-height: 520px;
  overflow: auto;
}
.preview-empty {
  display: grid;
  place-items: center;
  min-height: 220px;
  color: var(--text-faint);
  font-size: var(--text-sm);
  text-align: center;
}

/* ── 只读预览（操作列按钮） ──────────────────────────────────────── */
.readonly-preview {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.readonly-preview .preview-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text-faint);
}
.readonly-preview .readonly-subject {
  margin: 6px 0 0;
  color: var(--text-primary);
  font-size: var(--text-md);
  font-weight: 600;
  word-break: break-word;
}
.readonly-preview .preview-md {
  margin-top: 6px;
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(148, 163, 184, 0.05);
  max-height: 48vh;
  overflow: auto;
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

@media (max-width: 900px) {
  .tpl-split { grid-template-columns: 1fr; }
  .tpl-preview { position: static; }
}
</style>
