<script setup>
defineProps({
  action: {
    type: Object,
    default: () => ({}),
  },
});

function refLabel(ref = {}) {
  if (!ref || typeof ref !== "object") return "";
  return [ref.kind, ref.id].filter(Boolean).join(":");
}
</script>

<template>
  <article class="action-proposal-card" data-testid="action-proposal-card">
    <strong>{{ action.title || action.summary || "行动项" }}</strong>
    <span v-if="action.owner">{{ action.owner }}</span>
    <code v-if="refLabel(action.ref)">{{ refLabel(action.ref) }}</code>
  </article>
</template>

<style scoped>
.action-proposal-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.action-proposal-card strong {
  color: #0f172a;
  font-size: 13px;
}

.action-proposal-card span,
.action-proposal-card code {
  color: #64748b;
  font-size: 12px;
}
</style>
