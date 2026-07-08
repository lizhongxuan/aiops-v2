import { describe, expect, it } from "vitest";
import { createWorkflowAiInitialState, parseWorkflowAiPlanReply, workflowAiReducer } from "./workflowAiViewModel";

const context = { workflowId: "workflow", workflowName: "Redis", revision: "rev-1", selectedNodeId: "collect" };
const session = {
  id: "drawer",
  workflowId: "workflow",
  baseRevision: "rev-1",
  activeRevision: "rev-1",
  status: "active",
  stepBudget: { maxPatchReviewsPerTurn: 3, usedPatchReviews: 0 },
};
const plan = { id: "plan", items: [{ id: "item", title: "Add verify", status: "pending" }] };
const patch = { id: "patch", baseRevision: "rev-1", operations: [{ op: "update_node", nodeId: "collect" }] };

describe("workflowAiViewModel", () => {
  it("parses conversational plan replies without button actions", () => {
    expect(parseWorkflowAiPlanReply("确认")).toEqual({ type: "approve_plan" });
    expect(parseWorkflowAiPlanReply("可以，开始生成")).toEqual({ type: "approve_plan" });
    expect(parseWorkflowAiPlanReply("取消")).toEqual({ type: "cancel_plan" });
    expect(parseWorkflowAiPlanReply("先不要改了")).toEqual({ type: "cancel_plan" });
    expect(parseWorkflowAiPlanReply("把第 2 步改成先检查磁盘空间")).toEqual({
      type: "revise_plan",
      instruction: "把第 2 步改成先检查磁盘空间",
    });
  });

  it("moves through context, plan, patch and apply states one step at a time", () => {
    let state = createWorkflowAiInitialState();
    state = workflowAiReducer(state, { type: "drawer_opened", context, session });
    expect(state.stage).toBe("context_loaded");

    state = workflowAiReducer(state, { type: "submit", message: "添加验证步骤" });
    expect(state.stage).toBe("planning");

    state = workflowAiReducer(state, { type: "plan_ready", plan });
    expect(state.stage).toBe("plan_review");

    state = workflowAiReducer(state, { type: "start_plan_item", itemId: "item" });
    expect(state.stage).toBe("patch_generating");
    expect(state.plan?.items[0].status).toBe("in_progress");

    state = workflowAiReducer(state, { type: "patch_ready", patch });
    expect(state.stage).toBe("patch_review");

    state = workflowAiReducer(state, {
      type: "apply_success",
      result: { patchId: "patch", effect: { status: "changed" } },
      session,
    });
    expect(state.stage).toBe("post_apply_check");

    state = workflowAiReducer(state, { type: "post_apply_checked", effectStatus: "changed", session });
    expect(state.stage).toBe("patch_generating");
  });

  it("keeps non-effect statuses in patch review", () => {
    for (const effectStatus of ["no_effect", "duplicate", "metadata_only"] as const) {
      const state = workflowAiReducer(
        { ...createWorkflowAiInitialState(), stage: "post_apply_check", session },
        { type: "post_apply_checked", effectStatus, session },
      );
      expect(state.stage).toBe("patch_review");
    }
  });

  it("pauses when patch review budget is reached and resumes on continue", () => {
    const pausedSession = { ...session, status: "budget_paused", stepBudget: { maxPatchReviewsPerTurn: 3, usedPatchReviews: 3 } };
    let state = workflowAiReducer(
      { ...createWorkflowAiInitialState(), stage: "post_apply_check", session },
      { type: "post_apply_checked", effectStatus: "changed", session: pausedSession },
    );
    expect(state.stage).toBe("budget_paused");

    state = workflowAiReducer(state, { type: "continue_batch" });
    expect(state.stage).toBe("patch_generating");
    expect(state.session?.stepBudget?.usedPatchReviews).toBe(0);
  });

  it("moves patch review to conflict on stale revision", () => {
    const state = workflowAiReducer(
      { ...createWorkflowAiInitialState(), stage: "patch_review" },
      { type: "stale_revision", staleRevision: "rev-1", activeRevision: "rev-2", error: "stale_revision" },
    );
    expect(state.stage).toBe("conflict");
    expect(state.conflict?.activeRevision).toBe("rev-2");
  });

  it("undo success reloads context and undo conflict shows conflict", () => {
    let state = workflowAiReducer(
      { ...createWorkflowAiInitialState(), stage: "post_apply_check", session },
      { type: "undo_success", context: { ...context, revision: "rev-0" }, session: { ...session, activeRevision: "rev-0" } },
    );
    expect(state.stage).toBe("context_loaded");
    expect(state.context?.revision).toBe("rev-0");

    state = workflowAiReducer(state, { type: "undo_conflict", error: "manual interleaving" });
    expect(state.stage).toBe("conflict");
    expect(state.error).toContain("manual interleaving");
  });
});
