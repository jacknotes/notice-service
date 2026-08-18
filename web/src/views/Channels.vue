<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">渠道管理</h1>
        <p class="sub">配置通知投递渠道：SMTP 邮件、企业微信、钉钉、飞书、PushPlus</p>
      </div>
      <div class="actions">
        <el-input
          v-model="keyword"
          class="search-input"
          clearable
          :prefix-icon="Search"
          placeholder="搜索名称或类型…"
        />
        <el-button type="primary" :icon="Plus" @click="openCreate">
          新建渠道
        </el-button>
      </div>
    </div>

    <div v-loading="loading" class="card table-card">
      <el-table :data="filteredChannels" style="width: 100%" empty-text="暂无渠道，点击右上角「新建渠道」开始">
        <el-table-column prop="id" label="ID" width="72" align="center">
          <template #default="{ row }">
            <span class="mono id-cell">#{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="名称" min-width="160">
          <template #default="{ row }">
            <span class="ch-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="type" label="类型" width="150">
          <template #default="{ row }">
            <el-tag :style="typeTagStyle(row.type)" effect="plain" size="small">
              {{ typeLabel(row.type) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" effect="light" size="small">
              {{ row.enabled ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="创建时间" min-width="150">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.created_at || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="210" align="center" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" :loading="testingId === row.id" @click="testChannel(row)">
              测试
            </el-button>
            <el-button link type="primary" size="small" @click="openEdit(row)">
              编辑
            </el-button>
            <el-button link type="danger" size="small" @click="removeChannel(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- ── Create / Edit dialog ────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? '编辑渠道' : '新建渠道'"
      width="520px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-row">
          <el-form-item label="名称" prop="name" class="grow">
            <el-input v-model="form.name" placeholder="给渠道起个易记的名字" />
          </el-form-item>

          <el-form-item label="类型" prop="type" class="shrink">
            <el-select v-model="form.type" placeholder="选择类型" style="width: 160px" @change="onTypeChange">
              <el-option v-for="t in typeOptions" :key="t.value" :label="t.label" :value="t.value" />
            </el-select>
          </el-form-item>
        </div>

        <template v-for="field in currentFields" :key="field.key">
          <el-form-item :label="field.label" :prop="`config.${field.key}`">
            <el-input
              v-model="form.config[field.key]"
              :type="field.type"
              :placeholder="field.placeholder"
              :show-password="field.type === 'password'"
              clearable
            />
          </el-form-item>
        </template>

        <el-form-item label="状态">
          <div class="enabled-row">
            <el-switch
              v-model="form.enabled"
              inline-prompt
              active-text="启用"
              inactive-text="停用"
            />
            <span class="enabled-hint">停用后该渠道不再参与投递</span>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <el-button :loading="testing" :icon="Promotion" @click="testForm">
            测试连接
          </el-button>
          <span class="footer-grow"></span>
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveChannel">
            {{ form.id ? '保存' : '创建' }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Edit, Delete, Promotion, Search } from '@element-plus/icons-vue'
import client from '@/api/client'
import { channelApi } from '@/api'

interface ChannelRow {
  id: number
  type: string
  name: string
  config: Record<string, string>
  enabled: boolean
  created_at?: string
}

interface ConfigField {
  key: string
  label: string
  placeholder: string
  type: 'text' | 'password'
}

const typeOptions = [
  { value: 'email', label: 'SMTP 邮件' },
  { value: 'wecom', label: '企业微信' },
  { value: 'dingtalk', label: '钉钉' },
  { value: 'feishu', label: '飞书' },
  { value: 'wechat', label: 'PushPlus' },
]

const typeMeta: Record<string, { label: string; color: string }> = {
  email: { label: 'SMTP 邮件', color: '#818cf8' },
  wecom: { label: '企业微信', color: '#38bdf8' },
  dingtalk: { label: '钉钉', color: '#8b5cf6' },
  feishu: { label: '飞书', color: '#34d399' },
  wechat: { label: 'PushPlus', color: '#fbbf24' },
}

const configFields: Record<string, ConfigField[]> = {
  email: [
    { key: 'host', label: 'SMTP 服务器', placeholder: 'smtp.example.com', type: 'text' },
    { key: 'port', label: '端口', placeholder: '465 / 587 / 25', type: 'text' },
    { key: 'username', label: '用户名', placeholder: '发件邮箱账号', type: 'text' },
    { key: 'password', label: '授权码', placeholder: 'SMTP 授权码 / 密码', type: 'password' },
    { key: 'from', label: '发件人', placeholder: 'no-reply@example.com', type: 'text' },
  ],
  wecom: [
    { key: 'webhook_url', label: 'Webhook 地址', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…', type: 'text' },
  ],
  dingtalk: [
    { key: 'webhook_url', label: 'Webhook 地址', placeholder: 'https://oapi.dingtalk.com/robot/send?access_token=…', type: 'text' },
    { key: 'secret', label: '加签密钥（可选）', placeholder: 'SEC…（未启用加签可留空）', type: 'text' },
  ],
  feishu: [
    { key: 'webhook_url', label: 'Webhook 地址', placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/…', type: 'text' },
  ],
  wechat: [
    { key: 'pushplus_token', label: 'PushPlus Token', placeholder: 'https://www.pushplus.plus 获取的 token', type: 'password' },
  ],
}

const loading = ref(false)
const channels = ref<ChannelRow[]>([])
const testingId = ref<number | null>(null)

const keyword = ref('')

// 按名称或类型（原始值 / 中文标签）做客户端过滤
const filteredChannels = computed<ChannelRow[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return channels.value
  return channels.value.filter((c) =>
    (c.name || '').toLowerCase().includes(kw) ||
    (c.type || '').toLowerCase().includes(kw) ||
    (typeLabel(c.type) || '').toLowerCase().includes(kw)
  )
})

const dialogVisible = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  id: number
  name: string
  type: string
  enabled: boolean
  config: Record<string, string>
}>({
  id: 0,
  name: '',
  type: 'email',
  enabled: true,
  config: {},
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入渠道名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择渠道类型', trigger: 'change' }],
}

const currentFields = computed<ConfigField[]>(() => configFields[form.type] || [])

function typeLabel(t: string) {
  return typeMeta[t]?.label || t
}

function typeTagStyle(t: string) {
  const c = typeMeta[t]?.color || '#94a3b8'
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
    ElMessage.error(errMsg(e, '渠道列表加载失败'))
  } finally {
    loading.value = false
  }
}

/* ── Create / edit ───────────────────────────────────────────────── */
function openCreate() {
  form.id = 0
  form.name = ''
  form.type = 'email'
  form.enabled = true
  form.config = {}
  dialogVisible.value = true
}

function openEdit(row: ChannelRow) {
  form.id = row.id
  form.name = row.name
  form.type = row.type
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
    const payload = { type: form.type, name: form.name, config: form.config, enabled: form.enabled }
    if (form.id) {
      await channelApi.update(form.id, payload)
      ElMessage.success('渠道已更新')
    } else {
      await channelApi.create(payload)
      ElMessage.success('渠道已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

/* ── Test ────────────────────────────────────────────────────────── */
async function testChannel(row: ChannelRow) {
  testingId.value = row.id
  try {
    await channelApi.test(row.id)
    ElMessage.success(`「${row.name}」连接测试通过`)
  } catch (e: any) {
    ElMessage.error(`连接测试失败：${errMsg(e, '请检查配置')}`)
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
    ElMessage.success('连接测试通过')
  } catch (e: any) {
    ElMessage.error(`连接测试失败：${errMsg(e, '请检查配置')}`)
  } finally {
    testing.value = false
  }
}

/* ── Delete ──────────────────────────────────────────────────────── */
async function removeChannel(row: ChannelRow) {
  try {
    await ElMessageBox.confirm(
      `确定删除渠道「${row.name}」吗？删除后不可恢复。`,
      '删除渠道',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await channelApi.remove(row.id)
    ElMessage.success('渠道已删除')
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
