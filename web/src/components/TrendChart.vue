<template>
  <div class="trend-chart">
    <div ref="chartEl" class="trend-chart__canvas"></div>
    <div v-if="!hasData" class="trend-chart__empty">
      <el-empty description="暂无趋势数据" :image-size="64" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

export interface TrendPoint {
  date: string
  total: number
  success: number
  failed?: number
}

const props = defineProps<{
  data?: TrendPoint[]
}>()

const chartEl = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const hasData = computed(() => Array.isArray(props.data) && props.data.length > 0)

function render() {
  const el = chartEl.value
  if (!el) return
  if (!chart) chart = echarts.init(el)

  const data = props.data || []

  chart.setOption({
    backgroundColor: 'transparent',
    animationDuration: 600,
    grid: { left: 8, right: 18, top: 34, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(30, 41, 59, 0.92)',
      borderColor: 'rgba(148, 163, 184, 0.22)',
      borderWidth: 1,
      padding: [10, 14],
      textStyle: { color: '#e2e8f0', fontSize: 12 },
      axisPointer: { type: 'line', lineStyle: { color: 'rgba(148, 163, 184, 0.32)' } },
      valueFormatter: (v: unknown) => String(v),
    },
    legend: {
      data: ['发送量', '成功', '失败'],
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 16,
      itemHeight: 4,
      itemGap: 18,
      textStyle: { color: '#94a3b8', fontSize: 12 },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map((d) => d.date),
      axisLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.2)' } },
      axisTick: { show: false },
      axisLabel: { color: '#64748b', fontSize: 12, margin: 12 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.1)' } },
      axisLabel: { color: '#64748b', fontSize: 12 },
    },
    series: [
      {
        name: '发送量',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        showSymbol: false,
        lineStyle: { width: 3, color: '#6366f1' },
        itemStyle: { color: '#6366f1' },
        emphasis: { focus: 'series' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(99, 102, 241, 0.32)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0)' },
          ]),
        },
        data: data.map((d) => d.total),
      },
      {
        name: '成功',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        showSymbol: false,
        lineStyle: { width: 3, color: '#34d399' },
        itemStyle: { color: '#34d399' },
        emphasis: { focus: 'series' },
        data: data.map((d) => d.success),
      },
      {
        name: '失败',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        showSymbol: false,
        lineStyle: { width: 3, color: '#f87171' },
        itemStyle: { color: '#f87171' },
        emphasis: { focus: 'series' },
        data: data.map((d) => d.failed ?? 0),
      },
    ],
  })
}

function onResize() {
  chart?.resize()
}

onMounted(() => {
  render()
  window.addEventListener('resize', onResize)
})

watch(
  () => props.data,
  () => render(),
  { deep: true }
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.trend-chart {
  position: relative;
  width: 100%;
  height: 320px;
}
.trend-chart__canvas {
  width: 100%;
  height: 100%;
}
.trend-chart__empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  pointer-events: none;
}
</style>
