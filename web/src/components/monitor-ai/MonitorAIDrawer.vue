<script setup>
import { ref, computed } from "vue";
import { sendMessage as sendMessageApi } from "../../api/chat";
import { useAppStore } from "../../store";
import {
  XIcon,
  SparklesIcon,
  SearchIcon,
  WrenchIcon,
  FileTextIcon,
  SendIcon,
  LoaderCircleIcon,
} from "lucide-vue-next";

const props = defineProps({
  /** monitoring context object passed from the parent page */
  monitorContext: {
    type: Object,
    default: () => ({}),
  },
});

const emit = defineEmits(["close"]);
const store = useAppStore();

const freeInput = ref("");
const resultText = ref("");
const loading = ref(false);
const errorMsg = ref("");

const quickActions = [
  { key: "explain",  label: "解释当前面板",   icon: SearchIcon },
  { key: "diagnose", label: "定位异常原因",   icon: SparklesIcon },
  { key: "fix",      label: "生成修复工作台", icon: WrenchIcon },
  { key: "draft",    label: "生成执行草稿",   icon: FileTextIcon },
];

const actionPromptMap = {
  explain:  "请解释当前监控面板的含义和关键指标。",
  diagnose: "请根据当前监控数据，定位可能的异常原因。",
  fix:      "请根据当前异常，生成修复工作台方案。",
  draft:    "请根据当前状态，生成可执行的操作草稿。",
};

const canSend = computed(() => freeInput.value.trim().length > 0 && !loading.value);

async function sendMessage(message) {
  loading.value = true;
  errorMsg.value = "";
  resultText.value = "";

  try {
    await sendMessageApi({
      message,
      sessionId: store.activeSessionId || store.snapshot.sessionId,
      hostId: store.snapshot.selectedHostId || "server-local",
      monitorContext: props.monitorContext,
    });
    resultText.value = "请求已发送，请在主对话面板查看 AI 回复。";
  } catch (e) {
    errorMsg.value = "网络异常: " + e.message;
  } finally {
    loading.value = false;
  }
}

function handleQuickAction(key) {
  const prompt = actionPromptMap[key] || key;
  sendMessage(prompt);
}

function handleFreeSubmit() {
  if (!canSend.value) return;
  const msg = freeInput.value.trim();
  freeInput.value = "";
  sendMessage(msg);
}

function handleKeydown(e) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    handleFreeSubmit();
  }
}
</script>

<template>
  <div class="monitor-ai-drawer">
    <!-- Quick Actions -->
    <div class="quick-actions">
      <button
        v-for="action in quickActions"
        :key="action.key"
        class="action-btn"
        :disabled="loading"
        @click="handleQuickAction(action.key)"
      >
        <component :is="action.icon" :size="16" />
        <span>{{ action.label }}</span>
      </button>
    </div>

    <!-- Free Input -->
    <div class="free-input-area">
      <textarea
        v-model="freeInput"
        class="free-input"
        placeholder="输入自定义问题…"
        rows="2"
        :disabled="loading"
        @keydown="handleKeydown"
      />
      <button
        class="send-btn"
        :disabled="!canSend"
        title="发送"
        @click="handleFreeSubmit"
      >
        <SendIcon :size="16" />
      </button>
    </div>

    <!-- Result Panel -->
    <div class="result-panel">
      <div v-if="loading" class="result-loading">
        <LoaderCircleIcon :size="18" class="spin" />
        <span>AI 正在分析…</span>
      </div>
      <div v-else-if="errorMsg" class="result-error">{{ errorMsg }}</div>
      <div v-else-if="resultText" class="result-text">{{ resultText }}</div>
      <div v-else class="result-empty">选择快捷动作或输入问题，AI 将结合监控上下文回答。</div>
    </div>
  </div>
</template>

<style scoped>
.monitor-ai-drawer {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.quick-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  padding: 16px 0;
  flex-shrink: 0;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 12px;
  background: #fff;
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  color: #0f172a;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.action-btn:hover:not(:disabled) {
  border-color: #93c5fd;
  background: #eff6ff;
}
.action-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.free-input-area {
  display: flex;
  gap: 8px;
  padding: 0 0 14px;
  flex-shrink: 0;
  align-items: flex-end;
}

.free-input {
  flex: 1;
  padding: 10px 12px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 10px;
  background: #fff;
  font: inherit;
  font-size: 13px;
  color: #0f172a;
  resize: none;
  outline: none;
}
.free-input:focus { border-color: #93c5fd; }

.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: none;
  border-radius: 10px;
  background: #2563eb;
  color: #fff;
  cursor: pointer;
  flex-shrink: 0;
}
.send-btn:hover:not(:disabled) { background: #1d4ed8; }
.send-btn:disabled { opacity: 0.45; cursor: not-allowed; }

.result-panel {
  flex: 1;
  overflow-y: auto;
}

.result-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #64748b;
  font-size: 13px;
  padding: 20px 0;
}

.result-error {
  color: #dc2626;
  font-size: 13px;
  padding: 12px;
  background: #fee2e2;
  border-radius: 10px;
}

.result-text {
  font-size: 13px;
  line-height: 1.7;
  color: #334155;
  padding: 14px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  white-space: pre-wrap;
}

.result-empty {
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
  padding: 40px 0;
}

.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

@media (max-width: 640px) {
  .quick-actions { grid-template-columns: 1fr; }
}
</style>
