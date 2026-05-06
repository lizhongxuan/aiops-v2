import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import NodePicker from "./NodePicker.vue";

const actions = [
  { action: "cmd.run", label: "Command", category: "基础", description: "执行命令" },
  { action: "shell.run", label: "Shell Script", category: "基础", description: "执行脚本" },
  { action: "notify.send", label: "Notify", category: "治理", description: "发送通知" },
  { action: "approval.wait", label: "Approval", category: "治理", description: "等待审批" },
  { action: "wait.event", label: "Wait", category: "逻辑", description: "等待事件" },
];

describe("NodePicker", () => {
  it("groups failure follow-up recommendations ahead of the full catalog", () => {
    const wrapper = mount(NodePicker, {
      props: {
        actions,
        sourcePort: "failure",
      },
    });

    const recommended = wrapper.get('[data-testid="node-picker-section-recommended"]');
    expect(recommended.text()).toContain("推荐节点");
    expect(recommended.findAll(".runner-action-palette-item").map((item) => item.text())).toEqual([
      expect.stringContaining("Notify"),
      expect.stringContaining("Approval"),
      expect.stringContaining("Wait"),
    ]);
    expect(recommended.text()).not.toContain("Command");
  });

  it("shows recently used actions without duplicating recommended actions", () => {
    const wrapper = mount(NodePicker, {
      props: {
        actions,
        sourcePort: "failure",
        recentActionKeys: ["shell.run", "notify.send"],
      },
    });

    expect(wrapper.get('[data-testid="node-picker-section-recommended"]').text()).toContain("Notify");
    const recent = wrapper.get('[data-testid="node-picker-section-recent"]');
    expect(recent.text()).toContain("最近使用");
    expect(recent.text()).toContain("Shell Script");
    expect(recent.text()).not.toContain("Notify");
  });

  it("filters grouped actions by search keyword", async () => {
    const wrapper = mount(NodePicker, {
      props: {
        actions,
        sourcePort: "failure",
        recentActionKeys: ["shell.run"],
      },
    });

    await wrapper.get('input[aria-label="搜索节点"]').setValue("notify");

    expect(wrapper.text()).toContain("Notify");
    expect(wrapper.text()).not.toContain("Shell Script");
    expect(wrapper.text()).not.toContain("Approval");
  });
});
