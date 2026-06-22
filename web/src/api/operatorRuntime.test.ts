import { describe, expect, it, vi } from "vitest";

import { createOperatorRuntimeClient } from "./operatorRuntime";
import type { OperatorRuntimeError } from "./operatorRuntime";

function createRecordingHttpClient(payload: unknown = { items: [] }) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return Promise.resolve(payload);
    }),
    post: vi.fn((path: string, body?: unknown) => {
      calls.push({ method: "POST", path, body });
      return Promise.resolve(payload);
    }),
  };
}

describe("operator runtime API", () => {
  it("targets the generic operator runtime collection endpoints", async () => {
    const http = createRecordingHttpClient({ item: { id: "created" } });
    const api = createOperatorRuntimeClient(http);

    await api.listResources();
    await api.createResource({ name: "redis-cache", kind: "redis" });
    await api.listInspectionTemplates();
    await api.createInspectionTemplate({ name: "pg-latency", checks: ["replication_lag"] });
    await api.listProblemTypes();
    await api.createProblemType({ name: "replication_lag", severity: "warning" });
    await api.listActions();
    await api.createAction({ name: "restart_replica", kind: "workflow" });
    await api.listWorkflowBindings();
    await api.createWorkflowBinding({ name: "restart-replica-binding", workflowName: "restart-replica" });
    await api.listRules();
    await api.createRule({ name: "pg-replication-autoheal", enabled: false });

    expect(http.calls).toEqual([
      { method: "GET", path: "/api/v1/guards/resources" },
      { method: "POST", path: "/api/v1/guards/resources", body: { name: "redis-cache", kind: "redis" } },
      { method: "GET", path: "/api/v1/guards/inspection-templates" },
      { method: "POST", path: "/api/v1/guards/inspection-templates", body: { name: "pg-latency", checks: ["replication_lag"] } },
      { method: "GET", path: "/api/v1/guards/problem-types" },
      { method: "POST", path: "/api/v1/guards/problem-types", body: { name: "replication_lag", severity: "warning" } },
      { method: "GET", path: "/api/v1/guards/actions" },
      { method: "POST", path: "/api/v1/guards/actions", body: { name: "restart_replica", kind: "workflow" } },
      { method: "GET", path: "/api/v1/guards/workflow-bindings" },
      {
        method: "POST",
        path: "/api/v1/guards/workflow-bindings",
        body: { name: "restart-replica-binding", workflowName: "restart-replica" },
      },
      { method: "GET", path: "/api/v1/guards/rules" },
      { method: "POST", path: "/api/v1/guards/rules", body: { name: "pg-replication-autoheal", enabled: false } },
    ]);
  });

  it("targets rule state and GuardRun decision endpoints with encoded ids", async () => {
    const http = createRecordingHttpClient({ item: { id: "ok" } });
    const api = createOperatorRuntimeClient(http);

    await api.enableRule("rule/a 1");
    await api.disableRule("rule/a 1");
    await api.listRuns();
    await api.getRun("run/a 1");
    await api.approveRun("run/a 1");
    await api.rejectRun("run/a 1");

    expect(http.calls).toEqual([
      { method: "POST", path: "/api/v1/guards/rules/rule%2Fa%201/enable", body: undefined },
      { method: "POST", path: "/api/v1/guards/rules/rule%2Fa%201/disable", body: undefined },
      { method: "GET", path: "/api/v1/guards/runs" },
      { method: "GET", path: "/api/v1/guards/runs/run%2Fa%201" },
      { method: "POST", path: "/api/v1/guards/runs/run%2Fa%201/approve", body: undefined },
      { method: "POST", path: "/api/v1/guards/runs/run%2Fa%201/reject", body: undefined },
    ]);
  });

  it("normalizes backend validation errors into field errors", async () => {
    const http = {
      get: vi.fn(),
      post: vi.fn(() =>
        Promise.reject({
          status: 400,
          payload: {
            error: "invalid guard rule",
            fieldErrors: [{ field: "workflowBindingRefs", message: "至少需要一个 Workflow 绑定" }],
          },
        }),
      ),
    };
    const api = createOperatorRuntimeClient(http);

    await expect(api.createRule({ name: "pg-rule" })).rejects.toMatchObject({
      name: "OperatorRuntimeError",
      message: "invalid guard rule",
      status: 400,
      fieldErrors: [{ field: "workflowBindingRefs", message: "至少需要一个 Workflow 绑定" }],
    } satisfies Partial<OperatorRuntimeError>);
  });

  it("polls GuardRun detail until the run reaches a terminal state", async () => {
    const responses = [{ item: { id: "run-1", status: "running" } }, { item: { id: "run-1", status: "succeeded" } }];
    const http = {
      get: vi.fn(() => Promise.resolve(responses.shift())),
      post: vi.fn(),
    };
    const api = createOperatorRuntimeClient(http);

    const result = await api.pollRun("run-1", { intervalMs: 0, maxAttempts: 2 });

    expect(result.item).toMatchObject({ id: "run-1", status: "succeeded" });
    expect(http.get).toHaveBeenCalledTimes(2);
    expect(http.get).toHaveBeenNthCalledWith(1, "/api/v1/guards/runs/run-1");
    expect(http.get).toHaveBeenNthCalledWith(2, "/api/v1/guards/runs/run-1");
  });
});
