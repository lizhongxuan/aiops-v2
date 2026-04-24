<script setup>
import { computed } from "vue";
import { CheckCircle2Icon, CircleAlertIcon, PlayCircleIcon, SquareArrowOutUpRightIcon } from "lucide-vue-next";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  step: {
    type: [String, Object],
    required: true,
  },
  status: {
    type: String,
    default: "pending",
  },
  statusLabel: {
    type: String,
    default: "",
  },
  hostAgent: {
    type: [String, Object, Array],
    default: "",
  },
  detail: {
    type: String,
    default: "",
  },
  note: {
    type: String,
    default: "",
  },
  tags: {
    type: Array,
    default: () => [],
  },
  actions: {
    type: Array,
    default: () => [],
  },
  index: {
    type: [String, Number],
    default: "",
  },
});

const emit = defineEmits(["action", "host-select"]);

const normalizedStep = computed(() => {
  if (typeof props.step === "string") {
    return {
      title: props.step,
      description: "",
      stepId: "",
    };
  }
  const value = props.step && typeof props.step === "object" ? props.step : {};
  return {
    title: String(value.title || value.step || value.label || "Step"),
    description: String(value.description || value.summary || value.text || ""),
    stepId: String(value.id || value.stepId || value.key || ""),
  };
});

const normalizedHosts = computed(() => {
  const raw = props.hostAgent;
  const items = Array.isArray(raw) ? raw : raw ? [raw] : [];
  return items
    .map((item) => {
      if (typeof item === "string") {
        return { id: item, label: item, tone: "neutral" };
      }
      return {
        id: String(item?.id || item?.hostId || item?.name || item?.label || item?.title || ""),
        label: String(item?.label || item?.name || item?.title || item?.hostId || item?.id || ""),
        tone: String(item?.tone || item?.status || "neutral"),
      };
    })
    .filter((item) => item.label);
});

function resolveTone(status) {
  const value = String(status || "").toLowerCase();
  if (!value) return "neutral";
  if (
    value.includes("complete") ||
    value.includes("done") ||
    value.includes("success") ||
    value.includes("finished")
  ) {
    return "success";
  }
  if (value.includes("run") || value.includes("progress") || value.includes("active")) {
    return "active";
  }
  if (value.includes("wait") || value.includes("block") || value.includes("hold") || value.includes("warn")) {
    return "warning";
  }
  if (value.includes("fail") || value.includes("error") || value.includes("reject")) {
    return "danger";
  }
  if (value.includes("skip") || value.includes("mute")) {
    return "muted";
  }
  return "neutral";
}

const toneClass = computed(() => resolveTone(props.status));

function actionTone(action) {
  return String(action?.tone || action?.variant || "").toLowerCase();
}

function clickAction(action, index) {
  emit("action", { action, index });
}

function selectHost(host) {
  emit("host-select", host);
}
</script>

<template>
  <article class="protocol-plan-summary-card" :class="toneClass">
    <div class="plan-head">
      <div class="plan-step">
        <div class="plan-step-kicker">
          <span v-if="index !== ''" class="plan-index">Step {{ index }}</span>
          <span v-else class="plan-index">Plan step</span>
        </div>
        <h3>{{ normalizedStep.title }}</h3>
        <p v-if="normalizedStep.description">{{ normalizedStep.description }}</p>
      </div>

      <div class="plan-status">
        <component
          :is="toneClass === 'success' ? CheckCircle2Icon : toneClass === 'warning' ? CircleAlertIcon : PlayCircleIcon"
          size="16"
        />
        <span>{{ statusLabel || status || "pending" }}</span>
      </div>
    </div>

    <div class="plan-meta">
      <div class="plan-meta-block">
        <span class="plan-meta-label">Host-agent</span>
        <div v-if="normalizedHosts.length" class="plan-hosts">
          <button
            v-for="host in normalizedHosts"
            :key="host.id || host.label"
            type="button"
            class="plan-host-pill"
            :class="[host.tone, 'clickable']"
            @click="selectHost(host)"
          >
            {{ host.label }}
          </button>
        </div>
        <span v-else class="plan-meta-empty">未分配</span>
      </div>

      <div v-if="tags.length" class="plan-meta-block">
        <span class="plan-meta-label">Tags</span>
        <div class="plan-tags">
          <span v-for="tag in tags" :key="tag.id || tag.label || tag" class="plan-tag">
            {{ tag.label || tag.text || tag }}
          </span>
        </div>
      </div>
    </div>

    <p v-if="detail" class="plan-detail">{{ detail }}</p>
    <p v-if="note" class="plan-note">{{ note }}</p>

    <div v-if="actions.length" class="plan-actions">
      <button
        v-for="(action, actionIndex) in actions"
        :key="action.id || action.key || action.label || actionIndex"
        type="button"
        class="plan-action"
        :class="actionTone(action)"
        :disabled="action.disabled"
        @click="clickAction(action, actionIndex)"
      >
        <span>{{ action.label || action.text || "Action" }}</span>
        <SquareArrowOutUpRightIcon v-if="action.icon !== false" size="14" />
      </button>
    </div>
  </article>
</template>

<style scoped>
.protocol-plan-summary-card {
  border-radius: 22px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.04), transparent 34%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.06);
  padding: 16px;
}

.protocol-plan-summary-card.success {
  border-color: rgba(167, 243, 208, 0.95);
}

.protocol-plan-summary-card.active {
  border-color: rgba(191, 219, 254, 0.95);
}

.protocol-plan-summary-card.warning {
  border-color: rgba(253, 230, 138, 0.95);
}

.protocol-plan-summary-card.danger {
  border-color: rgba(254, 202, 202, 0.95);
}

.protocol-plan-summary-card.muted {
  opacity: 0.98;
}

.plan-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.plan-step {
  min-width: 0;
}

.plan-step-kicker {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.plan-index {
  display: inline-flex;
  align-items: center;
  padding: 5px 9px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.plan-step h3 {
  margin: 0;
  font-size: 16px;
  line-height: 1.4;
  color: #0f172a;
}

.plan-step p,
.plan-detail,
.plan-note {
  margin: 8px 0 0;
  color: #475569;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.plan-status {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-radius: 999px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.92);
  color: #334155;
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.plan-meta {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  margin-top: 14px;
}

.plan-meta-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.plan-meta-label {
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.plan-hosts,
.plan-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.plan-host-pill,
.plan-tag {
  display: inline-flex;
  align-items: center;
  padding: 7px 10px;
  border-radius: 999px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(248, 250, 252, 0.95);
  color: #0f172a;
  font-size: 12px;
  font-weight: 700;
}

.plan-host-pill.clickable {
  cursor: pointer;
  transition:
    transform 120ms ease,
    box-shadow 120ms ease,
    border-color 120ms ease;
}

.plan-host-pill.clickable:hover {
  transform: translateY(-1px);
  border-color: rgba(96, 165, 250, 0.9);
  box-shadow: 0 10px 20px rgba(37, 99, 235, 0.12);
}

.plan-host-pill.success {
  background: #ecfdf5;
  border-color: rgba(167, 243, 208, 0.95);
  color: #047857;
}

.plan-host-pill.warning {
  background: #fffbeb;
  border-color: rgba(253, 230, 138, 0.95);
  color: #b45309;
}

.plan-host-pill.danger {
  background: #fef2f2;
  border-color: rgba(254, 202, 202, 0.95);
  color: #b91c1c;
}

.plan-meta-empty {
  color: #94a3b8;
  font-size: 13px;
}

.plan-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}

.plan-action {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 9px 12px;
  border-radius: 12px;
  border: 1px solid rgba(191, 219, 254, 0.95);
  background: white;
  color: #1d4ed8;
  font-size: 13px;
  font-weight: 800;
  cursor: pointer;
  transition:
    transform 120ms ease,
    box-shadow 120ms ease,
    border-color 120ms ease;
}

.plan-action:hover:not(:disabled) {
  transform: translateY(-1px);
  border-color: rgba(96, 165, 250, 0.96);
  box-shadow: 0 10px 20px rgba(37, 99, 235, 0.1);
}

.plan-action:disabled {
  opacity: 0.58;
  cursor: not-allowed;
}

.plan-action.danger {
  color: #b91c1c;
  border-color: rgba(254, 202, 202, 0.95);
}

.plan-action.warning {
  color: #b45309;
  border-color: rgba(253, 230, 138, 0.95);
}

.plan-action.success {
  color: #047857;
  border-color: rgba(167, 243, 208, 0.95);
}

@media (max-width: 760px) {
  .plan-head {
    flex-direction: column;
  }

  .plan-actions {
    justify-content: flex-start;
  }
}
</style>
