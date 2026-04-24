<script setup>
import { computed } from "vue";
import ChatTerminalPreview from "./ChatTerminalPreview.vue";

const props = defineProps({
  state: {
    type: String,
    default: "working",
  },
  elapsedLabel: {
    type: String,
    default: "",
  },
  phaseLabel: {
    type: String,
    default: "Working for",
  },
  activityLines: {
    type: Array,
    default: () => [],
  },
  commandCard: {
    type: Object,
    default: null,
  },
  message: {
    type: String,
    default: "",
  },
});

const headerLabel = computed(() => {
  const elapsed = String(props.elapsedLabel || "0s").trim() || "0s";
  const state = String(props.state || "working").trim().toLowerCase();
  if (state === "failed") return `Failed after ${elapsed}`;
  if (state === "aborted") return `Stopped after ${elapsed}`;
  const phase = String(props.phaseLabel || "Working for").trim() || "Working for";
  return `${phase} ${elapsed}`;
});

const normalizedActivityLines = computed(() => {
  const seen = new Set();
  const lines = [];
  const message = String(props.message || "").trim();
  if (message) {
    lines.push({
      id: "status-message",
      text: message,
      tone: String(props.state || "working").trim().toLowerCase() === "failed" ? "danger" : "summary",
    });
  }
  return [...lines, ...(props.activityLines || [])]
    .map((line, index) => {
      const text = String(line?.text || line || "").trim();
      if (!text || seen.has(text)) return null;
      seen.add(text);
      return {
        id: String(line?.id || `activity-${index}`),
        text,
        tone: String(line?.tone || ""),
      };
    })
    .filter(Boolean);
});

const commandText = computed(() => {
  const card = props.commandCard || {};
  return String(card.command || card.title || "").trim();
});

const commandOutput = computed(() => {
  const card = props.commandCard || {};
  return String(card.output || card.stdout || card.stderr || card.text || card.summary || "")
    .replace(/\r\n/g, "\n")
    .trimEnd();
});

const hasCommandPreview = computed(() => !!props.commandCard && (!!commandText.value || !!commandOutput.value));
</script>

<template>
  <div class="codex-activity-section" data-testid="codex-activity-section">
    <div class="codex-activity-header" :class="{ 'is-failed': state === 'failed', 'is-aborted': state === 'aborted' }">
      <span class="codex-working-label">{{ headerLabel }}</span>
      <hr class="codex-activity-divider" />
    </div>

    <div class="codex-activity-lines">
      <div
        v-for="line in normalizedActivityLines"
        :key="line.id"
        class="codex-activity-line codex-activity-detail"
        :class="{
          'is-current': line.tone === 'current',
          'is-summary': line.tone === 'summary',
          'is-danger': line.tone === 'danger',
        }"
      >
        {{ line.text }}
      </div>

      <ChatTerminalPreview
        v-if="hasCommandPreview"
        test-id="chat-live-terminal-preview"
        :command="commandText"
        :output="commandOutput"
      />
    </div>
  </div>
</template>

<style scoped>
.codex-activity-section {
  margin: 12px 0;
  padding: 0;
  animation: fadeInUp 0.2s ease-out;
}

.codex-activity-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.codex-working-label {
  color: #7c8798;
  font-size: 12.5px;
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
}

.codex-activity-header.is-failed .codex-working-label {
  color: #b4535f;
}

.codex-activity-header.is-aborted .codex-working-label {
  color: #8b95a7;
}

.codex-activity-divider {
  flex: 1;
  margin: 0;
  border: none;
  border-top: 1px solid #e5e7eb;
}

.codex-activity-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.codex-activity-line {
  padding: 0;
  color: #94a3b8;
  font-size: 12.5px;
  line-height: 1.5;
}

.codex-activity-detail {
  color: #9aa4b2;
}

.codex-activity-detail.is-current {
  color: #64748b;
}

.codex-activity-detail.is-summary {
  color: #8692a3;
}

.codex-activity-detail.is-danger {
  color: #b4535f;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(4px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
