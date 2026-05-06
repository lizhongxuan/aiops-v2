<script setup>
import { computed, ref, watch } from "vue";
import { CheckIcon, PanelRightIcon, PlayIcon, XIcon } from "lucide-vue-next";
import AdvancedTab from "./node-config/AdvancedTab.vue";
import InputTab from "./node-config/InputTab.vue";
import RunAiTab from "./node-config/RunAiTab.vue";
import { getNodeCanvasMeta } from "./nodeTypeRegistry";
import { collectRunnerVariables } from "./runnerVariables";
import RunnerNextStepEditor from "./RunnerNextStepEditor.vue";
import RunnerSchemaEditor from "./RunnerSchemaEditor.vue";
import ActionNodePanel from "./panels/ActionNodePanel.vue";
import ApprovalNodePanel from "./panels/ApprovalNodePanel.vue";
import ConditionNodePanel from "./panels/ConditionNodePanel.vue";
import NotifyNodePanel from "./panels/NotifyNodePanel.vue";

const props = defineProps({
  node: {
    type: Object,
    default: null,
  },
  actions: {
    type: Array,
    default: () => [],
  },
  runState: {
    type: Object,
    default: () => ({ nodes: {} }),
  },
  graph: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(["close", "apply", "run-node", "open-run-details", "update:graph", "locate-node"]);

const activeTab = ref("settings");
const draftNode = ref(null);

const tabs = [
  { key: "settings", label: "设置" },
  { key: "input", label: "输入" },
  { key: "output", label: "输出" },
  { key: "advanced", label: "高级" },
  { key: "last-run", label: "上次运行" },
];

const nodeMeta = computed(() => getNodeCanvasMeta(draftNode.value || props.node || {}));
const settingsPanel = computed(() => {
  const node = draftNode.value || props.node || {};
  const type = String(node.type || "").toLowerCase();
  const action = String(node.step?.action || "").toLowerCase();
  if (type === "condition" || action === "condition.branch" || action === "condition.evaluate") return "condition";
  if (type === "manual_approval" || type === "approval" || action === "manual.approval" || action === "approval.wait") {
    return "approval";
  }
  if (nodeMeta.value.key === "notify" || action.startsWith("notify.")) return "notify";
  return "action";
});
const title = computed(
  () => draftNode.value?.step?.name || draftNode.value?.label || draftNode.value?.id || "节点配置",
);
const runNodeState = computed(() => props.runState?.nodes?.[draftNode.value?.id || props.node?.id] || null);
const availableVariables = computed(() => {
  if (!props.graph || !draftNode.value?.id) return [];
  return collectRunnerVariables(props.graph, draftNode.value.id);
});
const isStartNode = computed(() => String(draftNode.value?.type || "").toLowerCase() === "start");
const workflowInputs = computed(() => props.graph?.workflow?.inputs || []);
const runStatus = computed(() => runNodeState.value?.status || "not_run");
const durationText = computed(() => {
  const duration = runNodeState.value?.durationMs;
  if (typeof duration !== "number") return "";
  if (duration >= 1000) return `${Math.round(duration / 100) / 10}s`;
  return `${duration}ms`;
});

function cloneNode(node) {
  return node ? JSON.parse(JSON.stringify(node)) : null;
}

watch(
  () => props.node,
  (node) => {
    draftNode.value = cloneNode(node);
    activeTab.value = "settings";
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

function updateGraphWithDraftNode(graph) {
  if (!graph || !draftNode.value?.id) {
    emit("update:graph", graph);
    return;
  }
  emit("update:graph", {
    ...graph,
    workflow: { ...(graph.workflow || {}) },
    nodes: (graph.nodes || []).map((node) => (node.id === draftNode.value.id ? cloneNode(draftNode.value) : node)),
    edges: [...(graph.edges || [])],
  });
}

function runSingleNode() {
  if (!draftNode.value?.id) return;
  emit("run-node", draftNode.value.id);
}

function openRunDetails() {
  if (!draftNode.value?.id) return;
  emit("open-run-details", draftNode.value.id);
}

function updateWorkflowInputs(inputs) {
  if (!props.graph) return;
  emit("update:graph", {
    ...props.graph,
    workflow: {
      ...(props.graph.workflow || {}),
      inputs,
    },
    nodes: [...(props.graph.nodes || [])],
    edges: [...(props.graph.edges || [])],
  });
}
</script>

<template>
  <aside
    v-if="draftNode"
    class="runner-node-panel"
    role="complementary"
    aria-label="节点配置面板"
    data-testid="runner-node-panel"
  >
    <header class="runner-node-panel-head">
      <div class="runner-node-panel-identity">
        <span class="runner-node-panel-icon" :class="`tone-${nodeMeta.tone}`">{{ nodeMeta.iconText }}</span>
        <div>
          <p>{{ nodeMeta.action }}</p>
          <h2 data-testid="runner-node-panel-title">{{ title }}</h2>
          <span class="runner-node-panel-status" :class="`status-${runStatus}`">
            {{ runStatus }}<template v-if="durationText"> · {{ durationText }}</template>
          </span>
        </div>
      </div>
      <div class="runner-node-panel-actions">
        <button type="button" data-testid="runner-node-panel-run" @click="runSingleNode">
          <PlayIcon :size="15" />
          <span>运行</span>
        </button>
        <button type="button" data-testid="runner-node-panel-open-run" @click="openRunDetails">
          <PanelRightIcon :size="15" />
          <span>详情</span>
        </button>
        <button type="button" class="primary" data-testid="runner-node-panel-apply" @click="applyChanges">
          <CheckIcon :size="15" />
          <span>应用</span>
        </button>
        <button type="button" aria-label="关闭节点配置" data-testid="runner-node-panel-close" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </div>
    </header>

    <nav class="runner-node-panel-tabs" aria-label="节点配置页签" data-testid="runner-node-panel-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        :class="{ active: activeTab === tab.key }"
        :data-testid="`runner-node-panel-tab-${tab.key}`"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </nav>

    <main class="runner-node-panel-body">
      <template v-if="activeTab === 'settings'">
        <ConditionNodePanel
          v-if="settingsPanel === 'condition'"
          :node="draftNode"
          :variables="availableVariables"
          @update:node="updateDraft"
          @locate-node="emit('locate-node', $event)"
        />
        <ApprovalNodePanel
          v-else-if="settingsPanel === 'approval'"
          :node="draftNode"
          :variables="availableVariables"
          @update:node="updateDraft"
          @locate-node="emit('locate-node', $event)"
        />
        <NotifyNodePanel
          v-else-if="settingsPanel === 'notify'"
          :node="draftNode"
          :variables="availableVariables"
          @update:node="updateDraft"
          @locate-node="emit('locate-node', $event)"
        />
        <ActionNodePanel
          v-else
          :node="draftNode"
          :actions="actions"
          :variables="availableVariables"
          @update:node="updateDraft"
          @locate-node="emit('locate-node', $event)"
        />
      </template>
      <RunnerSchemaEditor
        v-else-if="activeTab === 'input' && isStartNode"
        mode="inputs"
        title="工作流输入 Schema"
        :inputs="workflowInputs"
        @update:inputs="updateWorkflowInputs"
      />
      <InputTab
        v-else-if="activeTab === 'input'"
        :node="draftNode"
        :variables="availableVariables"
        @update:node="updateDraft"
      />
      <RunnerSchemaEditor
        v-else-if="activeTab === 'output'"
        mode="outputs"
        title="节点输出 Schema"
        :outputs="draftNode.outputs || []"
        @update:outputs="updateDraft({ ...draftNode, outputs: $event })"
      />
      <AdvancedTab v-else-if="activeTab === 'advanced'" :node="draftNode" @update:node="updateDraft" />
      <RunAiTab v-else :node="draftNode" />
    </main>

    <RunnerNextStepEditor
      v-if="graph"
      :node="draftNode"
      :graph="graph"
      @update:graph="updateGraphWithDraftNode"
    />
  </aside>
</template>
