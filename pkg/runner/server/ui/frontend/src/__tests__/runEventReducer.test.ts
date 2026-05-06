import { describe, expect, it } from "vitest";
import { initialRunState, reduceRunEvent } from "../utils/runEventReducer";

describe("reduceRunEvent", () => {
  it("tracks node status and active nodes", () => {
    const running = reduceRunEvent(initialRunState, {
      type: "node_started",
      run_id: "run-1",
      status: "running",
      output: {
        node_id: "restart",
      },
    });

    expect(running.runId).toBe("run-1");
    expect(running.status).toBe("running");
    expect(running.activeNodeIds).toEqual(["restart"]);

    const done = reduceRunEvent(running, {
      type: "node_finished",
      run_id: "run-1",
      status: "success",
      output: {
        node_id: "restart",
      },
    });

    expect(done.nodeStatus.restart).toBe("success");
    expect(done.activeNodeIds).toEqual([]);
    expect(done.timeline).toHaveLength(2);
  });

  it("keeps graph run status unchanged for selected edges", () => {
    const running = reduceRunEvent(initialRunState, {
      type: "node_started",
      run_id: "run-1",
      status: "running",
      output: {
        node_id: "restore",
      },
    });

    const next = reduceRunEvent(running, {
      type: "edge_selected",
      run_id: "run-1",
      status: "selected",
      output: {
        edge_id: "edge-restore-verify",
      },
    });

    expect(next.status).toBe("running");
    expect(next.edgeStatus["edge-restore-verify"]).toBe("selected");
    expect(next.timeline[0].edgeId).toBe("edge-restore-verify");
  });

  it("does not mark a whole run successful from a successful node event", () => {
    const running = reduceRunEvent(initialRunState, {
      type: "run_start",
      run_id: "run-1",
      status: "running",
    });

    const nodeDone = reduceRunEvent(running, {
      type: "node_finished",
      run_id: "run-1",
      status: "success",
      output: { node_id: "restore" },
    });

    expect(nodeDone.status).toBe("running");
    expect(nodeDone.nodeStatus.restore).toBe("success");

    const runDone = reduceRunEvent(nodeDone, {
      type: "run_finish",
      run_id: "run-1",
      status: "success",
    });
    expect(runDone.status).toBe("success");
  });

  it("tracks manual approval waiting and resolved events", () => {
    const waiting = reduceRunEvent(initialRunState, {
      type: "approval_waiting",
      run_id: "run-approval",
      status: "waiting",
      message: "Waiting for approval",
      output: { node_id: "approve" },
    });

    expect(waiting.status).toBe("waiting");
    expect(waiting.nodeStatus.approve).toBe("waiting");
    expect(waiting.activeNodeIds).toEqual(["approve"]);

    const resolved = reduceRunEvent(waiting, {
      type: "approval_resolved",
      run_id: "run-approval",
      status: "success",
      message: "approved",
      output: { node_id: "approve" },
    });

    expect(resolved.status).toBe("running");
    expect(resolved.nodeStatus.approve).toBe("success");
    expect(resolved.activeNodeIds).toEqual([]);
  });

  it("collects host results, output streams, exported vars, and runner debug", () => {
    const withChunk = reduceRunEvent(initialRunState, {
      type: "output_delta",
      run_id: "run-1",
      step: "restore",
      host: "pg-01",
      output: {
        stream: "stdout",
        chunk: "starting restore\n",
      },
      timestamp: "2026-05-03T00:00:02Z",
    });

    const next = reduceRunEvent(withChunk, {
      type: "host_result",
      run_id: "run-1",
      step: "restore",
      host: "pg-01",
      status: "success",
      message: "host completed",
      output: {
        stdout: "starting restore\ndone\n",
        stderr: "warning: replay lag\n",
        exit_code: 0,
        vars: { restore_lsn: "0/42" },
        runner_debug: { resolved_address: "10.0.0.11" },
      },
      timestamp: "2026-05-03T00:00:03Z",
    });

    expect(next.hostResults).toHaveLength(1);
    expect(next.hostResults[0]).toMatchObject({
      step: "restore",
      host: "pg-01",
      status: "success",
      exitCode: 0,
    });
    expect(next.stdout.map((line) => line.content)).toEqual(["starting restore\n"]);
    expect(next.stderr.map((line) => line.content)).toEqual(["warning: replay lag\n"]);
    expect(next.exportedVars).toEqual({ restore_lsn: "0/42" });
    expect(next.runnerDebug).toEqual({ "restore/pg-01": { resolved_address: "10.0.0.11" } });
  });

  it("replays 1000 SSE events with bounded timeline and log buffers", () => {
    let state = reduceRunEvent(initialRunState, {
      type: "run_start",
      run_id: "run-load",
      status: "running",
    });

    const startedAt = performance.now();
    for (let index = 0; index < 1000; index += 1) {
      state = reduceRunEvent(state, {
        id: `evt-${index}`,
        type: "output_delta",
        run_id: "run-load",
        step: "restore",
        host: "pg-primary",
        output: {
          stream: "stdout",
          chunk: `chunk-${index}\n`,
        },
      });
    }
    const durationMs = performance.now() - startedAt;

    expect(state.status).toBe("running");
    expect(state.timeline).toHaveLength(200);
    expect(state.stdout).toHaveLength(500);
    expect(state.stdout[0].content).toBe("chunk-999\n");
    expect(durationMs).toBeLessThan(1000);
  });
});
