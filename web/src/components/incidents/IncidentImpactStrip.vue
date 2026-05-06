<script setup>
import { computed } from "vue";

const props = defineProps({
  incident: {
    type: Object,
    default: () => ({}),
  },
  impact: {
    type: Object,
    default: () => ({}),
  },
});

function list(value) {
  return Array.isArray(value) ? value : [];
}

const capabilities = computed(() => list(props.impact.capabilities || props.incident.capabilities));
const tenants = computed(() => list(props.impact.tenants || props.incident.tenants || props.incident.affectedTenants));
const metrics = computed(() => list(props.impact.metrics || props.incident.metrics));
const capabilityLabel = computed(() => props.incident.businessCapability || capabilities.value[0]?.name || "待确认");
</script>

<template>
  <section class="incident-impact-strip" data-testid="incident-impact-strip">
    <div>
      <span>业务能力</span>
      <strong>{{ capabilityLabel }}</strong>
    </div>
    <div>
      <span>受影响租户</span>
      <strong>{{ tenants.length }}</strong>
    </div>
    <div>
      <span>关键指标</span>
      <strong>{{ metrics.length }}</strong>
    </div>
  </section>
</template>

<style scoped>
.incident-impact-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #e2e8f0;
}

.incident-impact-strip > div {
  min-width: 0;
  padding: 12px;
  background: #fff;
}

.incident-impact-strip span {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.incident-impact-strip strong {
  display: block;
  margin-top: 4px;
  overflow: hidden;
  color: #0f172a;
  font-size: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .incident-impact-strip {
    grid-template-columns: 1fr;
  }
}
</style>
