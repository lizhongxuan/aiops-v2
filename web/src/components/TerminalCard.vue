<script setup>
import { ref, onMounted, onBeforeUnmount, watch, computed } from "vue";
import { useRouter } from "vue-router";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { ChevronRightIcon, ChevronDownIcon, TerminalIcon, CheckIcon, XIcon, ExternalLinkIcon } from "lucide-vue-next";
import "@xterm/xterm/css/xterm.css";
import { useAppStore } from "../store";
import { resolveHostBadge } from "../lib/hostDisplay";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const router = useRouter();
const store = useAppStore();
const isExpanded = ref(true);
const showCommandDetails = ref(false);
const terminalContainer = ref(null);
let term = null;
let fitAddon = null;

const isComplete = computed(() => {
  return (
    props.card.status === "completed" ||
    props.card.status === "error" ||
    props.card.status === "failed" ||
    props.card.status === "permission_denied" ||
    props.card.status === "disconnected" ||
    props.card.status === "host_timeout" ||
    props.card.status === "timeout" ||
    props.card.status === "cancelled"
  );
});

const isSuccess = computed(() => props.card.status === "completed");
const isFailed = computed(() => props.card.status === "error" || props.card.status === "failed");
const isPermissionDenied = computed(() => props.card.status === "permission_denied");
const isDisconnected = computed(() => props.card.status === "disconnected");
const isHostTimeout = computed(() => props.card.status === "host_timeout");
const isTimeout = computed(() => props.card.status === "timeout");
const isCancelled = computed(() => props.card.status === "cancelled");

const hasOutput = computed(() => !!props.card.output);
const isRunning = computed(() => !isComplete.value);
const summaryText = computed(() => (props.card.summary || "").trim());
const summaryHighlights = computed(() => props.card.highlights || []);
const summaryMetaRows = computed(() => props.card.kvRows || []);
const failureTypeLabel = computed(() => {
  if (isPermissionDenied.value) return "OS 权限不足";
  if (isDisconnected.value) return "Agent 断连";
  if (isHostTimeout.value) return "网络 / 心跳超时";
  if (isTimeout.value) return "命令执行超时";
  if (isCancelled.value) return "任务已停止";
  if (isFailed.value) return "命令退出失败";
  return "";
});
const failureHint = computed(() => {
  if (isPermissionDenied.value) return "建议切换更高权限用户，或先改成只读检查确认范围。";
  if (isDisconnected.value) return "建议刷新主机状态，待 host-agent 恢复在线后重试。";
  if (isHostTimeout.value) return "建议确认远程主机心跳恢复，再重新执行。";
  if (isTimeout.value) return "建议缩小命令范围，或在确认风险后适当延长 timeout。";
  if (isCancelled.value) return "这是一次主动中断，不代表命令执行成功。";
  if (isFailed.value) return "建议查看 stderr 和退出码，修正命令后再试。";
  return "";
});
const hasPresentation = computed(() => {
  return (
    !!summaryText.value ||
    summaryHighlights.value.length > 0 ||
    summaryMetaRows.value.length > 0 ||
    !!failureTypeLabel.value ||
    !!failureHint.value
  );
});
const showExitCode = computed(() => typeof props.card.exitCode === "number" && isComplete.value);
const canOpenTerminal = computed(() => {
  return (
    store.selectedHost?.status === "online" &&
    (store.selectedHost?.terminalCapable || store.selectedHost?.executable)
  );
});

const dynamicHeight = computed(() => {
  if (!props.card.output) return 60;
  const lines = props.card.output.split('\n').length;
  const h = Math.max(60, Math.min(250, lines * 20)); // approx 20px per line
  return h;
});

const commandDescriptor = computed(() => describeCommand(props.card.command || ""));
const hostCaption = computed(() => {
  const host = store.snapshot.hosts.find((item) => item.id === props.card.hostId) || {};
  return resolveHostBadge({
    ...host,
    hostId: props.card.hostId,
    hostName: props.card.hostName,
  });
});
const rawCommand = computed(() => (props.card.command || "").trim());
const hasFoldedCommand = computed(() => {
  return rawCommand.value.includes("\n") || rawCommand.value.includes("<<") || rawCommand.value.length > 140;
});
const commandDetailsLabel = computed(() => (showCommandDetails.value ? "收起脚本" : "展开脚本"));
const commandDetailsMeta = computed(() => {
  if (!hasFoldedCommand.value) return "";
  const lineCount = rawCommand.value ? rawCommand.value.split("\n").length : 0;
  const kind = commandDescriptor.value.kind || "Shell";
  if (lineCount > 1) {
    return `${kind} 脚本已折叠 · ${lineCount} 行 · ${rawCommand.value.length} 字符`;
  }
  return `${kind} 命令已折叠 · ${rawCommand.value.length} 字符`;
});

function stripMatchingQuotes(value) {
  if (!value || value.length < 2) return value;
  const first = value[0];
  const last = value[value.length - 1];
  if ((first === "'" && last === "'") || (first === '"' && last === '"')) {
    return value.slice(1, -1);
  }
  return value;
}

function shellKindLabel(shellName) {
  switch ((shellName || "").toLowerCase()) {
    case "zsh":
      return "Zsh";
    case "bash":
      return "Bash";
    case "sh":
      return "Sh";
    default:
      return "Shell";
  }
}

function normalizeDisplayCommand(value) {
  return value.replace(/\s+/g, " ").trim();
}

function truncateDisplayCommand(value, max = 108) {
  if (!value || value.length <= max) return value;
  return `${value.slice(0, max - 3)}...`;
}

function describeCommand(command) {
  const raw = (command || "").trim();
  if (!raw) {
    return { kind: "", display: "Executing..." };
  }

  let kind = "";
  let display = raw;

  const shellWrapper = raw.match(/^(?:\/bin\/)?(zsh|bash|sh)\s+-lc\s+([\s\S]+)$/i);
  if (shellWrapper) {
    kind = shellKindLabel(shellWrapper[1]);
    display = normalizeDisplayCommand(stripMatchingQuotes(shellWrapper[2].trim()));
  }

  const lowerDisplay = display.toLowerCase();
  if (/^(python|python3)\b/.test(lowerDisplay)) {
    kind = "Python";
    if (display.includes("<<")) {
      const binary = lowerDisplay.startsWith("python3") ? "python3" : "python";
      return { kind, display: `${binary} heredoc` };
    }
  } else if (/^(node|nodejs)\b/.test(lowerDisplay)) {
    kind = "Node";
  } else if (!kind) {
    kind = "Shell";
  }

  return {
    kind,
    display: truncateDisplayCommand(display),
  };
}

/* Human-readable duration */
const durationLabel = computed(() => {
  const ms = props.card.durationMs;
  if (!ms) return "";
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts = [];
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds}s`);
  return parts.join(" ");
});

function toggleExpand() {
  if (!hasOutput.value && isSuccess.value) {
    return;
  }
  isExpanded.value = !isExpanded.value;
  if (isExpanded.value) {
    setTimeout(() => {
      if (!term && hasOutput.value) {
        initTerminal();
        return;
      }
      fitAddon?.fit();
    }, 10);
  }
}

function openInTerminal(event) {
  event.stopPropagation();
  if (!canOpenTerminal.value) return;
  router.push(`/terminal/${store.snapshot.selectedHostId}`);
}

function toggleCommandDetails(event) {
  event.stopPropagation();
  showCommandDetails.value = !showCommandDetails.value;
}

function initTerminal() {
  if (!terminalContainer.value || !hasOutput.value) return;

  disposeTerminal();

  term = new Terminal({
    theme: {
      background: "#0f172a",
      foreground: "#f8fafc",
      cursor: "transparent",
      selection: "rgba(255, 255, 255, 0.3)",
    },
    fontFamily: '"SF Mono", "Fira Code", monospace',
    fontSize: 13,
    lineHeight: 1.4,
    cursorBlink: false,
    disableStdin: true,
    padding: 12,
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);
  fitAddon.fit();

  const formattedOutput = props.card.output
    .replace(/\n?\[?exit code: 0\]?\n?$/i, "")
    .replace(/\n/g, "\r\n");
  term.write(formattedOutput);
}

function disposeTerminal() {
  if (term) {
    term.dispose();
    term = null;
    fitAddon = null;
  }
}

watch(
  () => props.card.status,
  (status) => {
    if (status === "completed") {
      isExpanded.value = false;
      return;
    }
    if (
      status === "failed" ||
      status === "error" ||
      status === "permission_denied" ||
      status === "disconnected" ||
      status === "host_timeout" ||
      status === "timeout" ||
      status === "cancelled"
    ) {
      isExpanded.value = true;
    }
  }
);

watch(
  () => isExpanded.value,
  (expanded) => {
    if (!expanded) {
      disposeTerminal();
      return;
    }
    if (!term) {
      setTimeout(initTerminal, 10);
    }
  }
);

watch(
  () => props.card.output,
  (newOutput) => {
    if (newOutput && isExpanded.value && !term) {
      setTimeout(initTerminal, 10);
      return;
    }
    if (term) {
      term.clear();
      const cleanOutput = newOutput ? newOutput.replace(/\n?\[?exit code: 0\]?\n?$/i, "").replace(/\n/g, "\r\n") : "";
      term.write(cleanOutput);
      fitAddon?.fit();
    }
  }
);

onMounted(() => {
  if (isSuccess.value) {
    isExpanded.value = false;
  } else if (
    isFailed.value ||
    isPermissionDenied.value ||
    isDisconnected.value ||
    isHostTimeout.value ||
    isTimeout.value ||
    isCancelled.value
  ) {
    isExpanded.value = true;
    setTimeout(initTerminal, 10);
  } else {
    setTimeout(initTerminal, 10);
  }
});

onBeforeUnmount(() => {
  disposeTerminal();
});
</script>

<template>
  <div
    v-if="isSuccess && !isExpanded"
    class="timeline-summary"
    :class="{ clickable: hasOutput }"
    :title="hasOutput ? '点击查看输出' : ''"
    @click="toggleExpand"
    @keydown.enter.prevent="toggleExpand"
    @keydown.space.prevent="toggleExpand"
    :tabindex="hasOutput ? 0 : -1"
    :role="hasOutput ? 'button' : undefined"
  >
    <div class="timeline-left">
      <CheckIcon size="14" class="timeline-check" />
      <span v-if="hostCaption" class="term-host-badge">{{ hostCaption }}</span>
      <span v-if="commandDescriptor.kind" class="command-kind-badge">{{ commandDescriptor.kind }}</span>
      <div class="timeline-copy">
        <span class="timeline-label">
          已运行 <code :title="card.command">{{ commandDescriptor.display }}</code>
        </span>
        <span v-if="summaryText" class="timeline-summary-copy">{{ summaryText }}</span>
      </div>
    </div>
    <div class="timeline-divider"></div>
    <span class="timeline-duration" v-if="durationLabel">已处理 {{ durationLabel }}</span>
  </div>

  <!-- Full terminal card (running / failed / manually expanded) -->
  <div v-else class="terminal-card" :class="{'minimal': isComplete && !isExpanded}">
    <div class="term-header" @click="toggleExpand">
      <div class="term-title-group">
        <component :is="isExpanded ? ChevronDownIcon : ChevronRightIcon" size="16" class="icon-carat" />
        <TerminalIcon size="14" class="icon-term" />
        <span v-if="hostCaption" class="term-host-badge">{{ hostCaption }}</span>
        <span v-if="commandDescriptor.kind" class="command-kind-badge subtle">{{ commandDescriptor.kind }}</span>
        <span class="term-command mono" :title="card.command">{{ commandDescriptor.display }}</span>
      </div>

      <div class="term-meta">
        <button
          v-if="canOpenTerminal"
          type="button"
          class="term-open-btn"
          @click="openInTerminal"
        >
          <ExternalLinkIcon size="12" />
          <span>在终端中打开</span>
        </button>
        <span class="term-cwd" v-if="card.cwd">{{ card.cwd }}</span>
        <span class="term-status-badge success" v-if="isSuccess">
          <CheckIcon size="12" /> 成功
        </span>
        <span class="term-status-badge neutral" v-if="showExitCode">
          退出码 {{ card.exitCode }}
        </span>
        <span class="term-status-badge error" v-if="isFailed">
          <XIcon size="12" /> 失败
        </span>
        <span class="term-status-badge permission" v-if="isPermissionDenied">
          <XIcon size="12" /> 权限不足
        </span>
        <span class="term-status-badge warning" v-if="isDisconnected">
          <XIcon size="12" /> 主机断连
        </span>
        <span class="term-status-badge warning" v-if="isHostTimeout">
          <XIcon size="12" /> 心跳超时
        </span>
        <span class="term-status-badge timeout" v-if="isTimeout">
          <XIcon size="12" /> 执行超时
        </span>
        <span class="term-status-badge cancelled" v-if="isCancelled">
          <XIcon size="12" /> 已取消
        </span>
      </div>
    </div>

    <div v-if="hasPresentation || hasFoldedCommand" class="term-summary-panel">
      <div v-if="hasFoldedCommand" class="term-command-panel">
        <div class="term-command-panel-head">
          <span class="term-command-caption">{{ commandDetailsMeta }}</span>
          <button type="button" class="term-command-toggle" @click="toggleCommandDetails">
            {{ commandDetailsLabel }}
          </button>
        </div>
        <pre v-if="showCommandDetails" class="term-command-source">{{ rawCommand }}</pre>
      </div>
      <p v-if="failureTypeLabel" class="term-error-copy">{{ failureTypeLabel }}</p>
      <p v-if="summaryText" class="term-summary-copy">{{ summaryText }}</p>
      <p v-if="failureHint" class="term-error-hint">{{ failureHint }}</p>
      <div v-if="summaryMetaRows.length" class="term-meta-grid">
        <span v-for="row in summaryMetaRows" :key="`${row.key}-${row.value}`" class="term-meta-chip">
          <strong>{{ row.key }}</strong>
          <span>{{ row.value }}</span>
        </span>
      </div>
      <div v-if="summaryHighlights.length" class="term-highlight-list">
        <div v-for="line in summaryHighlights" :key="line" class="term-highlight-line">
          {{ line }}
        </div>
      </div>
    </div>

    <div class="term-body" v-if="isExpanded">
      <div v-if="hasOutput" class="xterm-wrapper" ref="terminalContainer" :style="{ height: dynamicHeight + 'px' }"></div>
      <div v-else class="terminal-placeholder">
        <div class="terminal-placeholder-label">{{ isRunning ? "命令执行中，等待输出..." : "该命令没有输出" }}</div>
        <div class="terminal-placeholder-command mono">{{ hasFoldedCommand ? commandDescriptor.display : (card.command || "Executing...") }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ====== Timeline summary line (collapsed success) ====== */
.timeline-summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 10px;
  padding: 4px 0;
  margin-left: 36px;
  max-width: 760px;
}

.timeline-summary.clickable {
  cursor: pointer;
}

.timeline-summary.clickable:hover .timeline-label,
.timeline-summary.clickable:focus-visible .timeline-label {
  color: #334155;
}

.timeline-summary.clickable:focus-visible {
  outline: none;
}

.timeline-left {
  display: flex;
  align-items: flex-start;
  gap: 5px;
  color: var(--text-meta, #9ca3af);
  font-size: var(--text-meta-size, 11px);
  min-width: 0;
  white-space: normal;
  flex-wrap: wrap;
}

.timeline-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.timeline-check {
  color: #22c55e;
}

.timeline-failed {
  color: #ef4444;
}

.timeline-label {
  color: #6b7280;
  min-width: 0;
}

.timeline-label code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  color: #374151;
  font-weight: 500;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.timeline-summary-copy {
  color: #475569;
  line-height: 1.4;
}

.timeline-divider {
  flex: 1;
  height: 1px;
  background: #e5e7eb;
  min-width: 32px;
}

.timeline-duration {
  font-size: var(--text-meta-size, 11px);
  color: var(--text-meta, #9ca3af);
  white-space: nowrap;
}

/* ====== Full terminal card ====== */
.terminal-card {
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid var(--border-card, #e5e7eb);
  overflow: hidden;
  margin-top: 2px;
  margin-left: 36px;
  max-width: 760px;
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.02);
  transition: all 0.2s;
}

.terminal-card.minimal {
  background: #f8fafc;
  box-shadow: none;
}

.terminal-card.minimal:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.term-header {
  padding: 8px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  user-select: none;
}

.term-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  overflow: hidden;
  min-width: 0;
}

.icon-carat {
  color: #94a3b8;
}

.icon-term {
  color: #64748b;
}

.term-command {
  font-size: 12.5px;
  color: #0f172a;
  white-space: normal;
  overflow: hidden;
  font-weight: 500;
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.terminal-card.minimal .term-command {
  color: #64748b;
}

.term-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: 12px;
  flex-shrink: 0;
}

.command-kind-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  border: 1px solid #bfdbfe;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
  flex-shrink: 0;
}

.term-host-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: #f8fafc;
  color: #475569;
  border: 1px solid #dbe3ee;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
  flex-shrink: 0;
}

.command-kind-badge.subtle {
  background: #f8fafc;
  color: #475569;
  border-color: #dbe3ee;
}

.term-open-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid #dbe3ee;
  background: #ffffff;
  color: #475569;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}

.term-open-btn:hover {
  background: #f8fafc;
  border-color: #cbd5e1;
}

.term-cwd {
  font-size: 11px;
  color: #94a3b8;
  max-width: 150px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.term-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 12px;
}

.term-status-badge.success {
  background: #dcfce7;
  color: #166534;
}

.term-status-badge.neutral {
  background: #f1f5f9;
  color: #334155;
}

.term-status-badge.error {
  background: #fee2e2;
  color: #991b1b;
}

.term-status-badge.permission {
  background: #fff7ed;
  color: #c2410c;
}

.term-status-badge.warning {
  background: #fff7ed;
  color: #c2410c;
}

.term-status-badge.timeout {
  background: #fef3c7;
  color: #92400e;
}

.term-status-badge.cancelled {
  background: #e2e8f0;
  color: #475569;
}

.term-body {
  background: #0f172a;
  padding: 6px;
  border-top: 1px solid #1e293b;
}

.term-summary-panel {
  padding: 12px;
  border-top: 1px solid #e5e7eb;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.term-command-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #dbe3ee;
}

.term-command-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.term-command-caption {
  font-size: 11px;
  font-weight: 700;
  color: #475569;
}

.term-command-toggle {
  border: 1px solid #dbe3ee;
  background: #f8fafc;
  color: #334155;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.term-command-toggle:hover {
  background: #eef2ff;
  border-color: #c7d2fe;
}

.term-command-source {
  margin: 0;
  padding: 10px 12px;
  border-radius: 10px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.term-summary-copy {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #0f172a;
}

.term-error-copy {
  margin: 0;
  font-size: 12px;
  font-weight: 700;
  color: #b91c1c;
}

.term-error-hint {
  margin: -4px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: #92400e;
}

.term-meta-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.term-meta-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background: #ffffff;
  border: 1px solid #dbe3ee;
  color: #334155;
  font-size: 11px;
}

.term-highlight-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.term-highlight-line {
  font-size: 12px;
  line-height: 1.45;
  color: #475569;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 8px 10px;
}

.xterm-wrapper {
  width: 100%;
  border-radius: 6px;
  overflow: hidden;
  max-height: 250px;
}

.terminal-placeholder {
  min-height: 72px;
  border-radius: 8px;
  background: #111827;
  color: #cbd5e1;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.terminal-placeholder-label {
  font-size: 12px;
  color: #94a3b8;
}

.terminal-placeholder-command {
  font-size: 13px;
  color: #f8fafc;
  line-height: 1.5;
  word-break: break-all;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

:deep(.xterm-viewport) {
  overflow-y: auto !important;
}

@media (max-width: 900px) {
  .timeline-summary {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }

  .timeline-divider {
    display: none;
  }

  .term-header {
    align-items: flex-start;
    gap: 10px;
  }

  .term-meta {
    margin-left: 0;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .term-command-panel-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
