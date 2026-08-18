<template>
  <div class="card stat-card reveal" :style="cardStyle">
    <div class="stat-top">
      <span class="stat-label">{{ label }}</span>
      <span class="stat-dot"></span>
    </div>

    <div class="stat-value">
      <span class="stat-num mono">{{ formatted }}</span>
      <span v-if="suffix" class="stat-suffix">{{ suffix }}</span>
    </div>

    <p v-if="hint" class="stat-hint">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    value: number | string
    suffix?: string
    color?: string
    hint?: string
    delay?: number
  }>(),
  {
    suffix: '',
    color: 'var(--indigo-400)',
    hint: '',
    delay: 0,
  }
)

const formatted = computed(() => {
  const v = props.value
  if (typeof v === 'number' && Number.isFinite(v)) return v.toLocaleString('zh-CN')
  return String(v ?? 0)
})

const cardStyle = computed(() => ({
  '--accent': props.color,
  '--accent-soft': `color-mix(in srgb, ${props.color} 16%, transparent)`,
  '--accent-faint': `color-mix(in srgb, ${props.color} 8%, transparent)`,
  '--d': props.delay,
}))
</script>

<style scoped>
.stat-card {
  padding: 20px 22px 18px;
  overflow: hidden;
}
.stat-card::before {
  content: '';
  position: absolute;
  inset: 0 0 auto 0;
  height: 2px;
  background: linear-gradient(90deg, var(--accent), transparent 72%);
  opacity: 0.9;
}

.card.stat-card:hover {
  border-color: var(--accent-soft);
  box-shadow: var(--shadow-float);
}

.stat-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.stat-label {
  color: var(--text-secondary);
  font-size: var(--text-sm);
  font-weight: 500;
  letter-spacing: 0.02em;
}
.stat-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 4px var(--accent-soft), 0 0 14px var(--accent);
}

.stat-value {
  display: flex;
  align-items: baseline;
  gap: 5px;
  margin-top: 14px;
}
.stat-num {
  font-family: var(--font-mono);
  font-size: 30px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: 0.01em;
  color: var(--accent);
  font-variant-numeric: tabular-nums;
}
.stat-suffix {
  font-size: var(--text-md);
  font-weight: 600;
  color: var(--accent);
}

.stat-hint {
  margin-top: 10px;
  color: var(--text-muted);
  font-size: var(--text-xs);
  letter-spacing: 0.01em;
}
</style>
