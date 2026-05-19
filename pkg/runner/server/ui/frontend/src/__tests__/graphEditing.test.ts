import { describe, expect, it } from "vitest";
import type { WorkflowGraph } from "../types/workflow";
import { autoLayoutGraph, connectGraphNodes, createControlNode, deleteNodeFromGraph } from "../utils/graphEditing";

const baseGraph: WorkflowGraph = {
  version: "v1",
  workflow: { version: "v0.1", name: "editing" },
  nodes: [
    { id: "start", type: "start", position: { x: 10, y: 20 } },
    { id: "run", type: "action", position: { x: 120, y: 20 }, step: { name: "run", action: "script.shell" } },
    { id: "end", type: "end", position: { x: 240, y: 20 } },
  ],
  edges: [{ id: "edge-start-run", source: "start", target: "run", kind: "next" }],
};

describe("graph editing helpers", () => {
  it("creates control nodes with type defaults", () => {
    const join = createControlNode("join", baseGraph, { x: 300, y: 140 });

    expect(join).toMatchObject({
      id: "join",
      type: "join",
      label: "Join",
      position: { x: 300, y: 140 },
      join: { strategy: "all_success" },
    });

    expect(createControlNode("handler", baseGraph, { x: 360, y: 220 })).toMatchObject({
      id: "handler",
      type: "handler",
      handler_name: "handler",
      handler: { name: "handler", action: "script.shell", args: { script: "echo notify" } },
    });
  });

  it("connects nodes while rejecting invalid graph edges", () => {
    const connected = connectGraphNodes(baseGraph, "run", "end");

    expect(connected.error).toBeUndefined();
    expect(connected.graph.edges).toEqual([
      expect.objectContaining({ source: "start", target: "run" }),
      expect.objectContaining({ source: "run", target: "end", kind: "next" }),
    ]);

    expect(connectGraphNodes(connected.graph, "run", "run").error).toContain("cannot connect to itself");
    expect(connectGraphNodes(connected.graph, "run", "end").error).toContain("already exists");
    expect(connectGraphNodes(connected.graph, "end", "run").error).toContain("End nodes");
  });

  it("deletes selected nodes and removes their edges", () => {
    const result = deleteNodeFromGraph(baseGraph, "run");

    expect(result.error).toBeUndefined();
    expect(result.graph.nodes.map((node) => node.id)).toEqual(["start", "end"]);
    expect(result.graph.edges).toEqual([]);
    expect(deleteNodeFromGraph(result.graph, "start").error).toContain("last start");
  });

  it("auto layouts graph left to right by reachable layer", () => {
    const graph = connectGraphNodes(baseGraph, "run", "end").graph;
    const laidOut = autoLayoutGraph(graph);

    expect(laidOut.layout?.direction).toBe("LR");
    expect(laidOut.nodes.find((node) => node.id === "start")?.position.x).toBeLessThan(
      laidOut.nodes.find((node) => node.id === "run")?.position.x || 0,
    );
    expect(laidOut.nodes.find((node) => node.id === "run")?.position.x).toBeLessThan(
      laidOut.nodes.find((node) => node.id === "end")?.position.x || 0,
    );
  });

  it("keeps 100-node graph auto layout bounded and stable", () => {
    const nodes: WorkflowGraph["nodes"] = [
      { id: "start", type: "start", position: { x: 0, y: 0 } },
      ...Array.from({ length: 100 }, (_, index) => ({
        id: `step-${index}`,
        type: "action" as const,
        position: { x: 0, y: 0 },
        step: { name: `step-${index}`, action: "script.shell", args: { script: "true" } },
      })),
      { id: "end", type: "end", position: { x: 0, y: 0 } },
    ];
    const edges: WorkflowGraph["edges"] = [
      { id: "start-step-0", source: "start", target: "step-0", kind: "next" },
      ...Array.from({ length: 99 }, (_, index) => ({
        id: `step-${index}-step-${index + 1}`,
        source: `step-${index}`,
        target: `step-${index + 1}`,
        kind: "success" as const,
      })),
      { id: "step-99-end", source: "step-99", target: "end", kind: "success" },
    ];
    const largeGraph: WorkflowGraph = {
      version: "v1",
      workflow: { version: "v0.1", name: "large-layout" },
      nodes,
      edges,
    };

    const startedAt = performance.now();
    const laidOut = autoLayoutGraph(largeGraph);
    const durationMs = performance.now() - startedAt;

    expect(laidOut.nodes).toHaveLength(102);
    expect(laidOut.layout?.direction).toBe("LR");
    expect(laidOut.nodes.find((node) => node.id === "start")?.position.x).toBeLessThan(
      laidOut.nodes.find((node) => node.id === "step-99")?.position.x || 0,
    );
    expect(durationMs).toBeLessThan(1000);
  });
});
