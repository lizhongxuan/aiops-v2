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
  addEdge,
  useReactFlow,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
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
import { RunnerCanvasEdge } from "./RunnerCanvasEdge";
import { RunnerCanvasNode } from "./RunnerCanvasNode";
import "./runnerStudio.css";

type RunnerAction = {
  action?: string;
  name?: string;
  label?: string;
  title?: string;
  category?: string;
  description?: string;
};

type RunnerCanvasProps = {
  graph: RunnerGraph;
  actions: RunnerAction[];
  runState?: {
    nodes?: Record<string, { status?: string; message?: string; error?: string; summary?: string; result?: unknown }>;
  };
  focusNodeId?: string;
  selectedNodeId?: string;
  fullscreen?: boolean;
  onUpdateGraph: (graph: RunnerGraph) => void;
  onSelectNode: (nodeId: string) => void;
  onOpenNodeConfig: (nodeId: string) => void;
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
const LAYOUT_START_X = 80;
const LAYOUT_START_Y = 160;
const LAYOUT_COLUMN_GAP = 300;
const LAYOUT_ROW_GAP = 170;
const CANVAS_FIT_MAX_ZOOM = 0.86;

type FlowPoint = { x: number; y: number };
type ActiveEdgeHit = { edgeId: string };
type RunnerGraphNode = NonNullable<RunnerGraph["nodes"]>[number];

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

function nodeLayoutRank(node: RunnerGraphNode, indexById: Map<string, number>) {
  const type = String(node.type || "").toLowerCase();
  if (type === "start" || node.id === "start") return -10000;
  if (type === "end" || node.id === "end") return 10000;
  return indexById.get(node.id) || 0;
}

function autoLayoutRunnerGraph(graph: RunnerGraph): RunnerGraph {
  const nodes = graph.nodes || [];
  if (!nodes.length) return graph;
  const nodeIds = new Set(nodes.map((node) => node.id));
  const indexById = new Map(nodes.map((node, index) => [node.id, index]));
  const incoming = new Map(nodes.map((node) => [node.id, [] as string[]]));
  for (const edge of graph.edges || []) {
    const source = String(edge.source || "");
    const target = String(edge.target || "");
    if (!nodeIds.has(source) || !nodeIds.has(target)) continue;
    incoming.get(target)?.push(source);
  }
  const depthMemo = new Map<string, number>();
  const depthForNode = (nodeId: string, visiting = new Set<string>()): number => {
    if (depthMemo.has(nodeId)) return depthMemo.get(nodeId) || 0;
    if (visiting.has(nodeId)) return 0;
    visiting.add(nodeId);
    const depth = (incoming.get(nodeId) || []).reduce((maxDepth, sourceId) => Math.max(maxDepth, depthForNode(sourceId, visiting) + 1), 0);
    visiting.delete(nodeId);
    depthMemo.set(nodeId, depth);
    return depth;
  };
  const layers = new Map<string, number>();
  for (const node of nodes) layers.set(node.id, depthForNode(node.id));
  const layerRows = new Map<number, typeof nodes>();
  for (const node of nodes) {
    const layer = layers.get(node.id) || 0;
    layerRows.set(layer, [...(layerRows.get(layer) || []), node]);
  }
  for (const [layer, layerNodes] of layerRows) {
    layerRows.set(
      layer,
      [...layerNodes].sort((a, b) => nodeLayoutRank(a, indexById) - nodeLayoutRank(b, indexById)),
    );
  }
  const rowById = new Map<string, number>();
  for (const layerNodes of layerRows.values()) layerNodes.forEach((node, row) => rowById.set(node.id, row));
  return {
    ...graph,
    nodes: nodes.map((node) => ({
      ...node,
      position: {
        x: LAYOUT_START_X + (layers.get(node.id) || 0) * LAYOUT_COLUMN_GAP,
        y: LAYOUT_START_Y + (rowById.get(node.id) || 0) * LAYOUT_ROW_GAP,
      },
    })),
  };
}

function RunnerCanvasInner(props: RunnerCanvasProps) {
  const reactFlow = useReactFlow();
  const canvasRef = useRef<HTMLElement | null>(null);
  const lastFocusedSignalRef = useRef("");
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
      let nextGraph = props.graph;
      for (const edgeId of edgeIds) nextGraph = removeGraphEdge(nextGraph, edgeId);
      setConnectionError("");
      setActiveEdgeHit(null);
      setSelectedEdgeId("");
      setEdgePickerEdgeId("");
      setEdgeMenu(null);
      props.onUpdateGraph(nextGraph);
    },
    [props],
  );
  const flowModel = useMemo(() => graphToFlowModel(props.graph, { selectedNodeId: props.selectedNodeId }), [props.graph, props.selectedNodeId]);
  const paletteActions = useMemo(() => getRunnerPaletteActions(props.actions), [props.actions]);
  const explicitFocusNodeId = useMemo(() => String(props.focusNodeId || "").split(":")[0], [props.focusNodeId]);
  const runFocusNodeId = useMemo(
    () => getRunnerFocusNodeId({ graph: props.graph, runState: props.runState, explicitNodeId: explicitFocusNodeId }),
    [explicitFocusNodeId, props.graph, props.runState],
  );
  const nodes = useMemo(
    () =>
      flowModel.nodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          runState: getRunnerNodeRunState(props.runState, node.id),
          onOpenConfig: props.onOpenNodeConfig,
          onNodeAction: props.onNodeAction,
        },
      })) as Node[],
    [flowModel.nodes, props.onOpenNodeConfig, props.onNodeAction, props.runState],
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
          active: activeEdgeHit?.edgeId === edge.id,
          onSelectEdge: selectGraphEdge,
          onInsertEdge: (edgeId: string) => {
            setPickerOpen(false);
            setEdgePickerEdgeId(edgeId);
            setActiveEdgeHit(null);
          },
          onOpenEdgeMenu: (edgeId: string, event: MouseEvent<SVGPathElement>) => {
            event.preventDefault();
            event.stopPropagation();
            setContextMenu(null);
            setPickerOpen(false);
            setEdgePickerEdgeId("");
            const menuPosition = menuPositionForEvent(event, 156, 48);
            setEdgeMenu({ ...menuPosition, edgeId });
          },
        },
        markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16, color: "#94a3b8" },
      })) as Edge[],
    [activeEdgeHit, flowModel.edges, menuPositionForEvent, selectGraphEdge, selectedEdgeId],
  );
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState(nodes);
  const [flowEdges, setFlowEdges, onEdgesChange] = useEdgesState(edges);

  useEffect(() => {
    setFlowNodes(nodes);
  }, [nodes, setFlowNodes]);

  useEffect(() => {
    setFlowEdges(edges);
  }, [edges, setFlowEdges]);

  useEffect(() => {
    if (!selectedEdgeId) return;
    const exists = (props.graph.edges || []).some((edge) => (edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`) === selectedEdgeId);
    if (!exists) setSelectedEdgeId("");
  }, [props.graph.edges, selectedEdgeId]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!selectedEdgeId) return;
      if (event.key !== "Delete" && event.key !== "Backspace") return;
      const target = event.target as HTMLElement | null;
      if (target && (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName) || target.isContentEditable)) return;
      event.preventDefault();
      deleteGraphEdges([selectedEdgeId]);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [deleteGraphEdges, selectedEdgeId]);

  useEffect(() => {
    if (!runFocusNodeId) return;
    const signal = `${props.focusNodeId || ""}:${runFocusNodeId}`;
    if (lastFocusedSignalRef.current === signal) return;
    const graphNode = (props.graph.nodes || []).find((node) => node.id === runFocusNodeId);
    if (!graphNode?.position) return;
    lastFocusedSignalRef.current = signal;
    const x = Number(graphNode.position.x || 0) + 102;
    const y = Number(graphNode.position.y || 0) + 46;
    void reactFlow.setCenter(x, y, { zoom: CANVAS_FIT_MAX_ZOOM, duration: 420 });
  }, [props.focusNodeId, props.graph.nodes, reactFlow, runFocusNodeId]);

  const addAction = useCallback(
    (action: RunnerAction, position = { x: 420, y: 180 }, options: { preservePosition?: boolean } = {}) => {
      props.onUpdateGraph(addCatalogActionNode(props.graph, action, position, options));
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
      const result = connectFlowEdge(props.graph, connection);
      if (result.error) {
        setConnectionError(result.error.message);
        return;
      }
      setConnectionError("");
      props.onUpdateGraph(result.graph);
      setFlowEdges((currentEdges) => addEdge(connection, currentEdges));
    },
    [props],
  );

  const onEdgesDelete = useCallback(
    (deletedEdges: Edge[]) => {
      deleteGraphEdges(deletedEdges.map((edge) => edge.id).filter(Boolean));
    },
    [deleteGraphEdges],
  );

  const onReconnect = useCallback(
    (oldEdge: Edge, connection: Connection) => {
      const graphWithoutOldEdge = removeGraphEdge(props.graph, oldEdge.id);
      const result = connectFlowEdge(graphWithoutOldEdge, connection);
      if (result.error) {
        setConnectionError(result.error.message);
        return;
      }
      setConnectionError("");
      setEdgeMenu(null);
      props.onUpdateGraph(result.graph);
    },
    [props],
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
      props.onUpdateGraph(updateGraphNodePosition(props.graph, node.id, node.position));
      setActiveEdgeHit(null);
    },
    [props],
  );

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
      setContextMenu(null);
      setEdgeMenu(null);
      setPendingAddPosition(null);
      const key = event.dataTransfer.getData("application/runner-action");
      const action = props.actions.find((item) => actionKey(item) === key || item.action === key || item.name === key);
      if (!action) return;
      const point = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      addAction(action, { x: point.x - CANVAS_NODE_WIDTH / 2, y: point.y - CANVAS_VISIBLE_NODE_HEIGHT / 2 });
    },
    [addAction, props.actions, reactFlow],
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
    [menuPositionForEvent, reactFlow],
  );

  const handleAutoLayout = useCallback(() => {
    const layoutGraph = autoLayoutRunnerGraph(props.graph);
    props.onUpdateGraph(layoutGraph);
    setActiveEdgeHit(null);
    setEdgePickerEdgeId("");
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        void reactFlow.fitView({ padding: 0.24, duration: 360, maxZoom: CANVAS_FIT_MAX_ZOOM });
      });
    });
  }, [props, reactFlow]);

  return (
    <section className={`runner-canvas-react ${pickerOpen ? "with-palette" : ""}`} data-testid="runner-canvas-dropzone" onClickCapture={handleCanvasClick} onDrop={handleDrop} onDragOver={(event) => event.preventDefault()}>
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
          <button type="button" className="danger" data-testid="runner-edge-delete" onClick={() => deleteGraphEdges([edgeMenu.edgeId])}>
            删除连线
          </button>
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
      <ReactFlow
        ref={canvasRef}
        nodes={flowNodes}
        edges={flowEdges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onEdgesDelete={onEdgesDelete}
        onConnect={onConnect}
        onReconnect={onReconnect}
        onEdgeClick={(event, edge) => {
          event.stopPropagation();
          selectGraphEdge(edge.id);
        }}
        onEdgeDoubleClick={(event, edge) => {
          event.preventDefault();
          event.stopPropagation();
          deleteGraphEdges([edge.id]);
        }}
        onEdgeContextMenu={(event, edge) => {
          event.preventDefault();
          event.stopPropagation();
          setContextMenu(null);
          setPickerOpen(false);
          setEdgePickerEdgeId("");
          const menuPosition = menuPositionForEvent(event, 156, 48);
          setEdgeMenu({ ...menuPosition, edgeId: edge.id });
        }}
        onNodeClick={(_event, node) => {
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
        onNodeDragStop={onNodeDragStop}
        onMouseMove={handleCanvasMouseMove}
        onMouseLeave={() => setActiveEdgeHit(null)}
        fitView
        fitViewOptions={{ padding: 0.24, maxZoom: CANVAS_FIT_MAX_ZOOM }}
        maxZoom={1.4}
        connectionRadius={44}
        connectionMode={ConnectionMode.Loose}
        edgesReconnectable
        reconnectRadius={16}
        connectOnClick
        deleteKeyCode={["Backspace", "Delete"]}
      >
        <Background />
        <MiniMap pannable zoomable />
        <Controls onFitView={handleAutoLayout}>
          <ControlButton
            data-testid="runner-node-picker-toggle"
            title="添加节点"
            aria-label="添加节点"
            onClick={openNodePicker}
          >
            <Plus size={16} />
          </ControlButton>
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
