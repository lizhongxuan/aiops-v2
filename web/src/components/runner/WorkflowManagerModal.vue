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
const createIntent = ref("");
const workflowName = ref("");

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

function slugBaseForName(value) {
  const text = String(value || "").trim();
  if (/检查/.test(text) && /主机/.test(text) && /资源/.test(text)) {
    return "host-resource-check";
  }
  const ascii = text
    .normalize("NFKD")
    .toLowerCase()
    .replace(/['"]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return ascii || "runner-workflow";
}

function uniqueWorkflowSlug(value) {
  const used = new Set(props.workflows.map((workflow) => workflowKey(workflow)).filter(Boolean));
  const base = slugBaseForName(value);
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

const createWorkflowSlug = computed(() => uniqueWorkflowSlug(workflowName.value));
const canSubmitCreate = computed(() => workflowName.value.trim().length > 0);

function beginCreateWorkflow(mode) {
  if (mode === "blank" || mode === "template") {
    createIntent.value = mode;
    workflowName.value = "";
    return;
  }
  emit("create-workflow", mode);
}

function cancelCreateWorkflow() {
  createIntent.value = "";
  workflowName.value = "";
}

function submitCreateWorkflow() {
  if (!canSubmitCreate.value || !createIntent.value) return;
  const title = workflowName.value.trim();
  emit("create-workflow", {
    mode: createIntent.value,
    name: title,
    title,
    slug: createWorkflowSlug.value,
  });
  cancelCreateWorkflow();
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
          @click="beginCreateWorkflow(mode.key)"
        >
          {{ mode.label }}
        </button>
      </section>

      <form
        v-if="createIntent"
        class="workflow-create-form"
        data-testid="workflow-create-form"
        @submit.prevent="submitCreateWorkflow"
      >
        <div>
          <strong>新建工作流</strong>
          <span>{{ createIntent === "template" ? "从模板创建前先命名" : "先命名，再进入编排画布" }}</span>
        </div>
        <label>
          工作流名称
          <input
            v-model="workflowName"
            type="text"
            placeholder="例如：检查主机资源"
            data-testid="workflow-create-name"
            autofocus
          />
        </label>
        <p>路径：/runner/{{ createWorkflowSlug }}</p>
        <div>
          <button type="button" class="secondary" @click="cancelCreateWorkflow">取消</button>
          <button type="submit" :disabled="!canSubmitCreate" data-testid="workflow-create-submit" @click="submitCreateWorkflow">
            创建并打开
          </button>
        </div>
      </form>

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
