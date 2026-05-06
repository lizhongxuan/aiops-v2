<script setup>
import { computed, ref } from "vue";
import { ArchiveIcon, CopyIcon, GitBranchIcon, StarIcon, XIcon } from "lucide-vue-next";
import {
  createWorkflowManagerState,
  filterWorkflowManagerItems,
  workflowKey,
} from "./workflowManagerState";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
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
  dirty: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits([
  "close",
  "select",
  "request-dirty-confirm",
  "toggle-favorite",
  "create-workflow",
  "clone-workflow",
  "archive-workflow",
  "view-versions",
]);

const query = ref("");
const status = ref("");
const includeArchived = ref(false);

const normalizedState = computed(() => createWorkflowManagerState(props.uiState));
const filteredWorkflows = computed(() =>
  filterWorkflowManagerItems(props.workflows, {
    query: query.value,
    status: status.value,
    includeArchived: includeArchived.value,
  }),
);

const createModes = [
  { key: "blank", label: "空白" },
  { key: "template", label: "模板" },
  { key: "yaml", label: "YAML 导入" },
  { key: "clone", label: "克隆" },
  { key: "ai", label: "AI 生成" },
];

function isFavorite(name) {
  return normalizedState.value.favorites.includes(name);
}

function selectWorkflow(name) {
  if (!name || name === props.selectedWorkflowName) return;
  if (props.dirty) {
    emit("request-dirty-confirm", name);
    return;
  }
  emit("select", name);
}
</script>

<template>
  <section v-if="show" class="workflow-manager-backdrop">
    <div class="workflow-manager-modal" role="dialog" aria-modal="true" data-testid="workflow-manager-modal">
      <header class="workflow-manager-head">
        <div>
          <p>WORKFLOW MANAGER</p>
          <h2>工作流管理</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>

      <section class="workflow-create-strip" aria-label="新建工作流">
        <button
          v-for="mode in createModes"
          :key="mode.key"
          type="button"
          :data-testid="`workflow-create-${mode.key}`"
          @click="emit('create-workflow', mode.key)"
        >
          {{ mode.label }}
        </button>
      </section>

      <section class="workflow-manager-controls">
        <input
          v-model="query"
          type="search"
          placeholder="搜索工作流"
          data-testid="workflow-manager-search"
        />
        <select v-model="status" data-testid="workflow-manager-status-filter">
          <option value="">全部状态</option>
          <option value="draft">draft</option>
          <option value="validated">validated</option>
          <option value="published">published</option>
          <option value="archived">archived</option>
        </select>
        <label>
          <input v-model="includeArchived" type="checkbox" data-testid="workflow-manager-include-archived" />
          包含归档
        </label>
      </section>

      <section class="workflow-manager-list" aria-label="完整工作流列表">
        <article
          v-for="workflow in filteredWorkflows"
          :key="workflowKey(workflow)"
          class="workflow-manager-row"
          :class="{ active: workflowKey(workflow) === selectedWorkflowName }"
        >
          <button
            type="button"
            class="workflow-manager-main"
            :data-testid="`workflow-select-${workflowKey(workflow)}`"
            @click="selectWorkflow(workflowKey(workflow))"
          >
            <span>{{ workflow.title || workflow.name || workflow.id }}</span>
            <small>{{ workflow.name || workflow.id }} · {{ workflow.status || "draft" }}</small>
          </button>
          <div class="workflow-manager-row-actions">
            <button
              type="button"
              class="workflow-icon-button"
              :class="{ active: isFavorite(workflowKey(workflow)) }"
              :data-testid="`workflow-favorite-${workflowKey(workflow)}`"
              @click="emit('toggle-favorite', workflowKey(workflow))"
            >
              <StarIcon :size="15" />
            </button>
            <button
              type="button"
              class="workflow-icon-button"
              :data-testid="`workflow-clone-${workflowKey(workflow)}`"
              @click="emit('clone-workflow', workflowKey(workflow))"
            >
              <CopyIcon :size="15" />
            </button>
            <button
              type="button"
              class="workflow-icon-button"
              :data-testid="`workflow-archive-${workflowKey(workflow)}`"
              @click="emit('archive-workflow', workflowKey(workflow))"
            >
              <ArchiveIcon :size="15" />
            </button>
            <button
              type="button"
              class="workflow-icon-button"
              :data-testid="`workflow-versions-${workflowKey(workflow)}`"
              @click="emit('view-versions', workflowKey(workflow))"
            >
              <GitBranchIcon :size="15" />
            </button>
          </div>
        </article>
      </section>
    </div>
  </section>
</template>
