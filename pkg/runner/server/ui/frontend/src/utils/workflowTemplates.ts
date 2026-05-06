import type { WorkflowDefinition, WorkflowGraph, WorkflowNode } from "../types/workflow";

export type WorkflowTemplateKind = "cmd-run-basic" | "shell-run-basic" | "manual-approval-basic";

export interface WorkflowTemplateInput {
  kind: WorkflowTemplateKind;
  name: string;
  version: string;
  description?: string;
  labels?: Record<string, string>;
}

export interface WorkflowCreateMetadata {
  name: string;
  version: string;
  description?: string;
}

export function createWorkflowGraphFromTemplate(input: WorkflowTemplateInput): WorkflowGraph {
  const workflow = workflowMetadata(input);
  if (input.kind === "shell-run-basic") {
    return {
      version: "v1",
      workflow,
      layout: { direction: "LR", viewport: { x: 0, y: 0, zoom: 1 } },
      nodes: [
        startNode(),
        {
          id: "run-shell",
          type: "action",
          label: "Run shell",
          position: { x: 320, y: 120 },
          step_id: "run-shell",
          step_name: "run-shell",
          step: {
            id: "run-shell",
            name: "run-shell",
            action: "shell.run",
            args: { script: "echo hello" },
          },
        },
        endNode({ x: 600, y: 120 }),
      ],
      edges: [
        { id: "start-run-shell", source: "start", target: "run-shell", kind: "next" },
        { id: "run-shell-end", source: "run-shell", target: "end", kind: "next" },
      ],
    };
  }

  if (input.kind === "manual-approval-basic") {
    return {
      version: "v1",
      workflow,
      layout: { direction: "LR", viewport: { x: 0, y: 0, zoom: 1 } },
      nodes: [
        startNode(),
        {
          id: "approve",
          type: "manual_approval",
          label: "Approve",
          position: { x: 300, y: 120 },
          step_id: "approve",
          step_name: "approve",
          step: {
            id: "approve",
            name: "approve",
            action: "manual.approval",
          },
          approval: {
            subjects: ["ops"],
            timeout: "30m",
            on_timeout: "reject",
          },
        },
        {
          id: "run-command",
          type: "action",
          label: "Run command",
          position: { x: 560, y: 120 },
          step_id: "run-command",
          step_name: "run-command",
          step: {
            id: "run-command",
            name: "run-command",
            action: "cmd.run",
            args: { cmd: "echo hello" },
          },
        },
        endNode({ x: 820, y: 120 }),
      ],
      edges: [
        { id: "start-approve", source: "start", target: "approve", kind: "next" },
        { id: "approve-run-command", source: "approve", target: "run-command", kind: "approval_approved" },
        { id: "run-command-end", source: "run-command", target: "end", kind: "next" },
      ],
    };
  }

  return {
    version: "v1",
    workflow,
    layout: { direction: "LR", viewport: { x: 0, y: 0, zoom: 1 } },
    nodes: [
      startNode(),
      {
        id: "run-command",
        type: "action",
        label: "Run command",
        position: { x: 320, y: 120 },
        step_id: "run-command",
        step_name: "run-command",
        step: {
          id: "run-command",
          name: "run-command",
          action: "cmd.run",
          args: { cmd: "echo hello" },
        },
      },
      endNode({ x: 600, y: 120 }),
    ],
    edges: [
      { id: "start-run-command", source: "start", target: "run-command", kind: "next" },
      { id: "run-command-end", source: "run-command", target: "end", kind: "next" },
    ],
  };
}

export function prepareWorkflowGraphForCreate(source: WorkflowGraph, metadata: WorkflowCreateMetadata): WorkflowGraph {
  const graph = cloneGraph(source);
  graph.workflow = {
    ...graph.workflow,
    name: metadata.name.trim(),
    version: metadata.version.trim() || graph.workflow.version || "v0.1",
    description: metadata.description?.trim() || undefined,
  };
  graph.ui = stripResourceVersion(graph.ui);
  graph.nodes = graph.nodes.map((node) => {
    const next = cloneNode(node);
    delete next.state;
    return next;
  });
  graph.edges = graph.edges.map((edge) => {
    const next = { ...edge };
    delete next.state;
    return next;
  });
  return graph;
}

function workflowMetadata(input: WorkflowTemplateInput): WorkflowDefinition {
  return {
    version: input.version.trim() || "v0.1",
    name: input.name.trim(),
    ...(input.description?.trim() ? { description: input.description.trim() } : {}),
  };
}

function startNode(): WorkflowNode {
  return {
    id: "start",
    type: "start",
    label: "Start",
    position: { x: 80, y: 120 },
  };
}

function endNode(position: { x: number; y: number }): WorkflowNode {
  return {
    id: "end",
    type: "end",
    label: "End",
    position,
  };
}

function stripResourceVersion(ui: Record<string, unknown> | undefined) {
  if (!ui) return undefined;
  const next = { ...ui };
  delete next.resource_version;
  return Object.keys(next).length > 0 ? next : undefined;
}

function cloneGraph(graph: WorkflowGraph): WorkflowGraph {
  return JSON.parse(JSON.stringify(graph)) as WorkflowGraph;
}

function cloneNode(node: WorkflowNode): WorkflowNode {
  return JSON.parse(JSON.stringify(node)) as WorkflowNode;
}
