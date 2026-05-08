import { describe, expect, it } from "vitest";
import {
  addCatalogActionNode,
  addGraphEdge,
  connectFlowEdge,
  flowConnectionToGraphEdge,
  getGraphUpstreamNodeIds,
  graphToCanvasModel,
  graphToFlowModel,
  updateGraphEdgeKind,
  validationIssueToCanvasFocus,
  validateGraphConnection,
  updateGraphNodePosition,
} from "./canvasGraphAdapter";

const graph = {
  version: "v1",
  workflow: { name: "demo" },
  nodes: [
    { id: "start", type: "start", position: { x: 40, y: 120 }, label: "Start" },
    { id: "pre-check", type: "action", position: { x: 260, y: 120 }, step: { name: "pre-check", action: "cmd.run" } },
  ],
  edges: [{ id: "start-pre-check", source: "start", target: "pre-check", kind: "next" }],
};

describe("canvasGraphAdapter", () => {
  it("converts backend graph into canvas nodes and edges without mutating backend graph", () => {
    const canvas = graphToCanvasModel(graph, { selectedNodeId: "pre-check" });

    expect(canvas.nodes).toEqual([
      expect.objectContaining({ id: "start", type: "runner-node", selected: false }),
      expect.objectContaining({ id: "pre-check", type: "runner-node", selected: true }),
    ]);
    expect(canvas.nodes[1].data.node.step.action).toBe("cmd.run");
    expect(canvas.edges).toEqual([
      expect.objectContaining({ id: "start-pre-check", source: "start", target: "pre-check", type: "runner-edge" }),
    ]);
    expect(graph.nodes[1]).not.toHaveProperty("data");
    expect(graph.edges[0]).not.toHaveProperty("type", "runner-edge");
  });

  it("adds a catalog action as a backend graph action node", () => {
    const action = { action: "shell.run", label: "Shell Script", defaults: { script: "set -e\ndf -h", env: { LC_ALL: "C" } } };
    const next = addCatalogActionNode(
      graph,
      action,
      { x: 480, y: 180 },
    );

    expect(next).not.toBe(graph);
    expect(next.nodes).toHaveLength(3);
    expect(next.nodes[2]).toMatchObject({
      id: "shell-run",
      type: "action",
      position: { x: 780, y: 180 },
      label: "Shell Script",
      step: {
        name: "shell-run",
        action: "shell.run",
        targets: ["local"],
        args: { script: "set -e\ndf -h", env: { LC_ALL: "C" } },
      },
    });
    expect(next.nodes[2].step.args).not.toBe(action.defaults);
    expect(Array.isArray(next.nodes[2].ports)).toBe(true);
    expect(next.nodes[2].ports).toEqual([
      { id: "in", type: "input", label: "输入" },
      { id: "next", type: "output", label: "下一步" },
      { id: "failure", type: "output", label: "失败" },
    ]);
    expect(graph.nodes).toHaveLength(2);
  });

  it("defaults executable action nodes to local so new drafts can validate and run without inventory setup", () => {
    const next = addCatalogActionNode(
      { version: "v1", workflow: { name: "demo" }, nodes: [], edges: [] },
      { action: "cmd.run", title: "Command", defaults: { cmd: "uptime" } },
      { x: 120, y: 160 },
    );

    expect(next.nodes[0].step.targets).toEqual(["local"]);
  });

  it("reads backend port arrays when converting graph nodes to flow canvas data", () => {
    const flow = graphToFlowModel({
      ...graph,
      nodes: [
        {
          id: "custom",
          type: "action",
          position: { x: 10, y: 20 },
          step: { name: "custom", action: "cmd.run" },
          ports: [
            { id: "input-a", type: "input", label: "输入 A" },
            { id: "success", type: "output", label: "成功" },
            { id: "failure", type: "output", label: "失败" },
          ],
        },
      ],
      edges: [],
    });

    expect(flow.nodes[0].data.ports.inputs).toEqual([{ id: "input-a", label: "输入 A" }]);
    expect(flow.nodes[0].data.ports.outputs).toEqual([
      { id: "success", label: "成功" },
      { id: "failure", label: "失败" },
    ]);
  });

  it("stagger positions when multiple catalog actions are dropped at the same point", () => {
    const first = addCatalogActionNode(
      { version: "v1", workflow: { name: "demo" }, nodes: [], edges: [] },
      { action: "cmd.run", label: "Command" },
      { x: 120, y: 160 },
    );
    const second = addCatalogActionNode(first, { action: "shell.run", label: "Shell Script" }, { x: 120, y: 160 });

    expect(first.nodes[0].position).toEqual({ x: 120, y: 160 });
    expect(second.nodes[1].position).toEqual({ x: 420, y: 160 });
  });

  it("connects nodes by appending a backend graph edge", () => {
    const next = addGraphEdge(graph, { source: "pre-check", target: "restore", kind: "success" });

    expect(next.edges.at(-1)).toEqual({
      id: "pre-check-restore-success",
      source: "pre-check",
      target: "restore",
      kind: "success",
    });
    expect(next.edges.at(-1)).not.toHaveProperty("data");
  });

  it("updates node position through graph contract instead of storing canvas-only layout", () => {
    const next = updateGraphNodePosition(graph, "pre-check", { x: 320, y: 240 });

    expect(next.nodes.find((node) => node.id === "pre-check").position).toEqual({ x: 320, y: 240 });
    expect(next.nodes.find((node) => node.id === "pre-check")).not.toHaveProperty("positionAbsolute");
  });

  it("converts backend graph into flow canvas nodes and semantic handles", () => {
    const flow = graphToFlowModel(graph, { selectedNodeId: "start" });

    expect(flow.nodes[0]).toMatchObject({
      id: "start",
      type: "runner-node",
      selected: true,
      position: { x: 40, y: 120 },
    });
    expect(flow.edges[0]).toMatchObject({
      source: "start",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
      label: "next",
    });
    expect(flow.nodes[0].data.node).toMatchObject({ id: "start", type: "start" });
  });

  it("turns flow canvas connections into graph edges without duplicating existing semantic edges", () => {
    const empty = { ...graph, edges: [] };
    const next = flowConnectionToGraphEdge(empty, {
      source: "start",
      target: "pre-check",
      sourceHandle: "success",
      targetHandle: "in",
    });
    const duplicate = flowConnectionToGraphEdge(next, {
      source: "start",
      target: "pre-check",
      sourceHandle: "success",
      targetHandle: "in",
    });

    expect(next.edges).toHaveLength(1);
    expect(next.edges[0]).toMatchObject({
      source: "start",
      target: "pre-check",
      kind: "success",
      source_port: "success",
      target_port: "in",
    });
    expect(duplicate.edges).toHaveLength(1);
  });

  it("rejects invalid flow canvas connections with actionable validation errors", () => {
    const validation = validateGraphConnection(graph, {
      source: "pre-check",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    });
    const badPort = connectFlowEdge(graph, {
      source: "pre-check",
      target: "start",
      sourceHandle: "approved",
      targetHandle: "in",
    });

    expect(validation).toMatchObject({
      valid: false,
      code: "self_connection",
    });
    expect(badPort.graph).toEqual(graph);
    expect(badPort.error).toMatchObject({
      valid: false,
      code: "invalid_source_port",
    });
  });

  it("does not treat an existing edge as a duplicate of itself during flow canvas rendering", () => {
    const graphWithEdge = {
      ...graph,
      edges: [
        {
          id: "start-pre-check-next",
          source: "start",
          target: "pre-check",
          kind: "next",
          source_port: "next",
          target_port: "in",
        },
      ],
    };

    expect(validateGraphConnection(graphWithEdge, {
      id: "start-pre-check-next",
      source: "start",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    })).toEqual({ valid: true });
  });

  it("updates an existing edge kind and keeps graph-only edge data", () => {
    const next = updateGraphEdgeKind(graph, "start-pre-check", "failure");

    expect(next.edges[0]).toMatchObject({
      id: "start-pre-check",
      source: "start",
      target: "pre-check",
      kind: "failure",
      source_port: "failure",
      target_port: "in",
    });
    expect(next.edges[0]).not.toHaveProperty("sourceHandle");
    expect(graph.edges[0].kind).toBe("next");
  });

  it("returns all upstream graph node ids in stable graph order", () => {
    const branchingGraph = {
      ...graph,
      nodes: [
        ...graph.nodes,
        { id: "restore", type: "action", step: { action: "shell.run" } },
        { id: "verify", type: "action", step: { action: "cmd.run" } },
        { id: "notify", type: "notify", step: { action: "notify.send" } },
      ],
      edges: [
        { id: "start-pre-check", source: "start", target: "pre-check", kind: "next" },
        { id: "pre-check-restore", source: "pre-check", target: "restore", kind: "next" },
        { id: "restore-verify", source: "restore", target: "verify", kind: "next" },
        { id: "start-notify", source: "start", target: "notify", kind: "failure" },
        { id: "notify-verify", source: "notify", target: "verify", kind: "next" },
      ],
    };

    expect(getGraphUpstreamNodeIds(branchingGraph, "verify")).toEqual(["start", "pre-check", "restore", "notify"]);
    expect(getGraphUpstreamNodeIds(branchingGraph, "start")).toEqual([]);
  });

  it("maps condition and approval nodes to semantic flow canvas handles", () => {
    const semanticGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        { id: "gate", type: "condition", position: { x: 20, y: 40 }, step: { action: "condition.branch" } },
        { id: "approve", type: "approval", position: { x: 240, y: 40 }, step: { action: "approval.wait" } },
      ],
      edges: [
        { id: "gate-approve-if", source: "gate", target: "approve", kind: "if", source_port: "if", target_port: "in" },
      ],
    };

    const flow = graphToFlowModel(semanticGraph);

    expect(flow.nodes[0].data.ports.outputs.map((port) => port.id)).toEqual(["if", "else"]);
    expect(flow.nodes[1].data.ports.outputs.map((port) => port.id)).toEqual(["approved", "rejected"]);
    expect(flow.edges[0]).toMatchObject({
      sourceHandle: "if",
      targetHandle: "in",
      label: "if",
    });
  });

  it("maps structured validation issues to canvas focus targets", () => {
    expect(
      validationIssueToCanvasFocus(graph, {
        code: "step_action_required",
        node_id: "pre-check",
        field: "nodes[1].step.action",
        suggestion: "Choose an action.",
      }),
    ).toEqual({
      kind: "node",
      nodeId: "pre-check",
      edgeId: "",
      field: "step.action",
      suggestion: "Choose an action.",
    });

    expect(
      validationIssueToCanvasFocus(graph, {
        code: "edge_source_port_missing",
        edge_id: "start-pre-check",
        field: "edges[0].source_port",
      }),
    ).toEqual({
      kind: "edge",
      nodeId: "",
      edgeId: "start-pre-check",
      field: "source_port",
      suggestion: "",
    });
  });
});
