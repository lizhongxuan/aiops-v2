import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerNextStepEditor from "./RunnerNextStepEditor.vue";

const graph = {
  version: "v1",
  workflow: { name: "demo" },
  nodes: [
    {
      id: "gate",
      type: "condition",
      step: { name: "gate", action: "condition.evaluate" },
    },
    { id: "restore", type: "action", step: { name: "restore", action: "shell.run" } },
    { id: "notify", type: "action", step: { name: "notify", action: "notify.send" } },
  ],
  edges: [{ id: "gate-restore-if", source: "gate", target: "restore", kind: "if", source_port: "if", target_port: "in" }],
};

describe("RunnerNextStepEditor", () => {
  it("shows one selector per source port and updates the graph edge target", async () => {
    const wrapper = mount(RunnerNextStepEditor, {
      props: { node: graph.nodes[0], graph },
    });

    expect(wrapper.get('[data-testid="runner-next-step-editor"]').text()).toContain("IF");
    expect(wrapper.get('[data-testid="runner-next-step-editor"]').text()).toContain("ELSE");
    expect(wrapper.get('[data-testid="next-step-select-if"]').element.value).toBe("restore");

    await wrapper.get('[data-testid="next-step-select-if"]').setValue("notify");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges).toEqual([
      expect.objectContaining({
        source: "gate",
        target: "notify",
        kind: "if",
        source_port: "if",
        target_port: "in",
      }),
    ]);
  });

  it("deletes an existing edge from the explicit next-step control", async () => {
    const wrapper = mount(RunnerNextStepEditor, {
      props: { node: graph.nodes[0], graph },
    });

    await wrapper.get('[data-testid="next-step-delete-if"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges).toEqual([]);
  });

  it("hides implicit failure-to-end routing because failures exit by default", () => {
    const graphWithImplicitFailure = {
      ...graph,
      nodes: [
        { id: "restore", type: "action", step: { name: "restore", action: "shell.run" } },
        { id: "end", type: "end", label: "End" },
      ],
      edges: [
        {
          id: "restore-end-failure",
          source: "restore",
          target: "end",
          kind: "failure",
          source_port: "failure",
          target_port: "in",
        },
      ],
    };

    const wrapper = mount(RunnerNextStepEditor, {
      props: { node: graphWithImplicitFailure.nodes[0], graph: graphWithImplicitFailure },
    });

    expect(wrapper.find('[data-testid="next-step-select-failure"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="runner-next-step-editor"]').text()).toContain("失败未设置时默认退出");
  });
});
