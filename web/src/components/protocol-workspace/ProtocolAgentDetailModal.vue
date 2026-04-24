<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ActivityIcon, BotIcon, ListTodoIcon, MessageSquareIcon, ShieldCheckIcon, XIcon } from "lucide-vue-next";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  open: {
    type: Boolean,
    default: false,
  },
  agent: {
    type: Object,
    default: () => ({}),
  },
  width: {
    type: String,
    default: "min(1180px, calc(100vw - 32px))",
  },
  maxHeight: {
    type: String,
    default: "min(88vh, 920px)",
  },
});

const emit = defineEmits(["close", "update:open"]);
const bodyOverflow = ref("");

function text(value) {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  return String(value).trim();
}

function stringifyRaw(value) {
  if (!value) return "";
  if (typeof value === "string") return value;
  if (Array.isArray(value)) return value.map((item) => stringifyRaw(item)).filter(Boolean).join("\n");
  if (typeof value === "object") {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }
  return String(value);
}

function normalizeItems(items) {
  return (Array.isArray(items) ? items : [])
    .map((item, index) => {
      if (item == null) {
        return null;
      }
      if (typeof item === "string") {
        const value = text(item);
        return value ? { id: `row-${index}`, label: "", value, time: "" } : null;
      }
      const label = text(item.label || item.key || item.title || item.name || item.id || "");
      const value = text(item.value ?? item.text ?? item.summary ?? item.content ?? "");
      const time = text(item.time || item.updatedAt || item.createdAt || "");
      const detail = text(item.detail || item.note || "");
      const raw = item.raw ?? item;
      const resolvedValue = value || detail || stringifyRaw(raw);
      if (!label && !resolvedValue && !time) {
        return null;
      }
      return {
        id: text(item.id || item.key || label || `row-${index}`),
        label,
        value: resolvedValue,
        time,
        raw,
      };
    })
    .filter(Boolean);
}

function normalizeSection(section, index) {
  const raw = section?.raw ?? null;
  const items = normalizeItems(section?.items);
  return {
    id: text(section?.key || section?.id || `section-${index}`),
    title: text(section?.title || section?.label || `Section ${index + 1}`),
    summary: text(section?.summary || section?.description || ""),
    items,
    raw,
    rawText: stringifyRaw(raw),
  };
}

const normalizedAgent = computed(() => {
  const source = props.agent && typeof props.agent === "object" ? props.agent : {};
  const sections = Array.isArray(source.sections) ? source.sections.map(normalizeSection) : [];
  return {
    ...source,
    id: text(source.id || source.hostId || "agent"),
    title: text(source.title || source.name || source.displayName || source.hostId || "background agent"),
    subtitle: text(source.subtitle || source.summary || source.taskTitle || ""),
    statusLabel: text(source.statusLabel || source.status || "idle"),
    statusKey: text(source.statusKey || source.status || "idle"),
    overviewItems: normalizeItems(source.overviewItems || source.overview || []),
    sections,
  };
});

const sectionMeta = computed(() => [
  {
    key: "task",
    icon: ListTodoIcon,
    label: "分配任务信息",
    count: normalizedAgent.value.sections.find((section) => section.id === "task")?.items?.length || 0,
  },
  {
    key: "conversation",
    icon: MessageSquareIcon,
    label: "与 AI 的对话信息",
    count: normalizedAgent.value.sections.find((section) => section.id === "conversation")?.items?.length || 0,
  },
  {
    key: "approval",
    icon: ShieldCheckIcon,
    label: "审核信息",
    count: normalizedAgent.value.sections.find((section) => section.id === "approval")?.items?.length || 0,
  },
  {
    key: "activity",
    icon: ActivityIcon,
    label: "当前状态 / 最近活动",
    count: normalizedAgent.value.sections.find((section) => section.id === "activity")?.items?.length || 0,
  },
]);

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

function requestClose() {
  emit("close");
  emit("update:open", false);
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
    <transition name="protocol-agent-detail-fade">
      <div v-if="open" class="protocol-agent-detail-backdrop" @click.self="onBackdropClick">
        <section class="protocol-agent-detail-modal" :style="{ maxWidth: width, maxHeight }" role="dialog" aria-modal="true">
          <header class="detail-head">
            <div class="detail-copy">
              <span class="detail-kicker">BACKGROUND AGENT</span>
              <h3>{{ normalizedAgent.title }}</h3>
              <p>{{ normalizedAgent.subtitle || "点击该 agent 可以查看任务、对话、审核和最近活动。" }}</p>
            </div>
            <button class="close-btn" type="button" @click="requestClose">
              <XIcon size="18" />
            </button>
          </header>

          <div class="detail-topline">
            <article class="status-card tone">
              <span class="status-label">状态</span>
              <strong>{{ normalizedAgent.statusLabel }}</strong>
              <small>{{ normalizedAgent.id }}</small>
            </article>
            <article
              v-for="chip in sectionMeta"
              :key="chip.key"
              class="status-card"
            >
              <component :is="chip.icon" size="14" />
              <span class="status-label">{{ chip.label }}</span>
              <strong>{{ chip.count }}</strong>
            </article>
          </div>

          <div class="detail-body">
            <section class="overview-block">
              <div class="overview-head">
                <BotIcon size="14" />
                <strong>Agent 概览</strong>
              </div>
              <div v-if="normalizedAgent.overviewItems.length" class="overview-grid">
                <article v-for="item in normalizedAgent.overviewItems" :key="item.id" class="overview-card">
                  <span>{{ item.label }}</span>
                  <strong>{{ item.value }}</strong>
                  <small v-if="item.time">{{ item.time }}</small>
                </article>
              </div>
              <p v-else class="overview-empty">当前没有可用的 agent 概览数据。</p>
            </section>

            <section v-for="section in normalizedAgent.sections" :key="section.id" class="detail-section">
              <header class="section-head">
                <div>
                  <h4>{{ section.title }}</h4>
                  <p>{{ section.summary || "暂无摘要。" }}</p>
                </div>
              </header>

              <div v-if="section.items.length" class="section-list">
                <article v-for="item in section.items" :key="item.id" class="section-item">
                  <div class="section-item-head">
                    <strong>{{ item.label || "详情" }}</strong>
                    <small v-if="item.time">{{ item.time }}</small>
                  </div>
                  <p>{{ item.value }}</p>
                </article>
              </div>
              <p v-else class="section-empty">当前 section 没有足够的数据。</p>

              <details v-if="section.rawText" class="section-raw">
                <summary>原始数据</summary>
                <pre>{{ section.rawText }}</pre>
              </details>
            </section>
          </div>
        </section>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.protocol-agent-detail-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1250;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(15, 23, 42, 0.58);
  backdrop-filter: blur(10px);
}

.protocol-agent-detail-modal {
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 28px;
  border: 1px solid rgba(148, 163, 184, 0.34);
  background:
    radial-gradient(circle at top left, rgba(14, 165, 233, 0.14), transparent 26%),
    radial-gradient(circle at top right, rgba(244, 114, 182, 0.12), transparent 22%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.99), rgba(248, 250, 252, 0.98));
  box-shadow: 0 28px 64px rgba(15, 23, 42, 0.28);
}

.detail-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 20px 22px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.92);
}

.detail-copy {
  min-width: 0;
}

.detail-kicker {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.detail-copy h3 {
  margin: 10px 0 8px;
  color: #0f172a;
  font-size: 20px;
}

.detail-copy p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
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

.close-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 10px 20px rgba(15, 23, 42, 0.08);
}

.detail-topline {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  padding: 14px 16px 0;
}

.status-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(255, 255, 255, 0.96);
  color: #0f172a;
  box-shadow: 0 8px 22px rgba(15, 23, 42, 0.04);
}

.status-card.tone {
  background: linear-gradient(180deg, rgba(224, 242, 254, 0.95), rgba(255, 255, 255, 0.96));
}

.status-card :deep(svg) {
  color: #0ea5e9;
}

.status-label {
  color: #64748b;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.status-card strong {
  font-size: 14px;
  line-height: 1.4;
}

.status-card small {
  color: #94a3b8;
  font-size: 11px;
  line-height: 1.4;
  word-break: break-all;
}

.detail-body {
  min-height: 0;
  overflow: auto;
  padding: 16px 18px 18px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.overview-block,
.detail-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border-radius: 22px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(255, 255, 255, 0.96);
}

.overview-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.overview-head strong {
  font-size: 14px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.overview-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  background: rgba(248, 250, 252, 0.92);
}

.overview-card span,
.overview-card small {
  color: #64748b;
  font-size: 11px;
}

.overview-card strong {
  color: #0f172a;
  font-size: 13px;
  line-height: 1.45;
  word-break: break-word;
}

.overview-empty,
.section-empty {
  margin: 0;
  color: #64748b;
  line-height: 1.65;
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.section-head h4 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
}

.section-head p {
  margin: 6px 0 0;
  color: #64748b;
  line-height: 1.6;
}

.section-list {
  display: grid;
  gap: 10px;
}

.section-item {
  padding: 14px 16px;
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(255, 255, 255, 0.98);
}

.section-item-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.section-item-head strong {
  color: #0f172a;
  font-size: 13px;
}

.section-item-head small {
  color: #94a3b8;
  font-size: 11px;
  flex: none;
}

.section-item p {
  margin: 8px 0 0;
  color: #334155;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.section-raw {
  padding: 0 2px;
}

.section-raw summary {
  cursor: pointer;
  color: #0ea5e9;
  font-size: 12px;
  font-weight: 700;
}

.section-raw pre {
  margin: 10px 0 0;
  padding: 14px;
  border-radius: 16px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  overflow: auto;
}

.protocol-agent-detail-fade-enter-active,
.protocol-agent-detail-fade-leave-active {
  transition: opacity 160ms ease;
}

.protocol-agent-detail-fade-enter-from,
.protocol-agent-detail-fade-leave-to {
  opacity: 0;
}

@media (max-width: 1024px) {
  .detail-topline,
  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .protocol-agent-detail-modal {
    max-width: none !important;
    max-height: none !important;
    height: calc(100vh - 32px);
  }

  .detail-head {
    padding: 18px 16px 14px;
  }

  .detail-topline,
  .overview-grid {
    grid-template-columns: 1fr;
  }

  .detail-body {
    padding: 14px 14px 16px;
  }
}
</style>
