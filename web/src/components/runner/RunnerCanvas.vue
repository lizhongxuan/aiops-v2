<script setup>
import { computed, markRaw, onBeforeUnmount, onMounted, ref } from "vue";
import { ConnectionMode, SelectionMode, VueFlow, useVueFlow } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import CanvasToolbar from "./CanvasToolbar.vue";
import NodeActionMenu from "./NodeActionMenu.vue";
import NodePicker from "./NodePicker.vue";
import RunnerCanvasEdge from "./RunnerCanvasEdge.vue";
import RunnerCanvasNode from "./RunnerCanvasNode.vue";
import RunnerConnectionLine from "./RunnerConnectionLine.vue";
import RunnerEdgeMenu from "./RunnerEdgeMenu.vue";
import {
  addCatalogActionNode,
  connectFlowEdge,
  graphToFlowModel,
  removeGraphEdge,
  updateGraphEdgeKind,
  updateGraphNodePosition,
  validateGraphConnection,
} from "./canvasGraphAdapter";
import { filterActionsForSourcePort, getActionIdentity } from "./nodeTypeRegistry";
import "./runnerStudio.css";

const props = defineProps({
  graph: {
    type: Object,
    default: () => ({ nodes: [], edges: [] }),
  },
  actions: {
    type: Array,
    default: () => [],
  },
  selectedNodeId: {
    type: String,
    default: "",
  },
  fullscreen: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["update:graph", "select-node", "open-node-config", "node-action", "toggle-fullscreen"]);

const { screenToFlowCoordinate, viewport } = useVueFlow();
const nodeTypes = { "runner-node": markRaw(RunnerCanvasNode) };
const CANVAS_NODE_WIDTH = 204;
const CANVAS_NODE_HEIGHT = 92;
const DEFAULT_VIEWPORT = Object.freeze({ x: 0, y: 0, zoom: 1 });
const MAX_ZOOM = 1.4;
const menuState = ref(null);
const edgeMenuState = ref(null);
const pendingConnection = ref(null);
const connectionCompleted = ref(false);
const contextualPicker = ref(null);
const validationMessage = ref("");
const recentActionKeys = ref([]);
const hoveredEdgeId = ref("");
const flowModel = computed(() => graphToFlowModel(props.graph, { selectedNodeId: props.selectedNodeId }));
const edgeLabelControls = computed(() => {
  const nodesById = new Map((props.graph.nodes || []).map((node) => [node.id, node]));
  const transform = viewport?.value || { x: 0, y: 0, zoom: 1 };
  const zoom = Number(transform.zoom || 1);
  return (props.graph.edges || [])
    .map((edge) => {
      const sourceNode = nodesById.get(edge.source);
      const targetNode = nodesById.get(edge.target);
      if (!sourceNode || !targetNode) return null;
      const sourcePosition = sourceNode.position || { x: 0, y: 0 };
      const targetPosition = targetNode.position || { x: 0, y: 0 };
      const sourceX = Number(sourcePosition.x || 0) + CANVAS_NODE_WIDTH;
      const sourceY = Number(sourcePosition.y || 0) + CANVAS_NODE_HEIGHT / 2;
      const targetX = Number(targetPosition.x || 0);
      const targetY = Number(targetPosition.y || 0) + CANVAS_NODE_HEIGHT / 2;
      const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`;
      const kind = edge.kind || edge.source_port || "next";
      return {
        edge: { ...edge, id: edgeId },
        id: edgeId,
        kind,
        className: `kind-${String(kind).replace(/[^a-zA-Z0-9_-]+/g, "-")}`,
        style: {
          left: `${((sourceX + targetX) / 2) * zoom + Number(transform.x || 0)}px`,
          top: `${((sourceY + targetY) / 2) * zoom + Number(transform.y || 0)}px`,
        },
        insertStyle: {
          left: `${((sourceX + targetX) / 2) * zoom + Number(transform.x || 0)}px`,
          top: `${((sourceY + targetY) / 2) * zoom + Number(transform.y || 0)}px`,
        },
        position: {
          x: (sourceX + targetX) / 2,
          y: (sourceY + targetY) / 2 - 46,
        },
      };
    })
    .filter(Boolean);
});
const contextualActions = computed(() => {
  if (!contextualPicker.value?.sourcePort) return props.actions;
  return filterActionsForSourcePort(props.actions, contextualPicker.value.sourcePort);
});

function addAction(action, position = { x: 80, y: 80 }) {
  const picker = contextualPicker.value || {};
  rememberRecentAction(action);
  const nextGraph = addCatalogActionNode(props.graph, action, position);
  const newNode = nextGraph.nodes.at(-1);
  const source = picker.source || pendingConnection.value?.source;
  const sourcePort = picker.sourcePort || pendingConnection.value?.sourcePort || "next";
  contextualPicker.value = null;
  pendingConnection.value = null;
  validationMessage.value = "";

  if (picker.edge?.id && newNode?.id) {
    const insertedGraph = insertNodeIntoEdge(nextGraph, newNode, picker.edge);
    emit("update:graph", insertedGraph);
    return;
  }

  if (source && newNode?.id && source !== newNode.id) {
    const result = connectFlowEdge(nextGraph, { source, target: newNode.id, sourceHandle: sourcePort, targetHandle: "in" });
    if (result.error) {
      validationMessage.value = result.error.message;
      emit("update:graph", nextGraph);
      return;
    }
    emit("update:graph", autoConnectNewNodeToEnd(result.graph, newNode, sourcePort));
    return;
  }

  emit("update:graph", nextGraph);
}

function insertNodeIntoEdge(graph, newNode, edge) {
  const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`;
  const sourcePort = edge.source_port || edge.sourceHandle || edge.kind || "next";
  const targetPort = edge.target_port || edge.targetHandle || "in";
  const graphWithoutEdge = removeGraphEdge(graph, edgeId);
  const first = connectFlowEdge(graphWithoutEdge, {
    source: edge.source,
    target: newNode.id,
    sourceHandle: sourcePort,
    targetHandle: "in",
    kind: edge.kind || sourcePort,
  });
  if (first.error) {
    validationMessage.value = first.error.message;
    return graph;
  }
  const nextPort = firstOutputPort(newNode);
  const second = connectFlowEdge(first.graph, {
    source: newNode.id,
    target: edge.target,
    sourceHandle: nextPort,
    targetHandle: targetPort,
    kind: nextPort,
  });
  if (second.error) {
    validationMessage.value = second.error.message;
    return graph;
  }
  return second.graph;
}

function firstOutputPort(node = {}) {
  const outputPort = (node.ports || []).find((port) => port.type === "output" && port.id === "next")
    || (node.ports || []).find((port) => port.type === "output");
  return outputPort?.id || "next";
}

function addActionFromToolbar(action) {
  addAction(action, { x: 80, y: 80 });
}

function handleDrop(event) {
  const raw = event.dataTransfer?.getData("application/runner-action");
  if (!raw) return;
  event.preventDefault();
  try {
    const action = JSON.parse(raw);
    addAction(action, eventToFlowPosition(event));
  } catch {
    // Ignore malformed drag payloads.
  }
}

function handleDragOver(event) {
  if (!event.dataTransfer?.types?.includes("application/runner-action")) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = "copy";
}

function handleNodeClick(event) {
  closeTransientOverlays();
  if (event?.node?.id) {
    emit("select-node", event.node.id);
  }
}

function handleNodeDragStop(event) {
  const node = event?.node;
  if (!node?.id) return;
  emit("update:graph", updateGraphNodePosition(props.graph, node.id, node.position || { x: 0, y: 0 }));
}

function handleConnect(connection) {
  connectionCompleted.value = true;
  contextualPicker.value = null;
  menuState.value = null;
  edgeMenuState.value = null;
  pendingConnection.value = null;
  const result = connectFlowEdge(props.graph, connection || {});
  if (result.error) {
    validationMessage.value = result.error.message;
    return;
  }
  validationMessage.value = "";
  emit("update:graph", result.graph);
}

function isValidConnection(connection) {
  if (!connection?.source || !connection?.target) return true;
  return validateGraphConnection(props.graph, connection || {}).valid;
}

function handleConnectStart(event, params = {}) {
  closeTransientOverlays();
  const payload = normalizeConnectPayload(event, params);
  const source = payload.nodeId || payload.node?.id || payload.source || payload.sourceNode?.id || "";
  const sourcePort = payload.handleId || payload.sourceHandle || payload.handle?.id || "next";
  connectionCompleted.value = false;
  pendingConnection.value = source ? { source, sourcePort } : null;
  validationMessage.value = "";
}

function handleConnectEnd(event) {
  if (connectionCompleted.value || !pendingConnection.value?.source) {
    connectionCompleted.value = false;
    return;
  }
  contextualPicker.value = {
    source: pendingConnection.value.source,
    sourcePort: pendingConnection.value.sourcePort || "next",
    position: eventToFlowPosition(event),
    screen: {
      x: getPointerEvent(event)?.clientX || 120,
      y: getPointerEvent(event)?.clientY || 120,
    },
  };
  menuState.value = null;
  edgeMenuState.value = null;
  connectionCompleted.value = false;
}

function openPortInsertPicker(payload = {}) {
  const node = (props.graph.nodes || []).find((item) => item.id === payload.nodeId);
  if (!node?.id) return;
  const event = payload.event || {};
  closeMenusBeforeOpeningPicker();
  contextualPicker.value = {
    source: node.id,
    sourcePort: payload.portId || "next",
    position: positionAfterNode(node),
    screen: {
      x: event.clientX || 120,
      y: event.clientY || 120,
    },
  };
  validationMessage.value = "";
}

function openEdgeInsertPicker(edge = {}, event = {}) {
  const graphEdge = findGraphEdge(edge.id) || edge;
  if (!graphEdge?.source || !graphEdge?.target) return;
  event?.preventDefault?.();
  event?.stopPropagation?.();
  closeMenusBeforeOpeningPicker();
  contextualPicker.value = {
    edge: { ...graphEdge, id: graphEdge.id || `${graphEdge.source}-${graphEdge.target}-${graphEdge.kind || "next"}` },
    source: graphEdge.source,
    sourcePort: graphEdge.source_port || graphEdge.sourceHandle || graphEdge.kind || "next",
    position: eventToFlowPosition(event),
    screen: {
      x: event.clientX || 120,
      y: event.clientY || 120,
    },
  };
  validationMessage.value = "";
}

function autoConnectNewNodeToEnd(graph, newNode, sourcePort = "next") {
  if ((sourcePort || "next") !== "next") return graph;
  if ((graph.edges || []).some((edge) => edge.source === newNode.id)) return graph;
  const endNode = (graph.nodes || []).find((node) => node.type === "end" || node.id === "end");
  if (!endNode?.id || endNode.id === newNode.id) return graph;
  const nextPort = firstOutputPort(newNode);
  if (!nextPort) return graph;
  const result = connectFlowEdge(graph, {
    source: newNode.id,
    target: endNode.id,
    sourceHandle: nextPort,
    targetHandle: "in",
    kind: nextPort,
  });
  return result.error ? graph : result.graph;
}

function handleEdgeHover(payload = {}) {
  const edgeId = payload.id || "";
  if (payload.hovering) {
    hoveredEdgeId.value = edgeId;
  } else if (hoveredEdgeId.value === edgeId) {
    hoveredEdgeId.value = "";
  }
}

function positionAfterNode(node = {}) {
  const position = node.position || { x: 0, y: 0 };
  return {
    x: Number(position.x || 0) + 300,
    y: Number(position.y || 0),
  };
}

function showNodeMenu(payload) {
  const event = payload?.event;
  contextualPicker.value = null;
  edgeMenuState.value = null;
  menuState.value = {
    node: payload?.node || { id: payload?.node?.id },
    x: event?.clientX || 0,
    y: event?.clientY || 0,
  };
}

function showEdgeMenu(payload) {
  contextualPicker.value = null;
  menuState.value = null;
  edgeMenuState.value = {
    edge: payload?.edge || { id: "" },
    x: payload?.x || 0,
    y: payload?.y || 0,
  };
}

function showOverlayEdgeMenu(edge, event) {
  event?.preventDefault?.();
  event?.stopPropagation?.();
  contextualPicker.value = null;
  menuState.value = null;
  edgeMenuState.value = {
    edge: findGraphEdge(edge?.id) || edge || { id: "" },
    x: event?.clientX || 0,
    y: event?.clientY || 0,
  };
}

function showFlowEdgeMenu(payload) {
  const edge = payload?.edge || payload;
  if (!edge?.id) return;
  const event = getPointerEvent(payload);
  event?.preventDefault?.();
  event?.stopPropagation?.();
  contextualPicker.value = null;
  menuState.value = null;
  edgeMenuState.value = {
    edge: findGraphEdge(edge.id) || edge,
    x: event?.clientX || 0,
    y: event?.clientY || 0,
  };
}

function handleNodeAction(action, nodeId) {
  emit("node-action", action, nodeId);
}

function handleEdgeAction(action, edgeId) {
  const edge = edgeMenuState.value?.edge || {};
  if (action === "delete" && edgeId) {
    emit("update:graph", removeGraphEdge(props.graph, edgeId));
  } else if (action?.startsWith("set-kind:") && edgeId) {
    const kind = action.slice("set-kind:".length) || "next";
    emit("update:graph", updateGraphEdgeKind(props.graph, edgeId, kind));
  } else if (action === "focus-source" && edge.source) {
    emit("select-node", edge.source);
  } else if (action === "focus-target" && edge.target) {
    emit("select-node", edge.target);
  }
  edgeMenuState.value = null;
}

function closeMenusBeforeOpeningPicker() {
  menuState.value = null;
  edgeMenuState.value = null;
}

function closeTransientOverlays() {
  menuState.value = null;
  edgeMenuState.value = null;
  contextualPicker.value = null;
  hoveredEdgeId.value = "";
}

function handlePaneClick() {
  closeTransientOverlays();
  emit("select-node", "");
}

function handleGlobalKeydown(event) {
  if (event.key === "Escape") {
    closeTransientOverlays();
  }
}

function eventToFlowPosition(event) {
  const pointerEvent = getPointerEvent(event);
  const x = pointerEvent?.clientX || 0;
  const y = pointerEvent?.clientY || 0;
  try {
    return screenToFlowCoordinate({ x, y });
  } catch {
    return { x, y };
  }
}

function normalizeConnectPayload(event, params = {}) {
  if (params?.nodeId || params?.handleId || params?.node || params?.source || params?.sourceNode) return params;
  if (event?.nodeId || event?.handleId || event?.node || event?.source || event?.sourceNode) return event;
  return {};
}

function getPointerEvent(event) {
  return event?.event || event?.sourceEvent || event;
}

function rememberRecentAction(action) {
  const identity = getActionIdentity(action);
  if (!identity) return;
  recentActionKeys.value = [identity, ...recentActionKeys.value.filter((key) => key !== identity)].slice(0, 6);
}

function findGraphEdge(edgeId) {
  return (props.graph.edges || []).find((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) === edgeId);
}

onMounted(() => {
  window.addEventListener("keydown", handleGlobalKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleGlobalKeydown);
});
</script>

<template>
  <section
    class="runner-canvas-shell"
  >
    <VueFlow
      class="runner-canvas-flow"
      data-testid="runner-canvas-dropzone"
      :nodes="flowModel.nodes"
      :edges="flowModel.edges"
      :node-types="nodeTypes"
      :nodes-draggable="true"
      :nodes-connectable="true"
      :elements-selectable="true"
      :selection-mode="SelectionMode.Partial"
      :connection-mode="ConnectionMode.Strict"
      :connection-radius="44"
      :connect-on-click="true"
      :is-valid-connection="isValidConnection"
      :default-viewport="DEFAULT_VIEWPORT"
      :max-zoom="MAX_ZOOM"
      @node-click="handleNodeClick"
      @node-drag-stop="handleNodeDragStop"
      @connect="handleConnect"
      @connect-start="handleConnectStart"
      @connect-end="handleConnectEnd"
      @edge-click="showFlowEdgeMenu"
      @edge-context-menu="showFlowEdgeMenu"
      @pane-click="handlePaneClick"
      @dragover="handleDragOver"
      @drop="handleDrop"
    >
      <Background variant="dots" :gap="22" :size="1" />
      <Controls position="bottom-left" />
      <MiniMap position="bottom-right" pannable zoomable />
      <template #node-runner-node="{ id, data, selected }">
        <RunnerCanvasNode
          :id="id"
          :data="data"
          :selected="Boolean(selected) || selectedNodeId === id"
          @open-node-config="emit('open-node-config', $event)"
          @open-menu="showNodeMenu"
          @insert-after-port="openPortInsertPicker"
        />
      </template>
      <template #edge-runner-edge="edgeProps">
        <RunnerCanvasEdge v-bind="edgeProps" @open-menu="showEdgeMenu" @edge-hover="handleEdgeHover" />
      </template>
      <template #connection-line="connectionLineProps">
        <RunnerConnectionLine v-bind="connectionLineProps" />
      </template>
    </VueFlow>

    <button
      v-for="label in edgeLabelControls"
      :key="`${label.id}-insert`"
      type="button"
      class="runner-edge-insert-button"
      :class="{ visible: hoveredEdgeId === label.id }"
      :style="label.insertStyle"
      :data-testid="`runner-edge-insert-${label.id}`"
      title="在这条连线上插入节点"
      @pointerenter="hoveredEdgeId = label.id"
      @pointerleave="hoveredEdgeId = ''"
      @pointerdown.stop
      @mousedown.stop
      @click.stop="openEdgeInsertPicker(label.edge, $event)"
    >
      +
    </button>

    <CanvasToolbar
      :actions="actions"
      :fullscreen="fullscreen"
      :recent-action-keys="recentActionKeys"
      @add-action="addActionFromToolbar"
      @toggle-fullscreen="emit('toggle-fullscreen')"
    />

    <p v-if="validationMessage" class="runner-canvas-validation" data-testid="runner-canvas-validation">
      {{ validationMessage }}
    </p>

    <NodePicker
      v-if="contextualPicker"
      class="runner-node-picker-contextual"
      :style="{ left: `${contextualPicker.screen.x}px`, top: `${contextualPicker.screen.y}px` }"
      :actions="contextualActions"
      :source-port="contextualPicker.sourcePort"
      :recent-action-keys="recentActionKeys"
      compact
      @select="addAction($event, contextualPicker.position)"
      @close="contextualPicker = null"
    />

    <NodeActionMenu
      v-if="menuState"
      :node="menuState.node"
      :x="menuState.x"
      :y="menuState.y"
      @action="handleNodeAction"
      @close="menuState = null"
    />

    <RunnerEdgeMenu
      v-if="edgeMenuState"
      :edge="edgeMenuState.edge"
      :x="edgeMenuState.x"
      :y="edgeMenuState.y"
      @action="handleEdgeAction"
      @close="edgeMenuState = null"
    />
  </section>
</template>
