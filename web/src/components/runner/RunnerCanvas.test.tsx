import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkflowCandidateFact } from "@/runner/workspace/workflowWorkspaceTypes";
import { WORKFLOW_CANVAS_LOCATE_EVENT } from "@/runner/workflowCanvasEvents";
import {
  RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT,
  RunnerCanvas,
  workflowOverlayViewportShift,
} from "./RunnerCanvas";
import type { RunnerGraph } from "./canvasGraphAdapter";

const mocks = vi.hoisted(() => ({
  flowProps: null as Record<string, any> | null,
  fitView: vi.fn(),
  setCenter: vi.fn(),
  getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
  setViewport: vi.fn(),
}));

vi.mock("@xyflow/react", async () => {
  const React = await import("react");
  return {
    Background: () => null,
    BaseEdge: () => null,
    ConnectionMode: { Loose: "loose" },
    ControlButton: ({ children, ...props }: any) => <button {...props}>{children}</button>,
    Controls: ({ children }: any) => <div>{children}</div>,
    EdgeLabelRenderer: ({ children }: any) => <>{children}</>,
    Handle: () => null,
    MarkerType: { ArrowClosed: "arrowclosed" },
    MiniMap: () => null,
    Position: { Left: "left", Right: "right" },
    ReactFlow: React.forwardRef(function MockReactFlow(props: any, _ref) {
      mocks.flowProps = props;
      return <div data-testid="mock-react-flow">{props.children}</div>;
    }),
    ReactFlowProvider: ({ children }: any) => <>{children}</>,
    addEdge: (edge: any, edges: any[]) => [...edges, edge],
    getBezierPath: () => ["M0 0", 0, 0],
    useEdgesState: (initial: any[]) => {
      const [value, setValue] = React.useState(initial);
      return [value, setValue, vi.fn()];
    },
    useNodesState: (initial: any[]) => {
      const [value, setValue] = React.useState(initial);
      return [value, setValue, vi.fn()];
    },
    useReactFlow: () => ({
      fitView: mocks.fitView,
      setCenter: mocks.setCenter,
      getViewport: mocks.getViewport,
      setViewport: mocks.setViewport,
      screenToFlowPosition: ({ x, y }: { x: number; y: number }) => ({ x, y }),
    }),
  };
});

const graph: RunnerGraph = {
  workflow: { name: "guard" },
  nodes: [
    { id: "start", type: "start", position: { x: 0, y: 0 }, ports: [{ id: "next", type: "output" }] },
    { id: "check", type: "action", position: { x: 260, y: 0 }, ports: [{ id: "in", type: "input" }, { id: "next", type: "output" }] },
  ],
  edges: [{ id: "edge-start", source: "start", source_port: "next", target: "check", target_port: "in", kind: "next" }],
};

function candidate(): WorkflowCandidateFact {
  return {
    candidate: {
      id: "candidate-1",
      workflowKey: { tenantId: "tenant-a", workflowId: "guard" },
      currentChangeSetId: "change-1",
      currentRevision: 1,
      currentPreviewHash: "preview-1",
      state: "ready",
      expiresAt: "2030-01-01T00:00:00Z",
    },
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2",
      id: "change-1",
      workflowKey: { tenantId: "tenant-a", workflowId: "guard" },
      candidateId: "candidate-1",
      candidateRevision: 1,
      actor: "agent",
      intent: "update check",
      base: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      operations: [
        { op: "update_node", nodeId: "check", set: { label: "Check now" } },
        { op: "update_workflow", set: { description: "Capacity guard" } },
      ],
      previewHash: "preview-1",
      validation: {
        basis: { resourceVersion: "rv-1", semanticHash: "semantic-1", catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" },
        valid: true,
        diagnostics: [],
      },
      state: "previewed",
      createdAt: "2026-07-21T00:00:00Z",
      expiresAt: "2030-01-01T00:00:00Z",
    },
  };
}

describe("workflowOverlayViewportShift", () => {
  it("keeps a 24px safe inset without moving nodes that are already clear", () => {
    expect(workflowOverlayViewportShift(
      { right: 876 },
      { left: 900 },
    )).toBe(0);
    expect(workflowOverlayViewportShift(
      { right: 900 },
      { left: 920 },
    )).toBe(4);
    expect(workflowOverlayViewportShift(
      { right: 960 },
      { left: 920 },
    )).toBe(64);
  });
});

describe("RunnerCanvas guarded interaction contract", () => {
  let container: HTMLDivElement;
  let root: Root;
  let onUpdateGraph: ReturnType<typeof vi.fn>;
  let onSelectNode: ReturnType<typeof vi.fn>;
  let onOpenNodeConfig: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mocks.flowProps = null;
    mocks.fitView.mockReset();
    mocks.setCenter.mockReset();
    onUpdateGraph = vi.fn();
    onSelectNode = vi.fn();
    onOpenNodeConfig = vi.fn();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  async function renderCanvas(extra: Record<string, unknown> = {}) {
    await act(async () => root.render(
      <RunnerCanvas
        graph={graph}
        actions={[]}
        onUpdateGraph={onUpdateGraph}
        onSelectNode={onSelectNode}
        onOpenNodeConfig={onOpenNodeConfig}
        {...extra}
      />,
    ));
  }

  it("selects and opens the edge menu on double-click without deleting", async () => {
    await renderCanvas();
    const event = {
      clientX: 80,
      clientY: 60,
      preventDefault: vi.fn(),
      stopPropagation: vi.fn(),
      currentTarget: { getBoundingClientRect: () => ({ left: 0, top: 0, width: 700, height: 500 }) },
    };
    await act(async () => mocks.flowProps?.onEdgeDoubleClick(event, { id: "edge-start" }));
    expect(onUpdateGraph).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="runner-edge-menu"]')).not.toBeNull();
  });

  it("uses the keyboard resolver for explicit delete/configuration/clear/fit and real undo callbacks", async () => {
    const onDeleteNode = vi.fn();
    const onUndo = vi.fn();
    const onRedo = vi.fn();
    await renderCanvas({ selectedNodeId: "check", onDeleteNode, onUndo, onRedo });

    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Delete", bubbles: true })));
    expect(onDeleteNode).toHaveBeenCalledWith("check");
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })));
    expect(onOpenNodeConfig).toHaveBeenCalledWith("check");
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(onSelectNode).toHaveBeenCalledWith("");
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "f", bubbles: true })));
    expect(mocks.fitView).toHaveBeenCalled();
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "z", ctrlKey: true, bubbles: true })));
    expect(onUndo).toHaveBeenCalledTimes(1);
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "y", ctrlKey: true, bubbles: true })));
    expect(onRedo).toHaveBeenCalledTimes(1);
  });

  it("makes the clicked business node the restorable focus target", async () => {
    await renderCanvas();
    const nodeElement = document.createElement("div");
    nodeElement.dataset.testid = "canvas-node-check";
    container.appendChild(nodeElement);
    await act(async () => mocks.flowProps?.onNodeClick({ target: nodeElement }, { id: "check" }));
    expect(nodeElement.tabIndex).toBe(0);
    expect(document.activeElement).toBe(nodeElement);
    expect(onSelectNode).toHaveBeenCalledWith("check");
  });

  it("restores Follow AI to the first precisely locatable Candidate object", async () => {
    await renderCanvas({ candidate: candidate(), committedResourceVersion: "rv-1" });

    await act(async () => window.dispatchEvent(new CustomEvent(WORKFLOW_CANVAS_LOCATE_EVENT, {
      detail: { changedObjectCount: 1 },
    })));

    expect(onSelectNode).toHaveBeenCalledWith("check");
    expect(mocks.setCenter).toHaveBeenCalledWith(362, 46, {
      zoom: 0.86,
      duration: 420,
    });
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("shows the exact connectFlowEdge reason during drag and never emits an invalid graph", async () => {
    await renderCanvas();
    await act(async () => {
      expect(mocks.flowProps?.isValidConnection({ source: "check", sourceHandle: "next", target: "check", targetHandle: "in" })).toBe(false);
    });
    expect(container.textContent).toContain("不能连接到自身");
    await act(async () => mocks.flowProps?.onConnect({ source: "check", sourceHandle: "next", target: "check", targetHandle: "in" }));
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("routes Candidate node movement only through the explicit revise callback", async () => {
    const onReviseCandidateGraph = vi.fn();
    await renderCanvas({
      candidate: candidate(),
      committedResourceVersion: "rv-1",
      onReviseCandidateGraph,
    });

    expect(mocks.flowProps?.nodesDraggable).toBe(true);
    await act(async () => mocks.flowProps?.onNodeDragStop({}, {
      id: "check",
      position: { x: 520, y: 180 },
    }));

    expect(onReviseCandidateGraph).toHaveBeenCalledTimes(1);
    const [previous, next] = onReviseCandidateGraph.mock.calls[0] as [RunnerGraph, RunnerGraph];
    expect(previous.nodes?.find((node) => node.id === "check")).toMatchObject({
      label: "Check now",
      position: { x: 260, y: 0 },
    });
    expect(next.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 520, y: 180 });
    expect(onUpdateGraph).not.toHaveBeenCalled();
    expect(graph.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 260, y: 0 });
  });

  it("routes Candidate connect and reconnect only through revise without mutating committed edges", async () => {
    const onReviseCandidateGraph = vi.fn();
    const graphWithEnd: RunnerGraph = {
      ...graph,
      nodes: [
        ...(graph.nodes || []),
        { id: "end", type: "end", position: { x: 520, y: 0 }, ports: [{ id: "in", type: "input" }] },
      ],
    };
    await renderCanvas({
      graph: graphWithEnd,
      candidate: candidate(),
      committedResourceVersion: "rv-1",
      onReviseCandidateGraph,
    });

    expect(mocks.flowProps?.nodesConnectable).toBe(true);
    expect(mocks.flowProps?.edgesReconnectable).toBe(true);
    const connection = { source: "check", sourceHandle: "next", target: "end", targetHandle: "in" };
    await act(async () => {
      expect(mocks.flowProps?.isValidConnection(connection)).toBe(true);
      mocks.flowProps?.onConnect(connection);
    });
    expect(onReviseCandidateGraph).toHaveBeenCalledTimes(1);
    expect((onReviseCandidateGraph.mock.calls[0][1] as RunnerGraph).edges).toContainEqual(expect.objectContaining({
      source: "check",
      target: "end",
    }));

    await act(async () => mocks.flowProps?.onReconnect(
      { id: "edge-start" },
      { source: "start", sourceHandle: "next", target: "end", targetHandle: "in" },
    ));
    expect(onReviseCandidateGraph).toHaveBeenCalledTimes(2);
    const reconnected = onReviseCandidateGraph.mock.calls[1][1] as RunnerGraph;
    expect(reconnected.edges).toContainEqual(expect.objectContaining({ id: "edge-start", source: "start", target: "end" }));
    expect(reconnected.edges).not.toContainEqual(expect.objectContaining({ source: "start", target: "check" }));
    expect(onUpdateGraph).not.toHaveBeenCalled();
    expect(graphWithEnd.edges).toEqual(graph.edges);
  });

  it("keeps Candidate interactions read-only when no revise callback is supplied", async () => {
    await renderCanvas({ candidate: candidate(), committedResourceVersion: "rv-1" });

    expect(mocks.flowProps?.nodesDraggable).toBe(false);
    expect(mocks.flowProps?.nodesConnectable).toBe(false);
    expect(mocks.flowProps?.edgesReconnectable).toBe(false);
    await act(async () => mocks.flowProps?.onNodeDragStop({}, { id: "check", position: { x: 999, y: 999 } }));
    await act(async () => mocks.flowProps?.onConnect({ source: "start", sourceHandle: "next", target: "check", targetHandle: "in" }));
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("keeps committed node movement on the existing manual update path", async () => {
    await renderCanvas();
    await act(async () => mocks.flowProps?.onNodeDragStop({}, {
      id: "check",
      position: { x: 420, y: 80 },
    }));

    expect(onUpdateGraph).toHaveBeenCalledTimes(1);
    expect(onUpdateGraph.mock.calls[0][0].nodes?.find((node: { id: string }) => node.id === "check")?.position).toEqual({ x: 420, y: 80 });
  });

  it("persists a completed keyboard position change as one authoritative move", async () => {
    await renderCanvas();

    await act(async () => mocks.flowProps?.onNodesChange([{
      id: "check",
      type: "position",
      position: { x: 265, y: 5 },
      dragging: false,
    }]));

    expect(onUpdateGraph).toHaveBeenCalledTimes(1);
    expect(onUpdateGraph.mock.calls[0][0].nodes?.find((node: { id: string }) => node.id === "check")?.position).toEqual({ x: 265, y: 5 });
  });

  it("routes a completed Candidate keyboard move through one revision", async () => {
    const onReviseCandidateGraph = vi.fn();
    await renderCanvas({
      candidate: candidate(),
      committedResourceVersion: "rv-1",
      onReviseCandidateGraph,
    });

    await act(async () => mocks.flowProps?.onNodesChange([{
      id: "check",
      type: "position",
      position: { x: 265, y: 5 },
      dragging: false,
    }]));

    expect(onReviseCandidateGraph).toHaveBeenCalledTimes(1);
    const [previous, next] = onReviseCandidateGraph.mock.calls[0] as [RunnerGraph, RunnerGraph];
    expect(previous.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 260, y: 0 });
    expect(next.nodes?.find((node) => node.id === "check")?.position).toEqual({ x: 265, y: 5 });
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("keeps the Apply shortcut enabled for an exact server-persisted active/ready revision", async () => {
    const revised = candidate();
    revised.candidate.state = "active";
    revised.candidate.currentRevision = 2;
    revised.changeSet.candidateRevision = 2;
    // Runner persistence calls this state `ready`; the transport contract
    // historically calls the same reviewed state `previewed`.
    revised.changeSet.state = "ready" as typeof revised.changeSet.state;
    const commands: unknown[] = [];
    const listener = (event: Event) => commands.push((event as CustomEvent).detail);
    window.addEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, listener);
    await renderCanvas({ candidate: revised, committedResourceVersion: "rv-1" });

    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", {
      key: "Enter",
      metaKey: true,
      bubbles: true,
    })));

    expect(commands).toEqual([{
      command: "apply_candidate",
      candidateId: "candidate-1",
      changeSetId: "change-1",
      previewHash: "preview-1",
    }]);

    revised.candidate.state = "unexpected-state";
    await renderCanvas({ candidate: revised, committedResourceVersion: "rv-1" });
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", {
      key: "Enter",
      metaKey: true,
      bubbles: true,
    })));
    window.removeEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, listener);
    expect(commands).toHaveLength(1);
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("keeps a pointer drag on its single drag-stop transaction", async () => {
    await renderCanvas();

    await act(async () => mocks.flowProps?.onNodeDragStart({}, {
      id: "check",
      position: { x: 260, y: 0 },
    }));
    await act(async () => mocks.flowProps?.onNodesChange([{
      id: "check",
      type: "position",
      position: { x: 320, y: 40 },
      dragging: true,
    }]));
    await act(async () => mocks.flowProps?.onNodesChange([{
      id: "check",
      type: "position",
      position: { x: 420, y: 80 },
      dragging: false,
    }]));
    await act(async () => mocks.flowProps?.onNodeDragStop({}, {
      id: "check",
      position: { x: 420, y: 80 },
    }));

    expect(onUpdateGraph).toHaveBeenCalledTimes(1);
    expect(onUpdateGraph.mock.calls[0][0].nodes?.find((node: { id: string }) => node.id === "check")?.position).toEqual({ x: 420, y: 80 });
  });

  it("does not persist selection, measurement or fit-view changes as node moves", async () => {
    await renderCanvas();

    await act(async () => mocks.flowProps?.onNodesChange([
      { id: "check", type: "select", selected: true },
      { id: "check", type: "dimensions", dimensions: { width: 204, height: 118 } },
    ]));
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "f", bubbles: true })));

    expect(mocks.fitView).toHaveBeenCalledTimes(1);
    expect(onUpdateGraph).not.toHaveBeenCalled();
  });

  it("appends a palette node by replacing the unique edge into end", async () => {
    const chain: RunnerGraph = {
      workflow: { name: "append" },
      nodes: [
        { id: "start", type: "start", position: { x: 0, y: 0 } },
        { id: "script-shell", type: "action", position: { x: 260, y: 0 }, step: { action: "script.shell" } },
        { id: "end", type: "end", position: { x: 780, y: 0 } },
      ],
      edges: [
        { id: "start-script", source: "start", target: "script-shell", kind: "next" },
        { id: "script-end", source: "script-shell", target: "end", kind: "next" },
      ],
    };
    await renderCanvas({
      graph: chain,
      actions: [{ action: "builtin.tcp_ping", label: "TCP Ping", category: "network", execution_available: true }],
    });

    await act(async () => (container.querySelector('[data-testid="runner-node-picker-toggle"]') as HTMLButtonElement).click());
    await act(async () => (container.querySelector('[data-testid="catalog-action-builtin-tcp_ping"]') as HTMLButtonElement).click());

    expect(onUpdateGraph).toHaveBeenCalledTimes(1);
    const next = onUpdateGraph.mock.calls[0][0] as RunnerGraph;
    expect(next.nodes?.map((node) => node.id)).toContain("builtin-tcp-ping");
    expect(next.edges?.some((edge) => edge.id === "script-end")).toBe(false);
    expect(next.edges).toEqual(expect.arrayContaining([
      expect.objectContaining({ source: "script-shell", target: "builtin-tcp-ping" }),
      expect.objectContaining({ source: "builtin-tcp-ping", target: "end" }),
    ]));
  });

  it("shows an error and emits no graph when palette append has multiple terminal edges", async () => {
    const ambiguous: RunnerGraph = {
      workflow: { name: "ambiguous" },
      nodes: [
        { id: "start", type: "start" },
        { id: "left", type: "action" },
        { id: "right", type: "action" },
        { id: "end", type: "end" },
      ],
      edges: [
        { id: "left-end", source: "left", target: "end", kind: "next" },
        { id: "right-end", source: "right", target: "end", kind: "next" },
      ],
    };
    await renderCanvas({
      graph: ambiguous,
      actions: [{ action: "builtin.tcp_ping", label: "TCP Ping", category: "network", execution_available: true }],
    });

    await act(async () => (container.querySelector('[data-testid="runner-node-picker-toggle"]') as HTMLButtonElement).click());
    await act(async () => (container.querySelector('[data-testid="catalog-action-builtin-tcp_ping"]') as HTMLButtonElement).click());

    expect(onUpdateGraph).not.toHaveBeenCalled();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("唯一");
  });

  it("emits Candidate navigation and Apply events without stealing dialog input", async () => {
    const commands: unknown[] = [];
    const listener = (event: Event) => commands.push((event as CustomEvent).detail);
    window.addEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, listener);
    await renderCanvas({ candidate: candidate(), committedResourceVersion: "rv-1" });
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", altKey: true, bubbles: true })));
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp", altKey: true, bubbles: true })));
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", ctrlKey: true, bubbles: true })));
    expect(commands).toEqual([
      expect.objectContaining({ command: "next_candidate_change", candidateId: "candidate-1", operationIndex: 0 }),
      expect.objectContaining({ command: "previous_candidate_change", candidateId: "candidate-1", operationIndex: 1 }),
      expect.objectContaining({ command: "apply_candidate", candidateId: "candidate-1" }),
    ]);

    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const input = document.createElement("input");
    dialog.appendChild(input);
    document.body.appendChild(dialog);
    await act(async () => input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", altKey: true, bubbles: true })));
    expect(commands).toHaveLength(3);
    dialog.remove();
    window.removeEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, listener);
  });

  it("disables ordinary edge deletion when no recoverable backend callback exists", async () => {
    await renderCanvas();
    await act(async () => mocks.flowProps?.onEdgeClick({ stopPropagation: vi.fn() }, { id: "edge-start" }));
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Delete", bubbles: true })));
    expect(onUpdateGraph).not.toHaveBeenCalled();
    expect(container.textContent).toContain("服务端可撤销事务");
  });
});
