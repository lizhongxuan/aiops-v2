<script setup>
import { computed } from "vue";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
});

const statusTone = computed(() => {
  if (props.card.status === "failed") return "failed";
  if (props.card.status === "completed") return "completed";
  return "running";
});

const summary = computed(() => compactText(props.card.text || props.card.title || ""));
const summaryParts = computed(() => splitSummary(summary.value));
const badgeLabel = computed(() => {
  if (props.card.status === "failed") return "失败";
  if (props.card.status === "completed") return "完成";
  return "进行中";
});

const contextLabel = computed(() => (props.sessionKind === "workspace" ? "工作台投影" : ""));

function compactText(value) {
  return (value || "").trim().replace(/\s+/g, " ");
}

function splitSummary(value) {
  if (!value) return { lead: "", detail: "" };
  const cleaned = value
    .replace(/^现在\s*/, "")
    .replace(/^已\s*/, "")
    .replace(/^正在\s*/, "");

  const match = cleaned.match(/^([^\s（(]+)(?:[：:]\s*|\s+)(.+)$/);
  if (match) {
    return {
      lead: match[1],
      detail: match[2],
    };
  }

  return {
    lead: cleaned,
    detail: "",
  };
}

const detailTone = computed(() => {
  if (props.card.status === "failed") return "danger";
  if (props.card.status === "completed") return "success";
  return "neutral";
});
</script>

<template>
  <div class="process-line-card" :class="[statusTone, detailTone]">
    <span class="process-badge">{{ badgeLabel }}</span>
    <span v-if="contextLabel" class="process-context">{{ contextLabel }}</span>
    <span class="process-lead">{{ summaryParts.lead }}</span>
    <span v-if="summaryParts.detail" class="process-detail">{{ summaryParts.detail }}</span>
  </div>
</template>

<style scoped>
.process-line-card {
  display: inline-flex;
  align-items: flex-start;
  gap: 8px;
  margin-left: 36px;
  padding: 2px 0;
  color: #475569;
  font-size: 11px;
  line-height: 1.45;
  flex-wrap: wrap;
  max-width: min(720px, calc(100vw - 80px));
}

.process-line-card.running {
  color: #64748b;
}

.process-line-card.completed {
  color: #334155;
}

.process-line-card.failed {
  color: #9a3412;
}

.process-badge {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 999px;
  background: #eef2ff;
  color: #3730a3;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.process-context {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.process-line-card.completed .process-badge {
  background: #ecfdf5;
  color: #047857;
}

.process-line-card.failed .process-badge {
  background: #fef2f2;
  color: #b91c1c;
}

.process-lead {
  font-weight: 600;
  color: #0f172a;
}

.process-detail {
  color: #64748b;
  white-space: pre-wrap;
  word-break: break-word;
}

.process-line-card.failed .process-detail {
  color: #9a3412;
}

@media (max-width: 640px) {
  .process-line-card {
    margin-left: 0;
    max-width: none;
  }
}
</style>
