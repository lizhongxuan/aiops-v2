<script setup>
import { computed, ref, watch } from "vue";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-vue-next";
import ChatTerminalPreview from "./ChatTerminalPreview.vue";

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

const transcript = computed(() => props.turn?.processTranscript || null);
const headerBlock = computed(() => transcript.value?.header || transcript.value?.blocks?.find((block) => block.kind === "header") || null);
const contentBlocks = computed(() => {
  return (transcript.value?.blocks || []).filter((block) => {
    if (!block || block.visibility === "hidden") return false;
    return !["header", "final-answer"].includes(block.kind);
  });
});

function resolveDefaultExpanded(turn = {}) {
  if (turn?.active) return true;
  return !turn?.processTranscript?.collapsedByDefault && !turn?.collapsedByDefault;
}

const expandedState = ref(resolveDefaultExpanded(props.turn));
const controlledExpanded = computed(() => typeof props.expanded === "boolean");
const expanded = computed(() => (controlledExpanded.value ? props.expanded : expandedState.value));
const expandedCommandIds = ref(new Set());
const expandedSearchIds = ref(new Set());

watch(
  () => props.turn?.id,
  () => {
    expandedState.value = controlledExpanded.value ? Boolean(props.expanded) : resolveDefaultExpanded(props.turn);
    expandedCommandIds.value = new Set();
    expandedSearchIds.value = new Set();
  },
  { immediate: true },
);

watch(
  () => props.expanded,
  (value) => {
    if (controlledExpanded.value) expandedState.value = Boolean(value);
  },
);

watch(expanded, (value) => {
  if (value) return;
  expandedCommandIds.value = new Set();
  expandedSearchIds.value = new Set();
});

const hasVisibleDetails = computed(() => contentBlocks.value.length > 0);
const hasLiveThinking = computed(() => Boolean(transcript.value?.showThinking));
const hasStatusOnlyContent = computed(() => {
  const status = String(headerBlock.value?.status || props.turn?.phase || "").trim().toLowerCase();
  return Boolean(props.turn?.active || ["failed", "aborted", "blocked"].includes(status));
});
const hasContent = computed(() => hasVisibleDetails.value || hasLiveThinking.value || Boolean(headerBlock.value && hasStatusOnlyContent.value));
const headerLabel = computed(() => headerBlock.value?.text || props.turn?.processLabel || "已处理");
const showDivider = computed(() => hasVisibleDetails.value || hasLiveThinking.value);

function toggleExpanded() {
  if (!hasVisibleDetails.value) return;
  const nextValue = !expanded.value;
  if (!controlledExpanded.value) expandedState.value = nextValue;
  emit("update:expanded", nextValue);
}

function blockText(block = {}) {
  return String(block.text || block.inputSummary || "").trim();
}

function normalizeExpansionKey(value = "") {
  return String(value || "").replace(/\s+/g, " ").trim().toLowerCase();
}

function commandText(block = {}) {
  return String(block.command || block.inputSummary || "").trim();
}

function commandOutput(block = {}) {
  return String(block.outputPreview || block.output || "").trim();
}

function commandExpansionKey(block = {}) {
  return `command:${normalizeExpansionKey(commandText(block) || block.id)}`;
}

function isCommandExpanded(block = {}) {
  return expandedCommandIds.value.has(commandExpansionKey(block));
}

function toggleCommand(block = {}) {
  if (!commandText(block)) return;
  const key = commandExpansionKey(block);
  const next = new Set(expandedCommandIds.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expandedCommandIds.value = next;
}

function searchExpansionKey(block = {}) {
  const queryText = Array.isArray(block.queries) && block.queries.length
    ? block.queries.join(" ")
    : block.inputSummary || block.query || block.text || block.id;
  return `search:${normalizeExpansionKey(queryText)}`;
}

function isSearchExpanded(block = {}) {
  return expandedSearchIds.value.has(searchExpansionKey(block));
}

function toggleSearch(block = {}) {
  const key = searchExpansionKey(block);
  const next = new Set(expandedSearchIds.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expandedSearchIds.value = next;
}

function searchQueries(block = {}) {
  if (Array.isArray(block.queries) && block.queries.length) return block.queries;
  return block.inputSummary ? [block.inputSummary] : [];
}

function searchResults(block = {}) {
  return Array.isArray(block.results) ? block.results : [];
}

function planSteps(block = {}) {
  return Array.isArray(block.steps) ? block.steps : [];
}

function evidenceMeta(block = {}) {
  return [
    block.source ? `来源 ${block.source}` : "",
    block.confidence ? `置信度 ${block.confidence}` : "",
    block.window ? `窗口 ${block.window}` : "",
    block.rawRef ? `引用 ${block.rawRef}` : "",
  ].filter(Boolean);
}

function approvalCommand(block = {}) {
  return String(block.command || "").trim();
}

function approvalMeta(block = {}) {
  return [
    block.risk ? `风险 ${block.risk}` : "",
    Array.isArray(block.targets) && block.targets.length ? `目标 ${block.targets.join(", ")}` : "",
  ].filter(Boolean);
}

function typedProcessMeta(block = {}) {
  return [
    block.source ? `来源 ${block.source}` : "",
    block.risk ? `风险 ${block.risk}` : "",
    block.runbookId ? `Runbook ${block.runbookId}` : "",
    block.runbookStep ? `步骤 ${block.runbookStep}` : "",
    block.confidence ? `置信度 ${block.confidence}` : "",
    block.window ? `窗口 ${block.window}` : "",
    block.rawRef ? `引用 ${block.rawRef}` : "",
  ].filter(Boolean);
}

function isTypedProcessBlock(block = {}) {
  return ["runbook-step", "proposal-step", "verification-step", "incident-step"].includes(block.kind);
}

function blockClass(block = {}) {
  return {
    "chat-process-narration": block.kind === "assistant-intent" || block.kind === "assistant-result",
    "chat-process-step": !["assistant-intent", "assistant-result"].includes(block.kind),
    "is-muted": block.kind !== "assistant-intent" && block.kind !== "assistant-result",
    "is-thinking": block.kind === "reasoning-summary" && block.status === "running",
  };
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
        data-testid="chat-process-header"
        :aria-expanded="hasVisibleDetails ? expanded : false"
        @click="toggleExpanded"
      >
        <span class="chat-process-label">{{ headerLabel }}</span>
        <component v-if="hasVisibleDetails" :is="expanded ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-icon" />
      </button>
      <span v-if="showDivider" class="chat-process-divider-line" />
    </div>

    <div v-if="expanded && hasVisibleDetails" class="chat-process-transcript" data-testid="chat-process-transcript">
      <template v-for="block in contentBlocks" :key="block.id">
        <div
          v-if="block.kind === 'assistant-intent' || block.kind === 'assistant-result' || block.kind === 'reasoning-summary'"
          :class="blockClass(block)"
          :data-testid="block.kind === 'reasoning-summary' ? 'process-step-reasoning' : 'process-step-narration'"
        >
          {{ blockText(block) }}
        </div>

        <div v-else-if="block.kind === 'command-step'" class="chat-process-command">
          <button
            type="button"
            class="chat-process-command-row"
            data-testid="process-step-command"
            :aria-expanded="isCommandExpanded(block)"
            @click="toggleCommand(block)"
          >
            <component :is="isCommandExpanded(block) ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-step-icon" />
            <span class="chat-process-command-label">{{ blockText(block).replace(commandText(block), '').trim() || (block.status === 'running' ? '正在运行' : '已运行') }}</span>
            <code class="chat-process-command-code">{{ commandText(block) }}</code>
          </button>

          <ChatTerminalPreview
            v-if="isCommandExpanded(block)"
            test-id="process-terminal-preview"
            :command="commandText(block)"
            :output="commandOutput(block)"
          />
        </div>

        <div v-else-if="block.kind === 'search-step'" class="chat-process-search">
          <button
            type="button"
            class="chat-process-search-row"
            data-testid="process-step-search"
            :aria-expanded="isSearchExpanded(block)"
            @click="toggleSearch(block)"
          >
            <component :is="isSearchExpanded(block) ? ChevronDownIcon : ChevronRightIcon" size="14" class="chat-process-step-icon" />
            <span>{{ blockText(block) }}</span>
          </button>

          <div v-if="isSearchExpanded(block)" class="chat-process-detail-panel" data-testid="process-search-preview">
            <div v-for="query in searchQueries(block)" :key="query" class="chat-process-detail-line">
              <span class="chat-process-detail-label">Query</span>
              <span>{{ query }}</span>
            </div>
            <div v-for="result in searchResults(block)" :key="`${result.title || ''}-${result.url || ''}`" class="chat-process-result">
              <div class="chat-process-result-title">{{ result.title || result.url }}</div>
              <div v-if="result.url" class="chat-process-result-url">{{ result.url }}</div>
              <div v-if="result.snippet" class="chat-process-result-snippet">{{ result.snippet }}</div>
            </div>
          </div>
        </div>

        <div v-else-if="block.kind === 'plan-step'" class="chat-process-plan chat-process-step" data-testid="process-step-plan">
          <div class="chat-process-step-title">{{ blockText(block) }}</div>
          <ol v-if="planSteps(block).length" class="chat-process-plan-list">
            <li v-for="step in planSteps(block)" :key="step.id || step.text" :class="`is-${step.status || 'pending'}`">
              <span class="chat-process-plan-status">{{ step.status || 'pending' }}</span>
              <span>{{ step.text }}</span>
            </li>
          </ol>
        </div>

        <div v-else-if="block.kind === 'evidence-step'" class="chat-process-evidence chat-process-step" data-testid="process-step-evidence">
          <div class="chat-process-step-title">{{ blockText(block) }}</div>
          <div v-if="evidenceMeta(block).length" class="chat-process-meta">
            <span v-for="item in evidenceMeta(block)" :key="item">{{ item }}</span>
          </div>
        </div>

        <div v-else-if="isTypedProcessBlock(block)" class="chat-process-typed chat-process-step" :data-testid="`process-step-${block.kind.replace('-step', '')}`">
          <div class="chat-process-step-title">{{ blockText(block) }}</div>
          <code v-if="block.command" class="chat-process-typed-command">{{ block.command }}</code>
          <div v-if="block.summary" class="chat-process-typed-summary">{{ block.summary }}</div>
          <div v-if="block.expectedEffect" class="chat-process-typed-summary">预期效果：{{ block.expectedEffect }}</div>
          <div v-if="block.rollback" class="chat-process-typed-summary">回退：{{ block.rollback }}</div>
          <div v-if="typedProcessMeta(block).length" class="chat-process-meta">
            <span v-for="item in typedProcessMeta(block)" :key="item">{{ item }}</span>
          </div>
        </div>

        <div v-else-if="block.kind === 'inline-approval'" class="chat-process-approval chat-process-step" data-testid="process-step-approval">
          <div class="chat-process-step-title">{{ blockText(block) || '等待确认' }}</div>
          <code v-if="approvalCommand(block)" class="chat-process-approval-command">{{ approvalCommand(block) }}</code>
          <div v-if="block.reason" class="chat-process-approval-reason">{{ block.reason }}</div>
          <div v-if="approvalMeta(block).length" class="chat-process-meta">
            <span v-for="item in approvalMeta(block)" :key="item">{{ item }}</span>
          </div>
        </div>

        <div v-else :class="blockClass(block)" data-testid="process-step-generic">
          {{ blockText(block) }}
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.chat-process-fold {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(860px, 100%);
  margin: 2px auto 10px;
}

.chat-process-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.chat-process-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: none;
  background: transparent;
  color: #737373;
  font-size: 13.5px;
  font-weight: 500;
  line-height: 1.4;
  cursor: pointer;
  white-space: nowrap;
}

.chat-process-toggle:hover {
  color: #404040;
}

.chat-process-icon,
.chat-process-step-icon {
  flex-shrink: 0;
  color: #a3a3a3;
}

.chat-process-divider-line {
  flex: 1;
  height: 1px;
  background: #e5e5e5;
}

.chat-process-transcript {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 0 0 2px;
}

.chat-process-narration {
  color: #171717;
  font-size: 14.5px;
  line-height: 1.62;
}

.chat-process-step {
  color: #a3a3a3;
  font-size: 13.5px;
  line-height: 1.52;
}

.chat-process-step.is-thinking {
  color: #b3b3b3;
}

.chat-process-command,
.chat-process-search {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-process-command-row,
.chat-process-search-row {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: #a3a3a3;
  font-size: 13.5px;
  line-height: 1.52;
  text-align: left;
  cursor: pointer;
}

.chat-process-command-row:hover,
.chat-process-search-row:hover {
  color: #737373;
}

.chat-process-command-label {
  flex-shrink: 0;
}

.chat-process-command-code {
  min-width: 0;
  max-width: min(700px, 100%);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: inherit;
  background: transparent;
  font-family: inherit;
  font-size: inherit;
  line-height: inherit;
}

.chat-process-detail-panel {
  width: min(700px, 100%);
  max-height: 260px;
  overflow: auto;
  border-radius: 10px;
  background: #f7f7f8;
  padding: 10px 12px;
  color: #525252;
  font-size: 12.5px;
  line-height: 1.55;
}

.chat-process-detail-line {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.chat-process-detail-label {
  flex-shrink: 0;
  color: #8a8a8a;
  font-weight: 600;
}

.chat-process-result + .chat-process-result {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid #e4e4e7;
}

.chat-process-result-title {
  color: #262626;
  font-weight: 600;
}

.chat-process-result-url {
  overflow-wrap: anywhere;
  color: #737373;
}

.chat-process-result-snippet {
  margin-top: 2px;
  color: #737373;
}

.chat-process-plan,
.chat-process-evidence,
.chat-process-approval,
.chat-process-typed {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-process-step-title {
  color: #525252;
}

.chat-process-plan-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding-left: 20px;
  color: #737373;
}

.chat-process-plan-list li.is-running,
.chat-process-plan-list li.is-in_progress {
  color: #404040;
}

.chat-process-plan-list li.is-completed {
  color: #737373;
}

.chat-process-plan-status {
  margin-right: 8px;
  color: #a3a3a3;
  font-size: 12px;
  text-transform: uppercase;
}

.chat-process-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  color: #737373;
  font-size: 12px;
}

.chat-process-meta span {
  border-radius: 999px;
  background: #f4f4f5;
  padding: 2px 8px;
}

.chat-process-approval-command {
  width: fit-content;
  max-width: min(700px, 100%);
  overflow-wrap: anywhere;
  border-radius: 6px;
  background: #f4f4f5;
  padding: 4px 7px;
  color: #404040;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 12.5px;
}

.chat-process-typed-command {
  width: fit-content;
  max-width: min(700px, 100%);
  overflow-wrap: anywhere;
  border-radius: 6px;
  background: #f4f4f5;
  padding: 4px 7px;
  color: #404040;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 12.5px;
}

.chat-process-typed-summary {
  color: #737373;
  font-size: 13px;
  line-height: 1.5;
}

.chat-process-approval-reason {
  color: #737373;
  font-size: 13px;
  line-height: 1.5;
}
</style>
