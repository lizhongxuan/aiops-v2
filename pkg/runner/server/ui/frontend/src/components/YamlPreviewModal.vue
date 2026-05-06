<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NAlert, NButton, NModal, NSpace, NTabPane, NTabs, NTag } from "naive-ui";
import CodeEditor from "./CodeEditor.vue";
import type { WorkflowGraph } from "../types/workflow";
import { buildGraphDiffSummary } from "../utils/graphDiff";
import { formatGraphJSON, graphPreviewText } from "../utils/graphPreview";

const props = defineProps<{
  show: boolean;
  graph: WorkflowGraph | null;
  baselineGraph?: WorkflowGraph | null;
  yamlPreview: string;
  previewCompiling?: boolean;
}>();

const emit = defineEmits<{
  "update:show": [show: boolean];
  "replace-graph": [graph: WorkflowGraph];
  "compile-yaml": [];
  "parse-yaml": [yaml: string];
}>();

const activeTab = ref("preview");
const yamlEdit = ref("");
const yamlEditError = ref<string | null>(null);
const graphJSON = ref(formatGraphJSON(props.graph));
const graphJSONError = ref<string | null>(null);

const previewText = computed(() => graphPreviewText(props.graph, props.yamlPreview));
const previewLanguage = computed(() => (props.yamlPreview.trim() ? "yaml" : "yaml"));
const previewLabel = computed(() => (props.yamlPreview.trim() ? "Compiled YAML" : "Graph YAML-like"));
const diffSummary = computed(() => buildGraphDiffSummary(props.baselineGraph, props.graph));

watch(
  () => props.show,
  (show) => {
    if (!show) return;
    activeTab.value = "preview";
    refreshYamlEdit();
    refreshGraphJSON();
  },
);

watch(
  () => [props.graph, props.yamlPreview] as const,
  () => {
    if (activeTab.value !== "yaml") refreshYamlEdit();
    if (activeTab.value !== "graph-json") refreshGraphJSON();
  },
  { deep: true },
);

function close() {
  emit("update:show", false);
}

function refreshGraphJSON() {
  graphJSON.value = formatGraphJSON(props.graph);
  graphJSONError.value = null;
}

function refreshYamlEdit() {
  yamlEdit.value = props.yamlPreview.trim() ? props.yamlPreview : "";
  yamlEditError.value = null;
}

function updateYamlEdit(value: string) {
  yamlEdit.value = value;
  yamlEditError.value = null;
}

function applyYamlEdit() {
  const yaml = yamlEdit.value.trim();
  if (!yaml) {
    yamlEditError.value = "Compile the graph first, or paste workflow YAML.";
    return;
  }
  emit("parse-yaml", yaml);
  yamlEditError.value = null;
}

function updateGraphJSON(value: string) {
  graphJSON.value = value;
  graphJSONError.value = null;
}

function applyGraphJSON() {
  try {
    const parsed = JSON.parse(graphJSON.value) as WorkflowGraph;
    if (!parsed || typeof parsed !== "object" || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) {
      throw new Error("Graph JSON must include nodes[] and edges[].");
    }
    emit("replace-graph", parsed);
    graphJSONError.value = null;
  } catch (error) {
    graphJSONError.value = error instanceof Error ? error.message : "Invalid graph JSON.";
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    class="yaml-preview-modal"
    title="Workflow YAML"
    :bordered="false"
    @update:show="emit('update:show', $event)"
  >
    <NTabs v-model:value="activeTab" type="line" animated>
      <NTabPane name="preview" tab="Preview">
        <div class="modal-tab-heading">
          <NTag size="small" :bordered="false">{{ previewLabel }}</NTag>
          <small v-if="previewCompiling">Compiling latest graph...</small>
          <small v-else-if="!yamlPreview.trim()">Editing graph; compiled YAML will refresh automatically when API is available.</small>
        </div>
        <CodeEditor :model-value="previewText" :language="previewLanguage" readonly height="520px" />
      </NTabPane>

      <NTabPane name="yaml" tab="YAML">
        <div class="modal-tab-heading">
          <NTag size="small" :bordered="false">Editable workflow YAML</NTag>
          <small>Applies through the server YAML parser into the current graph store.</small>
        </div>
        <CodeEditor
          :model-value="yamlEdit"
          language="yaml"
          height="520px"
          placeholder="version: v0.1&#10;name: workflow&#10;steps: []"
          @update:model-value="updateYamlEdit"
        />
        <NAlert v-if="yamlEditError" class="editor-alert" type="error" :bordered="false">
          {{ yamlEditError }}
        </NAlert>
        <NSpace justify="end" class="modal-actions">
          <NButton secondary @click="emit('compile-yaml')">Compile latest graph</NButton>
          <NButton secondary @click="refreshYamlEdit">Reset</NButton>
          <NButton type="primary" @click="applyYamlEdit">Apply YAML to graph</NButton>
        </NSpace>
      </NTabPane>

      <NTabPane name="diff" tab="Diff">
        <div class="modal-tab-heading">
          <NTag size="small" :bordered="false">Change review</NTag>
          <small>{{ diffSummary.changed ? "Review changes before save, dry-run, or run." : "No graph changes from the loaded baseline." }}</small>
        </div>
        <div class="diff-section-grid">
          <section
            v-for="section in diffSummary.sections"
            :key="section.kind"
            class="diff-section-card"
            :class="{ 'diff-section-card--changed': section.changed }"
          >
            <div class="diff-section-heading">
              <strong>{{ section.title }}</strong>
              <NTag size="small" :type="section.changed ? 'warning' : 'success'" :bordered="false">
                {{ section.changed ? "Changed" : "Clean" }}
              </NTag>
            </div>
            <ul v-if="section.paths.length" class="diff-path-list">
              <li v-for="path in section.paths.slice(0, 16)" :key="path">
                <code>{{ path }}</code>
              </li>
            </ul>
            <p v-else class="diff-empty">No changes in this category.</p>
          </section>
        </div>
      </NTabPane>

      <NTabPane name="graph-json" tab="Graph JSON">
        <div class="modal-tab-heading">
          <NTag size="small" :bordered="false">Advanced editor</NTag>
          <small>Applies directly to the current graph store.</small>
        </div>
        <CodeEditor
          :model-value="graphJSON"
          language="json"
          height="520px"
          placeholder="{ &quot;version&quot;: &quot;v1&quot;, &quot;nodes&quot;: [], &quot;edges&quot;: [] }"
          @update:model-value="updateGraphJSON"
        />
        <NAlert v-if="graphJSONError" class="editor-alert" type="error" :bordered="false">
          {{ graphJSONError }}
        </NAlert>
        <NSpace justify="end" class="modal-actions">
          <NButton secondary @click="refreshGraphJSON">Reset</NButton>
          <NButton type="primary" @click="applyGraphJSON">Apply graph JSON</NButton>
        </NSpace>
      </NTabPane>
    </NTabs>

    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">Close</NButton>
      </NSpace>
    </template>
  </NModal>
</template>
