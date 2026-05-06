<script setup>
import { computed } from "vue";
import "./runnerStudio.css";

const props = defineProps({
  state: {
    type: Object,
    default: () => ({ nodes: {}, logs: [], approvals: [], retries: [] }),
  },
  selectedNodeId: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["open-node-detail"]);

const selectedNode = computed(() => props.state.nodes?.[props.selectedNodeId] || null);
const stdoutLogs = computed(() => (props.state.logs || []).filter((log) => log.stream === "stdout"));
const stderrLogs = computed(() => (props.state.logs || []).filter((log) => log.stream === "stderr"));
const sseLogs = computed(() => (props.state.logs || []).filter((log) => log.stream === "sse"));
const approvals = computed(() => props.state.approvals || []);
const retries = computed(() => props.state.retries || []);

function durationLabel(value) {
  const ms = Number(value || 0);
  if (!ms) return "0s";
  if (ms < 1000) return `${ms}ms`;
  return `${Math.round(ms / 1000)}s`;
}
</script>

<template>
  <section class="run-log-drawer" data-testid="run-log-drawer">
    <header>
      <div>
        <strong>运行抽屉</strong>
        <span>stdout、stderr、SSE、审批事件和重试轨迹</span>
      </div>
    </header>

    <article v-if="selectedNode" class="selected-node-run-summary" data-testid="selected-node-run-summary">
      <div>
        <strong>{{ selectedNode.nodeId }}</strong>
        <span>{{ selectedNode.status }} · {{ durationLabel(selectedNode.durationMs) }}</span>
      </div>
      <button type="button" data-testid="open-node-run-detail" @click="emit('open-node-detail', selectedNode.nodeId)">
        完整结果
      </button>
    </article>

    <div class="runner-drawer-grid">
      <section>
        <h3>stdout</h3>
        <p v-if="!stdoutLogs.length">暂无 stdout。</p>
        <pre v-for="(log, index) in stdoutLogs" :key="`stdout-${index}`">{{ log.hostId }} {{ log.message }}</pre>
      </section>
      <section>
        <h3>stderr</h3>
        <p v-if="!stderrLogs.length">暂无 stderr。</p>
        <pre v-for="(log, index) in stderrLogs" :key="`stderr-${index}`">{{ log.hostId }} {{ log.message }}</pre>
      </section>
      <section>
        <h3>SSE 实时事件</h3>
        <p v-if="!sseLogs.length">暂无 SSE 事件。</p>
        <pre v-for="(log, index) in sseLogs" :key="`sse-${index}`">{{ log.event }} {{ log.message }}</pre>
      </section>
      <section>
        <h3>审批事件</h3>
        <p v-if="!approvals.length">暂无审批事件。</p>
        <pre v-for="approval in approvals" :key="approval.id">{{ approval.status }} {{ approval.summary }}</pre>
      </section>
      <section>
        <h3>重试轨迹</h3>
        <p v-if="!retries.length">暂无重试。</p>
        <pre v-for="(retry, index) in retries" :key="`retry-${index}`">{{ retry.nodeId }} {{ retry.attempt }}/{{ retry.maxAttempts }} {{ retry.reason }}</pre>
      </section>
    </div>
  </section>
</template>
