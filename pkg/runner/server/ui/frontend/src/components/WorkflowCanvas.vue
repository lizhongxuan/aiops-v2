<script setup lang="ts">
import { SelectionMode, VueFlow, type Connection, type NodeDragEvent, type NodeMouseEvent, useVueFlow } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import { computed, onBeforeUnmount, onMounted } from "vue";
import { ClipboardPaste, Copy, GitBranch, LayoutGrid, Redo2, Trash2, Undo2 } from "lucide-vue-next";
import { useWorkflowGraph } from "../composables/useWorkflowGraph";
import type { WorkflowGraph } from "../types/workflow";
import type { ControlNodeType } from "../utils/graphEditing";

const props = defineProps<{
  graph: WorkflowGraph | null;
  selectedNodeId: string | null;
  canUndo: boolean;
  canRedo: boolean;
  canPaste: boolean;
}>();

const emit = defineEmits<{
  "select-node": [nodeId: string | null];
  "add-action": [action: string, position: { x: number; y: number }];
  "add-control-node": [type: ControlNodeType, position: { x: number; y: number }];
  "update-node-position": [nodeId: string, position: { x: number; y: number }];
  "connect-nodes": [source: string | null | undefined, target: string | null | undefined];
  "delete-selected": [];
  "copy-selected": [];
  "paste-node": [];
  undo: [];
  redo: [];
  "auto-layout": [];
}>();

const { nodes, edges } = useWorkflowGraph(() => props.graph);
const { screenToFlowCoordinate } = useVueFlow();

const nodeClass = computed(() => {
  const selected = props.selectedNodeId;
  return (node: { id: string; data?: Record<string, unknown> }) => {
    const status = node.data?.status ? `is-${node.data.status}` : "";
    return ["runner-node", status, node.id === selected ? "is-selected" : ""].filter(Boolean).join(" ");
  };
});

function handleNodeClick(event: NodeMouseEvent) {
  emit("select-node", event.node.id);
}

function handleNodeDragStop(event: NodeDragEvent) {
  emit("update-node-position", event.node.id, {
    x: event.node.position.x,
    y: event.node.position.y,
  });
}

function handleDragOver(event: DragEvent) {
  const transfer = event.dataTransfer;
  const types = transfer?.types || [];
  if (!types.includes("application/runner-action") && !types.includes("application/runner-node-type")) return;
  event.preventDefault();
  if (transfer) transfer.dropEffect = "copy";
}

function handleDrop(event: DragEvent) {
  const transfer = event.dataTransfer;
  if (!transfer) return;
  const action = transfer.getData("application/runner-action");
  const nodeType = transfer.getData("application/runner-node-type") as ControlNodeType;
  if (!action && !nodeType) return;
  event.preventDefault();
  const position = screenToFlowCoordinate({ x: event.clientX, y: event.clientY });
  if (action) {
    emit("add-action", action, position);
    return;
  }
  emit("add-control-node", nodeType, position);
}

function handleConnect(connection: Connection) {
  emit("connect-nodes", connection.source, connection.target);
}

function handleKeydown(event: KeyboardEvent) {
  if (isEditableTarget(event.target)) return;
  const key = event.key.toLowerCase();
  const command = event.metaKey || event.ctrlKey;
  if (command && key === "c" && props.selectedNodeId) {
    event.preventDefault();
    emit("copy-selected");
  } else if (command && key === "v" && props.canPaste) {
    event.preventDefault();
    emit("paste-node");
  } else if (command && key === "z" && event.shiftKey && props.canRedo) {
    event.preventDefault();
    emit("redo");
  } else if (command && key === "z" && props.canUndo) {
    event.preventDefault();
    emit("undo");
  } else if ((event.key === "Delete" || event.key === "Backspace") && props.selectedNodeId) {
    event.preventDefault();
    emit("delete-selected");
  }
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return target.isContentEditable || tag === "input" || tag === "textarea" || tag === "select";
}

onMounted(() => window.addEventListener("keydown", handleKeydown));
onBeforeUnmount(() => window.removeEventListener("keydown", handleKeydown));
</script>

<template>
  <section class="canvas-panel">
    <div class="canvas-toolbar">
      <div>
        <GitBranch :size="16" />
        <span>Graph</span>
      </div>
      <div class="canvas-toolbar-actions">
        <small>{{ nodes.length }} nodes / {{ edges.length }} edges</small>
        <button class="icon-button" type="button" title="Undo" :disabled="!canUndo" @click="emit('undo')">
          <Undo2 :size="15" />
        </button>
        <button class="icon-button" type="button" title="Redo" :disabled="!canRedo" @click="emit('redo')">
          <Redo2 :size="15" />
        </button>
        <button class="icon-button" type="button" title="Copy selected node" :disabled="!selectedNodeId" @click="emit('copy-selected')">
          <Copy :size="15" />
        </button>
        <button class="icon-button" type="button" title="Paste node" :disabled="!canPaste" @click="emit('paste-node')">
          <ClipboardPaste :size="15" />
        </button>
        <button class="icon-button" type="button" title="Auto layout" @click="emit('auto-layout')">
          <LayoutGrid :size="15" />
        </button>
        <button class="icon-button" type="button" title="Delete selected node" :disabled="!selectedNodeId" @click="emit('delete-selected')">
          <Trash2 :size="15" />
        </button>
      </div>
    </div>
    <VueFlow
      class="workflow-canvas"
      :nodes="nodes"
      :edges="edges"
      :nodes-draggable="true"
      :nodes-connectable="true"
      :elements-selectable="true"
      :selection-mode="SelectionMode.Partial"
      fit-view-on-init
      :node-class-name="nodeClass"
      @node-click="handleNodeClick"
      @node-drag-stop="handleNodeDragStop"
      @connect="handleConnect"
      @pane-click="emit('select-node', null)"
      @dragover="handleDragOver"
      @drop="handleDrop"
    >
      <Background variant="dots" :gap="18" :size="1" />
      <Controls position="bottom-left" />
      <MiniMap position="bottom-right" pannable zoomable />
    </VueFlow>
  </section>
</template>
