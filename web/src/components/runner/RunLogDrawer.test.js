import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunLogDrawer from "./RunLogDrawer.vue";

const state = {
  nodes: {
    restore: {
      nodeId: "restore",
      status: "success",
      durationMs: 42000,
      result: { exit_code: 0, summary: "restore completed" },
    },
  },
  logs: [
    { stream: "stdout", nodeId: "restore", hostId: "pg-01", message: "restore started" },
    { stream: "stderr", nodeId: "restore", hostId: "pg-01", message: "waiting for replay" },
    { stream: "sse", event: "vars.exported", message: "restore_lsn exported" },
  ],
  approvals: [{ id: "approval-1", nodeId: "restore", summary: "promote primary", status: "pending" }],
  retries: [{ nodeId: "restore", attempt: 2, maxAttempts: 3, reason: "timeout" }],
};

describe("RunLogDrawer", () => {
  it("shows stdout, stderr, SSE, approval events, and retry trace", () => {
    const wrapper = mount(RunLogDrawer, {
      props: { state },
    });

    expect(wrapper.text()).toContain("stdout");
    expect(wrapper.text()).toContain("restore started");
    expect(wrapper.text()).toContain("stderr");
    expect(wrapper.text()).toContain("waiting for replay");
    expect(wrapper.text()).toContain("SSE");
    expect(wrapper.text()).toContain("restore_lsn exported");
    expect(wrapper.text()).toContain("审批事件");
    expect(wrapper.text()).toContain("promote primary");
    expect(wrapper.text()).toContain("重试轨迹");
    expect(wrapper.text()).toContain("2/3");
  });

  it("shows the recent run summary for the selected node and opens node detail", async () => {
    const wrapper = mount(RunLogDrawer, {
      props: { state, selectedNodeId: "restore" },
    });

    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("restore");
    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("success");
    expect(wrapper.get('[data-testid="selected-node-run-summary"]').text()).toContain("42s");

    await wrapper.get('[data-testid="open-node-run-detail"]').trigger("click");
    expect(wrapper.emitted("open-node-detail")?.[0]).toEqual(["restore"]);
  });
});
