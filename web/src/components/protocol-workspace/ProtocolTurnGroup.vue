<script setup>
import MessageCard from "../MessageCard.vue";
import ProtocolProcessFold from "./ProtocolProcessFold.vue";
import McpBundleHost from "../mcp/McpBundleHost.vue";
import McpUiCardHost from "../mcp/McpUiCardHost.vue";

const props = defineProps({
  turn: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["select-message", "select-process-item", "evidence-select", "action", "detail", "pin", "refresh"]);

function selectMessage(message, event) {
  if (!message || event?.target?.closest("button")) return;
  emit("select-message", message);
}

function selectProcessItem(item) {
  emit("select-process-item", {
    item,
    turn: props.turn,
  });
}

function selectEvidence(reference) {
  emit("evidence-select", {
    reference,
    turn: props.turn,
  });
}

function emitAction(payload) {
  emit("action", payload);
}

function handleBundleAction(payload) {
  const isTopLevelBundleRefresh =
    payload &&
    typeof payload === "object" &&
    (
      payload.bundleId ||
      payload.bundleKind ||
      payload.sections ||
      payload.target?.dataset?.testid === "mcp-bundle-action" ||
      payload.currentTarget?.dataset?.testid === "mcp-bundle-action"
    );

  if (isTopLevelBundleRefresh) {
    emit("refresh", payload);
    return;
  }
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
</script>

<template>
  <article class="protocol-turn-group" :data-testid="`protocol-turn-${turn.id}`">
    <div
      v-if="turn.userMessage"
      class="stream-row protocol-stream-row row-user protocol-turn-user"
      @click="selectMessage(turn.userMessage, $event)"
    >
      <MessageCard :card="turn.userMessage.card" />
    </div>

    <ProtocolProcessFold :turn="turn" @item-select="selectProcessItem" />

    <div v-if="turn.finalMessage" class="protocol-turn-final">
      <div v-if="turn.finalLabel" class="protocol-final-divider">
        <span class="protocol-final-divider-line" />
        <span class="protocol-final-divider-label">{{ turn.finalLabel }}</span>
        <span class="protocol-final-divider-line" />
      </div>

      <div class="stream-row protocol-stream-row row-assistant" @click="selectMessage(turn.finalMessage, $event)">
        <MessageCard :card="turn.finalMessage.card" />
      </div>

      <div
        v-if="turn.finalMessage.evidenceRefs?.length"
        class="protocol-turn-evidence"
        :data-testid="`protocol-turn-evidence-${turn.id}`"
      >
        <span class="protocol-turn-evidence-label">引用证据</span>
        <button
          v-for="reference in turn.finalMessage.evidenceRefs"
          :key="reference.evidenceId"
          type="button"
          class="protocol-turn-evidence-chip"
          :title="reference.summary || reference.title || reference.label"
          @click.stop="selectEvidence(reference)"
        >
          <strong>{{ reference.label }}</strong>
          <span v-if="reference.title || reference.summary">{{ reference.title || reference.summary }}</span>
        </button>
      </div>
    </div>

    <div v-if="turn.resultBundles?.length" class="protocol-turn-bundles">
      <McpBundleHost
        v-for="bundle in turn.resultBundles"
        :key="bundle.id"
        :bundle="bundle.model"
        @action="handleBundleAction"
        @open-detail="emitDetail"
        @pin="emitPin"
      />
    </div>

    <div v-if="turn.actionSurfaces?.length" class="protocol-turn-actions">
      <McpUiCardHost
        v-for="surface in turn.actionSurfaces"
        :key="surface.id"
        :card="surface.model"
        @action="emitAction"
        @detail="emitDetail"
        @refresh="emitRefresh"
      />
    </div>
  </article>
</template>

<style scoped>
.protocol-turn-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.protocol-turn-final {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 0;
}

.protocol-turn-bundles,
.protocol-turn-actions {
  display: grid;
  gap: 10px;
}

.protocol-turn-evidence {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: -1px;
  padding-left: 18px;
}

.protocol-turn-evidence-label {
  color: #64748b;
  font-size: 11px;
  font-weight: 700;
}

.protocol-turn-evidence-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: min(100%, 360px);
  padding: 6px 10px;
  border: 1px solid rgba(191, 219, 254, 0.9);
  border-radius: 999px;
  background: rgba(239, 246, 255, 0.88);
  color: #1d4ed8;
  font-size: 12px;
  cursor: pointer;
}

.protocol-turn-evidence-chip strong {
  flex-shrink: 0;
}

.protocol-turn-evidence-chip span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.protocol-final-divider {
  display: flex;
  align-items: center;
  gap: 10px;
}

.protocol-final-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(226, 232, 240, 0.82);
}

.protocol-final-divider-label {
  color: #64748b;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
}
</style>
