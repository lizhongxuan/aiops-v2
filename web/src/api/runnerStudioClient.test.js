import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { createRunnerStudioClient } from "./runnerStudioClient";

function createRecordingHttpClient() {
  const calls = [];
  const makeMethod = (method) =>
    vi.fn((path, body) => {
      calls.push({ method, path, body });
      return Promise.resolve({ ok: true });
    });

  return {
    calls,
    get: makeMethod("GET"),
    post: makeMethod("POST"),
    put: makeMethod("PUT"),
    delete: makeMethod("DELETE"),
  };
}

describe("runnerStudioClient", () => {
  const legacyExternalPattern = new RegExp(`${["", "8090"].join(":")}|VITE_RUNNER_UI_URL|https?:\\/\\/`);

  it("routes every Runner Studio method through same-origin /api/runner-studio paths", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const graphPayload = { graph: { workflow: { name: "demo" } } };

    await client.listRunnerStudioWorkflows({ labels: "env:prod" });
    await client.getRunnerStudioWorkflowGraph("demo workflow");
    await client.listRunnerStudioWorkflowVersions("demo workflow");
    await client.getRunnerStudioWorkflowVersion("demo workflow", "v/1");
    await client.rollbackRunnerStudioWorkflowVersion("demo workflow", "v/1", { save_note: "restore" });
    await client.exportRunnerStudioWorkflowBundle("demo workflow");
    await client.importRunnerStudioWorkflowBundle({ bundle: { name: "demo workflow", yaml: "version: '1'" } });
    await client.validateRunnerStudioWorkflow("demo workflow");
    await client.publishRunnerStudioWorkflow("demo workflow", { save_note: "approved" });
    await client.createRunnerStudioWorkflowGraph(graphPayload);
    await client.compileRunnerStudioWorkflowGraph(graphPayload);
    await client.parseRunnerStudioWorkflowYaml({ yaml: "version: v0.1" });
    await client.updateRunnerStudioWorkflowGraph("demo/workflow", graphPayload);
    await client.validateRunnerStudioWorkflowGraph(graphPayload);
    await client.resolveRunnerStudioWorkflowVariables({ ...graphPayload, node_id: "restore" });
    await client.dryRunRunnerStudioWorkflowGraph({ ...graphPayload, vars: { deploy: false } });
    await client.runRunnerStudioWorkflowGraph({ ...graphPayload, idempotency_key: "run-1" });
    await client.getRunnerStudioRunGraph("run/id 1");
    await client.getRunnerStudioRunEventHistory("run/id 1");
    await client.cancelRunnerStudioRun("run/id 1");
    await client.getRunnerStudioActionCatalog({ category: "command", experimental: false });
    const command = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow",
      expectedRevision: "r17",
      idempotencyKey: "manual-1",
      reason: "add a step",
      command: { kind: "add_node", node: { id: "check", type: "action", position: { x: 20, y: 40 } } },
    };
    await client.executeRunnerStudioWorkflowCommand(command);
    await client.moveRunnerStudioWorkflowNodes({
      ...command,
      idempotencyKey: "drag-stop-1",
      command: { kind: "move", layout: { nodes: [{ nodeId: "check", position: { x: 30, y: 50 } }] } },
    });
    const currentBasis = {
      resourceVersion: "r17", semanticHash: "semantic", catalogVersion: "catalog",
      actionSchemaDigest: "actions", referenceSnapshotHash: "references", environmentBindingVersion: "environment",
    };
    await client.dryRunAuthoritativeWorkflow({
      schemaVersion: "aiops.workflow.edit.v2", workflowId: "workflow", expectedBasis: currentBasis, environment: "staging",
    });
    const applyApproval = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow",
      candidateId: `history-${"a".repeat(64)}`,
      candidateRevision: 1,
      idempotencyKey: "history-apply-approval-1",
      action: "request_apply_approval",
    };
    await client.requestRunnerStudioWorkflowCandidateApplyApproval(applyApproval);

    expect(http.calls).toEqual([
      { method: "GET", path: "/api/runner-studio/workflows?labels=env%3Aprod", body: undefined },
      { method: "GET", path: "/api/runner-studio/workflows/demo%20workflow/graph", body: undefined },
      { method: "GET", path: "/api/runner-studio/workflows/demo%20workflow/versions", body: undefined },
      { method: "GET", path: "/api/runner-studio/workflows/demo%20workflow/versions/v%2F1", body: undefined },
      {
        method: "POST",
        path: "/api/runner-studio/workflows/demo%20workflow/versions/v%2F1/rollback",
        body: { save_note: "restore" },
      },
      { method: "GET", path: "/api/runner-studio/workflows/demo%20workflow/bundle", body: undefined },
      {
        method: "POST",
        path: "/api/runner-studio/workflows/bundles/import",
        body: { bundle: { name: "demo workflow", yaml: "version: '1'" } },
      },
      { method: "POST", path: "/api/runner-studio/workflows/demo%20workflow/validate", body: undefined },
      {
        method: "POST",
        path: "/api/runner-studio/workflows/demo%20workflow/publish",
        body: { save_note: "approved" },
      },
      { method: "POST", path: "/api/runner-studio/workflows/graph", body: graphPayload },
      { method: "POST", path: "/api/runner-studio/workflows/graph/compile", body: graphPayload },
      { method: "POST", path: "/api/runner-studio/workflows/graph/parse", body: { yaml: "version: v0.1" } },
      { method: "PUT", path: "/api/runner-studio/workflows/demo%2Fworkflow/graph", body: graphPayload },
      { method: "POST", path: "/api/runner-studio/workflows/graph/validate", body: graphPayload },
      {
        method: "POST",
        path: "/api/runner-studio/workflows/graph/variables/resolve",
        body: { ...graphPayload, node_id: "restore" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflows/graph/dry-run",
        body: { ...graphPayload, vars: { deploy: false } },
      },
      {
        method: "POST",
        path: "/api/runner-studio/runs",
        body: { ...graphPayload, idempotency_key: "run-1" },
      },
      { method: "GET", path: "/api/runner-studio/runs/run%2Fid%201/graph", body: undefined },
      { method: "GET", path: "/api/runner-studio/runs/run%2Fid%201/events/history", body: undefined },
      { method: "POST", path: "/api/runner-studio/runs/run%2Fid%201/cancel", body: undefined },
      {
        method: "GET",
        path: "/api/runner-studio/actions?category=command&experimental=false",
        body: undefined,
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-commands",
        body: command,
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-commands",
        body: {
          ...command,
          idempotencyKey: "drag-stop-1",
          command: { kind: "move", layout: { nodes: [{ nodeId: "check", position: { x: 30, y: 50 } }] } },
        },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-dry-run",
        body: {
          schemaVersion: "aiops.workflow.edit.v2", workflowId: "workflow", expectedBasis: currentBasis, environment: "staging",
        },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-candidate-apply-approval",
        body: applyApproval,
      },
    ]);

    expect(http.calls.map((call) => call.path)).toSatisfy((paths) =>
      paths.every((path) => path.startsWith("/api/runner-studio/")),
    );
    expect(http.calls.map((call) => call.path).join("\n")).not.toMatch(legacyExternalPattern);
    const serializedCalls = JSON.stringify(http.calls);
    expect(serializedCalls).not.toContain("api.z.ai");
    expect(serializedCalls).not.toContain("glm-5.1");
    expect(serializedCalls).not.toContain("apikey");
  });

  it("fails closed before sending an incomplete workflow mutation", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);

    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow",
      reason: "remove obsolete node",
      command: { kind: "delete_node", nodeId: "n1" },
    };
    expect(() => client.executeRunnerStudioWorkflowCommand(base))
      .toThrow("expectedRevision");
    expect(() => client.executeRunnerStudioWorkflowCommand({
      ...base,
      expectedRevision: "r17",
    })).toThrow("idempotencyKey");
    expect(() => client.moveRunnerStudioWorkflowNodes({
      ...base,
      expectedRevision: "r17",
      idempotencyKey: "move-1",
      command: { kind: "update_node", nodeId: "n1", nodePatch: { label: "changed" } },
    })).toThrow("move layout");
    expect(() => client.executeRunnerStudioWorkflowCommand({
      ...base,
      expectedRevision: "r17",
      idempotencyKey: "forged-1",
      actorId: "forged-user",
    })).toThrow("actorId");
    expect(() => client.executeRunnerStudioWorkflowCommand({
      ...base,
      expectedRevision: "r17",
      idempotencyKey: "unknown-1",
      command: { kind: "replace_full_graph", graph: {} },
    })).toThrow("kind");
    expect(() => client.dryRunAuthoritativeWorkflow({
      schemaVersion: "aiops.workflow.edit.v2", workflowId: "workflow", expectedBasis: { resourceVersion: "r17" }, environment: "staging",
    })).toThrow("full validation basis");
    expect(() => client.dryRunAuthoritativeWorkflow({
      schemaVersion: "aiops.workflow.edit.v2", workflowId: "workflow", expectedBasis: {
        resourceVersion: "r17", semanticHash: "semantic", catalogVersion: "catalog",
        actionSchemaDigest: "actions", referenceSnapshotHash: "references", environmentBindingVersion: "environment",
      }, environment: "staging", transactionId: "forged",
    })).toThrow("transactionId");
    expect(http.calls).toEqual([]);
  });

  it("posts exact draft and immutable published targets without accepting a graph", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const validationBasis = {
      resourceVersion: "rv-draft",
      semanticHash: "sha256:draft",
      catalogVersion: "catalog-1",
      actionSchemaDigest: "actions-1",
      referenceSnapshotHash: "refs-1",
      environmentBindingVersion: "env-1",
    };
    const draftTarget = {
      kind: "draft_non_production",
      workflowId: "workflow-a",
      resourceVersion: "rv-draft",
      semanticHash: "sha256:draft",
      validationBasis,
      environment: "staging",
    };
    const publishedTarget = {
      kind: "published_revision",
      workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
      resourceVersion: `sha256:${"a".repeat(64)}`,
      semanticHash: `sha256:${"b".repeat(64)}`,
    };

    await client.dryRunAuthoritativeWorkflowTarget(draftTarget);
    await client.runPublishedAuthoritativeWorkflow({
      target: publishedTarget,
      triggeredBy: "workflow-studio",
      idempotencyKey: "published-run-1",
    });

    expect(http.calls).toEqual([
      { method: "POST", path: "/api/runner-studio/workflow-dry-run", body: {
        schemaVersion: "aiops.workflow.edit.v2",
        workflowId: "workflow-a",
        expectedBasis: validationBasis,
        environment: "staging",
      } },
      { method: "POST", path: "/api/runner-studio/workflow-published-run", body: {
        target: {
          workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
          resourceVersion: `sha256:${"a".repeat(64)}`,
          semanticHash: `sha256:${"b".repeat(64)}`,
        },
        triggered_by: "workflow-studio",
        idempotency_key: "published-run-1",
      } },
    ]);
    expect(JSON.stringify(http.calls)).not.toContain('"graph"');

    expect(() => client.dryRunAuthoritativeWorkflowTarget({ ...draftTarget, graph: {} })).toThrow("graph");
    expect(() => client.runPublishedAuthoritativeWorkflow({
      target: { ...publishedTarget, graph: {} },
      triggeredBy: "workflow-studio",
      idempotencyKey: "published-run-2",
    })).toThrow("graph");
    expect(() => client.runPublishedAuthoritativeWorkflow({
      target: { ...publishedTarget, kind: "draft_non_production" },
      triggeredBy: "workflow-studio",
      idempotencyKey: "published-run-3",
    })).toThrow("published_revision");
    expect(http.calls).toHaveLength(2);
  });

  it("accepts exactly one of command or a non-empty ordered commands batch", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow",
      expectedRevision: "r17",
      idempotencyKey: "batch-1",
      reason: "insert node atomically",
    };
    const commands = [
      { kind: "disconnect_edge", edgeId: "check-end" },
      { kind: "add_node", node: { id: "notify", type: "action", position: { x: 400, y: 20 } } },
      { kind: "connect_edge", edge: { id: "check-notify", source: "check", target: "notify" } },
      { kind: "connect_edge", edge: { id: "notify-end", source: "notify", target: "end" } },
    ];

    await client.executeRunnerStudioWorkflowCommand({ ...base, commands });
    expect(http.calls).toEqual([{
      method: "POST",
      path: "/api/runner-studio/workflow-commands",
      body: { ...base, commands },
    }]);

    for (const invalid of [
      base,
      { ...base, commands: [] },
      { ...base, commands: Array.from({ length: 65 }, () => commands[0]) },
      { ...base, command: commands[0], commands },
      { ...base, commands: [{ kind: "replace_full_graph", graph: {} }] },
    ]) expect(() => client.executeRunnerStudioWorkflowCommand(invalid)).toThrow();
    expect(http.calls).toHaveLength(1);
  });

  it("posts a strict Undo/Redo history envelope without graph or trusted scope fields", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      expected: { resourceVersion: "r17", semanticHash: "semantic-17", layoutHash: "layout-17" },
      targetTransactionId: "txn-17",
      idempotencyKey: "history-undo-txn-17-r17",
      action: "undo",
    };

    await client.executeRunnerStudioWorkflowHistoryAction(payload);

    expect(http.calls).toEqual([{
      method: "POST",
      path: "/api/runner-studio/workflow-history-actions",
      body: payload,
    }]);
    expect(JSON.stringify(http.calls)).not.toContain("graph");
    expect(http.calls[0].body).not.toHaveProperty("tenantId");
    expect(http.calls[0].body).not.toHaveProperty("actorId");
    expect(http.calls[0].body).not.toHaveProperty("canvasSessionId");
  });

  it("rejects incomplete or expanded history action payloads before HTTP", () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      expected: { resourceVersion: "r17", semanticHash: "semantic-17", layoutHash: "layout-17" },
      targetTransactionId: "txn-17",
      idempotencyKey: "history-17",
      action: "undo",
    };

    expect(() => client.executeRunnerStudioWorkflowHistoryAction({ ...base, expected: { resourceVersion: "r17" } }))
      .toThrow("full expected revision");
    expect(() => client.executeRunnerStudioWorkflowHistoryAction({ ...base, action: "restore" }))
      .toThrow("undo or redo");
    expect(() => client.executeRunnerStudioWorkflowHistoryAction({ ...base, graph: { nodes: [] } }))
      .toThrow("graph");
    expect(() => client.executeRunnerStudioWorkflowHistoryAction({ ...base, actorId: "forged" }))
      .toThrow("actorId");
    expect(http.calls).toEqual([]);
  });

  it("posts strict candidate parameter, position, and connection revisions without Graph or trusted scope", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-2",
      baseChangeSetId: "changeset-2",
      expected: { resourceVersion: "r17", semanticHash: "semantic-17", layoutHash: "layout-17" },
      idempotencyKey: "revise-2",
      intent: "用户调整候选版本",
    };
    const operations = [
      { op: "update_node", nodeId: "check", set: { step: { args: { script: "uptime" } } } },
      { op: "update_layout", layout: { nodes: [{ nodeId: "check", position: { x: 320, y: 180 } }] } },
      { op: "add_edge", edge: { id: "check-end", source: "check", sourcePort: "next", target: "end", targetPort: "in", kind: "next" } },
    ];

    for (const [index, additionalOperation] of operations.entries()) {
      await client.reviseRunnerStudioWorkflowCandidate({ ...base, idempotencyKey: `revise-${index + 1}`, additionalOperation });
    }

    expect(http.calls).toEqual(operations.map((additionalOperation, index) => ({
      method: "POST",
      path: "/api/runner-studio/workflow-candidate-revise",
      body: { ...base, idempotencyKey: `revise-${index + 1}`, additionalOperation },
    })));
    expect(JSON.stringify(http.calls)).not.toMatch(/"(graph|tenantId|actorId|canvasSessionId)"/);
  });

  it("rejects expanded, partial, or non-canonical candidate revisions before HTTP", () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-2",
      baseChangeSetId: "changeset-2",
      expected: { resourceVersion: "r17", semanticHash: "semantic-17", layoutHash: "layout-17" },
      idempotencyKey: "revise-2",
      intent: "用户调整候选版本",
      additionalOperation: { op: "update_node", nodeId: "check", set: { label: "检查容量" } },
    };

    for (const forged of [
      { ...base, graph: { nodes: [] } },
      { ...base, tenantId: "forged" },
      { ...base, actorId: "forged" },
      { ...base, expected: { resourceVersion: "r17" } },
      { ...base, additionalOperation: { op: "replace_graph", graph: {} } },
      { ...base, additionalOperation: { op: "update_node", nodeId: "check", set: { graph: {} } } },
    ]) expect(() => client.reviseRunnerStudioWorkflowCandidate(forged)).toThrow();
    expect(http.calls).toEqual([]);
  });

  it("posts and validates a fully bound candidate discard action", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-2",
      candidateRevision: 2,
      changeSetId: "changeset-2",
      previewHash: "preview-2",
      expected: { resourceVersion: "r17", semanticHash: "semantic-17", layoutHash: "layout-17" },
      idempotencyKey: "discard-2",
      action: "discard",
    };

    await client.executeRunnerStudioWorkflowCandidateAction(payload);
    expect(http.calls).toEqual([{ method: "POST", path: "/api/runner-studio/workflow-candidate-actions", body: payload }]);
    for (const forged of [
      { ...payload, graph: {} },
      { ...payload, actorId: "forged" },
      { ...payload, candidateRevision: 0 },
      { ...payload, expected: { resourceVersion: "r17" } },
      { ...payload, action: "apply" },
    ]) expect(() => client.executeRunnerStudioWorkflowCandidateAction(forged)).toThrow();
    expect(http.calls).toHaveLength(1);
  });

  it("posts only the persisted History Candidate selector when requesting Apply approval", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: `history-${"b".repeat(64)}`,
      candidateRevision: 1,
      idempotencyKey: "history-apply-once",
      action: "request_apply_approval",
    };

    await client.requestRunnerStudioWorkflowCandidateApplyApproval(payload);

    expect(http.calls).toEqual([{
      method: "POST",
      path: "/api/runner-studio/workflow-candidate-apply-approval",
      body: payload,
    }]);
    expect(JSON.stringify(http.calls)).not.toMatch(/(changeSetId|previewHash|expectedRevision|approvalId|actionToken|tenantId|actorId|canvasSessionId|sessionId)/);
  });

  it("posts only an exact persisted revised Agent Candidate selector when requesting replacement approval", async () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: `candidate-${"d".repeat(24)}`,
      candidateRevision: 3,
      idempotencyKey: "revised-agent-apply-once",
      action: "request_apply_approval",
    };

    await client.requestRunnerStudioWorkflowCandidateApplyApproval(payload);

    expect(http.calls).toEqual([{
      method: "POST",
      path: "/api/runner-studio/workflow-candidate-apply-approval",
      body: payload,
    }]);
    expect(JSON.stringify(http.calls)).not.toMatch(/(changeSetId|previewHash|expectedRevision|approvalId|actionToken|tenantId|actorId|canvasSessionId|sessionId)/);
  });

  it("rejects expanded or non-persisted Candidate approval selectors before HTTP", () => {
    const http = createRecordingHttpClient();
    const client = createRunnerStudioClient(http);
    const base = {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: `history-${"c".repeat(64)}`,
      candidateRevision: 1,
      idempotencyKey: "history-apply-once",
      action: "request_apply_approval",
    };

    for (const forged of [
      { ...base, candidateId: "candidate-browser" },
      { ...base, candidateRevision: 2 },
      { ...base, candidateId: `candidate-${"d".repeat(24)}` },
      { ...base, candidateId: `candidate-${"D".repeat(24)}`, candidateRevision: 2 },
      { ...base, candidateId: `candidate-${"d".repeat(23)}`, candidateRevision: 2 },
      { ...base, candidateId: `candidate-${"d".repeat(24)}`, candidateRevision: Number.MAX_SAFE_INTEGER + 1 },
      { ...base, previewHash: "forged" },
      { ...base, changeSetId: "forged" },
      { ...base, expectedRevision: "forged" },
      { ...base, actionToken: "forged" },
      { ...base, action: "apply" },
    ]) expect(() => client.requestRunnerStudioWorkflowCandidateApplyApproval(forged)).toThrow();
    expect(http.calls).toEqual([]);
  });

  it("propagates workflow command failures without returning local success", async () => {
    const failure = new Error("STALE_REVISION");
    const http = createRecordingHttpClient();
    http.post.mockRejectedValueOnce(failure);
    const client = createRunnerStudioClient(http);

    await expect(client.executeRunnerStudioWorkflowCommand({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow",
      expectedRevision: "r17",
      idempotencyKey: "manual-1",
      reason: "remove obsolete edge",
      command: { kind: "disconnect_edge", edgeId: "edge-1" },
    })).rejects.toBe(failure);
  });

  it("does not embed the legacy Runner UI address in the Runner Studio client source", () => {
    const sourcePath = join(process.cwd(), "src/api/runnerStudioClient.js");
    const source = readFileSync(sourcePath, "utf8");

    expect(source).not.toMatch(legacyExternalPattern);
  });
});
