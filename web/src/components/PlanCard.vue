<script setup>
import { ref, computed, watch } from "vue";
import { NTimeline, NTimelineItem, NCollapse, NCollapseItem } from "naive-ui";
import { ListTodoIcon, ChevronDownIcon, ChevronRightIcon } from "lucide-vue-next";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
  compact: {
    type: Boolean,
    default: false,
  },
});

const isExpanded = ref(true);

const completedCount = computed(() => {
  return props.card.items?.filter((i) => i.status === "completed").length || 0;
});
const totalCount = computed(() => props.card.items?.length || 0);

const summaryText = computed(() => {
  return `共 ${totalCount.value} 个任务，已完成 ${completedCount.value} 个`;
});

const contextLabel = computed(() => (props.sessionKind === "workspace" ? "工作台计划投影" : "计划"));

const planTitle = computed(() => {
  if (props.card.title && props.card.title !== "PlanCard") return props.card.title;
  return null;
});

const planText = computed(() => {
  if (props.card.text) return props.card.text;
  return null;
});

function toggleExpand() {
  isExpanded.value = !isExpanded.value;
}

function timelineType(status) {
  if (status === "completed") return "success";
  if (status === "inProgress") return "info";
  if (status === "failed" || status === "error") return "error";
  if (status === "waiting" || status === "waiting_approval") return "warning";
  return "default";
}

function stepSummary(item, index) {
  return `${index + 1}. ${item.step || "未命名步骤"}`;
}

// Auto-expand executing steps, auto-collapse completed
const expandedStepNames = computed(() => {
  const items = props.card.items || [];
  return items
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => item.status === "inProgress")
    .map(({ index }) => `step-${index}`);
});
</script>

<template>
  <div class="plan-card" :class="{ compact }">
    <div class="plan-header" @click="toggleExpand">
      <div class="plan-left">
        <ListTodoIcon size="16" class="plan-icon" />
        <div class="plan-title-group">
          <span class="plan-context">{{ contextLabel }}</span>
          <span v-if="planTitle" class="plan-title">{{ planTitle }}</span>
          <span class="plan-summary">{{ summaryText }}</span>
        </div>
      </div>
      <component :is="isExpanded ? ChevronDownIcon : ChevronRightIcon" size="16" class="plan-toggle" />
    </div>

    <div class="plan-body" v-if="isExpanded && (card.items?.length || planText)">
      <div v-if="planText && !card.items?.length" class="plan-goal">{{ planText }}</div>
      <n-timeline v-if="card.items?.length">
        <n-timeline-item
          v-for="(item, index) in card.items"
          :key="index"
          :type="timelineType(item.status)"
          :title="stepSummary(item, index)"
          :class="{ 'step-completed': item.status === 'completed', 'step-active': item.status === 'inProgress' }"
        >
          <template v-if="item.output || item.detail">
            <n-collapse :default-expanded-names="expandedStepNames">
              <n-collapse-item :title="item.status === 'inProgress' ? '执行中...' : '查看详情'" :name="`step-${index}`">
                <pre class="step-output">{{ item.output || item.detail || '' }}</pre>
              </n-collapse-item>
            </n-collapse>
          </template>
        </n-timeline-item>
      </n-timeline>
    </div>
  </div>
</template>

<style scoped>
.plan-card {
  border: 1px solid var(--border-card, #e5e7eb);
  border-radius: 12px;
  background: #ffffff;
  overflow: hidden;
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.02);
  margin-top: 2px;
  margin-left: 36px;
  max-width: 600px;
}

.plan-card.compact {
  margin: 0;
  max-width: none;
  border-bottom-left-radius: 0;
  border-bottom-right-radius: 0;
  border-bottom: none;
  box-shadow: 0 -4px 12px rgba(15, 23, 42, 0.05);
}

.plan-header {
  padding: 8px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s;
}

.plan-card.compact .plan-header {
  padding: 10px 14px;
}

.plan-header:hover {
  background: #fafafa;
}

.plan-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.plan-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.plan-icon {
  color: #6b7280;
}

.plan-context {
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.plan-summary {
  font-size: 13px;
  font-weight: 600;
  color: #374151;
}

.plan-title {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
}

.plan-goal {
  padding: 8px 12px;
  font-size: 13px;
  color: #475569;
  line-height: 1.5;
  border-left: 3px solid #3b82f6;
  margin-bottom: 8px;
  background: #f8fafc;
  border-radius: 0 6px 6px 0;
}

.plan-toggle {
  color: #9ca3af;
}

.plan-body {
  padding: 6px 14px 14px;
}

.plan-card.compact .plan-body {
  padding: 6px 14px 14px;
}

.step-completed :deep(.n-timeline-item-content__title) {
  text-decoration: line-through;
  color: #9ca3af;
}

.step-active :deep(.n-timeline-item-content__title) {
  color: #0f172a;
  font-weight: 600;
}

.step-output {
  margin: 0;
  padding: 8px 10px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  color: #1f2937;
  font-size: 11px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  max-height: 200px;
  overflow: auto;
}
</style>
