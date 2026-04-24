<script setup>
import { computed } from "vue";

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

const emit = defineEmits(["detail", "refresh"]);

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function parseStamp(value) {
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? stamp : 0;
}

function firstRefreshAction(card = {}) {
  return (Array.isArray(card.actions) ? card.actions : []).find((action) => action.intent === "refresh" && !action.disabled) || null;
}

const refreshAction = computed(() => firstRefreshAction(props.card));
const stale = computed(() => {
  if (props.card?.stale === true) return true;
  const staleAt = parseStamp(props.card?.freshness?.staleAt);
  return Boolean(staleAt && staleAt <= Date.now());
});

const errorText = computed(() => compactText(props.card?.error || props.card?.errors?.[0]?.message || ""));
const scopePills = computed(() => {
  const scope = props.card?.scope || {};
  return [
    scope.service ? `服务 ${scope.service}` : "",
    scope.hostId ? `主机 ${scope.hostId}` : "",
    scope.env ? `环境 ${scope.env}` : "",
    scope.cluster ? `集群 ${scope.cluster}` : "",
    scope.timeRange ? `范围 ${scope.timeRange}` : "",
  ].filter(Boolean);
});
const freshnessLabel = computed(() => props.card?.freshness?.label || props.card?.freshness?.capturedAt || "");

function emitDetail() {
  emit("detail", props.card);
}

function emitRefresh() {
  emit("refresh", refreshAction.value || { intent: "refresh", cardId: props.card?.id || "" });
}
</script>

<template>
  <section
    class="mcp-readonly-card-frame"
    :class="{ embedded }"
    data-testid="mcp-readonly-card-frame"
  >
    <header v-if="!embedded" class="frame-header">
      <div class="frame-copy">
        <p class="frame-eyebrow">{{ card.source || "mcp" }} / {{ card.mcpServer || "default" }}</p>
        <h4 class="frame-title">{{ card.title || "监控卡片" }}</h4>
        <p v-if="card.summary" class="frame-summary">{{ card.summary }}</p>
      </div>
      <div class="frame-meta">
        <span v-if="freshnessLabel" class="meta-pill" data-testid="mcp-readonly-freshness">{{ freshnessLabel }}</span>
        <span v-if="card.scope?.timeRange" class="meta-pill">{{ card.scope.timeRange }}</span>
      </div>
    </header>

    <div v-if="scopePills.length && !embedded" class="scope-pills" data-testid="mcp-readonly-scope">
      <span
        v-for="pill in scopePills"
        :key="pill"
        class="scope-pill"
      >
        {{ pill }}
      </span>
    </div>

    <p
      v-if="stale"
      class="state-banner stale-banner"
      data-testid="mcp-card-stale-banner"
    >
      当前快照可能已过期。
    </p>

    <p
      v-if="errorText"
      class="state-banner error-banner"
      data-testid="mcp-card-error"
    >
      {{ errorText }}
    </p>

    <div
      v-if="card.empty"
      class="empty-state"
      data-testid="mcp-card-empty-state"
    >
      当前没有可展示的数据。
    </div>

    <div class="frame-body">
      <slot />
    </div>

    <footer class="frame-actions">
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-card-detail"
        @click="emitDetail"
      >
        查看详情
      </button>
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-card-refresh"
        @click="emitRefresh"
      >
        {{ refreshAction?.label || "刷新" }}
      </button>
    </footer>
  </section>
</template>

<style scoped>
.mcp-readonly-card-frame {
  display: grid;
  gap: 12px;
  padding: 14px;
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.94);
  border: 1px solid rgba(15, 23, 42, 0.08);
}

.mcp-readonly-card-frame.embedded {
  padding: 0;
  border: none;
  background: transparent;
}

.frame-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.frame-copy {
  display: grid;
  gap: 4px;
}

.frame-eyebrow {
  margin: 0;
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.frame-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.frame-summary {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #334155;
}

.frame-meta,
.scope-pills,
.frame-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.meta-pill,
.scope-pill {
  padding: 5px 10px;
  border-radius: 999px;
  background: rgba(226, 232, 240, 0.82);
  font-size: 12px;
  color: #334155;
}

.state-banner,
.empty-state {
  margin: 0;
  padding: 10px 12px;
  border-radius: 12px;
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

.frame-body {
  min-width: 0;
}

.secondary-btn {
  border: none;
  border-radius: 12px;
  padding: 8px 12px;
  background: rgba(226, 232, 240, 0.86);
  color: #0f172a;
  font-size: 13px;
  cursor: pointer;
}
</style>
