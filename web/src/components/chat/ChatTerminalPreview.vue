<script setup>
import { computed, nextTick, onMounted, ref, watch } from "vue";

const props = defineProps({
  command: {
    type: String,
    default: "",
  },
  output: {
    type: String,
    default: "",
  },
  label: {
    type: String,
    default: "Shell",
  },
  maxLines: {
    type: Number,
    default: 7,
  },
  testId: {
    type: String,
    default: "",
  },
});

const outputRef = ref(null);

function stripMatchingQuotes(value) {
  const text = String(value || "").trim();
  if (text.length < 2) return text;
  if ((text.startsWith("'") && text.endsWith("'")) || (text.startsWith("\"") && text.endsWith("\""))) {
    return text.slice(1, -1);
  }
  return text;
}

function displayCommand(value = "") {
  const raw = String(value || "").trim();
  if (!raw) return "";
  const shellMatch = raw.match(/^(?:\/[\w./-]+\/)?(?:zsh|bash|sh)\s+-lc\s+([\s\S]+)$/);
  if (shellMatch) return stripMatchingQuotes(shellMatch[1]);
  return raw;
}

const commandText = computed(() => displayCommand(props.command || ""));
const outputText = computed(() => {
  const value = String(props.output || "").replace(/\r\n/g, "\n");
  return value.replace(/\s+$/u, "");
});
const outputLines = computed(() => {
  const value = outputText.value;
  if (!value) return [""];
  return value.split("\n");
});
const visibleLineCount = computed(() => {
  return Math.max(1, Math.min(props.maxLines, outputLines.value.length));
});
const outputViewportStyle = computed(() => {
  const lineHeight = 18;
  const padding = 12;
  return {
    maxHeight: `${visibleLineCount.value * lineHeight + padding}px`,
    height: `${visibleLineCount.value * lineHeight + padding}px`,
  };
});

async function scrollToBottom() {
  await nextTick();
  const node = outputRef.value;
  if (!node) return;
  node.scrollTop = node.scrollHeight;
}

watch(outputText, () => {
  void scrollToBottom();
});

onMounted(() => {
  void scrollToBottom();
});
</script>

<template>
  <section
    class="chat-terminal-preview"
    :data-testid="testId || undefined"
  >
    <div class="chat-terminal-preview-label">{{ label }}</div>
    <div v-if="commandText" class="chat-terminal-preview-command">$ {{ commandText }}</div>
    <pre
      ref="outputRef"
      class="chat-terminal-preview-output"
      :style="outputViewportStyle"
    >{{ outputText || " " }}</pre>
  </section>
</template>

<style scoped>
.chat-terminal-preview {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(720px, 100%);
  border: 1px solid rgba(203, 213, 225, 0.9);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(243, 244, 246, 0.96), rgba(229, 231, 235, 0.98));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.7);
  padding: 10px 12px;
}

.chat-terminal-preview-label {
  color: #6b7280;
  font-size: 11.5px;
  font-weight: 600;
  line-height: 1.2;
}

.chat-terminal-preview-command {
  overflow-x: auto;
  color: #111827;
  font-family: "SF Mono", "Monaco", "Menlo", monospace;
  font-size: 12.5px;
  line-height: 1.45;
  white-space: pre;
}

.chat-terminal-preview-output {
  margin: 0;
  overflow-x: auto;
  overflow-y: auto;
  color: #4b5563;
  font-family: "SF Mono", "Monaco", "Menlo", monospace;
  font-size: 12.5px;
  line-height: 18px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
