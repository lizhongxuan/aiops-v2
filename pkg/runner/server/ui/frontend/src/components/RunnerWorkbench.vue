<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NCheckbox, NInput, NModal, NSelect, NTag } from "naive-ui";
import { CheckCircle2, Copy, Download, FileCode2, FileUp, History, Play, Plus, RefreshCw, RotateCcw, Save, Send, Square, UploadCloud } from "lucide-vue-next";
import ActionCatalog from "./ActionCatalog.vue";
import WorkflowCanvas from "./WorkflowCanvas.vue";
import PropertyPanel from "./PropertyPanel.vue";
import RunDrawer from "./RunDrawer.vue";
import YamlPreviewModal from "./YamlPreviewModal.vue";
import NewWorkflowModal from "./NewWorkflowModal.vue";
import { useGraphStore } from "../stores/graphStore";
import { buildGraphDiffSummary } from "../utils/graphDiff";
import type { WorkflowGraph } from "../types/workflow";

type NewWorkflowMode = "cmd-run-basic" | "shell-run-basic" | "manual-approval-basic" | "from-yaml" | "clone-current";

const store = useGraphStore();
const graphWithRunState = store.graphWithRunState;
const selectedNode = store.selectedNode;
const executionSemanticsChanged = store.executionSemanticsChanged;
const waitingApprovalNodes = store.waitingApprovalNodes;
const workflowTitle = computed(() => store.state.graph?.workflow.name || "workflow");
const workflowVersionText = computed(() => store.state.graph?.workflow.version || "unversioned");
const graphDiffSummary = computed(() => buildGraphDiffSummary(store.state.baselineGraph, store.state.graph));
const validationType = computed(() => (store.state.validation?.valid ? "success" : store.state.validation ? "error" : "default"));
const workflowStatusType = computed(() => (store.state.workflowStatus === "published" ? "success" : "warning"));
const editStatusType = computed(() => (store.state.dirty ? "warning" : "success"));
const editStatusText = computed(() => (store.state.dirty ? "unsaved draft" : "saved"));
const runStatusType = computed(() => {
  if (["running", "queued", "waiting"].includes(store.state.run.status)) return "info";
  if (store.state.run.status === "success") return "success";
  if (["failed", "canceled", "cancelled", "interrupted"].includes(store.state.run.status)) return "error";
  return "default";
});
const validationText = computed(() => {
  if (!store.state.validation) return store.state.dirty ? "draft" : "loaded";
  return store.state.validation.valid ? "valid" : `${store.state.validation.errors.length} errors`;
});
const canCancel = computed(() => ["queued", "running", "waiting"].includes(store.state.run.status));
const hasRiskWarning = computed(() => Boolean(store.state.dryRun?.warnings.some((warning) => warning.type === "dry_run_risk")));
const hasNonRiskWarning = computed(() => Boolean(store.state.dryRun?.warnings.some((warning) => warning.type !== "dry_run_risk")));
const yamlPreviewOpen = ref(false);
const historyOpen = ref(false);
const newWorkflowOpen = ref(false);
const newWorkflowInitialMode = ref<NewWorkflowMode>("cmd-run-basic");
const switchConfirmOpen = ref(false);
const pendingWorkflowName = ref("");
const bundleFileInput = ref<HTMLInputElement | null>(null);
const workflowSelectValue = computed(() => store.state.graph?.workflow.name || "");
const workflowSelectOptions = computed(() =>
  [...store.state.workflowOptions]
    .sort((a, b) => (b.updated_at || "").localeCompare(a.updated_at || "") || a.name.localeCompare(b.name))
    .map((workflow) => ({
      label: [workflow.name, workflow.version, workflow.status].filter(Boolean).join(" · "),
      value: workflow.name,
    })),
);

async function openHistory() {
  historyOpen.value = true;
  await store.loadWorkflowVersions();
}

async function exportBundle() {
  const bundle = await store.exportWorkflowBundle();
  if (!bundle) return;
  const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${safeBundleFileName(bundle.name || workflowTitle.value)}.workflow-bundle.json`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function openBundleImport() {
  bundleFileInput.value?.click();
}

async function importBundleFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  await store.importWorkflowBundle(await file.text());
}

function safeBundleFileName(name: string) {
  return name.trim().replace(/[^a-zA-Z0-9._-]+/g, "-") || "workflow";
}

function openNewWorkflow(mode: NewWorkflowMode = "cmd-run-basic") {
  newWorkflowInitialMode.value = mode;
  newWorkflowOpen.value = true;
}

async function createWorkflow(payload: { graph: WorkflowGraph; labels?: Record<string, string>; saveNote?: string }) {
  await store.createWorkflowFromGraph(payload.graph, { labels: payload.labels, saveNote: payload.saveNote });
  if (!store.state.error) {
    newWorkflowOpen.value = false;
  }
}

function requestWorkflowSwitch(name: string) {
  if (!name || name === store.state.graph?.workflow.name) return;
  if (store.state.dirty) {
    pendingWorkflowName.value = name;
    switchConfirmOpen.value = true;
    return;
  }
  void store.switchWorkflow(name);
}

async function confirmWorkflowSwitch() {
  const name = pendingWorkflowName.value;
  switchConfirmOpen.value = false;
  pendingWorkflowName.value = "";
  if (name) {
    await store.switchWorkflow(name, { force: true });
  }
}

onMounted(() => {
  void store.load();
});
</script>

<template>
  <div class="workbench-shell">
    <header class="topbar">
      <div class="brand-block">
        <div class="app-mark">R</div>
        <div>
          <h1>{{ workflowTitle }}</h1>
          <p>Runner visual workflow editor</p>
        </div>
      </div>
      <div class="topbar-actions">
        <NSelect
          class="workflow-select"
          size="small"
          :value="workflowSelectValue"
          :options="workflowSelectOptions"
          :loading="store.state.switchingWorkflow"
          :disabled="store.state.loading || store.state.switchingWorkflow"
          @update:value="requestWorkflowSwitch"
        />
        <NButton size="small" secondary :loading="store.state.creatingWorkflow" @click="openNewWorkflow('cmd-run-basic')">
          <template #icon><Plus :size="15" /></template>
          New
        </NButton>
        <NButton size="small" secondary :disabled="!store.state.graph" @click="openNewWorkflow('clone-current')">
          <template #icon><Copy :size="15" /></template>
          Clone
        </NButton>
        <NTag v-if="store.state.offline" type="warning" size="small">Mock data</NTag>
        <NTag v-else type="success" size="small">API connected</NTag>
        <NTag size="small">{{ workflowVersionText }}</NTag>
        <NTag :type="workflowStatusType" size="small">{{ store.state.workflowStatus }}</NTag>
        <NTag :type="editStatusType" size="small">{{ editStatusText }}</NTag>
        <NTag v-if="store.state.run.runId" :type="runStatusType" size="small">{{ store.state.run.status }}</NTag>
        <NTag :type="validationType" size="small">{{ validationText }}</NTag>
        <NButton size="small" secondary :loading="store.state.loading" @click="store.load()">
          <template #icon><RefreshCw :size="15" /></template>
          Reload
        </NButton>
        <NInput
          v-model:value="store.state.saveNote"
          class="save-note-input"
          size="small"
          clearable
          placeholder="Save note"
          :maxlength="160"
        />
        <NButton size="small" secondary :loading="store.state.saving" :disabled="!store.state.graph" @click="store.saveDraft()">
          <template #icon><Save :size="15" /></template>
          Save draft
        </NButton>
        <NButton size="small" secondary :loading="store.state.publishing" :disabled="!store.state.graph || store.state.dirty" @click="store.publishWorkflow()">
          <template #icon><UploadCloud :size="15" /></template>
          Publish
        </NButton>
        <NButton size="small" secondary :loading="store.state.validating" :disabled="!store.state.graph" @click="store.validateGraph()">
          <template #icon><CheckCircle2 :size="15" /></template>
          Validate
        </NButton>
        <NButton size="small" secondary :disabled="!store.state.graph" @click="yamlPreviewOpen = true">
          <template #icon><FileCode2 :size="15" /></template>
          YAML
        </NButton>
        <NCheckbox
          v-if="executionSemanticsChanged"
          v-model:checked="store.state.semanticChangeAcknowledged"
          class="risk-ack"
          size="small"
        >
          Execution reviewed
        </NCheckbox>
        <NButton size="small" secondary :loading="store.state.exportingBundle" :disabled="!store.state.graph" @click="exportBundle()">
          <template #icon><Download :size="15" /></template>
          Export
        </NButton>
        <NButton size="small" secondary :loading="store.state.importingBundle" @click="openBundleImport()">
          <template #icon><FileUp :size="15" /></template>
          Import Bundle
        </NButton>
        <input ref="bundleFileInput" type="file" accept="application/json,.json" style="display: none" @change="importBundleFile" />
        <NButton size="small" secondary :loading="store.state.loadingVersions" :disabled="!store.state.graph" @click="openHistory()">
          <template #icon><History :size="15" /></template>
          History
        </NButton>
        <NButton size="small" secondary :loading="store.state.dryRunning" :disabled="!store.state.graph" @click="store.dryRunGraph()">
          <template #icon><Play :size="15" /></template>
          Dry run
        </NButton>
        <NCheckbox v-if="hasRiskWarning" v-model:checked="store.state.riskAcknowledged" class="risk-ack" size="small">
          Risk reviewed
        </NCheckbox>
        <NCheckbox v-if="hasNonRiskWarning" v-model:checked="store.state.warningAcknowledged" class="risk-ack" size="small">
          Warnings reviewed
        </NCheckbox>
        <NButton size="small" type="primary" :loading="store.state.submitting" :disabled="!store.state.graph" @click="store.submitRun()">
          <template #icon><Send :size="15" /></template>
          Run
        </NButton>
        <NButton size="small" secondary type="error" :loading="store.state.canceling" :disabled="!canCancel" @click="store.cancelRun()">
          <template #icon><Square :size="14" /></template>
          Stop
        </NButton>
      </div>
    </header>

    <main class="workspace-grid">
      <ActionCatalog
        :actions="store.state.actions"
        @add-action="store.addActionNodeFromCatalog"
        @add-control-node="store.addControlNode"
      />

      <WorkflowCanvas
        :graph="graphWithRunState"
        :selected-node-id="store.state.selectedNodeId"
        :can-undo="store.state.historyPast.length > 0"
        :can-redo="store.state.historyFuture.length > 0"
        :can-paste="Boolean(store.state.clipboardNode)"
        @select-node="store.selectNode"
        @add-action="store.addActionNodeFromCatalog"
        @add-control-node="store.addControlNode"
        @update-node-position="(nodeId, position) => store.updateNode(nodeId, { position })"
        @connect-nodes="store.connectNodes"
        @delete-selected="store.deleteSelectedNode"
        @copy-selected="store.copySelectedNode"
        @paste-node="store.pasteNode"
        @undo="store.undo"
        @redo="store.redo"
        @auto-layout="store.autoLayout"
      />

      <PropertyPanel
        :node="selectedNode"
        :actions="store.state.actions"
        :workflow="store.state.graph?.workflow || null"
        :workflows="store.state.workflowOptions"
        :diff-summary="graphDiffSummary"
        @update-node="store.updateNode"
        @update-workflow="store.updateWorkflow"
      />
    </main>

    <RunDrawer
      :run="store.state.run"
      :validation="store.state.validation"
      :dry-run="store.state.dryRun"
      :error="store.state.error"
      :event-connected="store.state.eventConnected"
      :replaying="store.state.replaying"
      :approval-nodes="waitingApprovalNodes"
      :resolving-approval-node-id="store.state.resolvingApprovalNodeId"
      :resolving-approval-action="store.state.resolvingApprovalAction"
      @replay-run="store.replayRunHistory"
      @approve-node="store.approveNode"
      @reject-node="store.rejectNode"
    />

    <YamlPreviewModal
      v-model:show="yamlPreviewOpen"
      :graph="store.state.graph"
      :baseline-graph="store.state.baselineGraph"
      :yaml-preview="store.state.yamlPreview"
      :preview-compiling="store.state.previewCompiling"
      @replace-graph="store.replaceGraph"
      @compile-yaml="store.compilePreview"
      @parse-yaml="store.importGraphYAML"
    />

    <NewWorkflowModal
      v-model:show="newWorkflowOpen"
      :workflows="store.state.workflowOptions"
      :current-graph="store.state.graph"
      :creating="store.state.creatingWorkflow"
      :error="store.state.error"
      :initial-mode="newWorkflowInitialMode"
      @create="createWorkflow"
    />

    <NModal v-model:show="switchConfirmOpen" preset="card" title="Unsaved changes" class="confirm-modal">
      <p>Save or discard changes before switching workflows.</p>
      <template #footer>
        <div class="modal-actions">
          <NButton secondary @click="switchConfirmOpen = false">Cancel</NButton>
          <NButton type="warning" @click="confirmWorkflowSwitch">Switch anyway</NButton>
        </div>
      </template>
    </NModal>

    <NModal v-model:show="historyOpen" preset="card" title="History" class="history-modal">
      <div class="history-list">
        <div v-for="version in store.state.workflowVersions" :key="version.id" class="history-item">
          <div>
            <strong>{{ version.reason || version.id }}</strong>
            <small>{{ version.created_at || version.checksum }}</small>
          </div>
          <NTag size="small" :type="version.status === 'published' ? 'success' : 'warning'">{{ version.status || 'draft' }}</NTag>
          <NButton size="small" secondary :loading="store.state.rollingBack" @click="store.rollbackWorkflowVersion(version.id)">
            <template #icon><RotateCcw :size="14" /></template>
            Rollback
          </NButton>
        </div>
        <p v-if="!store.state.workflowVersions.length" class="empty-text">No history</p>
      </div>
    </NModal>
  </div>
</template>
