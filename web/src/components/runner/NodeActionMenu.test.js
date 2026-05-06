import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import NodeActionMenu from "./NodeActionMenu.vue";

describe("NodeActionMenu", () => {
  it("contains the production node actions and emits the chosen command", async () => {
    const wrapper = mount(NodeActionMenu, {
      props: {
        node: { id: "restore", label: "restore", type: "action" },
        x: 120,
        y: 80,
      },
    });

    for (const label of ["复制", "删除", "禁用", "单节点试跑", "最近运行", "AI 修复"]) {
      expect(wrapper.text()).toContain(label);
    }

    await wrapper.get('[data-testid="node-action-ai-fix"]').trigger("click");

    expect(wrapper.emitted("action")?.[0]).toEqual(["ai-fix", "restore"]);
  });
});
