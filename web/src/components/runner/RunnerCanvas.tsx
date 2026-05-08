import { useCallback, useEffect, useMemo, useState, type DragEvent } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  addEdge,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
  type NodeDragHandler,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import {
  addCatalogActionNode,
  connectFlowEdge,
  graphToFlowModel,
  updateGraphNodePosition,
  type RunnerGraph,
} from "./canvasGraphAdapter";
import { RunnerCanvasEdge } from "./RunnerCanvasEdge";
import { RunnerCanvasNode } from "./RunnerCanvasNode";
import "./runnerStudio.css";

type RunnerAction = {
  action?: string;
  name?: string;
  label?: string;
  title?: string;
  category?: string;
};

type RunnerCanvasProps = {
  graph: RunnerGraph;
  actions: RunnerAction[];
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

function actionKey(action: RunnerAction) {
  return String(action.action || action.name || action.label || action.title || "action").replace(/[^a-zA-Z0-9_-]+/g, "-").toLowerCase();
}

function actionLabel(action: RunnerAction) {
  return action.label || action.title || action.name || action.action || "Action";
}

function RunnerCanvasInner(props: RunnerCanvasProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const flowModel = useMemo(() => graphToFlowModel(props.graph, { selectedNodeId: props.selectedNodeId }), [props.graph, props.selectedNodeId]);
  const nodes = useMemo(
    () =>
      flowModel.nodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          onOpenConfig: props.onOpenNodeConfig,
          onNodeAction: props.onNodeAction,
        },
      })) as Node[],
    [flowModel.nodes, props.onOpenNodeConfig, props.onNodeAction],
  );
  const edges = flowModel.edges as Edge[];
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState(nodes);
  const [flowEdges, setFlowEdges, onEdgesChange] = useEdgesState(edges);

  useEffect(() => {
    setFlowNodes(nodes);
  }, [nodes, setFlowNodes]);

  useEffect(() => {
    setFlowEdges(edges);
  }, [edges, setFlowEdges]);

  const addAction = useCallback(
    (action: RunnerAction, position = { x: 420, y: 180 }) => {
      props.onUpdateGraph(addCatalogActionNode(props.graph, action, position));
    },
    [props],
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

  const onNodeDragStop = useCallback<NodeDragHandler>(
    (_event, node) => {
      props.onUpdateGraph(updateGraphNodePosition(props.graph, node.id, node.position));
    },
    [props],
  );

  const handleDrop = useCallback(
    (event: DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      const key = event.dataTransfer.getData("application/runner-action");
      const action = props.actions.find((item) => actionKey(item) === key || item.action === key || item.name === key);
      if (!action) return;
      const bounds = event.currentTarget.getBoundingClientRect();
      addAction(action, { x: event.clientX - bounds.left, y: event.clientY - bounds.top });
    },
    [addAction, props.actions],
  );

  return (
    <section className="runner-canvas-react" data-testid="runner-canvas-dropzone" onDrop={handleDrop} onDragOver={(event) => event.preventDefault()}>
      <div className="runner-canvas-toolbar">
        <button type="button" data-testid="runner-node-picker-toggle" onClick={() => setPickerOpen((value) => !value)}>
          添加节点
        </button>
        <button type="button" data-testid="runner-canvas-fullscreen-toggle" onClick={props.onToggleFullscreen}>
          {props.fullscreen ? "退出全屏" : "全屏"}
        </button>
      </div>
      {pickerOpen ? (
        <div className="runner-node-picker" data-testid="runner-node-picker">
          {props.actions.map((action) => (
            <button
              key={actionKey(action)}
              type="button"
              draggable
              data-testid={`catalog-action-${actionKey(action)}`}
              onDragStart={(event) => event.dataTransfer.setData("application/runner-action", actionKey(action))}
              onClick={() => addAction(action)}
            >
              <strong>{actionLabel(action)}</strong>
              <span>{action.category || action.action || action.name}</span>
            </button>
          ))}
        </div>
      ) : null}
      {connectionError ? <p className="runner-canvas-validation" role="alert">{connectionError}</p> : null}
      <ReactFlow
        nodes={flowNodes}
        edges={flowEdges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={(_event, node) => props.onSelectNode(node.id)}
        onPaneClick={() => props.onSelectNode("")}
        onNodeDragStop={onNodeDragStop}
        fitView
        maxZoom={1.4}
        connectionRadius={44}
        connectOnClick
      >
        <Background />
        <MiniMap pannable zoomable />
        <Controls />
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
