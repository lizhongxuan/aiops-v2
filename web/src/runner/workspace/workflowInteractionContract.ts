import type { ValidationBasis, WorkflowRevision } from "./workflowMcpContracts";

export type WorkflowTaskState =
  | "idle" | "reading_context" | "awaiting_clarification" | "planning" | "previewing" | "awaiting_review"
  | "applying" | "verifying" | "cancelling" | "cancelled" | "failed" | "settled";
export type WorkflowCandidateState = "none" | "building" | "ready" | "editing" | "stale" | "superseded" | "conflict";
export type WorkflowApprovalState = "none" | "requested" | "approved" | "rejected" | "expired";
export type WorkflowConnectionState = "online" | "reconnecting" | "offline";
export type WorkflowValidationState = "unknown" | "checking" | "valid" | "valid_with_warnings" | "configuration_required" | "saved_with_issues";
export type WorkflowLifecycleState = "draft" | "published" | "published_with_draft_changes";
export type WorkflowSavePresentation = "auto_saved" | "unsaved" | "applying" | "saved_with_issues";

export type WorkflowInteractionAxes = {
  taskState: WorkflowTaskState;
  candidateState: WorkflowCandidateState;
  approvalState: WorkflowApprovalState;
  connectionState: WorkflowConnectionState;
  validationState: WorkflowValidationState;
  lifecycleState: WorkflowLifecycleState;
};

export type WorkflowPrimaryActionOwner =
  | "command_bar"
  | "candidate_workspace_action_bar"
  | "confirmation_dialog";

export type WorkflowStudioActionCapability =
  | "workflow.validate"
  | "workflow.draft_dry_run"
  | "workflow.publish"
  | "workflow.run_published"
  | "workflow.review_issues"
  | "workflow.configure_environment"
  | "workflow.apply_candidate"
  | "workflow.resolve_conflict"
  | "workflow.undo"
  | "workflow.redo";

export type WorkflowSavedDraftTarget = {
  kind: "saved_draft";
  workflowId: string;
  resourceVersion: string;
  semanticHash: string;
};

export type WorkflowDraftDryRunTarget = {
  kind: "draft_non_production";
  workflowId: string;
  resourceVersion: string;
  semanticHash: string;
  validationBasis: ValidationBasis;
  environment: string;
};

export type WorkflowPublishedRevisionTarget = {
  kind: "published_revision";
  workflowKey: {
    tenantId: string;
    workflowId: string;
  };
  resourceVersion: string;
  semanticHash: string;
};

export type WorkflowCandidateActionTarget = {
  kind: "candidate";
  workflowId: string;
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
};

export type WorkflowPanelActionTarget = {
  kind: "workflow_panel";
  workflowId: string;
  panel: "changes" | "run";
};

export type WorkflowEnvironmentConfigurationTarget = {
  kind: "environment_configuration";
  workflowId: string;
  capabilityEntryId: string;
};

export type WorkflowHistoryActionTarget = {
  kind: "history_transaction";
  workflowId: string;
  action: "undo" | "redo";
  targetTransactionId: string;
  expected: WorkflowRevision;
  idempotencyKey: string;
};

export type WorkflowPrimaryActionTarget =
  | WorkflowSavedDraftTarget
  | WorkflowDraftDryRunTarget
  | WorkflowPublishedRevisionTarget
  | WorkflowCandidateActionTarget
  | WorkflowPanelActionTarget
  | WorkflowEnvironmentConfigurationTarget;

export type WorkflowPrimaryActionId =
  | "apply_changes"
  | "resolve_conflict"
  | "check"
  | "fix_issues"
  | "configure"
  | "dry_run"
  | "publish"
  | "publish_changes"
  | "run_published";

export type WorkflowPrimaryActionViewModel = {
  id: WorkflowPrimaryActionId;
  label: string;
  target: WorkflowPrimaryActionTarget;
  owner: WorkflowPrimaryActionOwner;
  enabled: boolean;
  reason: string | undefined;
  capability: WorkflowStudioActionCapability;
};

export type WorkflowHistoryActionViewModel = {
  id: "undo" | "redo";
  label: string;
  target: WorkflowHistoryActionTarget;
  owner: "command_bar";
  enabled: boolean;
  reason: string | undefined;
  capability: "workflow.undo" | "workflow.redo";
};

export type WorkflowStudioActionViewModel =
  | WorkflowPrimaryActionViewModel
  | WorkflowHistoryActionViewModel;

export type WorkflowLifecycleStageId = "save" | "check" | "draft_run" | "publish";
export type WorkflowLifecycleStageState = "complete" | "current" | "pending" | "blocked" | "in_progress";

export type WorkflowLifecycleTraceStageViewModel = {
  id: WorkflowLifecycleStageId;
  label: string;
  state: WorkflowLifecycleStageState;
  description: string;
};

export type WorkflowLifecycleTraceViewModel = {
  stages: WorkflowLifecycleTraceStageViewModel[];
  compactLabel: string;
  ariaLabel: string;
};

const WORKFLOW_LIFECYCLE_ACTION_IDS = new Set<WorkflowPrimaryActionId>([
  "apply_changes",
  "resolve_conflict",
  "check",
  "fix_issues",
  "configure",
  "dry_run",
  "publish",
  "publish_changes",
  "run_published",
]);

export function isWorkflowLifecycleAction(value: string): value is WorkflowPrimaryActionId {
  return WORKFLOW_LIFECYCLE_ACTION_IDS.has(value as WorkflowPrimaryActionId);
}

export const WORKFLOW_INTERACTION_COPY = {
  preparing: "正在准备修改",
  awaitingReview: "待审阅",
  apply: "应用修改",
  saved: "已保存",
  awaitingCheck: "待检查",
  configurationRequired: "还需配置",
  published: "已发布",
  savedWithIssues: "已保存但有问题",
  conflict: "需要解决冲突",
} as const;

export const CONTEXT_DOCK_DESKTOP_TABS = [
  { id: "ai", label: "AI 协作" },
  { id: "changes", label: "变更" },
  { id: "run", label: "运行" },
] as const;

export const CONTEXT_DOCK_MOBILE_MODES = [
  { id: "canvas", label: "画布" },
  { id: "ai", label: "AI" },
  { id: "changes", label: "变更" },
  { id: "run", label: "运行" },
] as const;

export const WORKFLOW_INTERACTION_SEMANTICS = {
  scopeChip: "发送消息时冻结 Workflow、resourceVersion、节点/边选区和环境；后续选区变化不得改写本轮 scope chip。",
  chatToCanvas: "点击 Chat 或 Review 修改项时，聚焦对应 ghost 对象并打开字段 Diff。",
  canvasToChat: "点击画布 ghost 对象时，Chat/Review 定位到同一 change item；对象和数量必须一致。",
  followAi: "跟随 AI 仅在用户主动开启时跟随；用户手动平移、缩放、选择或编辑后立即暂停，后台事件不得抢焦点。",
  candidateEdit: "直接编辑 ghost Candidate 会创建新 candidateRevision 和 previewHash，并使旧批准失效；不改变 committed Graph。",
  conflict: "冲突使用 base、当前已保存版本、我的 Candidate 三方 Diff；保留我的修改后由服务端重新计算。",
  mobileConfiguration: "移动端从画布选区打开 full-screen 配置 sheet，关闭后焦点回到触发对象。",
} as const;

const ACTIVE_CANDIDATES = new Set<WorkflowCandidateState>(["building", "ready", "editing", "stale", "conflict"]);

export type WorkflowInteractionIssue =
  | "approval_requires_candidate"
  | "applying_requires_candidate"
  | "applying_requires_approval"
  | "verifying_requires_committed_view"
  | "conflict_requires_review";

export function validateWorkflowInteractionAxes(axes: WorkflowInteractionAxes): WorkflowInteractionIssue[] {
  const issues: WorkflowInteractionIssue[] = [];
  if (axes.approvalState !== "none" && axes.candidateState !== "ready") issues.push("approval_requires_candidate");
  if (axes.taskState === "applying" && axes.candidateState !== "ready") issues.push("applying_requires_candidate");
  if (axes.taskState === "applying" && axes.approvalState !== "approved") issues.push("applying_requires_approval");
  if (axes.taskState === "verifying" && axes.candidateState !== "none") issues.push("verifying_requires_committed_view");
  if (axes.candidateState === "conflict" && axes.taskState !== "awaiting_review") issues.push("conflict_requires_review");
  return issues;
}

export function deriveWorkflowTruthAnswers(axes: WorkflowInteractionAxes) {
  const hasCandidate = ACTIVE_CANDIDATES.has(axes.candidateState);
  const savedVersion = hasCandidate
    ? "画布包含待应用修改；已保存版本没有改变"
    : axes.taskState === "applying"
      ? "正在保存修改；结果确认前仍以服务端版本为准"
      : "当前画布是已保存版本";
  const checked = {
    unknown: "已保存版本尚未检查",
    checking: "已保存，正在检查",
    valid: "已保存版本已检查，可以运行",
    valid_with_warnings: "已保存版本已检查；有提醒但未阻止运行",
    configuration_required: "结构已检查，还需配置后才能运行",
    saved_with_issues: "已检查；已保存版本仍有问题，不能运行",
  }[axes.validationState];
  let runnable = "检查完成前不能发布或运行";
  if (hasCandidate) runnable = "先应用或放弃待审阅修改，再运行已保存版本";
  else if (axes.connectionState !== "online") runnable = "连接恢复并核对服务端状态后再发布或运行";
  else if (axes.validationState === "valid" || axes.validationState === "valid_with_warnings") runnable = "当前已保存版本可以发布或运行";
  else if (axes.validationState === "configuration_required") runnable = "完成缺失配置并重新检查后才能发布或运行";
  else if (axes.validationState === "saved_with_issues") runnable = "修复已保存版本的问题并重新检查后才能发布或运行";
  return { savedVersion, checked, runnable };
}

const testIdSegment = (value: string) => value.replace(/[^a-zA-Z0-9_-]/g, "-");
export const WORKFLOW_INTERACTION_TEST_IDS = {
  workspace: "workflow-workspace",
  canvas: "workflow-canvas",
  contextDock: "workflow-context-dock",
  nodeSidebar: "workflow-node-sidebar",
  workflowDrawerOpen: "workflow-workbench-trigger",
  workflowDrawerClose: "workflow-context-drawer-close",
  primaryAction: "workflow-primary-action",
  scopeChip: "workflow-scope-chip",
  followAi: "workflow-follow-ai",
  outline: "workflow-structured-outline",
  progressLiveRegion: "workflow-progress-live-region",
  blockingErrorLiveRegion: "workflow-blocking-error-live-region",
  changeItem: (changeId: string) => `workflow-change-item-${testIdSegment(changeId)}`,
  canvasObject: (objectId: string) => `workflow-canvas-object-${testIdSegment(objectId)}`,
} as const;

export const WORKFLOW_KEYBOARD_COMMANDS = [
  { id: "search", shortcut: "Mod+K", ariaLabel: "搜索工作流" },
  { id: "add_node", shortcut: "A", ariaLabel: "添加节点" },
  { id: "delete_selection", shortcut: "Delete", ariaLabel: "删除所选对象" },
  { id: "undo", shortcut: "Mod+Z", ariaLabel: "撤销上一次修改" },
  { id: "redo", shortcut: "Mod+Shift+Z", ariaLabel: "重做上一次修改" },
  { id: "fit", shortcut: "F", ariaLabel: "适应画布内容" },
  { id: "open_ai", shortcut: "Mod+I", ariaLabel: "打开 AI" },
  { id: "open_issues", shortcut: "Mod+Shift+I", ariaLabel: "打开问题列表" },
  { id: "apply_changes", shortcut: "Mod+Enter", ariaLabel: "应用当前待审阅修改" },
] as const;

export const WORKFLOW_ARIA_CONTRACT = {
  progressLiveRegion: { politeness: "polite", examples: ["已生成 4 项待确认修改", "修改已保存，正在检查"] },
  blockingErrorLiveRegion: { politeness: "assertive", examples: ["保存失败，画布未改变"] },
  outline: { readsInternalIds: false, fields: ["业务名称", "类型", "运行状态", "检查状态"] },
  dialogFocus: "用户打开 modal/sheet 时焦点进入；关闭后回到触发点；后台事件不得移动焦点。",
  targetMinimumCssPixels: 44,
  statusUsesColorOnly: false,
} as const;
