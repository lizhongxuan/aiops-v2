<script setup>
import { computed, ref } from "vue";
import { BotIcon } from "lucide-vue-next";
import MessageCard from "../MessageCard.vue";
import Omnibar from "../Omnibar.vue";
import ThinkingCard from "../ThinkingCard.vue";
import ChoiceCard from "../ChoiceCard.vue";
import ProtocolInlinePlanWidget from "./ProtocolInlinePlanWidget.vue";
import ProtocolBackgroundAgentsCard from "./ProtocolBackgroundAgentsCard.vue";
import ProtocolTurnGroup from "./ProtocolTurnGroup.vue";
import { useChatScrollState } from "../../composables/useChatScrollState";
import { useChatHistoryPager } from "../../composables/useChatHistoryPager";
import { useVirtualTurnList } from "../../composables/useVirtualTurnList";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  title: {
    type: String,
    default: "Protocol conversation",
  },
  subtitle: {
    type: String,
    default: "对话流、计划映射和后台执行状态",
  },
  messages: {
    type: Array,
    default: () => [],
  },
  formattedTurns: {
    type: Array,
    default: () => [],
  },
  conversationCards: {
    type: Array,
    default: () => [],
  },
  planCards: {
    type: Array,
    default: () => [],
  },
  stepItems: {
    type: Array,
    default: () => [],
  },
  backgroundAgents: {
    type: Array,
    default: () => [],
  },
  choiceCards: {
    type: Array,
    default: () => [],
  },
  choiceSubmitting: {
    type: Object,
    default: () => ({}),
  },
  choiceErrors: {
    type: Object,
    default: () => ({}),
  },
  runningAgents: {
    type: Array,
    default: () => [],
  },
  planSummary: {
    type: [Object, Array, String],
    default: () => [],
  },
  planSummaryLabel: {
    type: String,
    default: "",
  },
  planOverviewRows: {
    type: Array,
    default: () => [],
  },
  statusCard: {
    type: Object,
    default: null,
  },
  draft: {
    type: String,
    default: "",
  },
  draftPlaceholder: {
    type: String,
    default: "继续输入需求、约束或补充说明",
  },
  sending: {
    type: Boolean,
    default: false,
  },
  busy: {
    type: Boolean,
    default: false,
  },
  primaryActionOverride: {
    type: String,
    default: "",
  },
  showComposer: {
    type: Boolean,
    default: true,
  },
  allowFollowUp: {
    type: Boolean,
    default: false,
  },
  virtualizationSuspended: {
    type: Boolean,
    default: false,
  },
  emptyLabel: {
    type: String,
    default: "这里会显示主 Agent 的对话流。",
  },
  starterCard: {
    type: Object,
    default: null,
  },
  historyResetKey: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["update:draft", "send", "stop", "choice", "select-message", "process-item-select", "evidence-select", "plan-action", "agent-select", "open-history", "action", "detail", "pin", "refresh"]);

const draftModel = computed({
  get: () => props.draft,
  set: (value) => emit("update:draft", value),
});

const normalizedMessages = computed(() =>
  (Array.isArray(props.messages) && props.messages.length
    ? props.messages
    : Array.isArray(props.conversationCards)
      ? props.conversationCards
      : [])
    .map((message, index) => {
      const role = String(message?.role || message?.source || "assistant").toLowerCase();
      const isUser = role === "user" || role === "human";
      const text = [message?.text || message?.message || message?.summary || "", message?.detail || message?.note || ""]
        .map((item) => String(item || "").trim())
        .filter(Boolean)
        .join("\n\n");

      if (!text) return null;

      return {
        id: String(message?.id || message?.key || `message-${index}`),
        role,
        isUser,
        card: {
          id: String(message?.id || message?.key || `message-card-${index}`),
          type: isUser ? "UserMessageCard" : "AssistantMessageCard",
          role: isUser ? "user" : "assistant",
          text,
          status: message?.status || "",
        },
      };
    })
    .filter(Boolean),
);

const normalizedPlanCards = computed(() => {
  const source = Array.isArray(props.planCards) && props.planCards.length
    ? props.planCards
    : Array.isArray(props.stepItems)
      ? props.stepItems
      : Array.isArray(props.planSummary)
        ? props.planSummary
        : props.planSummary && typeof props.planSummary === "object"
          ? Object.entries(props.planSummary).map(([key, value], index) => ({
              id: key,
              step: key,
              status: value?.status || value?.state || "pending",
              statusLabel: value?.statusLabel || value?.label || "",
              hostAgent: value?.hostAgent || value?.hostAgents || value?.hosts || [],
              detail: value?.detail || value?.summary || value?.text || (typeof value === "string" ? value : ""),
              note: value?.note || "",
              tags: value?.tags || [],
              actions: value?.actions || value?.buttons || [],
              index: index + 1,
            }))
          : typeof props.planSummary === "string" && props.planSummary.trim()
            ? [
                {
                  id: "plan-summary",
                  step: "摘要",
                  status: "pending",
                  detail: props.planSummary.trim(),
                  index: 1,
                },
              ]
            : [];

  return source;
});

const normalizedBackgroundAgents = computed(() =>
  (Array.isArray(props.backgroundAgents) && props.backgroundAgents.length
    ? props.backgroundAgents
    : Array.isArray(props.runningAgents)
      ? props.runningAgents
      : []),
);

const normalizedChoiceCards = computed(() =>
  (Array.isArray(props.choiceCards) ? props.choiceCards : []).filter((card) => card?.status === "pending"),
);

function choiceRequestId(card) {
  return String(card?.requestId || card?.id || "");
}

const normalizedTurns = computed(() =>
  Array.isArray(props.formattedTurns) && props.formattedTurns.length ? props.formattedTurns : [],
);

const historyPager = useChatHistoryPager({
  items: computed(() => (normalizedTurns.value.length ? normalizedTurns.value : normalizedMessages.value)),
  resetKey: computed(() => props.historyResetKey),
  pageSize: 6,
  initialCount: 4,
  topThreshold: 64,
});

function rowClass(message) {
  return message.isUser ? "row-user" : "row-assistant";
}

function sendDraft() {
  emit("send", String(props.draft || "").trim());
}

function stopDraft() {
  emit("stop");
}

function submitChoice(payload) {
  emit("choice", payload);
}

function selectMessage(message, event) {
  if (event?.target?.closest("button")) return;
  emit("select-message", message);
}

function selectProcessItem(payload) {
  emit("process-item-select", payload);
}

function selectEvidence(payload) {
  emit("evidence-select", payload);
}

function forwardAction(payload) {
  emit("action", payload);
}

function forwardDetail(payload) {
  emit("detail", payload);
}

function forwardPin(payload) {
  emit("pin", payload);
}

function forwardRefresh(payload) {
  emit("refresh", payload);
}

function planAction(payload, plan) {
  emit("plan-action", { ...payload, plan });
}

function selectHost(payload, plan) {
  emit("plan-action", {
    action: { key: "host" },
    host: payload,
    plan,
  });
}

function selectAgent(agent) {
  emit("agent-select", agent);
}

const visibleStreamItems = computed(() => historyPager.visibleItems.value);

const streamSignature = computed(() => {
  if (visibleStreamItems.value.length) {
    const visibleSignature = visibleStreamItems.value
      .map((turn) => [
        turn.id,
        turn.processItems?.length || 0,
        turn.finalMessage?.id || "",
        turn.finalMessage?.card?.text?.length || 0,
        turn.liveHint || "",
      ].join(":"))
      .join("|");
    return [
      visibleSignature,
      props.statusCard?.phase || "",
      props.statusCard?.hint || "",
      historyPager.topSentinel.value?.kind || "",
    ].join("|");
  }
  return [
    visibleStreamItems.value.length,
    visibleStreamItems.value[visibleStreamItems.value.length - 1]?.id || "",
    visibleStreamItems.value[visibleStreamItems.value.length - 1]?.card?.text?.length || 0,
    props.statusCard?.phase || "",
    props.statusCard?.hint || "",
    historyPager.topSentinel.value?.kind || "",
  ].join("|");
});

/* ---- Scroll / unread ---- */
const scrollContainer = ref(null);
const scrollContent = ref(null);
const {
  unreadCount,
  unreadAnchorId,
  showUnreadPill,
  handleScroll,
  jumpToLatest,
  markRead,
} = useChatScrollState({
  scrollContainer,
  scrollContent,
  items: visibleStreamItems,
  signature: streamSignature,
  getItemId: (item) => String(item?.id || ""),
});

const virtualTurns = useVirtualTurnList({
  items: visibleStreamItems,
  scrollContainer,
  suspended: computed(() => props.virtualizationSuspended),
  estimateSize(turn) {
    if (turn?.active) return 220;
    return turn?.collapsedByDefault ? 180 : 212;
  },
  overscan: 4,
  minItemCount: 18,
  getItemKey: (turn, index) => turn?.id || index,
});

const renderedTurns = computed(() => {
  if (!visibleStreamItems.value.length) return [];
  const rows = [];
  if (virtualTurns.enabled.value && virtualTurns.topSpacerHeight.value > 0) {
    rows.push({
      id: `turn-spacer-top-${virtualTurns.startIndex.value}`,
      kind: "spacer",
      spacer: "top",
      height: virtualTurns.topSpacerHeight.value,
    });
  }
  for (const entry of virtualTurns.virtualItems.value) {
    const turn = entry.item;
    if (unreadAnchorId.value && turn.id === unreadAnchorId.value) {
      rows.push({
        id: `unread-divider-${turn.id}`,
        kind: "divider",
      });
    }
    rows.push({
      id: turn.id,
      kind: "turn",
      turn,
    });
  }
  if (virtualTurns.enabled.value && virtualTurns.bottomSpacerHeight.value > 0) {
    rows.push({
      id: `turn-spacer-bottom-${virtualTurns.endIndex.value}`,
      kind: "spacer",
      spacer: "bottom",
      height: virtualTurns.bottomSpacerHeight.value,
    });
  }
  return rows;
});

const showLegacyMessageStream = computed(() => {
  if (visibleStreamItems.value.length && normalizedTurns.value.length) return false;
  if (!visibleStreamItems.value.length && !props.statusCard) return false;
  if (normalizedPlanCards.value.length || normalizedBackgroundAgents.value.length) {
    return false;
  }
  return true;
});

function openHistory() {
  emit("open-history");
}

async function loadOlderHistory() {
  const loaded = await historyPager.loadOlder();
  if (loaded) {
    markRead();
  }
}

function handlePaneScroll(event) {
  handleScroll(event);
  virtualTurns.handleScroll(event);
}
</script>

<template>
  <section class="protocol-conversation-pane">
    <div class="chat-container protocol-chat-container" ref="scrollContainer" @scroll="handlePaneScroll">
      <div class="chat-stream-inner protocol-chat-inner" ref="scrollContent">
        <div v-if="historyPager.topSentinel.value" class="protocol-history-sentinel" :class="`is-${historyPager.topSentinel.value.kind}`" data-testid="protocol-history-sentinel">
          <span class="protocol-history-sentinel-text">{{ historyPager.topSentinel.value.text }}</span>
          <span v-if="historyPager.topSentinel.value.detail" class="protocol-history-sentinel-detail">{{ historyPager.topSentinel.value.detail }}</span>
          <div class="protocol-history-sentinel-actions">
            <button
              v-if="historyPager.topSentinel.value.actionLabel === '加载更早消息' || historyPager.topSentinel.value.actionLabel === '重试'"
              type="button"
              class="protocol-history-sentinel-btn primary"
              data-testid="protocol-history-load-older"
              @click="loadOlderHistory"
            >
              {{ historyPager.topSentinel.value.actionLabel }}
            </button>
            <button
              v-if="historyPager.topSentinel.value.kind !== 'loading'"
              type="button"
              class="protocol-history-sentinel-btn secondary"
              data-testid="protocol-history-open"
              @click="openHistory"
            >
              查看完整历史
            </button>
          </div>
        </div>

        <div v-if="visibleStreamItems.length && normalizedTurns.length" class="chat-stream protocol-chat-stream protocol-turn-stream">
          <template v-for="entry in renderedTurns" :key="entry.id">
            <div
              v-if="entry.kind === 'spacer'"
              class="protocol-turn-spacer"
              :class="`is-${entry.spacer}`"
              :style="{ height: `${entry.height}px` }"
              aria-hidden="true"
            />

            <div v-else-if="entry.kind === 'divider'" class="protocol-unread-divider" data-testid="protocol-unread-divider">
              <span class="protocol-unread-divider-line" />
              <span class="protocol-unread-divider-label">未读更新</span>
              <span class="protocol-unread-divider-count">{{ unreadCount }} 条新结果</span>
              <span class="protocol-unread-divider-line" />
            </div>

            <ProtocolTurnGroup
              v-else
              :turn="entry.turn"
              @select-message="selectMessage"
              @select-process-item="selectProcessItem"
              @evidence-select="selectEvidence"
              @action="forwardAction"
              @detail="forwardDetail"
              @pin="forwardPin"
              @refresh="forwardRefresh"
            />
          </template>

          <div v-if="statusCard" class="stream-row row-assistant protocol-thinking-row" data-testid="protocol-live-status-card">
            <ThinkingCard :card="statusCard" />
          </div>
        </div>

        <div v-else-if="showLegacyMessageStream" class="chat-stream protocol-chat-stream">
          <div
            v-for="message in visibleStreamItems"
            :key="message.id"
            class="stream-row protocol-stream-row"
            :class="rowClass(message)"
            @click="selectMessage(message, $event)"
          >
            <MessageCard :card="message.card" />
          </div>

          <div v-if="statusCard" class="stream-row row-assistant protocol-thinking-row" data-testid="protocol-live-status-card">
            <ThinkingCard :card="statusCard" />
          </div>
        </div>

        <div v-else class="protocol-starter-thread">
          <div class="protocol-starter-kicker">
            <BotIcon size="14" />
            <span>{{ starterCard?.eyebrow || "SYSTEM CONTEXT" }}</span>
          </div>
          <div class="protocol-starter-copy">
            <p class="protocol-starter-title">{{ starterCard?.title || title }}</p>
            <p class="protocol-starter-text">{{ starterCard?.text || emptyLabel }}</p>
            <div v-if="starterCard?.meta" class="protocol-starter-meta">{{ starterCard.meta }}</div>
          </div>
        </div>
      </div>
    </div>

    <button
      v-if="showUnreadPill"
      type="button"
      class="protocol-unread-pill"
      data-testid="protocol-unread-pill"
      @click="jumpToLatest"
    >
      {{ unreadCount }} 条新结果
    </button>

    <footer v-if="showComposer" class="omnibar-dock protocol-omnibar-dock">
      <div
        v-if="normalizedChoiceCards.length || normalizedPlanCards.length || normalizedBackgroundAgents.length"
        class="protocol-composer-widgets"
      >
        <div v-if="normalizedChoiceCards.length" class="protocol-composer-choice-stack" data-testid="protocol-choice-stack">
          <ChoiceCard
            v-for="choiceCard in normalizedChoiceCards"
            :key="choiceCard.id"
            :card="choiceCard"
            :submitting="Boolean(choiceSubmitting[choiceRequestId(choiceCard)])"
            :error-message="choiceErrors[choiceRequestId(choiceCard)] || ''"
            session-kind="workspace"
            @choice="submitChoice"
          />
        </div>

        <ProtocolInlinePlanWidget
          v-if="normalizedPlanCards.length"
          docked
          :initially-expanded="false"
          :steps="normalizedPlanCards"
          :summary-label="planSummaryLabel"
          :overview-rows="planOverviewRows"
          @plan-action="({ action }) => planAction({ action }, null)"
          @step-action="({ action, plan }) => planAction({ action }, plan)"
          @host-select="({ host, plan }) => selectHost(host, plan)"
        />

        <ProtocolBackgroundAgentsCard
          v-if="normalizedBackgroundAgents.length"
          docked
          :agents="normalizedBackgroundAgents"
          @select="selectAgent"
        />
      </div>

      <Omnibar
        v-model="draftModel"
        :placeholder="draftPlaceholder"
        :disabled="sending"
        :busy="busy"
        :force-enabled="true"
        :allow-follow-up="allowFollowUp"
        :primary-action-override="primaryActionOverride"
        @send="sendDraft"
        @stop="stopDraft"
      />
    </footer>
  </section>
</template>

<style scoped>
.protocol-conversation-pane {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  background: #ffffff;
}

.protocol-chat-container {
  padding-top: 20px;
  padding-bottom: 220px;
  padding-left: 28px;
  padding-right: 28px;
}

.protocol-history-sentinel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 820px;
  width: 100%;
  margin: 0 auto 10px;
  padding: 12px 14px;
  border: 1px solid #dbe3ee;
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.94);
  color: #334155;
}

.protocol-history-sentinel.is-loading {
  background: rgba(239, 246, 255, 0.98);
  border-color: rgba(147, 197, 253, 0.55);
}

.protocol-history-sentinel.is-error {
  background: rgba(255, 247, 237, 0.96);
  border-color: rgba(251, 146, 60, 0.35);
}

.protocol-history-sentinel-text {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
}

.protocol-history-sentinel-detail {
  font-size: 12px;
  line-height: 1.5;
  color: #64748b;
}

.protocol-history-sentinel-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 8px;
}

.protocol-history-sentinel-btn {
  height: 30px;
  padding: 0 12px;
  border-radius: 999px;
  border: 1px solid #cbd5e1;
  background: white;
  color: #0f172a;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.protocol-history-sentinel-btn.primary {
  background: #eff6ff;
  border-color: rgba(59, 130, 246, 0.22);
  color: #1d4ed8;
}

.protocol-history-sentinel-btn.secondary {
  background: white;
}

.protocol-chat-inner {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  max-width: 1080px;
  margin: 0 auto;
}

.protocol-chat-stream {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.protocol-turn-stream {
  gap: 8px;
}

.protocol-turn-spacer {
  width: 100%;
  flex: 0 0 auto;
  pointer-events: none;
}

.protocol-unread-divider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 2px 0;
}

.protocol-unread-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(59, 130, 246, 0.18);
}

.protocol-unread-divider-label {
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
}

.protocol-unread-divider-count {
  color: #64748b;
  font-size: 12px;
  line-height: 1.4;
}

.protocol-stream-row {
  cursor: default;
  margin-bottom: 0;
}

.protocol-agent-section {
  width: min(980px, 100%);
  margin: 0 auto;
}

.protocol-composer-widgets {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
  max-width: 1080px;
  margin: 0 auto 6px;
}

.protocol-composer-choice-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.protocol-composer-choice-stack :deep(.choice-card) {
  margin-left: 0;
  max-width: 100%;
}

.protocol-omnibar-dock {
  padding-bottom: 20px;
  padding-left: 20px;
  padding-right: 20px;
}

.protocol-unread-pill {
  position: absolute;
  left: 50%;
  bottom: 138px;
  transform: translateX(-50%);
  z-index: 4;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid rgba(59, 130, 246, 0.18);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.12);
  color: #1d4ed8;
  font-size: 12.5px;
  font-weight: 600;
}

.protocol-conversation-pane :deep(.message-wrapper) {
  gap: 6px;
  align-items: flex-start;
}

.protocol-conversation-pane :deep(.stream-row) {
  width: min(980px, 100%);
  margin-inline: auto;
}

.protocol-conversation-pane :deep(.message-content) {
  max-width: min(100ch, calc(100% - 24px)) !important;
}

.protocol-conversation-pane :deep(.message-text) {
  font-size: 13.25px !important;
  line-height: 1.5 !important;
  letter-spacing: 0;
  color: #0f172a;
}

.protocol-conversation-pane :deep(.message-line) {
  line-height: 1.5 !important;
}

.protocol-conversation-pane :deep(.rich-message) {
  gap: 2px;
}

.protocol-conversation-pane :deep(.message-spacer) {
  height: 2px;
}

/* Markdown body spacing in protocol pane */
.protocol-conversation-pane :deep(.markdown-body p) {
  margin: 0 0 1px;
}

.protocol-conversation-pane :deep(.markdown-body ul),
.protocol-conversation-pane :deep(.markdown-body ol) {
  margin: 1px 0 3px;
}

.protocol-conversation-pane :deep(.markdown-body li) {
  margin: 0 0 1px;
}

.protocol-conversation-pane :deep(.avatar),
.protocol-conversation-pane :deep(.user-avatar) {
  display: none;
}

.protocol-conversation-pane :deep(.message-wrapper) {
  gap: 0;
  align-items: flex-start;
}

.protocol-conversation-pane :deep(.message-content) {
  max-width: min(100ch, 100%) !important;
}

.protocol-conversation-pane :deep(.relative-block) {
  max-width: min(100ch, calc(100vw - 320px)) !important;
}

.protocol-conversation-pane :deep(.copy-btn) {
  bottom: 4px;
  right: -2px;
  border-radius: 6px;
}

.protocol-conversation-pane :deep(.is-user .message-content) {
  max-width: min(46ch, 64%) !important;
}

.protocol-conversation-pane :deep(.is-user .message-text) {
  background: #f3f4f6 !important;
  padding: 7px 12px !important;
  border-radius: 14px !important;
  color: #0f172a;
  display: inline-block;
  font-size: 13.25px !important;
  font-weight: 400;
  line-height: 1.5 !important;
}

.protocol-conversation-pane :deep(.thinking-wrapper) {
  padding: 0;
  margin-left: 0;
}

.protocol-conversation-pane :deep(.thinking-indicator) {
  gap: 4px;
  padding: 7px 12px 8px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.03);
  color: #475569;
  max-width: min(88ch, calc(100vw - 300px));
}

.protocol-conversation-pane :deep(.thinking-text) {
  color: #0f172a;
  font-size: 13px;
  font-weight: 500;
}

.protocol-conversation-pane :deep(.thinking-detail) {
  color: #64748b;
  font-size: 11.5px;
  line-height: 1.45;
}

.protocol-conversation-pane :deep(.omnibar-wrapper) {
  max-width: 820px !important;
  border-radius: 16px !important;
  border: 1px solid var(--border-color);
  padding: 10px 12px 9px !important;
  background: var(--omnibar-bg);
  box-shadow: 0 4px 18px rgba(15, 23, 42, 0.06);
  gap: 6px;
}

.protocol-conversation-pane :deep(.omnibar-wrapper:focus-within) {
  border-color: #cbd5e1;
  box-shadow: 0 12px 40px rgba(15, 23, 42, 0.12);
  background: #ffffff;
}

.protocol-conversation-pane :deep(.omnibar-input) {
  min-height: 60px !important;
  font-size: 14px !important;
  line-height: 1.5 !important;
  padding: 2px 4px 0 !important;
  color: #0f172a;
}

.protocol-conversation-pane :deep(.omnibar-input::placeholder) {
  color: #94a3b8;
}

.protocol-conversation-pane :deep(.omnibar-tools) {
  padding-top: 0;
  border-top: none;
}

.protocol-conversation-pane :deep(.hint-text) {
  font-size: 11px;
  color: #94a3b8;
  font-weight: 500;
}

.protocol-conversation-pane :deep(.pill-tag) {
  background: rgba(15, 23, 42, 0.06);
  color: #334155;
}

.protocol-conversation-pane :deep(.send-btn) {
  width: 32px;
  height: 32px;
  background: #0f172a;
  box-shadow: none;
}

.protocol-conversation-pane :deep(.send-btn.stop-btn) {
  background: #dc2626;
}

.protocol-starter-thread {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-width: 1020px;
  margin: 8px auto 0;
  padding-bottom: 6px;
}

.protocol-starter-kicker {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #94a3b8;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.protocol-starter-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.protocol-starter-title {
  margin: 0;
  color: #0f172a;
  font-size: 15px;
  line-height: 1.45;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.protocol-starter-text {
  margin: 0;
  color: #4b5563;
  font-size: 14px;
  line-height: 1.6;
}

.protocol-starter-meta {
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
}

.protocol-conversation-pane :deep(.stop-link-btn) {
  border-color: rgba(226, 232, 240, 0.95);
  color: #475569;
  background: #ffffff;
}

.protocol-conversation-pane :deep(.stop-link-btn:hover) {
  background: #f8fafc;
}

@media (max-width: 900px) {
  .protocol-agent-section {
    width: 100%;
    margin-inline: auto;
  }

  .protocol-chat-container,
  .protocol-omnibar-dock {
    padding-left: 18px;
    padding-right: 18px;
  }

  .protocol-conversation-pane :deep(.thinking-wrapper) {
    margin-left: 0;
  }

  .protocol-conversation-pane :deep(.relative-block),
  .protocol-conversation-pane :deep(.thinking-indicator),
  .protocol-conversation-pane :deep(.message-content),
  .protocol-conversation-pane :deep(.is-user .message-content) {
    max-width: 100%;
  }

  .protocol-conversation-pane :deep(.omnibar-wrapper) {
    border-radius: 26px !important;
    padding: 18px 18px 16px !important;
  }

  .protocol-conversation-pane :deep(.omnibar-input) {
    min-height: 120px !important;
  }

  .protocol-starter-thread {
    max-width: 100%;
    margin-left: 0;
  }

  .protocol-starter-title {
    font-size: 20px;
  }

  .protocol-starter-text {
    font-size: 16px;
  }
}
</style>
