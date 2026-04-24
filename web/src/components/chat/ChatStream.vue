<script setup>
import { ref } from "vue";
import { BotIcon } from "lucide-vue-next";
import CardItem from "../CardItem.vue";
import ThinkingCard from "../ThinkingCard.vue";
import ChatTurnGroup from "./ChatTurnGroup.vue";
import LiveStatusCard from "./LiveStatusCard.vue";

const props = defineProps({
  containerStyle: {
    type: [Object, Array, String],
    default: null,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  showWorkspaceBanner: {
    type: Boolean,
    default: false,
  },
  workspaceSessionLabel: {
    type: String,
    default: "",
  },
  workspaceDetailLinkLabel: {
    type: String,
    default: "",
  },
  showEmptyState: {
    type: Boolean,
    default: false,
  },
  noticeMessage: {
    type: String,
    default: "",
  },
  selectedHostAlert: {
    type: String,
    default: "",
  },
  errorMessage: {
    type: String,
    default: "",
  },
  entries: {
    type: Array,
    default: () => [],
  },
  virtualItems: {
    type: Array,
    default: () => [],
  },
  unreadCount: {
    type: Number,
    default: 0,
  },
  showVirtualTopSpacer: {
    type: Boolean,
    default: false,
  },
  showVirtualBottomSpacer: {
    type: Boolean,
    default: false,
  },
  virtualTopSpacerHeight: {
    type: Number,
    default: 0,
  },
  virtualBottomSpacerHeight: {
    type: Number,
    default: 0,
  },
  singleHostLiveTurnId: {
    type: String,
    default: "",
  },
  workingElapsedLabel: {
    type: String,
    default: "",
  },
  singleHostLiveActivityLines: {
    type: Array,
    default: () => [],
  },
  latestRunningCommandCard: {
    type: Object,
    default: null,
  },
  feedbackByMessageId: {
    type: Object,
    default: () => ({}),
  },
  activeActivityLine: {
    type: String,
    default: "",
  },
  activeLineExpandable: {
    type: Boolean,
    default: false,
  },
  summaryLine: {
    type: String,
    default: "",
  },
  summaryExpandable: {
    type: Boolean,
    default: false,
  },
  showFileDetails: {
    type: Boolean,
    default: false,
  },
  viewedFileDetails: {
    type: Array,
    default: () => [],
  },
  showSearchDetails: {
    type: Boolean,
    default: false,
  },
  searchedQueryDetails: {
    type: Array,
    default: () => [],
  },
  thinkingCard: {
    type: Object,
    default: null,
  },
  isWorkspaceSession: {
    type: Boolean,
    default: false,
  },
  getRowClass: {
    type: Function,
    default: () => "",
  },
});

const emit = defineEmits([
  "scroll",
  "turn-action",
  "mcp-detail",
  "mcp-pin",
  "mcp-refresh",
  "workspace-detail",
  "toggle-active-details",
  "toggle-summary-details",
  "approval",
  "choice",
  "retry",
  "card-refresh",
]);

const scrollContainerEl = ref(null);
const scrollContentEl = ref(null);

defineExpose({
  scrollContainerEl,
  scrollContentEl,
});

function forwardScroll(event) {
  emit("scroll", event);
}

function emitTurnAction(payload) {
  emit("turn-action", payload);
}

function emitMcpDetail(payload) {
  emit("mcp-detail", payload);
}

function emitMcpPin(payload) {
  emit("mcp-pin", payload);
}

function emitMcpRefresh(payload) {
  emit("mcp-refresh", payload);
}

function openWorkspaceDetail() {
  emit("workspace-detail");
}

function toggleActiveDetails() {
  emit("toggle-active-details");
}

function toggleSummaryDetails() {
  emit("toggle-summary-details");
}
</script>

<template>
  <div class="chat-container" ref="scrollContainerEl" :style="containerStyle" @scroll="forwardScroll">
    <div class="chat-stream-inner" ref="scrollContentEl">
      <div v-if="loading" class="chat-banner loading-banner">
        <span class="spinner"></span> 正在初始化...
      </div>

      <div v-if="showWorkspaceBanner" class="workspace-banner">
        <div class="workspace-banner-copy">
          <strong>{{ workspaceSessionLabel }}</strong>
          <span>下方卡片是后端投影出的只读过程和结果，不会直接改写当前会话。</span>
        </div>
        <button class="workspace-banner-btn" @click="openWorkspaceDetail">{{ workspaceDetailLinkLabel }}</button>
      </div>

      <div v-if="showEmptyState" class="empty-state-canvas">
        <BotIcon size="48" class="empty-icon" />
        <h2>What can I help you build?</h2>
        <p>I can help you write code, manage servers, execute commands, and orchestrate complex tasks.</p>
      </div>

      <p v-if="noticeMessage" class="chat-banner info">{{ noticeMessage }}</p>

      <p v-if="selectedHostAlert" class="chat-banner warn">{{ selectedHostAlert }}</p>

      <p v-if="errorMessage" class="chat-banner error">{{ errorMessage }}</p>

      <div class="chat-stream">
        <div
          v-if="showVirtualTopSpacer"
          class="chat-virtual-spacer"
          data-testid="chat-virtual-top-spacer"
          :style="{ height: `${virtualTopSpacerHeight}px` }"
          aria-hidden="true"
        />

        <template v-for="entry in virtualItems" :key="entry.id">
          <div v-if="entry.kind === 'divider'" class="chat-unread-divider" data-testid="chat-unread-divider">
            <span class="chat-unread-divider-line" />
            <span class="chat-unread-divider-label">未读更新</span>
            <span class="chat-unread-divider-count">{{ unreadCount }} 条新结果</span>
            <span class="chat-unread-divider-line" />
          </div>

          <ChatTurnGroup
            v-else-if="entry.kind === 'turn'"
            :turn="entry.turn"
            :show-live-status="!isWorkspaceSession && entry.turn?.id === singleHostLiveTurnId"
            :feedback="feedbackByMessageId?.[entry.turn?.finalMessage?.id || ''] || ''"
            @action="emitTurnAction"
            @detail="emitMcpDetail"
            @pin="emitMcpPin"
            @refresh="emitMcpRefresh"
          >
            <template #live-status>
              <div class="stream-row row-assistant" data-testid="chat-live-status-card">
                <LiveStatusCard
                  :elapsed-label="workingElapsedLabel"
                  phase-label="Working for"
                  :activity-lines="singleHostLiveActivityLines"
                  :command-card="latestRunningCommandCard"
                />
              </div>
            </template>
          </ChatTurnGroup>

          <div v-else-if="entry.kind === 'card'" class="stream-row" :class="getRowClass(entry.card)">
            <CardItem
              :card="entry.card"
              :session-kind="isWorkspaceSession ? 'workspace' : 'single_host'"
              @approval="$emit('approval', $event)"
              @choice="$emit('choice', $event)"
              @retry="$emit('retry', $event)"
              @refresh="$emit('card-refresh', $event)"
            />
          </div>

          <div v-else-if="entry.kind === 'activity'" class="activity-summary">
            <button
              v-if="activeActivityLine"
              type="button"
              class="activity-line plain"
              :disabled="!activeLineExpandable"
              @click="toggleActiveDetails"
            >
              {{ activeActivityLine }}
            </button>

            <button
              v-else-if="summaryLine"
              type="button"
              class="activity-line"
              :disabled="!summaryExpandable"
              @click="toggleSummaryDetails"
            >
              {{ summaryLine }}
            </button>

            <div v-if="showFileDetails && viewedFileDetails.length" class="activity-details">
              <div v-for="entryItem in viewedFileDetails" :key="entryItem.label || entryItem.path" class="activity-detail-item">
                {{ entryItem.label || entryItem.path }}
              </div>
            </div>

            <div v-if="showSearchDetails && searchedQueryDetails.length" class="activity-details">
              <div v-for="entryItem in searchedQueryDetails" :key="entryItem.label || entryItem.query" class="activity-detail-item">
                {{ entryItem.label || entryItem.query }}
              </div>
            </div>
          </div>

          <div v-else-if="entry.kind === 'thinking'" class="stream-row row-assistant" data-testid="chat-live-status-card">
            <ThinkingCard v-if="isWorkspaceSession" :card="thinkingCard" />
            <LiveStatusCard
              v-else
              :elapsed-label="workingElapsedLabel"
              phase-label="Working for"
              :activity-lines="singleHostLiveActivityLines"
              :command-card="latestRunningCommandCard"
            />
          </div>
        </template>

        <div
          v-if="showVirtualBottomSpacer"
          class="chat-virtual-spacer"
          data-testid="chat-virtual-bottom-spacer"
          :style="{ height: `${virtualBottomSpacerHeight}px` }"
          aria-hidden="true"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.workspace-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin: 6px 0 3px;
  padding: 8px 10px;
  width: min(1040px, calc(100% - 40px));
  margin-left: auto;
  margin-right: auto;
  border-radius: 12px;
  border: 1px solid #dbeafe;
  background: linear-gradient(135deg, #eff6ff, #ffffff);
}

.workspace-banner-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #1e293b;
}

.workspace-banner-copy strong {
  font-size: 12px;
  font-weight: 700;
}

.workspace-banner-copy span {
  font-size: 11px;
  color: #475569;
  line-height: 1.45;
}

.workspace-banner-btn {
  flex-shrink: 0;
  border: 1px solid #bfdbfe;
  background: #ffffff;
  color: #1d4ed8;
  border-radius: 999px;
  padding: 7px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.workspace-banner-btn:hover {
  background: #eff6ff;
}

.chat-unread-divider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 2px 0 6px;
}

.chat-unread-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(59, 130, 246, 0.18);
}

.chat-unread-divider-label {
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
}

.chat-unread-divider-count {
  color: #64748b;
  font-size: 12px;
  line-height: 1.4;
}

.chat-virtual-spacer {
  width: 100%;
  flex: none;
  pointer-events: none;
}

.activity-summary {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 2px 0;
  width: min(1040px, calc(100% - 40px));
  margin: 0 auto;
  animation: fadeInUp 0.2s ease-out;
}

.activity-line {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  padding: 0;
  border: none;
  background: transparent;
  font-size: var(--text-meta-size, 11px);
  color: var(--text-meta, #9ca3af);
  font-weight: 500;
  cursor: pointer;
}

.activity-line:disabled {
  cursor: default;
}

.activity-line:not(:disabled):hover {
  color: #6b7280;
}

.activity-line.plain {
  color: #9ca3af;
}

.activity-details {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 2px;
  padding-left: 10px;
  color: #94a3b8;
  font-size: 11px;
}

.activity-detail-item {
  line-height: 1.45;
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
