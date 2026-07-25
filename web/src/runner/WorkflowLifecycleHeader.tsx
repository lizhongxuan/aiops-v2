import { useMemo } from "react";

import {
  WorkflowCommandBar,
} from "./WorkflowCommandBar";
import type { WorkflowActionMenuItem } from "./WorkflowActionMenu";
import {
  selectApplyOutcome,
  selectLifecycleState,
  selectWorkflowLifecycleTraceView,
  selectWorkflowPrimaryActionViewModel,
  selectWorkflowRunView,
  type WorkflowRunGateFacts,
  type WorkflowRunGateState,
} from "./workspace/workflowWorkspaceSelectors";
import type {
  WorkflowPrimaryActionViewModel,
  WorkflowSavePresentation,
} from "./workspace/workflowInteractionContract";
import type { WorkflowWorkspaceState } from "./workspace/workflowWorkspaceTypes";

export type { WorkflowSavePresentation } from "./workspace/workflowInteractionContract";
export type WorkflowLifecycleAction = WorkflowPrimaryActionViewModel;

export type WorkflowLifecycleHeaderView = {
  action?: WorkflowLifecycleAction;
  save: { state: WorkflowSavePresentation; label: string; description: string };
  validation: { state: WorkflowRunGateState; label: string };
  candidateCount: number;
  publishedRevision?: string;
  lifecycleLabel: string;
  trace: ReturnType<typeof selectWorkflowLifecycleTraceView>;
};

export type WorkflowLifecycleHeaderProps = {
  state: WorkflowWorkspaceState;
  facts?: WorkflowRunGateFacts;
  title?: string;
  environment?: string;
  saveState?: WorkflowSavePresentation;
  placement?: "workspace" | "topbar";
  canHandleAction?: (action: WorkflowLifecycleAction) => boolean;
  onAction?: (action: WorkflowLifecycleAction) => void;
  onBack?: () => void;
  onUndo?: () => void;
  onRedo?: () => void;
  onOpenWorkbench?: () => void;
  menuItems?: readonly WorkflowActionMenuItem[];
};

export function deriveWorkflowLifecycleHeaderView(
  state: WorkflowWorkspaceState,
  facts?: WorkflowRunGateFacts,
  saveState?: WorkflowSavePresentation,
): WorkflowLifecycleHeaderView {
  const run = selectWorkflowRunView(state, facts);
  const lifecycle = selectLifecycleState(state);
  const save = savePresentation(state, saveState);
  const action = selectWorkflowPrimaryActionViewModel(state, facts) ?? undefined;
  return {
    action,
    save,
    validation: { state: run.gateState, label: VALIDATION_LABEL[run.gateState] },
    candidateCount: state.candidate?.changeSet.operations.length ?? 0,
    publishedRevision: run.publishedRun.target?.resourceVersion,
    lifecycleLabel: LIFECYCLE_LABEL[lifecycle],
    trace: selectWorkflowLifecycleTraceView(state, facts, save.state),
  };
}

/**
 * Compatibility wrapper while RunnerStudioPage migrates from the old toolbar.
 * It consumes the shared selector and delegates all visible composition to the
 * Command Bar; it does not derive another action or target.
 */
export function WorkflowLifecycleHeader({
  state,
  facts,
  title = "工作流",
  saveState,
  placement = "workspace",
  canHandleAction,
  onAction,
  onBack,
  onUndo,
  onRedo,
  onOpenWorkbench,
  menuItems,
}: WorkflowLifecycleHeaderProps) {
  const view = useMemo(
    () => deriveWorkflowLifecycleHeaderView(state, facts, saveState),
    [facts, saveState, state],
  );
  const commandBarAction = view.action?.owner === "command_bar" &&
    onAction &&
    (canHandleAction ? canHandleAction(view.action) : true)
    ? view.action
    : undefined;
  const candidateOwnsPrimary = view.action?.owner === "candidate_workspace_action_bar";
  const blockedReason = view.action?.owner === "command_bar" && !commandBarAction
    ? view.action.reason || "当前操作暂不可用"
    : undefined;

  const content = (
    <>
      <WorkflowCommandBar
        title={title}
        lifecycleLabel={view.lifecycleLabel}
        trace={view.trace}
        primaryAction={commandBarAction}
        blockedReason={blockedReason}
        onPrimaryAction={onAction}
        onBack={onBack}
        onUndo={onUndo}
        onRedo={onRedo}
        onOpenWorkbench={onOpenWorkbench}
        menuItems={menuItems}
        showIdentity
      />
      {candidateOwnsPrimary ? <span className="sr-only">主操作位于审阅栏</span> : null}
    </>
  );

  if (placement === "topbar") {
    return (
      <div
        data-testid="workflow-lifecycle-header"
        data-placement="topbar"
        data-candidate-primary-owner={candidateOwnsPrimary ? "candidate_workspace_action_bar" : "none"}
      >
        {content}
      </div>
    );
  }

  return (
    <header
      data-testid="workflow-lifecycle-header"
      data-placement="workspace"
      data-candidate-primary-owner={candidateOwnsPrimary ? "candidate_workspace_action_bar" : "none"}
    >
      {content}
    </header>
  );
}

function savePresentation(
  state: WorkflowWorkspaceState,
  override?: WorkflowSavePresentation,
) {
  const derived = override ||
    (state.task.state === "applying" || selectApplyOutcome(state) === "pending_event"
      ? "applying"
      : Object.values(state.validationArtifacts).some((artifact) =>
        artifact.status === "saved_with_issues")
        ? "saved_with_issues"
        : "auto_saved");
  return {
    state: derived,
    ...({
      auto_saved: { label: "已自动保存", description: "当前已保存版本来自服务端确认" },
      unsaved: { label: "未保存", description: "修改尚未收到服务端确认" },
      applying: { label: "正在应用", description: "等待事务回执和版本事件" },
      saved_with_issues: {
        label: "已保存但有问题",
        description: "保存已完成；检查或环境验证仍有阻断问题",
      },
    } as const)[derived],
  };
}

const VALIDATION_LABEL: Record<WorkflowRunGateState, string> = {
  unavailable: "检查依据未同步",
  unchecked: "待检查",
  checking: "检查中",
  validated: "静态检查通过",
  current: "检查与试运行通过",
  stale: "检查已过期",
  configuration_required: "还需配置",
  saved_with_issues: "已保存但有问题",
};

const LIFECYCLE_LABEL = {
  draft: "草稿",
  published: "已发布",
  published_with_draft_changes: "有未发布更改",
} as const;
