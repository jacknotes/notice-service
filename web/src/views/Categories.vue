<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="title-row">
          <h1 class="grad-text">{{ t('nav.categories') }}</h1>
          <el-tag v-if="!isAdmin" type="info" effect="plain" size="small">{{ t('common.readOnlyMode') }}</el-tag>
        </div>
        <p class="sub">{{ t('categories.subtitle') }}</p>
      </div>
      <div class="actions">
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
              <el-dropdown-item command="delete" :icon="Delete" class="danger-dropdown-item">
                {{ t('common.batchDelete') }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button v-if="isAdmin" type="primary" :icon="Plus" @click="openCreate">{{ t('categories.createTitle') }}</el-button>
      </div>
    </div>

    <el-alert
      v-if="unusedHint && unusedSet.size"
      class="unused-alert"
      type="info"
      :closable="false"
      show-icon
      :title="t('categories.unusedHint')"
    />

    <div v-loading="loading" class="card table-card">
      <el-table
        ref="tableRef"
        :data="paged"
        style="width: 100%"
        :empty-text="t('categories.emptyTable')"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column
          v-if="isAdmin"
          type="selection"
          width="48"
          align="center"
          :selectable="isSelectableRow"
        />
        <el-table-column prop="id" label="ID" width="72" align="center" sortable="custom">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.name')" min-width="180" sortable="custom" prop="name" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cat-name">{{ row.name }}</span>
            <el-tag v-if="row.name === 'default'" effect="plain" size="small" class="default-tag">
              {{ t('categories.defaultTag') }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('categories.usage')" min-width="220" show-overflow-tooltip sortable="custom" prop="total">
          <template #default="{ row }">
            <div v-if="row.total" class="usage-chips">
              <el-tag v-if="row.channels" size="small" effect="plain" class="usage-chip ch">
                {{ row.channels }} {{ t('categories.refChannels') }}
              </el-tag>
              <el-tag v-if="row.templates" size="small" effect="plain" class="usage-chip tpl">
                {{ row.templates }} {{ t('categories.refTemplates') }}
              </el-tag>
              <el-tag v-if="row.tasks" size="small" effect="plain" class="usage-chip task">
                {{ row.tasks }} {{ t('categories.refTasks') }}
              </el-tag>
            </div>
            <span v-else class="text-muted">{{ t('categories.unused') }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('categories.createdAt')" min-width="150" sortable="custom" prop="created_at">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.action')" width="170" align="center" fixed="right">
          <template #default="{ row }">
            <template v-if="isAdmin">
              <el-button
                link
                type="primary"
                size="small"
                :disabled="row.name === 'default'"
                :title="row.name === 'default' ? t('categories.defaultNotEditable') : ''"
                @click="openEdit(row)"
              >
                {{ t('common.edit') }}
              </el-button>
              <el-button
                link
                type="danger"
                size="small"
                :disabled="row.name === 'default' || row.total > 0"
                :title="row.name === 'default' || row.total > 0 ? t('categories.deleteRefHint') : ''"
                @click="removeCategory(row)"
              >
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

    <!-- ── Create / Edit dialog ─────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? t('categories.editTitle') : t('categories.createTitle')"
      width="420px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-alert v-if="isEdit && form.oldName" type="info" :closable="false" show-icon class="edit-hint" :title="t('categories.editHint')" />
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent>
        <el-form-item :label="t('common.name')" prop="name">
          <el-input
            v-model="form.name"
            :placeholder="t('categories.namePlaceholder')"
            @keyup.enter="saveCategory"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveCategory">
            {{ isEdit ? t('common.save') : t('common.create') }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules, TableInstance } from 'element-plus'
import { Plus, ArrowDown, Delete } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { categoryApi, channelApi, templateApi, taskApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useTablePaging } from '@/composables/useTablePaging'

const { t } = useI18n()

interface CategoryRow {
  id: number
  name: string
  created_at: string
  channels: number
  templates: number
  tasks: number
  total: number
  inUse: boolean
}

const auth = useAuthStore()
const isAdmin = computed(() => auth.user?.role === 'admin')

const loading = ref(false)
const rows = ref<CategoryRow[]>([])
const unusedSet = ref<Set<string>>(new Set())

// 空分类（过滤掉 usedRows 处理前保留原样）：分类池列表全部
const pagedSource = computed<CategoryRow[]>(() => rows.value)

const { page, size, onSortChange, paged, total, onPageSizeChange } = useTablePaging<CategoryRow>(pagedSource)

const unusedHint = computed(() => t('categories.unusedHint'))

/* ── 批量删除（操作下拉） ─────────────────────────────────────────── */
const tableRef = ref<TableInstance>()
const selectedRows = ref<CategoryRow[]>([])

function onSelectionChange(rows: CategoryRow[]) {
  selectedRows.value = rows
}

// 只允许勾选未被引用的分类（default 与已被引用的分类不可批量删除）
function isSelectableRow(row: CategoryRow) {
  return row.name !== 'default' && row.total === 0
}

function onBatchCommand(cmd: string) {
  if (cmd === 'delete') return batchDelete()
}

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
  // 分类删除语义是逐个校验引用（default 与已引用不可删），循环调用单条删除
  let ok = 0
  for (const row of rows) {
    try {
      await categoryApi.remove(row.name)
      ok++
    } catch {
      /* 单条失败不中断，继续删除其余 */
    }
  }
  if (ok > 0) ElMessage.success(t('categories.batchDeletedOk', { n: ok }))
  if (ok < rows.length) ElMessage.warning(t('common.batchDeleteFailed'))
  tableRef.value?.clearSelection()
  await load()
}

const dialogVisible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{ id: number; oldName: string; name: string }>({
  id: 0,
  oldName: '',
  name: '',
})

const isEdit = computed(() => form.id > 0)

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('categories.nameRequired'), trigger: 'blur' }],
}))

function errMsg(e: any, fallback: string) {
  return e?.response?.data?.error || e?.message || fallback
}

async function load() {
  loading.value = true
  try {
    // 共享分类池：各资源列表仅用其 category 字段统计引用数。
    // 显式标注返回类型：channel/template/task API 返回 any，混入 Promise.all 后
    // TS 会把解构出的变量推断为 unknown（countBy 泛型无法收窄），需在此断言。
    const [cats, chs, tpls, tasks] = await Promise.all([
      categoryApi.list(),
      channelApi.list() as Promise<Array<{ category?: string }>>,
      templateApi.list() as Promise<Array<{ category?: string }>>,
      taskApi.list() as Promise<Array<{ category?: string }>>,
    ])
    const chCount = countBy(chs, (c) => c.category || 'default')
    const tplCount = countBy(tpls, (c) => c.category || 'default')
    const taskCount = countBy(tasks, (c) => c.category || 'default')
    rows.value = (cats || []).map((c) => {
      const ch = chCount[c.name] || 0
      const tpl = tplCount[c.name] || 0
      const task = taskCount[c.name] || 0
      const totalCount = ch + tpl + task
      return { ...c, channels: ch, templates: tpl, tasks: task, total: totalCount, inUse: totalCount > 0 }
    })
    try {
      unusedSet.value = new Set(await categoryApi.unused())
    } catch {
      unusedSet.value = new Set()
    }
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('categories.loadFailed')))
  } finally {
    loading.value = false
  }
}

function countBy<T>(list: T[], pick: (x: T) => string): Record<string, number> {
  const out: Record<string, number> = {}
  for (const x of list || []) {
    const k = pick(x)
    out[k] = (out[k] || 0) + 1
  }
  return out
}

function openCreate() {
  form.id = 0
  form.oldName = ''
  form.name = ''
  dialogVisible.value = true
}

function openEdit(row: CategoryRow) {
  form.id = row.id
  form.oldName = row.name
  form.name = row.name
  dialogVisible.value = true
}

async function saveCategory() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (isEdit.value) {
      await categoryApi.update(form.oldName, form.name.trim())
      ElMessage.success(t('categories.updatedOk'))
    } else {
      await categoryApi.create(form.name.trim())
      ElMessage.success(t('categories.createdOk'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('categories.createFailed')))
  } finally {
    saving.value = false
  }
}

async function removeCategory(row: CategoryRow) {
  try {
    await ElMessageBox.confirm(
      t('categories.deleteConfirmMsg', { name: row.name }),
      t('categories.deleteConfirmTitle'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await categoryApi.remove(row.name)
    ElMessage.success(t('categories.deletedOk'))
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, t('common.deleteFailed')))
  }
}

onMounted(load)
</script>

<style scoped>
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* 夜晚（默认深色控制台）：信息类警告用深蓝灰背景，与卡片/页面底色融为一体。
   白天模式由下方 [data-theme='light'] 块还原为 Element Plus 默认 info 浅色。
   class 直接落在 el-alert 根元素上（.el-alert.unused-alert），!important 覆盖 EP 背景。 */
.unused-alert,
.edit-hint {
  background-color: #1e2a3f !important;
  color: var(--text-secondary) !important;
  border: 1px solid rgba(148, 163, 184, 0.18);
  --el-alert-bg-color: #1e2a3f;
}

.unused-alert {
  margin-bottom: var(--space-3);
}

/* ── 批量操作下拉 ─────────────────────────────────────────────────── */
.danger-dropdown-item {
  color: var(--rose-400) !important;
}

.edit-hint {
  margin-bottom: var(--space-3);
}

/* 白天模式：还原 Element Plus 默认 info 浅色 alert */
[data-theme='light'] .unused-alert,
[data-theme='light'] .edit-hint {
  background-color: #f4f4f5 !important;
  color: var(--text-primary) !important;
  border: 1px solid transparent;
  --el-alert-bg-color: #f4f4f5;
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

.cat-name {
  color: var(--text-primary);
  font-weight: 600;
  font-size: var(--text-sm);
}

.default-tag {
  margin-left: 8px;
  color: var(--amber-400) !important;
  border-color: rgba(251, 191, 36, 0.35) !important;
  background: rgba(251, 191, 36, 0.1) !important;
}

.usage-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.usage-chip {
  font-size: 11px;
}
.usage-chip.ch {
  color: var(--sky-400) !important;
  border-color: rgba(56, 189, 248, 0.3) !important;
  background: rgba(56, 189, 248, 0.08) !important;
}
.usage-chip.tpl {
  color: var(--violet-400) !important;
  border-color: rgba(139, 92, 246, 0.3) !important;
  background: rgba(139, 92, 246, 0.08) !important;
}
.usage-chip.task {
  color: var(--indigo-400) !important;
  border-color: rgba(129, 140, 248, 0.3) !important;
  background: rgba(129, 140, 248, 0.08) !important;
}

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }
</style>