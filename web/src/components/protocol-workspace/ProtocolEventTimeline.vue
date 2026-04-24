<script setup>
import { computed } from "vue";
import { ChevronRightIcon, Clock3Icon } from "lucide-vue-next";

defineOptions({
  inheritAttrs: false,
});

const props = defineProps({
  items: {
    type: Array,
    default: () => [],
  },
  title: {
    type: String,
    default: "实时事件",
  },
  subtitle: {
    type: String,
    default: "轻量时间线只呈现关键变化，便于快速扫一眼当前动态。",
  },
  emptyLabel: {
    type: String,
    default: "当前还没有可展示的实时事件。",
  },
  maxItems: {
    type: Number,
    default: 0,
  },
  activeItemId: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["select"]);

const timelineItems = computed(() => {
  const list = Array.isArray(props.items) ? props.items : [];
  const mapped = list.map((item, index) => normalizeItem(item, index));
  return props.maxItems > 0 ? mapped.slice(0, props.maxItems) : mapped;
});

function normalizeItem(item, index) {
  const source = item && typeof item === "object" ? item : {};
  return {
    id: String(source.id || source.eventId || source.key || index),
    time: String(source.time || source.at || source.createdAt || source.updatedAt || ""),
    title: String(source.title || source.label || source.name || source.type || "事件"),
    text: String(source.text || source.summary || source.detail || source.message || "点击查看上下文"),
    tone: toneClass(source.tone || source.status || source.level || source.variant),
    source: String(source.source || source.origin || source.channel || "system"),
    host: String(source.host || source.hostName || source.hostId || ""),
    tag: String(source.tag || source.category || ""),
    raw: source,
  };
}

function toneClass(value) {
  const tone = String(value || "neutral").toLowerCase();
  if (tone.includes("success") || tone.includes("done") || tone.includes("complete")) return "success";
  if (tone.includes("warning") || tone.includes("wait") || tone.includes("pending")) return "warning";
  if (tone.includes("danger") || tone.includes("fail") || tone.includes("error")) return "danger";
  if (tone.includes("info")) return "info";
  return "neutral";
}

function handleSelect(item) {
  emit("select", item.raw || item);
}
</script>

<template>
  <section class="protocol-event-timeline" data-testid="protocol-event-timeline">
    <header class="timeline-head">
      <div>
        <span class="timeline-kicker">EVENT STREAM</span>
        <h3>{{ title }}</h3>
        <p>{{ subtitle }}</p>
      </div>
    </header>

    <div v-if="timelineItems.length" class="timeline-list">
      <button
        v-for="item in timelineItems"
        :key="item.id"
        type="button"
        class="timeline-item"
        :class="[item.tone, { active: item.id === activeItemId }]"
        :data-testid="`protocol-event-${item.id}`"
        @click="handleSelect(item)"
      >
        <div class="timeline-rail">
          <span class="timeline-dot"></span>
          <span class="timeline-line"></span>
        </div>

        <div class="timeline-copy">
          <div class="timeline-row">
            <div class="timeline-title">
              <strong>{{ item.title }}</strong>
            </div>
            <span v-if="item.time" class="timeline-time">
              <Clock3Icon size="12" />
              <span>{{ item.time }}</span>
            </span>
          </div>
          <p v-if="item.text">{{ item.text }}</p>
        </div>

        <ChevronRightIcon size="14" class="timeline-arrow" />
      </button>
    </div>

    <div v-else class="timeline-empty">{{ emptyLabel }}</div>
  </section>
</template>

<style scoped>
.protocol-event-timeline {
  height: 100%;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-top: none;
  padding: 0;
  border-radius: 0;
  border-left: none;
  border-right: none;
  border-bottom: none;
  background: #ffffff;
  box-shadow: none;
}

.timeline-head {
  padding: 10px 14px;
  flex-shrink: 0;
}

.timeline-head h3 {
  margin: 2px 0 0;
  color: #1e293b;
  font-size: 14px;
  line-height: 1.2;
}

.timeline-head p {
  display: none;
}

.timeline-kicker {
  display: inline-flex;
  align-items: center;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #0ea5e9;
}

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  flex: 1;
  overflow-y: auto;
  padding: 0 14px;
}

.timeline-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  padding: 6px 0;
  border: none;
  border-bottom: 1px solid #f5f5f5;
  border-radius: 0;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition: background 0.1s;
}

.timeline-item:last-child {
  border-bottom: none;
}

.timeline-item:hover {
  background: #f8fafc;
  transform: none;
  box-shadow: none;
}

.timeline-item.active {
  background: rgba(239, 246, 255, 0.88);
}

.timeline-item.active .timeline-title strong {
  color: #1d4ed8;
}

.timeline-rail {
  position: relative;
  display: flex;
  justify-content: center;
  padding-top: 5px;
  flex-shrink: 0;
}

.timeline-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #3b82f6;
  box-shadow: none;
}

.timeline-line {
  display: none;
}

.timeline-item.success .timeline-dot { background: #22c55e; }
.timeline-item.warning .timeline-dot { background: #f59e0b; }
.timeline-item.danger .timeline-dot { background: #ef4444; }
.timeline-item.info .timeline-dot { background: #3b82f6; }

.timeline-copy {
  min-width: 0;
  flex: 1;
}

.timeline-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: flex-start;
}

.timeline-title {
  min-width: 0;
}

.timeline-title strong {
  font-size: 12px;
  font-weight: 600;
  color: #334155;
}

.timeline-title span {
  font-size: 11px;
  color: #94a3b8;
  margin-left: 4px;
}

.timeline-meta {
  display: none;
}

.timeline-time {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #94a3b8;
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  white-space: nowrap;
  flex-shrink: 0;
}

.timeline-copy p {
  margin: 2px 0 0;
  color: #475569;
  font-size: 11px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.timeline-item.warning .timeline-copy p {
  color: #d97706;
  font-weight: 500;
}

.timeline-item.danger .timeline-copy p {
  color: #dc2626;
  font-weight: 500;
}

.timeline-arrow {
  display: none;
}

.timeline-empty {
  padding: 0 14px 14px;
  color: #94a3b8;
  background: transparent;
  font-size: 12px;
  font-weight: 600;
}
</style>
