<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { XIcon } from "lucide-vue-next";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  open: {
    type: Boolean,
    default: false,
  },
  activeTab: {
    type: String,
    default: "main-agent-plan",
  },
  title: {
    type: String,
    default: "证据弹框",
  },
  subtitle: {
    type: String,
    default: "按 tab 查看主 Agent 计划摘要、worker 对话、Host Terminal 与审批上下文。",
  },
  tabs: {
    type: Array,
    default: () => [
      { value: "main-agent-plan", label: "主 Agent 计划摘要" },
      { value: "worker-conversation", label: "Worker 对话" },
      { value: "host-terminal", label: "Host Terminal" },
      { value: "mcp-surface", label: "MCP 面板" },
      { value: "approval-context", label: "审批上下文" },
    ],
  },
  panels: {
    type: Object,
    default: () => ({}),
  },
  width: {
    type: String,
    default: "min(1120px, calc(100vw - 32px))",
  },
  maxHeight: {
    type: String,
    default: "min(86vh, 920px)",
  },
});

const emit = defineEmits(["close", "update:open", "update:activeTab", "switch", "action", "pin"]);
const bodyOverflow = ref("");

const normalizedTabs = computed(() =>
  (Array.isArray(props.tabs) ? props.tabs : [])
    .map((tab) => ({
      value: String(tab?.value || ""),
      label: String(tab?.label || tab?.value || ""),
      badge: tab?.badge,
    }))
    .filter((tab) => tab.value),
);

const currentTab = computed(() => normalizedTabs.value.find((tab) => tab.value === props.activeTab) || normalizedTabs.value[0] || null);
const displayTab = computed(() => currentTab.value?.value || props.activeTab);
const activePanel = computed(() => {
  if (!currentTab.value) return null;
  return normalizePanel(props.panels?.[currentTab.value.value] || props.panels?.[currentTab.value.value.replace(/-/g, "_")] || {});
});
const activePanelListItems = computed(() => panelListItems(activePanel.value));
const activePanelRawText = computed(() => panelRawText(activePanel.value));

watch(
  () => props.open,
  (value) => {
    if (typeof document === "undefined") return;
    if (value) {
      bodyOverflow.value = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = bodyOverflow.value;
    }
  },
  { immediate: true },
);

function normalizePanel(panel) {
  if (!panel || typeof panel !== "object") {
    return {
      title: "",
      summary: "",
      items: [],
      raw: "",
    };
  }
  return {
    ...panel,
    title: String(panel.title || panel.label || ""),
    summary: String(panel.summary || panel.description || ""),
    raw: panel.raw ?? panel.text ?? panel.content ?? "",
    items: Array.isArray(panel.items) ? panel.items : [],
    lines: Array.isArray(panel.lines) ? panel.lines : [],
    sections: Array.isArray(panel.sections) ? panel.sections : [],
    actions: Array.isArray(panel.actions) ? panel.actions : [],
  };
}

function selectTab(value) {
  if (!value || value === props.activeTab) return;
  emit("update:activeTab", value);
  emit("switch", value);
}

function panelListItems(panel = {}) {
  const source = panel || {};
  if (Array.isArray(source.items) && source.items.length) return source.items;
  if (Array.isArray(source.lines) && source.lines.length) return source.lines;
  if (Array.isArray(source.sections) && source.sections.length) return source.sections;
  return [];
}

function panelRawText(panel = {}) {
  const raw = panel?.raw;
  if (typeof raw === "string") return raw;
  if (raw && typeof raw === "object") {
    try {
      return JSON.stringify(raw, null, 2);
    } catch {
      return String(raw);
    }
  }
  return "";
}

function panelItemKey(item, index) {
  if (item && typeof item === "object") {
    return item.id || item.key || item.label || index;
  }
  return index;
}

function panelItemLabel(item) {
  if (!item || typeof item !== "object") return "";
  return item.label || item.key || "";
}

function panelValueText(value) {
  if (value === null || value === undefined) return "";
  if (["string", "number", "boolean"].includes(typeof value)) return String(value);
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function panelItemValue(item) {
  if (item && typeof item === "object") {
    return panelValueText(item.value ?? item.text ?? item.summary ?? item.content ?? item);
  }
  return panelValueText(item);
}

function requestClose() {
  emit("close");
  emit("update:open", false);
}

function triggerAction(action) {
  if (!action || typeof action !== "object") return;
  emit("action", action);
}

function requestPin() {
  emit("pin", {
    activeTab: displayTab.value,
  });
}

function onBackdropClick() {
  requestClose();
}

function onKeydown(event) {
  if (event.key === "Escape" && props.open) {
    requestClose();
  }
}

onMounted(() => {
  document.addEventListener("keydown", onKeydown);
});

onBeforeUnmount(() => {
  document.removeEventListener("keydown", onKeydown);
  if (typeof document !== "undefined") {
    document.body.style.overflow = bodyOverflow.value;
  }
});
</script>

<template>
  <teleport to="body">
    <transition name="protocol-evidence-fade">
      <div v-if="open" class="protocol-evidence-backdrop" @click.self="onBackdropClick">
        <section class="protocol-evidence-modal" :style="{ maxWidth: width, maxHeight }" role="dialog" aria-modal="true">
          <header class="modal-head">
            <div class="modal-copy">
              <span class="modal-kicker">EVIDENCE</span>
              <h3>{{ title }}</h3>
              <p>{{ subtitle }}</p>
            </div>
            <div class="modal-head-actions">
              <button class="pin-btn" type="button" data-testid="protocol-evidence-pin" @click="requestPin">
                固定到证据抽屉
              </button>
              <button class="close-btn" type="button" @click="requestClose">
                <XIcon size="18" />
              </button>
            </div>
          </header>

          <nav class="modal-tabs" aria-label="证据分区">
            <button
              v-for="tab in normalizedTabs"
              :key="tab.value"
              type="button"
              class="modal-tab"
              :class="{ active: tab.value === activeTab }"
              @click="selectTab(tab.value)"
            >
              <span>{{ tab.label }}</span>
              <small v-if="tab.badge !== undefined">{{ tab.badge }}</small>
            </button>
          </nav>

          <div class="modal-body">
            <slot :name="displayTab" :panel="activePanel">
              <section class="panel-fallback">
                <div v-if="activePanel?.title || activePanel?.summary" class="panel-hero">
                  <h4 v-if="activePanel?.title">{{ activePanel.title }}</h4>
                  <p v-if="activePanel?.summary">{{ activePanel.summary }}</p>
                  <div v-if="activePanel.actions?.length" class="panel-actions">
                    <button
                      v-for="action in activePanel.actions"
                      :key="action.id || action.kind || action.label"
                      type="button"
                      class="panel-action"
                      @click="triggerAction(action)"
                    >
                      {{ action.label || action.title || action.kind }}
                    </button>
                  </div>
                </div>

                <div v-if="activePanelListItems.length" class="panel-list">
                  <article v-for="(item, index) in activePanelListItems" :key="panelItemKey(item, index)" class="panel-item">
                    <strong v-if="panelItemLabel(item)">{{ panelItemLabel(item) }}</strong>
                    <span>{{ panelItemValue(item) }}</span>
                  </article>
                </div>

                <pre v-else-if="activePanelRawText" class="panel-raw">{{ activePanelRawText }}</pre>

                <div v-else class="panel-empty">当前 tab 还没有内容。</div>
              </section>
            </slot>
          </div>
        </section>
      </div>
    </transition>
  </teleport>
</template>
<style scoped>
.protocol-evidence-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(15, 23, 42, 0.58);
  backdrop-filter: blur(10px);
}

.protocol-evidence-modal {
  width: 100%;
  min-height: min(520px, calc(100vh - 32px));
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 28px;
  border: 1px solid rgba(148, 163, 184, 0.34);
  background:
    radial-gradient(circle at top left, rgba(14, 165, 233, 0.12), transparent 26%),
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.12), transparent 24%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(248, 250, 252, 0.98));
  box-shadow: 0 28px 64px rgba(15, 23, 42, 0.28);
}

.modal-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 20px 22px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
}

.modal-copy {
  min-width: 0;
}

.modal-kicker {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.modal-copy h3 {
  margin: 10px 0 8px;
  color: #0f172a;
  font-size: 20px;
}

.modal-copy p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
}

.modal-head-actions {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.pin-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 38px;
  padding: 0 14px;
  border-radius: 999px;
  border: 1px solid rgba(14, 165, 233, 0.18);
  background: rgba(255, 255, 255, 0.96);
  color: #0369a1;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
}

.pin-btn:hover {
  background: rgba(240, 249, 255, 0.98);
}

.close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(255, 255, 255, 0.98);
  color: #0f172a;
  cursor: pointer;
  transition: transform 120ms ease, box-shadow 120ms ease;
}

.panel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.panel-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 7px 12px;
  border-radius: 999px;
  border: 1px solid rgba(191, 219, 254, 0.95);
  background: rgba(239, 246, 255, 0.92);
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.close-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 10px 20px rgba(15, 23, 42, 0.08);
}

.modal-tabs {
  display: flex;
  gap: 8px;
  padding: 14px 16px 0;
  overflow: auto;
}

.modal-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
  padding: 10px 14px;
  border-radius: 999px;
  border: 1px solid rgba(226, 232, 240, 0.96);
  background: rgba(248, 250, 252, 0.98);
  color: #475569;
  font-size: 13px;
  font-weight: 800;
  cursor: pointer;
  transition:
    transform 120ms ease,
    border-color 120ms ease,
    box-shadow 120ms ease,
    background 120ms ease;
}

.modal-tab:hover {
  transform: translateY(-1px);
  border-color: rgba(125, 211, 252, 0.8);
}

.modal-tab.active {
  background: white;
  color: #0f172a;
  border-color: rgba(191, 219, 254, 0.98);
  box-shadow: 0 10px 22px rgba(37, 99, 235, 0.1);
}

.modal-tab small {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
  color: #1d4ed8;
  background: rgba(219, 234, 254, 0.82);
}

.modal-body {
  flex: 1 1 auto;
  min-height: 180px;
  overflow: auto;
  padding: 16px 18px 18px;
}

:deep(.panel-fallback) {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

:deep(.panel-hero) {
  padding: 16px;
  border-radius: 20px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(248, 250, 252, 0.92);
}

:deep(.panel-hero h4) {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
}

:deep(.panel-hero p) {
  margin: 8px 0 0;
  color: #475569;
  line-height: 1.65;
}

:deep(.panel-list) {
  display: grid;
  gap: 10px;
}

:deep(.panel-item) {
  padding: 14px 16px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(255, 255, 255, 0.98);
}

:deep(.panel-item strong) {
  display: block;
  margin-bottom: 4px;
  color: #0f172a;
  font-size: 13px;
}

:deep(.panel-item span) {
  color: #334155;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

:deep(.panel-raw) {
  margin: 0;
  padding: 16px;
  border-radius: 18px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.65;
  overflow: auto;
}

:deep(.panel-empty) {
  padding: 18px;
  border-radius: 18px;
  border: 1px dashed rgba(203, 213, 225, 0.95);
  color: #64748b;
  background: rgba(248, 250, 252, 0.85);
}

.protocol-evidence-fade-enter-active,
.protocol-evidence-fade-leave-active {
  transition: opacity 160ms ease;
}

.protocol-evidence-fade-enter-from,
.protocol-evidence-fade-leave-to {
  opacity: 0;
}

@media (max-width: 720px) {
  .protocol-evidence-modal {
    max-width: none !important;
    max-height: none !important;
    height: calc(100vh - 32px);
  }

  .modal-head {
    flex-direction: column;
    padding: 18px 16px 14px;
  }

  .modal-head-actions {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
