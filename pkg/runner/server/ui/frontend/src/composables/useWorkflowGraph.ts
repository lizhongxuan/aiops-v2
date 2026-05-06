import type { Edge, Node } from "@vue-flow/core";
import { computed } from "vue";
import type { WorkflowEdge, WorkflowGraph, WorkflowNode } from "../types/workflow";

export function useWorkflowGraph(graph: () => WorkflowGraph | null) {
  const nodes = computed<Node[]>(() => {
    return (graph()?.nodes || []).map(toFlowNode);
  });

  const edges = computed<Edge[]>(() => {
    return (graph()?.edges || []).map(toFlowEdge);
  });

  return { nodes, edges };
}

function toFlowNode(node: WorkflowNode): Node {
  const status = node.state?.status;
  return {
    id: node.id,
    type: "default",
    label: node.label || node.step_name || node.id,
    position: node.position,
    class: ["runner-node", status ? `is-${status}` : ""].filter(Boolean).join(" "),
    data: {
      nodeType: node.type,
      action: node.step?.action || node.handler?.action,
      status,
    },
  };
}

function toFlowEdge(edge: WorkflowEdge): Edge {
  const status = edge.state?.status;
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.kind,
    animated: status === "selected" || edge.kind === "success" || edge.kind === "next",
    class: ["runner-edge", status ? `is-${status}` : ""].filter(Boolean).join(" "),
  };
}
