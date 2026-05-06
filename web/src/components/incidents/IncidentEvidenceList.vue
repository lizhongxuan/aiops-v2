<script setup>
import { computed } from "vue";

const props = defineProps({
  evidence: {
    type: Array,
    default: () => [],
  },
  emptyText: {
    type: String,
    default: "暂无证据",
  },
});

const rows = computed(() => props.evidence.filter(Boolean));
</script>

<template>
  <section class="incident-evidence-list" data-testid="incident-evidence-list">
    <div v-if="!rows.length" class="incident-evidence-empty">{{ emptyText }}</div>
    <article v-for="item in rows" :key="item.id || item.rawRef || item.title" class="incident-evidence-item">
      <div class="incident-evidence-head">
        <span>{{ item.source || "source" }}</span>
        <time>{{ item.createdAt || item.time || "" }}</time>
      </div>
      <strong>{{ item.title || item.summary || item.rawRef || "证据" }}</strong>
      <p v-if="item.summary">{{ item.summary }}</p>
      <code v-if="item.rawRef">{{ item.rawRef }}</code>
    </article>
  </section>
</template>

<style scoped>
.incident-evidence-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.incident-evidence-empty,
.incident-evidence-item {
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.incident-evidence-empty {
  color: #64748b;
}

.incident-evidence-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: #64748b;
  font-size: 12px;
}

.incident-evidence-item strong {
  display: block;
  margin-top: 6px;
  color: #0f172a;
  font-size: 13px;
}

.incident-evidence-item p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 1.55;
}

.incident-evidence-item code {
  display: inline-block;
  margin-top: 8px;
  color: #475569;
  font-size: 12px;
}
</style>
