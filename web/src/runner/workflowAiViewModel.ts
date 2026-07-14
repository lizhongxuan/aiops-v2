import type {
  WorkflowAiContext,
  WorkflowAiEffectStatus,
  WorkflowAiPlanReply,
  WorkflowAiSession,
  WorkflowAiStage,
  WorkflowAiToolLogEntry,
  WorkflowEditPlan,
  WorkflowPatch,
  WorkflowPatchResult,
  WorkflowPatchValidation,
} from "./workflowAiTypes";

export type WorkflowAiState = {
  stage: WorkflowAiStage;
  context?: WorkflowAiContext;
  session?: WorkflowAiSession;
  userMessages: string[];
  plan?: WorkflowEditPlan;
  currentPatch?: WorkflowPatch;
  validation?: WorkflowPatchValidation;
  lastResult?: WorkflowPatchResult;
  effectStatus?: WorkflowAiEffectStatus;
  readonlyAnswer?: string;
  planReplyInstruction?: string;
  error?: string;
  conflict?: { reason: string; staleRevision?: string; activeRevision?: string };
  toolLog: WorkflowAiToolLogEntry[];
};

export type WorkflowAiEvent =
  | { type: "drawer_opened"; context: WorkflowAiContext; session?: WorkflowAiSession }
  | { type: "submit"; message: string }
  | { type: "readonly_answer_ready"; answer: string }
  | { type: "plan_ready"; plan: WorkflowEditPlan }
  | { type: "plan_reply"; reply: WorkflowAiPlanReply; message: string }
  | { type: "plan_cancelled"; message?: string }
  | { type: "start_plan_item"; itemId: string }
  | { type: "patch_ready"; patch: WorkflowPatch; validation?: WorkflowPatchValidation }
  | { type: "apply_success"; result: WorkflowPatchResult; session?: WorkflowAiSession }
  | { type: "post_apply_checked"; effectStatus: WorkflowAiEffectStatus; session?: WorkflowAiSession }
  | { type: "continue_batch" }
  | { type: "stale_revision"; staleRevision?: string; activeRevision?: string; error?: string }
  | { type: "undo_success"; context: WorkflowAiContext; session?: WorkflowAiSession }
  | { type: "undo_conflict"; error: string }
  | { type: "tool_log"; entry: WorkflowAiToolLogEntry }
  | { type: "error"; error: string };

export function createWorkflowAiInitialState(): WorkflowAiState {
  return {
    stage: "idle",
    userMessages: [],
    toolLog: [],
  };
}

export function parseWorkflowAiPlanReply(message: string): WorkflowAiPlanReply {
  const text = String(message || "").trim();
  const normalized = text.toLowerCase();
  if (!text) return { type: "revise_plan", instruction: "" };
  if (/^(确认|可以|同意|开始|开始生成|按计划执行|继续|执行|好的|好|ok|yes|approve)/i.test(text) || normalized === "ok") {
    return { type: "approve_plan" };
  }
  if (/^(取消|停止|先不要|不要改|别改|算了|退出|cancel|stop)/i.test(text)) {
    return { type: "cancel_plan" };
  }
  return { type: "revise_plan", instruction: text };
}

export function workflowAiReducer(state: WorkflowAiState, event: WorkflowAiEvent): WorkflowAiState {
  switch (event.type) {
    case "drawer_opened":
      return {
        ...state,
        stage: "context_loaded",
        context: event.context,
        session: event.session ?? state.session,
        error: undefined,
        conflict: undefined,
      };
    case "submit":
      return {
        ...state,
        stage: "planning",
        userMessages: [...state.userMessages, event.message],
        readonlyAnswer: undefined,
        planReplyInstruction: undefined,
        error: undefined,
      };
    case "readonly_answer_ready":
      return {
        ...state,
        stage: "complete",
        readonlyAnswer: event.answer,
        plan: undefined,
        currentPatch: undefined,
        error: undefined,
      };
    case "plan_ready":
      return {
        ...state,
        stage: "plan_review",
        plan: event.plan,
        planReplyInstruction: undefined,
        error: undefined,
      };
    case "plan_reply":
      if (event.reply.type === "approve_plan") {
        return {
          ...state,
          stage: "patch_generating",
          userMessages: [...state.userMessages, event.message],
          currentPatch: undefined,
          error: undefined,
        };
      }
      if (event.reply.type === "cancel_plan") {
        return {
          ...state,
          stage: "complete",
          userMessages: [...state.userMessages, event.message],
          currentPatch: undefined,
          error: undefined,
        };
      }
      return {
        ...state,
        stage: "planning",
        userMessages: [...state.userMessages, event.message],
        currentPatch: undefined,
        planReplyInstruction: event.reply.instruction,
        error: undefined,
      };
    case "plan_cancelled":
      return {
        ...state,
        stage: "complete",
        currentPatch: undefined,
        error: undefined,
      };
    case "start_plan_item":
      return {
        ...state,
        stage: "patch_generating",
        plan: markPlanItem(state.plan, event.itemId, "in_progress"),
      };
    case "patch_ready":
      return {
        ...state,
        stage: "patch_review",
        currentPatch: event.patch,
        validation: event.validation,
        error: undefined,
      };
    case "apply_success":
      return {
        ...state,
        stage: "post_apply_check",
        lastResult: event.result,
        session: event.session ?? state.session,
        effectStatus: event.result.effect?.status,
        error: undefined,
      };
    case "post_apply_checked": {
      const nextSession = event.session ?? state.session;
      if (budgetIsPaused(nextSession)) {
        return { ...state, stage: "budget_paused", session: nextSession, effectStatus: event.effectStatus };
      }
      if (event.effectStatus === "changed") {
        return { ...state, stage: "patch_generating", session: nextSession, effectStatus: event.effectStatus };
      }
      return { ...state, stage: "patch_review", session: nextSession, effectStatus: event.effectStatus };
    }
    case "continue_batch":
      return {
        ...state,
        stage: "patch_generating",
        session: resetSessionBudget(state.session),
        error: undefined,
      };
    case "stale_revision":
      return {
        ...state,
        stage: "conflict",
        conflict: {
          reason: event.error ?? "stale_revision",
          staleRevision: event.staleRevision,
          activeRevision: event.activeRevision,
        },
        error: event.error,
      };
    case "undo_success":
      return {
        ...state,
        stage: "context_loaded",
        context: event.context,
        session: event.session ?? state.session,
        lastResult: undefined,
        error: undefined,
        conflict: undefined,
      };
    case "undo_conflict":
      return {
        ...state,
        stage: "conflict",
        conflict: { reason: event.error },
        error: event.error,
      };
    case "tool_log":
      return {
        ...state,
        toolLog: [...state.toolLog, event.entry],
      };
    case "error":
      return {
        ...state,
        error: event.error,
      };
    default:
      return state;
  }
}

function markPlanItem(plan: WorkflowEditPlan | undefined, itemId: string, status: string) {
  if (!plan) return plan;
  return {
    ...plan,
    items: plan.items.map((item) => (item.id === itemId ? { ...item, status } : item)),
  };
}

function budgetIsPaused(session?: WorkflowAiSession) {
  const budget = session?.stepBudget;
  if (!budget) return false;
  if (session?.status === "budget_paused") return true;
  const max = Number(budget.maxPatchReviewsPerTurn || 0);
  return max > 0 && Number(budget.usedPatchReviews || 0) >= max;
}

function resetSessionBudget(session?: WorkflowAiSession): WorkflowAiSession | undefined {
  if (!session) return session;
  return {
    ...session,
    status: "active",
    stepBudget: {
      ...session.stepBudget,
      usedPatchReviews: 0,
    },
  };
}
