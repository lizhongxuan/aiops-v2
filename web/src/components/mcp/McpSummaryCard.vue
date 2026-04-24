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

const rows = computed(() => {
  return Array.isArray(props.card?.rows) ? props.card.rows : [];
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
    <article class="summary-shell" data-testid="mcp-summary-card">
      <p v-if="!rows.length" class="summary-copy">{{ card.summary || "暂无摘要，可展开查看详情。" }}</p>
      <dl v-else class="summary-rows" data-testid="mcp-summary-rows">
        <div
          v-for="(row, idx) in rows"
          :key="idx"
          class="summary-row"
          :class="{ highlight: row.highlight }"
        >
          <dt class="row-label">{{ row.label }}</dt>
          <dd class="row-value">{{ row.value }}</dd>
        </div>
      </dl>
      <p v-if="rows.length && card.summary" class="summary-copy summary-extra">{{ card.summary }}</p>
    </article>
  </McpReadonlyCardFrame>
</template>

<style scoped>
.summary-shell {
  padding: 12px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(15, 23, 42, 0.06);
}

.summary-copy {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: #334155;
}

.summary-extra {
  margin-top: 8px;
}

.summary-rows {
  margin: 0;
  display: grid;
  gap: 6px;
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 4px 0;
  border-bottom: 1px solid rgba(226, 232, 240, 0.6);
}

.summary-row:last-child {
  border-bottom: none;
}

.summary-row.highlight {
  background: rgba(254, 243, 199, 0.5);
  border-radius: 6px;
  padding: 4px 8px;
}

.row-label {
  font-size: 13px;
  color: #64748b;
  font-weight: 500;
}

.row-value {
  margin: 0;
  font-size: 13px;
  color: #0f172a;
  font-weight: 600;
}
</style>
