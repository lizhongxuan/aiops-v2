import type { WorkflowEdge, WorkflowGraph, WorkflowNode } from "../types/workflow";

export interface FlowNode {
  id: string;
  label: string;
  position: { x: number; y: number };
  className: string;
  data: {
    nodeType: string;
    action?: string;
    status?: string;
  };
}

export interface FlowEdge {
  id: string;
  source: string;
  target: string;
  label?: string;
  className: string;
}

export function toFlowNodes(graph: WorkflowGraph | null): FlowNode[] {
  return (graph?.nodes || []).map(toFlowNode);
}

export function toFlowEdges(graph: WorkflowGraph | null): FlowEdge[] {
  return (graph?.edges || []).map(toFlowEdge);
}

function toFlowNode(node: WorkflowNode): FlowNode {
  const status = node.state?.status;
  return {
    id: node.id,
    label: node.label || node.step_name || node.id,
    position: node.position,
    className: ["runner-node", status ? `is-${status}` : ""].filter(Boolean).join(" "),
    data: {
      nodeType: node.type,
      action: node.step?.action || node.handler?.action,
      status,
    },
  };
}

function toFlowEdge(edge: WorkflowEdge): FlowEdge {
  const status = edge.state?.status;
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.kind,
    className: ["runner-edge", status ? `is-${status}` : ""].filter(Boolean).join(" "),
  };
}
