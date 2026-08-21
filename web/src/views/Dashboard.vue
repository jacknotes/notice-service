<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">仪表盘</h1>
        <p class="sub">按日期查看投递状态与趋势（默认最近一周）</p>
      </div>
    </div>

    <el-alert
      v-if="error"
      :title="error"
      type="error"
      :closable="false"
      show-icon
      class="mb-4"
    />

    <div class="filters">
      <div class="date-quick">
        <el-button
          v-for="p in quickPresets"
          :key="p.key"
          size="small"
          :type="quickPreset === p.key ? 'primary' : 'default'"
          plain
          @click="applyPreset(p)"
        >
          {{ p.label }}
        </el-button>
      </div>
      <el-date-picker
        v-model="dateRange"
        type="daterange"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        value-format="YYYY-MM-DD"
        :clearable="false"
        style="width: 240px"
        @change="onDateRangeChange"
      />
      <span class="range-hint mono">{{ dateRange ? `${dateRange[0]} ~ ${dateRange[1]}` : '' }}</span>
    </div>

    <div v-loading="loading" class="stat-grid">
      <StatCard label="发送量" :value="stats.today_total" color="#6366f1" hint="区间累计投递请求" :delay="0" />
      <StatCard label="成功" :value="stats.today_success" color="#34d399" hint="成功送达的通知" :delay="1" />
      <StatCard label="失败" :value="stats.today_failed" color="#f87171" hint="需要关注的失败回执" :delay="2" />
      <StatCard label="成功率" :value="rateDisplay" suffix="%" color="#8b5cf6" hint="成功 ÷ 发送量" :delay="3" />
      <StatCard label="任务数" :value="stats.task_count" color="#38bdf8" hint="区间内有投递的任务" :delay="4" />
      <StatCard label="渠道数" :value="stats.channel_count" color="#f59e0b" hint="区间内使用的渠道" :delay="5" />
    </div>

    <div class="charts-row">
      <div v-loading="loading" class="card chart-card donut-card reveal d-1">
        <div class="chart-head">
          <div class="chart-title">
            <h3>状态分布</h3>
            <span class="chart-sub mono">STATUS MIX</span>
          </div>
        </div>
        <div ref="donutEl" class="donut-canvas"></div>
      </div>

      <div v-loading="loading" class="card chart-card reveal d-2">
        <div class="chart-head">
          <div class="chart-title">
            <h3>发送趋势</h3>
            <span class="chart-sub mono">DAILY THROUGHPUT</span>
          </div>
        </div>
        <TrendChart :data="trend" />
      </div>
    </div>

    <div class="lists-row">
      <div v-loading="loading" class="card list-card reveal d-3">
        <div class="chart-head">
          <div class="chart-title">
            <h3>TOP 任务</h3>
            <span class="chart-sub mono">TOP TASKS</span>
          </div>
        </div>
        <div v-if="topTasks.length" class="rank-list">
          <div v-for="(t, i) in topTasks" :key="t.task_id" class="rank-item">
            <span class="rank-no mono">{{ i + 1 }}</span>
            <div class="rank-main">
              <span class="rank-name">{{ taskName(t.task_id) }}</span>
              <div class="rank-bar">
                <span class="rank-bar-fill" :style="{ width: rankWidth(t) + '%' }"></span>
              </div>
            </div>
            <span class="rank-num mono">{{ t.total }}<span class="rank-succ mono">/{{ t.success }} 成功</span></span>
          </div>
        </div>
        <el-empty v-else description="暂无数据" :image-size="56" />
      </div>

      <div v-loading="loading" class="card list-card reveal d-4">
        <div class="chart-head">
          <div class="chart-title">
            <h3>渠道分布</h3>
            <span class="chart-sub mono">CHANNEL MIX</span>
          </div>
        </div>
        <div v-if="channelStats.length" class="rank-list">
          <div v-for="c in channelStats" :key="c.channel_id" class="rank-item">
            <span class="rank-no mono">▸</span>
            <div class="rank-main">
              <span class="rank-name">{{ channelName(c.channel_id) }}</span>
              <div class="rank-bar">
                <span class="rank-bar-fill ch" :style="{ width: channelWidth(c) + '%' }"></span>
              </div>
            </div>
            <span class="rank-num mono">{{ c.total }}<span class="rank-succ mono">/{{ c.success }} 成功</span></span>
          </div>
        </div>
        <el-empty v-else description="暂无数据" :image-size="56" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import { channelApi, dashboardApi, taskApi } from '@/api'
import StatCard from '@/components/StatCard.vue'
import TrendChart from '@/components/TrendChart.vue'
import type { TrendPoint } from '@/components/TrendChart.vue'

const loading = ref(false)
const error = ref('')

const stats = reactive({
  today_total: 0,
  today_success: 0,
  today_failed: 0,
  success_rate: 0,
  task_count: 0,
  channel_count: 0,
})

const trend = ref<TrendPoint[]>([])
const topTasks = ref<{ task_id: number; total: number; success: number; failed: number }[]>([])
const channelStats = ref<{ channel_id: number; total: number; success: number; failed: number }[]>([])
const tasks = ref<{ id: number; name: string }[]>([])
const channels = ref<{ id: number; name: string }[]>([])

const rateDisplay = computed(() =>
  Number.isFinite(stats.success_rate) ? Math.round(stats.success_rate * 10) / 10 : 0
)

const taskName = (id: number) => tasks.value.find((t) => t.id === id)?.name || `任务 #${id}`
const channelName = (id: number) => channels.value.find((c) => c.id === id)?.name || `渠道 #${id}`

/* ── 日期范围（默认近 7 天） ─────────────────────────────────────────── */
const quickPresets = [
  { key: 'week', label: '近 7 天', days: 7 },
  { key: '14d', label: '近 14 天', days: 14 },
  { key: '30d', label: '近 30 天', days: 30 },
]
const quickPreset = ref('week')
const dateRange = ref<[string, string] | null>(null)

function fmtDate(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function applyPreset(p: { key: string; days: number }) {
  quickPreset.value = p.key
  const end = new Date(Date.now() + 86400000) // to 排他
  const start = new Date(end.getTime() - p.days * 86400000)
  dateRange.value = [fmtDate(start), fmtDate(end)]
  load()
}

function onDateRangeChange() {
  if (!dateRange.value) {
    applyPreset(quickPresets[0])
    return
  }
  quickPreset.value = ''
  load()
}

applyPreset(quickPresets[0])

/* ── 状态环形图 ─────────────────────────────────────────────────────── */
const donutEl = ref<HTMLDivElement | null>(null)
let donut: echarts.ECharts | null = null

function renderDonut() {
  const el = donutEl.value
  if (!el) return
  if (!donut) donut = echarts.init(el)
  const success = stats.today_success
  const failed = stats.today_failed
  const empty = success + failed === 0
  donut.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, icon: 'circle', itemWidth: 8, itemHeight: 8, textStyle: { color: '#94a3b8', fontSize: 12 } },
    series: [
      {
        type: 'pie',
        radius: ['52%', '76%'],
        center: ['50%', '44%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 6, borderColor: 'rgba(15,23,42,0.6)', borderWidth: 2 },
        label: { show: false },
        emphasis: { label: { show: true, fontSize: 16, fontWeight: 700, color: '#e2e8f0' } },
        labelLine: { show: false },
        data: empty
          ? [{ name: '暂无数据', value: 1, itemStyle: { color: 'rgba(148,163,184,0.15)' } }]
          : [
              { name: '成功', value: success, itemStyle: { color: '#34d399' } },
              { name: '失败', value: failed, itemStyle: { color: '#f87171' } },
            ],
      },
    ],
    graphic: empty
      ? []
      : [
          {
            type: 'text',
            left: 'center',
            top: '38%',
            style: {
              text: `${rateDisplay.value}%`,
              fill: '#e2e8f0',
              fontSize: 22,
              fontWeight: 700,
            },
          },
        ],
  })
}

function onResize() {
  donut?.resize()
}

/* ── TOP 任务 / 渠道横条 ────────────────────────────────────────────── */
function rankWidth(t: { total: number; success: number }) {
  const max = Math.max(...topTasks.value.map((x) => x.total), 1)
  return Math.max((t.total / max) * 100, 4)
}
function channelWidth(c: { total: number; success: number }) {
  const max = Math.max(...channelStats.value.map((x) => x.total), 1)
  return Math.max((c.total / max) * 100, 4)
}

/* ── 加载 ───────────────────────────────────────────────────────────── */
async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = dateRange.value ? { from: dateRange.value[0], to: dateRange.value[1] } : {}
    const [s, t, top, chs] = await Promise.all([
      dashboardApi.stats(params),
      dashboardApi.trend(params),
      dashboardApi.topTasks(params),
      dashboardApi.channelStats(params),
    ])
    Object.assign(stats, s || {})
    trend.value = Array.isArray(t) ? t : []
    topTasks.value = Array.isArray(top) ? top : []
    channelStats.value = Array.isArray(chs) ? chs : []
    renderDonut()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '仪表盘数据加载失败，请稍后重试'
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

async function loadOptions() {
  try {
    const [ts, chs] = await Promise.all([taskApi.list(), channelApi.list()])
    tasks.value = ts || []
    channels.value = chs || []
    load()
  } catch {
    load()
  }
}

watch(() => stats, renderDonut, { deep: true })

onMounted(() => {
  loadOptions()
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  donut?.dispose()
  donut = null
})
</script>

<style scoped>
.filters {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  margin-bottom: var(--space-5);
}
.date-quick {
  display: flex;
  gap: 6px;
}
.range-hint {
  color: var(--text-faint);
  font-size: 11px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: var(--space-4);
  margin-bottom: var(--space-6);
  min-height: 132px;
}

.charts-row {
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}
.lists-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
}
.chart-card,
.list-card {
  padding: 22px 24px 18px;
}
.donut-canvas {
  height: 300px;
  width: 100%;
}

.chart-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.chart-title h3 {
  font-size: var(--text-md);
  font-weight: 600;
  color: var(--text-primary);
}
.chart-sub {
  margin-top: 3px;
  display: block;
  color: var(--text-faint);
  font-size: 10px;
  letter-spacing: 0.22em;
}

/* ── TOP / 渠道列表 ─────────────────────────────────────────────────── */
.rank-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-4);
}
.rank-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.rank-no {
  width: 20px;
  color: var(--text-faint);
  font-size: var(--text-xs);
}
.rank-main {
  flex: 1;
  min-width: 0;
}
.rank-name {
  display: block;
  color: var(--text-secondary);
  font-size: var(--text-xs);
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rank-bar {
  height: 6px;
  border-radius: var(--radius-pill);
  background: rgba(148, 163, 184, 0.12);
  overflow: hidden;
}
.rank-bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: linear-gradient(90deg, var(--indigo-500), var(--violet-500));
}
.rank-bar-fill.ch {
  background: linear-gradient(90deg, #0ea5e9, #38bdf8);
}
.rank-num {
  color: var(--text-secondary);
  font-size: var(--text-sm);
  font-weight: 600;
  white-space: nowrap;
}
.rank-succ {
  margin-left: 6px;
  color: var(--text-faint);
  font-size: 11px;
  font-weight: 400;
}

@media (max-width: 1200px) {
  .stat-grid { grid-template-columns: repeat(3, 1fr); }
  .charts-row { grid-template-columns: 1fr; }
  .lists-row { grid-template-columns: 1fr; }
}
@media (max-width: 768px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); gap: var(--space-3); }
  .chart-card, .list-card { padding: 18px 16px 14px; }
}
@media (max-width: 480px) {
  .stat-grid { grid-template-columns: 1fr; }
}
</style>
