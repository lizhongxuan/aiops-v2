import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import GraphDiffSummary from "./GraphDiffSummary.vue";

describe("GraphDiffSummary", () => {
  it("shows execution semantic diff and UI layout diff separately", () => {
    const wrapper = mount(GraphDiffSummary, {
      props: {
        diff: {
          semantic_changes: [{ title: "restore action", detail: "shell.run -> script.shell" }],
          layout_changes: [{ title: "restore position", detail: "x: 220 -> 420" }],
        },
      },
    });

    expect(wrapper.text()).toContain("执行语义 diff");
    expect(wrapper.text()).toContain("restore action");
    expect(wrapper.text()).toContain("shell.run -> script.shell");
    expect(wrapper.text()).toContain("UI layout diff");
    expect(wrapper.text()).toContain("restore position");
    expect(wrapper.text()).toContain("x: 220 -> 420");
  });

  it("shows semantic conflict when graph cannot be linearized", () => {
    const wrapper = mount(GraphDiffSummary, {
      props: {
        diff: {
          semantic_conflicts: [{ title: "branch fan-in", detail: "缺少 join 节点" }],
          linearizable: false,
        },
      },
    });

    expect(wrapper.text()).toContain("语义冲突");
    expect(wrapper.text()).toContain("不可线性化");
    expect(wrapper.text()).toContain("不允许生成顺序假象");
    expect(wrapper.text()).toContain("缺少 join 节点");
  });
});
