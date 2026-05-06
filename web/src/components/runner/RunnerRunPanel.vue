<script setup>
import { computed } from "vue";
import "./runnerStudio.css";

const props = defineProps({
  state: {
    type: Object,
    default: () => ({
      runId: "",
      status: "idle",
      nodes: {},
      logs: [],
      approvals: [],
      retries: [],
      variables: { inputs: [], outputs: [], exports: [], nodeResults: [] },
      artifacts: [],
    }),
  },
  selectedNodeId: {
    type: String,
    default: "",
  },
  graph: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(["select-node", "open-node-detail"]);

const graphNodeById = computed(() => {
  const entries = (props.graph?.nodes || []).map((node) => [node.id, node]);
  return Object.fromEntries(entries);
});
const runNodes = computed(() => {
  const nodes = Object.values(props.state.nodes || {});
  return nodes.map((node) => withNodeLabel(node));
});
const selectedNode = computed(() => {
  const node = props.state.nodes?.[props.selectedNodeId];
  return node ? withNodeLabel(node) : null;
});
const stdoutLogs = computed(() => logsByStream("stdout"));
const stderrLogs = computed(() => logsByStream("stderr"));
const sseLogs = computed(() => logsByStream("sse"));
const approvals = computed(() => props.state.approvals || []);
const retries = computed(() => props.state.retries || []);
const variables = computed(() => props.state.variables || {});
const inputVars = computed(() => variables.value.inputs || []);
const outputVars = computed(() => variables.value.outputs || []);
const exportVars = computed(() => variables.value.exports || []);
const nodeResults = computed(() =>
  (variables.value.nodeResults || []).filter((item) => !props.selectedNodeId || item.nodeId === props.selectedNodeId),
);
const artifacts = computed(() => props.state.artifacts || props.state.files || []);
const runId = computed(() => props.state.runId || props.state.run_id || "");
const runStatus = computed(() => props.state.status || "idle");
const hasRunData = computed(
  () =>
    Boolean(runId.value) ||
    runNodes.value.length > 0 ||
    (props.state.logs || []).length > 0 ||
    approvals.value.length > 0 ||
    retries.value.length > 0 ||
    inputVars.value.length > 0 ||
    outputVars.value.length > 0 ||
    exportVars.value.length > 0 ||
    nodeResults.value.length > 0 ||
    artifacts.value.length > 0,
);

function withNodeLabel(node) {
  const graphNode = graphNodeById.value[node.nodeId] || {};
  return {
    ...node,
    label: graphNode.step?.name || graphNode.label || node.nodeId,
    action: graphNode.step?.action || graphNode.action || "",
  };
}

function logsByStream(stream) {
  return (props.state.logs || []).filter((log) => log.stream === stream);
}

function durationLabel(value) {
  const ms = Number(value || 0);
  if (!ms) return "0s";
  if (ms < 1000) return `${ms}ms`;
  return `${Math.round(ms / 100) / 10}s`;
}

function displayValue(value) {
  if (value === undefined) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}

function logLine(log) {
  return [log.ts, log.nodeId, log.hostId, log.event, log.message].filter(Boolean).join(" ");
}
</script>

<template>
  <section class="runner-run-panel" data-testid="runner-run-panel">
    <section v-if="!hasRunData" class="runner-run-empty-state" data-testid="runner-run-empty-state">
      <strong>暂无运行记录</strong>
      <span>点击“运行”后，这里会显示 stdout、stderr、SSE、审批事件、变量和节点结果。</span>
    </section>

    <template v-else>
    <section class="runner-run-overview">
      <article>
        <span>Run ID</span>
        <strong>{{ runId || "未记录" }}</strong>
      </article>
      <article>
        <span>状态</span>
        <strong>{{ runStatus }}</strong>
      </article>
      <article>
        <span>节点</span>
        <strong>{{ runNodes.length }}</strong>
      </article>
      <article>
        <span>日志</span>
        <strong>{{ (state.logs || []).length }}</strong>
      </article>
    </section>

    <article v-if="selectedNode" class="selected-node-run-summary" data-testid="selected-node-run-summary">
      <div>
        <strong>{{ selectedNode.label }}</strong>
        <span>{{ selectedNode.status }} · {{ durationLabel(selectedNode.durationMs) }}</span>
      </div>
      <button type="button" data-testid="open-node-run-detail" @click="emit('open-node-detail', selectedNode.nodeId)">
        完整结果
      </button>
    </article>

    <section class="runner-run-section">
      <header>
        <strong>节点 trace</strong>
        <span>点击节点行定位画布节点</span>
      </header>
      <p v-if="!runNodes.length" class="runner-run-empty">暂无节点运行记录。</p>
      <article
        v-for="node in runNodes"
        :key="node.nodeId"
        class="runner-run-trace-row"
        :class="`status-${node.status || 'idle'}`"
      >
        <button type="button" :data-testid="`runner-run-trace-${node.nodeId}`" @click="emit('select-node', node.nodeId)">
          <span>
            <strong>{{ node.label }}</strong>
            <small>{{ node.action || node.nodeId }}</small>
          </span>
          <span>{{ node.status || "idle" }} · {{ durationLabel(node.durationMs) }}</span>
        </button>
        <button
          type="button"
          class="runner-run-detail-button"
          :data-testid="`runner-run-detail-${node.nodeId}`"
          @click="emit('open-node-detail', node.nodeId)"
        >
          详情
        </button>
      </article>
    </section>

    <div class="runner-run-grid">
      <section class="runner-run-section">
        <h3>stdout</h3>
        <p v-if="!stdoutLogs.length" class="runner-run-empty">暂无 stdout。</p>
        <pre v-for="(log, index) in stdoutLogs" :key="`stdout-${index}`">{{ logLine(log) }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>stderr</h3>
        <p v-if="!stderrLogs.length" class="runner-run-empty">暂无 stderr。</p>
        <pre v-for="(log, index) in stderrLogs" :key="`stderr-${index}`">{{ logLine(log) }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>SSE 实时事件</h3>
        <p v-if="!sseLogs.length" class="runner-run-empty">暂无 SSE 事件。</p>
        <pre v-for="(log, index) in sseLogs" :key="`sse-${index}`">{{ logLine(log) }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>审批事件</h3>
        <p v-if="!approvals.length" class="runner-run-empty">暂无审批事件。</p>
        <pre v-for="approval in approvals" :key="approval.id">{{ approval.status }} {{ approval.summary }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>重试轨迹</h3>
        <p v-if="!retries.length" class="runner-run-empty">暂无重试。</p>
        <pre v-for="(retry, index) in retries" :key="`retry-${index}`">{{ retry.nodeId }} {{ retry.attempt }}/{{ retry.maxAttempts }} {{ retry.reason }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>变量检查</h3>
        <p v-if="!inputVars.length && !outputVars.length && !exportVars.length" class="runner-run-empty">
          暂无运行变量。
        </p>
        <pre v-for="item in inputVars" :key="`input-${item.nodeId}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
        <pre v-for="item in outputVars" :key="`output-${item.nodeId}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
        <pre v-for="item in exportVars" :key="`export-${item.nodeId || 'run'}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>最近节点结果</h3>
        <p v-if="!nodeResults.length" class="runner-run-empty">暂无节点结果。</p>
        <pre v-for="item in nodeResults" :key="`${item.nodeId}-${JSON.stringify(item.result)}`">{{ item.nodeId }} {{ displayValue(item.result) }}</pre>
      </section>

      <section class="runner-run-section">
        <h3>Artifacts</h3>
        <p v-if="!artifacts.length" class="runner-run-empty">暂无 artifacts。</p>
        <a v-for="artifact in artifacts" :key="`${artifact.nodeId || 'run'}-${artifact.name}`" :href="artifact.url || '#'" target="_blank" rel="noreferrer">
          {{ artifact.nodeId ? `${artifact.nodeId} / ` : "" }}{{ artifact.name || artifact.url }}
        </a>
      </section>
    </div>
    </template>
  </section>
</template>
