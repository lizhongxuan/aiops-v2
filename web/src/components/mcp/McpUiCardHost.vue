<script setup>
import { computed } from "vue";
import GenericMcpActionCard from "./GenericMcpActionCard.vue";
import McpActionFormCard from "./McpActionFormCard.vue";
import McpControlPanelCard from "./McpControlPanelCard.vue";
import McpKpiStripCard from "./McpKpiStripCard.vue";
import McpSummaryCard from "./McpSummaryCard.vue";
import McpTimeseriesChartCard from "./McpTimeseriesChartCard.vue";
import McpStatusTableCard from "./McpStatusTableCard.vue";
import McpTopologyCard from "./McpTopologyCard.vue";
import { MCP_UI_KINDS, normalizeMcpUiCard } from "../../lib/mcpUiCardModel";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["action", "detail", "refresh"]);

const KNOWN_UI_KIND_SET = new Set(MCP_UI_KINDS);
const TABLE_VISUAL_KINDS = new Set(["table", "status_table", "status-table"]);

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function parseStamp(value) {
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? stamp : 0;
}

const normalizedCard = computed(() => {
  const actions = props.card?.actions?.length
    ? props.card.actions
    : props.card?.action
      ? [props.card.action]
      : [];

  return {
    ...props.card,
    ...normalizeMcpUiCard({
      ...props.card,
      actions,
    }),
  };
});

const rawUiKind = computed(() => compactText(props.card?.uiKind || props.card?.ui_kind).toLowerCase());
const placement = computed(() => {
  if ((resolvedUiKind.value === "fallback_action" || resolvedUiKind.value === "fallback_form") && !props.card?.placement) {
    return "inline_action";
  }
  return normalizedCard.value.placement || "inline_final";
});
const hasActionSurface = computed(() => Boolean(props.card?.action) || normalizedCard.value.actions.length > 0);

const resolvedUiKind = computed(() => {
  if (!KNOWN_UI_KIND_SET.has(rawUiKind.value) && hasActionSurface.value) {
    return props.card?.action?.payloadSchema ? "fallback_form" : "fallback_action";
  }
  return normalizedCard.value.uiKind;
});

const rendererCard = computed(() => {
  if (["action_panel", "form_panel", "fallback_action", "fallback_form"].includes(resolvedUiKind.value)) {
    return {
      ...normalizedCard.value,
      uiKind: ["fallback_action", "fallback_form"].includes(resolvedUiKind.value)
        ? props.card?.uiKind || props.card?.ui_kind || normalizedCard.value.uiKind
        : resolvedUiKind.value,
      action: props.card?.action || normalizedCard.value.actions[0] || null,
    };
  }
  return normalizedCard.value;
});

const rendererComponent = computed(() => {
  if (resolvedUiKind.value === "fallback_action" || resolvedUiKind.value === "fallback_form") {
    return GenericMcpActionCard;
  }
  if (resolvedUiKind.value === "form_panel") {
    return McpActionFormCard;
  }
  if (resolvedUiKind.value === "action_panel") {
    return McpControlPanelCard;
  }

  const visualKind = compactText(rendererCard.value.visual?.kind).toLowerCase();
  if (resolvedUiKind.value === "readonly_summary" && (visualKind === "kpi_strip" || visualKind === "kpi-strip" || props.card?.kpis?.length)) {
    return McpKpiStripCard;
  }
  if (resolvedUiKind.value === "readonly_chart") {
    return TABLE_VISUAL_KINDS.has(visualKind) ? McpStatusTableCard : McpTimeseriesChartCard;
  }
  if (resolvedUiKind.value === "topology_card") {
    return McpTopologyCard;
  }
  return hasActionSurface.value ? GenericMcpActionCard : McpSummaryCard;
});

const stale = computed(() => {
  if (props.card?.stale === true) return true;
  const staleAt = parseStamp(rendererCard.value.freshness?.staleAt);
  return Boolean(staleAt && staleAt <= Date.now());
});

const errorText = computed(() => compactText(props.card?.error || rendererCard.value.error || rendererCard.value.errors?.[0]?.message || ""));

function forwardAction(payload) {
  emit("action", payload);
}

function forwardDetail(payload) {
  emit("detail", payload);
}

function forwardRefresh(payload) {
  emit("refresh", payload);
}
</script>

<template>
  <section
    class="mcp-ui-card-host"
    :class="[`placement-${placement}`, `ui-${resolvedUiKind}`]"
    :data-placement="placement"
    :data-ui-kind="resolvedUiKind"
    data-testid="mcp-ui-card-host"
  >
    <p
      v-if="stale"
      class="state-banner stale-banner"
      data-testid="mcp-card-stale-banner"
    >
      当前快照可能已过期，建议重新拉取后再执行操作。
    </p>

    <p
      v-if="errorText"
      class="state-banner error-banner"
      data-testid="mcp-card-error"
    >
      {{ errorText }}
    </p>

    <div
      v-if="rendererCard.empty"
      class="empty-state"
      data-testid="mcp-card-empty-state"
    >
      当前没有可展示的数据。
    </div>

    <component
      :is="rendererComponent"
      :card="rendererCard"
      embedded
      @action="forwardAction"
      @detail="forwardDetail"
      @refresh="forwardRefresh"
    />
  </section>
</template>

<style scoped>
.mcp-ui-card-host {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid rgba(15, 23, 42, 0.09);
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.98));
}

.placement-inline_action {
  background: linear-gradient(180deg, rgba(255, 251, 235, 0.95), rgba(255, 255, 255, 0.98));
}

.placement-side_panel,
.placement-drawer,
.placement-modal {
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
}

.state-banner,
.empty-state {
  margin: 0;
  padding: 10px 12px;
  border-radius: 14px;
  font-size: 13px;
}

.stale-banner {
  background: rgba(254, 243, 199, 0.85);
  color: #92400e;
}

.error-banner {
  background: rgba(254, 226, 226, 0.9);
  color: #991b1b;
}

.empty-state {
  background: rgba(241, 245, 249, 0.95);
  color: #475569;
}
</style>
