import { afterEach, describe, expect, it, vi } from "vitest";
import type { ActionSpec, WorkflowBundle, WorkflowGraph } from "../types/workflow";
import { __resetGraphStoreForTests, useGraphStore } from "../stores/graphStore";

const graph: WorkflowGraph = {
  version: "v1",
  workflow: { version: "v0.1", name: "store-test" },
  nodes: [
    { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
    { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
  ],
  edges: [],
};

const actions: ActionSpec[] = [
  {
    action: "script.shell",
    title: "Shell Script",
    category: "script",
    node_type: "action",
    defaults: { script: "echo ok" },
  },
];

describe("graph store editing actions", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    __resetGraphStoreForTests();
  });

  it("adds catalog and control nodes through one graph state", () => {
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    store.addActionNodeFromCatalog("script.shell", { x: 240, y: 160 });
    store.addControlNode("join", { x: 400, y: 160 });
    store.addControlNode("handler", { x: 400, y: 280 });

    expect(store.state.graph?.nodes.map((node) => node.id)).toEqual(["start", "end", "script-shell", "join", "handler"]);
    expect(store.state.selectedNodeId).toBe("handler");
    expect(store.state.dirty).toBe(true);
    expect(store.state.graph?.nodes.find((node) => node.id === "script-shell")?.step?.args).toEqual({ script: "echo ok" });
    expect(store.state.graph?.nodes.find((node) => node.id === "handler")?.handler).toMatchObject({ action: "script.shell" });
  });

  it("loads workflow summaries for subflow selection with the graph and action catalog", async () => {
    const loadedGraph = cloneGraph(graph);
    const fetchMock = vi.fn(async (url: string | URL | Request, _init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/graph")) {
        return new Response(JSON.stringify(loadedGraph), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/actions/catalog")) {
        return new Response(JSON.stringify({ items: actions }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/workflows?limit=200")) {
        return new Response(
          JSON.stringify({
            items: [
              { name: "restore-verify", version: "v3", description: "restore verification" },
              { name: "store-test", version: "v1" },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests();
    const store = useGraphStore();

    await store.load("store-test");

    expect(store.state.workflowOptions.map((item) => item.name)).toEqual(["restore-verify", "store-test"]);
    expect(store.state.graph?.workflow.name).toBe("store-test");
    expect(store.state.baselineGraph?.workflow.name).toBe("store-test");
    expect(store.state.actions).toHaveLength(1);
    expect(store.state.offline).toBe(false);
  });

  it("creates workflow resources through the graph-native create API", async () => {
    const createdGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "created-graph", description: "created from UI" },
      ui: { resource_version: "sha256:new" },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell", args: { script: "echo ok" } } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    let createBody = "";
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/graph")) {
        createBody = String(init?.body || "");
        return new Response(
          JSON.stringify({
            name: "created-graph",
            status: "draft",
            workflow: createdGraph.workflow,
            graph: createdGraph,
            yaml: "version: v0.1\nname: created-graph\n",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
      }
      if (target.endsWith("/workflows?limit=200")) {
        return new Response(JSON.stringify({ items: [{ name: "created-graph", status: "draft" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({
      graph: cloneGraph(graph),
      baselineGraph: cloneGraph(graph),
      actions,
      dirty: true,
      validation: { valid: false, errors: [{ type: "validation", message: "old" }], warnings: [] },
      dryRun: { valid: false, errors: [], warnings: [], steps_count: 0, target_hosts: [], actions_used: [], agents_status: {}, summary: "old" },
      yamlPreview: "old yaml",
      offline: false,
    });
    const store = useGraphStore();

    await store.createWorkflowFromGraph(createdGraph, {
      labels: { source: "visual-ui" },
      saveNote: "initial visual workflow draft",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph",
      expect.objectContaining({
        method: "POST",
      }),
    );
    expect(createBody).toContain('"labels":{"source":"visual-ui"}');
    expect(createBody).toContain('"save_note":"initial visual workflow draft"');
    expect(store.state.graph?.workflow.name).toBe("created-graph");
    expect(store.state.baselineGraph?.workflow.name).toBe("created-graph");
    expect(store.state.selectedNodeId).toBe("run");
    expect(store.state.workflowStatus).toBe("draft");
    expect(store.state.workflowOptions.map((item) => item.name)).toEqual(["created-graph"]);
    expect(store.state.yamlPreview).toBe("version: v0.1\nname: created-graph\n");
    expect(store.state.dirty).toBe(false);
    expect(store.state.validation).toBeNull();
    expect(store.state.dryRun).toBeNull();
  });

  it("keeps the current graph when graph-native create fails", async () => {
    const fetchMock = vi.fn(async () => new Response("already exists", { status: 409 }));
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({
      graph: cloneGraph(graph),
      actions,
      offline: false,
      workflowOptions: [{ name: "store-test", version: "v0.1", status: "published" }],
    });
    const store = useGraphStore();
    const attempted = { ...cloneGraph(graph), workflow: { version: "v0.1", name: "duplicate" } };

    await store.createWorkflowFromGraph(attempted);

    expect(store.state.graph?.workflow.name).toBe("store-test");
    expect(store.state.error).toContain("already exists");
    expect(store.state.dirty).toBe(false);
  });

  it("requires confirmation before switching away from a dirty workflow", async () => {
    const nextGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "next-workflow" },
    };
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/next-workflow/graph")) {
        return new Response(JSON.stringify(nextGraph), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/actions/catalog")) {
        return new Response(JSON.stringify({ items: actions }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/workflows?limit=200")) {
        return new Response(JSON.stringify({ items: [{ name: "store-test" }, { name: "next-workflow" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false, dirty: true });
    const store = useGraphStore();

    await store.switchWorkflow("next-workflow");

    expect(store.state.graph?.workflow.name).toBe("store-test");
    expect(store.state.error).toContain("Save or discard changes");
    expect(fetchMock).not.toHaveBeenCalled();

    await store.switchWorkflow("next-workflow", { force: true });

    expect(store.state.graph?.workflow.name).toBe("next-workflow");
    expect(store.state.dirty).toBe(false);
    expect(store.state.workflowOptions.map((item) => item.name)).toEqual(["store-test", "next-workflow"]);
  });

  it("connects, lays out, and deletes selected nodes", () => {
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    store.addActionNodeFromCatalog("script.shell", { x: 240, y: 160 });
    store.connectNodes("start", "script-shell");
    store.connectNodes("script-shell", "end");
    expect(store.state.graph?.edges).toHaveLength(2);

    store.autoLayout();
    const startX = store.state.graph?.nodes.find((node) => node.id === "start")?.position.x || 0;
    const actionX = store.state.graph?.nodes.find((node) => node.id === "script-shell")?.position.x || 0;
    expect(startX).toBeLessThan(actionX);

    store.selectNode("script-shell");
    store.deleteSelectedNode();
    expect(store.state.graph?.nodes.map((node) => node.id)).toEqual(["start", "end"]);
    expect(store.state.graph?.edges).toEqual([]);
  });

  it("builds a non-trivial DAG and validates it through the server API", async () => {
    const validated = { graph: null as WorkflowGraph | null };
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/graph/validate")) {
        validated.graph = JSON.parse(String(init?.body || "{}")).graph as WorkflowGraph;
        return new Response(JSON.stringify({ valid: true, errors: [], warnings: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows/graph/compile")) {
        return new Response(JSON.stringify({ yaml: "name: dag-preview\n" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false });
    const store = useGraphStore();

    store.addControlNode("parallel", { x: 220, y: 120 });
    store.addActionNodeFromCatalog("script.shell", { x: 420, y: 60 });
    store.addActionNodeFromCatalog("script.shell", { x: 420, y: 180 });
    store.addControlNode("join", { x: 640, y: 120 });
    store.connectNodes("start", "parallel");
    store.connectNodes("parallel", "script-shell");
    store.connectNodes("parallel", "script-shell-2");
    store.connectNodes("script-shell", "join");
    store.connectNodes("script-shell-2", "join");
    store.connectNodes("join", "end");

    await store.validateGraph();

    if (!validated.graph) throw new Error("server validation request was not captured");
    expect(validated.graph.nodes.map((node) => node.type)).toEqual(["start", "end", "parallel", "action", "action", "join"]);
    expect(validated.graph.edges.filter((edge) => edge.target === "join")).toHaveLength(2);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph/validate",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"type":"parallel"'),
      }),
    );
    expect(store.state.validation?.valid).toBe(true);
  });

  it("copies, pastes, undoes, and redoes graph edits through one history stack", () => {
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    store.addActionNodeFromCatalog("script.shell", { x: 240, y: 160 });
    store.copySelectedNode();
    expect(store.state.clipboardNode?.id).toBe("script-shell");

    store.pasteNode();
    const pasted = store.state.graph?.nodes.find((node) => node.id === "script-shell-copy");
    expect(pasted?.step?.name).toBe("script-shell-copy");
    expect(pasted?.position).toEqual({ x: 276, y: 196 });

    store.undo();
    expect(store.state.graph?.nodes.some((node) => node.id === "script-shell-copy")).toBe(false);
    expect(store.state.historyFuture).toHaveLength(1);

    store.redo();
    expect(store.state.graph?.nodes.some((node) => node.id === "script-shell-copy")).toBe(true);
  });

  it("updates workflow vars and inventory through the same graph history", () => {
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    store.updateWorkflow({
      vars: { service: "billing-api" },
      inventory: {
        hosts: {
          "app-01": { address: "agent://app-01" },
        },
      },
    });

    expect(store.state.graph?.workflow.vars).toEqual({ service: "billing-api" });
    expect(store.state.graph?.workflow.inventory).toEqual({ hosts: { "app-01": { address: "agent://app-01" } } });
    expect(store.state.dirty).toBe(true);
    expect(store.state.historyPast).toHaveLength(1);

    store.undo();
    expect(store.state.graph?.workflow.vars).toBeUndefined();
    expect(store.state.graph?.workflow.inventory).toBeUndefined();
  });

  it("does not push history or dirty the graph for no-op edits", () => {
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, dirty: false });
    const store = useGraphStore();

    store.updateNode("start", { label: "Start" });
    store.updateWorkflow({ name: "store-test" });
    store.replaceGraph(cloneGraph(graph));

    expect(store.state.historyPast).toHaveLength(0);
    expect(store.state.historyFuture).toHaveLength(0);
    expect(store.state.dirty).toBe(false);
  });

  it("sanitizes runtime state from undo and redo snapshots", () => {
    __resetGraphStoreForTests({
      graph: {
        ...cloneGraph(graph),
        nodes: [
          { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 }, state: { status: "running" } },
          { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
        ],
        edges: [{ id: "start-end", source: "start", target: "end", kind: "next", state: { status: "selected" } }],
      },
      actions,
    });
    const store = useGraphStore();

    store.updateNode("start", { label: "Begin" });
    store.undo();

    expect(store.state.graph?.nodes.find((node) => node.id === "start")?.state).toBeUndefined();
    expect(store.state.graph?.edges[0]?.state).toBeUndefined();
    expect(store.state.validation).toBeNull();
    expect(store.state.dryRun).toBeNull();
  });

  it("debugs the selected action node without changing run state", async () => {
    const actionGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { ...graph.workflow, vars: { service: "api" } },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "dns", type: "action", position: { x: 280, y: 120 }, step: { name: "dns", action: "builtin.dns_resolve", args: { name: "localhost" } } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    let debugBody = "";
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/graph/nodes/dns/debug")) {
        debugBody = String(init?.body || "");
        return new Response(
          JSON.stringify({ node_id: "dns", action: "builtin.dns_resolve", status: "success", output: { ok: true, records: ["127.0.0.1"] } }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: actionGraph, actions, selectedNodeId: "dns", offline: false });
    const store = useGraphStore();

    await store.debugSelectedNode();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph/nodes/dns/debug",
      expect.objectContaining({ method: "POST" }),
    );
    expect(debugBody).toContain('"mode":"dry_run"');
    expect(debugBody).toContain('"vars":{"service":"api"}');
    expect(store.state.nodeDebugResult?.status).toBe("success");
    expect(store.state.run.runId).toBeUndefined();
  });

  it("updates graph immediately and refreshes compiled YAML preview after form edits", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/graph/compile")) {
        return new Response(JSON.stringify({ yaml: "name: compiled-preview\n" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false });
    const store = useGraphStore();

    store.updateNode("start", { label: "Begin" });
    expect(store.state.graph?.nodes.find((node) => node.id === "start")?.label).toBe("Begin");

    await vi.advanceTimersByTimeAsync(350);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph/compile",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"label":"Begin"'),
      }),
    );
    expect(store.state.yamlPreview).toBe("name: compiled-preview\n");
  });

  it("saves draft with an operator save note through the graph update API", async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/graph")) {
        return new Response(JSON.stringify({ workflow: graph.workflow, yaml: "name: store-test\n" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false });
    const store = useGraphStore();

    store.state.saveNote = "reviewed dry run output before save";
    await store.saveDraft();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/store-test/graph",
      expect.objectContaining({
        method: "PUT",
        body: expect.stringContaining('"save_note":"reviewed dry run output before save"'),
      }),
    );
    expect(store.state.yamlPreview).toBe("name: store-test\n");
    expect(store.state.saveNote).toBe("");
    expect(store.state.dirty).toBe(false);
    expect(store.state.workflowOptions.find((item) => item.name === "store-test")).toMatchObject({
      status: "draft",
      save_note: "reviewed dry run output before save",
    });
  });

  it("requires explicit review before saving execution semantic changes", async () => {
    const semanticGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell", args: { script: "echo ok" } } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
      edges: [
        { id: "start-run", source: "start", target: "run", kind: "next" },
        { id: "run-end", source: "run", target: "end", kind: "next" },
      ],
    };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ workflow: semanticGraph.workflow, yaml: "name: store-test\n" }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(semanticGraph), baselineGraph: cloneGraph(semanticGraph), actions, offline: false });
    const store = useGraphStore();

    store.updateNode("run", { step: { name: "run", action: "script.shell", args: { script: "hostname" } } });
    await store.saveDraft();

    expect(store.executionSemanticsChanged.value).toBe(true);
    expect(store.state.error).toContain("execution semantic changes");
    expect(fetchMock).not.toHaveBeenCalled();

    store.state.semanticChangeAcknowledged = true;
    await store.saveDraft();

    expect(fetchMock).toHaveBeenCalledWith("/api/v1/workflows/store-test/graph", expect.objectContaining({ method: "PUT" }));
    expect(store.state.dirty).toBe(false);
    expect(store.state.semanticChangeAcknowledged).toBe(false);
  });

  it("submits graph runs with explicit risk acknowledgement", async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/graph/runs")) {
        return new Response(JSON.stringify({ run_id: "run-risk", status: "queued", workflow_name: "store-test", created_at: "2026-05-03T00:00:00Z" }), {
          status: 202,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false });
    const store = useGraphStore();

    store.state.riskAcknowledged = true;
    await store.submitRun();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph/runs",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"risk_acknowledged":true'),
      }),
    );
    expect(store.state.run.runId).toBe("run-risk");
  });

  it("resolves waiting approval nodes through the run node API", async () => {
    const approvalGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        {
          id: "approve",
          type: "manual_approval",
          label: "Approve rollout",
          position: { x: 280, y: 120 },
          approval: { subjects: ["sre"], timeout: "30m", on_timeout: "reject" },
        },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    let approveBody = "";
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/runs/run-approval/nodes/approve/approve")) {
        approveBody = String(init?.body || "");
        return new Response(JSON.stringify({ run_id: "run-approval", node_id: "approve", status: "approved" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/runs/run-approval/graph")) {
        return new Response(
          JSON.stringify({
            ...approvalGraph,
            nodes: approvalGraph.nodes.map((node) => (node.id === "approve" ? { ...node, state: { status: "success" } } : node)),
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: approvalGraph, actions, offline: false });
    const store = useGraphStore();
    store.pushRunEvent({ type: "run_start", run_id: "run-approval", status: "running" });
    store.pushRunEvent({ type: "approval_waiting", run_id: "run-approval", status: "waiting", output: { node_id: "approve" } });

    expect(store.waitingApprovalNodes.value.map((node) => node.id)).toEqual(["approve"]);
    await store.approveNode("approve", "ship");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/runs/run-approval/nodes/approve/approve",
      expect.objectContaining({ method: "POST" }),
    );
    expect(approveBody).toContain('"actor":"ui"');
    expect(approveBody).toContain('"comment":"ship"');
    expect(store.state.run.nodeStatus.approve).toBe("success");
    expect(store.waitingApprovalNodes.value).toEqual([]);
  });

  it("publishes a saved draft workflow and updates workflow status", async () => {
    let publishBody = "";
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/publish")) {
        publishBody = String(init?.body || "");
        return new Response(JSON.stringify({ name: "store-test", status: "published", save_note: "ship it", published_at: "2026-05-03T00:00:00Z" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({
      graph: cloneGraph(graph),
      actions,
      offline: false,
      dirty: false,
      workflowOptions: [{ name: "store-test", version: "v0.1", status: "draft" }],
    });
    const store = useGraphStore();

    store.state.saveNote = "ship it";
    store.state.riskAcknowledged = true;
    store.state.warningAcknowledged = true;
    await store.publishWorkflow();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/store-test/publish",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"save_note":"ship it"'),
      }),
    );
    expect(publishBody).toContain('"warning_acknowledged":true');
    expect(store.state.workflowStatus).toBe("published");
    expect(store.state.saveNote).toBe("");
    expect(store.state.workflowOptions.find((item) => item.name === "store-test")).toMatchObject({
      status: "published",
      published_at: "2026-05-03T00:00:00Z",
    });
  });

  it("loads workflow history and rolls back through the workflow versions API", async () => {
    const rolledBackGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "store-test", description: "rolled back" },
    };
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/versions")) {
        return new Response(JSON.stringify({ items: [{ id: "v1", name: "store-test", yaml: "name: store-test\n" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows/store-test/versions/v1/rollback")) {
        return new Response(JSON.stringify({ name: "store-test", status: "draft", yaml: "name: store-test\n" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows/store-test/graph")) {
        return new Response(JSON.stringify(rolledBackGraph), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false, dirty: false, saveNote: "rollback note" });
    const store = useGraphStore();

    await store.loadWorkflowVersions();
    await store.rollbackWorkflowVersion("v1");

    expect(store.state.workflowVersions.map((item) => item.id)).toEqual(["v1"]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/store-test/versions/v1/rollback",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ save_note: "rollback note" }),
      }),
    );
    expect(store.state.graph?.workflow.description).toBe("rolled back");
    expect(store.state.workflowStatus).toBe("draft");
    expect(store.state.dirty).toBe(false);
    expect(store.state.historyPast).toHaveLength(0);
    expect(store.state.historyFuture).toHaveLength(0);
  });

  it("exports and imports workflow bundles through the workflow bundle API", async () => {
    const bundle: WorkflowBundle = {
      bundle_version: "runner.workflow.bundle/v1",
      name: "store-test",
      yaml: "version: v0.1\nname: store-test\n",
      versions: [{ id: "v1", name: "store-test", yaml: "version: v0.1\nname: store-test\n" }],
    };
    const importedBundle: WorkflowBundle = {
      bundle_version: "runner.workflow.bundle/v1",
      name: "imported-bundle",
      yaml: "version: v0.1\nname: imported-bundle\n",
      versions: [{ id: "v1", name: "imported-bundle", yaml: "version: v0.1\nname: imported-bundle\n" }],
    };
    const importedGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "imported-bundle" },
    };
    let importBody = "";
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/bundle")) {
        return new Response(JSON.stringify(bundle), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows/bundles/import")) {
        importBody = String(init?.body || "");
        return new Response(JSON.stringify({ name: "imported-bundle", status: "draft", yaml: "version: v0.1\nname: imported-bundle\n" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows/imported-bundle/graph")) {
        return new Response(JSON.stringify(importedGraph), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (target.endsWith("/workflows?limit=200")) {
        return new Response(JSON.stringify({ items: [{ name: "imported-bundle", status: "draft" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions, offline: false, dirty: false, saveNote: "bundle import" });
    const store = useGraphStore();

    const exported = await store.exportWorkflowBundle();
    await store.importWorkflowBundle(JSON.stringify(importedBundle));

    expect(exported?.name).toBe("store-test");
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/workflows/store-test/bundle", expect.anything());
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/bundles/import",
      expect.objectContaining({
        method: "POST",
      }),
    );
    expect(importBody).toContain('"save_note":"bundle import"');
    expect(store.state.graph?.workflow.name).toBe("imported-bundle");
    expect(store.state.workflowStatus).toBe("draft");
    expect(store.state.dirty).toBe(false);
    expect(store.state.saveNote).toBe("");
  });

  it("imports edited YAML through the server parser", async () => {
    const parsedGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "parsed" },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell" } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(parsedGraph), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    await store.importGraphYAML("version: v0.1\nname: parsed\n");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/workflows/graph/parse",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ yaml: "version: v0.1\nname: parsed\n" }) }),
    );
    expect(store.state.graph?.workflow.name).toBe("parsed");
    expect(store.state.selectedNodeId).toBe("run");
    expect(store.state.yamlPreview).toBe("version: v0.1\nname: parsed\n");
    expect(store.state.dirty).toBe(true);
  });

  it("replays run history through the shared run reducer and refreshes graph overlay", async () => {
    const runGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      workflow: { version: "v0.1", name: "run-history" },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell" } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    const history = [
      { type: "run_queued", run_id: "run-1", status: "queued", timestamp: "2026-05-03T00:00:00Z" },
      { type: "run_start", run_id: "run-1", status: "running", timestamp: "2026-05-03T00:00:01Z" },
      { type: "node_started", run_id: "run-1", status: "running", output: { node_id: "run" }, timestamp: "2026-05-03T00:00:02Z" },
      { type: "node_finished", run_id: "run-1", status: "success", output: { node_id: "run" }, timestamp: "2026-05-03T00:00:03Z" },
      { type: "run_finish", run_id: "run-1", status: "success", timestamp: "2026-05-03T00:00:04Z" },
    ];
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/events/history")) {
        return new Response(JSON.stringify(history), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/graph")) {
        return new Response(JSON.stringify(runGraph), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests({ graph: cloneGraph(graph), actions });
    const store = useGraphStore();

    await store.replayRunHistory("run-1");

    expect(fetchMock).toHaveBeenCalledWith("/api/v1/runs/run-1/events/history", expect.anything());
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/runs/run-1/graph", expect.anything());
    expect(store.state.run.status).toBe("success");
    expect(store.state.run.timeline).toHaveLength(5);
    expect(store.state.run.nodeStatus.run).toBe("success");
    expect(store.state.graph?.workflow.name).toBe("run-history");
    expect(store.state.eventConnected).toBe(false);
  });

  it("restores a long-running graph run after page refresh and reconnects SSE", async () => {
    const storage = new Map([["runner.visual.lastRunId", "run-long"]]);
    const eventSources: Array<{ url: string; close: ReturnType<typeof vi.fn> }> = [];
    vi.stubGlobal("localStorage", {
      getItem: vi.fn((key: string) => storage.get(key) || null),
      setItem: vi.fn((key: string, value: string) => storage.set(key, value)),
    });
    vi.stubGlobal(
      "EventSource",
      class {
        url: string;
        close = vi.fn();
        onmessage: ((message: MessageEvent) => void) | null = null;
        onerror: ((event: Event) => void) | null = null;
        constructor(url: string) {
          this.url = url;
          eventSources.push(this);
        }
      },
    );
    const runGraph: WorkflowGraph = {
      ...cloneGraph(graph),
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
        { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell" }, state: { status: "running" } },
        { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
      ],
    };
    const history = [
      { type: "run_queued", run_id: "run-long", status: "queued", timestamp: "2026-05-03T00:00:00Z" },
      { type: "run_start", run_id: "run-long", status: "running", timestamp: "2026-05-03T00:00:01Z" },
      { type: "node_started", run_id: "run-long", status: "running", output: { node_id: "run" }, timestamp: "2026-05-03T00:00:02Z" },
    ];
    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      const target = String(url);
      if (target.endsWith("/workflows/store-test/graph")) {
        return new Response(JSON.stringify(cloneGraph(graph)), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/actions/catalog")) {
        return new Response(JSON.stringify({ items: actions }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/workflows?limit=200")) {
        return new Response(JSON.stringify({ items: [{ name: "store-test", status: "draft" }] }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/runs/run-long/events/history")) {
        return new Response(JSON.stringify(history), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (target.endsWith("/runs/run-long/graph")) {
        return new Response(JSON.stringify(runGraph), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    __resetGraphStoreForTests();
    const store = useGraphStore();

    await store.load("store-test");

    expect(fetchMock).toHaveBeenCalledWith("/api/v1/runs/run-long/events/history", expect.anything());
    expect(store.state.run.runId).toBe("run-long");
    expect(store.state.run.status).toBe("running");
    expect(store.state.run.nodeStatus.run).toBe("running");
    expect(store.state.eventConnected).toBe(true);
    expect(eventSources[0]?.url).toBe("/api/v1/runs/run-long/events");
  });

  it("focuses the failed graph node from runtime events", () => {
    __resetGraphStoreForTests({
      graph: {
        ...cloneGraph(graph),
        nodes: [
          { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
          { id: "run", type: "action", position: { x: 280, y: 120 }, step: { name: "run", action: "script.shell" } },
          { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
        ],
      },
      actions,
    });
    const store = useGraphStore();

    store.pushRunEvent({ type: "node_finished", run_id: "run-2", status: "failed", output: { node_id: "run" } });

    expect(store.state.selectedNodeId).toBe("run");
    expect(store.state.run.status).toBe("failed");
  });
});

function cloneGraph(input: WorkflowGraph): WorkflowGraph {
  return JSON.parse(JSON.stringify(input)) as WorkflowGraph;
}
