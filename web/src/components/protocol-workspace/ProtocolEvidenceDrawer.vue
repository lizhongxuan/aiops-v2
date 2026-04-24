<script setup>
import { computed } from "vue";
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
    default: "证据抽屉",
  },
  kicker: {
    type: String,
    default: "EVIDENCE DRAWER",
  },
  subtitle: {
    type: String,
    default: "把当前重细节内容固定到侧边抽屉，方便边看边对照主线程。",
  },
  tabs: {
    type: Array,
    default: () => [],
  },
  panels: {
    type: Object,
    default: () => ({}),
  },
  offsetRight: {
    type: String,
    default: "16px",
  },
  testId: {
    type: String,
    default: "protocol-evidence-drawer",
  },
});

const emit = defineEmits(["close", "update:open", "update:activeTab", "switch", "action"]);

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
const activePanel = computed(() => {
  if (!currentTab.value) return normalizePanel({});
  return normalizePanel(props.panels?.[currentTab.value.value] || props.panels?.[currentTab.value.value.replace(/-/g, "_")] || {});
});

const activePanelListItems = computed(() => panelListItems(activePanel.value));
const activePanelSections = computed(() => (Array.isArray(activePanel.value?.sections) ? activePanel.value.sections : []));
const activePanelRawText = computed(() => panelRawText(activePanel.value));

function normalizePanel(panel) {
  if (!panel || typeof panel !== "object") {
    return {
      title: "",
      summary: "",
      items: [],
      raw: "",
      actions: [],
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

function panelListItems(panel = {}) {
  const source = panel || {};
  if (Array.isArray(source.items) && source.items.length) return source.items;
  if (Array.isArray(source.lines) && source.lines.length) return source.lines;
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

function selectTab(value) {
  if (!value || value === props.activeTab) return;
  emit("update:activeTab", value);
  emit("switch", value);
}

function requestClose() {
  emit("close");
  emit("update:open", false);
}

function triggerAction(action) {
  if (!action || typeof action !== "object") return;
  emit("action", action);
}
</script>

<template>
  <transition name="protocol-evidence-drawer-slide">
    <aside v-if="open" class="protocol-evidence-drawer" :style="{ '--drawer-offset-right': offsetRight }" :data-testid="testId">
      <header class="drawer-head">
        <div class="drawer-copy">
          <span class="drawer-kicker">{{ kicker }}</span>
          <h3>{{ title }}</h3>
          <p>{{ subtitle }}</p>
        </div>
        <button class="drawer-close" type="button" @click="requestClose">
          <XIcon size="18" />
        </button>
      </header>

      <nav class="drawer-tabs" aria-label="证据抽屉分区">
        <button
          v-for="tab in normalizedTabs"
          :key="tab.value"
          type="button"
          class="drawer-tab"
          :class="{ active: tab.value === activeTab }"
          @click="selectTab(tab.value)"
        >
          <span>{{ tab.label }}</span>
          <small v-if="tab.badge !== undefined">{{ tab.badge }}</small>
        </button>
      </nav>

      <div class="drawer-body">
        <section class="drawer-panel">
          <div v-if="activePanel?.title || activePanel?.summary" class="drawer-panel-hero">
            <h4 v-if="activePanel?.title">{{ activePanel.title }}</h4>
            <p v-if="activePanel?.summary">{{ activePanel.summary }}</p>
            <div v-if="activePanel.actions?.length" class="drawer-panel-actions">
              <button
                v-for="action in activePanel.actions"
                :key="action.id || action.kind || action.label"
                type="button"
                class="drawer-panel-action"
                @click="triggerAction(action)"
              >
                {{ action.label || action.title || action.kind }}
              </button>
            </div>
          </div>

          <div v-if="activePanelListItems.length" class="drawer-panel-list">
            <article v-for="(item, index) in activePanelListItems" :key="panelItemKey(item, index)" class="drawer-panel-item">
              <strong v-if="panelItemLabel(item)">{{ panelItemLabel(item) }}</strong>
              <span>{{ panelItemValue(item) }}</span>
            </article>
          </div>

          <div v-if="activePanelSections.length" class="drawer-panel-sections">
            <article v-for="(section, index) in activePanelSections" :key="section.id || section.kind || index" class="drawer-panel-section">
              <strong>{{ section.title || section.kind || `Section ${index + 1}` }}</strong>
              <span>{{ section.summary || `${Array.isArray(section.cards) ? section.cards.length : 0} 个卡片` }}</span>
            </article>
          </div>

          <pre v-if="activePanelRawText" class="drawer-panel-raw">{{ activePanelRawText }}</pre>

          <div v-if="!activePanelListItems.length && !activePanelSections.length && !activePanelRawText" class="drawer-panel-empty">
            当前抽屉还没有内容。
          </div>
        </section>
      </div>
    </aside>
  </transition>
</template>

<style scoped>
.protocol-evidence-drawer {
  position: fixed;
  top: 88px;
  right: var(--drawer-offset-right, 16px);
  bottom: 16px;
  width: min(440px, calc(100vw - 32px));
  z-index: 1100;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 24px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background:
    radial-gradient(circle at top right, rgba(14, 165, 233, 0.12), transparent 26%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(248, 250, 252, 0.98));
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.22);
}

.drawer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 18px 14px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
}

.drawer-copy h3 {
  margin: 8px 0 6px;
  color: #0f172a;
  font-size: 18px;
}

.drawer-copy p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
  font-size: 13px;
}

.drawer-kicker {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.drawer-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(255, 255, 255, 0.98);
  color: #0f172a;
  cursor: pointer;
}

.drawer-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.86);
}

.drawer-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  border-radius: 999px;
  border: 1px solid rgba(226, 232, 240, 0.95);
  background: rgba(255, 255, 255, 0.98);
  color: #334155;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.drawer-tab.active {
  border-color: rgba(59, 130, 246, 0.9);
  background: rgba(239, 246, 255, 0.95);
  color: #1d4ed8;
}

.drawer-body {
  min-height: 0;
  flex: 1;
  overflow: auto;
  padding: 18px;
}

.drawer-panel {
  display: grid;
  gap: 14px;
}

.drawer-panel-hero h4 {
  margin: 0 0 6px;
  color: #0f172a;
  font-size: 16px;
}

.drawer-panel-hero p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
  font-size: 13px;
}

.drawer-panel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.drawer-panel-action {
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

.drawer-panel-list {
  display: grid;
  gap: 10px;
}

.drawer-panel-sections {
  display: grid;
  gap: 10px;
}

.drawer-panel-item {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(255, 255, 255, 0.92);
}

.drawer-panel-item strong {
  color: #64748b;
  font-size: 12px;
}

.drawer-panel-item span {
  color: #0f172a;
  line-height: 1.6;
  word-break: break-word;
}

.drawer-panel-section {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(248, 250, 252, 0.94);
}

.drawer-panel-section strong {
  color: #0f172a;
  font-size: 13px;
}

.drawer-panel-section span {
  color: #475569;
  line-height: 1.6;
  font-size: 12px;
}

.drawer-panel-raw {
  margin: 0;
  padding: 14px 16px;
  border-radius: 16px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  overflow: auto;
}

.drawer-panel-empty {
  padding: 18px;
  border-radius: 16px;
  border: 1px dashed rgba(226, 232, 240, 0.95);
  color: #64748b;
  text-align: center;
}

.protocol-evidence-drawer-slide-enter-active,
.protocol-evidence-drawer-slide-leave-active {
  transition: transform 180ms ease, opacity 180ms ease;
}

.protocol-evidence-drawer-slide-enter-from,
.protocol-evidence-drawer-slide-leave-to {
  transform: translateX(24px);
  opacity: 0;
}

@media (max-width: 720px) {
  .protocol-evidence-drawer {
    top: 72px;
    right: 12px;
    left: 12px;
    bottom: 12px;
    width: auto;
  }
}
</style>
