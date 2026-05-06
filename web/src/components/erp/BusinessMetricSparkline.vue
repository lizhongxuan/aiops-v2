<script setup>
import { computed } from "vue";

const props = defineProps({
  metric: {
    type: Object,
    default: () => ({}),
  },
});

const values = computed(() => Array.isArray(props.metric.trend) ? props.metric.trend.map((value) => Number(value)).filter(Number.isFinite) : []);
const bars = computed(() => {
  const max = Math.max(...values.value, 1);
  return values.value.map((value) => Math.max(10, Math.round((value / max) * 42)));
});
</script>

<template>
  <article class="business-metric-sparkline" data-testid="business-metric-sparkline">
    <div>
      <strong>{{ metric.name || metric.id || "metric" }}</strong>
      <span>{{ metric.value ?? metric.current ?? "-" }}</span>
    </div>
    <div class="spark-bars" aria-hidden="true">
      <i v-for="(height, index) in bars" :key="index" :style="{ height: `${height}px` }" />
    </div>
  </article>
</template>

<style scoped>
.business-metric-sparkline {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.business-metric-sparkline strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.business-metric-sparkline span {
  color: #64748b;
  font-size: 12px;
}

.spark-bars {
  display: flex;
  align-items: end;
  gap: 3px;
  height: 44px;
}

.spark-bars i {
  width: 5px;
  border-radius: 999px;
  background: #22c55e;
}
</style>
