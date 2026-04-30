<script setup>
import { computed, ref, watch } from "vue";
import MessageCard from "../MessageCard.vue";
import ChatProcessFold from "./ChatProcessFold.vue";
import AssistantActionBar from "./AssistantActionBar.vue";
import McpBundleHost from "../mcp/McpBundleHost.vue";
import McpUiCardHost from "../mcp/McpUiCardHost.vue";

const props = defineProps({
  turn: {
    type: Object,
    required: true,
  },
  showLiveStatus: {
    type: Boolean,
    default: false,
  },
  feedback: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["action", "detail", "pin", "refresh"]);
const processExpanded = ref(false);

function finalMessageId(turn = {}) {
  return String(turn?.finalMessage?.id || turn?.finalMessage?.card?.id || "").trim();
}

function defaultProcessExpanded(turn = {}) {
  if (turn?.active) return true;
  if (finalMessageId(turn)) return false;
  return !turn?.collapsedByDefault;
}

watch(
  () => ({
    id: String(props.turn?.id || "").trim(),
    active: Boolean(props.turn?.active),
    finalMessageId: finalMessageId(props.turn),
  }),
  (current, previous) => {
    if (!previous || current.id !== previous.id) {
      processExpanded.value = defaultProcessExpanded(props.turn);
      return;
    }

    const finalArrived = !previous.finalMessageId && current.finalMessageId;
    const justFinished = previous.active && !current.active;
    if (current.finalMessageId && !current.active && (justFinished || finalArrived)) {
      processExpanded.value = false;
    }
  },
  { immediate: true },
);

const showActionBar = computed(() => {
  const finalCard = props.turn?.finalMessage?.card || null;
  if (props.turn?.active) return false;
  if (!finalCard) {
    return String(props.turn?.phase || "").trim().toLowerCase() === "failed" && !!props.turn?.userMessage;
  }
  return String(finalCard.status || "").toLowerCase() !== "inprogress";
});

const showRetryOnly = computed(() => !props.turn?.finalMessage && String(props.turn?.phase || "").trim().toLowerCase() === "failed");
const showProcessFold = computed(() => Boolean(props.turn?.processTranscript));
const finalMessageStreaming = computed(() => {
  const status = String(props.turn?.finalMessage?.card?.status || "").trim().toLowerCase();
  return status === "inprogress" || status === "streaming" || status === "running";
});
const showBottomThinkingStatus = computed(() => Boolean(props.turn?.processTranscript?.showThinking || finalMessageStreaming.value));
const primaryPanelPayload = computed(() => {
  if (props.turn?.actionSurfaces?.length) return props.turn.actionSurfaces[0]?.model || props.turn.actionSurfaces[0];
  if (props.turn?.resultBundles?.length) return props.turn.resultBundles[0]?.model || props.turn.resultBundles[0];
  if (props.turn?.actionBundles?.length) return props.turn.actionBundles[0]?.model || props.turn.actionBundles[0];
  if (props.turn?.workspaceSurfaces?.length) return props.turn.workspaceSurfaces[0]?.model || props.turn.workspaceSurfaces[0];
  return null;
});

function emitAction(payload) {
  emit("action", payload);
}

function emitDetail(payload) {
  emit("detail", payload);
}

function emitPin(payload) {
  emit("pin", payload);
}

function emitRefresh(payload) {
  emit("refresh", payload);
}

function handleRegenerate() {
  const prompt = props.turn?.userMessage?.card?.text || props.turn?.userMessage?.card?.message || "";
  if (!prompt) return;
  emitAction({
    type: "assistant_regenerate",
    turnId: props.turn?.id || "",
    messageId: props.turn?.finalMessage?.id || "",
    message: prompt,
  });
}

function handleFeedback(value) {
  emitAction({
    type: "assistant_feedback",
    turnId: props.turn?.id || "",
    messageId: props.turn?.finalMessage?.id || "",
    value,
  });
}

function handleOpenPanel() {
  if (!primaryPanelPayload.value) return;
  emitDetail(primaryPanelPayload.value);
}
</script>

<template>
  <article class="chat-turn-group" :data-testid="`chat-turn-${turn.id}`">
    <div v-if="turn.userMessage" class="stream-row row-user">
      <MessageCard :card="turn.userMessage.card" />
    </div>

    <ChatProcessFold
      v-if="showProcessFold"
      :turn="turn"
      :expanded="processExpanded"
      @update:expanded="processExpanded = $event"
    />

    <div v-if="turn.finalMessage" class="chat-turn-final">
      <div v-if="turn.finalLabel" class="chat-turn-final-divider">
        <span class="chat-turn-final-divider-line" />
        <span class="chat-turn-final-divider-label">{{ turn.finalLabel }}</span>
        <span class="chat-turn-final-divider-line" />
      </div>

      <div class="stream-row row-assistant">
        <MessageCard :card="turn.finalMessage.card" :show-copy-button="!showActionBar" />
      </div>

      <div
        v-if="showActionBar"
        class="chat-turn-action-bar"
        :data-testid="`assistant-action-bar-${turn.id}`"
      >
        <AssistantActionBar
          :copy-text="turn.finalMessage.card?.text || ''"
          :allow-copy="!showRetryOnly"
          :allow-regenerate="showRetryOnly"
          :allow-feedback="false"
          :regenerate-label="showRetryOnly ? '重试' : '重新生成'"
          :feedback="feedback"
          :can-open-panel="!!primaryPanelPayload"
          @regenerate="handleRegenerate"
          @open-panel="handleOpenPanel"
          @update:feedback="handleFeedback"
        />
      </div>
    </div>

    <div
      v-else-if="showActionBar"
      class="chat-turn-action-bar"
      :data-testid="`assistant-action-bar-${turn.id}`"
    >
      <AssistantActionBar
        copy-text=""
        :allow-regenerate="true"
        :allow-copy="false"
        :allow-feedback="false"
        regenerate-label="重试"
        :feedback="feedback"
        :can-open-panel="!!primaryPanelPayload"
        @regenerate="handleRegenerate"
        @open-panel="handleOpenPanel"
        @update:feedback="handleFeedback"
      />
    </div>

    <div v-if="turn.resultBundles?.length" class="chat-turn-bundles">
      <McpBundleHost
        v-for="bundle in turn.resultBundles"
        :key="bundle.id"
        :bundle="bundle.model"
        @action="emitAction"
        @open-detail="emitDetail"
        @pin="emitPin"
      />
    </div>

    <div v-if="turn.actionSurfaces?.length" class="chat-turn-actions">
      <McpUiCardHost
        v-for="surface in turn.actionSurfaces"
        :key="surface.id"
        :card="surface.model"
        @action="emitAction"
        @detail="emitDetail"
        @refresh="emitRefresh"
      />
    </div>

    <div
      v-if="showBottomThinkingStatus"
      class="chat-turn-thinking-row"
      data-testid="turn-thinking-row"
    >
      <div class="chat-turn-thinking-status" data-testid="turn-thinking-status">
        <span class="chat-turn-thinking-dot" aria-hidden="true" />
        <span>正在思考</span>
      </div>
    </div>
  </article>
</template>

<style scoped>
.chat-turn-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
  content-visibility: auto;
  contain-intrinsic-size: 1px 240px;
}

.chat-turn-final {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 0;
}

.chat-turn-action-bar {
  width: min(860px, 100%);
  margin: 0 auto;
}

.chat-turn-thinking-status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  max-width: min(860px, 100%);
  margin: 0;
  color: #737373;
  font-size: 13.5px;
  line-height: 1.52;
}

.chat-turn-thinking-row {
  display: flex;
  justify-content: flex-start;
  width: min(860px, 100%);
  margin: -2px auto 0;
}

.chat-turn-thinking-dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: #a3a3a3;
  animation: chat-turn-thinking-pulse 1.2s ease-in-out infinite;
}

@keyframes chat-turn-thinking-pulse {
  0%, 100% { opacity: 0.35; transform: scale(0.85); }
  50% { opacity: 1; transform: scale(1); }
}

.chat-turn-bundles,
.chat-turn-actions {
  display: grid;
  gap: 8px;
}

.chat-turn-final-divider {
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(860px, 100%);
  margin: 0 auto;
}

.chat-turn-final-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(226, 232, 240, 0.82);
}

.chat-turn-final-divider-label {
  color: #64748b;
  font-size: 10.5px;
  font-weight: 600;
  line-height: 1.4;
}
</style>
