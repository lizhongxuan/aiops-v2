import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import NodeConfigModal from "./NodeConfigModal.vue";
import RunnerStudioShell from "./RunnerStudioShell.vue";

const actions = [{ action: "cmd.run" }, { action: "shell.run" }];
const workflow = {
  name: "pg-restore",
  title: "PG Restore",
  status: "draft",
  graph: {
    version: "v1",
    workflow: { name: "pg-restore" },
    nodes: [
      { id: "start", type: "start", position: { x: 40, y: 140 }, label: "Start" },
      {
        id: "restore",
        type: "action",
        position: { x: 260, y: 140 },
        step: { name: "restore", action: "cmd.run", targets: ["pg-01"], retries: 1, timeout: "30s" },
        inputs: [{ key: "script", type: "string" }],
        outputs: [{ key: "restore_lsn", type: "string" }],
      },
    ],
    edges: [{ id: "start-restore", source: "start", target: "restore", kind: "next" }],
  },
};

describe("NodeConfigModal", () => {
  it("renders five tabs for action node configuration", async () => {
    const wrapper = mount(NodeConfigModal, {
      props: {
        show: true,
        node: workflow.graph.nodes[1],
        actions,
      },
    });

    for (const label of ["基础", "输入", "输出", "高级", "运行与 AI"]) {
      expect(wrapper.get('[data-testid="node-config-tabs"]').text()).toContain(label);
    }

    await wrapper.get('[data-testid="tab-input"]').trigger("click");
    expect(wrapper.get('[data-testid="input-tab"]').text()).toContain("script");

    await wrapper.get('[data-testid="tab-output"]').trigger("click");
    expect(wrapper.get('[data-testid="output-tab"]').text()).toContain("restore_lsn");

    await wrapper.get('[data-testid="tab-advanced"]').trigger("click");
    expect(wrapper.get('[data-testid="advanced-tab"]').text()).toContain("when");
    expect(wrapper.get('[data-testid="advanced-tab"]').text()).toContain("join/loop/subflow");

    await wrapper.get('[data-testid="tab-run-ai"]').trigger("click");
    expect(wrapper.get('[data-testid="run-ai-tab"]').text()).toContain("最近试跑");
    expect(wrapper.get('[data-testid="run-ai-tab"]').text()).toContain("AI diff");
  });

  it("opens the docked node panel from canvas double click and applies basic edits to graph", async () => {
    const wrapper = mount(RunnerStudioShell, {
      props: {
        workflows: [workflow],
        selectedWorkflowName: "pg-restore",
        actions,
        workflowUiState: { recent: ["pg-restore"], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="canvas-node-restore"]').trigger("dblclick");
    expect(wrapper.get('[data-testid="runner-node-panel"]').text()).toContain("restore");
    expect(wrapper.find('[data-testid="node-config-modal"]').exists()).toBe(false);

    await wrapper.get('[data-testid="basic-name"]').setValue("restore-primary");
    await wrapper.get('[data-testid="runner-node-panel-apply"]').trigger("click");

    const emittedGraph = wrapper.emitted("update-workflow-graph")?.[0]?.[0];
    expect(emittedGraph.nodes.find((node) => node.id === "restore").step.name).toBe("restore-primary");
  });
});
