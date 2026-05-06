<script setup>
import { computed } from "vue";

const props = defineProps({
  matches: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => props.matches.filter(Boolean));

function scoreLabel(value) {
  const score = Number(value || 0);
  if (!Number.isFinite(score)) return "0%";
  return `${Math.round(score * 100)}%`;
}
</script>

<template>
  <section class="runbook-match-result" data-testid="runbook-match-result">
    <div v-if="!rows.length" class="match-empty">暂无匹配结果</div>
    <article v-for="item in rows" :key="item.id || item.runbookId || item.title">
      <strong>{{ item.title || item.name || item.id || item.runbookId }}</strong>
      <span>{{ scoreLabel(item.score) }}</span>
    </article>
  </section>
</template>

<style scoped>
.runbook-match-result {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.runbook-match-result article,
.match-empty {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.runbook-match-result strong {
  color: #0f172a;
  font-size: 13px;
}

.runbook-match-result span,
.match-empty {
  color: #166534;
  font-size: 12px;
  font-weight: 800;
}
</style>
