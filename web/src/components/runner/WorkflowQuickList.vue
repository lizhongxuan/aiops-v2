<script setup>
import { computed } from "vue";
import { FolderOpenIcon, PlusIcon, StarIcon } from "lucide-vue-next";
import { createWorkflowManagerState, getQuickWorkflows, workflowKey } from "./workflowManagerState";

const props = defineProps({
  workflows: {
    type: Array,
    default: () => [],
  },
  selectedWorkflowName: {
    type: String,
    default: "",
  },
  uiState: {
    type: Object,
    default: () => createWorkflowManagerState(),
  },
  loading: {
    type: Boolean,
    default: false,
  },
  showAll: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["select", "toggle-favorite", "open-manager", "create"]);

const normalizedState = computed(() => createWorkflowManagerState(props.uiState));
const quickWorkflows = computed(() => getQuickWorkflows(props.workflows, normalizedState.value));
const visibleWorkflows = computed(() => (props.showAll ? props.workflows : quickWorkflows.value));

function isFavorite(name) {
  return normalizedState.value.favorites.includes(name);
}
</script>

<template>
  <div class="workflow-quick-list">
    <div class="runner-studio-panel-head">
      <span>工作流</span>
      <div class="workflow-quick-actions">
        <button type="button" :disabled="loading" data-testid="runner-open-manager" @click="emit('open-manager')">
          <FolderOpenIcon :size="14" />
          <span>管理</span>
        </button>
        <button type="button" :disabled="loading" data-testid="runner-create-workflow" @click="emit('create')">
          <PlusIcon :size="14" />
          <span>新建</span>
        </button>
      </div>
    </div>

    <div v-if="visibleWorkflows.length === 0" class="runner-studio-empty">
      {{ showAll ? "暂无工作流，点击新建开始编排。" : "暂无最近或收藏工作流" }}
    </div>
    <div
      v-for="workflow in visibleWorkflows"
      :key="workflowKey(workflow)"
      class="runner-studio-workflow-row"
      :class="{ active: workflowKey(workflow) === selectedWorkflowName }"
    >
      <button
        type="button"
        class="runner-studio-workflow"
        :data-testid="`runner-workflow-${workflowKey(workflow)}`"
        @click="emit('select', workflowKey(workflow))"
      >
        <span>{{ workflow.title || workflow.name || workflow.id }}</span>
        <small>{{ workflow.status || "draft" }}</small>
      </button>
      <button
        type="button"
        class="workflow-favorite-button"
        :class="{ active: isFavorite(workflowKey(workflow)) }"
        :data-testid="`runner-favorite-${workflowKey(workflow)}`"
        @click="emit('toggle-favorite', workflowKey(workflow))"
      >
        <StarIcon :size="14" />
      </button>
    </div>
  </div>
</template>
