<script setup>
import { computed, ref } from "vue";
import { SearchIcon, XIcon } from "lucide-vue-next";
import { getActionIdentity, isActionRecommendedAfterPort, sortActionsForSourcePort } from "./nodeTypeRegistry";

const props = defineProps({
  actions: {
    type: Array,
    default: () => [],
  },
  compact: {
    type: Boolean,
    default: false,
  },
  sourcePort: {
    type: String,
    default: "",
  },
  recentActionKeys: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["select", "close"]);
const query = ref("");

const filteredActions = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  const candidates = sortActionsForSourcePort(props.actions, props.sourcePort || "next");
  if (!keyword) return candidates;
  return candidates.filter((action) => {
    const text = `${actionLabel(action)} ${action.action || ""} ${action.category || ""} ${action.description || ""}`.toLowerCase();
    return text.includes(keyword);
  });
});

const recommendedActions = computed(() => {
  if (!props.sourcePort) return [];
  return filteredActions.value.filter((action) => isActionRecommendedAfterPort(action, props.sourcePort));
});

const recentActions = computed(() => {
  const recommendedKeys = new Set(recommendedActions.value.map((action) => getActionIdentity(action)));
  const actionByIdentity = new Map(filteredActions.value.map((action) => [getActionIdentity(action), action]));
  return props.recentActionKeys
    .map((key) => actionByIdentity.get(String(key)))
    .filter((action) => action && !recommendedKeys.has(getActionIdentity(action)));
});

const allActions = computed(() => {
  const usedKeys = new Set([...recommendedActions.value, ...recentActions.value].map((action) => getActionIdentity(action)));
  return filteredActions.value.filter((action) => !usedKeys.has(getActionIdentity(action)));
});

const pickerSections = computed(() => [
  { key: "recommended", title: "推荐节点", actions: recommendedActions.value },
  { key: "recent", title: "最近使用", actions: recentActions.value },
  { key: "all", title: props.sourcePort ? "其他节点" : "全部节点", actions: allActions.value },
].filter((section) => section.actions.length > 0));

function actionKey(action) {
  return String(action.action || action.name || "action").replace(/[^a-zA-Z0-9_-]+/g, "-");
}

function actionLabel(action) {
  return action.label || action.title || action.name || action.action || "Action";
}

function actionCategory(action) {
  return action.category || "动作";
}

function actionDescription(action) {
  return action.description || action.action || action.name || "添加到当前工作流";
}

function startActionDrag(event, action) {
  if (!event.dataTransfer) return;
  event.dataTransfer.effectAllowed = "copy";
  event.dataTransfer.setData("application/runner-action", JSON.stringify(action));
}
</script>

<template>
  <section class="runner-node-picker" :class="{ compact }" data-testid="runner-node-picker">
    <div class="runner-node-picker-head">
      <label class="runner-node-picker-search">
        <SearchIcon :size="16" />
        <input v-model="query" type="search" placeholder="搜索节点" aria-label="搜索节点" />
      </label>
      <button
        type="button"
        class="runner-node-picker-close"
        data-testid="runner-node-picker-close"
        aria-label="关闭节点选择器"
        title="关闭"
        @click="emit('close')"
      >
        <XIcon :size="15" />
      </button>
    </div>
    <div class="runner-node-picker-list">
      <section
        v-for="section in pickerSections"
        :key="section.key"
        class="runner-node-picker-section"
        :data-testid="`node-picker-section-${section.key}`"
      >
        <h3>{{ section.title }}</h3>
        <button
          v-for="action in section.actions"
          :key="`${section.key}-${actionKey(action)}`"
          type="button"
          class="runner-action-palette-item"
          draggable="true"
          :data-testid="`catalog-action-${actionKey(action)}`"
          @dragstart="startActionDrag($event, action)"
          @click="emit('select', action)"
        >
          <span class="runner-action-category">{{ actionCategory(action) }}</span>
          <strong>{{ actionLabel(action) }}</strong>
          <small>{{ actionDescription(action) }}</small>
        </button>
      </section>
      <p v-if="filteredActions.length === 0" class="runner-node-picker-empty">没有匹配的节点。</p>
    </div>
  </section>
</template>
