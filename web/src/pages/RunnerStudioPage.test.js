import { mount, flushPromises } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelRunnerStudioRun,
  createRunnerStudioWorkflowGraph,
  dryRunRunnerStudioWorkflowGraph,
  exportRunnerStudioWorkflowBundle,
  getRunnerStudioRunEventHistory,
  getRunnerStudioActionCatalog,
  getRunnerStudioWorkflowGraph,
  importRunnerStudioWorkflowBundle,
  listRunnerStudioWorkflows,
  listRunnerStudioWorkflowVersions,
  parseRunnerStudioWorkflowYaml,
  rollbackRunnerStudioWorkflowVersion,
  runRunnerStudioWorkflowGraph,
  updateRunnerStudioWorkflowGraph,
  validateRunnerStudioWorkflow,
  validateRunnerStudioWorkflowGraph,
} from "../api/runnerStudioClient";
import RunnerStudioShell from "../components/runner/RunnerStudioShell.vue";
import RunnerStudioPage from "./RunnerStudioPage.vue";

const routerMock = vi.hoisted(() => ({
  route: { name: "runner-ui", params: {} },
  push: vi.fn(() => Promise.resolve()),
}));

vi.mock("vue-router", () => ({
  useRoute: () => routerMock.route,
  useRouter: () => ({ push: routerMock.push }),
}));

vi.mock("../api/runnerStudioClient", () => ({
  listRunnerStudioWorkflows: vi.fn(),
  getRunnerStudioActionCatalog: vi.fn(),
  getRunnerStudioWorkflowGraph: vi.fn(),
  listRunnerStudioWorkflowVersions: vi.fn(),
  exportRunnerStudioWorkflowBundle: vi.fn(),
  importRunnerStudioWorkflowBundle: vi.fn(),
  parseRunnerStudioWorkflowYaml: vi.fn(),
  rollbackRunnerStudioWorkflowVersion: vi.fn(),
  createRunnerStudioWorkflowGraph: vi.fn(),
  updateRunnerStudioWorkflowGraph: vi.fn(),
  validateRunnerStudioWorkflow: vi.fn(),
  validateRunnerStudioWorkflowGraph: vi.fn(),
  dryRunRunnerStudioWorkflowGraph: vi.fn(),
  runRunnerStudioWorkflowGraph: vi.fn(),
  getRunnerStudioRunEventHistory: vi.fn(),
  cancelRunnerStudioRun: vi.fn(),
}));

describe("RunnerStudioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    routerMock.route.name = "runner-ui";
    routerMock.route.params = {};
    routerMock.push.mockResolvedValue(undefined);
    window.localStorage.clear();
    listRunnerStudioWorkflows.mockResolvedValue({
      workflows: [{ name: "pg-restore", title: "PG Restore", status: "draft" }],
    });
    getRunnerStudioActionCatalog.mockResolvedValue({
      items: [{ action: "cmd.run" }, { action: "shell.run" }],
    });
    getRunnerStudioWorkflowGraph.mockResolvedValue({
      version: "v1",
      workflow: { name: "pg-restore" },
      ui: { resource_version: "rv-1" },
      nodes: [],
      edges: [],
    });
    createRunnerStudioWorkflowGraph.mockImplementation((payload) =>
      Promise.resolve({
        name: payload?.graph?.workflow?.name || "runner-blank",
        status: "draft",
        graph: {
          ...(payload?.graph || {}),
          ui: { ...(payload?.graph?.ui || {}), resource_version: "rv-created" },
        },
      }),
    );
    validateRunnerStudioWorkflowGraph.mockResolvedValue({
      valid: true,
      status: "validated",
      validated_graph_hash: "graph-hash-test",
      warnings: ["ok"],
    });
    updateRunnerStudioWorkflowGraph.mockImplementation((name, payload) =>
      Promise.resolve({
        name,
        status: "draft",
        graph: {
          ...(payload?.graph || {}),
          ui: { ...(payload?.graph?.ui || {}), resource_version: "rv-2" },
        },
      }),
    );
    validateRunnerStudioWorkflow.mockResolvedValue({
      name: "pg-restore",
      status: "validated",
      validated_graph_hash: "graph-hash-test",
      validated_at: "2026-05-05T00:00:00Z",
    });
    dryRunRunnerStudioWorkflowGraph.mockResolvedValue({ run_id: "dry-run-test", status: "accepted" });
    runRunnerStudioWorkflowGraph.mockResolvedValue({ run_id: "run-test", status: "running" });
    getRunnerStudioRunEventHistory.mockResolvedValue([]);
    cancelRunnerStudioRun.mockResolvedValue({ run_id: "run-test", status: "canceled" });
    listRunnerStudioWorkflowVersions.mockResolvedValue({
      items: [
        {
          id: "v-1",
          status: "draft",
          reason: "create",
          yaml: "name: pg-restore\nsteps: []\n",
          created_at: "2026-05-05T08:00:00Z",
        },
      ],
    });
    exportRunnerStudioWorkflowBundle.mockResolvedValue({
      bundle_version: "runner.workflow.bundle/v1",
      name: "pg-restore",
      yaml: "name: pg-restore\nsteps: []\n",
      versions: [],
    });
    parseRunnerStudioWorkflowYaml.mockResolvedValue({
      version: "v1",
      workflow: { name: "imported-yaml" },
      nodes: [],
      edges: [],
    });
    importRunnerStudioWorkflowBundle.mockResolvedValue({
      name: "imported-bundle",
      status: "draft",
      yaml: "name: imported-bundle\nsteps: []\n",
    });
    rollbackRunnerStudioWorkflowVersion.mockResolvedValue({
      name: "pg-restore",
      status: "draft",
      yaml: "name: pg-restore\nsteps: []\n",
    });
  });

  it("renders an in-app Runner Studio shell without the legacy external Runner UI entry", async () => {
    const legacyAddress = ["127.0.0.1", "8090"].join(":");
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-page"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="runner-workflow-library"]').text()).toContain("PG Restore");
    expect(wrapper.find('[data-testid="runner-studio-topbar"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-studio-bottom-drawer"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain(legacyAddress);
    expect(wrapper.text()).not.toContain("go run ./server -config");
  });

  it("shows same-page API error state without linking to an external Runner UI", async () => {
    const legacyAddress = ["127.0.0.1", "8090"].join(":");
    listRunnerStudioWorkflows.mockRejectedValue(new Error("catalog unavailable"));

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-error"]').text()).toContain("Runner Studio API 不可用");
    expect(wrapper.get('[data-testid="runner-studio-error"]').text()).toContain("catalog unavailable");
    expect(wrapper.text()).not.toContain(legacyAddress);
    expect(wrapper.find('[data-testid="runner-open-link"]').exists()).toBe(false);
  });

  it("explains a missing same-origin Runner Studio route as a non-blocking local mode notice", async () => {
    listRunnerStudioWorkflows.mockRejectedValue(
      Object.assign(new Error("Request failed with status 404"), {
        status: 404,
        url: "/api/runner-studio/workflows",
        payload: { error: "404 page not found" },
      }),
    );

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.find('[data-testid="runner-studio-error"]').exists()).toBe(false);
    const notice = wrapper.get('[data-testid="runner-studio-api-notice"]').text();
    expect(notice).toContain("本地编排模式");
    expect(notice).toContain("/api/runner-studio/*");
    expect(notice).toContain("重启最新 ai-server");
  });

  it("lets users close the local Runner Studio API notice", async () => {
    listRunnerStudioWorkflows.mockRejectedValue(
      Object.assign(new Error("Request failed with status 503"), {
        status: 503,
        url: "/api/runner-studio/workflows",
        payload: { error: "runner studio upstream is not configured" },
      }),
    );

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-api-notice"]').text()).toContain("本地编排模式");
    await wrapper.get('[data-testid="runner-api-notice-close"]').trigger("click");

    expect(wrapper.find('[data-testid="runner-studio-api-notice"]').exists()).toBe(false);
  });

  it("falls back to a local editable catalog when the Runner Studio API route is missing", async () => {
    const missingRoute = Object.assign(new Error("Request failed with status 404"), {
      status: 404,
      url: "/api/runner-studio/actions",
      payload: { error: "404 page not found" },
    });
    listRunnerStudioWorkflows.mockRejectedValueOnce(missingRoute);
    getRunnerStudioActionCatalog.mockRejectedValueOnce(missingRoute);

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.find('[data-testid="runner-studio-error"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="runner-studio-api-notice"]').text()).toContain("本地编排模式");
    await wrapper.get('[data-testid="runner-open-manager"]').trigger("click");
    await wrapper.get('[data-testid="workflow-create-blank"]').trigger("click");
    await flushPromises();

    await wrapper.get('[data-testid="runner-node-picker-toggle"]').trigger("click");

    const picker = wrapper.get('[data-testid="runner-node-picker"]');
    expect(picker.text()).toContain("Shell Script");
    expect(picker.text()).toContain("条件分支");
    expect(wrapper.get('[data-testid="runner-toolbar-save"]').attributes("disabled")).toBeUndefined();
    expect(wrapper.get('[data-testid="runner-toolbar-save"]').attributes("title")).toContain("保存、校验、运行和发布需要");
    await wrapper.get('[data-testid="runner-toolbar-save"]').trigger("click");
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').text()).toContain("Runner Studio API 不可用");
    expect(wrapper.get('[data-testid="runner-toolbar-save-error"]').text()).toContain("重启最新 ai-server");
    expect(wrapper.get('[data-testid="runner-toolbar-run"]').attributes("disabled")).toBeUndefined();
  });

  it("creates a blank workflow inside the native Studio and records it as recent", async () => {
    listRunnerStudioWorkflows.mockResolvedValueOnce({ workflows: [] });
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    await wrapper.get('[data-testid="runner-open-manager"]').trigger("click");
    await wrapper.get('[data-testid="workflow-create-blank"]').trigger("click");
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("runner-blank");
    expect(wrapper.find('[data-testid="runner-workflow-runner-blank"]').exists()).toBe(false);
    expect(wrapper.findComponent(RunnerStudioShell).props("workflows")[0].graph.nodes).toMatchObject([
      {
        id: "start",
        type: "start",
        label: "Start",
        position: { x: 80, y: 160 },
        ports: [{ id: "next", type: "output", label: "下一步" }],
      },
      {
        id: "end",
        type: "end",
        label: "End",
        position: { x: 720, y: 160 },
        ports: [{ id: "in", type: "input", label: "输入" }],
      },
    ]);
    expect(routerMock.push).toHaveBeenCalledWith({
      name: "runner-workflow",
      params: { workflowName: "runner-blank" },
    });
    expect(JSON.parse(window.localStorage.getItem("runner.studio.workflowManager"))).toMatchObject({
      recent: ["runner-blank"],
    });
  });

  it("opens a workflow editor directly from the /runner/:workflowName route", async () => {
    routerMock.route.name = "runner-workflow";
    routerMock.route.params = { workflowName: "pg-restore" };

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("PG Restore");
    expect(wrapper.find('[data-testid="runner-workflow-library"]').exists()).toBe(false);
    expect(JSON.parse(window.localStorage.getItem("runner.studio.workflowManager"))).toMatchObject({
      recent: ["pg-restore"],
    });
  });

  it("opens a missing route workflow as a saveable local draft instead of disabling the server toolbar", async () => {
    routerMock.route.name = "runner-workflow";
    routerMock.route.params = { workflowName: "runner-blank" };
    listRunnerStudioWorkflows.mockResolvedValueOnce({ workflows: [] });
    getRunnerStudioWorkflowGraph.mockRejectedValueOnce(
      Object.assign(new Error("workflow not found"), {
        status: 404,
        url: "/api/runner-studio/workflows/runner-blank/graph",
        payload: { error: "not found" },
      }),
    );

    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.find('[data-testid="runner-studio-api-notice"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="runner-toolbar-save"]').attributes("disabled")).toBeUndefined();
    expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("本地草稿");
    expect(wrapper.findComponent(RunnerStudioShell).props("workflows")[0].graph.nodes).toMatchObject([
      { id: "start", type: "start", label: "Start", ports: [{ id: "next", type: "output" }] },
      { id: "end", type: "end", label: "End", ports: [{ id: "in", type: "input" }] },
    ]);
  });

  it("pushes the workflow editor route when users select a workflow from the management list", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("PG Restore");
    expect(routerMock.push).toHaveBeenCalledWith({
      name: "runner-workflow",
      params: { workflowName: "pg-restore" },
    });
  });

  it("returns from an opened workflow back to the workflow library", async () => {
    routerMock.route.name = "runner-workflow";
    routerMock.route.params = { workflowName: "runner-blank" };
    listRunnerStudioWorkflows.mockResolvedValueOnce({ workflows: [] });
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("runner-blank");
    await wrapper.get('[data-testid="runner-back-to-library"]').trigger("click");
    await flushPromises();
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-workflow-library"]').text()).toContain("runner-blank");
    expect(wrapper.find('[data-testid="runner-studio-canvas"]').exists()).toBe(false);
    expect(routerMock.push).toHaveBeenCalledWith({ name: "runner-ui" });
  });

  it("persists graph edits in page state and validates or dry-runs the selected graph after flushing draft", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    const graph = {
      version: "v1",
      workflow: { name: "pg-restore" },
      nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
      edges: [],
    };
    const graphWithLoadedVersion = {
      ...graph,
      ui: { resource_version: "rv-1" },
    };
    wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
    await flushPromises();

    await wrapper.get('[data-testid="runner-toolbar-validate"]').trigger("click");
    await flushPromises();

    expect(updateRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore", {
      graph: graphWithLoadedVersion,
      save_note: "autosave before validate",
    });
    expect(validateRunnerStudioWorkflow).toHaveBeenCalledWith("pg-restore");
    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("validated");

    dryRunRunnerStudioWorkflowGraph.mockResolvedValueOnce({
      valid: true,
      status: "dry_run_passed",
      dry_run_graph_hash: "graph-hash-test",
      dry_run_at: "2026-05-05T00:01:00Z",
    });
    await wrapper.get('[data-testid="runner-toolbar-dry-run"]').trigger("click");
    await flushPromises();

    expect(updateRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore", {
      graph: { ...graph, ui: { resource_version: "rv-2" } },
      save_note: "autosave before dry-run",
    });
    expect(dryRunRunnerStudioWorkflowGraph).toHaveBeenCalledWith({
      workflow_name: "pg-restore",
      graph: { ...graph, ui: { resource_version: "rv-2" } },
      vars: {},
      triggered_by: "ui",
    });
    expect(validateRunnerStudioWorkflow).toHaveBeenCalledTimes(2);
    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("dry_run_passed");
  });

  it("shows actionable validation and run failures instead of silent toolbar clicks", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    const graph = {
      version: "v1",
      workflow: { name: "pg-restore" },
      nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
      edges: [],
    };
    wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
    validateRunnerStudioWorkflow.mockRejectedValueOnce(
      Object.assign(new Error("invalid request"), {
        payload: { error: 'action "shell.run" requires args.script' },
      }),
    );

    await wrapper.get('[data-testid="runner-toolbar-validate"]').trigger("click");
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("校验失败");
    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').attributes("title")).toContain("requires args.script");
    expect(wrapper.get('[data-testid="runner-toolbar-save-error"]').text()).toContain("requires args.script");

    runRunnerStudioWorkflowGraph.mockRejectedValueOnce(new Error("runner executor unavailable"));
    await wrapper.get('[data-testid="runner-toolbar-run"]').trigger("click");
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("运行失败");
    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').attributes("title")).toContain("runner executor unavailable");
    expect(wrapper.get('[data-testid="runner-toolbar-save-error"]').text()).toContain("runner executor unavailable");
  });

  it("loads the selected workflow graph and autosaves graph edits with resource_version after debounce", async () => {
    vi.useFakeTimers();
    try {
      const wrapper = mount(RunnerStudioPage);
      await flushPromises();
      await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
      await flushPromises();

      expect(getRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore");

      const graph = {
        version: "v1",
        workflow: { name: "pg-restore" },
        ui: { resource_version: "rv-1" },
        nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
        edges: [],
      };
      wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
      await flushPromises();

      expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("未保存");
      await vi.advanceTimersByTimeAsync(1999);
      expect(updateRunnerStudioWorkflowGraph).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(1);
      await flushPromises();

      expect(updateRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore", {
        graph,
        save_note: "autosave",
      });
      expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("已保存");
    } finally {
      vi.useRealTimers();
    }
  });

  it("flushes unsaved graph edits before returning to the workflow library", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    const graph = {
      version: "v1",
      workflow: { name: "pg-restore" },
      ui: { resource_version: "rv-1" },
      nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
      edges: [],
    };
    wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
    await flushPromises();

    await wrapper.get('[data-testid="runner-back-to-library"]').trigger("click");
    await flushPromises();
    await flushPromises();

    expect(updateRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore", {
      graph,
      save_note: "autosave before switch",
    });
    expect(routerMock.push).toHaveBeenCalledWith({ name: "runner-ui" });
  });

  it("opens a draft conflict dialog and can retry the local graph with the remote resource version", async () => {
    vi.useFakeTimers();
    try {
      getRunnerStudioWorkflowGraph
        .mockResolvedValueOnce({
          version: "v1",
          workflow: { name: "pg-restore" },
          ui: { resource_version: "rv-1" },
          nodes: [],
          edges: [],
        })
        .mockResolvedValueOnce({
          version: "v1",
          workflow: { name: "pg-restore" },
          ui: { resource_version: "rv-remote" },
          nodes: [{ id: "remote", type: "action", step: { name: "remote", action: "cmd.run" } }],
          edges: [],
        });
      updateRunnerStudioWorkflowGraph
        .mockRejectedValueOnce(
          Object.assign(new Error("workflow graph changed since it was loaded"), {
            status: 409,
            payload: { error: "workflow graph changed since it was loaded" },
          }),
        )
        .mockResolvedValueOnce({
          name: "pg-restore",
          status: "draft",
          graph: {
            version: "v1",
            workflow: { name: "pg-restore" },
            ui: { resource_version: "rv-3" },
            nodes: [{ id: "local", type: "action", step: { name: "local", action: "shell.run" } }],
            edges: [],
          },
        });

      const wrapper = mount(RunnerStudioPage);
      await flushPromises();
      await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
      await flushPromises();

      const localGraph = {
        version: "v1",
        workflow: { name: "pg-restore" },
        ui: { resource_version: "rv-1" },
        nodes: [{ id: "local", type: "action", step: { name: "local", action: "shell.run" } }],
        edges: [],
      };
      wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", localGraph);
      await vi.advanceTimersByTimeAsync(2000);
      await flushPromises();

      expect(wrapper.get('[data-testid="runner-draft-conflict-modal"]').text()).toContain("草稿保存冲突");
      expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("保存冲突");

      await wrapper.get('[data-testid="runner-conflict-keep-local"]').trigger("click");
      await flushPromises();

      expect(updateRunnerStudioWorkflowGraph).toHaveBeenLastCalledWith("pg-restore", {
        graph: {
          ...localGraph,
          ui: { resource_version: "rv-remote" },
        },
        save_note: "autosave after conflict",
      });
      expect(wrapper.find('[data-testid="runner-draft-conflict-modal"]').exists()).toBe(false);
      expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("已保存");
    } finally {
      vi.useRealTimers();
    }
  });

  it("flushes draft, submits a run, replays run history, and cancels the active run", async () => {
    getRunnerStudioRunEventHistory.mockResolvedValueOnce([
      { type: "run_start", run_id: "run-test", status: "running" },
      { type: "step_finish", run_id: "run-test", step: "restore", status: "success", output: { stdout: "ok" } },
    ]);
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    const graph = {
      version: "v1",
      workflow: { name: "pg-restore" },
      nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
      edges: [],
    };
    const graphWithLoadedVersion = {
      ...graph,
      ui: { resource_version: "rv-1" },
    };
    const graphWithSavedVersion = {
      ...graph,
      ui: { resource_version: "rv-2" },
    };
    wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
    await flushPromises();

    await wrapper.get('[data-testid="runner-toolbar-run"]').trigger("click");
    await flushPromises();

    expect(updateRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore", {
      graph: graphWithLoadedVersion,
      save_note: "autosave before run",
    });
    expect(runRunnerStudioWorkflowGraph).toHaveBeenCalledWith({
      workflow_name: "pg-restore",
      graph: graphWithSavedVersion,
      vars: {},
      triggered_by: "ui",
      idempotency_key: expect.stringMatching(/^pg-restore-run-/),
      risk_acknowledged: true,
    });
    expect(getRunnerStudioRunEventHistory).toHaveBeenCalledWith("run-test");
    expect(wrapper.findComponent(RunnerStudioShell).props("runEvents").map((event) => event.type)).toContain("step_finish");
    expect(wrapper.get('[data-testid="runner-toolbar-stop-run"]').text()).toContain("停止运行");

    await wrapper.get('[data-testid="runner-toolbar-stop-run"]').trigger("click");
    await flushPromises();

    expect(cancelRunnerStudioRun).toHaveBeenCalledWith("run-test");
    expect(wrapper.findComponent(RunnerStudioShell).props("runEvents").at(-1)).toMatchObject({
      type: "run.cancelled",
      run_id: "run-test",
      status: "canceled",
    });
  });

  it("submits a single-node run with explicit node scope from the node panel", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    const graph = {
      version: "v1",
      workflow: { name: "pg-restore" },
      nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
      edges: [],
    };
    const graphWithSavedVersion = {
      ...graph,
      ui: { resource_version: "rv-2" },
    };
    wrapper.findComponent(RunnerStudioShell).vm.$emit("update-workflow-graph", graph);
    wrapper.findComponent(RunnerStudioShell).vm.$emit("node-action", "run-node", "restore");
    await flushPromises();

    expect(runRunnerStudioWorkflowGraph).toHaveBeenCalledWith({
      workflow_name: "pg-restore",
      graph: graphWithSavedVersion,
      vars: {},
      triggered_by: "ui",
      idempotency_key: expect.stringMatching(/^pg-restore-restore-/),
      risk_acknowledged: true,
      node_id: "restore",
      run_scope: "single_node",
    });
  });

  it("refreshes the selected workflow state after publish succeeds", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();
    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await flushPromises();

    wrapper.findComponent(RunnerStudioShell).vm.$emit("workflow-published", {
      name: "pg-restore",
      status: "published",
      published_graph_hash: "graph-hash-test",
    });
    await flushPromises();

    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("published");
  });

  it("opens version history from the manager, exports bundle text, restores a version, and imports YAML as draft", async () => {
    const wrapper = mount(RunnerStudioPage);
    await flushPromises();

    await wrapper.get('[data-testid="runner-open-manager"]').trigger("click");
    await wrapper.get('[data-testid="workflow-versions-pg-restore"]').trigger("click");
    await flushPromises();

    expect(listRunnerStudioWorkflowVersions).toHaveBeenCalledWith("pg-restore");
    expect(wrapper.get('[data-testid="runner-version-history-panel"]').text()).toContain("v-1");

    await wrapper.get('[data-testid="runner-version-export"]').trigger("click");
    await flushPromises();
    expect(exportRunnerStudioWorkflowBundle).toHaveBeenCalledWith("pg-restore");
    expect(wrapper.get('[data-testid="runner-version-export-text"]').text()).toContain("runner.workflow.bundle/v1");

    await wrapper.get('[data-testid="runner-version-rollback-v-1"]').trigger("click");
    await flushPromises();
    expect(rollbackRunnerStudioWorkflowVersion).toHaveBeenCalledWith("pg-restore", "v-1", {
      save_note: "rollback from Runner Studio",
    });
    expect(getRunnerStudioWorkflowGraph).toHaveBeenCalledWith("pg-restore");

    await wrapper.get('[data-testid="runner-version-import-mode"]').setValue("yaml");
    await wrapper.get('[data-testid="runner-version-import-text"]').setValue("name: imported-yaml\nsteps: []\n");
    await wrapper.get('[data-testid="runner-version-import-submit"]').trigger("click");
    await flushPromises();

    expect(parseRunnerStudioWorkflowYaml).toHaveBeenCalledWith({ yaml: "name: imported-yaml\nsteps: []" });
    expect(createRunnerStudioWorkflowGraph).toHaveBeenCalledWith({
      graph: { version: "v1", workflow: { name: "imported-yaml" }, nodes: [], edges: [] },
      save_note: "imported yaml from Runner Studio",
    });
    expect(wrapper.get('[data-testid="runner-studio-topbar"]').text()).toContain("imported-yaml");
  });
});
