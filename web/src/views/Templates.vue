<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">模板管理</h1>
        <p class="sub">维护通知模板：标题、Markdown 正文与可注入变量</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          placeholder="搜索名称或标题…"
        />
        <el-button type="primary" :icon="Plus" @click="openCreate">新建模板</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table :data="filteredTemplates" style="width: 100%" empty-text="暂无模板，点击右上角「新建模板」开始">
        <el-table-column prop="id" label="ID" width="72" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="名称" min-width="170">
          <template #default="{ row }">
            <span class="tpl-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="subject" label="标题" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="subject-cell">{{ row.subject || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="变量" width="140" align="center">
          <template #default="{ row }">
            <span v-if="(row.variables || []).length" class="mono var-count">{{ row.variables.length }} 个</span>
            <span v-else class="text-muted">—</span>
          </template>
        </el-table-column>

        <el-table-column label="更新时间" min-width="150">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.updated_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="150" align="center" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" @click="removeTemplate(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- ── Create / Edit dialog ─────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? '编辑模板' : '新建模板'"
      width="900px"
      top="4vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div class="tpl-split">
        <!-- Left: form -->
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="tpl-form">
          <el-form-item label="名称" prop="name">
            <el-input v-model="form.name" placeholder="给模板起个易记的名字" />
          </el-form-item>

          <el-form-item label="标题" prop="subject">
            <el-input v-model="form.subject" :placeholder="SUBJECT_PLACEHOLDER" />
            <div class="field-hint mono">{{ VAR_HINT }}</div>
          </el-form-item>

          <el-form-item label="内容（Markdown）" prop="content_md">
            <el-input
              v-model="form.content_md"
              type="textarea"
              :rows="10"
              :placeholder="CONTENT_PLACEHOLDER"
            />
          </el-form-item>

          <el-form-item label="变量">
            <div class="var-editor">
              <div
                v-for="(v, i) in form.variables"
                :key="i"
                class="var-row"
              >
                <el-input
                  v-model="v.name"
                  placeholder="变量名，如 username"
                  class="grow mono"
                />
                <el-input
                  v-model="v.default"
                  placeholder="默认值"
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
              <el-button class="add-var" text :icon="Plus" @click="addVar">添加变量</el-button>
            </div>
          </el-form-item>
        </el-form>

        <!-- Right: live preview -->
        <div class="tpl-preview">
          <div class="preview-head">
            <div>
              <h3>实时预览</h3>
              <span class="preview-sub mono">LIVE RENDER</span>
            </div>
            <el-button
              size="small"
              :icon="View"
              :loading="previewing"
              @click="useServerPreview"
            >
              使用当前值预览
            </el-button>
          </div>
          <div v-loading="previewing" class="preview-body">
            <MarkdownPreview :content="previewMarkdown" />
            <div v-if="!previewMarkdown" class="preview-empty">输入标题与内容后，这里会实时渲染 Markdown 效果</div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveTemplate">
            {{ form.id ? '保存' : '创建' }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Delete, View, Search } from '@element-plus/icons-vue'
import { templateApi } from '@/api'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

interface TemplateVar { name: string; default: string }
interface TemplateRow {
  id: number
  name: string
  subject: string
  content_md: string
  variables: TemplateVar[]
  created_at?: string
  updated_at?: string
}

// Literals containing `{{` must live in JS, not the template (Vue interpolation).
const VAR_HINT = '{{变量}} 会在发送时被替换'
const SUBJECT_PLACEHOLDER = '邮件 / 卡片标题，支持 {{变量}}'
const CONTENT_PLACEHOLDER =
  '支持 Markdown 语法，例如：\n## 标题\n**加粗** / `代码` / > 引用\n正文… 变量用 {{变量名}} 表示'

const loading = ref(false)
const templates = ref<TemplateRow[]>([])

const keyword = ref('')

// 按名称或标题做客户端过滤
const filteredTemplates = computed<TemplateRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return templates.value
  return templates.value.filter((t) =>
    (t.name || '').toLowerCase().includes(kw) ||
    (t.subject || '').toLowerCase().includes(kw)
  )
})

const dialogVisible = ref(false)
const saving = ref(false)
const previewing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  subject: string
  content_md: string
  variables: TemplateVar[]
}>({
  id: 0,
  name: '',
  subject: '',
  content_md: '',
  variables: [],
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入模板名称', trigger: 'blur' }],
  subject: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  content_md: [{ required: true, message: '请输入内容', trigger: 'blur' }],
}

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
    ElMessage.error(errMsg(e, '模板列表加载失败'))
  } finally {
    loading.value = false
  }
}

/* ── Create / edit ─────────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.subject = ''
  form.content_md = ''
  form.variables = []
  serverPreview.value = null
  dialogVisible.value = true
}

function openEdit(row: TemplateRow) {
  form.id = row.id
  form.name = row.name
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
    const payload = { name: form.name, subject: form.subject, content_md: form.content_md, variables: vars }

    if (form.id) {
      await templateApi.update(form.id, payload)
      ElMessage.success('模板已更新')
    } else {
      await templateApi.create(payload)
      ElMessage.success('模板已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

/* ── Server preview ────────────────────────────────────────────────── */
async function useServerPreview() {
  // New template has no id yet → local preview only.
  if (!form.id) {
    ElMessage.info('模板尚未保存，当前为本地实时预览')
    serverPreview.value = null
    return
  }
  const vars: Record<string, string> = {}
  for (const v of form.variables) {
    if (v.name.trim()) vars[v.name.trim()] = v.default
  }
  previewing.value = true
  try {
    const res = await templateApi.preview(form.id, vars)
    serverPreview.value = { subject: res?.subject ?? form.subject, content: res?.content ?? form.content_md }
    ElMessage.success('已按当前变量值渲染')
  } catch (e: any) {
    ElMessage.error(errMsg(e, '预览生成失败'))
  } finally {
    previewing.value = false
  }
}

/* ── Delete ────────────────────────────────────────────────────────── */
async function removeTemplate(row: TemplateRow) {
  try {
    await ElMessageBox.confirm(
      `确定删除模板「${row.name}」吗？删除后不可恢复。`,
      '删除模板',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await templateApi.remove(row.id)
    ElMessage.success('模板已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

onMounted(load)
</script>

<style scoped>
.search-input { width: 220px; }

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

.dialog-footer {
  display: flex;
  align-items: center;
  width: 100%;
}
.footer-grow { flex: 1; }

@media (max-width: 900px) {
  .tpl-split { grid-template-columns: 1fr; }
  .tpl-preview { position: static; }
}
</style>
