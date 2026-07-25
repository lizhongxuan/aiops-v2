import { describe, expect, it } from "vitest";

import type { RunnerGraph } from "@/components/runner/canvasGraphAdapter";
import {
  buildWorkflowElkGraph,
  createWorkflowLayoutOperation,
} from "./workflowElkLayout";
import { runWorkflowElkLayout } from "./workflowLayoutWorker";

const chainGraph: RunnerGraph = {
  nodes: [
    { id: "end", type: "end", position: { x: 40, y: 40 } },
    { id: "check", type: "action", position: { x: 20, y: 20 } },
    { id: "start", type: "start", position: { x: 0, y: 0 } },
  ],
  edges: [
    { id: "check-end", source: "check", source_port: "next", target: "end", target_port: "in" },
    { id: "start-check", source: "start", source_port: "next", target: "check", target_port: "in" },
  ],
};

describe("workflow ELK layout", () => {
  it("returns one strict deterministic update_layout operation without mutating Graph", async () => {
    const before = structuredClone(chainGraph);
    const first = await createWorkflowLayoutOperation(chainGraph, { direction: "RIGHT" }, runWorkflowElkLayout);
    const second = await createWorkflowLayoutOperation(structuredClone(chainGraph), { direction: "RIGHT" }, runWorkflowElkLayout);

    expect(first).toEqual(second);
    expect(Object.keys(first)).toEqual(["op", "layout"]);
    expect(Object.keys(first.layout)).toEqual(["nodes"]);
    expect(first.op).toBe("update_layout");
    expect(first.layout.nodes.map((item) => item.nodeId)).toEqual(["check", "end", "start"]);
    expect(first.layout.nodes.every((item) => Number.isFinite(item.position.x) && Number.isFinite(item.position.y))).toBe(true);
    expect(chainGraph).toEqual(before);

    const positions = Object.fromEntries(first.layout.nodes.map((item) => [item.nodeId, item.position]));
    expect(positions.start.x).toBeLessThan(positions.check.x);
    expect(positions.check.x).toBeLessThan(positions.end.x);
  });

  it("supports DOWN and preserves pinned and out-of-scope positions by omitting them", async () => {
    const operation = await createWorkflowLayoutOperation(chainGraph, {
      direction: "DOWN",
      scope: { nodeIds: ["check", "end"] },
      pinnedNodeIds: ["check"],
    }, runWorkflowElkLayout);

    expect(operation.layout.nodes.map((item) => item.nodeId)).toEqual(["end"]);
    expect(operation.layout.nodes[0].position.y).toBeGreaterThan(chainGraph.nodes?.find((node) => node.id === "check")?.position?.y || 0);
  });

  it("maps W6 parallel/join/group/subflow types and stable multi-handle ports", () => {
    const graph: RunnerGraph = {
      nodes: [
        { id: "parallel", type: "parallel", ports: { inputs: [{ id: "in" }], outputs: [{ id: "b" }, { id: "a" }] } },
        { id: "group", type: "group", parentId: "parallel", ports: [{ id: "in", type: "input" }, { id: "next", type: "output" }] },
        { id: "subflow", type: "subflow", ports: [{ id: "in", type: "input" }, { id: "success", type: "output" }] },
        { id: "join", type: "join", ports: { inputs: [{ id: "right" }, { id: "left" }], outputs: [{ id: "next" }] } },
      ],
      edges: [
        { id: "a", source: "parallel", source_port: "a", target: "group", target_port: "in" },
        { id: "b", source: "parallel", source_port: "b", target: "subflow", target_port: "in" },
        { id: "group-join", source: "group", source_port: "next", target: "join", target_port: "left" },
        { id: "subflow-join", source: "subflow", source_port: "success", target: "join", target_port: "right" },
      ],
    };

    const right = buildWorkflowElkGraph(graph, "RIGHT");
    const down = buildWorkflowElkGraph(graph, "DOWN");
    const parallel = right.children?.find((node) => node.id === "parallel");
    const group = right.children?.find((node) => node.id === "group");
    const subflow = right.children?.find((node) => node.id === "subflow");
    const join = right.children?.find((node) => node.id === "join");

    expect(right.children?.map((node) => node.id)).toEqual(["group", "join", "parallel", "subflow"]);
    expect(parallel?.ports?.map((port) => port.id)).toEqual([
      "parallel:input:in",
      "parallel:output:a",
      "parallel:output:b",
    ]);
    expect(parallel?.ports?.map((port) => port.layoutOptions?.["org.eclipse.elk.port.side"])).toEqual(["WEST", "EAST", "EAST"]);
    expect(down.children?.find((node) => node.id === "parallel")?.ports?.map((port) => port.layoutOptions?.["org.eclipse.elk.port.side"])).toEqual(["NORTH", "SOUTH", "SOUTH"]);
    expect(group?.width).toBeGreaterThan(parallel?.width || 0);
    expect(subflow?.height).toBeGreaterThan(0);
    expect(join?.ports).toHaveLength(3);
    expect(right.edges?.find((edge) => edge.id === "a")?.sources).toEqual(["parallel:output:a"]);
    expect(right.edges?.find((edge) => edge.id === "a")?.targets).toEqual(["group:input:in"]);
  });

  it("keeps the W4 500-node/700-edge seeded recipe non-overlapping and stable", async () => {
    const graph = largeBranchingGraph(500, 700);
    const first = await createWorkflowLayoutOperation(graph, { direction: "RIGHT" }, runWorkflowElkLayout);
    const second = await createWorkflowLayoutOperation(graph, { direction: "RIGHT" }, runWorkflowElkLayout);
    expect(second).toEqual(first);

    const boxes = first.layout.nodes.map((item) => ({ ...item.position, id: item.nodeId, width: 204, height: 118 }));
    const collisions: string[] = [];
    for (let index = 0; index < boxes.length; index += 1) {
      for (let other = index + 1; other < boxes.length; other += 1) {
        if (overlaps(boxes[index], boxes[other])) collisions.push(`${boxes[index].id}:${boxes[other].id}`);
      }
    }
    expect(collisions).toEqual([]);
  // This intentionally runs the full 500/700 layout twice to prove
  // determinism. Keep a case-local budget so parallel Vitest workers cannot
  // turn expected CPU contention into a suite-level flaky timeout.
  }, 45_000);

  it("keeps deterministic depth layout as an explicit capability fallback", async () => {
    const operation = await createWorkflowLayoutOperation(chainGraph, { direction: "RIGHT", engine: "depth" });
    const failedWorkerFallback = await createWorkflowLayoutOperation(chainGraph, { direction: "RIGHT" }, async () => { throw new Error("worker failed"); });
    expect(operation).toEqual({
      op: "update_layout",
      layout: {
        nodes: [
          { nodeId: "check", position: { x: 380, y: 160 } },
          { nodeId: "end", position: { x: 680, y: 160 } },
          { nodeId: "start", position: { x: 80, y: 160 } },
        ],
      },
    });
    expect(failedWorkerFallback).toEqual(operation);
  });
});

function largeBranchingGraph(count: number, edgeCount: number): RunnerGraph {
  const nodes = Array.from({ length: count }, (_, index) => ({
    id: `node-${String(index).padStart(2, "0")}`,
    type: index === 0 ? "start" : index === count - 1 ? "join" : index % 7 === 0 ? "parallel" : index % 9 === 0 ? "subflow" : "action",
    ports: [{ id: "in", type: "input" }, { id: "next", type: "output" }],
  }));
  const edges = nodes.slice(1).map((node, index) => ({
    id: `edge-${index}`,
    source: nodes[Math.max(0, Math.floor(index / 2))].id,
    source_port: "next",
    target: node.id,
    target_port: "in",
  }));
  const used = new Set(edges.map((edge) => `${edge.source}:${edge.target}`));
  for (let index = edges.length; index < edgeCount; index += 1) {
    const targetIndex = 2 + ((index * 17) % (count - 2));
    let sourceIndex = (index * 31) % targetIndex;
    while (used.has(`${nodes[sourceIndex].id}:${nodes[targetIndex].id}`)) sourceIndex = (sourceIndex + 1) % targetIndex;
    used.add(`${nodes[sourceIndex].id}:${nodes[targetIndex].id}`);
    edges.push({ id: `edge-${index}`, source: nodes[sourceIndex].id, source_port: "next", target: nodes[targetIndex].id, target_port: "in" });
  }
  return { nodes, edges };
}

function overlaps(left: { x: number; y: number; width: number; height: number }, right: { x: number; y: number; width: number; height: number }) {
  return left.x < right.x + right.width && left.x + left.width > right.x && left.y < right.y + right.height && left.y + left.height > right.y;
}
