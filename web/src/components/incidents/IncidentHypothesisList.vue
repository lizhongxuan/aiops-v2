<script setup>
import { computed } from "vue";

const props = defineProps({
  hypotheses: {
    type: Array,
    default: () => [],
  },
});

const rows = computed(() => [...props.hypotheses].filter(Boolean).sort((left, right) => Number(right.score || 0) - Number(left.score || 0)));
</script>

<template>
  <section class="incident-hypothesis-list" data-testid="incident-hypothesis-list">
    <div v-if="!rows.length" class="incident-hypothesis-empty">暂无 hypothesis</div>
    <article v-for="item in rows" :key="item.id || item.title" class="incident-hypothesis-item">
      <div>
        <strong>{{ item.title || item.summary || "候选根因" }}</strong>
        <p v-if="item.evidence">{{ item.evidence }}</p>
      </div>
      <span>{{ Math.round(Number(item.score || 0) * 100) }}%</span>
    </article>
  </section>
</template>

<style scoped>
.incident-hypothesis-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.incident-hypothesis-empty,
.incident-hypothesis-item {
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.incident-hypothesis-empty {
  color: #64748b;
}

.incident-hypothesis-item {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.incident-hypothesis-item strong {
  color: #0f172a;
  font-size: 13px;
}

.incident-hypothesis-item p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 1.55;
}

.incident-hypothesis-item span {
  flex: 0 0 auto;
  color: #166534;
  font-size: 13px;
  font-weight: 800;
}
</style>
