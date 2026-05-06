import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerRunPanel from "./RunnerRunPanel.vue";

const state = {
  status: "running",
  runId: "run_20260505_001",
  nodes: {
    restore: { nodeId: "restore", status: "success", durationMs: 42000, result: { exit_code: 0 } },
    verify: { nodeId: "verify", status: "running", durationMs: 1200 },
  },
  logs: [
    { nodeId: "restore", stream: "stdout", hostId: "pg-01", message: "restore ok" },
    { nodeId: "verify", stream: "stderr", hostId: "pg-01", message: "waiting" },
    { stream: "sse", event: "vars.exported", message: "restore_lsn" },
  ],
  approvals: [{ id: "approval-1", status: "pending", summary: "DBA approval" }],
  retries: [{ nodeId: "restore", attempt: 2, maxAttempts: 3, reason: "timeout" }],
  variables: {
    inputs: [{ nodeId: "restore", key: "backup_id", value: "b-1" }],
    outputs: [{ nodeId: "restore", key: "restore_lsn", value: "0/16B6C50" }],
    exports: [{ key: "promoted", value: false }],
    nodeResults: [{ nodeId: "restore", result: { exit_code: 0 } }],
  },
  artifacts: [{ nodeId: "restore", name: "restore.log", url: "/artifacts/restore.log" }],
};

describe("RunnerRunPanel", () => {
  it("shows a compact empty state before the first real run", () => {
    const wrapper = mount(RunnerRunPanel, {
      props: {
        state: { status: "idle", nodes: {}, logs: [], variables: {}, approvals: [], retries: [], artifacts: [] },
        graph: {
          nodes: [{ id: "cmd-run", step: { name: "cmd-run", action: "cmd.run" } }],
          edges: [],
        },
      },
    });

    const text = wrapper.get('[data-testid="runner-run-panel"]').text();
    expect(text).toContain("暂无运行记录");
    expect(text).not.toContain("local-draft");
    expect(text).not.toContain("not_run");
    expect(wrapper.find('[data-testid="runner-run-trace-cmd-run"]').exists()).toBe(false);
  });

  it("shows overview, node trace, logs, variables, approvals, retries, and artifacts", () => {
    const wrapper = mount(RunnerRunPanel, {
      props: { state, selectedNodeId: "restore" },
    });

    const text = wrapper.get('[data-testid="runner-run-panel"]').text();
    expect(text).toContain("run_20260505_001");
    expect(text).toContain("restore");
    expect(text).toContain("success");
    expect(text).toContain("restore ok");
    expect(text).toContain("waiting");
    expect(text).toContain("DBA approval");
    expect(text).toContain("2/3");
    expect(text).toContain("backup_id=b-1");
    expect(text).toContain("restore_lsn=0/16B6C50");
    expect(text).toContain("restore.log");
  });

  it("emits node selection and node detail events from trace rows", async () => {
    const wrapper = mount(RunnerRunPanel, {
      props: { state },
    });

    await wrapper.get('[data-testid="runner-run-trace-restore"]').trigger("click");
    await wrapper.get('[data-testid="runner-run-detail-restore"]').trigger("click");

    expect(wrapper.emitted("select-node")?.[0]).toEqual(["restore"]);
    expect(wrapper.emitted("open-node-detail")?.[0]).toEqual(["restore"]);
  });
});
