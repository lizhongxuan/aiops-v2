<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NAlert, NButton, NForm, NFormItem, NInput, NModal, NSelect, NSpace, NTag } from "naive-ui";
import { runnerApi } from "../api/client";
import type { WorkflowGraph, WorkflowSummary } from "../types/workflow";
import { createWorkflowGraphFromTemplate, prepareWorkflowGraphForCreate, type WorkflowTemplateKind } from "../utils/workflowTemplates";

type CreateMode = WorkflowTemplateKind | "from-yaml" | "clone-current";

const props = withDefaults(
  defineProps<{
    show: boolean;
    workflows: WorkflowSummary[];
    currentGraph: WorkflowGraph | null;
    creating?: boolean;
    error?: string | null;
    initialMode?: CreateMode;
  }>(),
  {
    creating: false,
    error: null,
    initialMode: "cmd-run-basic",
  },
);

const emit = defineEmits<{
  "update:show": [value: boolean];
  create: [payload: { graph: WorkflowGraph; labels?: Record<string, string>; saveNote?: string }];
}>();

const mode = ref<CreateMode>("cmd-run-basic");
const name = ref("new-workflow");
const version = ref("v0.1");
const description = ref("");
const labelsText = ref("source=visual-ui");
const saveNote = ref("initial visual workflow draft");
const yamlText = ref("");
const parsedYamlGraph = ref<WorkflowGraph | null>(null);
const parseError = ref("");
const parsing = ref(false);
const validationError = ref("");
const validating = ref(false);

const modeOptions = [
  { label: "Command", value: "cmd-run-basic" },
  { label: "Shell Script", value: "shell-run-basic" },
  { label: "Manual Approval", value: "manual-approval-basic" },
  { label: "From YAML", value: "from-yaml" },
  { label: "Clone Current", value: "clone-current" },
];

const duplicateName = computed(() => props.workflows.some((workflow) => workflow.name === name.value.trim()));
const invalidName = computed(() => !/^[a-zA-Z0-9._-]+$/.test(name.value.trim()));
const canCreate = computed(() => Boolean(buildGraph()) && !duplicateName.value && !invalidName.value && !validationError.value && !props.creating && !validating.value);

watch(
  () => props.show,
  (show) => {
    if (show) resetForm(props.initialMode);
  },
);

watch(
  () => props.initialMode,
  (next) => {
    if (props.show) mode.value = next;
  },
);

watch([mode, name, version, description, labelsText, yamlText], () => {
  validationError.value = "";
});

async function parseYaml() {
  const raw = yamlText.value.trim();
  parsedYamlGraph.value = null;
  parseError.value = "";
  if (!raw) {
    parseError.value = "YAML is required.";
    return;
  }
  parsing.value = true;
  try {
    const graph = await runnerApi.parseGraphYAML(raw);
    parsedYamlGraph.value = graph;
    if (!name.value.trim() || name.value === "new-workflow") {
      name.value = graph.workflow.name || name.value;
    }
    if (graph.workflow.description && !description.value.trim()) {
      description.value = graph.workflow.description;
    }
  } catch (error) {
    parseError.value = error instanceof Error ? error.message : "Unable to parse YAML.";
  } finally {
    parsing.value = false;
  }
}

async function submit() {
  const graph = buildGraph();
  if (!graph || duplicateName.value || invalidName.value) return;
  validationError.value = "";
  validating.value = true;
  try {
    const result = await runnerApi.validateGraph(graph);
    if (!result.valid) {
      validationError.value = result.errors[0]?.message || result.summary || "Workflow graph validation failed.";
      return;
    }
  } catch (error) {
    validationError.value = error instanceof Error ? error.message : "Workflow graph validation failed.";
    return;
  } finally {
    validating.value = false;
  }
  emit("create", {
    graph,
    labels: parseLabels(labelsText.value),
    saveNote: saveNote.value.trim() || undefined,
  });
}

function buildGraph(): WorkflowGraph | null {
  const metadata = {
    name: name.value.trim(),
    version: version.value.trim() || "v0.1",
    description: description.value,
  };
  if (!metadata.name) return null;
  if (mode.value === "from-yaml") {
    return parsedYamlGraph.value ? prepareWorkflowGraphForCreate(parsedYamlGraph.value, metadata) : null;
  }
  if (mode.value === "clone-current") {
    return props.currentGraph ? prepareWorkflowGraphForCreate(props.currentGraph, metadata) : null;
  }
  return createWorkflowGraphFromTemplate({
    kind: mode.value,
    ...metadata,
  });
}

function close() {
  emit("update:show", false);
}

function resetForm(nextMode: CreateMode) {
  mode.value = nextMode;
  name.value = nextMode === "clone-current" && props.currentGraph ? `${props.currentGraph.workflow.name}-copy` : "new-workflow";
  version.value = props.currentGraph?.workflow.version || "v0.1";
  description.value = "";
  labelsText.value = "source=visual-ui";
  saveNote.value = "initial visual workflow draft";
  yamlText.value = "";
  parsedYamlGraph.value = null;
  parseError.value = "";
  validationError.value = "";
}

function parseLabels(raw: string) {
  const labels: Record<string, string> = {};
  for (const part of raw.split(/[\n,]+/)) {
    const trimmed = part.trim();
    if (!trimmed) continue;
    const separator = trimmed.indexOf("=");
    if (separator <= 0) continue;
    const key = trimmed.slice(0, separator).trim();
    const value = trimmed.slice(separator + 1).trim();
    if (key && value) labels[key] = value;
  }
  return Object.keys(labels).length > 0 ? labels : undefined;
}
</script>

<template>
  <NModal :show="show" preset="card" title="New Workflow" class="new-workflow-dialog" @update:show="emit('update:show', $event)">
    <NForm class="new-workflow-form" label-placement="top">
      <NFormItem label="Template">
        <NSelect v-model:value="mode" :options="modeOptions" />
      </NFormItem>

      <div class="new-workflow-grid">
        <NFormItem label="Name" :feedback="duplicateName ? 'Workflow name already exists.' : invalidName ? 'Use letters, numbers, dot, dash, or underscore.' : ''" :validation-status="duplicateName || invalidName ? 'error' : undefined">
          <NInput v-model:value="name" placeholder="workflow-name" />
        </NFormItem>
        <NFormItem label="Version">
          <NInput v-model:value="version" placeholder="v0.1" />
        </NFormItem>
      </div>

      <NFormItem label="Description">
        <NInput v-model:value="description" type="textarea" placeholder="What this workflow does" />
      </NFormItem>

      <NFormItem label="Labels">
        <NInput v-model:value="labelsText" type="textarea" placeholder="source=visual-ui" />
      </NFormItem>

      <NFormItem v-if="mode === 'from-yaml'" label="YAML">
        <div class="yaml-create-block">
          <NInput v-model:value="yamlText" type="textarea" placeholder="Paste workflow YAML" />
          <NSpace align="center">
            <NButton secondary :loading="parsing" @click="parseYaml">Parse YAML</NButton>
            <NTag v-if="parsedYamlGraph" type="success" size="small">Parsed</NTag>
          </NSpace>
          <NAlert v-if="parseError" type="error">{{ parseError }}</NAlert>
        </div>
      </NFormItem>

      <NAlert v-if="mode === 'clone-current' && !currentGraph" type="warning">Load a workflow before cloning.</NAlert>
      <NAlert v-if="validationError" type="error">{{ validationError }}</NAlert>
      <NAlert v-if="error" type="error">{{ error }}</NAlert>

      <NFormItem label="Save note">
        <NInput v-model:value="saveNote" placeholder="initial visual workflow draft" />
      </NFormItem>
    </NForm>

    <template #footer>
      <div class="modal-actions">
        <NButton secondary @click="close">Cancel</NButton>
        <NButton type="primary" :loading="creating || validating" :disabled="!canCreate" @click="submit">Create workflow</NButton>
      </div>
    </template>
  </NModal>
</template>
