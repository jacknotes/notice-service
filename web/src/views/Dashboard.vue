<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1 class="grad-text">仪表盘</h1>
        <p class="sub">今日投递状态与近 7 天发送趋势</p>
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

    <div v-loading="loading" class="stat-grid">
      <StatCard
        label="今日发送量"
        :value="stats.today_total"
        color="#6366f1"
        hint="全天累计投递请求"
        :delay="0"
      />
      <StatCard
        label="今日成功"
        :value="stats.today_success"
        color="#34d399"
        hint="成功送达的通知"
        :delay="1"
      />
      <StatCard
        label="今日失败"
        :value="stats.today_failed"
        color="#f87171"
        hint="需要关注的失败回执"
        :delay="2"
      />
      <StatCard
        label="成功率"
        :value="rateDisplay"
        suffix="%"
        color="#8b5cf6"
        hint="今日成功 ÷ 今日发送量"
        :delay="3"
      />
    </div>

    <div v-loading="loading" class="card chart-card reveal d-2">
      <div class="chart-head">
        <div class="chart-title">
          <h3>近 7 天发送趋势</h3>
          <span class="chart-sub mono">7-DAY THROUGHPUT</span>
        </div>
        <div class="chart-legend">
          <span class="lg"><i class="lg-dot indigo"></i>发送量</span>
          <span class="lg"><i class="lg-dot emerald"></i>成功</span>
        </div>
      </div>
      <TrendChart :data="trend" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { dashboardApi } from '@/api'
import StatCard from '@/components/StatCard.vue'
import TrendChart from '@/components/TrendChart.vue'

const loading = ref(false)
const error = ref('')

const stats = reactive({
  today_total: 0,
  today_success: 0,
  today_failed: 0,
  success_rate: 0,
})

const trend = ref<{ date: string; total: number; success: number }[]>([])

const rateDisplay = computed(() =>
  Number.isFinite(stats.success_rate)
    ? Math.round(stats.success_rate * 10) / 10
    : 0
)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [s, t] = await Promise.all([dashboardApi.stats(), dashboardApi.trend()])
    Object.assign(stats, s || {})
    trend.value = Array.isArray(t) ? t : []
  } catch (e: any) {
    error.value = e?.response?.data?.error || '仪表盘数据加载失败，请稍后重试'
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
  margin-bottom: var(--space-6);
  min-height: 132px;
}

.chart-card {
  padding: 22px 24px 18px;
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
.chart-legend {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  color: var(--text-secondary);
  font-size: var(--text-xs);
}
.lg {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.lg-dot {
  width: 16px;
  height: 4px;
  border-radius: var(--radius-pill);
}
.lg-dot.indigo { background: #6366f1; box-shadow: 0 0 8px rgba(99, 102, 241, 0.6); }
.lg-dot.emerald { background: #34d399; box-shadow: 0 0 8px rgba(52, 211, 153, 0.5); }

@media (max-width: 1080px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
}
@media (max-width: 768px) {
  .stat-grid { grid-template-columns: 1fr 1fr; gap: var(--space-3); }
  .chart-card { padding: 18px 16px 14px; }
}
@media (max-width: 480px) {
  .stat-grid { grid-template-columns: 1fr; }
}
</style>
