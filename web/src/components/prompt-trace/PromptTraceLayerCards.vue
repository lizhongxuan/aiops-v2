<script setup>
import { computed, ref, watch } from "vue";
import { ChevronDownIcon, ChevronRightIcon, CopyIcon } from "lucide-vue-next";
import { formatCount, formatIndex } from "../../utils/promptTraceViewModel";

const props = defineProps({
  layers: { type: Array, default: () => [] },
});

const emit = defineEmits(["copy"]);
const expanded = ref(new Set());

const sortedLayers = computed(() => [...(props.layers || [])].sort((left, right) => left.index - right.index));

watch(
  sortedLayers,
  (layers) => {
    const next = new Set();
    for (const layer of layers) {
      if (shouldDefaultExpand(layer)) next.add(layer.id);
    }
    expanded.value = next;
  },
  { immediate: true },
);

function shouldDefaultExpand(layer) {
  return (layer.warnings || []).length > 0;
}

function isExpanded(layer) {
  return expanded.value.has(layer.id);
}

function toggle(layer) {
  const next = new Set(expanded.value);
  if (next.has(layer.id)) next.delete(layer.id);
  else next.add(layer.id);
  expanded.value = next;
}

function copyLayer(layer) {
  emit("copy", { label: layer.title, value: layer.content });
}
</script>

<template>
  <div class="trace-layer-list" data-testid="prompt-trace-layers">
    <article v-for="layer in sortedLayers" :key="layer.id" class="trace-layer-card">
      <header class="trace-layer-header">
        <button
          type="button"
          class="trace-layer-toggle"
          :aria-expanded="isExpanded(layer)"
          @click="toggle(layer)"
        >
          <ChevronDownIcon v-if="isExpanded(layer)" size="16" />
          <ChevronRightIcon v-else size="16" />
          <span class="trace-layer-title">{{ formatIndex(layer.index) }} {{ layer.title }}</span>
        </button>
        <button type="button" class="trace-icon-btn" title="复制本层内容" @click="copyLayer(layer)">
          <CopyIcon size="15" />
        </button>
      </header>

      <div class="trace-layer-meta">
        <span>{{ layer.providerRole || "role -" }}</span>
        <span>{{ layer.semanticRole || "semantic -" }}</span>
        <span>{{ layer.promptLayer || "layer -" }}</span>
        <span>{{ formatCount(layer.charCount) }} chars</span>
        <span>{{ formatCount(layer.lineCount) }} lines</span>
        <span v-if="layer.shortHash">hash {{ layer.shortHash }}</span>
      </div>

      <div v-if="layer.warnings?.length" class="trace-layer-warnings">
        <span v-for="item in layer.warnings" :key="item">{{ item }}</span>
      </div>

      <pre v-if="isExpanded(layer)" class="trace-layer-content">{{ layer.content }}</pre>
    </article>

    <div v-if="!sortedLayers.length" class="trace-empty">暂无 modelInput messages</div>
  </div>
</template>

<style scoped>
.trace-layer-list {
  display: grid;
  gap: 10px;
}

.trace-layer-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  overflow: hidden;
}

.trace-layer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid #eef2f7;
}

.trace-layer-toggle,
.trace-icon-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  color: #111827;
  cursor: pointer;
}

.trace-layer-title {
  font-weight: 700;
}

.trace-icon-btn {
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 7px;
  color: #4b5563;
}

.trace-icon-btn:hover,
.trace-layer-toggle:hover {
  background: #f3f4f6;
}

.trace-layer-meta,
.trace-layer-warnings {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  padding: 10px 12px 0;
}

.trace-layer-meta span,
.trace-layer-warnings span {
  border: 1px solid #d1d5db;
  border-radius: 999px;
  padding: 2px 7px;
  color: #4b5563;
  background: #f9fafb;
  font-size: 12px;
}

.trace-layer-warnings span {
  border-color: #f59e0b;
  background: #fffbeb;
  color: #92400e;
}

.trace-layer-content {
  max-height: min(70vh, 720px);
  margin: 10px 0 0;
  padding: 12px;
  overflow: auto;
  border-top: 1px solid #f3f4f6;
  background: #111827;
  color: #e5e7eb;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.trace-empty {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 18px;
  background: #ffffff;
  color: #6b7280;
}
</style>
