import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type MouseEvent } from "react";
import {
  Background,
  ControlButton,
  Controls,
  ConnectionMode,
  MiniMap,
  MarkerType,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
  type NodeChange,
  type NodeDragHandler,
} from "@xyflow/react";
import { Maximize2, Minimize2, Plus } from "lucide-react";
import "@xyflow/react/dist/style.css";

import {
  addCatalogActionNode,
  connectFlowEdge,
  graphToFlowModel,
  insertCatalogActionOnEdge,
  removeGraphEdge,
  updateGraphNodePosition,
  type RunnerGraph,
  type RunnerPosition,
} from "./canvasGraphAdapter";
import { filterActionsForSourcePort } from "./nodeTypeRegistry";
import { getRunnerActionCategoryLabel, getRunnerActionDescription, getRunnerPaletteActions } from "./runnerActionPalette";
import { getRunnerFocusNodeId, getRunnerNodeRunState } from "./runnerRunVisualState";
import { candidateEdgeColor, RunnerCanvasEdge } from "./RunnerCanvasEdge";
import { RunnerCanvasNode } from "./RunnerCanvasNode";
import { resolveWorkflowCanvasKeyboardCommand } from "./workflowCanvasKeyboard";
import { tryProjectWorkflowCanvas } from "./workflowCanvasProjection";
import { createWorkflowLayoutOperation, projectWorkflowLayout, type WorkflowLayoutOptions } from "@/runner/layout/workflowElkLayout";
import { WORKFLOW_CONTEXT_DOCK_OPEN_EVENT } from "@/runner/WorkflowContextDock";
import type { WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";
import type { WorkflowCandidateFact } from "@/runner/workspace/workflowWorkspaceTypes";
import { WORKFLOW_CANVAS_LOCATE_EVENT } from "@/runner/workflowCanvasEvents";
import "./runnerStudio.css";

export const RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT = "aiops:runner-canvas:candidate-command";

type RunnerAction = {
  action?: string;
  name?: string;
  label?: string;
  title?: string;
  category?: string;
  description?: string;
  execution_available?: boolean;
  access?: { allowed?: boolean; reason?: string };
};

type RunnerCanvasProps = {
  graph: RunnerGraph;
  actions: RunnerAction[];
  runState?: {
    nodes?: Record<string, { status?: string; message?: string; error?: string; summary?: string; result?: unknown }>;
  };
  focusNodeId?: string;
  selectedNodeId?: string;
  aiHighlightedNodeIds?: string[];
  candidate?: WorkflowCandidateFact;
  committedResourceVersion?: string;
  autoLayoutOptions?: WorkflowLayoutOptions;
  fullscreen?: boolean;
  onUpdateGraph: (graph: RunnerGraph) => void;
  onReviseCandidateGraph?: (previousCandidateGraph: RunnerGraph, nextCandidateGraph: RunnerGraph) => void | Promise<void>;
  onSelectNode: (nodeId: string) => void;
  onOpenNodeConfig: (nodeId: string) => void;
  onDeleteNode?: (nodeId: string) => void;
  onDeleteEdge?: (edgeId: string) => void;
  onUndo?: () => void;
  onRedo?: () => void;
  onNodeAction?: (action: string, nodeId: string) => void;
  onToggleFullscreen?: () => void;
};

const nodeTypes = { "runner-node": RunnerCanvasNode };
const edgeTypes = { "runner-edge": RunnerCanvasEdge };
const CANVAS_NODE_WIDTH = 204;
const CANVAS_NODE_HEIGHT = 118;
const CANVAS_VISIBLE_NODE_HEIGHT = 66;
const CANVAS_NODE_HANDLE_Y = 46;
const EDGE_HOVER_DISTANCE = 30;
const CANVAS_FIT_MAX_ZOOM = 0.86;
const WORKFLOW_OVERLAY_SAFE_GAP = 24;

export function workflowOverlayViewportShift(
  nodeBounds: Pick<DOMRect, "right">,
  drawerBounds: Pick<DOMRect, "left">,
  gap = WORKFLOW_OVERLAY_SAFE_GAP,
) {
  return Math.max(0, Math.ceil(nodeBounds.right - drawerBounds.left + gap));
}

function candidateReadyForCanvasCommand(candidate: WorkflowCandidateFact | undefined): boolean {
  if (!candidate) return false;
  const { changeSet, candidate: current } = candidate;
  const changeSetState = String(changeSet.state || "");
  const candidateState = String(current.state || "");
  return current.currentChangeSetId === changeSet.id
    && current.currentRevision === changeSet.candidateRevision
    && current.currentPreviewHash === changeSet.previewHash
    && changeSet.validation.valid === true
    && ["previewed", "ready", "approved"].includes(changeSetState)
    && ["ready", "active"].includes(candidateState);
}

type FlowPoint = { x: number; y: number };
type ActiveEdgeHit = { edgeId: string };

function actionKey(action: RunnerAction) {
  return String(action.action || action.name || action.label || action.title || "action").replace(/[^a-zA-Z0-9_-]+/g, "-").toLowerCase();
}

function actionLabel(action: RunnerAction) {
  return action.label || action.title || action.name || action.action || "Action";
}

function pointInsideNode(point: FlowPoint, node: Node, padding = 0) {
  const x = Number(node.position?.x || 0) - padding;
  const y = Number(node.position?.y || 0) - padding;
  return point.x >= x && point.x <= x + CANVAS_NODE_WIDTH + padding * 2 && point.y >= y && point.y <= y + CANVAS_NODE_HEIGHT + padding * 2;
}

function isPointInsideAnyNode(point: FlowPoint, nodes: Node[], padding = 0) {
  return nodes.some((node) => pointInsideNode(point, node, padding));
}

function nearestPointOnSegment(point: FlowPoint, start: FlowPoint, end: FlowPoint) {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  const lengthSquared = dx * dx + dy * dy;
  if (!lengthSquared) {
    return { point: start, distance: Math.hypot(point.x - start.x, point.y - start.y), t: 0 };
  }
  const rawT = ((point.x - start.x) * dx + (point.y - start.y) * dy) / lengthSquared;
  const t = Math.min(0.86, Math.max(0.14, rawT));
  const candidate = { x: start.x + dx * t, y: start.y + dy * t };
  return { point: candidate, distance: Math.hypot(point.x - candidate.x, point.y - candidate.y), t };
}

function edgeAnchorPoints(edge: Edge, nodesById: Map<string, Node>) {
  const source = edge.source ? nodesById.get(edge.source) : undefined;
  const target = edge.target ? nodesById.get(edge.target) : undefined;
  if (!source || !target) return null;
  const sourceX = Number(source.position?.x || 0);
  const sourceY = Number(source.position?.y || 0);
  const targetX = Number(target.position?.x || 0);
  const targetY = Number(target.position?.y || 0);
  const sourceToTarget = sourceX <= targetX;
  return {
    start: { x: sourceX + (sourceToTarget ? CANVAS_NODE_WIDTH : 0), y: sourceY + CANVAS_NODE_HANDLE_Y },
    end: { x: targetX + (sourceToTarget ? 0 : CANVAS_NODE_WIDTH), y: targetY + CANVAS_NODE_HANDLE_Y },
  };
}

function getActiveEdgeHit(point: FlowPoint, nodes: Node[], edges: Edge[]): ActiveEdgeHit | null {
  if (isPointInsideAnyNode(point, nodes, 8)) return null;
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  let best: { edgeId: string; distance: number } | null = null;
  for (const edge of edges) {
    const anchors = edgeAnchorPoints(edge, nodesById);
    if (!anchors) continue;
    const nearest = nearestPointOnSegment(point, anchors.start, anchors.end);
    if (nearest.distance > EDGE_HOVER_DISTANCE) continue;
    if (!best || nearest.distance < best.distance) {
      best = { edgeId: edge.id, distance: nearest.distance };
    }
  }
  return best ? { edgeId: best.edgeId } : null;
}

function RunnerCanvasInner(props: RunnerCanvasProps) {
  const reactFlow = useReactFlow();
  const canvasRef = useRef<HTMLElement | null>(null);
  const lastFocusedSignalRef = useRef("");
  const candidateChangeIndexRef = useRef(-1);
  const latestGraphRef = useRef(props.graph);
  const layoutRequestRef = useRef(0);
  const overlayInsetFrameRef = useRef<number>();
  const pointerNodeDragActiveRef = useRef(false);
  latestGraphRef.current = props.graph;
  const [pickerOpen, setPickerOpen] = useState(false);
  const [edgePickerEdgeId, setEdgePickerEdgeId] = useState("");
  const [activeEdgeHit, setActiveEdgeHit] = useState<ActiveEdgeHit | null>(null);
  const [actionQuery, setActionQuery] = useState("");
  const [actionCategory, setActionCategory] = useState("all");
  const [connectionError, setConnectionError] = useState("");
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; insertPosition: RunnerPosition } | null>(null);
  const [edgeMenu, setEdgeMenu] = useState<{ x: number; y: number; edgeId: string } | null>(null);
  const [pendingAddPosition, setPendingAddPosition] = useState<RunnerPosition | null>(null);
  const [selectedEdgeId, setSelectedEdgeId] = useState("");
  const [recoverableDeletion, setRecoverableDeletion] = useState("");
  const menuPositionForEvent = useCallback((event: MouseEvent, menuWidth = 210, menuHeight = 178) => {
    const bounds = canvasRef.current?.getBoundingClientRect() || event.currentTarget.getBoundingClientRect();
    return {
      x: Math.min(Math.max(event.clientX - bounds.left, 8), Math.max(bounds.width - menuWidth - 8, 8)),
      y: Math.min(Math.max(event.clientY - bounds.top, 8), Math.max(bounds.height - menuHeight - 8, 8)),
    };
  }, []);
  const selectGraphEdge = useCallback(
    (edgeId: string) => {
      setSelectedEdgeId(edgeId);
      props.onSelectNode("");
      setActiveEdgeHit({ edgeId });
      setContextMenu(null);
      setEdgeMenu(null);
    },
    [props],
  );
  const deleteGraphEdges = useCallback(
    (edgeIds: string[]) => {
      if (!edgeIds.length) return;
      if (!props.onDeleteEdge || !props.onUndo) {
        setConnectionError("删除连线需要服务端可撤销事务；当前工作流未提供此能力。");
        return;
      }
      for (const edgeId of edgeIds) props.onDeleteEdge(edgeId);
      setConnectionError("");
      setRecoverableDeletion(edgeIds.length === 1 ? "连线已删除，可撤销。" : `${edgeIds.length} 条连线已删除，可撤销。`);
      setActiveEdgeHit(null);
      setSelectedEdgeId("");
      setEdgePickerEdgeId("");
      setEdgeMenu(null);
    },
    [props.onDeleteEdge, props.onUndo],
  );
  const deleteGraphNode = useCallback(
    (nodeId = "") => {
      const targetNodeId = String(nodeId || "").trim();
      if (!targetNodeId) return;
      if (!props.onDeleteNode) {
        setConnectionError("删除节点属于高风险操作，当前页面没有提供明确的服务端删除回调。");
        return;
      }
      props.onDeleteNode(targetNodeId);
      props.onSelectNode("");
    },
    [props.onDeleteNode, props.onSelectNode],
  );
  const canvasProjection = useMemo(
    () => tryProjectWorkflowCanvas(props.graph, props.candidate, { committedResourceVersion: props.committedResourceVersion }),
    [props.candidate, props.committedResourceVersion, props.graph],
  );
  const canvasGraph = canvasProjection.graph;
  const candidateActive = Boolean(props.candidate);
  const candidateEditable = candidateActive
    && canvasProjection.status === "candidate"
    && Boolean(props.onReviseCandidateGraph);
  const flowModel = useMemo(() => graphToFlowModel(canvasGraph, { selectedNodeId: props.selectedNodeId }), [canvasGraph, props.selectedNodeId]);
  const paletteActions = useMemo(() => getRunnerPaletteActions(props.actions), [props.actions]);
  const explicitFocusNodeId = useMemo(() => String(props.focusNodeId || "").split(":")[0], [props.focusNodeId]);
  const runFocusNodeId = useMemo(
    () => getRunnerFocusNodeId({ graph: canvasGraph, runState: props.runState, explicitNodeId: explicitFocusNodeId }),
    [canvasGraph, explicitFocusNodeId, props.runState],
  );
  const aiHighlightedNodeIds = useMemo(() => new Set((props.aiHighlightedNodeIds || []).map(String)), [props.aiHighlightedNodeIds]);
  const nodes = useMemo(
    () =>
      flowModel.nodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          aiHighlighted: aiHighlightedNodeIds.has(node.id),
          candidate: canvasProjection.nodes[node.id],
          candidateReadOnly: candidateActive && !candidateEditable,
          runState: getRunnerNodeRunState(props.runState, node.id),
          onOpenConfig: props.onOpenNodeConfig,
          onNodeAction: props.onNodeAction,
        },
      })) as Node[],
    [aiHighlightedNodeIds, candidateActive, candidateEditable, canvasProjection.nodes, flowModel.nodes, props.onOpenNodeConfig, props.onNodeAction, props.runState],
  );
  const edges = useMemo(
    () =>
      flowModel.edges.map((edge) => ({
        ...edge,
        reconnectable: true,
        selectable: true,
        selected: selectedEdgeId === edge.id,
        data: {
          ...(edge.data || {}),
          candidate: canvasProjection.edges[edge.id],
          active: activeEdgeHit?.edgeId === edge.id,
          candidateReadOnly: candidateActive,
          onSelectEdge: selectGraphEdge,
          onInsertEdge: candidateActive ? undefined : (edgeId: string) => {
            setPickerOpen(false);
            setEdgePickerEdgeId(edgeId);
            setActiveEdgeHit(null);
          },
          onOpenEdgeMenu: candidateActive ? undefined : (edgeId: string, event: MouseEvent<SVGPathElement>) => {
            event.preventDefault();
            event.stopPropagation();
            setContextMenu(null);
            setPickerOpen(false);
            setEdgePickerEdgeId("");
            const menuPosition = menuPositionForEvent(event, 156, 48);
            setEdgeMenu({ ...menuPosition, edgeId });
          },
        },
        markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16, color: candidateEdgeColor(canvasProjection.edges[edge.id]) },
      })) as Edge[],
    [activeEdgeHit, candidateActive, canvasProjection.edges, flowModel.edges, menuPositionForEvent, selectGraphEdge, selectedEdgeId],
  );
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState(nodes);
  const [flowEdges, setFlowEdges, onEdgesChange] = useEdgesState(edges);

  const onFlowNodesChange = useCallback(
    (changes: NodeChange[]) => {
      if (changes.some((change) => change.type === "position" && change.dragging === true)) {
        pointerNodeDragActiveRef.current = true;
      }
      onNodesChange(changes);

      const completedMoves = changes.filter(
        (change): change is Extract<NodeChange, { type: "position" }> & { position: RunnerPosition; dragging: false } =>
          change.type === "position" && change.dragging === false && Boolean(change.position),
      );
      if (!completedMoves.length || pointerNodeDragActiveRef.current) return;
      if (candidateActive && !candidateEditable) {
        setFlowNodes(nodes);
        return;
      }

      const sourceGraph = candidateEditable ? canvasGraph : props.graph;
      const movedGraph = completedMoves.reduce(
        (nextGraph, change) => updateGraphNodePosition(nextGraph, change.id, change.position),
        sourceGraph,
      );
      if (candidateEditable) {
        void props.onReviseCandidateGraph?.(canvasGraph, movedGraph);
        setFlowNodes(nodes);
        return;
      }
      props.onUpdateGraph(movedGraph);
    },
    [candidateActive, candidateEditable, canvasGraph, nodes, onNodesChange, props.graph, props.onReviseCandidateGraph, props.onUpdateGraph, setFlowNodes],
  );

  const emitCandidateCommand = useCallback((command: "previous_candidate_change" | "next_candidate_change" | "apply_candidate") => {
    const candidate = props.candidate;
    if (!candidate) return;
    const operations = candidate.changeSet.operations;
    let operationIndex: number | undefined;
    let operation: WorkflowOperation | undefined;
    if (command !== "apply_candidate" && operations.length > 0) {
      const step = command === "next_candidate_change" ? 1 : -1;
      const current = candidateChangeIndexRef.current;
      operationIndex = current < 0
        ? (step > 0 ? 0 : operations.length - 1)
        : (current + step + operations.length) % operations.length;
      candidateChangeIndexRef.current = operationIndex;
      operation = operations[operationIndex];
      const target = candidateOperationTarget(operation);
      if (target.nodeId) {
        props.onSelectNode(target.nodeId);
        const node = (canvasGraph.nodes || []).find((item) => item.id === target.nodeId);
        if (node?.position) void reactFlow.setCenter(Number(node.position.x) + 102, Number(node.position.y) + 46, { zoom: CANVAS_FIT_MAX_ZOOM, duration: 240 });
      } else if (target.edgeId) {
        selectGraphEdge(target.edgeId);
      }
    }
    window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: {
        command,
        candidateId: candidate.candidate.id,
        changeSetId: candidate.changeSet.id,
        previewHash: candidate.changeSet.previewHash,
        ...(operationIndex !== undefined ? { operationIndex, operation } : {}),
      },
    }));
  }, [canvasGraph.nodes, props.candidate, props.onSelectNode, reactFlow, selectGraphEdge]);

  useEffect(() => {
    setFlowNodes(nodes);
  }, [nodes, setFlowNodes]);

  useEffect(() => {
    setFlowEdges(edges);
  }, [edges, setFlowEdges]);

  useEffect(() => {
    if (candidateActive) return;
    if (!selectedEdgeId) return;
    const exists = (props.graph.edges || []).some((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) === selectedEdgeId);
    if (!exists) setSelectedEdgeId("");
  }, [candidateActive, props.graph.edges, selectedEdgeId]);

  useEffect(() => {
    candidateChangeIndexRef.current = -1;
  }, [props.candidate?.candidate.id, props.candidate?.candidate.currentRevision]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const command = resolveWorkflowCanvasKeyboardCommand(event, {
        selectedNodeId: props.selectedNodeId,
        selectedEdgeId,
        candidateActive,
        // Transport previews use ready/previewed while a server-persisted
        // direct revision uses active/ready. Check the exact validated binding
        // instead of one layer's state spelling.
        candidateReady: candidateReadyForCanvasCommand(props.candidate),
        candidateChangeCount: props.candidate?.changeSet.operations.length || 0,
        canUndo: Boolean(props.onUndo),
        canRedo: Boolean(props.onRedo),
      });
      if (!command || ![
        "delete_selection", "undo", "redo", "open_configuration", "clear_selection", "fit",
        "previous_candidate_change", "next_candidate_change", "apply_candidate",
      ].includes(command)) return;
      event.preventDefault();
      event.stopPropagation();
      if (command === "delete_selection") {
        if (selectedEdgeId) deleteGraphEdges([selectedEdgeId]);
        else deleteGraphNode(props.selectedNodeId);
      }
      if (command === "undo") props.onUndo?.();
      if (command === "redo") props.onRedo?.();
      if (command === "open_configuration" && props.selectedNodeId) props.onOpenNodeConfig(props.selectedNodeId);
      if (command === "clear_selection") {
        props.onSelectNode("");
        setSelectedEdgeId("");
        setActiveEdgeHit(null);
        setContextMenu(null);
        setEdgeMenu(null);
      }
      if (command === "fit") void reactFlow.fitView({ padding: 0.24, duration: 240, maxZoom: CANVAS_FIT_MAX_ZOOM });
      if (command === "previous_candidate_change" || command === "next_candidate_change" || command === "apply_candidate") emitCandidateCommand(command);
    };
    document.addEventListener("keydown", handleKeyDown, true);
    return () => document.removeEventListener("keydown", handleKeyDown, true);
  }, [candidateActive, deleteGraphEdges, deleteGraphNode, emitCandidateCommand, props.candidate, props.onOpenNodeConfig, props.onRedo, props.onSelectNode, props.onUndo, props.selectedNodeId, reactFlow, selectedEdgeId]);

  useEffect(() => {
    if (!runFocusNodeId) return;
    const signal = `${props.focusNodeId || ""}:${runFocusNodeId}`;
    if (lastFocusedSignalRef.current === signal) return;
    const graphNode = (canvasGraph.nodes || []).find((node) => node.id === runFocusNodeId);
    if (!graphNode?.position) return;
    lastFocusedSignalRef.current = signal;
    const x = Number(graphNode.position.x || 0) + 102;
    const y = Number(graphNode.position.y || 0) + 46;
    void reactFlow.setCenter(x, y, { zoom: CANVAS_FIT_MAX_ZOOM, duration: 420 });
  }, [canvasGraph.nodes, props.focusNodeId, reactFlow, runFocusNodeId]);

  useEffect(() => {
    const keepSelectedNodeClearOfWorkflowOverlay = (event: Event) => {
      const requestedTab = (event as CustomEvent<{ tab?: string }>).detail?.tab;
      if (requestedTab === "configuration") return;
      if (overlayInsetFrameRef.current !== undefined) {
        window.cancelAnimationFrame(overlayInsetFrameRef.current);
      }
      overlayInsetFrameRef.current = window.requestAnimationFrame(() => {
        overlayInsetFrameRef.current = window.requestAnimationFrame(() => {
          overlayInsetFrameRef.current = undefined;
          const drawer = document.querySelector<HTMLElement>('[data-testid="workflow-context-dock"]:not([hidden])');
          const canvas = canvasRef.current;
          if (!drawer || !canvas) return;
          const selected = props.selectedNodeId
            ? findCanvasNodeElement(props.selectedNodeId)
            : null;
          const visibleNodes = [...canvas.querySelectorAll<HTMLElement>('[data-testid^="canvas-node-"]')];
          const target = selected || visibleNodes
            .filter((node) => node.getBoundingClientRect().right > drawer.getBoundingClientRect().left)
            .sort((left, right) => right.getBoundingClientRect().right - left.getBoundingClientRect().right)[0];
          if (!target) return;
          const shift = workflowOverlayViewportShift(target.getBoundingClientRect(), drawer.getBoundingClientRect());
          if (shift <= 0) return;
          const viewport = reactFlow.getViewport();
          void reactFlow.setViewport(
            { ...viewport, x: viewport.x - shift },
            { duration: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 80 },
          );
        });
      });
    };
    window.addEventListener(WORKFLOW_CONTEXT_DOCK_OPEN_EVENT, keepSelectedNodeClearOfWorkflowOverlay);
    return () => {
      window.removeEventListener(WORKFLOW_CONTEXT_DOCK_OPEN_EVENT, keepSelectedNodeClearOfWorkflowOverlay);
      if (overlayInsetFrameRef.current !== undefined) {
        window.cancelAnimationFrame(overlayInsetFrameRef.current);
      }
    };
  }, [props.selectedNodeId, reactFlow]);

  useEffect(() => {
    const locateCurrentCandidate = () => {
      const target = (props.candidate?.changeSet.operations || [])
        .map(candidateOperationTarget)
        .find((item) => item.nodeId || item.edgeId);
      if (target?.nodeId) {
        const node = (canvasGraph.nodes || []).find((item) => item.id === target.nodeId);
        if (!node?.position) return;
        setSelectedEdgeId("");
        setActiveEdgeHit(null);
        props.onSelectNode(target.nodeId);
        void reactFlow.setCenter(
          Number(node.position.x || 0) + CANVAS_NODE_WIDTH / 2,
          Number(node.position.y || 0) + CANVAS_NODE_HANDLE_Y,
          { zoom: CANVAS_FIT_MAX_ZOOM, duration: 420 },
        );
        return;
      }
      if (target?.edgeId) {
        const edge = (canvasGraph.edges || []).find((item) => (item.id || `${item.source}-${item.target}-${item.kind || "next"}`) === target.edgeId);
        const source = (canvasGraph.nodes || []).find((item) => item.id === edge?.source);
        const destination = (canvasGraph.nodes || []).find((item) => item.id === edge?.target);
        if (!edge || !source?.position || !destination?.position) return;
        selectGraphEdge(target.edgeId);
        void reactFlow.setCenter(
          (Number(source.position.x || 0) + Number(destination.position.x || 0) + CANVAS_NODE_WIDTH) / 2,
          (Number(source.position.y || 0) + Number(destination.position.y || 0)) / 2 + CANVAS_NODE_HANDLE_Y,
          { zoom: CANVAS_FIT_MAX_ZOOM, duration: 420 },
        );
        return;
      }
      void reactFlow.fitView({ padding: 0.24, duration: 240, maxZoom: CANVAS_FIT_MAX_ZOOM });
    };
    window.addEventListener(WORKFLOW_CANVAS_LOCATE_EVENT, locateCurrentCandidate);
    return () => window.removeEventListener(WORKFLOW_CANVAS_LOCATE_EVENT, locateCurrentCandidate);
  }, [canvasGraph.edges, canvasGraph.nodes, props.candidate, props.onSelectNode, reactFlow, selectGraphEdge]);

  const addAction = useCallback(
    (action: RunnerAction, position = { x: 420, y: 180 }, options: { preservePosition?: boolean } = {}) => {
      try {
        const nextGraph = addCatalogActionNode(props.graph, action, position, {
          ...options,
          requireSafeInsertion: true,
        });
        setConnectionError("");
        props.onUpdateGraph(nextGraph);
      } catch (error) {
        setConnectionError(error instanceof Error ? error.message : "无法安全添加节点");
      }
    },
    [props],
  );

  const insertActionOnEdge = useCallback(
    (action: RunnerAction) => {
      if (!edgePickerEdgeId) return;
      props.onUpdateGraph(insertCatalogActionOnEdge(props.graph, edgePickerEdgeId, action));
      setEdgePickerEdgeId("");
    },
    [edgePickerEdgeId, props],
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      if (candidateActive && !candidateEditable) return;
      const sourceGraph = candidateEditable ? canvasGraph : props.graph;
      const result = connectFlowEdge(sourceGraph, connection);
      if (result.error) {
        setConnectionError(result.error.message);
        return;
      }
      setConnectionError("");
      if (candidateEditable) {
        void props.onReviseCandidateGraph?.(canvasGraph, result.graph);
        setFlowEdges(edges);
        return;
      }
      props.onUpdateGraph(result.graph);
    },
    [candidateActive, candidateEditable, canvasGraph, edges, props, setFlowEdges],
  );

  const isValidConnection = useCallback(
    (connection: Connection | Edge) => {
      if (candidateActive && !candidateEditable) return false;
      const result = connectFlowEdge(candidateEditable ? canvasGraph : props.graph, connection as Connection);
      setConnectionError(result.error?.message || "");
      return !result.error;
    },
    [candidateActive, candidateEditable, canvasGraph, props.graph],
  );

  const onEdgesDelete = useCallback(
    (deletedEdges: Edge[]) => {
      deleteGraphEdges(deletedEdges.map((edge) => edge.id).filter(Boolean));
    },
    [deleteGraphEdges],
  );

  const onReconnect = useCallback(
    (oldEdge: Edge, connection: Connection) => {
      if (candidateActive && !candidateEditable) return;
      const sourceGraph = candidateEditable ? canvasGraph : props.graph;
      const graphWithoutOldEdge = removeGraphEdge(sourceGraph, oldEdge.id);
      const result = connectFlowEdge(graphWithoutOldEdge, connection);
      if (result.error) {
        setConnectionError(result.error.message);
        return;
      }
      const reconnectedGraph = preserveReconnectedEdgeIdentity(graphWithoutOldEdge, result.graph, oldEdge.id);
      setConnectionError("");
      setEdgeMenu(null);
      if (candidateEditable) {
        void props.onReviseCandidateGraph?.(canvasGraph, reconnectedGraph);
        setFlowEdges(edges);
        return;
      }
      props.onUpdateGraph(reconnectedGraph);
    },
    [candidateActive, candidateEditable, canvasGraph, edges, props, setFlowEdges],
  );

  const edgeForPicker = useMemo(
    () => (props.graph.edges || []).find((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) === edgePickerEdgeId) || null,
    [edgePickerEdgeId, props.graph.edges],
  );
  const edgePickerSourcePort = String(edgeForPicker?.source_port || edgeForPicker?.sourceHandle || edgeForPicker?.kind || "next");
  const edgeActions = useMemo(() => filterActionsForSourcePort(paletteActions, edgePickerSourcePort), [paletteActions, edgePickerSourcePort]);
  const actionCategories = useMemo(() => ["all", ...Array.from(new Set(paletteActions.map(getRunnerActionCategoryLabel)))], [paletteActions]);
  const visibleActions = useMemo(() => {
    const query = actionQuery.trim().toLowerCase();
    return paletteActions.filter((action) => {
      const category = getRunnerActionCategoryLabel(action);
      if (actionCategory !== "all" && category !== actionCategory) return false;
      if (!query) return true;
      return [actionLabel(action), getRunnerActionDescription(action), action.action, action.name, category].some((value) => String(value || "").toLowerCase().includes(query));
    });
  }, [actionCategory, actionQuery, paletteActions]);

  const onNodeDragStop = useCallback<NodeDragHandler>(
    (_event, node) => {
      pointerNodeDragActiveRef.current = false;
      if (candidateActive && !candidateEditable) return;
      if (candidateEditable) {
        void props.onReviseCandidateGraph?.(
          canvasGraph,
          updateGraphNodePosition(canvasGraph, node.id, node.position),
        );
        setFlowNodes(nodes);
        setActiveEdgeHit(null);
        return;
      }
      props.onUpdateGraph(updateGraphNodePosition(props.graph, node.id, node.position));
      setActiveEdgeHit(null);
    },
    [candidateActive, candidateEditable, canvasGraph, nodes, props, setFlowNodes],
  );

  const onNodeDragStart = useCallback<NodeDragHandler>(() => {
    pointerNodeDragActiveRef.current = true;
  }, []);

  const handleCanvasMouseMove = useCallback(
    (event: MouseEvent) => {
      if (edgePickerEdgeId) return;
      const point = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const hit = getActiveEdgeHit(point, flowModel.nodes as Node[], flowModel.edges as Edge[]);
      setActiveEdgeHit((current) => {
        if (!current && !hit) return current;
        if (current && hit && current.edgeId === hit.edgeId) return current;
        return hit;
      });
    },
    [edgePickerEdgeId, flowModel.edges, flowModel.nodes, reactFlow],
  );

  const handleCanvasClick = useCallback(
    (event: MouseEvent<HTMLElement>) => {
      const target = event.target as Element | null;
      if (
        target?.closest(
          "button,input,textarea,select,.react-flow__node,.runner-node-picker,.runner-canvas-context-menu,.runner-edge-menu,.react-flow__controls,.react-flow__minimap",
        )
      ) {
        return;
      }
      const point = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const hit = getActiveEdgeHit(point, flowModel.nodes as Node[], flowModel.edges as Edge[]);
      if (!hit) return;
      event.stopPropagation();
      selectGraphEdge(hit.edgeId);
    },
    [flowModel.edges, flowModel.nodes, reactFlow, selectGraphEdge],
  );

  const handleDrop = useCallback(
    (event: DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      if (candidateActive) return;
      setContextMenu(null);
      setEdgeMenu(null);
      setPendingAddPosition(null);
      const key = event.dataTransfer.getData("application/runner-action");
      const action = props.actions.find((item) => actionKey(item) === key || item.action === key || item.name === key);
      if (!action) return;
      const point = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      addAction(action, { x: point.x - CANVAS_NODE_WIDTH / 2, y: point.y - CANVAS_VISIBLE_NODE_HEIGHT / 2 });
    },
    [addAction, candidateActive, props.actions, reactFlow],
  );

  const openNodePicker = useCallback(() => {
    setEdgePickerEdgeId("");
    setActiveEdgeHit(null);
    setContextMenu(null);
    setEdgeMenu(null);
    setPendingAddPosition(null);
    setPickerOpen((value) => !value);
  }, []);

  const handlePaneContextMenu = useCallback(
    (event: MouseEvent) => {
      event.preventDefault();
      if (candidateActive) return;
      const flowPoint = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const menuPosition = menuPositionForEvent(event);
      setContextMenu({
        ...menuPosition,
        insertPosition: {
          x: flowPoint.x - CANVAS_NODE_WIDTH / 2,
          y: flowPoint.y - CANVAS_VISIBLE_NODE_HEIGHT / 2,
        },
      });
      setEdgeMenu(null);
      setPendingAddPosition(null);
      setPickerOpen(false);
      setEdgePickerEdgeId("");
      setActiveEdgeHit(null);
    },
    [candidateActive, menuPositionForEvent, reactFlow],
  );

  const handleAutoLayout = useCallback(() => {
    if (candidateActive) {
      void reactFlow.fitView({ padding: 0.24, duration: 240, maxZoom: CANVAS_FIT_MAX_ZOOM });
      return;
    }
    const graph = props.graph;
    const request = ++layoutRequestRef.current;
    void createWorkflowLayoutOperation(graph, props.autoLayoutOptions).then((operation) => {
      if (request !== layoutRequestRef.current || latestGraphRef.current !== graph) return;
      props.onUpdateGraph(projectWorkflowLayout(graph, operation));
      setActiveEdgeHit(null);
      setEdgePickerEdgeId("");
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => {
          void reactFlow.fitView({ padding: 0.24, duration: 360, maxZoom: CANVAS_FIT_MAX_ZOOM });
        });
      });
    }).catch(() => {
      if (request === layoutRequestRef.current) setConnectionError("自动布局失败，请重试");
    });
  }, [candidateActive, props, reactFlow]);

  return (
    <section className={`runner-canvas-react ${pickerOpen ? "with-palette" : ""}`} data-testid="runner-canvas-dropzone" onClickCapture={handleCanvasClick} onDrop={handleDrop} onDragOver={(event) => event.preventDefault()}>
      {candidateActive ? (
        <aside
          role={canvasProjection.status === "blocked" ? "alert" : "status"}
          data-testid="runner-candidate-canvas-status"
          style={{ position: "absolute", zIndex: 55, top: 12, right: 12, maxWidth: 340, border: `1px solid ${canvasProjection.status === "blocked" ? "#dc2626" : "#93c5fd"}`, borderRadius: 8, background: "#fff", color: "#334155", padding: "8px 10px", fontSize: 12, fontWeight: 700, boxShadow: "0 8px 20px rgba(15,23,42,.10)" }}
        >
          {canvasProjection.status === "blocked" ? `候选预览已阻止：${canvasProjection.error}` : `候选版本 ${canvasProjection.candidateRevision}（尚未保存）${canvasProjection.workflow.blocked === "conflict" ? " · 存在冲突" : ""}`}
        </aside>
      ) : null}
      {contextMenu ? (
        <section className="runner-canvas-context-menu" data-testid="runner-canvas-context-menu" style={{ left: contextMenu.x, top: contextMenu.y }}>
          <button type="button" data-testid="runner-context-add-node" onClick={() => { setPendingAddPosition(contextMenu.insertPosition); setContextMenu(null); setPickerOpen(true); }}>
            添加节点
          </button>
          <button type="button" onClick={() => { setContextMenu(null); handleAutoLayout(); }}>
            重新布局
          </button>
          <button type="button" disabled>
            粘贴到这里
          </button>
          <button type="button" disabled>
            导出 DSL
          </button>
        </section>
      ) : null}
      {edgeMenu ? (
        <section className="runner-edge-menu runner-edge-menu-compact" data-testid="runner-edge-menu" style={{ left: edgeMenu.x, top: edgeMenu.y }}>
          <button type="button" className="danger" data-testid="runner-edge-delete" disabled={!props.onDeleteEdge || !props.onUndo} title={!props.onDeleteEdge || !props.onUndo ? "需要服务端可撤销事务" : "删除后可撤销"} onClick={() => deleteGraphEdges([edgeMenu.edgeId])}>
            {!props.onDeleteEdge || !props.onUndo ? "删除连线（不可用）" : "删除连线"}
          </button>
          {!props.onDeleteEdge || !props.onUndo ? <small>当前服务端未提供可撤销删除。</small> : null}
        </section>
      ) : null}
      {pickerOpen ? (
        <aside className="runner-node-picker runner-canvas-palette" data-testid="runner-node-picker" aria-label="节点库">
          <header className="runner-canvas-palette-head">
            <div>
              <strong>节点</strong>
              <span>{visibleActions.length} 个节点</span>
            </div>
            <button type="button" aria-label="收起节点库" onClick={() => { setPendingAddPosition(null); setPickerOpen(false); }}>x</button>
          </header>
          <input
            className="runner-canvas-palette-search"
            value={actionQuery}
            placeholder="搜索节点"
            onChange={(event) => setActionQuery(event.target.value)}
          />
          <div className="runner-canvas-palette-tabs" role="tablist" aria-label="节点分类">
            {actionCategories.map((category) => (
              <button key={category} type="button" className={actionCategory === category ? "active" : ""} onClick={() => setActionCategory(category)}>
                {category === "all" ? "全部" : category}
              </button>
            ))}
          </div>
          <div className="runner-canvas-palette-list">
          {visibleActions.map((action) => (
            <button
              key={actionKey(action)}
              type="button"
              draggable
              data-testid={`catalog-action-${actionKey(action)}`}
              onDragStart={(event) => event.dataTransfer.setData("application/runner-action", actionKey(action))}
              onClick={() => {
                addAction(action, pendingAddPosition || undefined, { preservePosition: Boolean(pendingAddPosition) });
                setPendingAddPosition(null);
                setPickerOpen(false);
              }}
            >
              <strong>{actionLabel(action)}</strong>
              <span>{getRunnerActionDescription(action)}</span>
            </button>
          ))}
          </div>
        </aside>
      ) : null}
      {edgePickerEdgeId ? (
        <div className="runner-node-picker runner-edge-node-picker" data-testid="runner-edge-node-picker">
          <header>
            <strong>插入节点</strong>
            <button type="button" aria-label="关闭插入节点" onClick={() => setEdgePickerEdgeId("")}>
              x
            </button>
          </header>
          {edgeActions.map((action) => (
            <button
              key={actionKey(action)}
              type="button"
              data-testid={`edge-catalog-action-${actionKey(action)}`}
              onClick={() => insertActionOnEdge(action)}
            >
              <strong>{actionLabel(action)}</strong>
              <span>{getRunnerActionDescription(action)}</span>
            </button>
          ))}
        </div>
      ) : null}
      {connectionError ? <p className="runner-canvas-validation" role="alert">{connectionError}</p> : null}
      {recoverableDeletion ? <p className="runner-canvas-validation" role="status">{recoverableDeletion}<button type="button" onClick={() => { props.onUndo?.(); setRecoverableDeletion(""); }}>撤销</button></p> : null}
      <ReactFlow
        ref={canvasRef}
        nodes={flowNodes}
        edges={flowEdges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={onFlowNodesChange}
        onEdgesChange={onEdgesChange}
        onEdgesDelete={onEdgesDelete}
        onConnect={onConnect}
        isValidConnection={isValidConnection}
        onConnectStart={() => setConnectionError("")}
        onReconnect={onReconnect}
        onEdgeClick={(event, edge) => {
          event.stopPropagation();
          selectGraphEdge(edge.id);
        }}
        onEdgeDoubleClick={(event, edge) => {
          event.preventDefault();
          event.stopPropagation();
          selectGraphEdge(edge.id);
          if (!candidateActive) {
            const menuPosition = menuPositionForEvent(event, 156, 76);
            setEdgeMenu({ ...menuPosition, edgeId: edge.id });
          }
        }}
        onEdgeContextMenu={(event, edge) => {
          event.preventDefault();
          event.stopPropagation();
          if (candidateActive) return;
          setContextMenu(null);
          setPickerOpen(false);
          setEdgePickerEdgeId("");
          const menuPosition = menuPositionForEvent(event, 156, 48);
          setEdgeMenu({ ...menuPosition, edgeId: edge.id });
        }}
        onNodeClick={(event, node) => {
          const eventTarget = event.target instanceof HTMLElement ? event.target : null;
          const focusTarget = eventTarget?.closest<HTMLElement>(`[data-testid="canvas-node-${node.id}"]`)
            || findCanvasNodeElement(node.id);
          if (focusTarget) {
            focusTarget.tabIndex = 0;
            focusTarget.focus({ preventScroll: true });
          }
          setSelectedEdgeId("");
          props.onSelectNode(node.id);
        }}
        onPaneClick={() => {
          props.onSelectNode("");
          setActiveEdgeHit(null);
          setSelectedEdgeId("");
          setContextMenu(null);
          setEdgeMenu(null);
        }}
        onPaneContextMenu={handlePaneContextMenu}
        onNodeDragStart={onNodeDragStart}
        onNodeDragStop={onNodeDragStop}
        nodesDraggable={!candidateActive || candidateEditable}
        nodesConnectable={!candidateActive || candidateEditable}
        edgesReconnectable={!candidateActive || candidateEditable}
        onMouseMove={handleCanvasMouseMove}
        onMouseLeave={() => setActiveEdgeHit(null)}
        fitView
        fitViewOptions={{ padding: 0.24, maxZoom: CANVAS_FIT_MAX_ZOOM }}
        maxZoom={1.4}
        connectionRadius={44}
        connectionMode={ConnectionMode.Loose}
        reconnectRadius={16}
        connectOnClick
        deleteKeyCode={null}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <MiniMap pannable zoomable />
        <Controls onFitView={handleAutoLayout}>
          {!candidateActive ? (
            <ControlButton
              data-testid="runner-node-picker-toggle"
              title="添加节点"
              aria-label="添加节点"
              onClick={openNodePicker}
            >
              <Plus size={16} />
            </ControlButton>
          ) : null}
          <ControlButton
            className="runner-canvas-fullscreen-control"
            data-testid="runner-canvas-fullscreen-toggle"
            title={props.fullscreen ? "退出全屏" : "全屏"}
            aria-label={props.fullscreen ? "退出全屏" : "全屏"}
            onClick={props.onToggleFullscreen}
          >
            {props.fullscreen ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
          </ControlButton>
        </Controls>
      </ReactFlow>
    </section>
  );
}

export function RunnerCanvas(props: RunnerCanvasProps) {
  return (
    <ReactFlowProvider>
      <RunnerCanvasInner {...props} />
    </ReactFlowProvider>
  );
}

function candidateOperationTarget(operation: WorkflowOperation): { nodeId?: string; edgeId?: string } {
  if (operation.op === "add_node") return { nodeId: operation.node.id || operation.clientRef };
  if (operation.op === "update_node" || operation.op === "delete_node") return { nodeId: operation.nodeId };
  if (operation.op === "add_edge") return operation.edge.id ? { edgeId: operation.edge.id } : { nodeId: operation.edge.target };
  if (operation.op === "update_edge" || operation.op === "delete_edge") return { edgeId: operation.edgeId };
  if (operation.op === "update_layout") return { nodeId: operation.layout.nodes[0]?.nodeId };
  return {};
}

function findCanvasNodeElement(nodeId: string): HTMLElement | null {
  const testId = `canvas-node-${nodeId}`;
  return [...document.querySelectorAll<HTMLElement>('[data-testid^="canvas-node-"]')]
    .find((element) => element.dataset.testid === testId) || null;
}

function preserveReconnectedEdgeIdentity(before: RunnerGraph, after: RunnerGraph, edgeId: string): RunnerGraph {
  const existingIds = new Set((before.edges || []).map((edge) => edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`));
  const addedEdge = (after.edges || []).find((edge) => !existingIds.has(edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`));
  if (!addedEdge) return after;
  return {
    ...after,
    edges: (after.edges || []).map((edge) => edge === addedEdge ? { ...edge, id: edgeId } : edge),
  };
}
