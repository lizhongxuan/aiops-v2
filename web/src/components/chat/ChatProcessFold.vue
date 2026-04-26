<script setup>
import { computed, ref, watch } from "vue";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-vue-next";
import MessageCard from "../MessageCard.vue";
import ChatTerminalPreview from "./ChatTerminalPreview.vue";
import ToolDisplayRenderer from "./tool-display/ToolDisplayRenderer.vue";

const props = defineProps({
  turn: {
    type: Object,
    required: true,
  },
  expanded: {
    type: Boolean,
    default: undefined,
  },
});

const emit = defineEmits(["update:expanded"]);

function isFailedStatus(status) {
  return ["failed", "error", "rejected"].includes(String(status || "").trim().toLowerCase());
}

function requiresAttention(turn = {}) {
  const phase = String(turn?.phase || "").trim().toLowerCase();
  if (phase === "waiting_approval" || phase === "waiting_input") return true;
  return (turn?.processItems || []).some((item) => isFailedStatus(item?.status));
}

function resolveDefaultExpanded(turn = {}) {
  if (turn?.active) return true;
  if (requiresAttention(turn)) return true;
  return !turn?.collapsedByDefault;
}

const expandedState = ref(resolveDefaultExpanded(props.turn));
const controlledExpanded = computed(() => typeof props.expanded === "boolean");
const expanded = computed(() => (controlledExpanded.value ? props.expanded : expandedState.value));
const expandedCommandIds = ref(new Set());

watch(
  () => [props.turn?.id, props.turn?.collapsedByDefault, props.turn?.phase, props.turn?.active, props.expanded],
  () => {
    expandedState.value = controlledExpanded.value ? Boolean(props.expanded) : resolveDefaultExpanded(props.turn);
    expandedCommandIds.value = new Set();
  },
);

const hasContent = computed(() => {
  return (
    (props.turn?.processItems?.length > 0) ||
    props.turn?.liveHint ||
    props.turn?.summary ||
    intermediateMessages.value.length > 0 ||
    historyItems.value.length > 0
  );
});

// Intermediate assistant messages (model's thinking text, not the final answer)
const intermediateMessages = computed(() => {
  const items = props.turn?.processItems || [];
  return items.filter(item => item.kind === "assistant" || item.kind === "assistant_message" || item.kind === "message");
});

const historyItems = computed(() => {
  const items = props.turn?.processItems || [];
  const msgSet = new Set(intermediateMessages.value.map(i => i.id));
  return items.filter(item => !msgSet.has(item.id));
});

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function classifyStageItem(item = {}) {
  const kind = compactText(item.kind).toLowerCase();
  const processKind = compactText(item.processKind).toLowerCase();
  const text = compactText(`${item.text || ""} ${item.detail || ""}`).toLowerCase();
  const haystack = `${kind} ${processKind} ${text}`;
  if (/web[_\s-]?search|search_web|搜索网页|网页搜索/.test(haystack)) return "search";
  if (/open_page|浏览网页|打开网页|https?:\/\//.test(haystack)) return "page";
  if (/read_file|file_read|读取文件|read\s+[\w./-]+/.test(haystack)) return "file";
  if (/list_dir|list_files|浏览目录|列出|listed files|\bls\b/.test(haystack)) return "list";
  if (kind === "command" || /exec_command|terminal|shell|运行命令|已运行|ran\s+\d*\s*commands?/.test(haystack)) return "command";
  if (/apply_patch|write_file|diff|patch|修改文件|已编辑/.test(haystack)) return "edit";
  return "detail";
}

function stageStatusVerb(items = []) {
  const statuses = items.map((item) => compactText(item.status).toLowerCase()).filter(Boolean);
  if (statuses.some((status) => status.includes("fail") || status.includes("error") || status.includes("denied"))) {
    return "处理失败";
  }
  if (statuses.some((status) => status.includes("run") || status.includes("progress") || status.includes("queued"))) {
    return "正在处理";
  }
  return "已探索";
}

function pluralizeEnglishUnit(count, singular, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function summarizeStageGroup(items = []) {
  const counters = { search: 0, page: 0, file: 0, list: 0, command: 0, edit: 0, detail: 0 };
  for (const item of items) {
    const bucket = classifyStageItem(item);
    counters[bucket] = (counters[bucket] || 0) + 1;
  }
  const phrases = [];
  if (counters.search) phrases.push(`${counters.search} 次搜索`);
  if (counters.page) phrases.push(`${counters.page} 个网页`);
  if (counters.file) phrases.push(`${counters.file} 个文件`);
  if (counters.list) phrases.push(`${counters.list} 个目录`);
  if (counters.command) phrases.push(`ran ${pluralizeEnglishUnit(counters.command, "command")}`);
  if (counters.edit) phrases.push(`edited ${pluralizeEnglishUnit(counters.edit, "file")}`);
  const counted = counters.search + counters.page + counters.file + counters.list + counters.command + counters.edit;
  const remaining = Math.max(0, items.length - counted);
  if (!phrases.length && remaining) phrases.push(`${remaining} 条明细`);
  return `${stageStatusVerb(items)} ${phrases.join("，") || `${items.length} 条明细`}`;
}

function buildStageGroups(items = [], turnId = "") {
  const visibleItems = (items || []).filter((item) => item?.text || item?.display);
  if (!visibleItems.length) return [];
  return [{
    id: `${turnId || "turn"}-stage-0`,
    summary: summarizeStageGroup(visibleItems),
    items: visibleItems,
  }];
}

const stageGroups = computed(() => buildStageGroups(historyItems.value, props.turn?.id));
const expandedStageIds = ref(new Set());

watch(
  () => [props.turn?.id, stageGroups.value.map((group) => group.id).join("|")],
  () => {
    expandedStageIds.value = new Set(stageGroups.value.map((group) => group.id));
  },
  { immediate: true },
);

const foldLabel = computed(() => {
  return props.turn?.processLabel || "已处理";
});

const collapsedSummary = computed(() => {
  if (props.turn?.active) return "";
  return String(props.turn?.summary || "").trim();
});

const headerLabel = computed(() => {
  return foldLabel.value;
});

const showDivider = computed(() => expanded.value);

function toggleExpanded() {
  if (!hasContent.value) return;
  const nextValue = !expanded.value;
  if (!controlledExpanded.value) {
    expandedState.value = nextValue;
  }
  emit("update:expanded", nextValue);
}

function isStageExpanded(group) {
  return expandedStageIds.value.has(group.id);
}

function toggleStageGroup(group) {
  const next = new Set(expandedStageIds.value);
  if (next.has(group.id)) next.delete(group.id);
  else next.add(group.id);
  expandedStageIds.value = next;
}

function isCommandItem(item) {
  return item?.kind === "command" && (item?.command || item?.commandCard?.command);
}

function commandOutput(item) {
  return String(
    item?.output ||
    item?.commandCard?.output ||
    item?.commandCard?.stdout ||
    item?.commandCard?.stderr ||
    item?.detail ||
    "",
  ).trim();
}

function itemDisplay(item) {
  return item?.display || null;
}

function isCommandExpanded(item) {
  return expandedCommandIds.value.has(item.id);
}

function toggleCommandItem(item) {
  if (!isCommandItem(item)) return;
  const next = new Set(expandedCommandIds.value);
  if (next.has(item.id)) next.delete(item.id);
  else next.add(item.id);
  expandedCommandIds.value = next;
}
</script>

<template>
  <section
    v-if="hasContent"
    class="chat-process-fold"
    :data-testid="`chat-process-fold-${turn.id}`"
  >
    <div class="chat-process-header">
      <button
        type="button"
        class="chat-process-toggle"
        :aria-expanded="expanded"
        @click="toggleExpanded"
      >
        <span class="chat-process-label">{{ headerLabel }}</span>
        <component :is="expanded ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-icon" />
      </button>
      <span v-if="showDivider" class="chat-process-divider-line" />
    </div>

    <div v-if="expanded" class="chat-process-surface">
      <div class="chat-process-body">
        <div
          v-if="collapsedSummary"
          class="chat-process-summary"
          data-testid="chat-process-summary"
        >
          {{ collapsedSummary }}
        </div>

        <div v-for="msg in intermediateMessages" :key="msg.id" class="chat-process-message">
          <MessageCard v-if="msg.card" :card="msg.card" />
          <div v-else class="chat-process-text">{{ msg.text }}</div>
          <ToolDisplayRenderer v-if="itemDisplay(msg)" class="chat-process-item-display" :display="itemDisplay(msg)" />
        </div>

        <div v-for="group in stageGroups" :key="group.id" class="chat-process-stage">
          <button
            type="button"
            class="chat-process-stage-toggle"
            :aria-expanded="isStageExpanded(group)"
            :data-testid="`chat-process-stage-toggle-${group.id}`"
            @click="toggleStageGroup(group)"
          >
            <component :is="isStageExpanded(group) ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-stage-icon" />
            <span>{{ group.summary }}</span>
          </button>

          <div
            v-if="isStageExpanded(group)"
            class="chat-process-stage-details"
            :data-testid="`chat-process-stage-details-${group.id}`"
          >
            <template v-for="item in group.items" :key="item.id">
              <template v-if="isCommandItem(item)">
                <button
                  type="button"
                  class="chat-process-command-row"
                  :data-testid="`chat-process-command-row-${item.id}`"
                  @click="toggleCommandItem(item)"
                >
                  <component :is="isCommandExpanded(item) ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-command-icon" />
                  <span class="chat-process-command-text">{{ item.text }}</span>
                </button>

                <ToolDisplayRenderer v-if="itemDisplay(item)" class="chat-process-item-display" :display="itemDisplay(item)" />

                <ChatTerminalPreview
                  v-if="isCommandExpanded(item)"
                  :test-id="`chat-process-terminal-${item.id}`"
                  :command="item.command || item.commandCard?.command || item.commandCard?.title || ''"
                  :output="commandOutput(item)"
                />
              </template>

              <div v-else class="chat-process-item">
                <div v-if="item.text" class="chat-process-item-line">
                  <span>{{ item.text }}</span>
                </div>
                <ToolDisplayRenderer v-if="itemDisplay(item)" class="chat-process-item-display" :display="itemDisplay(item)" />
              </div>
            </template>
          </div>
        </div>

        <div v-if="turn.active && turn.liveHint" class="chat-process-live">
          <span class="chat-process-live-dot" aria-hidden="true" />
          <span>{{ turn.liveHint }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.chat-process-fold {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: min(920px, 100%);
  margin: 2px auto;
}

.chat-process-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.chat-process-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: none;
  background: transparent;
  color: #7c8798;
  font-size: 12.5px;
  font-weight: 500;
  line-height: 1.3;
  cursor: pointer;
  white-space: nowrap;
}

.chat-process-toggle:hover {
  color: #475569;
}

.chat-process-label {
  color: inherit;
}

.chat-process-icon {
  color: #9ca3af;
  flex-shrink: 0;
}

.chat-process-divider-line {
  flex: 1;
  height: 1px;
  background: #e5e7eb;
}

.chat-process-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chat-process-surface {
  padding-top: 2px;
}

.chat-process-summary {
  color: #8b95a7;
  font-size: 13px;
  line-height: 1.55;
}

.chat-process-message {
  /* Intermediate messages render inline */
}

.chat-process-surface :deep(.message-wrapper) {
  padding-left: 0;
}

.chat-process-surface :deep(.assistant-thread-block) {
  background: transparent;
  border: none;
  box-shadow: none;
}

.chat-process-surface :deep(.message-text),
.chat-process-surface :deep(.markdown-body) {
  color: #475569;
}

.chat-process-item-line {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: #8b95a7;
  line-height: 1.45;
}

.chat-process-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chat-process-stage {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chat-process-stage-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: fit-content;
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: #111827;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.45;
  text-align: left;
  cursor: pointer;
}

.chat-process-stage-toggle:hover {
  color: #334155;
}

.chat-process-stage-icon {
  flex-shrink: 0;
  color: #9ca3af;
}

.chat-process-stage-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 20px;
}

.chat-process-text {
  font-size: 13px;
  color: #475569;
  line-height: 1.5;
}

.chat-process-item-display {
  margin-top: 4px;
}

.chat-process-command-row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: fit-content;
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: #8b95a7;
  font-size: 12.5px;
  line-height: 1.45;
  text-align: left;
  cursor: pointer;
}

.chat-process-command-row:hover {
  color: #475569;
}

.chat-process-command-icon {
  flex-shrink: 0;
  color: #94a3b8;
}

.chat-process-command-text {
  min-width: 0;
}

.chat-process-live {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: #64748b;
  font-size: 12.5px;
  line-height: 1.42;
}

.chat-process-live-dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: #94a3b8;
  animation: processPulse 1.2s ease-in-out infinite;
}

@keyframes processPulse {
  0%,
  100% {
    opacity: 0.45;
  }

  50% {
    opacity: 1;
  }
}
</style>
