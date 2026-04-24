<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import { useRouter } from "vue-router";
import { useAppStore } from "../store";
import { createTerminalSession } from "../api/terminal";
import { createTerminalSocketClient } from "../realtime/terminalSocket";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
  ArrowLeftIcon,
  TerminalIcon,
  RotateCcwIcon,
  Trash2Icon,
  XIcon,
  InfoIcon,
} from "lucide-vue-next";

const props = defineProps({
  hostId: {
    type: String,
    required: true,
  },
});

const router = useRouter();
const store = useAppStore();

const termContainer = ref(null);
const isDrawerOpen = ref(false);
let term = null;
let fitAddon = null;
let terminalSocket = null;
let resizeObserver = null;
let sessionRequestVersion = 0;
let hostFlowVersion = 0;
let pendingHostSelectionCount = 0;
let isAlive = true;
const sessionId = ref("");
const connectionStatus = ref("connecting"); // connecting | connected | reconnecting | disconnected | error
const currentCwd = ref("~");
const currentShell = ref("/bin/zsh");
const startedAt = ref(null);

const hostInfo = computed(() => {
  return (
    store.snapshot.hosts.find((h) => h.id === props.hostId) || {
      id: props.hostId,
      name: props.hostId,
      status: "offline",
    }
  );
});

const isOnline = computed(() => hostInfo.value.status === "online");
const statusText = computed(() => {
  switch (connectionStatus.value) {
    case "connected":
      return "已连接";
    case "connecting":
      return "连接中";
    case "reconnecting":
      return "重连中";
    case "disconnected":
      return "已断开";
    case "error":
      return "异常";
    default:
      return connectionStatus.value;
  }
});
const connectionHint = computed(() => {
  if (!isOnline.value) {
    return `主机 ${hostInfo.value.name}（${hostInfo.value.id}）当前离线，终端不会回退到 server-local。`;
  }
  if (connectionStatus.value === "error") {
    return "终端连接失败，请检查 host-agent 在线状态、心跳和 shell 环境后重试。";
  }
  if (connectionStatus.value === "disconnected") {
    return "终端会话已结束或远程连接已断开，可重新建立会话。";
  }
  if (connectionStatus.value === "connecting" || connectionStatus.value === "reconnecting") {
    return "正在与当前选中的远程主机建立会话。";
  }
  return "";
});
const canReconnect = computed(() => isOnline.value);

function isStaleHostFlow(targetHostId, flowVersion) {
  return flowVersion !== hostFlowVersion || targetHostId !== props.hostId;
}

function currentHostStatus(hostId) {
  return store.snapshot.hosts.find((host) => host.id === hostId)?.status || "offline";
}

async function ensureSelectedHost(targetHostId, flowVersion) {
  pendingHostSelectionCount += 1;
  const ok = await store.selectHost(targetHostId, {
    shouldApply: () => isAlive && !isStaleHostFlow(targetHostId, flowVersion),
  });
  pendingHostSelectionCount -= 1;
  if (!isAlive) {
    return false;
  }
  if (isStaleHostFlow(targetHostId, flowVersion)) {
    return false;
  }
  if (!ok) {
    connectionStatus.value = "error";
    term?.write("\r\n\x1b[31m无法切换当前主机上下文\x1b[0m\r\n");
  }
  return ok;
}

function goBack() {
  router.push("/");
}

function toggleDrawer() {
  isDrawerOpen.value = !isDrawerOpen.value;
}

async function createSession(targetHostId, flowVersion) {
  connectionStatus.value = "connecting";
  const requestVersion = ++sessionRequestVersion;
  try {
    const data = await createTerminalSession({
      hostId: targetHostId,
      cwd: "~",
      shell: "/bin/zsh",
      cols: term ? term.cols : 120,
      rows: term ? term.rows : 36,
    });
    if (requestVersion !== sessionRequestVersion || isStaleHostFlow(targetHostId, flowVersion)) {
      return;
    }
    sessionId.value = data.sessionId;
    currentCwd.value = data.cwd || currentCwd.value;
    currentShell.value = data.shell || currentShell.value;
    startedAt.value = data.startedAt || new Date().toISOString();
    connectTerminalWs(data.sessionId);
  } catch (e) {
    if (requestVersion !== sessionRequestVersion || isStaleHostFlow(targetHostId, flowVersion)) {
      return;
    }
    connectionStatus.value = "error";
    term?.write("\r\n\x1b[31m网络错误\x1b[0m\r\n");
  }
}

function connectTerminalWs(sid) {
  terminalSocket?.disconnect();
  terminalSocket = createTerminalSocketClient({
    onOpen: () => {
      connectionStatus.value = "connected";
      term?.focus();
    },
    onReady: (msg) => {
      connectionStatus.value = "connected";
      currentCwd.value = msg.cwd || currentCwd.value;
      currentShell.value = msg.shell || currentShell.value;
      startedAt.value = msg.startedAt || startedAt.value;
    },
    onOutput: (msg) => {
      term?.write(msg.data);
    },
    onExit: (msg) => {
      term?.write(`\r\n\x1b[90m[进程已退出，退出码: ${msg.code}]\x1b[0m\r\n`);
      connectionStatus.value = "disconnected";
    },
    onStatus: (msg) => {
      connectionStatus.value = msg.status;
    },
    onErrorMessage: (msg) => {
      term?.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`);
      connectionStatus.value = "error";
    },
    onRawMessage: (data) => {
      term?.write(data);
    },
    onClose: () => {
      connectionStatus.value = "disconnected";
    },
    onSocketError: () => {
      connectionStatus.value = "error";
    },
  });
  terminalSocket.connect(sid);
}

function sendInput(data) {
  terminalSocket?.sendInput(data);
}

function sendSignal(signal) {
  terminalSocket?.sendSignal(signal);
}

function sendResize() {
  if (term) {
    terminalSocket?.sendResize({ cols: term.cols, rows: term.rows });
  }
}

function clearScreen() {
  term?.clear();
}

function reconnect() {
  if (!canReconnect.value) {
    term?.write("\r\n\x1b[33m主机离线，无法重连到远程终端\x1b[0m\r\n");
    connectionStatus.value = "error";
    return;
  }
  terminalSocket?.disconnect();
  const flowVersion = ++hostFlowVersion;
  createSession(props.hostId, flowVersion);
}

async function startHostFlow(targetHostId) {
  const flowVersion = ++hostFlowVersion;
  sessionRequestVersion += 1;
  connectionStatus.value = "connecting";
  if (!(await ensureSelectedHost(targetHostId, flowVersion))) {
    return;
  }
  if (isStaleHostFlow(targetHostId, flowVersion)) {
    return;
  }
  if (currentHostStatus(targetHostId) === "online") {
    await createSession(targetHostId, flowVersion);
    return;
  }
  connectionStatus.value = "error";
  term?.write("\r\n\x1b[33m主机当前离线，无法建立终端连接\x1b[0m\r\n");
}

function initTerminal() {
  if (!termContainer.value) return;
  resizeObserver?.disconnect();
  resizeObserver = null;

  term = new Terminal({
    theme: {
      background: "#0f172a",
      foreground: "#f8fafc",
      cursor: "#f8fafc",
      cursorAccent: "#0f172a",
      selection: "rgba(255, 255, 255, 0.2)",
    },
    fontFamily: '"SF Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace',
    fontSize: 14,
    lineHeight: 1.4,
    cursorBlink: true,
    padding: 16,
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(termContainer.value);
  fitAddon.fit();

  // Forward user input to WS
  term.onData((data) => {
    sendInput(data);
  });

  // Handle window resize
  resizeObserver = new ResizeObserver(() => {
    fitAddon?.fit();
    sendResize();
  });
  resizeObserver.observe(termContainer.value);

  term.write("正在连接到 " + props.hostId + " ...\r\n");
}

onMounted(() => {
  nextTick(async () => {
    initTerminal();
    await startHostFlow(props.hostId);
  });
});

onBeforeUnmount(() => {
  isAlive = false;
  hostFlowVersion += 1;
  sessionRequestVersion += 1;
  terminalSocket?.disconnect();
  resizeObserver?.disconnect();
  resizeObserver = null;
  if (term) {
    term.dispose();
  }
});

watch(
  () => store.snapshot.selectedHostId,
  (selectedHostId) => {
    if (pendingHostSelectionCount > 0 && selectedHostId !== props.hostId) {
      return;
    }
    if (!selectedHostId || selectedHostId === props.hostId) return;
    router.replace(`/terminal/${selectedHostId}`);
  }
);

watch(
  () => props.hostId,
  async (nextHostId, previousHostId) => {
    if (!nextHostId || nextHostId === previousHostId) return;
    sessionRequestVersion += 1;
    hostFlowVersion += 1;
    terminalSocket?.disconnect();
    terminalSocket = null;
    sessionId.value = "";
    connectionStatus.value = "connecting";
    term?.clear();
    term?.write("正在连接到 " + nextHostId + " ...\r\n");
    await startHostFlow(nextHostId);
  }
);
</script>

<template>
  <div class="terminal-page">
    <!-- Top bar -->
    <header class="term-topbar">
      <div class="topbar-left">
        <button class="topbar-btn" @click="goBack" title="返回聊天">
          <ArrowLeftIcon size="16" />
        </button>
        <TerminalIcon size="16" class="topbar-icon" />
        <div class="topbar-host-meta">
          <span class="topbar-host">{{ hostInfo.name }}</span>
          <span class="topbar-host-id">ID {{ hostInfo.id }}</span>
        </div>
        <span
          class="topbar-status-dot"
          :class="{
            online: connectionStatus === 'connected',
            connecting: connectionStatus === 'connecting' || connectionStatus === 'reconnecting',
            offline: connectionStatus === 'disconnected' || connectionStatus === 'error',
          }"
        ></span>
        <span class="topbar-status-text">{{ statusText }}</span>
      </div>

      <div class="topbar-center">
        <span class="topbar-shell" v-if="currentShell">{{ currentShell }}</span>
      </div>

      <div class="topbar-right">
        <button class="topbar-btn" @click="sendSignal('SIGINT')" title="Ctrl+C">
          <XIcon size="14" />
          <span>Ctrl+C</span>
        </button>
        <button class="topbar-btn" @click="clearScreen" title="清屏">
          <Trash2Icon size="14" />
          <span>清屏</span>
        </button>
        <button class="topbar-btn" :disabled="!canReconnect" @click="reconnect" title="重新连接">
          <RotateCcwIcon size="14" />
          <span>重连</span>
        </button>
        <button class="topbar-btn" @click="toggleDrawer" title="信息">
          <InfoIcon size="14" />
        </button>
      </div>
    </header>

    <div
      v-if="connectionHint"
      class="term-alert"
      :class="{
        danger: !isOnline || connectionStatus === 'error',
        subtle: connectionStatus === 'connecting' || connectionStatus === 'reconnecting',
      }"
    >
      {{ connectionHint }}
    </div>

    <div class="term-main-area">
      <!-- Terminal body -->
      <div class="term-canvas" ref="termContainer"></div>

      <!-- Right drawer -->
      <aside class="term-drawer" v-if="isDrawerOpen">
        <div class="drawer-header">
          <h4>会话信息</h4>
        </div>
        <div class="drawer-body">
          <div class="info-row">
            <span class="info-label">会话 ID</span>
            <span class="info-value mono">{{ sessionId || '—' }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">主机</span>
            <span class="info-value">{{ hostInfo.name }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">状态</span>
            <span class="info-value">{{ statusText }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">启动时间</span>
            <span class="info-value mono">{{ startedAt || '—' }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Shell</span>
            <span class="info-value mono">{{ currentShell }}</span>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.terminal-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #0f172a;
  color: #f8fafc;
}

/* Top bar */
.term-topbar {
  height: 48px;
  min-height: 48px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  background: #1e293b;
  border-bottom: 1px solid #334155;
  flex-shrink: 0;
}

.topbar-left,
.topbar-center,
.topbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.topbar-icon {
  color: #94a3b8;
}

.topbar-host {
  font-size: 14px;
  font-weight: 600;
  color: #e2e8f0;
}

.topbar-host-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.topbar-host-id {
  font-size: 11px;
  color: #94a3b8;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.topbar-status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #94a3b8;
}

.topbar-status-dot.online {
  background: #22c55e;
  box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
}

.topbar-status-dot.connecting {
  background: #eab308;
  animation: pulse 1.5s ease-in-out infinite;
}

.topbar-status-dot.offline {
  background: #ef4444;
}

.topbar-status-text {
  font-size: 12px;
  color: #94a3b8;
}

.topbar-shell {
  font-size: 12px;
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.topbar-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 10px;
  border-radius: 6px;
  background: transparent;
  border: 1px solid #334155;
  color: #94a3b8;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s;
}

.topbar-btn:hover {
  background: #334155;
  color: #e2e8f0;
}

.topbar-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* Main area */
.term-main-area {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.term-alert {
  padding: 10px 16px;
  font-size: 12px;
  line-height: 1.5;
  color: #fdba74;
  background: rgba(194, 65, 12, 0.16);
  border-bottom: 1px solid rgba(251, 146, 60, 0.2);
}

.term-alert.danger {
  color: #fecaca;
  background: rgba(127, 29, 29, 0.45);
  border-bottom-color: rgba(248, 113, 113, 0.28);
}

.term-alert.subtle {
  color: #cbd5f5;
  background: rgba(30, 41, 59, 0.9);
  border-bottom-color: rgba(148, 163, 184, 0.18);
}

.term-canvas {
  flex: 1;
  padding: 8px;
  overflow: hidden;
}

/* Drawer */
.term-drawer {
  width: 280px;
  min-width: 280px;
  background: #1e293b;
  border-left: 1px solid #334155;
  display: flex;
  flex-direction: column;
}

.drawer-header {
  padding: 16px;
  border-bottom: 1px solid #334155;
}

.drawer-header h4 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: #e2e8f0;
}

.drawer-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.info-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-value {
  font-size: 13px;
  color: #e2e8f0;
  word-break: break-all;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
