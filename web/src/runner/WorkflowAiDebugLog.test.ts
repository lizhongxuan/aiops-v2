import { describe, expect, it } from "vitest";
import { serializeWorkflowAiDebugLog } from "./WorkflowAiDebugLog";

describe("WorkflowAiDebugLog", () => {
  it("serializes a copyable workflow ai edit log", () => {
    const text = serializeWorkflowAiDebugLog({
      session: { id: "drawer", workflowId: "workflow", baseRevision: "rev-1", activeRevision: "rev-2" },
      context: { workflowId: "workflow", selectedNodeId: "collect" },
      userMessages: ["添加验证步骤"],
      plans: [{ id: "plan", items: [{ id: "item", title: "verify" }] }],
      patches: [{ id: "patch", operations: [{ op: "update_node" }] }],
      validations: [{ valid: true }],
      results: [{ patchId: "patch", effect: { status: "changed" }, undoCheckpoint: { id: "undo" } }],
      toolLog: [{ id: "tool", toolName: "workflow.apply_patch", status: "completed", durationMs: 12, traceId: "trace-1" }],
      errors: ["none", "apikey=secret-value password:secret-pass Bearer abc.def"],
    });

    expect(text).toContain("session_id: drawer");
    expect(text).toContain("workflow_id: workflow");
    expect(text).toContain("plan_ids: plan");
    expect(text).toContain("patch_ids: patch");
    expect(text).toContain("effect_status: changed");
    expect(text).toContain("undo_checkpoint_ids: undo");
    expect(text).toContain("tool_names: workflow.apply_patch");
    expect(text).toContain("trace_ids: trace-1");
    expect(text).toContain("apikey=[redacted]");
    expect(text).not.toContain("secret-value");
    expect(text).not.toContain("secret-pass");
    expect(text).not.toContain("abc.def");
  });
});
