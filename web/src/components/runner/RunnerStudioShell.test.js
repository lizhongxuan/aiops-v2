import { readFileSync } from "node:fs";
import { join } from "node:path";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerStudioShell from "./RunnerStudioShell.vue";

const workflows = [
  { name: "pg-restore", title: "PG Restore", status: "validated" },
  { name: "cache-warmup", title: "Cache Warmup", status: "draft" },
];

const actions = [{ action: "cmd.run" }, { action: "shell.run" }, { action: "script.shell" }];

const workflowWithGraph = {
  name: "pg-restore",
  title: "PG Restore",
  status: "validated",
  validated_graph_hash: "graph-hash-1",
  graph: {
    version: "v1",
    workflow: { name: "pg-restore" },
    nodes: [
      {
        id: "restore",
        type: "action",
        position: { x: 80, y: 90 },
        step: { name: "restore", action: "shell.run" },
      },
    ],
    edges: [],
  },
};

describe("RunnerStudioShell", () => {
  it("renders the compact topbar with workflow identity and the allowed actions only", () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
      },
    });

    const toolbar = wrapper.get('[data-testid="runner-studio-topbar"]');
    expect(toolbar.text()).toContain("PG Restore");
    expect(toolbar.text()).toContain("validated");
    expect(toolbar.text()).toContain("保存");
    expect(toolbar.text()).toContain("校验");
    expect(toolbar.text()).toContain("Dry Run");
    expect(toolbar.text()).toContain("运行");
    expect(toolbar.text()).toContain("发布");
    expect(toolbar.text()).toContain("AI 生成");
    expect(toolbar.text()).not.toContain("RUNNER STUDIO");
    expect(toolbar.text()).not.toContain("节点配置");
  });

  it("shows visible save feedback next to the toolbar when a save is pending or complete", () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
        saveState: { status: "saved", message: "", lastSavedAt: "01:50:00", error: "" },
      },
    });

    expect(wrapper.get('[data-testid="runner-save-state"]').text()).toContain("已保存 01:50:00");
    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').text()).toContain("已保存 01:50:00");
    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').classes()).toContain("status-saved");
  });

  it("shows actionable toolbar errors and still emits server-bound actions in local mode", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
        serverActionsDisabled: true,
        serverActionsDisabledReason: "Runner API upstream 尚未配置，请设置 AIOPS_RUNNER_STUDIO_UPSTREAM_URL 后重启。",
        saveState: {
          status: "error",
          message: "保存失败",
          lastSavedAt: "",
          error: "HTTP 503: runner studio upstream is not configured",
        },
      },
    });

    const saveButton = wrapper.get('[data-testid="runner-toolbar-save"]');
    expect(saveButton.attributes("disabled")).toBeUndefined();
    expect(wrapper.get('[data-testid="runner-toolbar-save-feedback"]').text()).toContain("保存失败");
    expect(wrapper.get('[data-testid="runner-toolbar-save-error"]').text()).toContain("runner studio upstream");

    await saveButton.trigger("click");

    expect(wrapper.emitted("toolbar-action")?.at(-1)).toEqual(["save"]);
  });

  it("mounts the editor canvas with run details in a side drawer instead of a bottom drawer", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
      },
    });

    expect(wrapper.find('[data-testid="runner-studio-sidebar"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="runner-back-to-library"]').text()).toContain("工作流");
    expect(wrapper.find(".runner-studio-canvas-head").exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-studio-bottom-drawer"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-run-drawer"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="节点配置"]').exists()).toBe(false);

    await wrapper.get('[data-testid="runner-toolbar-run-details"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-run-drawer"]').text()).toContain("运行详情");
    expect(wrapper.get('[data-testid="runner-run-drawer"]').text()).toContain("stdout");
    await wrapper.get('[data-testid="runner-run-drawer-close"]').trigger("click");
    expect(wrapper.find('[data-testid="runner-run-drawer"]').exists()).toBe(false);

    await wrapper.get('[data-testid="runner-back-to-library"]').trigger("click");
    expect(wrapper.emitted("update:selectedWorkflowName")?.[0]).toEqual([""]);
  });

  it("shows only the workflow library before a workflow is selected", () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
      },
    });

    expect(wrapper.get('[data-testid="runner-workflow-library"]').text()).toContain("工作流");
    expect(wrapper.get('[data-testid="runner-workflow-library"]').text()).toContain("Cache Warmup");
    expect(wrapper.find('[data-testid="runner-studio-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-studio-bottom-drawer"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-studio-topbar"]').exists()).toBe(false);
  });

  it("hides the workflow list in editor mode and exposes a back button", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows,
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["cache-warmup"], favorites: ["pg-restore"] },
      },
    });

    expect(wrapper.find('[data-testid="runner-studio-sidebar"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="runner-back-to-library"]').text()).toContain("工作流");

    await wrapper.get('[data-testid="runner-back-to-library"]').trigger("click");

    expect(wrapper.emitted("update:selectedWorkflowName")?.[0]).toEqual([""]);
  });

  it("keeps desktop and tablet responsive rules in the shared shell stylesheet", () => {
    const css = readFileSync(join(process.cwd(), "src/components/runner/runnerStudio.css"), "utf8");

    expect(css).toContain(".runner-studio-shell");
    expect(css).toContain(".runner-studio-page {\n  display: flex;");
    expect(css).toContain("flex-direction: column");
    expect(css).toContain("flex: 1 1 auto");
    expect(css).toContain("grid-template-columns");
    expect(css).toContain("@media (max-width: 1180px)");
    expect(css).toContain("@media (max-width: 860px)");
    expect(css).toContain(".runner-studio-shell.fullscreen");
    expect(css).toContain(".runner-studio-run-drawer-backdrop");
    expect(css).not.toContain("height: clamp(132px, 20vh, 220px)");
    expect(css).not.toContain("grid-template-columns: 220px minmax(0, 1fr)");
  });

  it("shows the recent run summary in the run details drawer after a node is selected", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
        runEvents: [
          {
            type: "node.completed",
            node_id: "restore",
            status: "success",
            duration_ms: 42000,
            result: { exit_code: 0 },
          },
        ],
      },
    });

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("click");
    await wrapper.get('[data-testid="runner-toolbar-run-details"]').trigger("click");

    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("restore");
    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("success");
    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("42s");
  });

  it("opens run details from the selected node panel status", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
        runEvents: [
          {
            type: "node.completed",
            node_id: "restore",
            status: "success",
            duration_ms: 42000,
            result: { exit_code: 0 },
          },
        ],
      },
    });

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("click");
    await wrapper.get('[data-testid="runner-node-panel-open-run"]').trigger("click");

    expect(wrapper.get('[data-testid="runner-run-drawer"]').text()).toContain("运行详情");
    expect(wrapper.get('[data-testid="runner-run-panel"]').text()).toContain("restore");
    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("42s");
  });

  it("toggles fullscreen mode from the canvas toolbar", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="runner-canvas-fullscreen-toggle"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-studio-shell"]').classes()).toContain("fullscreen");

    await wrapper.get('[data-testid="runner-canvas-fullscreen-toggle"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-studio-shell"]').classes()).not.toContain("fullscreen");
  });

  it("opens a docked node panel on node click and updates it when selection changes", async () => {
    const workflow = {
      ...workflowWithGraph,
      graph: {
        ...workflowWithGraph.graph,
        nodes: [
          ...workflowWithGraph.graph.nodes,
          {
            id: "verify",
            type: "action",
            position: { x: 360, y: 120 },
            step: { name: "verify", action: "cmd.run" },
          },
        ],
      },
    };
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflow],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    expect(wrapper.find('[data-testid="node-config-modal"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="runner-node-panel"]').exists()).toBe(false);

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-node-panel-title"]').text()).toContain("restore");
    expect(wrapper.get('[data-testid="runner-node-panel-tabs"]').text()).toContain("设置");

    await wrapper.get('[data-testid="canvas-node-verify"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-node-panel-title"]').text()).toContain("verify");

    await wrapper.get('[data-testid="runner-node-panel-close"]').trigger("click");
    expect(wrapper.find('[data-testid="runner-node-panel"]').exists()).toBe(false);
  });

  it("applies node panel edits into the workflow graph without opening a modal", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("click");
    await wrapper.get('[data-testid="basic-name"]').setValue("restore-primary");
    await wrapper.get('[data-testid="runner-node-panel-apply"]').trigger("click");

    const emittedGraph = wrapper.emitted("update-workflow-graph")?.[0]?.[0];
    expect(emittedGraph.nodes.find((item) => item.id === "restore").step.name).toBe("restore-primary");
    expect(wrapper.find('[data-testid="node-config-modal"]').exists()).toBe(false);
  });

  it("updates next-step edges from the docked node panel", async () => {
    const workflow = {
      ...workflowWithGraph,
      graph: {
        ...workflowWithGraph.graph,
        nodes: [
          ...workflowWithGraph.graph.nodes,
          {
            id: "notify",
            type: "action",
            position: { x: 580, y: 120 },
            step: { name: "notify", action: "notify.send" },
          },
          {
            id: "audit",
            type: "action",
            position: { x: 760, y: 120 },
            step: { name: "audit", action: "notify.send" },
          },
        ],
        edges: [
          ...workflowWithGraph.graph.edges,
          {
            id: "restore-notify-failure",
            source: "restore",
            target: "notify",
            kind: "failure",
            source_port: "failure",
            target_port: "in",
          },
        ],
      },
    };
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflow],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("click");
    await wrapper.get('[data-testid="next-step-select-failure"]').setValue("audit");

    const emittedGraph = wrapper.emitted("update-workflow-graph")?.[0]?.[0];
    expect(emittedGraph.edges).toContainEqual(
      expect.objectContaining({
        source: "restore",
        target: "audit",
        kind: "failure",
        source_port: "failure",
      }),
    );
  });

  it("opens the AI assistant modal from the topbar AI action", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="runner-toolbar-ai-generate"]').trigger("click");

    expect(wrapper.get('[data-testid="runner-ai-modal"]').text()).toContain("AI 生成工作流草稿");
  });

  it("opens the publish review modal from the topbar publish action", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflowWithGraph],
        actions,
        selectedWorkflowName: "pg-restore",
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="runner-toolbar-publish"]').trigger("click");

    expect(wrapper.get('[data-testid="publish-review-modal"]').text()).toContain("发布审阅");
  });
});
