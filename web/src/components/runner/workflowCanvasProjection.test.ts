import { describe, expect, it } from "vitest";

import type { RunnerGraph } from "./canvasGraphAdapter";
import { projectWorkflowLayout } from "@/runner/layout/workflowElkLayout";
import type { WorkflowCandidateFact } from "@/runner/workspace/workflowWorkspaceTypes";
import {
  projectWorkflowCanvas,
  tryProjectWorkflowCanvas,
} from "./workflowCanvasProjection";

const basis = {
  resourceVersion: "rv-7",
  semanticHash: "sha256:semantic-7",
  catalogVersion: "catalog-1",
  actionSchemaDigest: "sha256:actions",
  referenceSnapshotHash: "sha256:references",
  environmentBindingVersion: "environment-1",
};

function candidate(
  operations: WorkflowCandidateFact["changeSet"]["operations"],
  options: {
    state?: WorkflowCandidateFact["changeSet"]["state"];
    diagnostics?: WorkflowCandidateFact["changeSet"]["validation"]["diagnostics"];
    projection?: WorkflowCandidateFact["projection"];
  } = {},
): WorkflowCandidateFact {
  return {
    candidate: {
      id: "candidate-1",
      workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
      currentChangeSetId: "change-1",
      currentRevision: 3,
      currentPreviewHash: "sha256:preview-3",
      state: options.state === "conflict" ? "conflict" : "ready",
      expiresAt: "2030-01-01T00:00:00Z",
    },
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2",
      id: "change-1",
      workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
      candidateId: "candidate-1",
      candidateRevision: 3,
      actor: "agent",
      intent: "update the workflow",
      base: { resourceVersion: "rv-7", semanticHash: "sha256:semantic-7", layoutHash: "sha256:layout-7" },
      operations,
      previewHash: "sha256:preview-3",
      validation: { basis, valid: !(options.diagnostics || []).some((item) => item.severity === "error"), diagnostics: options.diagnostics || [] },
      state: options.state || "previewed",
      createdAt: "2026-07-21T00:00:00Z",
      expiresAt: "2030-01-01T00:00:00Z",
    },
    projection: options.projection,
  };
}

function committedGraph(): RunnerGraph {
  return {
    version: "rv-7",
    workflow: { name: "workflow-a", enabled: false },
    nodes: [
      { id: "start", type: "start", label: "Start", position: { x: 0, y: 0 }, ports: [{ id: "next", type: "output" }] },
      { id: "check", type: "action", label: "Check", position: { x: 280, y: 0 }, step: { action: "check", args: { threshold: 70 } } },
      { id: "end", type: "end", label: "End", position: { x: 560, y: 0 } },
    ],
    edges: [
      { id: "edge-start", source: "start", source_port: "next", target: "check", target_port: "in" },
      { id: "edge-end", source: "check", source_port: "next", target: "end", target_port: "in" },
    ],
  };
}

describe("workflow canvas layout projection", () => {
  it("projects only update_layout positions and preserves every semantic field", () => {
    const graph: RunnerGraph = {
      workflow: { name: "guard" },
      nodes: [
        { id: "start", type: "start", label: "开始", position: { x: 0, y: 0 } },
        { id: "check", type: "action", step: { action: "check", args: { threshold: 80 } }, position: { x: 10, y: 10 } },
      ],
      edges: [{ id: "edge", source: "start", source_port: "next", target: "check", target_port: "in" }],
    };

    const projected = projectWorkflowLayout(graph, {
      op: "update_layout",
      layout: { nodes: [{ nodeId: "check", position: { x: 320, y: 160 } }] },
    });

    expect(projected).not.toBe(graph);
    expect(projected.nodes?.find((node) => node.id === "start")?.position).toEqual({ x: 0, y: 0 });
    expect(projected.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 320, y: 160 });
    expect(projected.nodes?.find((node) => node.id === "check")?.step).toEqual(graph.nodes?.[1].step);
    expect(projected.edges).toBe(graph.edges);
    expect(graph.nodes?.[1].position).toEqual({ x: 10, y: 10 });
  });

  it("fails closed for duplicate or unknown node layout entries", () => {
    const graph: RunnerGraph = { nodes: [{ id: "start", position: { x: 0, y: 0 } }] };
    expect(() => projectWorkflowLayout(graph, {
      op: "update_layout",
      layout: { nodes: [{ nodeId: "missing", position: { x: 1, y: 2 } }] },
    })).toThrow(/unknown workflow node/i);
    expect(() => projectWorkflowLayout(graph, {
      op: "update_layout",
      layout: { nodes: [
        { nodeId: "start", position: { x: 1, y: 2 } },
        { nodeId: "start", position: { x: 3, y: 4 } },
      ] },
    })).toThrow(/duplicate workflow node/i);
  });
});

describe("committed + candidate canvas projection", () => {
  it("[UI-007][W1] overlays added, changed and deleted ghosts without mutating committed graph", () => {
    const committed = committedGraph();
    const before = JSON.stringify(committed);
    const fact = candidate([
      { op: "update_node", nodeId: "check", set: { label: "Check capacity", "step.args.threshold": 85 } },
      { op: "add_node", clientRef: "notify", node: { id: "notify", type: "action", label: "Notify", position: { x: 560, y: 140 }, step: { action: "notify" } } },
      { op: "delete_node", nodeId: "end", cascadeEdges: true },
      { op: "add_edge", clientRef: "edge-notify", edge: { id: "edge-notify", source: "check", sourcePort: "next", target: "notify", targetPort: "in" } },
    ], { projection: { nodes: [{ id: "forged", type: "action", position: { x: 1, y: 1 } }] } });

    const view = projectWorkflowCanvas(committed, fact, { committedResourceVersion: "rv-7" });

    expect(view.status).toBe("candidate");
    expect(view.graph).not.toBe(committed);
    expect(view.graph.nodes?.map((node) => node.id)).toEqual(["start", "check", "end", "notify"]);
    expect(view.graph.nodes?.find((node) => node.id === "check")?.step?.args).toEqual({ threshold: 85 });
    expect(view.nodes.check).toMatchObject({ change: "changed", fieldPaths: ["label", "step.args.threshold"] });
    expect(view.nodes.notify).toMatchObject({ change: "added" });
    expect(view.nodes.end).toMatchObject({ change: "deleted" });
    expect(view.edges["edge-end"]).toMatchObject({ change: "deleted" });
    expect(view.edges["edge-notify"]).toMatchObject({ change: "added" });
    expect(view.graph.nodes?.some((node) => node.id === "forged")).toBe(false);
    expect(JSON.stringify(committed)).toBe(before);
  });

  it("[UI-008][W2] projects edge field deltas and typed warning/conflict markers", () => {
    const warning = projectWorkflowCanvas(committedGraph(), candidate([
      { op: "update_edge", edgeId: "edge-start", set: { kind: "success", condition: "healthy" } },
    ], {
      diagnostics: [{ code: "EDGE_WARNING", severity: "warning", message: "路径仍需确认", edgeId: "edge-start", field: "condition" }],
    }));
    expect(warning.edges["edge-start"]).toMatchObject({
      change: "changed",
      blocked: "warning",
      fieldPaths: ["condition", "kind"],
      messages: ["路径仍需确认"],
    });

    const conflict = projectWorkflowCanvas(committedGraph(), candidate([
      { op: "update_node", nodeId: "check", set: { label: "Conflicting label" } },
    ], { state: "conflict" }));
    expect(conflict.nodes.check).toMatchObject({ change: "changed", blocked: "conflict" });
    expect(conflict.status).toBe("candidate");
  });

  it("[UI-009][W0] clearing candidate returns the exact committed graph and no overlay", () => {
    const committed: RunnerGraph = { workflow: { name: "blank" }, nodes: [], edges: [] };
    const view = projectWorkflowCanvas(committed);
    expect(view.graph).toBe(committed);
    expect(view.nodes).toEqual({});
    expect(view.edges).toEqual({});
    expect(view.status).toBe("committed");
  });

  it("[UI-010][W3] preserves hidden End and applies only explicit layout positions", () => {
    const committed = committedGraph();
    const view = projectWorkflowCanvas(committed, candidate([
      { op: "update_layout", layout: { nodes: [{ nodeId: "check", position: { x: 320, y: 180 } }] } },
    ]));
    expect(view.graph.nodes?.find((node) => node.id === "start")?.position).toEqual({ x: 0, y: 0 });
    expect(view.graph.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 320, y: 180 });
    expect(view.graph.nodes?.find((node) => node.id === "end")).toBeTruthy();
    expect(view.nodes.check.fieldPaths).toEqual(["position"]);
  });

  it("[UI-011][W6] fails closed atomically for stale, duplicate and dangling deltas", () => {
    const committed = committedGraph();
    committed.nodes![1] = {
      ...committed.nodes![1],
      ports: [{ id: "in", type: "input" }, { id: "success", type: "output" }, { id: "failure", type: "output" }],
      ui: { pinned: true },
    };
    const stale = tryProjectWorkflowCanvas(committed, candidate([
      { op: "update_node", nodeId: "check", set: { label: "must not appear" } },
    ]), { committedResourceVersion: "rv-new" });
    expect(stale).toMatchObject({ status: "blocked", graph: committed });
    expect(stale.graph.nodes?.find((node) => node.id === "check")?.label).toBe("Check");

    for (const fact of [
      candidate([{ op: "add_node", clientRef: "check", node: { id: "check", type: "action", position: { x: 1, y: 1 } } }]),
      candidate([{ op: "add_edge", clientRef: "dangling", edge: { id: "dangling", source: "missing", target: "check" } }]),
      candidate([{ op: "update_node", nodeId: "missing", set: { label: "no" } }]),
      candidate([], { diagnostics: [{ code: "UNKNOWN", severity: "error", message: "unknown", nodeId: "missing" }] }),
    ]) {
      const result = tryProjectWorkflowCanvas(committed, fact);
      expect(result.status).toBe("blocked");
      expect(result.graph).toBe(committed);
    }
    const unchanged = projectWorkflowCanvas(committed, candidate([{ op: "update_node", nodeId: "check", set: { label: "Checked" } }]));
    expect(unchanged.graph.nodes?.find((node) => node.id === "check")?.ports).toEqual(committed.nodes?.[1].ports);
    expect(unchanged.graph.nodes?.find((node) => node.id === "check")?.ui).toEqual({ pinned: true });
    expect(unchanged.graph.nodes?.find((node) => node.type === "end")?.id).toBe("end");
  });

  it("[UI-014][W4] projects 500 nodes and 700 edges without copying on highlight-only inputs", () => {
    const nodes = Array.from({ length: 500 }, (_, index) => ({ id: `node-${index}`, type: index === 499 ? "end" : "action", position: { x: index * 8, y: (index % 20) * 90 } }));
    const edges = Array.from({ length: 700 }, (_, index) => ({ id: `edge-${index}`, source: `node-${index % 499}`, target: `node-${(index + 1) % 500}` }));
    const committed: RunnerGraph = { nodes, edges };
    const started = performance.now();
    const view = projectWorkflowCanvas(committed, candidate([
      { op: "update_node", nodeId: "node-250", set: { label: "Changed" } },
      { op: "update_layout", layout: { nodes: [{ nodeId: "node-251", position: { x: 42, y: 84 } }] } },
    ]));
    expect(performance.now() - started).toBeLessThan(750);
    expect(view.graph.nodes).toHaveLength(500);
    expect(view.graph.edges).toHaveLength(700);
    expect(view.graph.nodes?.[0]).toBe(committed.nodes?.[0]);
    expect(view.graph.nodes?.[250]).not.toBe(committed.nodes?.[250]);
    expect(view.graph.nodes?.[300]).toBe(committed.nodes?.[300]);
  });
});
