<script setup>
import { computed } from "vue";

const props = defineProps({
  items: {
    type: Array,
    default: () => [],
  },
  emptyText: {
    type: String,
    default: "暂无时间线",
  },
});

const rows = computed(() => props.items.filter(Boolean));
</script>

<template>
  <ol class="incident-timeline" data-testid="incident-timeline">
    <li v-if="!rows.length" class="is-empty">{{ emptyText }}</li>
    <li v-for="item in rows" :key="item.id || item.createdAt || item.title">
      <time>{{ item.createdAt || item.time || "时间待定" }}</time>
      <strong>{{ item.title || item.summary || item.source || "事件" }}</strong>
      <p v-if="item.summary">{{ item.summary }}</p>
    </li>
  </ol>
</template>

<style scoped>
.incident-timeline {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.incident-timeline li {
  padding-left: 12px;
  border-left: 2px solid #d8e2ee;
}

.incident-timeline time {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.incident-timeline strong {
  display: block;
  margin-top: 2px;
  color: #0f172a;
  font-size: 13px;
}

.incident-timeline p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 1.55;
}

.incident-timeline .is-empty {
  color: #64748b;
}
</style>
