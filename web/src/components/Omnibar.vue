<script setup>
import { ref, nextTick, computed, watch, toRef, onBeforeUnmount } from "vue";
import { useAppStore } from "../store";
import { usePasteAssist } from "../composables/usePasteAssist";
import { NButton, NAutoComplete } from "naive-ui";
import { resolveHostDisplay } from "../lib/hostDisplay";
import { selectActiveProjection, selectRuntimeBusy, selectRuntimeStatus } from "../events/agentEventProjection";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  placeholder: {
    type: String,
    default: "",
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  isDockedBottom: {
    type: Boolean,
    default: false,
  },
  allowFollowUp: {
    type: Boolean,
    default: false,
  },
  forceEnabled: {
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
});

const emit = defineEmits(["update:modelValue", "send", "stop"]);
const store = useAppStore();

const textareaRef = ref(null);
const attachmentHintVisible = ref(false);
const mentionPopover = ref({
  visible: false,
  query: "",
  x: 0,
  y: 0,
  selectedIndex: 0,
});
let attachmentHintTimer = null;

const mentionOptions = computed(() => {
  const query = mentionPopover.value.query.toLowerCase();

  return store.snapshot.hosts
    .map((host) => ({ type: "host", id: host.id, label: host.name }))
    .filter((option) => option.label.toLowerCase().includes(query))
    .slice(0, 5);
});

const activeMentions = computed(() => {
  const mentions = [];
  const seen = new Set();
  for (const match of props.modelValue.matchAll(/@([A-Za-z0-9._-]+)/g)) {
    const label = match[1];
    if (seen.has(label)) continue;
    seen.add(label);
    mentions.push(label);
  }
  return mentions;
});
const pasteAssist = usePasteAssist(toRef(props, "modelValue"));
const artifactPills = computed(() => pasteAssist.artifactPills.value);
const showToolTags = computed(() => activeMentions.value.length || artifactPills.value.length);
const activeSessionId = computed(() => store.activeSessionId || store.snapshot.sessionId || "");
const activeProjection = computed(() => selectActiveProjection(store.agentEventState, activeSessionId.value));
const turnActive = computed(() => selectRuntimeBusy(store.agentEventState, activeSessionId.value) || props.busy);
const turnPendingStart = computed(() => {
  const projection = activeProjection.value;
  const turnId = projection?.currentTurnId || "";
  const currentTurn = (projection?.timeline || []).find((row) => row?.kind === "turn" && row?.turnId === turnId);
  return currentTurn?.status === "queued";
});

const canStop = computed(() => {
  if (props.primaryActionOverride === "send") return false;
  if (props.primaryActionOverride === "stop") return true;
  return turnActive.value;
});
const forceSendAction = computed(() => props.primaryActionOverride === "send");

// Stabilize the stop button — once active, hold it for at least 2s to prevent flickering
const stableCanStop = ref(false);
let stopStabilityTimer = null;
watch(canStop, (value) => {
  if (value) {
    // Immediately show stop button
    stableCanStop.value = true;
    if (stopStabilityTimer) { clearTimeout(stopStabilityTimer); stopStabilityTimer = null; }
  } else {
    // Delay hiding the stop button to prevent flicker
    if (!stopStabilityTimer) {
      stopStabilityTimer = setTimeout(() => {
        stableCanStop.value = false;
        stopStabilityTimer = null;
      }, 2000);
    }
  }
}, { immediate: true });

const primaryAction = computed(() => (stableCanStop.value ? "stop" : "send"));
const canSendMessage = computed(() => (props.forceEnabled ? true : !!store.canSend));
const inputDisabled = computed(
  () =>
    props.disabled ||
    props.busy ||
    !canSendMessage.value ||
    store.sending,
);
const sendDisabled = computed(
  () =>
    props.disabled ||
    props.busy ||
    !canSendMessage.value ||
    store.sending ||
    (!forceSendAction.value && turnActive.value) ||
    pasteAssist.sendBlocked.value ||
    !props.modelValue.trim(),
);
const showSecondaryStop = computed(() => false);
const showHintText = computed(() => Boolean(hintText.value));
const showToolsLeft = computed(() => showToolTags.value || showHintText.value || pasteAssist.hasPendingArtifact.value);
const compactStopMode = computed(() => {
  return (
    primaryAction.value === "stop" &&
    !props.modelValue.trim() &&
    !showToolsLeft.value &&
    !slashResultVisible.value &&
    !slashCommandVisible.value &&
    !mentionPopover.value.visible
  );
});
const hintTestId = computed(() => {
  if (attachmentHintVisible.value) return "omnibar-attachment-hint";
  if (!pasteAssist.indicator.value) return "omnibar-hint";
  if (pasteAssist.indicator.value.kind === "focus") return "omnibar-focus-hint";
  if (pasteAssist.indicator.value.kind === "artifact") return "omnibar-attachment-indicator";
  return "omnibar-paste-indicator";
});
const hintText = computed(() => {
  if (attachmentHintVisible.value) return "可粘贴文本/路径，或拖入图片作为上下文。";
  if (pasteAssist.indicator.value) return pasteAssist.indicator.value.text;
  if (primaryAction.value === "stop" || turnPendingStart.value) return "";
  return "Enter 发送";
});
const primaryActionLabel = computed(() => (primaryAction.value === "stop" ? "停止生成" : "发送"));
const configuredModel = computed(() => String(store.snapshot?.config?.model || "").trim());
const modelLabel = computed(() => {
  const value = configuredModel.value;
  if (!value) return "";
  return value.replace(/^gpt/i, "GPT").replace(/^o([0-9])/i, "O$1");
});
const showExecutionControls = computed(() => Boolean(modelLabel.value));

function focus() {
  nextTick(() => textareaRef.value?.focus());
}

function showAttachmentHint() {
  attachmentHintVisible.value = true;
  if (attachmentHintTimer) {
    clearTimeout(attachmentHintTimer);
  }
  attachmentHintTimer = setTimeout(() => {
    attachmentHintVisible.value = false;
    attachmentHintTimer = null;
  }, 2600);
  focus();
}

onBeforeUnmount(() => {
  if (attachmentHintTimer) {
    clearTimeout(attachmentHintTimer);
  }
});

defineExpose({
  focus,
});

function emitSend() {
  // Check for /switch command before sending
  if (tryExecuteSlashSwitch(props.modelValue)) return;
  pasteAssist.resetPasteState();
  emit("send");
}

function onInput(e) {
  const text = e.target.value;
  emit("update:modelValue", text);

  // Check slash command trigger
  checkSlashTrigger(text);

  const cursor = e.target.selectionStart;
  const textBeforeCursor = text.slice(0, cursor);
  const match = textBeforeCursor.match(/@([A-Za-z0-9._-]*)$/);

  if (match) {
    const coords = getCaretCoordinates(e.target, cursor);
    mentionPopover.value = {
      visible: true,
      query: match[1],
      x: coords.left,
      y: coords.top - 160, // approximate popover height above
      selectedIndex: 0,
    };
    if (!mentionOptions.value.length) {
      mentionPopover.value.visible = false;
    }
  } else {
    mentionPopover.value.visible = false;
  }
}

function insertTextAtSelection(text) {
  const textarea = textareaRef.value;
  const currentValue = props.modelValue ?? "";
  if (!textarea) {
    emit("update:modelValue", `${currentValue}${text}`);
    return;
  }

  const hasFocus = typeof document !== "undefined" ? document.activeElement === textarea : false;
  const start = hasFocus && Number.isInteger(textarea.selectionStart) ? textarea.selectionStart : currentValue.length;
  const end = hasFocus && Number.isInteger(textarea.selectionEnd) ? textarea.selectionEnd : start;
  const nextValue = `${currentValue.slice(0, start)}${text}${currentValue.slice(end)}`;
  const nextCursor = start + text.length;

  textarea.value = nextValue;
  emit("update:modelValue", nextValue);

  nextTick(() => {
    if (!textareaRef.value) return;
    textareaRef.value.setSelectionRange(nextCursor, nextCursor);
  });
}

function onPaste(event) {
  pasteAssist.handlePaste(event);
  if (event?.defaultPrevented) return;

  const text = event?.clipboardData?.getData?.("text/plain") ?? "";
  if (!text) return;

  event.preventDefault();
  insertTextAtSelection(text);
}

function onDrop(event) {
  pasteAssist.handleDrop(event);
}

function onFocus() {
  pasteAssist.handleFocus();
}

function onBlur() {
  pasteAssist.handleBlur();
}

function onKeydown(e) {
  if (mentionPopover.value.visible && !mentionOptions.value.length) {
    mentionPopover.value.visible = false;
  }

  if (mentionPopover.value.visible) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      mentionPopover.value.selectedIndex = (mentionPopover.value.selectedIndex + 1) % mentionOptions.value.length;
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      mentionPopover.value.selectedIndex = (mentionPopover.value.selectedIndex - 1 + mentionOptions.value.length) % mentionOptions.value.length;
      return;
    }
    if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      selectMention(mentionOptions.value[mentionPopover.value.selectedIndex]);
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault();
      mentionPopover.value.visible = false;
      return;
    }
  }
  
  if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
    e.preventDefault();
    if (primaryAction.value === "stop") {
      emit("stop");
    } else if (!sendDisabled.value) {
      emitSend();
    }
    return;
  }

  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    if (primaryAction.value !== "stop" && !sendDisabled.value) {
      emitSend();
    }
  }
}

function selectMention(option) {
  if (!option) return;

  const text = props.modelValue;
  const cursor = textareaRef.value.selectionStart;
  const textBeforeCursor = text.slice(0, cursor);
  const match = textBeforeCursor.match(/@([A-Za-z0-9._-]*)$/);

  if (match) {
    const newText = text.slice(0, match.index) + `@${option.label} ` + text.slice(cursor);
    emit("update:modelValue", newText);
    mentionPopover.value.visible = false;

    nextTick(() => {
      textareaRef.value.focus();
      const newCursorPos = match.index + option.label.length + 2;
      textareaRef.value.setSelectionRange(newCursorPos, newCursorPos);
    });
  }
}

// Helper to get caret coords in textarea (naive approach for MVP)
function getCaretCoordinates(element, position) {
  // Simple approximation without creating mirror div
  return { left: 24, top: 0 }; 
}

// --- Slash command support ---
const slashCommandVisible = ref(false);
const slashQuery = ref("");

const SLASH_COMMANDS = [
  { label: "/hosts", value: "/hosts", description: "列出所有主机及状态" },
  { label: "/switch", value: "/switch ", description: "切换当前主机" },
  { label: "/approve", value: "/approve", description: "批量审批当前 pending 项" },
  { label: "/status", value: "/status", description: "显示系统状态摘要" },
];

const slashOptions = computed(() => {
  const q = slashQuery.value.toLowerCase();
  return SLASH_COMMANDS
    .filter((cmd) => cmd.label.toLowerCase().includes(q))
    .map((cmd) => ({
      label: `${cmd.label}  —  ${cmd.description}`,
      value: cmd.value,
    }));
});

function checkSlashTrigger(text) {
  const trimmed = text.trimStart();
  if (trimmed.startsWith("/")) {
    slashQuery.value = trimmed;
    slashCommandVisible.value = true;
  } else {
    slashCommandVisible.value = false;
  }
}

function handleSlashSelect(value) {
  slashCommandVisible.value = false;
  const cmd = value.trim();
  if (cmd === "/hosts") {
    executeSlashHosts();
  } else if (cmd === "/approve") {
    executeSlashApprove();
  } else if (cmd === "/status") {
    executeSlashStatus();
  } else if (cmd.startsWith("/switch ")) {
    // Set the input to "/switch " so user can type host name
    emit("update:modelValue", "/switch ");
    nextTick(() => textareaRef.value?.focus());
    return;
  } else if (cmd.startsWith("/switch")) {
    emit("update:modelValue", "/switch ");
    nextTick(() => textareaRef.value?.focus());
    return;
  }
  emit("update:modelValue", "");
}

const slashResultMessage = ref("");
const slashResultVisible = ref(false);

function showSlashResult(msg) {
  slashResultMessage.value = msg;
  slashResultVisible.value = true;
  setTimeout(() => { slashResultVisible.value = false; }, 6000);
}

function executeSlashHosts() {
  const hosts = store.snapshot.hosts || [];
  if (!hosts.length) {
    showSlashResult("当前没有已注册的主机。");
    return;
  }
  const lines = hosts.map((h) => `• ${h.name || h.id} — ${h.status || "unknown"}${h.address ? ` (${h.address})` : ""}`);
  showSlashResult(`主机列表：\n${lines.join("\n")}`);
}

async function executeSlashApprove() {
  const approvals = (store.snapshot.approvals || []).filter((a) => a.status === "pending");
  if (!approvals.length) {
    showSlashResult("当前没有待审批项。");
    return;
  }
  let approved = 0;
  for (const approval of approvals) {
    try {
      await submitApprovalDecision(approval.id, "approve");
      approved++;
    } catch (_) { /* skip */ }
  }
  showSlashResult(`已批量审批 ${approved}/${approvals.length} 项。`);
}

function executeSlashStatus() {
  const hosts = store.snapshot.hosts || [];
  const onlineCount = hosts.filter((h) => h.status === "online").length;
  const pendingApprovals = (store.snapshot.approvals || []).filter((a) => a.status === "pending").length;
  const runtimeStatus = selectRuntimeStatus(store.agentEventState, activeSessionId.value);
  const wsStatus = store.wsStatus || "unknown";
  const selectedHost = resolveHostDisplay(store.selectedHost) || "server-local";

  const lines = [
    `WebSocket: ${wsStatus}`,
    `主机: ${onlineCount}/${hosts.length} 在线`,
    `当前主机: ${selectedHost}`,
    `Turn: ${turnActive.value ? runtimeStatus : "空闲"}`,
    `待审批: ${pendingApprovals} 项`,
  ];
  showSlashResult(`系统状态：\n${lines.join("\n")}`);
}

// Handle /switch <host> on send
function tryExecuteSlashSwitch(text) {
  const match = text.trim().match(/^\/switch\s+(.+)$/i);
  if (!match) return false;
  const target = match[1].trim();
  const host = (store.snapshot.hosts || []).find(
    (h) => h.name === target || h.id === target || h.address === target,
  );
  if (!host) {
    showSlashResult(`未找到主机: ${target}`);
    emit("update:modelValue", "");
    return true;
  }
  store.createOrActivateSingleHostSessionForHost?.(host.id, host);
  showSlashResult(`已切换到主机: ${host.name || host.id}`);
  emit("update:modelValue", "");
  return true;
}


</script>

<template>
  <div
    class="omnibar-wrapper"
    :class="{
      'is-docked-bottom': isDockedBottom,
      'is-stop-mode': primaryAction === 'stop',
      'is-compact-stop': compactStopMode,
    }"
  >
    <!-- Slash command auto-complete -->
    <div v-if="slashCommandVisible && slashOptions.length" class="slash-popover">
      <div class="popover-header">Slash 命令</div>
      <div class="popover-list">
        <div
          v-for="opt in slashOptions"
          :key="opt.value"
          class="popover-item"
          @click="handleSlashSelect(opt.value)"
        >
          {{ opt.label }}
        </div>
      </div>
    </div>

    <!-- Slash result message -->
    <div v-if="slashResultVisible" class="slash-result" data-testid="slash-result">
      <pre class="slash-result-text">{{ slashResultMessage }}</pre>
    </div>

    <!-- Popover -->
    <div class="mention-popover" v-if="mentionPopover.visible && mentionOptions.length" :style="{ left: mentionPopover.x + 'px', bottom: '100%' }">
      <div class="popover-header">Hosts</div>
      <div class="popover-list">
        <div 
          v-for="(opt, idx) in mentionOptions" 
          :key="opt.id"
          class="popover-item"
          :class="{ active: idx === mentionPopover.selectedIndex }"
          @click="selectMention(opt)"
        >
          <span class="type-badge">{{ opt.type }}</span>
          {{ opt.label }}
        </div>
      </div>
    </div>

    <textarea
      ref="textareaRef"
      :value="modelValue"
      @input="onInput"
      @paste="onPaste"
      @drop.prevent="onDrop"
      @dragover.prevent
      @keydown="onKeydown"
      @focus="onFocus"
      @blur="onBlur"
      rows="1"
      class="omnibar-input"
      :class="{ 'is-compact-stop': compactStopMode }"
      :placeholder="placeholder"
      :disabled="inputDisabled"
      data-testid="omnibar-input"
    ></textarea>
    
    <div class="omnibar-tools" :class="{ 'is-compact-stop': compactStopMode }">
      <div class="tools-left">
         <button
           type="button"
           class="composer-icon-btn"
           data-testid="omnibar-attachment-action"
           aria-label="添加附件或上下文"
           @click="showAttachmentHint"
         >
           +
         </button>
         <span v-for="mention in activeMentions" :key="mention" class="pill-tag"><span class="pill-icon">@</span> {{ mention }}</span>
         <span
           v-for="artifact in artifactPills"
           :key="artifact.id"
           class="pill-tag artifact-pill"
           data-testid="omnibar-artifact-pill"
         >{{ artifact.label }}</span>
         <span
           v-if="showHintText"
           class="hint-text"
           :class="{
             'is-paste-indicator': pasteAssist.indicator.value?.kind === 'buffering' || pasteAssist.indicator.value?.kind === 'ready',
             'is-artifact-indicator': pasteAssist.indicator.value?.kind === 'artifact',
             'is-focus-indicator': pasteAssist.indicator.value?.kind === 'focus' || attachmentHintVisible,
           }"
           :data-testid="hintTestId"
         >{{ hintText }}</span>
         <button
           v-if="pasteAssist.hasPendingArtifact.value"
           type="button"
           class="hint-clear-btn"
           data-testid="omnibar-clear-pending"
           @click="pasteAssist.clearPendingArtifact()"
         >
           清除
         </button>
      </div>
      <div class="tools-right" :class="{ 'is-compact-stop': compactStopMode }">
         <div
           v-if="showExecutionControls"
           class="execution-controls"
           data-testid="omnibar-execution-controls"
           aria-label="执行上下文"
         >
           <button
             type="button"
             class="execution-chip"
             data-testid="omnibar-model-control"
             :aria-label="`模型：${modelLabel}`"
           >
             {{ modelLabel }}
           </button>
         </div>
         <div class="action-group">
           <n-button
             circle
             class="omnibar-action-button"
             :type="primaryAction === 'stop' ? 'error' : 'primary'"
             :size="compactStopMode ? 'small' : 'medium'"
             :disabled="primaryAction === 'stop' ? busy : sendDisabled"
             :aria-label="primaryActionLabel"
             data-testid="omnibar-primary-action"
             @click="primaryAction === 'stop' ? emit('stop') : emitSend()"
           >
             <span v-if="primaryAction === 'stop'">■</span>
             <span v-else-if="store.sending" class="spinner-small"></span>
             <span v-else>↑</span>
           </n-button>
           <button v-if="showSecondaryStop" type="button" class="stop-link-btn" @click="emit('stop')">停止</button>
         </div>
      </div>
    </div>


  </div>
</template>

<style scoped>
.omnibar-wrapper {
  width: 100%;
  max-width: 820px;
  margin: 0 auto;
  background: var(--omnibar-bg);
  border: 1px solid var(--border-color);
  border-radius: 26px;
  padding: 12px 16px 10px;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.06);
  display: flex;
  flex-direction: column;
  gap: 8px;
  transition: box-shadow 0.2s, border-color 0.2s;
  position: relative;
}

.omnibar-wrapper.is-stop-mode {
  background: rgba(241, 245, 249, 0.9);
}

.omnibar-wrapper.is-compact-stop {
  padding: 6px 12px;
  gap: 4px;
  border-radius: 14px;
  box-shadow: 0 1px 4px rgba(15, 23, 42, 0.05);
}

.omnibar-wrapper.is-docked-bottom {
  border-top-left-radius: 0;
  border-top-right-radius: 0;
  border-top-color: transparent;
}

.omnibar-wrapper:focus-within {
  border-color: #cbd5e1;
  box-shadow: 0 16px 36px rgba(15, 23, 42, 0.1);
  background: #ffffff;
}

.omnibar-input {
  width: 100%;
  border: none;
  background: transparent;
  resize: none;
  outline: none;
  min-height: 36px;
  font-size: 14px;
  line-height: 1.6;
  padding: 4px 6px 0;
  color: var(--text-main);
  font-family: inherit;
}

.omnibar-input.is-compact-stop {
  min-height: 24px;
  line-height: 1.45;
  padding: 2px 2px 0;
}

.omnibar-input::placeholder {
  color: #94a3b8;
}

.omnibar-input:disabled {
  color: #64748b;
}

.omnibar-tools {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
}

.omnibar-tools.is-compact-stop {
  align-items: center;
  gap: 8px;
}

.tools-left {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
  flex: 1;
  min-width: 0;
}

.pill-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  background: rgba(15, 23, 42, 0.06);
  color: var(--text-main);
  padding: 5px 10px;
  border-radius: 9999px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.tools-right {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
  margin-left: auto;
}

.tools-right.is-alone {
  width: 100%;
  justify-content: flex-end;
}

.tools-right.is-compact-stop {
  gap: 0;
}

.action-group {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.execution-controls {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #7c8798;
  font-size: 12px;
  line-height: 1;
}

.execution-chip,
.composer-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  border: none;
  background: transparent;
  color: #7c8798;
  font: inherit;
  line-height: 1;
  cursor: pointer;
}

.execution-chip {
  gap: 4px;
  padding: 0 6px;
  border-radius: 999px;
  white-space: nowrap;
}

.execution-chip:hover,
.composer-icon-btn:hover {
  background: rgba(15, 23, 42, 0.05);
  color: #334155;
}

.composer-icon-btn {
  width: 28px;
  border-radius: 999px;
  font-size: 20px;
}

.tools-left > .composer-icon-btn {
  color: #64748b;
  font-size: 22px;
}

.omnibar-wrapper :deep(.omnibar-action-button.n-button) {
  --n-color: #111827 !important;
  --n-color-hover: #1f2937 !important;
  --n-color-pressed: #030712 !important;
  --n-color-focus: #1f2937 !important;
  --n-color-disabled: #111827 !important;
  --n-border: 1px solid #111827 !important;
  --n-border-hover: 1px solid #1f2937 !important;
  --n-border-pressed: 1px solid #030712 !important;
  --n-border-focus: 1px solid #1f2937 !important;
  --n-border-disabled: 1px solid #111827 !important;
  --n-ripple-color: #111827 !important;
}

.tools-right.is-compact-stop .action-group {
  gap: 0;
}

.hint-text {
  font-size: 11.5px;
  color: #94a3b8;
  line-height: 1.4;
  text-align: right;
}

.hint-text.is-paste-indicator {
  color: #0f766e;
}

.hint-text.is-artifact-indicator {
  color: #1d4ed8;
}

.hint-text.is-focus-indicator {
  color: #7c3aed;
}

.artifact-pill {
  background: rgba(59, 130, 246, 0.12);
  color: #1d4ed8;
}

.hint-clear-btn {
  border: none;
  background: transparent;
  color: #64748b;
  font-size: 11px;
  cursor: pointer;
  padding: 0;
}

.hint-clear-btn:hover {
  color: #0f172a;
}

.stop-link-btn {
  border: 1px solid #fecaca;
  background: #fff;
  color: #b91c1c;
  border-radius: 999px;
  padding: 8px 12px;
  font-size: 11.5px;
  font-weight: 600;
  cursor: pointer;
}

.stop-link-btn:hover {
  background: #fef2f2;
}

.pill-icon {
  color: var(--primary);
  font-weight: 700;
}

.mention-popover {
  position: absolute;
  margin-bottom: 12px;
  width: 260px;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.1);
  overflow: hidden;
  z-index: 100;
}

.popover-header {
  padding: 8px 12px;
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  background: #f8fafc;
  border-bottom: 1px solid #f1f5f9;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.popover-list {
  display: flex;
  flex-direction: column;
  padding: 4px;
}

.popover-item {
  padding: 8px 12px;
  font-size: 13px;
  color: #0f172a;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
}

.popover-item:hover, .popover-item.active {
  background: #f1f5f9;
}

.type-badge {
  font-size: 9px;
  padding: 2px 4px;
  border-radius: 4px;
  background: #e2e8f0;
  color: #475569;
  text-transform: uppercase;
}

.spinner-small {
  display: inline-block;
  width: 14px; height: 14px;
  border: 2px solid rgba(255,255,255,0.3);
  border-radius: 50%;
  border-top-color: white;
  animation: spin 1s linear infinite;
}

@keyframes spin { 
  to { transform: rotate(360deg); }
}

/* Slash command popover */
.slash-popover {
  position: absolute;
  bottom: 100%;
  left: 14px;
  right: 14px;
  margin-bottom: 8px;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.1);
  overflow: hidden;
  z-index: 100;
}

/* Slash result */
.slash-result {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 8px 12px;
  margin-bottom: 4px;
}

.slash-result-text {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: #334155;
  white-space: pre-wrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}


</style>
import { submitApprovalDecision } from "../api/approvals";
