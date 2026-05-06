import { describe, expect, it } from "vitest";
import { createInitialRunState, reduceRunEvents } from "./runStateReducer";

describe("runStateReducer", () => {
  it("coalesces SSE run events into node, edge, host, log, approval, retry, and variable state", () => {
    const state = reduceRunEvents(
      [
        { type: "run.started", run_id: "run-1", ts: "10:00:00" },
        { type: "node.started", node_id: "restore", status: "running", ts: "10:00:01" },
        { type: "edge.traversed", edge_id: "pre-restore", source: "pre", target: "restore", ts: "10:00:02" },
        { type: "host.stdout", node_id: "restore", host_id: "pg-01", message: "restore started" },
        { type: "host.stderr", node_id: "restore", host_id: "pg-01", message: "waiting for replay" },
        { type: "sse.event", event: "vars.exported", message: "restore_lsn exported" },
        { type: "approval.requested", approval_id: "approval-1", node_id: "restore", summary: "promote primary" },
        { type: "retry.scheduled", node_id: "restore", attempt: 2, max_attempts: 3, reason: "timeout" },
        { type: "vars.input", node_id: "restore", key: "backup_id", value: "b-1" },
        { type: "vars.output", node_id: "restore", key: "restore_lsn", value: "0/16B6C50" },
        { type: "vars.exported", key: "promoted", value: false },
        { type: "node.completed", node_id: "restore", status: "success", duration_ms: 42000, result: { exit_code: 0 } },
      ],
      createInitialRunState(),
    );

    expect(state.runId).toBe("run-1");
    expect(state.nodes.restore).toMatchObject({ status: "success", durationMs: 42000, result: { exit_code: 0 } });
    expect(state.edges["pre-restore"]).toMatchObject({ source: "pre", target: "restore", status: "traversed" });
    expect(state.hosts["pg-01"]).toMatchObject({ hostId: "pg-01", nodeId: "restore" });
    expect(state.logs.map((log) => log.stream)).toEqual(["stdout", "stderr", "sse"]);
    expect(state.approvals[0]).toMatchObject({ id: "approval-1", nodeId: "restore", status: "pending" });
    expect(state.retries[0]).toMatchObject({ nodeId: "restore", attempt: 2, maxAttempts: 3 });
    expect(state.variables.inputs[0]).toMatchObject({ nodeId: "restore", key: "backup_id", value: "b-1" });
    expect(state.variables.outputs[0]).toMatchObject({ nodeId: "restore", key: "restore_lsn", value: "0/16B6C50" });
    expect(state.variables.exports[0]).toMatchObject({ key: "promoted", value: false });
    expect(state.variables.nodeResults[0]).toMatchObject({ nodeId: "restore", result: { exit_code: 0 } });
  });

  it("replays runner server run history events into the same canvas run state", () => {
    const state = reduceRunEvents(
      [
        { type: "run_queued", run_id: "run-2", status: "queued", timestamp: "10:00:00" },
        { type: "run_start", run_id: "run-2", status: "running", timestamp: "10:00:01" },
        { type: "step_start", run_id: "run-2", step: "restore", status: "running", timestamp: "10:00:02" },
        {
          type: "host_result",
          run_id: "run-2",
          step: "restore",
          host: "pg-01",
          status: "success",
          message: "restore ok",
          output: { stdout: "ok", exit_code: 0 },
        },
        { type: "step_finish", run_id: "run-2", step: "restore", status: "success", timestamp: "10:00:04" },
        { type: "run_finish", run_id: "run-2", status: "success", timestamp: "10:00:05" },
      ],
      createInitialRunState(),
    );

    expect(state.runId).toBe("run-2");
    expect(state.status).toBe("success");
    expect(state.nodes.restore).toMatchObject({ nodeId: "restore", status: "success", result: { stdout: "ok", exit_code: 0 } });
    expect(state.hosts["pg-01"]).toMatchObject({ hostId: "pg-01", nodeId: "restore", lastStream: "stdout" });
    expect(state.logs[0]).toMatchObject({ stream: "stdout", nodeId: "restore", hostId: "pg-01", message: "ok" });
    expect(state.variables.nodeResults[0]).toMatchObject({ nodeId: "restore", result: { stdout: "ok", exit_code: 0 } });
  });
});
