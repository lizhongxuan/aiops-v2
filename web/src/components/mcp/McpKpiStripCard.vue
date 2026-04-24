<script setup>
import { computed } from "vue";
import McpReadonlyCardFrame from "./McpReadonlyCardFrame.vue";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  embedded: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["detail", "refresh"]);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

const kpis = computed(() => {
  const visual = asObject(props.card?.visual);
  return asArray(props.card?.kpis || visual.kpis || visual.items || visual.metrics)
    .map((item, index) => {
      const source = asObject(item);
      return {
        id: source.id || `kpi-${index + 1}`,
        label: source.label || source.name || `指标 ${index + 1}`,
        value: source.value ?? source.current ?? "--",
        delta: source.delta || source.change || "",
        tone: source.tone || source.status || source.color || "neutral",
      };
    });
});

function forwardDetail(payload) {
  emit("detail", payload);
}

function forwardRefresh(payload) {
  emit("refresh", payload);
}
</script>

<template>
  <McpReadonlyCardFrame
    :card="card"
    :embedded="embedded"
    @detail="forwardDetail"
    @refresh="forwardRefresh"
  >
    <div class="kpi-grid">
      <article
        v-for="kpi in kpis"
        :key="kpi.id"
        class="kpi-tile"
        :data-tone="kpi.tone"
        data-testid="mcp-kpi-tile"
      >
        <span class="kpi-label">{{ kpi.label }}</span>
        <strong class="kpi-value">{{ kpi.value }}</strong>
        <span v-if="kpi.delta" class="kpi-delta">{{ kpi.delta }}</span>
      </article>
    </div>
  </McpReadonlyCardFrame>
</template>

<style scoped>
.kpi-grid {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
}

.kpi-tile {
  display: grid;
  gap: 4px;
  padding: 12px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(15, 23, 42, 0.06);
}

.kpi-tile[data-tone="positive"] {
  background: rgba(236, 253, 245, 0.95);
}

.kpi-tile[data-tone="warning"] {
  background: rgba(255, 251, 235, 0.95);
}

.kpi-tile[data-tone="danger"] {
  background: rgba(254, 242, 242, 0.95);
}

.kpi-label {
  font-size: 12px;
  color: #64748b;
}

.kpi-value {
  font-size: 22px;
  line-height: 1.1;
  color: #0f172a;
}

.kpi-delta {
  font-size: 12px;
  color: #475569;
}
</style>
