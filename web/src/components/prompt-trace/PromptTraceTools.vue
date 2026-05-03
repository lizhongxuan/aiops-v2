<script setup>
import { computed, ref } from "vue";
import { CopyIcon, SearchIcon } from "lucide-vue-next";
import { formatCount } from "../../utils/promptTraceViewModel";

const props = defineProps({
  tools: { type: Object, default: () => ({ visible: [], risky: [], registryText: "" }) },
});

const emit = defineEmits(["copy"]);
const query = ref("");
const registryExpanded = ref(false);

const visibleTools = computed(() => props.tools?.visible || []);
const riskyTools = computed(() => props.tools?.risky || []);
const registryText = computed(() => props.tools?.registryText || "");
const filteredTools = computed(() => {
  const needle = query.value.trim().toLowerCase();
  if (!needle) return visibleTools.value;
  return visibleTools.value.filter((tool) => String(tool).toLowerCase().includes(needle));
});
</script>

<template>
  <div class="trace-tools" data-testid="prompt-trace-tools">
    <section class="trace-tools-section">
      <div class="trace-tools-header">
        <div>
          <div class="trace-section-title">Visible Tools</div>
          <div class="trace-muted">{{ formatCount(visibleTools.length) }} tools</div>
        </div>
        <label class="trace-tool-search">
          <SearchIcon size="15" />
          <input v-model="query" type="search" placeholder="搜索工具" />
        </label>
      </div>

      <div v-if="filteredTools.length" class="trace-chip-list">
        <span
          v-for="tool in filteredTools"
          :key="tool"
          class="trace-chip"
          :class="{ 'is-risky': riskyTools.includes(tool) }"
        >
          {{ tool }}
        </span>
      </div>
      <div v-else class="trace-empty-inline">没有匹配工具</div>
    </section>

    <section class="trace-tools-section">
      <div class="trace-tools-header">
        <div>
          <div class="trace-section-title">Tool Registry Prompt</div>
          <div class="trace-muted">{{ formatCount(registryText.length) }} chars</div>
        </div>
        <button
          type="button"
          class="trace-icon-btn"
          :disabled="!registryText"
          title="复制 tool registry"
          @click="emit('copy', { label: 'Tool Registry', value: registryText })"
        >
          <CopyIcon size="15" />
        </button>
      </div>

      <button
        v-if="registryText"
        type="button"
        class="trace-expand-btn"
        :aria-expanded="registryExpanded"
        @click="registryExpanded = !registryExpanded"
      >
        {{ registryExpanded ? "折叠 Tool Registry" : "展开完整 Tool Registry" }}
      </button>
      <pre v-if="registryText && registryExpanded" class="trace-tool-registry">{{ registryText }}</pre>
      <div v-else-if="!registryText" class="trace-empty-inline">没有 tool registry prompt</div>
    </section>
  </div>
</template>

<style scoped>
.trace-tools {
  display: grid;
  gap: 12px;
}

.trace-tools-section {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 14px;
}

.trace-tools-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.trace-section-title {
  margin-bottom: 4px;
  color: #6b7280;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.trace-muted,
.trace-empty-inline {
  color: #6b7280;
  font-size: 13px;
}

.trace-tool-search {
  display: flex;
  align-items: center;
  gap: 7px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 6px 9px;
  background: #ffffff;
  color: #6b7280;
}

.trace-tool-search input {
  width: 150px;
  border: 0;
  outline: 0;
  color: #111827;
}

.trace-chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.trace-chip {
  border: 1px solid #d1d5db;
  border-radius: 999px;
  padding: 3px 8px;
  color: #374151;
  background: #f9fafb;
  font-size: 12px;
}

.trace-chip.is-risky {
  border-color: #f59e0b;
  background: #fffbeb;
  color: #92400e;
}

.trace-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: #4b5563;
  cursor: pointer;
}

.trace-icon-btn:hover {
  background: #f3f4f6;
}

.trace-icon-btn:disabled {
  cursor: default;
  opacity: 0.45;
}

.trace-expand-btn {
  border: 1px solid #d1d5db;
  border-radius: 7px;
  background: #ffffff;
  padding: 6px 10px;
  color: #374151;
  cursor: pointer;
}

.trace-tool-registry {
  max-height: min(70vh, 720px);
  margin: 10px 0 0;
  padding: 12px;
  overflow: auto;
  border-radius: 7px;
  background: #111827;
  color: #e5e7eb;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
