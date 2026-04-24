<script setup>
import { computed } from "vue";
import { normalizeMcpUiAction, normalizeMcpUiActions } from "../../lib/mcpUiCardModel";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["action", "detail", "refresh"]);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function labelFromScope(scope = {}) {
  const candidate = asObject(scope);
  return [candidate.service, candidate.hostId, candidate.resourceType && candidate.resourceId ? `${candidate.resourceType}:${candidate.resourceId}` : candidate.resourceId]
    .filter(Boolean)
    .join(" / ");
}

function paramsEntries(params = {}) {
  return Object.entries(asObject(params))
    .filter(([, value]) => value !== undefined && value !== null && value !== "");
}

const resolvedCardActions = computed(() => {
  const rawActions = props.card?.actions?.length
    ? props.card.actions
    : props.card?.action
      ? [props.card.action]
      : [];
  const normalized = normalizeMcpUiActions(asArray(rawActions), {
    uiKind: props.card?.uiKind || "action_panel",
  });
  if (normalized.length) return normalized;
  return [
    normalizeMcpUiAction(
      {
        id: "fallback-open",
        label: "未命名操作",
        intent: "open",
        mutation: false,
      },
      0,
      props.card?.uiKind || "action_panel",
    ),
  ];
});

const refreshAction = computed(() =>
  resolvedCardActions.value.find((action) => action.intent === "refresh" && !action.disabled) || null,
);

const primaryAction = computed(() => {
  return resolvedCardActions.value.find((action) => action.intent !== "refresh" && !action.disabled)
    || refreshAction.value
    || resolvedCardActions.value[0];
});

const targetLabel = computed(() => {
  const actionTarget = asObject(primaryAction.value?.target);
  return actionTarget.label
    || actionTarget.name
    || actionTarget.resourceId
    || actionTarget.id
    || labelFromScope(props.card?.scope)
    || "当前监控对象";
});

const actionParams = computed(() => paramsEntries(primaryAction.value?.params));

const permissionLabel = computed(() => {
  const action = primaryAction.value;
  if (!action) return "未声明权限要求";
  if (action.approvalMode === "required" || action.mutation) return "变更操作，需要审批";
  if (action.approvalMode === "optional") return "可选审批，建议先确认影响范围";
  if (action.intent === "refresh") return "只读刷新，不触发变更";
  return "只读操作，可直接执行";
});

const permissionPath = computed(() => {
  const schema = asObject(primaryAction.value?.payloadSchema);
  return primaryAction.value?.permissionPath
    || primaryAction.value?.permission_path
    || schema.permissionPath
    || schema.permission_path
    || "";
});

const primaryButtonLabel = computed(() => {
  const action = primaryAction.value;
  if (!action) return "查看详情";
  if (action.approvalMode === "required" || action.mutation) {
    return action.label ? `${action.label}（审批）` : "请求审批";
  }
  if (action.intent === "refresh") return action.label || "刷新";
  if (action.intent === "open" && action.label === "未命名操作") return "执行";
  return action.label || "执行";
});

function emitPrimaryAction() {
  if (!primaryAction.value) return;
  if (primaryAction.value.intent === "refresh") {
    emit("refresh", primaryAction.value);
    return;
  }
  emit("action", primaryAction.value);
}

function emitRefresh() {
  if (!refreshAction.value) return;
  emit("refresh", refreshAction.value);
}

function emitDetail() {
  emit("detail", props.card);
}

const scopeLabel = computed(() => labelFromScope(props.card?.scope));
const freshnessLabel = computed(() => props.card?.freshness?.label || props.card?.freshness?.capturedAt || "");
</script>

<template>
  <section class="generic-mcp-action-card" data-testid="generic-mcp-action-card">
    <header class="generic-header">
      <div class="generic-copy">
        <p class="generic-eyebrow">{{ card.uiKind === "form_panel" ? "结构化表单" : "通用操作面板" }}</p>
        <h4 class="generic-title">{{ card.title || "MCP 操作" }}</h4>
        <p v-if="card.summary" class="generic-summary">{{ card.summary }}</p>
      </div>
      <span class="permission-badge" data-testid="generic-mcp-permission">
        {{ permissionLabel }}
      </span>
    </header>

    <dl class="generic-meta">
      <div class="meta-row">
        <dt>操作名称</dt>
        <dd>{{ primaryAction?.label || "未命名操作" }}</dd>
      </div>
      <div class="meta-row">
        <dt>目标对象</dt>
        <dd data-testid="generic-mcp-action-target">{{ targetLabel }}</dd>
      </div>
      <div class="meta-row">
        <dt>主操作</dt>
        <dd>{{ primaryAction?.intent || "open" }}</dd>
      </div>
      <div v-if="permissionPath" class="meta-row">
        <dt>权限路径</dt>
        <dd data-testid="generic-mcp-action-permission">{{ permissionPath }}</dd>
      </div>
      <div v-if="scopeLabel" class="meta-row">
        <dt>作用范围</dt>
        <dd data-testid="generic-mcp-action-scope">{{ scopeLabel }}</dd>
      </div>
      <div v-if="freshnessLabel" class="meta-row">
        <dt>数据时效</dt>
        <dd data-testid="generic-mcp-action-freshness">{{ freshnessLabel }}</dd>
      </div>
    </dl>

    <div
      v-if="actionParams.length"
      class="generic-params"
      data-testid="generic-mcp-action-params"
    >
      <span class="params-label">关键参数</span>
      <div class="params-list">
        <span
          v-for="[key, value] in actionParams"
          :key="key"
          class="param-pill"
        >
          {{ key }}={{ value }}
        </span>
      </div>
    </div>

    <div class="generic-actions">
      <button
        type="button"
        class="secondary-btn"
        data-testid="generic-mcp-action-detail"
        @click="emitDetail"
      >
        查看详情
      </button>
      <button
        v-if="refreshAction"
        type="button"
        class="secondary-btn"
        data-testid="generic-mcp-action-refresh"
        @click="emitRefresh"
      >
        {{ refreshAction.label || "刷新" }}
      </button>
      <button
        type="button"
        class="primary-btn"
        data-testid="generic-mcp-action-primary"
        :data-approval-mode="primaryAction?.approvalMode || 'none'"
        @click="emitPrimaryAction"
      >
        {{ primaryButtonLabel }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.generic-mcp-action-card {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid rgba(15, 23, 42, 0.09);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
}

.generic-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.generic-copy {
  display: grid;
  gap: 4px;
}

.generic-eyebrow {
  margin: 0;
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.generic-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.generic-summary {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #334155;
}

.permission-badge {
  align-self: flex-start;
  padding: 5px 10px;
  border-radius: 999px;
  background: rgba(251, 191, 36, 0.16);
  color: #92400e;
  font-size: 12px;
  white-space: nowrap;
}

.generic-meta {
  display: grid;
  gap: 8px;
  margin: 0;
}

.meta-row {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: 10px;
  font-size: 13px;
}

.meta-row dt {
  color: #64748b;
}

.meta-row dd {
  margin: 0;
  color: #0f172a;
  word-break: break-word;
}

.generic-params {
  display: grid;
  gap: 8px;
}

.params-label {
  font-size: 12px;
  color: #64748b;
}

.params-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.param-pill {
  padding: 5px 10px;
  border-radius: 999px;
  background: rgba(226, 232, 240, 0.8);
  font-size: 12px;
  color: #334155;
}

.generic-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.primary-btn,
.secondary-btn {
  border: none;
  border-radius: 12px;
  padding: 9px 14px;
  font-size: 13px;
  cursor: pointer;
}

.primary-btn {
  background: #0f172a;
  color: #ffffff;
}

.secondary-btn {
  background: rgba(226, 232, 240, 0.85);
  color: #0f172a;
}
</style>
