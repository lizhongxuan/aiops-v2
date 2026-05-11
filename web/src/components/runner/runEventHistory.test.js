import { describe, expect, it } from "vitest";
import {
  extractRunnerRunEvents,
  finalRunnerRunStatus,
  isRunnerRunHistoryTerminal,
  mapRunnerRunEventsToGraph,
  unwrapRunnerPayload,
} from "./runEventHistory";

describe("runEventHistory", () => {
  it("unwraps direct, data, items, and events run history payloads", () => {
    const runStart = { type: "run_start", run_id: "run-1" };
    expect(unwrapRunnerPayload({ data: { run_id: "run-1" } })).toEqual({ run_id: "run-1" });
    expect(extractRunnerRunEvents([runStart])).toEqual([runStart]);
    expect(extractRunnerRunEvents({ items: [runStart] })).toEqual([runStart]);
    expect(extractRunnerRunEvents({ data: { events: [runStart] } })).toEqual([runStart]);
  });

  it("maps runner step-name events back to graph node ids after node rename", () => {
    const graph = {
      nodes: [
        { id: "cmd-run", step: { id: "cmd-run", name: "e2e-command", action: "cmd.run" } },
        { id: "end", type: "end" },
      ],
    };
    const mapped = mapRunnerRunEventsToGraph(
      [
        { type: "step_start", step: "e2e-command", status: "running" },
        { type: "host_result", step: "e2e-command", output: { stdout: "hello\n" }, status: "success" },
      ],
      graph,
    );
    expect(mapped[0]).toMatchObject({ node_id: "cmd-run", step_name: "e2e-command" });
    expect(mapped[1]).toMatchObject({ node_id: "cmd-run", step_name: "e2e-command" });
  });

  it("adds start and end node states around step-only run history", () => {
    const graph = {
      nodes: [
        { id: "start", type: "start" },
        { id: "shell-run", type: "action", step: { name: "shell-run", action: "shell.run" } },
        { id: "end", type: "end" },
      ],
    };

    const mapped = mapRunnerRunEventsToGraph(
      [
        { type: "run_start", run_id: "run-1", status: "running", timestamp: "10:00:00" },
        { type: "step_start", run_id: "run-1", step: "shell-run", status: "running", timestamp: "10:00:01" },
        { type: "step_finish", run_id: "run-1", step: "shell-run", status: "success", timestamp: "10:00:02" },
        { type: "run_finish", run_id: "run-1", status: "success", timestamp: "10:00:03" },
      ],
      graph,
    );

    expect(mapped).toEqual([
      expect.objectContaining({ type: "run_start" }),
      expect.objectContaining({ type: "node_started", node_id: "start", status: "running" }),
      expect.objectContaining({ type: "node_finished", node_id: "start", status: "success" }),
      expect.objectContaining({ type: "step_start", node_id: "shell-run" }),
      expect.objectContaining({ type: "step_finish", node_id: "shell-run" }),
      expect.objectContaining({ type: "node_started", node_id: "end", status: "running" }),
      expect.objectContaining({ type: "node_finished", node_id: "end", status: "success" }),
      expect.objectContaining({ type: "run_finish" }),
    ]);
  });

  it("marks the end node failed when the run finishes failed", () => {
    const graph = { nodes: [{ id: "start", type: "start" }, { id: "shell-run", type: "action", step: { name: "shell-run" } }, { id: "end", type: "end" }] };
    const mapped = mapRunnerRunEventsToGraph(
      [
        { type: "run_start", run_id: "run-1", status: "running" },
        { type: "step_finish", run_id: "run-1", step: "shell-run", status: "failed", message: "agent offline" },
        { type: "run_finish", run_id: "run-1", status: "failed", message: "agent offline" },
      ],
      graph,
    );

    expect(mapped).toContainEqual(expect.objectContaining({ type: "node_finished", node_id: "end", status: "failed", message: "agent offline" }));
  });

  it("detects terminal run history and final status", () => {
    const events = [
      { type: "run_start", status: "running" },
      { type: "run_finish", status: "success" },
    ];
    expect(isRunnerRunHistoryTerminal(events)).toBe(true);
    expect(finalRunnerRunStatus(events)).toBe("success");
  });

  it("treats failed host results as a failed final status while the runner is still flushing", () => {
    const events = [
      { type: "run_start", status: "running" },
      { type: "host_result", step: "deploy", status: "failed", message: "agent dispatch failed" },
    ];
    expect(isRunnerRunHistoryTerminal(events)).toBe(true);
    expect(finalRunnerRunStatus(events)).toBe("failed");
  });
});
