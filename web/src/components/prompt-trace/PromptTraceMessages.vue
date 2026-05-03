<script setup>
import { computed, ref, watch } from "vue";
import { CopyIcon } from "lucide-vue-next";
import { formatCount, formatIndex } from "../../utils/promptTraceViewModel";

const props = defineProps({
  messages: { type: Array, default: () => [] },
});

const emit = defineEmits(["copy"]);
const expanded = ref(new Set());

const sortedMessages = computed(() => [...(props.messages || [])].sort((left, right) => left.index - right.index));

watch(
  sortedMessages,
  () => {
    expanded.value = new Set();
  },
  { immediate: true },
);

function isExpanded(message) {
  return expanded.value.has(message.id);
}

function toggle(message) {
  const next = new Set(expanded.value);
  if (next.has(message.id)) next.delete(message.id);
  else next.add(message.id);
  expanded.value = next;
}
</script>

<template>
  <div class="trace-message-list" data-testid="prompt-trace-messages">
    <article v-for="message in sortedMessages" :key="message.id" class="trace-message-row">
      <button
        type="button"
        class="trace-message-main"
        :aria-expanded="isExpanded(message)"
        @click="toggle(message)"
      >
        <span class="trace-message-index">{{ formatIndex(message.index) }}</span>
        <span class="trace-message-title">{{ message.providerRole || "role -" }}</span>
        <span class="trace-badge">{{ message.semanticRole || "semantic -" }}</span>
        <span class="trace-badge">{{ message.promptLayer || "layer -" }}</span>
        <span class="trace-message-size">{{ formatCount(message.charCount) }} chars</span>
      </button>
      <button type="button" class="trace-icon-btn" title="复制 message" @click="emit('copy', { label: message.title, value: message.content })">
        <CopyIcon size="15" />
      </button>
      <div v-if="message.toolCallCount || message.toolCallId" class="trace-message-tools">
        <span v-if="message.toolCallCount">{{ message.toolCallCount }} tool calls</span>
        <span v-if="message.toolCallId">toolCallId {{ message.toolCallId }}</span>
      </div>
      <pre v-if="isExpanded(message)" class="trace-message-content">{{ message.content }}</pre>
    </article>

    <div v-if="!sortedMessages.length" class="trace-empty">暂无 provider messages</div>
  </div>
</template>

<style scoped>
.trace-message-list {
  display: grid;
  gap: 9px;
}

.trace-message-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 10px;
}

.trace-message-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-width: 0;
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.trace-message-index {
  color: #111827;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 700;
}

.trace-message-title {
  color: #111827;
  font-weight: 700;
}

.trace-badge,
.trace-message-size,
.trace-message-tools span {
  border: 1px solid #d1d5db;
  border-radius: 999px;
  padding: 2px 7px;
  color: #4b5563;
  background: #f9fafb;
  font-size: 12px;
}

.trace-message-tools {
  grid-column: 1 / -1;
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
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

.trace-icon-btn:hover,
.trace-message-main:hover {
  background: #f3f4f6;
}

.trace-message-content {
  grid-column: 1 / -1;
  max-height: min(70vh, 720px);
  margin: 0;
  padding: 10px;
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

.trace-empty {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 18px;
  background: #ffffff;
  color: #6b7280;
}
</style>
