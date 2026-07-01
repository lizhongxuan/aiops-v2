import {
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  reconnectEdge,
  useEdgesState,
  useNodesState,
  type Connection,
  type EdgeTypes,
  type Node,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useEffect, useMemo } from "react";
import type { DragEvent } from "react";

import { OpsGraphCanvasEdge } from "./OpsGraphCanvasEdge";
import { OpsGraphCanvasGroup } from "./OpsGraphCanvasGroup";
import { OpsGraphCanvasNode } from "./OpsGraphCanvasNode";
import type { OpsGraphRecord, OpsGraphRelationship } from "./opsGraphTypes";
import { buildCanvasModel, nodeTypeLabel, reconnectOpsGraphRelationship } from "./opsGraphViewModel";

const nodeTypes: NodeTypes = {
  opsgraphNode: OpsGraphCanvasNode,
  opsgraphGroup: OpsGraphCanvasGroup,
};

const edgeTypes: EdgeTypes = {
  opsgraphEdge: OpsGraphCanvasEdge,
};

type OpsGraphCanvasProps = {
  graph: OpsGraphRecord;
  onCreateNode?: (node: { type: string; subtype?: string; name: string; position: { x: number; y: number } }) => void;
  onCreateRelationship?: (relationship: { from: string; to: string; type: string }) => void;
  onSelectNode?: (nodeId: string | null) => void;
  onSelectRelationship?: (relationshipId: string | null) => void;
  onReconnectRelationship?: (relationship: OpsGraphRelationship) => void;
  selectedRelationshipId?: string | null;
  onSaveLayout?: (nodes: Array<{ id: string; position?: { x: number; y: number }; collapsed?: boolean }>, viewport?: { x: number; y: number; zoom: number }) => void;
};

export function OpsGraphCanvas({
  graph,
  onCreateNode,
  onCreateRelationship,
  onSelectNode,
  onSelectRelationship,
  onReconnectRelationship,
  selectedRelationshipId,
  onSaveLayout,
}: OpsGraphCanvasProps) {
  const model = useMemo(() => buildCanvasModel(graph), [graph]);
  const canvasNodes = useMemo(() => model.nodes.map((node) => ({
    ...node,
    data: {
      ...node.data,
      onSelect: onSelectNode,
    },
  })), [model.nodes, onSelectNode]);
  const canvasEdges = useMemo(() => model.edges.map((edge) => ({
    ...edge,
    selected: edge.id === selectedRelationshipId,
    data: {
      ...edge.data,
      onSelectRelationship,
    },
  })), [model.edges, onSelectRelationship, selectedRelationshipId]);
  const [nodes, setNodes, onNodesChange] = useNodesState(canvasNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(canvasEdges);

  useEffect(() => {
    setNodes(canvasNodes);
    setEdges(canvasEdges);
  }, [canvasEdges, canvasNodes, setEdges, setNodes]);

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    const raw = event.dataTransfer.getData("application/x-opsgraph-node");
    const legacyType = event.dataTransfer.getData("application/x-opsgraph-node-type");
    const payload = parsePalettePayload(raw, legacyType);
    if (!payload.type) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    onCreateNode?.({
      type: payload.type,
      subtype: payload.subtype,
      name: defaultNodeName(payload.type, payload.subtype, payload.label),
      position: { x: event.clientX - bounds.left, y: event.clientY - bounds.top },
    });
  }

  function handleConnect(connection: Connection) {
    if (!connection.source || !connection.target) return;
    onCreateRelationship?.({ from: connection.source, to: connection.target, type: "depends_on" });
  }

  function handleReconnect(edge: any, connection: Connection) {
    const relationship = edge.data?.relationship as OpsGraphRelationship | undefined;
    if (!relationship) return;
    const next = reconnectOpsGraphRelationship(relationship, connection);
    if (!next) return;
    setEdges((currentEdges) => reconnectEdge(edge, connection, currentEdges));
    onReconnectRelationship?.(next);
  }

  function handleNodeDragStop(draggedNode: Node) {
    const nextNodes = nodes.map((node) => (node.id === draggedNode.id ? { ...node, position: draggedNode.position } : node));
    setNodes(nextNodes);
    onSaveLayout?.(nextNodes.map((node) => ({
      id: node.id,
      position: node.position,
      collapsed: Boolean(node.data?.node && typeof node.data.node === "object" && "collapsed" in node.data.node)
        ? Boolean((node.data.node as { collapsed?: boolean }).collapsed)
        : undefined,
    })));
  }

  return (
    <ReactFlowProvider>
      <div
        data-testid="opsgraph-canvas"
        className="h-full min-h-0 rounded-lg border bg-slate-50"
        onDrop={handleDrop}
        onDragOver={(event) => {
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
        }}
      >
        <ReactFlow
          key={`${graph.id}:${graph.nodes.length}:${graph.edges.length}`}
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          fitView
          fitViewOptions={{ padding: 0.2, maxZoom: 1 }}
          onConnect={handleConnect}
          onEdgesChange={onEdgesChange}
          onNodesChange={onNodesChange}
          onEdgeClick={(_, edge) => onSelectRelationship?.(edge.id)}
          onReconnect={handleReconnect}
          onNodeClick={(_, node) => onSelectNode?.(node.id)}
          onNodeDragStop={(_, node) => handleNodeDragStop(node)}
          edgesReconnectable
          reconnectRadius={12}
          onPaneClick={() => {
            onSelectNode?.(null);
            onSelectRelationship?.(null);
          }}
        >
          <Background gap={18} size={1} />
          <Controls />
        </ReactFlow>
      </div>
    </ReactFlowProvider>
  );
}

function parsePalettePayload(raw: string, legacyType: string): { type: string; subtype?: string; label?: string } {
  if (!raw) return { type: legacyType };
  try {
    const value = JSON.parse(raw) as { type?: string; subtype?: string; label?: string };
    return { type: value.type || legacyType, subtype: value.subtype, label: value.label };
  } catch {
    return { type: legacyType };
  }
}

function defaultNodeName(type: string, subtype?: string, label?: string) {
  if (type === "service") return "新服务";
  if (type === "middleware" && (!subtype || subtype === "generic")) return "新中间件";
  if (type === "external") return "新外部服务";
  if (label) return `新${label}`;
  if (type === "middleware" && subtype && subtype !== "generic") return `新${subtype}`;
  return nodeTypeLabel(type);
}
