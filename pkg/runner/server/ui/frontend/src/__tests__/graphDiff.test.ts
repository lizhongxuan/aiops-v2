import { describe, expect, it } from "vitest";
import type { WorkflowGraph } from "../types/workflow";
import { buildGraphDiffSummary } from "../utils/graphDiff";

const baseGraph: WorkflowGraph = {
  version: "v1",
  workflow: {
    version: "v0.1",
    name: "diff-test",
    vars: { service: "billing-api" },
  },
  layout: {
    direction: "LR",
    viewport: { x: 0, y: 0, zoom: 1 },
  },
  nodes: [
    { id: "start", type: "start", position: { x: 80, y: 120 }, label: "Start" },
    {
      id: "run",
      type: "action",
      position: { x: 320, y: 120 },
      step: { name: "run", action: "cmd.run", targets: ["local"], args: { cmd: "uptime" } },
    },
    { id: "end", type: "end", position: { x: 560, y: 120 }, label: "End" },
  ],
  edges: [
    { id: "start-run", source: "start", target: "run", kind: "next" },
    { id: "run-end", source: "run", target: "end", kind: "success" },
  ],
};

describe("graph diff summary", () => {
  it("separates execution, layout, and metadata changes", () => {
    const execution = cloneGraph(baseGraph);
    execution.nodes[1].step = { ...execution.nodes[1].step, args: { cmd: "hostname" } };
    expect(buildGraphDiffSummary(baseGraph, execution).sections.map((section) => [section.kind, section.changed])).toEqual([
      ["execution", true],
      ["layout", false],
      ["metadata", false],
    ]);

    const layout = cloneGraph(baseGraph);
    layout.nodes[1].position = { x: 360, y: 220 };
    expect(buildGraphDiffSummary(baseGraph, layout).sections.map((section) => [section.kind, section.changed])).toEqual([
      ["execution", false],
      ["layout", true],
      ["metadata", false],
    ]);

    const metadata = cloneGraph(baseGraph);
    metadata.workflow.vars = { service: "orders-api" };
    expect(buildGraphDiffSummary(baseGraph, metadata).sections.map((section) => [section.kind, section.changed])).toEqual([
      ["execution", false],
      ["layout", false],
      ["metadata", true],
    ]);
  });

  it("reports concrete changed paths for review", () => {
    const next = cloneGraph(baseGraph);
    next.edges[1] = { ...next.edges[1], kind: "failure" };
    next.nodes[1].position = { x: 360, y: 120 };

    const summary = buildGraphDiffSummary(baseGraph, next);

    expect(summary.changed).toBe(true);
    expect(summary.sections.find((section) => section.kind === "execution")?.paths).toEqual(["edges.run-end.kind"]);
    expect(summary.sections.find((section) => section.kind === "layout")?.paths).toEqual(["nodes.run.position.x"]);
  });
});

function cloneGraph(input: WorkflowGraph): WorkflowGraph {
  return JSON.parse(JSON.stringify(input)) as WorkflowGraph;
}
