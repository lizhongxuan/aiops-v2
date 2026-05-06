<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NInput, NTabPane, NTabs, NTag } from "naive-ui";
import { Activity, Check, CircleDot, Download, History, Radio, ShieldCheck, X } from "lucide-vue-next";
import type { DryRunResult, ValidationResult } from "../types/workflow";
import { stringifyRedacted } from "../utils/redaction";
import type { RunLogLine, RunState } from "../utils/runEventReducer";

const props = defineProps<{
  run: RunState;
  validation: ValidationResult | null;
  dryRun: DryRunResult | null;
  error: string | null;
  eventConnected: boolean;
  replaying: boolean;
  approvalNodes: Array<{ id: string; label: string }>;
  resolvingApprovalNodeId: string | null;
  resolvingApprovalAction: "approve" | "reject" | null;
}>();

const emit = defineEmits<{
  "replay-run": [runId: string];
  "approve-node": [nodeId: string, comment: string];
  "reject-node": [nodeId: string, comment: string];
}>();

const replayRunId = ref("");
const activeTab = ref("events");
const approvalComments = ref<Record<string, string>>({});

const hostRows = computed(() => props.run.hostResults);
const stdoutText = computed(() => formatLogLines(props.run.stdout));
const stderrText = computed(() => formatLogLines(props.run.stderr));
const exportedVarsText = computed(() => stringifyRedacted(props.run.exportedVars));
const runnerDebugText = computed(() => stringifyRedacted(props.run.runnerDebug));
const hasExportedVars = computed(() => Object.keys(props.run.exportedVars).length > 0);
const hasRunnerDebug = computed(() => Object.keys(props.run.runnerDebug).length > 0);
const hasLogs = computed(() => props.run.stdout.length > 0 || props.run.stderr.length > 0 || props.run.timeline.length > 0);
const simulatedPaths = computed(() => props.dryRun?.path_simulation?.paths ?? []);

watch(
  () => props.run.runId,
  (runId) => {
    if (runId) replayRunId.value = runId;
  },
  { immediate: true },
);

function replay() {
  const runId = replayRunId.value.trim();
  if (runId) emit("replay-run", runId);
}

function approveNode(nodeId: string) {
  emit("approve-node", nodeId, approvalComments.value[nodeId] || "");
}

function rejectNode(nodeId: string) {
  emit("reject-node", nodeId, approvalComments.value[nodeId] || "");
}

function isResolving(nodeId: string, action: "approve" | "reject") {
  return props.resolvingApprovalNodeId === nodeId && props.resolvingApprovalAction === action;
}

function statusTagType(status?: string) {
  switch ((status || "").toLowerCase()) {
    case "success":
      return "success";
    case "failed":
    case "error":
      return "error";
    case "running":
      return "warning";
    case "queued":
    case "waiting":
      return "info";
    default:
      return "default";
  }
}

function formatLogLines(lines: RunLogLine[]): string {
  return [...lines]
    .reverse()
    .map((line) => {
      const scope = [line.timestamp, line.step, line.host].filter(Boolean).join(" ");
      return scope ? `[${scope}] ${line.content}` : line.content;
    })
    .join("\n");
}

function exportLogs() {
  if (!hasLogs.value) return;
  const sections = [
    `run_id=${props.run.runId || ""}`,
    `status=${props.run.status}`,
    "",
    "[timeline]",
    ...props.run.timeline
      .slice()
      .reverse()
      .map((item) => [item.timestamp, item.type, item.status, item.nodeId || item.edgeId, item.message].filter(Boolean).join(" | ")),
    "",
    "[stdout]",
    stdoutText.value || "",
    "",
    "[stderr]",
    stderrText.value || "",
  ];
  const blob = new Blob([sections.join("\n")], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${sanitizeFileName(props.run.runId || "runner-run")}.log`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function sanitizeFileName(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]/g, "_");
}
</script>

<template>
  <section class="run-drawer" :class="{ 'has-approvals': approvalNodes.length > 0 }">
    <div class="run-summary">
      <div>
        <Activity :size="16" />
        <span>Run</span>
        <NTag size="small" :bordered="false">{{ run.status }}</NTag>
        <NTag v-if="eventConnected" size="small" type="success" :bordered="false">
          <template #icon><Radio :size="12" /></template>
          SSE
        </NTag>
      </div>
      <div class="run-history-controls">
        <NInput
          v-model:value="replayRunId"
          size="small"
          placeholder="run id"
          clearable
          @keyup.enter="replay"
        />
        <NButton size="small" secondary :loading="replaying" :disabled="!replayRunId.trim()" @click="replay">
          <template #icon><History :size="14" /></template>
          Replay
        </NButton>
        <NButton size="small" secondary :disabled="!hasLogs" @click="exportLogs">
          <template #icon><Download :size="14" /></template>
          Export logs
        </NButton>
      </div>
    </div>

    <div v-if="approvalNodes.length" class="approval-strip">
      <article v-for="node in approvalNodes" :key="node.id" class="approval-item">
        <div class="approval-node">
          <ShieldCheck :size="15" />
          <strong>{{ node.label }}</strong>
          <code>{{ node.id }}</code>
        </div>
        <NInput v-model:value="approvalComments[node.id]" size="small" clearable placeholder="approval comment" :maxlength="160" />
        <NButton size="small" type="primary" secondary :loading="isResolving(node.id, 'approve')" @click="approveNode(node.id)">
          <template #icon><Check :size="14" /></template>
          Approve
        </NButton>
        <NButton size="small" type="error" secondary :loading="isResolving(node.id, 'reject')" @click="rejectNode(node.id)">
          <template #icon><X :size="14" /></template>
          Reject
        </NButton>
      </article>
    </div>

    <NTabs v-model:value="activeTab" class="run-tabs" type="line" size="small" animated>
      <NTabPane name="events" tab="Events">
        <div class="timeline">
          <article v-if="error" class="timeline-item is-error">
            <CircleDot :size="14" />
            <div>
              <strong>request_error</strong>
              <span>{{ error }}</span>
            </div>
            <time></time>
          </article>

          <article v-if="validation" class="timeline-item">
            <CircleDot :size="14" />
            <div>
              <strong>validation</strong>
              <span>{{ validation.summary || (validation.valid ? "Graph is valid" : "Graph has errors") }}</span>
            </div>
            <time>{{ validation.valid ? "valid" : `${validation.errors.length} errors` }}</time>
          </article>

          <article v-if="dryRun" class="timeline-item">
            <CircleDot :size="14" />
            <div>
              <strong>dry_run</strong>
              <span>{{ dryRun.summary || `${dryRun.steps_count} steps / ${dryRun.target_hosts.length} hosts` }}</span>
              <ul v-if="simulatedPaths.length" class="path-simulation-list">
                <li v-for="(path, index) in simulatedPaths.slice(0, 6)" :key="`${path.terminal_node_id || index}-${path.edge_ids.join('-')}`">
                  <code>{{ path.node_ids.join(" -> ") }}</code>
                  <NTag size="small" :bordered="false">{{ path.status }}</NTag>
                </li>
              </ul>
            </div>
            <time>{{ dryRun.workflow_name || "" }}</time>
          </article>

          <article v-for="item in run.timeline" :key="item.id" class="timeline-item">
            <CircleDot :size="14" />
            <div>
              <strong>{{ item.type }}</strong>
              <span>{{ item.message || item.nodeId || item.edgeId || "Event received" }}</span>
            </div>
            <time>{{ item.timestamp || "" }}</time>
          </article>
        </div>
      </NTabPane>

      <NTabPane name="hosts" tab="Hosts">
        <div class="run-panel">
          <table class="host-result-table">
            <thead>
              <tr>
                <th>Step</th>
                <th>Host</th>
                <th>Status</th>
                <th>Exit</th>
                <th>Message</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="hostRows.length === 0">
                <td colspan="6" class="empty-cell">No host result yet.</td>
              </tr>
              <tr v-for="row in hostRows" :key="row.id">
                <td>{{ row.step || "-" }}</td>
                <td>{{ row.host || "-" }}</td>
                <td>
                  <NTag size="small" :bordered="false" :type="statusTagType(row.status)">
                    {{ row.status || "unknown" }}
                  </NTag>
                </td>
                <td>{{ row.exitCode ?? "-" }}</td>
                <td>{{ row.message || "-" }}</td>
                <td>{{ row.timestamp || "-" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </NTabPane>

      <NTabPane name="logs" tab="Logs">
        <div class="run-panel log-grid">
          <section>
            <div class="drawer-section-heading">
              <strong>stdout</strong>
              <span>{{ run.stdout.length }} entries</span>
            </div>
            <pre class="log-block">{{ stdoutText || "No stdout yet." }}</pre>
          </section>
          <section>
            <div class="drawer-section-heading">
              <strong>stderr</strong>
              <span>{{ run.stderr.length }} entries</span>
            </div>
            <pre class="log-block stderr">{{ stderrText || "No stderr yet." }}</pre>
          </section>
        </div>
      </NTabPane>

      <NTabPane name="vars" tab="Vars">
        <div class="run-panel">
          <pre class="json-panel">{{ hasExportedVars ? exportedVarsText : "No exported vars yet." }}</pre>
        </div>
      </NTabPane>

      <NTabPane name="debug" tab="Debug">
        <div class="run-panel">
          <pre class="json-panel">{{ hasRunnerDebug ? runnerDebugText : "No runner_debug yet." }}</pre>
        </div>
      </NTabPane>
    </NTabs>
  </section>
</template>
