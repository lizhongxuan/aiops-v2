<script setup>
import { computed, ref } from "vue";
import {
  ArrowLeftIcon,
  BotIcon,
  CheckCircleIcon,
  FlaskConicalIcon,
  PanelRightIcon,
  PlayIcon,
  RocketIcon,
  SaveIcon,
  XIcon,
} from "lucide-vue-next";
import RunnerCanvas from "./RunnerCanvas.vue";
import NodeRunDetailModal from "./NodeRunDetailModal.vue";
import PublishReviewModal from "./PublishReviewModal.vue";
import RunnerNodePanel from "./RunnerNodePanel.vue";
import RunnerRunPanel from "./RunnerRunPanel.vue";
import RunnerAiAssistantModal from "./ai/RunnerAiAssistantModal.vue";
import WorkflowQuickList from "./WorkflowQuickList.vue";
import { createInitialRunState, reduceRunEvents } from "./runStateReducer";
import "./runnerStudio.css";

const props = defineProps({
  workflows: {
    type: Array,
    default: () => [],
  },
  actions: {
    type: Array,
    default: () => [],
  },
  selectedWorkflowName: {
    type: String,
    default: "",
  },
  loading: {
    type: Boolean,
    default: false,
  },
  workflowUiState: {
    type: Object,
    default: () => ({ recent: [], favorites: [] }),
  },
  runEvents: {
    type: Array,
    default: () => [],
  },
  saveState: {
    type: Object,
    default: () => ({ status: "idle", message: "", lastSavedAt: "", error: "" }),
  },
  serverActionsDisabled: {
    type: Boolean,
    default: false,
  },
  serverActionsDisabledReason: {
    type: String,
    default: "",
  },
});

const emit = defineEmits([
  "update:selectedWorkflowName",
  "update-workflow-graph",
  "create-workflow",
  "open-workflow-manager",
  "toggle-workflow-favorite",
  "open-node-config",
  "node-action",
  "toolbar-action",
  "workflow-published",
]);

const selectedWorkflow = computed(
  () =>
    props.workflows.find((item) => item.name === props.selectedWorkflowName || item.id === props.selectedWorkflowName) ||
    null,
);
const hasSelectedWorkflow = computed(() => Boolean(selectedWorkflow.value));

const selectedWorkflowTitle = computed(
  () => selectedWorkflow.value?.title || selectedWorkflow.value?.name || selectedWorkflow.value?.id || "未选择工作流",
);

const selectedWorkflowStatus = computed(() => selectedWorkflow.value?.status || "draft");
const saveStateLabel = computed(() => {
  const state = props.saveState || {};
  if (state.status === "pending") return state.message || "未保存";
  if (state.status === "saving") return state.message || "正在保存";
  if (state.status === "saved") return state.lastSavedAt ? `已保存 ${state.lastSavedAt}` : state.message || "已保存";
  if (state.status === "local_draft") return state.message || "本地草稿，未同步";
  if (state.status === "blocked") return state.message || "操作被阻止";
  if (state.status === "failed" || state.status === "error") return state.message || "操作失败";
  if (state.status === "conflict") return state.message || "保存冲突";
  return state.message || "草稿";
});
const saveStateTitle = computed(() => {
  const state = props.saveState || {};
  return state.error || state.message || saveStateLabel.value;
});
const saveStateError = computed(() => String(props.saveState?.error || "").trim());
const selectedNodeId = ref("");
const configNodeId = ref("");
const detailNodeId = ref("");
const aiAssistantOpen = ref(false);
const isFullscreen = ref(false);
const publishReviewOpen = ref(false);
const runDrawerOpen = ref(false);
const canvasGraph = computed(
  () =>
    selectedWorkflow.value?.graph || {
      version: "v1",
      workflow: { name: props.selectedWorkflowName || "draft" },
      nodes: [],
      edges: [],
    },
);
const configNode = computed(() => canvasGraph.value.nodes?.find((node) => node.id === configNodeId.value) || null);
const runState = computed(() => reduceRunEvents(props.runEvents, createInitialRunState()));
const publishDiffSummary = computed(() => selectedWorkflow.value?.diff_summary || selectedWorkflow.value?.diffSummary || {});
const publishRiskSummary = computed(() => selectedWorkflow.value?.risk_summary || selectedWorkflow.value?.riskSummary || {});
const publishValidationResult = computed(
  () =>
    selectedWorkflow.value?.validation_result ||
    selectedWorkflow.value?.validationResult || {
      valid: selectedWorkflowStatus.value === "validated" || selectedWorkflowStatus.value === "published",
      warnings: [],
      errors: [],
    },
);
const isRunActive = computed(() => {
  if (["queued", "running"].includes(runState.value.status)) return true;
  return Object.values(runState.value.nodes || {}).some((node) => ["queued", "running"].includes(node.status));
});
const serverActionKeys = new Set(["dry-run", "run", "stop-run", "publish"]);
const serverActionDisabledReason = computed(
  () => props.serverActionsDisabledReason || "Runner Studio API 不可用，运行和发布暂不可用；保存会落本地草稿。",
);
const toolbarActions = computed(() => [
  { key: "save", label: "保存", icon: SaveIcon },
  { key: "validate", label: "校验", icon: CheckCircleIcon },
  { key: "dry-run", label: "Dry Run", icon: FlaskConicalIcon },
  isRunActive.value
    ? { key: "stop-run", label: "停止运行", icon: XIcon, danger: true }
    : { key: "run", label: "运行", icon: PlayIcon, primary: true },
  { key: "run-details", label: "运行详情", icon: PanelRightIcon },
  { key: "publish", label: "发布", icon: RocketIcon },
  { key: "ai-generate", label: "AI 生成", icon: BotIcon },
].map((action) => {
  if (!serverActionKeys.has(action.key) || !props.serverActionsDisabled) {
    return action;
  }
  return {
    ...action,
    serverBlocked: true,
    disabledReason: serverActionDisabledReason.value,
  };
}));

function selectNode(nodeId) {
  selectedNodeId.value = nodeId;
  configNodeId.value = nodeId || "";
}

function openNodeConfig(nodeId) {
  selectedNodeId.value = nodeId;
  configNodeId.value = nodeId;
  emit("open-node-config", nodeId);
}

function applyNodeConfig(node) {
  const nextGraph = {
    ...canvasGraph.value,
    workflow: { ...(canvasGraph.value.workflow || {}) },
    nodes: (canvasGraph.value.nodes || []).map((item) => (item.id === node.id ? node : item)),
    edges: [...(canvasGraph.value.edges || [])],
  };
  emit("update-workflow-graph", nextGraph);
  selectedNodeId.value = node.id;
  configNodeId.value = node.id;
}

function closeNodePanel() {
  selectedNodeId.value = "";
  configNodeId.value = "";
}

function runSingleNode(nodeId) {
  emit("node-action", "run-node", nodeId);
}

function openRunDetailsForNode(nodeId) {
  selectedNodeId.value = nodeId;
  configNodeId.value = nodeId || configNodeId.value;
  runDrawerOpen.value = true;
}

function handleToolbarAction(actionKey) {
  if (actionKey === "run-details") {
    runDrawerOpen.value = true;
    return;
  }
  if (actionKey === "ai-generate") {
    aiAssistantOpen.value = true;
  }
  if (actionKey === "publish") {
    publishReviewOpen.value = true;
  }
  emit("toolbar-action", actionKey);
}

function applyAiPatch(payload) {
  if (payload?.graph) {
    emit("update-workflow-graph", payload.graph);
  }
  aiAssistantOpen.value = false;
}

function handleWorkflowPublished(payload) {
  emit("workflow-published", payload);
  publishReviewOpen.value = false;
}
</script>

<template>
  <section
    class="runner-studio-shell"
    :class="{ fullscreen: isFullscreen }"
    :aria-busy="loading ? 'true' : 'false'"
    data-testid="runner-studio-shell"
  >
    <section v-if="!hasSelectedWorkflow" class="runner-workflow-library" data-testid="runner-workflow-library">
      <header class="runner-workflow-library-head">
        <div>
          <p>RUNNER WORKFLOWS</p>
          <h1>工作流</h1>
        </div>
      </header>
      <WorkflowQuickList
        :workflows="workflows"
        :selected-workflow-name="selectedWorkflowName"
        :ui-state="workflowUiState"
        :loading="loading"
        show-all
        @select="emit('update:selectedWorkflowName', $event)"
        @toggle-favorite="emit('toggle-workflow-favorite', $event)"
        @open-manager="emit('open-workflow-manager')"
        @create="emit('create-workflow')"
      />
    </section>

    <template v-else>
    <header class="runner-studio-topbar" data-testid="runner-studio-topbar">
      <div class="runner-studio-current-workflow">
        <button
          type="button"
          class="runner-studio-back-button"
          data-testid="runner-back-to-library"
          @click="runDrawerOpen = false; isFullscreen = false; emit('update:selectedWorkflowName', '')"
        >
          <ArrowLeftIcon :size="15" />
          <span>工作流</span>
        </button>
        <h1>{{ selectedWorkflowTitle }}</h1>
        <span class="runner-studio-status">{{ selectedWorkflowStatus }}</span>
        <span
          class="runner-studio-save-state"
          :class="`status-${saveState.status || 'idle'}`"
          :title="saveStateTitle"
          data-testid="runner-save-state"
        >
          {{ saveStateLabel }}
        </span>
      </div>
      <div class="runner-studio-toolbar-actions" aria-label="Runner Studio 操作">
        <span
          v-if="(saveState.status || 'idle') !== 'idle'"
          class="runner-toolbar-save-feedback"
          :class="`status-${saveState.status || 'idle'}`"
          :title="saveStateTitle"
          data-testid="runner-toolbar-save-feedback"
          aria-live="polite"
        >
          <span>{{ saveStateLabel }}</span>
          <small v-if="saveStateError" data-testid="runner-toolbar-save-error">{{ saveStateError }}</small>
        </span>
        <button
          v-for="action in toolbarActions"
          :key="action.key"
          type="button"
          class="runner-studio-action-button"
          :class="{ primary: action.primary, danger: action.danger, 'server-blocked': action.serverBlocked }"
          :data-testid="`runner-toolbar-${action.key}`"
          :disabled="loading || action.disabled || (saveState.status === 'saving' && action.key === 'save')"
          :title="action.disabledReason || (saveState.status === 'saving' && action.key === 'save' ? saveStateTitle : action.label)"
          @click="handleToolbarAction(action.key)"
        >
          <component :is="action.icon" :size="15" />
          <span>{{ action.label }}</span>
        </button>
      </div>
    </header>

    <div class="runner-studio-workspace" :class="{ 'with-node-panel': Boolean(configNode) }">
      <section class="runner-studio-main">
        <section class="runner-studio-canvas" aria-label="工作流画布" data-testid="runner-studio-canvas">
          <RunnerCanvas
            :graph="canvasGraph"
            :actions="actions"
            :selected-node-id="selectedNodeId"
            :fullscreen="isFullscreen"
            @update:graph="emit('update-workflow-graph', $event)"
            @select-node="selectNode"
            @open-node-config="openNodeConfig"
            @node-action="(...args) => emit('node-action', ...args)"
            @toggle-fullscreen="isFullscreen = !isFullscreen"
          />
        </section>
      </section>
      <RunnerNodePanel
        v-if="configNode"
        :node="configNode"
        :graph="canvasGraph"
        :actions="actions"
        :run-state="runState"
        @close="closeNodePanel"
        @apply="applyNodeConfig"
        @run-node="runSingleNode"
        @open-run-details="openRunDetailsForNode"
        @update:graph="emit('update-workflow-graph', $event)"
        @locate-node="selectNode"
      />
    </div>
    </template>

    <section
      v-if="hasSelectedWorkflow && runDrawerOpen"
      class="runner-studio-run-drawer-backdrop"
      role="dialog"
      aria-modal="true"
      aria-label="运行详情"
      data-testid="runner-run-drawer"
      @click.self="runDrawerOpen = false"
    >
      <aside class="runner-studio-run-drawer-panel">
        <header class="runner-studio-run-drawer-head">
          <div>
            <strong>运行详情</strong>
            <span>stdout、stderr、SSE、审批事件、变量和最近节点结果</span>
          </div>
          <button
            type="button"
            class="runner-run-drawer-close"
            data-testid="runner-run-drawer-close"
            aria-label="关闭运行详情"
            @click="runDrawerOpen = false"
          >
            <XIcon :size="18" />
          </button>
        </header>
        <div class="runner-studio-run-drawer-body">
          <RunnerRunPanel
            :state="runState"
            :selected-node-id="selectedNodeId"
            :graph="canvasGraph"
            @select-node="selectNode"
            @open-node-detail="detailNodeId = $event"
          />
        </div>
      </aside>
    </section>

    <NodeRunDetailModal
      :show="Boolean(detailNodeId)"
      :node-id="detailNodeId"
      :state="runState"
      @close="detailNodeId = ''"
    />
    <RunnerAiAssistantModal
      :show="aiAssistantOpen"
      :workflow="selectedWorkflow || { name: selectedWorkflowName, status: selectedWorkflowStatus }"
      :graph="canvasGraph"
      @close="aiAssistantOpen = false"
      @apply-patch="applyAiPatch"
    />
    <PublishReviewModal
      :show="publishReviewOpen"
      :workflow="selectedWorkflow || { name: selectedWorkflowName, status: selectedWorkflowStatus }"
      :diff-summary="publishDiffSummary"
      :risk-summary="publishRiskSummary"
      :validation-result="publishValidationResult"
      @close="publishReviewOpen = false"
      @published="handleWorkflowPublished"
    />
  </section>
</template>
