import { describe, expect, it } from "vitest";

import { firstRunnableNodeId, getRunnerFocusNodeId, getRunnerNodeRunState } from "./runnerRunVisualState";

const graph = {
  nodes: [
    { id: "start", type: "start" },
    { id: "cmd-run", type: "action", step: { action: "cmd.run" } },
    { id: "approval", type: "action", step: { action: "manual.approval" } },
    { id: "end", type: "end" },
  ],
};

describe("runnerRunVisualState", () => {
  it("finds the first runnable node after start/end system nodes", () => {
    expect(firstRunnableNodeId(graph)).toBe("cmd-run");
  });

  it("maps node run history to canvas visual states", () => {
    const runState = {
      nodes: {
        "cmd-run": { status: "running", message: "executing" },
        approval: { status: "success" },
        end: { status: "failed", error: "boom" },
      },
    };

    expect(getRunnerNodeRunState(runState, "cmd-run")).toMatchObject({ status: "running", label: "运行中" });
    expect(getRunnerNodeRunState(runState, "approval")).toMatchObject({ status: "success", label: "成功" });
    expect(getRunnerNodeRunState(runState, "end")).toMatchObject({ status: "failed", label: "失败" });
    expect(getRunnerNodeRunState(runState, "missing")).toEqual({ status: "", label: "", message: "" });
  });

  it("focuses failed nodes before running nodes and explicit run targets", () => {
    expect(
      getRunnerFocusNodeId({
        graph,
        explicitNodeId: "cmd-run",
        runState: { nodes: { "cmd-run": { status: "running" }, approval: { status: "failed" } } },
      }),
    ).toBe("approval");

    expect(
      getRunnerFocusNodeId({
        graph,
        explicitNodeId: "approval",
        runState: { nodes: { "cmd-run": { status: "running" } } },
      }),
    ).toBe("cmd-run");

    expect(getRunnerFocusNodeId({ graph, explicitNodeId: "approval", runState: { nodes: {} } })).toBe("approval");
  });
});
