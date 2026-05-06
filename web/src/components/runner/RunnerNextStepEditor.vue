<script setup>
import { computed } from "vue";
import { connectFlowEdge } from "./canvasGraphAdapter";
import { getNodePorts } from "./nodeTypeRegistry";

const props = defineProps({
  node: {
    type: Object,
    required: true,
  },
  graph: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["update:graph"]);

const IMPLICIT_EXIT_PORTS = new Set(["failure", "rejected", "timeout"]);

const sourcePorts = computed(() => getNodePorts(props.node).outputs || []);
const candidateNodes = computed(() =>
  (props.graph.nodes || []).filter((node) => node.id && node.id !== props.node.id && node.type !== "group"),
);
const visibleSourcePorts = computed(() =>
  sourcePorts.value.filter((port) => {
    if (!IMPLICIT_EXIT_PORTS.has(port.id)) return true;
    const edge = edgeForPort(props.node.id, port.id);
    if (!edge) return false;
    return !isTerminalNode(edge.target);
  }),
);
const hasImplicitExitPorts = computed(() => sourcePorts.value.some((port) => IMPLICIT_EXIT_PORTS.has(port.id)));

function edgePort(edge) {
  return edge.source_port || edge.sourceHandle || edge.kind || "next";
}

function edgeForPort(nodeId, portId) {
  return (props.graph.edges || []).find((edge) => edge.source === nodeId && edgePort(edge) === portId) || null;
}

function isTerminalNode(nodeId) {
  const node = (props.graph.nodes || []).find((item) => item.id === nodeId);
  if (!node) return false;
  const type = String(node.type || "").toLowerCase();
  const action = String(node.step?.action || node.action || "").toLowerCase();
  return type === "end" || action === "end" || node.id === "end";
}

function currentTarget(portId) {
  return edgeForPort(props.node.id, portId)?.target || "";
}

function nodeLabel(node) {
  return node.step?.name || node.label || node.id;
}

function updateNextStep(portId, targetId) {
  const graphWithoutPortEdge = {
    ...props.graph,
    workflow: { ...(props.graph.workflow || {}) },
    nodes: [...(props.graph.nodes || [])],
    edges: (props.graph.edges || []).filter((edge) => !(edge.source === props.node.id && edgePort(edge) === portId)),
  };
  if (!targetId) {
    emit("update:graph", graphWithoutPortEdge);
    return;
  }
  const result = connectFlowEdge(graphWithoutPortEdge, {
    source: props.node.id,
    sourceHandle: portId,
    target: targetId,
    targetHandle: "in",
    kind: portId,
  });
  emit("update:graph", result.graph);
}

function clearNextStep(portId) {
  updateNextStep(portId, "");
}
</script>

<template>
  <section class="runner-next-step-editor" data-testid="runner-next-step-editor">
    <header>
      <strong>下一步</strong>
      <span>按输出端口选择后继节点</span>
    </header>
    <p v-if="hasImplicitExitPorts" class="runner-next-step-note" data-testid="next-step-default-exit">
      失败未设置时默认退出。
    </p>
    <div v-if="visibleSourcePorts.length" class="runner-next-step-list">
      <label v-for="port in visibleSourcePorts" :key="port.id" class="runner-next-step-row">
        <span>{{ port.label || port.id }}</span>
        <div class="runner-next-step-control">
          <select
            :value="currentTarget(port.id)"
            :data-testid="`next-step-select-${port.id}`"
            @change="updateNextStep(port.id, $event.target.value)"
          >
            <option value="">选择下一个节点</option>
            <option v-for="candidate in candidateNodes" :key="candidate.id" :value="candidate.id">
              {{ nodeLabel(candidate) }}
            </option>
          </select>
          <button
            v-if="currentTarget(port.id)"
            type="button"
            :data-testid="`next-step-delete-${port.id}`"
            @click.prevent="clearNextStep(port.id)"
          >
            删除
          </button>
        </div>
      </label>
    </div>
    <p v-else>当前节点没有可连接的输出端口。</p>
  </section>
</template>
