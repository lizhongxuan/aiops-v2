<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { RefreshCcwIcon, SquareTerminalIcon, PlayIcon, SquareIcon, RotateCcwIcon } from "lucide-vue-next";
import "@xterm/xterm/css/xterm.css";
import { useAppStore } from "../../store";
import {
  createWorkspaceTerminalSession,
  normalizeWorkspaceTerminalLines,
  normalizeWorkspaceTerminalOutput,
  openWorkspaceTerminalSocket,
} from "../../lib/workspaceTerminal";

const props = defineProps({
  hostId: {
    type: String,
    required: true,
  },
  hostName: {
    type: String,
    default: "",
  },
  title: {
    type: String,
    default: "终端",
  },
  subtitle: {
    type: String,
    default: "",
  },
  output: {
    type: [String, Array, Object],
    default: "",
  },
  allowTakeover: {
    type: Boolean,
    default: true,
  },
  autoTakeover: {
    type: Boolean,
    default: false,
  },
  interactiveSessionId: {
    type: String,
    default: "",
  },
  defaultCwd: {
    type: String,
    default: "~",
  },
  defaultShell: {
    type: String,
    default: "/bin/zsh",
  },
  panelHeight: {
    type: String,
    default: "320px",
  },
  takeoverLabel: {
    type: String,
    default: "接管终端",
  },
  readonlyLabel: {
    type: String,
    default: "只读",
  },
  emptyLabel: {
    type: String,
    default: "暂无终端输出",
  },
});

const emit = defineEmits(["connected", "disconnected", "error", "takeover-start", "takeover-stop"]);

const store = useAppStore();
const terminalContainer = ref(null);
const currentMode = ref("readonly");
const connectionStatus = ref("idle");
const sessionId = ref("");
const startedAt = ref("");
const currentCwd = ref(props.defaultCwd);
const currentShell = ref(props.defaultShell);
const lastError = ref("");

let term = null;
let fitAddon = null;
let resizeObserver = null;
let socket = null;

const hostLabel = computed(() => props.hostName || props.subtitle || props.hostId || "unknown-host");
const isInteractive = computed(() => currentMode.value === "interactive");
const normalizedOutput = computed(() => normalizeWorkspaceTerminalOutput(props.output));
const readonlyLines = computed(() => normalizeWorkspaceTerminalLines(props.output));
const lineCount = computed(() => readonlyLines.value.length);

const statusLabel = computed(() => {
  if (currentMode.value === "interactive") {
    switch (connectionStatus.value) {
      case "connecting":
        return "创建会话中";
      case "connected":
        return "已接管";
      case "reconnecting":
        return "重连中";
      case "disconnected":
        return "会话已断开";
      case "error":
        return lastError.value || "连接失败";
      default:
        return "准备接管";
    }
  }
  return props.readonlyLabel;
});

const statusTone = computed(() => {
  if (connectionStatus.value === "error") return "danger";
  if (connectionStatus.value === "connecting" || connectionStatus.value === "reconnecting") return "warning";
  if (currentMode.value === "interactive" && connectionStatus.value === "connected") return "success";
  return "neutral";
});

const terminalHint = computed(() => {
  if (currentMode.value === "interactive") {
    if (connectionStatus.value === "connected") {
      return `会话 ${sessionId.value || "-"} · ${currentCwd.value} · ${currentShell.value}`;
    }
    if (connectionStatus.value === "error" && lastError.value) {
      return lastError.value;
    }
    return `主机 ${hostLabel.value} 正在建立终端连接。`;
  }
  const lines = lineCount.value;
  if (!lines) {
    return props.emptyLabel;
  }
  return `已捕获 ${lines} 行输出`;
});

function createTerminalInstance({ interactive = false } = {}) {
  disposeTerminal();
  if (!terminalContainer.value) {
    return;
  }

  term = new Terminal({
    theme: {
      background: "#0f172a",
      foreground: "#e2e8f0",
      cursor: interactive ? "#f8fafc" : "transparent",
      selection: "rgba(255, 255, 255, 0.2)",
    },
    fontFamily: '"SF Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace',
    fontSize: 13,
    lineHeight: 1.45,
    cursorBlink: interactive,
    disableStdin: !interactive,
    scrollback: 5000,
    convertEol: true,
    allowTransparency: true,
    rendererType: "canvas",
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);
  fitTerminal();

  if (interactive) {
    term.onData((data) => {
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "input", data }));
      }
    });
  }

  renderTerminalContent();
}

function renderReadonlyContent() {
  if (!term) {
    return;
  }
  term.clear();
  term.writeln(`\u001b[90m${props.readonlyLabel} · ${hostLabel.value}\u001b[0m`);
  term.writeln("");
  if (!normalizedOutput.value) {
    term.writeln(`\u001b[90m${props.emptyLabel}\u001b[0m`);
    return;
  }
  const lines = readonlyLines.value;
  for (const line of lines) {
    term.writeln(line || "");
  }
}

function renderInteractiveBanner() {
  if (!term) {
    return;
  }
  term.writeln("");
  term.writeln("\u001b[90m[终端接管已开启]\u001b[0m");
  if (sessionId.value) {
    term.writeln(`\u001b[90mSession ${sessionId.value}\u001b[0m`);
  }
  term.writeln("");
}

function renderTerminalContent() {
  if (!term) {
    return;
  }
  if (currentMode.value === "readonly") {
    renderReadonlyContent();
    return;
  }
  renderReadonlyContent();
  renderInteractiveBanner();
}

function fitTerminal() {
  if (!fitAddon) {
    return;
  }
  requestAnimationFrame(() => {
    try {
      fitAddon.fit();
      syncResize();
    } catch (_error) {
      // The panel can be temporarily hidden; retry on the next resize.
    }
  });
}

function syncResize() {
  if (!socket || socket.readyState !== WebSocket.OPEN || !term) {
    return;
  }
  socket.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
}

function cleanupSocket() {
  if (socket) {
    try {
      socket.close();
    } catch (_error) {
      // Ignore socket close errors during teardown.
    }
  }
  socket = null;
}

function disposeTerminal() {
  cleanupSocket();
  if (term) {
    term.dispose();
    term = null;
  }
  fitAddon = null;
}

async function connectInteractiveSession() {
  if (!props.allowTakeover) {
    return;
  }
  if (connectionStatus.value === "connecting" || connectionStatus.value === "connected") {
    return;
  }

  currentMode.value = "interactive";
  connectionStatus.value = "connecting";
  lastError.value = "";
  emit("takeover-start", { hostId: props.hostId });

  const hostSelected = await store.selectHost(props.hostId);
  if (!hostSelected) {
    connectionStatus.value = "error";
    lastError.value = store.errorMessage || "无法切换当前主机上下文";
    emit("error", new Error(lastError.value));
    return;
  }

  createTerminalInstance({ interactive: true });

  let terminalSession = null;
  try {
    if (props.interactiveSessionId) {
      terminalSession = {
        sessionId: props.interactiveSessionId,
        cwd: props.defaultCwd,
        shell: props.defaultShell,
        startedAt: "",
      };
    } else {
      terminalSession = await createWorkspaceTerminalSession({
        hostId: props.hostId,
        cwd: props.defaultCwd,
        shell: props.defaultShell,
        cols: term?.cols || 120,
        rows: term?.rows || 36,
      });
    }
  } catch (error) {
    connectionStatus.value = "error";
    lastError.value = error?.message || "无法创建终端会话";
    renderReadonlyContent();
    emit("error", error instanceof Error ? error : new Error(lastError.value));
    return;
  }

  sessionId.value = terminalSession.sessionId || terminalSession.sessionId || "";
  currentCwd.value = terminalSession.cwd || props.defaultCwd;
  currentShell.value = terminalSession.shell || props.defaultShell;
  startedAt.value = terminalSession.startedAt || "";

  cleanupSocket();
  socket = openWorkspaceTerminalSocket(sessionId.value, {
    onOpen: () => {
      connectionStatus.value = "connected";
      emit("connected", {
        hostId: props.hostId,
        sessionId: sessionId.value,
      });
      term?.focus();
    },
    onReady: (message) => {
      connectionStatus.value = "connected";
      currentCwd.value = message.cwd || currentCwd.value;
      currentShell.value = message.shell || currentShell.value;
      startedAt.value = message.startedAt || startedAt.value;
    },
    onOutput: (message) => {
      term?.write(message.data || "");
    },
    onExit: (message) => {
      term?.writeln(`\r\n\u001b[90m[进程已退出，退出码: ${message.code}]\u001b[0m`);
      connectionStatus.value = "disconnected";
      emit("disconnected", {
        hostId: props.hostId,
        sessionId: sessionId.value,
        code: message.code,
      });
    },
    onStatus: (message) => {
      connectionStatus.value = message.status || connectionStatus.value;
    },
    onError: (message) => {
      const text = message.message || "终端连接失败";
      lastError.value = text;
      connectionStatus.value = "error";
      term?.writeln(`\r\n\u001b[31m${text}\u001b[0m`);
      emit("error", new Error(text));
    },
    onClose: () => {
      if (connectionStatus.value === "connected" || connectionStatus.value === "connecting" || connectionStatus.value === "reconnecting") {
        connectionStatus.value = "disconnected";
        emit("disconnected", {
          hostId: props.hostId,
          sessionId: sessionId.value,
        });
      }
    },
    onSocketError: () => {
      connectionStatus.value = "error";
      lastError.value = "终端 websocket 连接错误";
      emit("error", new Error(lastError.value));
    },
  });
}

function stopInteractiveSession() {
  if (currentMode.value !== "interactive") {
    return;
  }
  cleanupSocket();
  currentMode.value = "readonly";
  connectionStatus.value = "idle";
  sessionId.value = "";
  startedAt.value = "";
  currentCwd.value = props.defaultCwd;
  currentShell.value = props.defaultShell;
  lastError.value = "";
  createTerminalInstance({ interactive: false });
  emit("takeover-stop", { hostId: props.hostId });
}

function reconnectInteractiveSession() {
  cleanupSocket();
  connectionStatus.value = "reconnecting";
  void connectInteractiveSession();
}

function refreshReadonlyTerminal() {
  if (currentMode.value !== "readonly") {
    return;
  }
  renderTerminalContent();
  fitTerminal();
}

defineExpose({
  takeover: connectInteractiveSession,
  stopTakeover: stopInteractiveSession,
  reconnect: reconnectInteractiveSession,
  refreshReadonlyTerminal,
});

watch(
  () => [props.output, props.hostId, props.hostName, props.subtitle, props.emptyLabel, props.readonlyLabel],
  () => {
    if (currentMode.value === "readonly") {
      nextTick(() => {
        renderTerminalContent();
        fitTerminal();
      });
    }
  },
  { deep: true },
);

watch(
  () => props.interactiveSessionId,
  (nextSessionId) => {
    if (currentMode.value !== "interactive" || !nextSessionId) {
      return;
    }
    reconnectInteractiveSession();
  },
);

onMounted(async () => {
  await nextTick();
  createTerminalInstance({ interactive: false });
  resizeObserver = new ResizeObserver(() => {
    fitTerminal();
  });
  if (terminalContainer.value) {
    resizeObserver.observe(terminalContainer.value);
  }
  if (props.autoTakeover && props.allowTakeover) {
    void connectInteractiveSession();
  }
});

onBeforeUnmount(() => {
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  disposeTerminal();
});
</script>

<template>
  <section class="workspace-terminal-panel">
    <header class="terminal-head">
      <div class="terminal-head-copy">
        <div class="terminal-kicker">Workspace Terminal</div>
        <h3>{{ title }}</h3>
        <p>{{ subtitle || hostLabel }}</p>
      </div>

      <div class="terminal-head-actions">
        <span class="terminal-status" :class="statusTone">{{ statusLabel }}</span>
        <button v-if="allowTakeover && !isInteractive" class="terminal-btn primary" @click="connectInteractiveSession">
          <PlayIcon size="14" />
          <span>{{ takeoverLabel }}</span>
        </button>
        <button v-if="isInteractive" class="terminal-btn ghost" @click="reconnectInteractiveSession">
          <RotateCcwIcon size="14" />
          <span>重连</span>
        </button>
        <button v-if="isInteractive" class="terminal-btn ghost" @click="stopInteractiveSession">
          <SquareIcon size="14" />
          <span>停止接管</span>
        </button>
        <button class="terminal-btn ghost" @click="refreshReadonlyTerminal">
          <RefreshCcwIcon size="14" />
          <span>刷新输出</span>
        </button>
      </div>
    </header>

    <div class="terminal-meta">
      <span>{{ terminalHint }}</span>
      <span v-if="lineCount && !isInteractive">{{ lineCount }} 行</span>
      <span v-if="startedAt">{{ startedAt }}</span>
    </div>

    <div ref="terminalContainer" class="terminal-container"></div>
  </section>
</template>

<style scoped>
.workspace-terminal-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-radius: 20px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.12), transparent 24%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.98), rgba(15, 23, 42, 0.94));
  overflow: hidden;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.18);
}

.terminal-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 16px 18px 12px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}

.terminal-head-copy {
  min-width: 0;
}

.terminal-kicker {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #93c5fd;
}

.terminal-head-copy h3 {
  margin: 8px 0 6px;
  font-size: 18px;
  color: #f8fafc;
}

.terminal-head-copy p {
  margin: 0;
  color: rgba(226, 232, 240, 0.76);
  line-height: 1.6;
}

.terminal-head-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.terminal-status {
  display: inline-flex;
  align-items: center;
  min-height: 34px;
  padding: 0 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  background: rgba(255, 255, 255, 0.08);
  color: #e2e8f0;
}

.terminal-status.success {
  background: rgba(34, 197, 94, 0.14);
  color: #86efac;
}

.terminal-status.warning {
  background: rgba(245, 158, 11, 0.14);
  color: #fde68a;
}

.terminal-status.danger {
  background: rgba(239, 68, 68, 0.16);
  color: #fca5a5;
}

.terminal-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 34px;
  border-radius: 12px;
  padding: 0 12px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.04);
  color: #e2e8f0;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.terminal-btn.primary {
  background: #2563eb;
  border-color: #2563eb;
  color: white;
}

.terminal-btn.ghost:hover,
.terminal-btn.primary:hover {
  filter: brightness(1.05);
}

.terminal-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  padding: 0 18px 14px;
  color: rgba(226, 232, 240, 0.72);
  font-size: 12px;
}

.terminal-meta span + span::before {
  content: "·";
  margin-right: 10px;
  color: rgba(148, 163, 184, 0.72);
}

.terminal-container {
  flex: 1;
  min-height: 0;
  height: v-bind(panelHeight);
  padding: 0 12px 12px;
}

:deep(.terminal-container .xterm) {
  height: 100%;
}

:deep(.terminal-container .xterm-viewport) {
  overflow-y: auto !important;
}

@media (max-width: 900px) {
  .terminal-head {
    flex-direction: column;
  }

  .terminal-head-actions {
    justify-content: flex-start;
  }

  .terminal-container {
    height: min(48vh, v-bind(panelHeight));
  }
}
</style>
