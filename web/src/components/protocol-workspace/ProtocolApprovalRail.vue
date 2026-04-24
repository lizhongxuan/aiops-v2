<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { CheckCircle2Icon, Clock3Icon, MonitorIcon, ShieldAlertIcon, ShieldCheckIcon, ShieldXIcon, SparklesIcon } from "lucide-vue-next";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  approval: {
    type: Object,
    default: null,
  },
  title: {
    type: String,
    default: "审批决策栏",
  },
  subtitle: {
    type: String,
    default: "固定右侧展示当前审批，证据通过弹框查看。",
  },
  stickyTop: {
    type: String,
    default: "20px",
  },
  busy: {
    type: Boolean,
    default: false,
  },
  queueItems: {
    type: Array,
    default: () => [],
  },
  activeApprovalId: {
    type: String,
    default: "",
  },
  hostLabel: {
    type: String,
    default: "Host",
  },
  commandLabel: {
    type: String,
    default: "命令",
  },
  countdownLabel: {
    type: String,
    default: "倒计时",
  },
  detailsLabel: {
    type: String,
    default: "详情",
  },
  detailButtonLabel: {
    type: String,
    default: "详情",
  },
  authorizeLabel: {
    type: String,
    default: "授权",
  },
  rejectLabel: {
    type: String,
    default: "拒绝",
  },
  acceptLabel: {
    type: String,
    default: "同意执行",
  },
  emptyLabel: {
    type: String,
    default: "当前没有待处理的审批。",
  },
  countdownSource: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["detail", "authorize", "reject", "accept"]);

const now = ref(Date.now());
let timer = null;

const normalizedQueue = computed(() => {
  const queue = Array.isArray(props.queueItems) ? props.queueItems : [];
  const source = queue.length ? queue : props.approval ? [props.approval] : [];
  return source.map((item) => normalizeApproval(item)).filter(Boolean);
});
const hasApproval = computed(() => normalizedQueue.value.length > 0);
const queueCount = computed(() => normalizedQueue.value.length);

function normalizeApproval(value) {
  if (!value || typeof value !== "object") return null;
  return {
    ...value,
    id: String(value.id || value.approvalId || value.requestId || value.hostId || value.title || ""),
    host: String(value.host || value.hostName || value.hostId || ""),
    command: String(value.command || value.text || value.summary || ""),
    title: String(value.title || value.label || value.name || ""),
    commandLabel: String(value.commandLabel || "执行命令:"),
    supportsAuthorize: value.supportsAuthorize !== false,
  };
}

function approvalRows(approval) {
  return normalizeDetailRows(
    approval?.details ||
      approval?.detailRows ||
      approval?.info ||
      approval?.meta ||
      approval?.context ||
      approval?.notes ||
      approval?.summary,
  );
}

function normalizeDetailRows(value) {
  if (!value) return [];
  if (Array.isArray(value)) {
    return value
      .flatMap((item) => {
        if (item == null) return [];
        if (typeof item === "string") {
          const text = item.trim();
          return text ? [{ key: "", value: text, text }] : [];
        }
        if (typeof item === "object") {
          const key = item.key || item.label || item.name || "";
          const val = item.value ?? item.text ?? item.content ?? item.summary ?? "";
          const text = item.text || item.valueText || "";
          if (key || val || text) {
            return [{ key: String(key), value: String(val), text: text ? String(text) : "" }];
          }
        }
        return [{ key: "", value: String(item), text: "" }];
      })
      .filter((row) => row.key || row.value || row.text);
  }
  if (typeof value === "object") {
    return Object.entries(value).map(([key, val]) => ({
      key: String(key),
      value: Array.isArray(val) ? val.join(", ") : String(val ?? ""),
      text: "",
    }));
  }
  const text = String(value).trim();
  return text ? [{ key: "", value: text, text }] : [];
}

function approvalTone(approval) {
  const tone = String(approval?.tone || approval?.status || approval?.state || approval?.risk || "").toLowerCase();
  if (tone.includes("danger") || tone.includes("reject") || tone.includes("fail") || tone.includes("block")) return "danger";
  if (tone.includes("warning") || tone.includes("wait") || tone.includes("pending")) return "warning";
  if (tone.includes("success") || tone.includes("allow") || tone.includes("approved")) return "success";
  return "neutral";
}

function formatDuration(ms) {
  const total = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  if (hours > 0) return `${hours}h ${minutes.toString().padStart(2, "0")}m`;
  if (minutes > 0) return `${minutes}m ${seconds.toString().padStart(2, "0")}s`;
  return `${seconds}s`;
}

function countdownText(approval) {
  if (!approval) return "";
  const explicit = approval.countdown || approval.remainingLabel || approval.timeLeft || approval.expiresIn;
  if (explicit) return String(explicit);
  const deadline = approval.deadlineAt || approval.expiresAt || props.countdownSource;
  if (!deadline) return "";
  const target = new Date(deadline).getTime();
  if (!Number.isFinite(target)) return "";
  const diff = target - now.value;
  if (diff <= 0) return "已超时";
  return formatDuration(diff);
}

function startTimer() {
  stopTimer();
  const deadline = normalizedQueue.value[0]?.deadlineAt || normalizedQueue.value[0]?.expiresAt || props.countdownSource;
  if (!deadline) return;
  timer = window.setInterval(() => {
    now.value = Date.now();
  }, 1000);
}

function stopTimer() {
  if (timer) {
    window.clearInterval(timer);
    timer = null;
  }
}

function emitAction(name, approval) {
  if (!approval || props.busy) return;
  emit(name, approval);
}

function selectApproval(approval) {
  if (!approval) return;
  emit("detail", approval);
}

onMounted(() => {
  if (typeof window !== "undefined") {
    startTimer();
  }
});

watch(
  () => [props.approval, props.queueItems, props.countdownSource],
  () => {
    now.value = Date.now();
    if (typeof window !== "undefined") {
      startTimer();
    }
  },
  { deep: true },
);

onBeforeUnmount(() => {
  stopTimer();
});
</script>

<template>
  <aside class="protocol-approval-rail" :style="{ top: stickyTop }" data-testid="protocol-approval-rail">
    <header class="rail-head">
      <div class="rail-head-copy">
        <span class="rail-kicker">
          <ShieldAlertIcon size="14" />
          <span>APPROVAL</span>
        </span>
        <h3>{{ title }}</h3>
        <p>{{ subtitle }}</p>
      </div>
      <div class="rail-count">
        <strong>{{ queueCount }}</strong>
        <span>待处理</span>
      </div>
    </header>

    <div v-if="hasApproval" class="approval-list">
      <section
        v-for="approval in normalizedQueue"
        :key="approval.id"
        class="approval-card"
        :class="[`tone-${approvalTone(approval)}`, { active: approval.id === activeApprovalId }]"
        :data-testid="`protocol-approval-${approval.id}`"
      >
        <div class="approval-card-top">
          <div class="approval-card-title">
            <strong>
              <MonitorIcon size="14" style="color: #64748b" />
              {{ approval.title || approval.label || approval.name || approval.host || approval.hostName || approval.hostId || props.title }}
            </strong>
          </div>
          <span v-if="countdownText(approval)" class="approval-timer">⏱ {{ countdownText(approval) }}</span>
        </div>

        <div class="approval-command-inline">
          <span class="cmd-dot"></span>
          <span class="cmd-label">{{ approval.commandLabel || commandLabel }}</span>
        </div>
        <div class="approval-command-text">{{ approval.command || approval.text || approval.summary || "未提供计划或命令" }}</div>

        <div class="approval-actions">
          <button class="action-btn ghost" type="button" :disabled="busy" @click="selectApproval(approval)">
            <span>{{ detailButtonLabel }}</span>
          </button>
          <button v-if="approval.supportsAuthorize" class="action-btn secondary" type="button" :disabled="busy" @click="emitAction('authorize', approval)">
            <span>{{ authorizeLabel }}</span>
          </button>
          <button class="action-btn danger" type="button" :disabled="busy" @click="emitAction('reject', approval)">
            <span>{{ rejectLabel }}</span>
          </button>
          <button class="action-btn primary" type="button" :disabled="busy" @click="emitAction('accept', approval)">
            <CheckCircle2Icon size="12" />
            <span>{{ acceptLabel }}</span>
          </button>
        </div>
      </section>
    </div>

    <section v-else class="approval-empty">
      <span class="section-label">状态</span>
      <strong>{{ emptyLabel }}</strong>
    </section>
  </aside>
</template>

<style scoped>
.protocol-approval-rail {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  flex: 1 1 0;
  overflow: hidden;
  background: #f8fafc;
}

.approval-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1;
  overflow-y: auto;
  padding: 0 12px 12px;
}

.rail-head {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
  padding: 12px 14px;
  flex-shrink: 0;
}

.rail-head-copy {
  min-width: 0;
}

.rail-kicker {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #f59e0b;
}

.section-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.rail-head h3 {
  margin: 4px 0 0;
  color: #1e293b;
  font-size: 15px;
  line-height: 1.2;
}

.rail-head p {
  display: none;
}

.rail-count {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1px;
  min-width: 48px;
  padding: 6px 8px;
  border-radius: 10px;
  background: #fef3c7;
  color: #92400e;
}

.rail-count strong {
  font-size: 18px;
  line-height: 1;
}

.rail-count span {
  font-size: 10px;
  font-weight: 600;
}

/* ===== Approval Card (target design) ===== */
.approval-card,
.approval-empty {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  background: #ffffff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.approval-card.active {
  border-color: #93c5fd;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04), 0 0 0 2px rgba(191, 219, 254, 0.5);
}

.approval-card-top {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: flex-start;
}

.approval-card-title {
  min-width: 0;
}

.approval-badge {
  display: none;
}

.approval-card-title strong {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: #1e293b;
  font-size: 14px;
}

.approval-card-title p {
  display: none;
}

.approval-link {
  display: none;
}

/* Timer pill (red) */
.approval-meta {
  display: none;
}

/* Show timer inline in card top */
.approval-card-top::after {
  content: attr(data-timer);
}

.approval-command-block {
  margin-top: 8px;
}

.approval-command-block .section-label {
  display: none;
}

.approval-command-block pre {
  margin: 0;
  padding: 0;
  border-radius: 0;
  background: transparent;
  color: #1e293b;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.approval-detail-block {
  display: none;
}

.approval-actions {
  display: flex;
  gap: 6px;
  margin-top: 10px;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 32px;
  padding: 0 12px;
  border-radius: 8px;
  border: 1px solid transparent;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}

.action-btn:hover:not(:disabled) {
  transform: translateY(-1px);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.action-btn.ghost {
  background: #f1f5f9;
  border-color: #e2e8f0;
  color: #475569;
}
.action-btn.ghost:hover { background: #e2e8f0; }

.action-btn.secondary {
  background: #3b82f6;
  color: #ffffff;
}
.action-btn.secondary:hover { background: #2563eb; }

.action-btn.danger {
  background: #f97316;
  color: #ffffff;
}
.action-btn.danger:hover { background: #ea580c; }

.action-btn.primary {
  background: #22c55e;
  color: #ffffff;
  flex: 1;
  box-shadow: none;
}
.action-btn.primary:hover { background: #16a34a; }

.approval-empty {
  padding: 10px 14px;
}


.approval-timer {
  font-size: 11px;
  font-weight: 600;
  color: #ef4444;
  background: #fef2f2;
  padding: 2px 8px;
  border-radius: 10px;
  white-space: nowrap;
  flex-shrink: 0;
}

.approval-command-inline {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  font-size: 12px;
  color: #64748b;
}

.cmd-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22c55e;
  flex-shrink: 0;
}

.cmd-label {
  white-space: nowrap;
}

.approval-command-text {
  margin-top: 2px;
  padding-left: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  color: #1e293b;
  font-weight: 500;
  line-height: 1.5;
  word-break: break-word;
}

.approval-empty p {
  margin: 6px 0 0;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 960px) {
  .approval-actions {
    flex-wrap: wrap;
  }
}
</style>
