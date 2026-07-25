import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WorkflowEditorShell,
  workflowCommandForGraphChange,
  workflowCommandsForGraphChange,
  workflowReviewFieldDiffs,
} from "./WorkflowEditorShell";
import type { WorkflowCandidateFact } from "./workspace/workflowWorkspaceTypes";

const mocks = vi.hoisted(() => ({
  setSelection: vi.fn(),
  canvasProps: null as Record<string, any> | null,
  state: {} as Record<string, any>,
  transport: {} as Record<string, any>,
  commands: {
    executeBatch: vi.fn(),
    addNode: vi.fn(),
    updateNode: vi.fn(),
    deleteNode: vi.fn(),
    connectEdge: vi.fn(),
    updateEdge: vi.fn(),
    disconnectEdge: vi.fn(),
    moveNodes: vi.fn(),
  },
}));

describe("WorkflowEditorShell atomic manual command mapping", () => {
  const chain = {
    workflow: { name: "guard" },
    nodes: [
      { id: "start", type: "start", position: { x: 0, y: 0 } },
      { id: "check", type: "action", position: { x: 260, y: 0 }, step: { action: "cmd.run" } },
      { id: "end", type: "end", position: { x: 780, y: 0 } },
    ],
    edges: [
      { id: "start-check", source: "start", target: "check", kind: "next" },
      { id: "check-end", source: "check", target: "end", kind: "next" },
    ],
  };

  it("maps edge insertion into disconnect, add, reconnect, and layout commands in order", () => {
    const next = {
      ...chain,
      nodes: [
        chain.nodes[0],
        chain.nodes[1],
        { ...chain.nodes[2], position: { x: 1100, y: 0 } },
        { id: "notify", type: "action", position: { x: 520, y: 0 }, step: { action: "notify.send" } },
      ],
      edges: [
        chain.edges[0],
        { id: "check-notify", source: "check", target: "notify", kind: "next" },
        { id: "notify-end", source: "notify", target: "end", kind: "next" },
      ],
    };

    expect(workflowCommandsForGraphChange(chain, next)).toEqual([
      { kind: "disconnect_edge", edgeId: "check-end" },
      { kind: "add_node", node: expect.objectContaining({ id: "notify", type: "action" }) },
      { kind: "connect_edge", edge: { id: "check-notify", source: "check", target: "notify", kind: "next" } },
      { kind: "connect_edge", edge: { id: "notify-end", source: "notify", target: "end", kind: "next" } },
      { kind: "move", layout: { nodes: [{ nodeId: "end", position: { x: 1100, y: 0 } }] } },
    ]);
  });

  it("maps a bridged simple-chain deletion into one delete plus one connect batch", () => {
    const next = {
      ...chain,
      nodes: [chain.nodes[0], chain.nodes[2]],
      edges: [{ id: "start-end-next", source: "start", target: "end", kind: "next" }],
    };

    expect(workflowCommandsForGraphChange(chain, next)).toEqual([
      { kind: "delete_node", nodeId: "check", cascadeEdges: true },
      { kind: "connect_edge", edge: { id: "start-end-next", source: "start", target: "end", kind: "next" } },
    ]);
  });

  it("keeps single update and move mapping compatible", () => {
    expect(workflowCommandForGraphChange(chain, {
      ...chain,
      nodes: chain.nodes.map((node) => node.id === "check" ? { ...node, label: "Check now" } : node),
    })).toEqual({ kind: "update_node", nodeId: "check", nodePatch: { label: "Check now" } });
    expect(workflowCommandForGraphChange(chain, {
      ...chain,
      nodes: chain.nodes.map((node) => node.id === "check" ? { ...node, position: { x: 300, y: 40 } } : node),
    })).toEqual({ kind: "move", layout: { nodes: [{ nodeId: "check", position: { x: 300, y: 40 } }] } });
  });

  it("derives exact Review field diffs from committed and Candidate projections", () => {
    const fact = candidate();
    const projected = {
      ...committedGraph,
      nodes: committedGraph.nodes.map((node) => node.id === "check" ? { ...node, label: "Candidate check" } : node),
    };

    expect(workflowReviewFieldDiffs(fact.changeSet, committedGraph, projected)).toEqual([{
      operationIndex: 0,
      path: "/nodes/check/label",
      before: "Committed check",
      after: "Candidate check",
    }]);
  });
});

vi.mock("@/runner/transport/WorkflowTransportActivityContext", () => ({
  useWorkflowTransportActivity: () => ({
    busy: mocks.transport.status === "working" || Object.keys(mocks.transport.runtimeLiveness?.activeTurns || {}).length > 0,
    canStop: false,
    canRetry: false,
  }),
}));

vi.mock("@/components/runner/RunnerCanvas", async () => {
  const React = await import("react");
  return {
    RunnerCanvas: (props: { onSelectNode: (id: string) => void; onOpenNodeConfig: (id: string) => void }) => {
      mocks.canvasProps = props;
      return React.createElement("div", {},
        React.createElement("button", { type: "button", "data-testid": "mock-select", onClick: () => props.onSelectNode("check") }, "select"),
        React.createElement("button", { type: "button", "data-testid": "mock-clear", onClick: () => props.onSelectNode("") }, "clear"),
        React.createElement("button", { type: "button", "data-testid": "mock-config", onClick: () => props.onOpenNodeConfig("check") }, "config"),
      );
    },
  };
});

vi.mock("@/runner/workspace/WorkflowWorkspaceProvider", () => ({
  useWorkflowWorkspace: () => ({
    state: mocks.state,
    commands: mocks.commands,
    pendingCommands: [],
    unsavedError: undefined,
    setSelection: mocks.setSelection,
  }),
}));

describe("WorkflowEditorShell selection scope", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    if (!globalThis.CSS) Object.defineProperty(globalThis, "CSS", { configurable: true, value: {} });
    if (!globalThis.CSS.escape) Object.defineProperty(globalThis.CSS, "escape", { configurable: true, value: (value: string) => String(value).replace(/[^a-zA-Z0-9_-]/g, "\\$&") });
    mocks.setSelection.mockReset();
    mocks.canvasProps = null;
    mocks.state = workspaceState();
    mocks.transport = transportState();
    Object.values(mocks.commands).forEach((command) => {
      command.mockReset();
      command.mockResolvedValue(undefined);
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("keeps Workspace scope synchronized while preserving Page callbacks", async () => {
    const onSelectNode = vi.fn();
    const onOpenNodeConfig = vi.fn();
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={{ nodes: [{ id: "check", label: "Check", position: { x: 0, y: 0 } }], edges: [] }}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onSelectNode={onSelectNode}
        onOpenNodeConfig={onOpenNodeConfig}
        onToggleFullscreen={() => {}}
      />,
    ));

    await act(async () => (container.querySelector('[data-testid="mock-select"]') as HTMLButtonElement).click());
    expect(mocks.setSelection).toHaveBeenLastCalledWith({ nodeIds: ["check"], edgeIds: [], anchorId: "check" });
    expect(onSelectNode).toHaveBeenCalledWith("check");
    await act(async () => (container.querySelector('[data-testid="mock-clear"]') as HTMLButtonElement).click());
    expect(mocks.setSelection).toHaveBeenLastCalledWith({ nodeIds: [], edgeIds: [] });
    expect(onSelectNode).toHaveBeenCalledWith("");
    await act(async () => (container.querySelector('[data-testid="mock-config"]') as HTMLButtonElement).click());
    expect(mocks.setSelection).toHaveBeenLastCalledWith({ nodeIds: ["check"], edgeIds: [], anchorId: "check" });
    expect(onOpenNodeConfig).toHaveBeenCalledWith("check");
  });

  it("renders node operations separately from the workflow drawer and clears both selections on close", async () => {
    const onSelectNode = vi.fn();
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId="check"
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        aiTransportPanel={<div data-testid="mock-ai-panel">AI workspace</div>}
        onSelectNode={onSelectNode}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
        renderNodeInspector={({ node, onCloseNode }) => (
          <div data-testid="mock-node-inspector">
            <span>{node?.label}</span>
            <input data-testid="mock-node-draft" defaultValue="keep this draft" />
            <button type="button" data-testid="mock-node-close" onClick={onCloseNode}>close</button>
          </div>
        )}
      />,
    ));

    const nodeSidebar = container.querySelector('[data-testid="workflow-node-sidebar"]') as HTMLElement;
    const workflowDrawer = container.querySelector('[data-testid="workflow-context-dock"]') as HTMLElement;
    expect(nodeSidebar).not.toBeNull();
    expect(nodeSidebar.getAttribute("aria-label")).toBe("节点操作");
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(workflowDrawer.hidden).toBe(true);

    await act(async () => (container.querySelector('[data-testid="workflow-workbench-trigger"]') as HTMLButtonElement).click());
    expect(workflowDrawer.hidden).toBe(false);
    expect(workflowDrawer.getAttribute("aria-label")).toBe("工作流面板");
    expect(nodeSidebar.getAttribute("aria-hidden")).toBe("true");
    expect(nodeSidebar.hasAttribute("inert")).toBe(true);
    expect((container.querySelector('[data-testid="mock-node-draft"]') as HTMLInputElement).value).toBe("keep this draft");

    await act(async () => (container.querySelector('[data-testid="workflow-context-drawer-close"]') as HTMLButtonElement).click());
    expect(workflowDrawer.hidden).toBe(true);
    expect(nodeSidebar.getAttribute("aria-hidden")).not.toBe("true");
    expect(nodeSidebar.hasAttribute("inert")).toBe(false);
    expect((container.querySelector('[data-testid="mock-node-draft"]') as HTMLInputElement).value).toBe("keep this draft");

    await act(async () => (container.querySelector('[data-testid="mock-node-close"]') as HTMLButtonElement).click());
    expect(mocks.setSelection).toHaveBeenLastCalledWith({ nodeIds: [], edgeIds: [] });
    expect(onSelectNode).toHaveBeenLastCalledWith("");
  });

  it("does not render the removed Follow AI status strip", async () => {
    mocks.transport = transportState({
      status: "working",
      currentTurnId: "turn-1",
      runtimeLiveness: {
        activeTurns: { "turn-1": true }, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        aiTransportPanel={<div>AI</div>}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));
    expect(container.textContent).not.toContain("跟随 AI");
    expect(container.querySelector('[data-testid="workflow-follow-ai"]')).toBeNull();

    mocks.transport = transportState();
    mocks.state = { ...workspaceState(), task: { state: "planning" } };
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        aiTransportPanel={<div>AI</div>}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));
    expect(container.textContent).not.toContain("跟随 AI");
    expect(container.querySelector('[data-testid="workflow-follow-ai"]')).toBeNull();
  });

  it("uses one Command Bar and keeps durable changes separate from current-session runs", async () => {
    const onCheckWorkflow = vi.fn();
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        workflowTitle="guard"
        aiTransportPanel={<div data-testid="mock-ai-panel">AI workspace</div>}
        runGateFacts={{
          currentBasis: validationBasis("rv-1"),
          environment: { name: "staging", bindingVersion: "env-1" },
        }}
        durableHistoryEntries={[{
          source: "durable_transaction",
          transactionId: "txn-1",
          businessLabel: "更新检查节点",
          status: "completed",
          occurredAt: "2026-07-21T08:00:00Z",
          actorLabel: "用户",
          touchedObjectCount: 1,
          reversible: true,
        }]}
        sessionRuns={[{
          id: "run-1",
          status: "completed",
          targetLabel: "当前草稿",
          startedAt: "2026-07-21T08:05:00Z",
          durationMs: 900,
        }]}
        onCheckWorkflow={onCheckWorkflow}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));

    expect(container.querySelectorAll('[data-testid="workflow-command-bar"]')).toHaveLength(1);
    expect(container.querySelectorAll('[data-testid="workflow-primary-action"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="workflow-primary-action"]')?.getAttribute("data-action")).toBe("check");

    await act(async () => (container.querySelector('[data-testid="workflow-primary-action"]') as HTMLButtonElement).click());
    expect(onCheckWorkflow).toHaveBeenCalledWith({
      kind: "saved_draft",
      workflowId: "guard",
      resourceVersion: "rv-1",
      semanticHash: "semantic-1",
    });

    await act(async () => (container.querySelector('[data-testid="workflow-workbench-trigger"]') as HTMLButtonElement).click());
    const tabLabels = [...container.querySelectorAll('[role="tab"][data-testid^="workflow-context-tab-"]')]
      .map((tab) => tab.textContent?.trim());
    expect(tabLabels).toEqual(["AI 协作", "变更", "运行"]);
    expect(container.querySelector('[data-testid="workflow-context-tab-review"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-history"]')).toBeNull();

    const changesPanel = container.querySelector('[data-testid="workflow-context-panel-changes"]') as HTMLElement;
    const runPanel = container.querySelector('[data-testid="workflow-context-panel-run"]') as HTMLElement;
    expect(changesPanel.querySelector('[data-testid="workflow-history-panel"]')).not.toBeNull();
    expect(changesPanel.querySelector('[data-testid="workflow-run-inspector"]')).toBeNull();
    expect(runPanel.querySelector('[data-testid="workflow-run-inspector"]')).not.toBeNull();
    expect(runPanel.querySelector('[data-testid="workflow-history-panel"]')).toBeNull();

    await act(async () => (container.querySelector('[data-testid="workflow-context-tab-changes"]') as HTMLButtonElement).click());
    expect(changesPanel.textContent).toContain("更新检查节点");
    expect(changesPanel.textContent).not.toContain("当前会话");

    await act(async () => (container.querySelector('[data-testid="workflow-context-tab-run"]') as HTMLButtonElement).click());
    expect(runPanel.textContent).toContain("当前会话");
    expect(runPanel.textContent).toContain("当前草稿");
    expect(runPanel.textContent).not.toContain("更新检查节点");
  });

  it("turns a typed Chat focus into Canvas focus and routes a ghost node back to the same Candidate item", async () => {
    mocks.state = workspaceState(candidate());
    const onSelectNode = vi.fn();
    const canvasFocusEvents: unknown[] = [];
    const recordFocus = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail?.source === "canvas") canvasFocusEvents.push(detail);
    };
    window.addEventListener("aiops:workflow-surface-focus", recordFocus);
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onSelectNode={onSelectNode}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));

    await act(async () => window.dispatchEvent(new CustomEvent("aiops:workflow-surface-focus", { detail: {
      schemaVersion: "aiops.workflow.surface-focus.v1",
      requestId: "chat-focus-1",
      source: "chat",
      workflowId: "guard",
      target: { nodeId: "check" },
      candidate: { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" },
      operationIndex: 0,
    } })));

    expect(mocks.setSelection).toHaveBeenLastCalledWith({ nodeIds: ["check"], edgeIds: [], anchorId: "check" });
    expect(onSelectNode).toHaveBeenLastCalledWith("check");
    expect(mocks.canvasProps?.selectedNodeId).toBe("check");
    expect(mocks.canvasProps?.focusNodeId).toBe("check:chat-focus-1");

    await act(async () => (container.querySelector('[data-testid="mock-select"]') as HTMLButtonElement).click());
    window.removeEventListener("aiops:workflow-surface-focus", recordFocus);
    expect(canvasFocusEvents).toEqual([expect.objectContaining({
      schemaVersion: "aiops.workflow.surface-focus.v1",
      source: "canvas",
      workflowId: "guard",
      target: { nodeId: "check" },
      operationIndex: 0,
      candidate: { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" },
    })]);
  });

  it("uses the local bootstrap callback before the first authoritative revision exists", async () => {
    mocks.state = { ...workspaceState(), committed: undefined };
    const onBootstrapGraphChange = vi.fn();
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onBootstrapGraphChange={onBootstrapGraphChange}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));

    const nextGraph = {
      ...committedGraph,
      nodes: [...committedGraph.nodes, { id: "bootstrap", type: "action", position: { x: 520, y: 0 } }],
    };
    await act(async () => mocks.canvasProps?.onUpdateGraph(nextGraph));

    expect(onBootstrapGraphChange).toHaveBeenCalledWith(nextGraph);
    expect(Object.values(mocks.commands).every((command) => command.mock.calls.length === 0)).toBe(true);
  });

  it("submits a compound authoritative canvas change through one ordered batch call", async () => {
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId=""
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));
    const notify = { id: "notify", type: "action", position: { x: 130, y: 0 }, step: { action: "notify.send" } };
    const nextGraph = {
      ...committedGraph,
      nodes: [committedGraph.nodes[0], notify, committedGraph.nodes[1]],
      edges: [
        { id: "start-notify", source: "start", target: "notify", kind: "next" },
        { id: "notify-check", source: "notify", target: "check", kind: "next" },
      ],
    };

    await act(async () => {
      mocks.canvasProps?.onUpdateGraph(nextGraph);
      await Promise.resolve();
    });

    expect(mocks.commands.executeBatch).toHaveBeenCalledTimes(1);
    expect(mocks.commands.executeBatch).toHaveBeenCalledWith([
      { kind: "disconnect_edge", edgeId: "edge-start" },
      { kind: "add_node", node: expect.objectContaining({ id: "notify" }) },
      { kind: "connect_edge", edge: { id: "start-notify", source: "start", target: "notify", kind: "next" } },
      { kind: "connect_edge", edge: { id: "notify-check", source: "notify", target: "check", kind: "next" } },
    ], expect.objectContaining({ reason: "manual canvas batch" }));
    expect(mocks.commands.addNode).not.toHaveBeenCalled();
    expect(mocks.commands.disconnectEdge).not.toHaveBeenCalled();
  });

  it("requires exact confirmation for a connected canvas deletion before sending the authoritative batch", async () => {
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId="check"
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
      />,
    ));

    await act(async () => mocks.canvasProps?.onDeleteNode("check"));
    expect(mocks.commands.executeBatch).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="runner-node-delete-confirmation"]')).not.toBeNull();
    expect(container.textContent).toContain("edge-start");

    await act(async () => {
      (container.querySelector('[data-testid="runner-node-delete-confirm"]') as HTMLButtonElement).click();
      await Promise.resolve();
    });
    expect(mocks.commands.executeBatch).toHaveBeenCalledTimes(1);
    expect(mocks.commands.executeBatch).toHaveBeenCalledWith(
      expect.arrayContaining([expect.objectContaining({ kind: "delete_node", nodeId: "check" })]),
      expect.any(Object),
    );
  });

  it("uses the projected Candidate node in Inspector and routes parameter edits only to revise", async () => {
    mocks.state = workspaceState(candidate());
    const onReviseCandidateGraph = vi.fn();
    let inspectorReadOnly: boolean | undefined;
    let inspectorLabel = "";
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId="check"
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onReviseCandidateGraph={onReviseCandidateGraph}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
        renderNodeInspector={({ graph, node, readOnly, onApplyNode }) => {
          inspectorReadOnly = readOnly;
          inspectorLabel = String(node?.label || "");
          return (
            <button
              type="button"
              data-testid="revise-parameter"
              onClick={() => onApplyNode({ ...node!, step: { ...node?.step, args: { threshold: 95 } } })}
            >
              revise
            </button>
          );
        }}
      />,
    ));

    expect(inspectorReadOnly).toBe(false);
    expect(inspectorLabel).toBe("Candidate check");
    expect(mocks.canvasProps?.onReviseCandidateGraph).toEqual(expect.any(Function));
    await act(async () => (container.querySelector('[data-testid="revise-parameter"]') as HTMLButtonElement).click());

    expect(onReviseCandidateGraph).toHaveBeenCalledTimes(1);
    const [previous, next] = onReviseCandidateGraph.mock.calls[0];
    expect(previous.nodes.find((node: { id: string }) => node.id === "check")).toMatchObject({ label: "Candidate check" });
    expect(next.nodes.find((node: { id: string }) => node.id === "check")?.step?.args).toEqual({ threshold: 95 });
    expect(mocks.commands.updateNode).not.toHaveBeenCalled();
    expect((mocks.state.committed.graph.nodes as Array<{ id: string; label?: string }>).find((node) => node.id === "check")?.label).toBe("Committed check");
  });

  it("keeps Candidate Inspector and Canvas read-only without an explicit revise callback", async () => {
    mocks.state = workspaceState(candidate());
    const onCommandStateChange = vi.fn();
    let inspectorReadOnly: boolean | undefined;
    await act(async () => root.render(
      <WorkflowEditorShell
        graph={committedGraph}
        actions={[]}
        runState={{}}
        focusNodeId=""
        selectedNodeId="check"
        aiHighlightedNodeIds={[]}
        fullscreen={false}
        onCommandStateChange={onCommandStateChange}
        onSelectNode={() => {}}
        onOpenNodeConfig={() => {}}
        onToggleFullscreen={() => {}}
        renderNodeInspector={({ node, readOnly, onApplyNode }) => {
          inspectorReadOnly = readOnly;
          return <button type="button" data-testid="readonly-apply" onClick={() => onApplyNode({ ...node!, label: "Must not commit" })}>apply</button>;
        }}
      />,
    ));

    expect(inspectorReadOnly).toBe(true);
    expect(mocks.canvasProps?.onReviseCandidateGraph).toBeUndefined();
    await act(async () => (container.querySelector('[data-testid="readonly-apply"]') as HTMLButtonElement).click());
    expect(onCommandStateChange).toHaveBeenCalledWith(expect.objectContaining({ status: "failed", message: expect.stringContaining("只读") }));
    expect(Object.values(mocks.commands).every((command) => command.mock.calls.length === 0)).toBe(true);
  });
});

const committedGraph = {
  workflow: { name: "guard" },
  nodes: [
    { id: "start", type: "start", position: { x: 0, y: 0 } },
    { id: "check", type: "action", label: "Committed check", position: { x: 260, y: 0 }, step: { action: "cmd.run", args: { threshold: 80 } } },
  ],
  edges: [{ id: "edge-start", source: "start", target: "check", kind: "next" }],
};

function workspaceState(candidateFact?: WorkflowCandidateFact) {
  return {
    schemaVersion: "aiops.workflow.workspace.v1",
    workflowKey: { tenantId: "tenant-a", workflowId: "guard" },
    committed: {
      workflowKey: { tenantId: "tenant-a", workflowId: "guard" },
      revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      graph: JSON.parse(JSON.stringify(committedGraph)),
      sequence: 1,
      loadedAt: "2026-07-21T00:00:00Z",
    },
    candidate: candidateFact,
    selection: { nodeIds: [], edgeIds: [] },
    viewport: undefined,
    task: { state: "idle" },
    validationArtifacts: {},
    lifecycleArtifacts: {},
    receipts: {},
    receiptOrder: [],
    events: {},
    eventOrder: [],
    bufferedEvents: {},
    lastSequence: 1,
    connection: { state: "online" },
    runTarget: "draft",
  };
}

function transportState(overrides: Record<string, unknown> = {}) {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "session-a",
    threadId: "workflow:guard",
    status: "idle",
    turns: {},
    turnOrder: [],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: { activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {} },
    hostMissions: {},
    childAgents: {},
    seq: 1,
    updatedAt: "2026-07-21T00:00:00Z",
    ...overrides,
  };
}

function validationBasis(resourceVersion: string) {
  return {
    resourceVersion,
    semanticHash: "semantic-1",
    catalogVersion: "catalog-1",
    actionSchemaDigest: "actions-1",
    referenceSnapshotHash: "refs-1",
    environmentBindingVersion: "env-1",
  };
}

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
      intent: "adjust check",
      base: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      operations: [{ op: "update_node", nodeId: "check", set: { label: "Candidate check" } }],
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
