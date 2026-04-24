<script setup>
import { computed, ref, watch } from "vue";
import McpUiCardHost from "./McpUiCardHost.vue";
import { normalizeMcpBundle } from "../../lib/mcpUiCardModel";

const props = defineProps({
  bundle: {
    type: Object,
    required: true,
  },
  compact: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["action", "open-detail", "pin"]);

const SECTION_ORDER = ["root_cause", "recommended_actions", "control_panels", "validation_panels"];

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

const normalizedBundle = computed(() => normalizeMcpBundle(props.bundle || {}, { bundleKind: "remediation_bundle" }));
const expanded = ref(false);

watch(
  () => normalizedBundle.value.bundleId,
  () => {
    expanded.value = false;
  },
  { immediate: true },
);

const sectionMap = computed(() => {
  return new Map(normalizedBundle.value.sections.map((section) => [section.kind, section]));
});

const allSections = computed(() => {
  return SECTION_ORDER.map((kind, index) => {
    const existing = sectionMap.value.get(kind);
    if (existing && kind === "recommended_actions" && normalizedBundle.value.recommendedActions.length) {
      return {
        ...existing,
        cards: [...existing.cards, ...normalizedBundle.value.recommendedActions],
      };
    }
    if (existing && kind === "validation_panels" && normalizedBundle.value.validationPanels.length) {
      return {
        ...existing,
        cards: [...existing.cards, ...normalizedBundle.value.validationPanels],
      };
    }
    return existing || {
      id: `${kind}-${index + 1}`,
      kind,
      title: compactText(kind).replace(/_/g, " "),
      summary: "",
      cards: [],
    };
  });
});

const visibleSections = computed(() => {
  if (!props.compact || expanded.value) return allSections.value;
  return allSections.value.filter((section) => section.cards.length || section.summary).slice(0, 2);
});

const hiddenSectionCount = computed(() => {
  const populatedCount = allSections.value.filter((section) => section.cards.length || section.summary).length;
  return Math.max(0, populatedCount - visibleSections.value.length);
});
const subjectLabel = computed(() => {
  const subject = normalizedBundle.value.subject || {};
  return [subject.type || "service", subject.name || subject.service || "unknown", subject.env || ""].filter(Boolean).join(" / ");
});
const freshnessLabel = computed(() => normalizedBundle.value.freshness?.label || normalizedBundle.value.freshness?.capturedAt || "");

const derivedLastActivity = computed(() => {
  if (normalizedBundle.value.lastActivity?.label) return normalizedBundle.value.lastActivity;
  if (normalizedBundle.value.recentActivities.length > 5) {
    return normalizedBundle.value.recentActivities[normalizedBundle.value.recentActivities.length - 1];
  }
  return null;
});

const visibleActivities = computed(() => {
  const source = normalizedBundle.value.recentActivities.slice();
  const trimmed = derivedLastActivity.value && source.length ? source.slice(0, -1) : source;
  return trimmed.slice(-5);
});

function emitOpenDetail(payload = normalizedBundle.value) {
  emit("open-detail", payload);
}

function emitAction(payload = normalizedBundle.value) {
  emit("action", payload);
}

function emitPin(payload = normalizedBundle.value) {
  emit("pin", payload);
}
</script>

<template>
  <section
    class="mcp-remediation-bundle-card"
    data-bundle-kind="remediation_bundle"
  >
    <header class="bundle-header">
      <div class="bundle-copy">
        <p class="bundle-eyebrow">{{ normalizedBundle.source || "mcp" }} / {{ normalizedBundle.mcpServer || "default" }}</p>
        <h3 class="bundle-title" data-testid="mcp-bundle-subject">{{ subjectLabel }}</h3>
        <p class="bundle-summary" data-testid="mcp-bundle-summary">{{ normalizedBundle.summary || "暂无 remediation 摘要" }}</p>
        <p class="bundle-root-cause" data-testid="mcp-bundle-root-cause">根因：{{ normalizedBundle.rootCause || "待补充" }}</p>
        <p class="bundle-confidence" data-testid="mcp-bundle-confidence">置信度：{{ normalizedBundle.confidence }}</p>
      </div>
      <div class="bundle-meta">
        <span
          v-if="freshnessLabel"
          class="meta-pill"
          data-testid="mcp-bundle-freshness"
        >
          {{ freshnessLabel }}
        </span>
        <button
          type="button"
          class="bundle-btn"
          data-testid="mcp-bundle-action"
          @click="emitAction"
        >
          执行推荐操作
        </button>
        <button
          type="button"
          class="bundle-btn"
          data-testid="mcp-bundle-open-detail"
          @click="emitOpenDetail()"
        >
          查看完整面板
        </button>
        <button
          type="button"
          class="bundle-btn"
          data-testid="mcp-bundle-pin"
          @click="emitPin()"
        >
          固定
        </button>
      </div>
    </header>

    <section
      v-if="visibleActivities.length"
      class="activity-strip"
      data-testid="mcp-bundle-recent-activity-strip"
    >
      <div class="activity-strip-header">
        <strong>最近活动</strong>
        <span v-if="derivedLastActivity?.label" class="activity-tail">最终进度：{{ derivedLastActivity.label }}</span>
      </div>
      <div class="activity-list">
        <article
          v-for="(activity, index) in visibleActivities"
          :key="activity.id"
          class="activity-item"
          :data-emphasis="index === visibleActivities.length - 1 ? 'last' : 'normal'"
          data-testid="mcp-bundle-recent-activity-item"
        >
          <strong>{{ activity.label }}</strong>
          <span v-if="activity.detail">{{ activity.detail }}</span>
        </article>
      </div>
    </section>

    <section
      v-for="section in visibleSections"
      :key="section.id"
      class="bundle-section"
      :data-testid="`mcp-bundle-section-${section.kind}`"
    >
      <header class="section-header">
        <h4 class="section-title">{{ section.title || section.kind }}</h4>
        <p v-if="section.summary" class="section-summary">{{ section.summary }}</p>
      </header>
      <div class="section-cards">
        <McpUiCardHost
          v-for="card in section.cards"
          :key="card.id"
          :card="card"
          @action="emitAction"
          @detail="emitOpenDetail"
          @refresh="emitAction"
          @pin="emitPin"
        />
      </div>
    </section>

    <button
      v-if="props.compact && hiddenSectionCount > 0 && !expanded"
      type="button"
      class="bundle-expand"
      data-testid="mcp-bundle-expand-more"
      @click="expanded = true"
    >
      展开剩余 {{ hiddenSectionCount }} 个分区
    </button>
  </section>
</template>

<style scoped>
.mcp-remediation-bundle-card {
  display: grid;
  gap: 16px;
  padding: 16px;
  border-radius: 20px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  background: linear-gradient(180deg, rgba(255, 250, 245, 0.99), rgba(248, 250, 252, 0.98));
}

.bundle-header,
.bundle-meta,
.activity-strip-header {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 10px;
}

.bundle-copy {
  display: grid;
  gap: 5px;
}

.bundle-eyebrow,
.section-summary,
.activity-tail {
  margin: 0;
  font-size: 11px;
  color: #64748b;
}

.bundle-title,
.section-title {
  margin: 0;
  color: #0f172a;
}

.bundle-title {
  font-size: 16px;
}

.bundle-summary,
.bundle-root-cause,
.bundle-confidence {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #334155;
}

.activity-strip,
.bundle-section,
.section-cards {
  display: grid;
  gap: 12px;
}

.activity-list {
  display: grid;
  gap: 8px;
}

.activity-item {
  display: grid;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(15, 23, 42, 0.06);
  font-size: 13px;
  color: #334155;
}

.activity-item[data-emphasis="last"] {
  border-color: rgba(249, 115, 22, 0.28);
  background: rgba(255, 247, 237, 0.96);
}

.meta-pill,
.bundle-btn,
.bundle-expand {
  border: none;
  border-radius: 999px;
  padding: 7px 12px;
  background: rgba(226, 232, 240, 0.86);
  color: #0f172a;
  font-size: 12px;
  cursor: pointer;
}

.bundle-expand {
  justify-self: flex-start;
}
</style>
