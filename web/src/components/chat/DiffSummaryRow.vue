<script setup>
import { computed } from "vue";

const props = defineProps({
  diff: {
    type: Object,
    default: null,
  },
});

const filesCount = computed(() => Number(props.diff?.filesCount || props.diff?.files?.length || 0));
const added = computed(() => Number(props.diff?.addedLines || 0));
const removed = computed(() => Number(props.diff?.removedLines || 0));
const summary = computed(() => String(props.diff?.summary || "").trim());
</script>

<template>
  <div v-if="diff" class="diff-summary-row" data-testid="diff-summary-row">
    <span class="diff-label">变更</span>
    <strong>{{ summary || `${filesCount} 个文件` }}</strong>
    <span class="diff-stat">{{ filesCount }} files</span>
    <span class="diff-add">+{{ added }}</span>
    <span class="diff-remove">-{{ removed }}</span>
  </div>
</template>

<style scoped>
.diff-summary-row {
  display: flex;
  align-items: center;
  gap: 9px;
  width: min(920px, 100%);
  margin: 2px auto;
  padding: 8px 0;
  color: #52525b;
  font-size: 13.5px;
}

.diff-label,
.diff-stat {
  color: #9ca3af;
  font-size: 12px;
}

.diff-summary-row strong {
  color: #262626;
  font-weight: 500;
}

.diff-add {
  color: #15803d;
}

.diff-remove {
  color: #b91c1c;
}
</style>
