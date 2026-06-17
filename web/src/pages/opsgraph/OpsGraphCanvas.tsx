import {
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
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
import type { OpsGraphRecord } from "./opsGraphTypes";
import { buildCanvasModel, nodeTypeLabel } from "./opsGraphViewModel";

const nodeTypes: NodeTypes = {
  opsgraphNode: OpsGraphCanvasNode,
  opsgraphGroup: OpsGraphCanvasGroup,
};

const edgeTypes: EdgeTypes = {
  opsgraphEdge: OpsGraphCanvasEdge,
};

type OpsGraphCanvasProps = {
  graph: OpsGraphRecord;
  onCreateNode?: (node: { type: string; name: string; position: { x: number; y: number }; parentId?: string }) => void;
  onCreateRelationship?: (relationship: { from: string; to: string; type: string }) => void;
  onSaveLayout?: (nodes: Array<{ id: string; position?: { x: number; y: number }; collapsed?: boolean }>, viewport?: { x: number; y: number; zoom: number }) => void;
};

export function OpsGraphCanvas({ graph, onCreateNode, onCreateRelationship, onSaveLayout }: OpsGraphCanvasProps) {
  const model = useMemo(() => buildCanvasModel(graph), [graph]);
  const [nodes, setNodes, onNodesChange] = useNodesState(model.nodes);
  const [edges, setEdges] = useEdgesState(model.edges);

  useEffect(() => {
    setNodes(model.nodes);
    setEdges(model.edges);
  }, [model.nodes, model.edges, setEdges, setNodes]);

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    const type = event.dataTransfer.getData("application/x-opsgraph-node-type");
    if (!type) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    onCreateNode?.({
      type,
      name: defaultNodeName(type),
      position: { x: event.clientX - bounds.left, y: event.clientY - bounds.top },
    });
  }

  function handleConnect(connection: Connection) {
    if (!connection.source || !connection.target) return;
    onCreateRelationship?.({ from: connection.source, to: connection.target, type: "depends_on" });
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
          onNodesChange={onNodesChange}
          onNodeDragStop={(_, node) => handleNodeDragStop(node)}
        >
          <Background gap={18} size={1} />
          <Controls />
        </ReactFlow>
      </div>
    </ReactFlowProvider>
  );
}

function defaultNodeName(type: string) {
  return nodeTypeLabel(type);
}
