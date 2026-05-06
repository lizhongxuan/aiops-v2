<script setup>
import { computed, ref, watch } from "vue";
import { XIcon } from "lucide-vue-next";
import AdvancedTab from "./node-config/AdvancedTab.vue";
import BasicTab from "./node-config/BasicTab.vue";
import InputTab from "./node-config/InputTab.vue";
import OutputTab from "./node-config/OutputTab.vue";
import RunAiTab from "./node-config/RunAiTab.vue";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  node: {
    type: Object,
    default: null,
  },
  actions: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["close", "apply"]);

const activeTab = ref("basic");
const draftNode = ref(null);

const tabs = [
  { key: "basic", label: "基础" },
  { key: "input", label: "输入" },
  { key: "output", label: "输出" },
  { key: "advanced", label: "高级" },
  { key: "run-ai", label: "运行与 AI" },
];

const title = computed(() => draftNode.value?.step?.name || draftNode.value?.label || draftNode.value?.id || "节点配置");

function cloneNode(node) {
  return node ? JSON.parse(JSON.stringify(node)) : null;
}

watch(
  () => props.node,
  (node) => {
    draftNode.value = cloneNode(node);
    activeTab.value = "basic";
  },
  { immediate: true },
);

function updateDraft(node) {
  draftNode.value = cloneNode(node);
}

function applyChanges() {
  if (!draftNode.value) return;
  emit("apply", cloneNode(draftNode.value));
}
</script>

<template>
  <section v-if="show && draftNode" class="node-config-backdrop">
    <div class="node-config-modal" role="dialog" aria-modal="true" data-testid="node-config-modal">
      <header class="node-config-head">
        <div>
          <p>NODE CONFIG</p>
          <h2>{{ title }}</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>

      <nav class="node-config-tabs" data-testid="node-config-tabs" aria-label="节点配置页签">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          :class="{ active: activeTab === tab.key }"
          :data-testid="`tab-${tab.key}`"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </nav>

      <main class="node-config-body">
        <BasicTab
          v-if="activeTab === 'basic'"
          :node="draftNode"
          :actions="actions"
          @update:node="updateDraft"
        />
        <InputTab v-else-if="activeTab === 'input'" :node="draftNode" @update:node="updateDraft" />
        <OutputTab v-else-if="activeTab === 'output'" :node="draftNode" @update:node="updateDraft" />
        <AdvancedTab v-else-if="activeTab === 'advanced'" :node="draftNode" @update:node="updateDraft" />
        <RunAiTab v-else :node="draftNode" />
      </main>

      <footer class="node-config-footer">
        <button type="button" @click="emit('close')">取消</button>
        <button type="button" class="primary" data-testid="node-config-apply" @click="applyChanges">应用</button>
      </footer>
    </div>
  </section>
</template>
