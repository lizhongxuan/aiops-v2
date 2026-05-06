import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { createInitialRunState } from "./runStateReducer";
import RunnerNodePanel from "./RunnerNodePanel.vue";

const actions = [
  { action: "cmd.run" },
  {
    action: "shell.run",
    inputs_schema: {
      type: "object",
      required: ["script"],
      properties: {
        script: { type: "string", title: "Script" },
      },
    },
  },
];
const node = {
  id: "restore",
  type: "action",
  step: { name: "restore", action: "shell.run", targets: ["pg-01"], retries: 1, timeout: "30s" },
  inputs: [{ key: "script", type: "string" }],
  outputs: [{ key: "restore_lsn", type: "string" }],
};

describe("RunnerNodePanel", () => {
  it("renders node identity, action, run status and configuration tabs", async () => {
    const runState = {
      ...createInitialRunState(),
      nodes: {
        restore: { nodeId: "restore", status: "success", durationMs: 42000 },
      },
    };
    const wrapper = mount(RunnerNodePanel, {
      props: { node, actions, runState },
    });

    expect(wrapper.get('[data-testid="runner-node-panel"]').text()).toContain("restore");
    expect(wrapper.get('[data-testid="runner-node-panel"]').text()).toContain("shell.run");
    expect(wrapper.get('[data-testid="runner-node-panel"]').text()).toContain("success");
    expect(wrapper.get('[data-testid="runner-node-panel-run"]').text()).toContain("运行");
    expect(wrapper.find(".runner-node-panel-footer").exists()).toBe(false);
    for (const label of ["设置", "输入", "输出", "高级", "上次运行"]) {
      expect(wrapper.get('[data-testid="runner-node-panel-tabs"]').text()).toContain(label);
    }

    await wrapper.get('[data-testid="runner-node-panel-tab-input"]').trigger("click");
    expect(wrapper.get('[data-testid="input-tab"]').text()).toContain("script");

    await wrapper.get('[data-testid="runner-node-panel-tab-settings"]').trigger("click");
    await wrapper.get('[data-testid="basic-name"]').setValue("restore-primary");
    await wrapper.get('[data-testid="runner-node-panel-apply"]').trigger("click");

    expect(wrapper.emitted("apply")?.[0]?.[0].step.name).toBe("restore-primary");
  });

  it("emits close and single-node run actions from the panel header", async () => {
    const wrapper = mount(RunnerNodePanel, {
      props: { node, actions },
    });

    await wrapper.get('[data-testid="runner-node-panel-run"]').trigger("click");
    await wrapper.get('[data-testid="runner-node-panel-close"]').trigger("click");

    expect(wrapper.emitted("run-node")?.[0]).toEqual(["restore"]);
    expect(wrapper.emitted("close")?.[0]).toEqual([]);
  });

  it("opens run details from the compact header action", async () => {
    const runState = {
      ...createInitialRunState(),
      nodes: {
        restore: { nodeId: "restore", status: "success", durationMs: 42000 },
      },
    };
    const wrapper = mount(RunnerNodePanel, {
      props: { node, actions, runState },
    });

    await wrapper.get('[data-testid="runner-node-panel-open-run"]').trigger("click");

    expect(wrapper.emitted("open-run-details")?.[0]).toEqual(["restore"]);
  });

  it("uses dedicated settings panels for action and condition nodes", async () => {
    const wrapper = mount(RunnerNodePanel, {
      props: { node, actions },
    });

    expect(wrapper.find('[data-testid="action-node-panel"]').exists()).toBe(true);

    await wrapper.setProps({
      node: {
        id: "gate",
        type: "condition",
        step: { name: "gate", action: "condition.evaluate", args: {} },
      },
    });

    expect(wrapper.find('[data-testid="condition-node-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="action-node-panel"]').exists()).toBe(false);
  });

  it("edits workflow inputs from the start node without creating private node state", async () => {
    const graph = {
      version: "v1",
      workflow: {
        name: "pg-restore",
        inputs: [{ key: "backup_id", type: "string", required: true }],
      },
      nodes: [{ id: "start", type: "start", label: "Start" }, node],
      edges: [{ id: "start-restore", source: "start", target: "restore", kind: "next" }],
    };
    const wrapper = mount(RunnerNodePanel, {
      props: { node: graph.nodes[0], actions, graph },
    });

    await wrapper.get('[data-testid="runner-node-panel-tab-input"]').trigger("click");
    await wrapper.get('[data-testid="input-key-backup_id"]').setValue("backup_uri");
    await wrapper.get('[data-testid="input-type-backup_uri"]').setValue("string");

    const emittedGraph = wrapper.emitted("update:graph")?.at(-1)?.[0];
    expect(emittedGraph.workflow.inputs[0]).toMatchObject({
      key: "backup_uri",
      type: "string",
    });
    expect(emittedGraph.nodes.find((item) => item.id === "start")).not.toHaveProperty("inputs");
  });

  it("keeps unapplied action args when editing next-step edges", async () => {
    const graph = {
      version: "v1",
      workflow: { name: "host-resource-check" },
      nodes: [
        {
          ...node,
          ports: [
            { id: "in", type: "input", label: "输入" },
            { id: "next", type: "output", label: "下一步" },
            { id: "failure", type: "output", label: "失败" },
          ],
        },
        { id: "end", type: "end", label: "End", ports: [{ id: "in", type: "input", label: "输入" }] },
      ],
      edges: [],
    };
    const wrapper = mount(RunnerNodePanel, {
      props: { node: graph.nodes[0], actions, graph },
    });

    await wrapper.get('[data-testid="action-schema-field-script"]').setValue("df -h && uptime");
    await wrapper.get('[data-testid="next-step-select-next"]').setValue("end");

    const emittedGraph = wrapper.emitted("update:graph")?.at(-1)?.[0];
    expect(emittedGraph.nodes.find((item) => item.id === "restore").step.args.script).toBe("df -h && uptime");

    await wrapper.get('[data-testid="runner-node-panel-apply"]').trigger("click");

    expect(wrapper.emitted("apply")?.at(-1)?.[0].step.args.script).toBe("df -h && uptime");
  });
});
