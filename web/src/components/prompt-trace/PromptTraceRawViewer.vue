<script setup>
import { computed } from "vue";
import { CopyIcon, FileJsonIcon, FileTextIcon } from "lucide-vue-next";

const props = defineProps({
  options: { type: Array, default: () => [] },
  active: { type: String, default: "" },
  content: { type: String, default: "" },
  loading: { type: Boolean, default: false },
  error: { type: String, default: "" },
});

const emit = defineEmits(["update:active", "copy"]);

const activeOption = computed(() => props.options.find((item) => item.key === props.active) || props.options[0] || null);

function iconFor(key = "") {
  return key === "json" ? FileJsonIcon : FileTextIcon;
}
</script>

<template>
  <div class="trace-raw-viewer" data-testid="prompt-trace-raw">
    <div class="trace-raw-tabs">
      <button
        v-for="option in options"
        :key="option.key"
        type="button"
        class="trace-raw-tab"
        :class="{ 'is-active': option.key === activeOption?.key }"
        @click="emit('update:active', option.key)"
      >
        <component :is="iconFor(option.key)" size="15" />
        {{ option.label }}
      </button>
      <button type="button" class="trace-copy-btn" :disabled="!content" @click="emit('copy')">
        <CopyIcon size="15" />
        复制
      </button>
    </div>

    <div v-if="loading" class="trace-raw-empty">加载中...</div>
    <div v-else-if="error" class="trace-raw-error">{{ error }}</div>
    <pre v-else class="trace-raw-code">{{ content }}</pre>
  </div>
</template>

<style scoped>
.trace-raw-viewer {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  overflow: hidden;
}

.trace-raw-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px;
  border-bottom: 1px solid #e5e7eb;
  background: #f9fafb;
}

.trace-raw-tab,
.trace-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid #d1d5db;
  border-radius: 7px;
  background: #ffffff;
  padding: 6px 10px;
  color: #374151;
  cursor: pointer;
}

.trace-raw-tab.is-active {
  border-color: #2563eb;
  background: #eff6ff;
  color: #1d4ed8;
}

.trace-copy-btn:disabled {
  cursor: default;
  opacity: 0.45;
}

.trace-raw-code {
  min-height: 52vh;
  max-height: 68vh;
  margin: 0;
  padding: 16px;
  overflow: auto;
  background: #0f172a;
  color: #e5e7eb;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.trace-raw-empty,
.trace-raw-error {
  padding: 18px;
  color: #6b7280;
}

.trace-raw-error {
  color: #991b1b;
}
</style>
