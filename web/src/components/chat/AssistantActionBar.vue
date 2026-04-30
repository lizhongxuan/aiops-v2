<script setup>
import { ref } from "vue";
import {
  CheckIcon,
  CopyIcon,
  PanelRightOpenIcon,
  RefreshCwIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
} from "lucide-vue-next";

const props = defineProps({
  copyText: {
    type: String,
    default: "",
  },
  allowRegenerate: {
    type: Boolean,
    default: false,
  },
  allowCopy: {
    type: Boolean,
    default: true,
  },
  allowFeedback: {
    type: Boolean,
    default: false,
  },
  regenerateLabel: {
    type: String,
    default: "重新生成",
  },
  feedback: {
    type: String,
    default: "",
  },
  canOpenPanel: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["regenerate", "open-panel", "update:feedback"]);
const copied = ref(false);

async function handleCopy() {
  if (!props.copyText || copied.value) return;
  try {
    await navigator.clipboard.writeText(props.copyText);
    copied.value = true;
    window.setTimeout(() => {
      copied.value = false;
    }, 1500);
  } catch (error) {
    console.error("Failed to copy final answer:", error);
  }
}

function toggleFeedback(value) {
  emit("update:feedback", props.feedback === value ? "" : value);
}
</script>

<template>
  <div class="assistant-action-bar" data-testid="assistant-action-bar">
    <button
      v-if="allowCopy"
      type="button"
      class="assistant-action-btn assistant-action-copy-btn"
      data-testid="assistant-action-copy"
      @click="handleCopy"
    >
      <CheckIcon v-if="copied" size="14" />
      <CopyIcon v-else size="14" />
      <span class="assistant-action-label">{{ copied ? "已复制" : "复制" }}</span>
    </button>

    <button
      v-if="allowRegenerate"
      type="button"
      class="assistant-action-btn"
      data-testid="assistant-action-regenerate"
      @click="$emit('regenerate')"
    >
      <RefreshCwIcon size="14" />
      <span>{{ regenerateLabel }}</span>
    </button>

    <button
      v-if="allowFeedback"
      type="button"
      class="assistant-action-btn"
      data-testid="assistant-action-feedback-up"
      :aria-pressed="feedback === 'up' ? 'true' : 'false'"
      :class="{ 'is-active': feedback === 'up' }"
      @click="toggleFeedback('up')"
    >
      <ThumbsUpIcon size="14" />
      <span>赞同</span>
    </button>

    <button
      v-if="allowFeedback"
      type="button"
      class="assistant-action-btn"
      data-testid="assistant-action-feedback-down"
      :aria-pressed="feedback === 'down' ? 'true' : 'false'"
      :class="{ 'is-active': feedback === 'down' }"
      @click="toggleFeedback('down')"
    >
      <ThumbsDownIcon size="14" />
      <span>不满意</span>
    </button>

    <button
      v-if="canOpenPanel"
      type="button"
      class="assistant-action-btn"
      data-testid="assistant-action-open-panel"
      @click="$emit('open-panel')"
    >
      <PanelRightOpenIcon size="14" />
      <span>打开面板</span>
    </button>
  </div>
</template>

<style scoped>
.assistant-action-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.assistant-action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: none;
  background: transparent;
  color: #7c8798;
  font-size: 12.5px;
  font-weight: 500;
  line-height: 1.35;
  cursor: pointer;
}

.assistant-action-btn:hover {
  color: #334155;
}

.assistant-action-btn.is-active {
  color: #111827;
}

.assistant-action-copy-btn .assistant-action-label {
  display: inline-block;
  max-width: 0;
  margin-left: -6px;
  opacity: 0;
  overflow: hidden;
  white-space: nowrap;
  transform: translateX(-2px);
  transition:
    max-width 0.16s ease,
    margin-left 0.16s ease,
    opacity 0.14s ease,
    transform 0.16s ease;
}

.assistant-action-copy-btn:hover .assistant-action-label,
.assistant-action-copy-btn:focus-visible .assistant-action-label {
  max-width: 4em;
  margin-left: 0;
  opacity: 1;
  transform: translateX(0);
}

.assistant-action-btn :deep(svg) {
  flex-shrink: 0;
}

</style>
