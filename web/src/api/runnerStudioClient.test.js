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
    await client.createRunnerStudioWorkflowAiSession({ workflow_id: "workflow" });
    await client.getRunnerStudioWorkflowAiSnapshot({ workflow_id: "workflow" });
    await client.proposeRunnerStudioWorkflowAiPlan({ workflow_id: "workflow", message: "添加验证步骤" });
    await client.proposeRunnerStudioWorkflowAiPatch({ plan_id: "plan", item_id: "item" });
    await client.validateRunnerStudioWorkflowAiPatch({ patch_id: "patch" });
    await client.previewRunnerStudioWorkflowAiPatch({ patch_id: "patch" });
    await client.describeRunnerStudioWorkflowAiPatch({ patch_id: "patch" });
    await client.detectRunnerStudioWorkflowAiPatchEffect({ patch_id: "patch" });
    await client.applyRunnerStudioWorkflowAiPatch({
      patch_id: "patch",
      user_confirmation_id: "confirm",
      drawer_session_id: "drawer",
    });
    await client.undoRunnerStudioWorkflowAiPatch({ undo_checkpoint_id: "undo", drawer_session_id: "drawer" });
    await client.proposeRunnerStudioWorkflowManualCandidate({ workflow_id: "workflow" });
    await client.proposeRunnerStudioWorkflowManualUpdate({ workflow_id: "workflow", manual_id: "manual" });
    await client.createRunnerStudioWorkflowAiDraftFromPlan({ user_confirmation_id: "confirm", plan: { title: "workflow" } });

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
        path: "/api/runner-studio/workflow-ai/sessions",
        body: { workflow_id: "workflow" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/snapshot",
        body: { workflow_id: "workflow" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/plan",
        body: { workflow_id: "workflow", message: "添加验证步骤" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/patch",
        body: { plan_id: "plan", item_id: "item" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/validate",
        body: { patch_id: "patch" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/preview",
        body: { patch_id: "patch" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/describe",
        body: { patch_id: "patch" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/effect",
        body: { patch_id: "patch" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/apply",
        body: { patch_id: "patch", user_confirmation_id: "confirm", drawer_session_id: "drawer" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/undo",
        body: { undo_checkpoint_id: "undo", drawer_session_id: "drawer" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/manual-candidate",
        body: { workflow_id: "workflow" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/manual-update",
        body: { workflow_id: "workflow", manual_id: "manual" },
      },
      {
        method: "POST",
        path: "/api/runner-studio/workflow-ai/create-draft",
        body: { user_confirmation_id: "confirm", plan: { title: "workflow" } },
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

  it("does not embed the legacy Runner UI address in the Runner Studio client source", () => {
    const sourcePath = join(process.cwd(), "src/api/runnerStudioClient.js");
    const source = readFileSync(sourcePath, "utf8");

    expect(source).not.toMatch(legacyExternalPattern);
  });
});
