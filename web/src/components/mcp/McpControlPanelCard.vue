<script setup>
import { computed } from "vue";
import { normalizeMcpUiAction, normalizeMcpUiActions } from "../../lib/mcpUiCardModel";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  embedded: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["action", "detail", "refresh"]);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function labelFromScope(scope = {}) {
  const candidate = asObject(scope);
  return [
    candidate.service,
    candidate.hostId,
    candidate.resourceType && candidate.resourceId ? `${candidate.resourceType}:${candidate.resourceId}` : candidate.resourceId,
  ]
    .filter(Boolean)
    .join(" / ");
}

const resolvedActions = computed(() => {
  const rawActions = props.card?.actions?.length
    ? props.card.actions
    : props.card?.action
      ? [props.card.action]
      : [];
  return normalizeMcpUiActions(asArray(rawActions), {
    uiKind: "action_panel",
  });
});

const refreshAction = computed(() => {
  return resolvedActions.value.find((action) => action.intent === "refresh" && !action.disabled) || null;
});

const primaryAction = computed(() => {
  return resolvedActions.value.find((action) => action.intent !== "refresh" && !action.disabled)
    || refreshAction.value
    || normalizeMcpUiAction(props.card?.action || {}, 0, "action_panel");
});

const targetLabel = computed(() => {
  const explicitTarget = asObject(props.card?.target);
  const actionTarget = asObject(primaryAction.value?.target);
  return explicitTarget.label
    || explicitTarget.name
    || actionTarget.label
    || actionTarget.name
    || actionTarget.resourceId
    || labelFromScope(props.card?.scope)
    || "当前监控对象";
});

const currentState = computed(() => {
  return compactText(props.card?.currentState || props.card?.current_state || props.card?.state || "unknown");
});

const permissionPath = computed(() => {
  return compactText(
    props.card?.permissionPath
      || props.card?.permission_path
      || primaryAction.value?.permissionPath
      || primaryAction.value?.permission_path
      || "",
  );
});

const riskText = computed(() => compactText(props.card?.risk || props.card?.impact || props.card?.summary || ""));
const confirmText = computed(() => compactText(primaryAction.value?.confirmText || props.card?.confirmText || props.card?.confirm_text || ""));

const showConfirmation = computed(() => {
  return Boolean(
    confirmText.value
      || riskText.value
      || primaryAction.value?.mutation
      || primaryAction.value?.destructive
      || primaryAction.value?.approvalMode === "required",
  );
});

const primaryButtonLabel = computed(() => {
  if (primaryAction.value?.approvalMode === "required" || primaryAction.value?.mutation) {
    return `${primaryAction.value?.label || "执行"}（审批）`;
  }
  return primaryAction.value?.label || "执行";
});

function emitPrimaryAction() {
  emit("action", primaryAction.value);
}

function emitDetail() {
  emit("detail", props.card);
}

function emitRefresh() {
  emit("refresh", refreshAction.value || { intent: "refresh", cardId: props.card?.id || "" });
}
</script>

<template>
  <section
    class="mcp-control-panel-card"
    :class="{ embedded }"
    data-testid="mcp-control-panel-card"
  >
    <header class="panel-header">
      <div class="panel-copy">
        <p class="panel-eyebrow">控制面板</p>
        <h4 class="panel-title">{{ card.title || "控制面板" }}</h4>
        <p v-if="card.summary" class="panel-summary">{{ card.summary }}</p>
      </div>
      <span class="panel-state" data-testid="mcp-control-panel-state">{{ currentState }}</span>
    </header>

    <dl class="panel-meta">
      <div class="meta-row">
        <dt>目标对象</dt>
        <dd data-testid="mcp-control-panel-target">{{ targetLabel }}</dd>
      </div>
      <div v-if="permissionPath" class="meta-row">
        <dt>权限路径</dt>
        <dd data-testid="mcp-control-panel-permission">{{ permissionPath }}</dd>
      </div>
      <div v-if="card.freshness?.label || card.freshness?.capturedAt" class="meta-row">
        <dt>时效</dt>
        <dd>{{ card.freshness?.label || card.freshness?.capturedAt }}</dd>
      </div>
    </dl>

    <p
      v-if="riskText"
      class="risk-copy"
      data-testid="mcp-control-panel-risk"
    >
      {{ riskText }}
    </p>

    <div
      v-if="showConfirmation"
      class="confirmation-box"
      data-testid="mcp-control-panel-confirmation"
    >
      <strong class="confirmation-title">执行前确认</strong>
      <p v-if="confirmText" class="confirmation-copy">{{ confirmText }}</p>
      <p v-if="riskText" class="confirmation-copy">{{ riskText }}</p>
    </div>

    <footer class="panel-actions">
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-control-panel-detail"
        @click="emitDetail"
      >
        查看详情
      </button>
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-control-panel-refresh"
        @click="emitRefresh"
      >
        {{ refreshAction?.label || "刷新状态" }}
      </button>
      <button
        type="button"
        class="primary-btn"
        data-testid="mcp-control-panel-action"
        @click="emitPrimaryAction"
      >
        {{ primaryButtonLabel }}
      </button>
    </footer>
  </section>
</template>

<style scoped>
.mcp-control-panel-card {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid rgba(15, 23, 42, 0.09);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 251, 235, 0.88), rgba(255, 255, 255, 0.98));
}

.mcp-control-panel-card.embedded {
  padding: 0;
  border: none;
  background: transparent;
}

.panel-header,
.panel-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 10px;
}

.panel-copy {
  display: grid;
  gap: 4px;
}

.panel-eyebrow {
  margin: 0;
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #92400e;
}

.panel-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.panel-summary,
.risk-copy,
.confirmation-copy {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #334155;
}

.panel-state {
  align-self: flex-start;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(226, 232, 240, 0.88);
  font-size: 12px;
  color: #0f172a;
}

.panel-meta {
  display: grid;
  gap: 8px;
  margin: 0;
}

.meta-row {
  display: grid;
  grid-template-columns: 70px 1fr;
  gap: 10px;
  font-size: 13px;
}

.meta-row dt {
  color: #64748b;
}

.meta-row dd {
  margin: 0;
  color: #0f172a;
}

.confirmation-box {
  display: grid;
  gap: 6px;
  padding: 12px;
  border-radius: 14px;
  background: rgba(254, 242, 242, 0.92);
  border: 1px solid rgba(248, 113, 113, 0.18);
}

.confirmation-title {
  font-size: 12px;
  color: #991b1b;
}

.primary-btn,
.secondary-btn {
  border: none;
  border-radius: 12px;
  padding: 8px 12px;
  font-size: 13px;
  cursor: pointer;
}

.primary-btn {
  background: #0f172a;
  color: white;
}

.secondary-btn {
  background: rgba(226, 232, 240, 0.86);
  color: #0f172a;
}
</style>
