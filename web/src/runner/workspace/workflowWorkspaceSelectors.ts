import {
  validateWorkflowInteractionAxes,
  type WorkflowApprovalState,
  type WorkflowCandidateState,
  type WorkflowInteractionAxes,
  type WorkflowLifecycleTraceStageViewModel,
  type WorkflowLifecycleTraceViewModel,
  type WorkflowLifecycleState,
  type WorkflowPrimaryActionViewModel,
  type WorkflowSavePresentation,
  type WorkflowValidationState,
} from "./workflowInteractionContract";
import type {
  ValidationBasis,
  WorkflowDiagnostic,
  WorkflowKey,
} from "./workflowMcpContracts";
import type {
  WorkflowLifecycleArtifactFact,
  WorkflowReceiptFact,
  WorkflowValidationArtifactFact,
  WorkflowWorkspaceState,
} from "./workflowWorkspaceTypes";

export function selectWorkflowWorkspaceAxes(state: WorkflowWorkspaceState, now = new Date()): WorkflowInteractionAxes {
  const candidateState = selectCandidateState(state);
  const latestReceipt = selectLatestReceipt(state);
  const taskState = latestReceipt?.status === "verifying" ? "verifying" : state.task.state;
  return {
    taskState,
    candidateState,
    approvalState: selectApprovalState(state, candidateState, now),
    connectionState: state.gap && state.connection.state === "online" ? "reconnecting" : state.connection.state,
    validationState: selectCommittedValidationState(state),
    lifecycleState: selectLifecycleState(state),
  };
}

export type WorkflowRunConfigurationRequirements = {
  secretRefs?: string[];
  environments?: string[];
};

export type WorkflowRunPostCheckFact = {
  basis: ValidationBasis;
  mode: "draft_non_production";
  status: "dry_run_passed" | "completed" | "completed_with_warnings" | "configuration_required" | "saved_with_issues";
  checkedAt: string;
  diagnostics?: WorkflowDiagnostic[];
  configuration?: WorkflowRunConfigurationRequirements;
};

export type WorkflowRunGateFacts = {
  /** Server-projected basis for the currently committed Graph. */
  currentBasis: ValidationBasis;
  /** Server-projected environment identity; never an environment value bag. */
  environment: { name: string; bindingVersion: string };
  /** Optional server artifact. Absence cannot be interpreted as a passed Dry-run. */
  postCheck?: WorkflowRunPostCheckFact;
};

export type WorkflowRunGateState =
  | "unavailable"
  | "unchecked"
  | "checking"
  | "validated"
  | "current"
  | "stale"
  | "configuration_required"
  | "saved_with_issues";

export type WorkflowPublishedRunTarget = {
  workflowKey: WorkflowKey;
  resourceVersion: string;
  semanticHash: string;
};

export type WorkflowDraftDryRunTarget = {
  mode: "draft_non_production";
  workflowKey: WorkflowKey;
  basis: ValidationBasis;
  environment: string;
};

export type WorkflowRunView = {
  gateState: WorkflowRunGateState;
  canPublish: boolean;
  configuration: { secretRefs: string[]; environments: string[] };
  currentRevision?: string;
  environment?: string;
  draftDryRun: { enabled: boolean; target?: WorkflowDraftDryRunTarget; reason?: string };
  publishedRun: { enabled: boolean; target?: WorkflowPublishedRunTarget; reason?: string };
};

/**
 * Derives Run UI from authoritative workspace facts. Full-basis and environment
 * facts are explicit because the committed Graph revision alone cannot prove
 * Catalog/reference/environment freshness. Missing facts always fail closed.
 */
export function selectWorkflowRunView(state: WorkflowWorkspaceState, facts?: WorkflowRunGateFacts): WorkflowRunView {
  const committed = state.committed;
  const currentBasis = facts?.currentBasis;
  const currentBasisMatches = Boolean(
    committed && currentBasis && validBasis(currentBasis) &&
    currentBasis.resourceVersion === committed.revision.resourceVersion &&
    currentBasis.semanticHash === committed.revision.semanticHash &&
    facts.environment.bindingVersion === currentBasis.environmentBindingVersion,
  );
  const postCheckIsCurrent = Boolean(
    currentBasisMatches && facts?.postCheck && validPostCheck(facts.postCheck) && sameBasis(facts.postCheck.basis, currentBasis!),
  );
  const currentValidation = currentBasisMatches ? selectCommittedValidation(state, currentBasis) : undefined;
  const currentReceipt = currentBasisMatches ? selectLatestReceiptForBasis(state, currentBasis!) : undefined;
  const gateState = selectRunGateState({
    currentBasisMatches,
    currentValidation,
    currentReceipt,
    postCheck: postCheckIsCurrent ? facts?.postCheck : undefined,
    hasExplicitFacts: Boolean(facts && committed),
    hasAnyGate: Boolean(facts?.postCheck || Object.keys(state.validationArtifacts).length || state.receiptOrder.length),
  });
  const workflowKey = committed?.workflowKey;
  const environmentName = safeIdentifier(facts?.environment.name);
  const environmentCurrent = Boolean(
    currentBasisMatches && environmentName && facts?.environment.bindingVersion === currentBasis?.environmentBindingVersion,
  );
  const candidateBlocksActions = selectCandidateState(state) !== "none";
  const commonActionReady = state.connection.state === "online" && !state.gap && !candidateBlocksActions;
  const dryRunAllowed = commonActionReady && environmentCurrent && (gateState === "validated" || gateState === "current");
  const published = selectLatestPublishedArtifact(state);
  const publishedTarget = workflowKey && published && validBasis(published.basis)
    ? { workflowKey, resourceVersion: published.basis.resourceVersion, semanticHash: published.basis.semanticHash }
    : undefined;
  const lifecycleBlocked = gateState === "unavailable" || gateState === "checking" || gateState === "configuration_required" || gateState === "saved_with_issues";
  const publishedAllowed = commonActionReady && Boolean(publishedTarget) && !lifecycleBlocked;

  return {
    gateState,
    canPublish: commonActionReady && gateState === "current",
    configuration: postCheckIsCurrent && facts?.postCheck?.status === "configuration_required"
      ? safeConfiguration(facts.postCheck.configuration)
      : { secretRefs: [], environments: [] },
    ...(committed ? { currentRevision: committed.revision.resourceVersion } : {}),
    ...(environmentName ? { environment: environmentName } : {}),
    draftDryRun: {
      enabled: dryRunAllowed,
      ...(dryRunAllowed && workflowKey && currentBasis && environmentName ? {
        target: { mode: "draft_non_production", workflowKey, basis: cloneBasis(currentBasis), environment: environmentName },
      } : {}),
      ...(!dryRunAllowed ? { reason: runDisabledReason(gateState, facts, candidateBlocksActions, state) } : {}),
    },
    publishedRun: {
      enabled: publishedAllowed,
      ...(publishedTarget ? { target: publishedTarget } : {}),
      ...(!publishedAllowed ? { reason: publishedTarget ? runDisabledReason(gateState, facts, candidateBlocksActions, state) : "尚无已发布版本" } : {}),
    },
  };
}

/**
 * The single lifecycle Primary projection consumed by every presenter.
 * A missing authoritative target returns null instead of a disabled or
 * best-effort action. Handler/capability availability is enforced by the
 * action registry, never by falling back to a legacy Graph endpoint.
 */
export function selectWorkflowPrimaryActionViewModel(
  state: WorkflowWorkspaceState,
  facts?: WorkflowRunGateFacts,
): WorkflowPrimaryActionViewModel | null {
  const committed = state.committed;
  const workflowId = committed?.workflowKey.workflowId;
  if (!committed || !workflowId) return null;

  const candidateState = selectCandidateState(state);
  if (state.connection.state !== "online" ||
    state.gap ||
    ["reading_context", "planning", "previewing", "applying", "cancelling"].includes(state.task.state)) {
    return null;
  }
  const candidate = state.candidate;
  if ((candidateState === "ready" || candidateState === "conflict") && candidate) {
    const target = {
      kind: "candidate" as const,
      workflowId,
      candidateId: candidate.candidate.id,
      changeSetId: candidate.changeSet.id,
      previewHash: candidate.changeSet.previewHash,
      expectedRevision: candidate.changeSet.base.resourceVersion,
    };
    return candidateState === "conflict"
      ? {
          id: "resolve_conflict",
          label: "审阅冲突",
          target,
          owner: "candidate_workspace_action_bar",
          enabled: true,
          reason: undefined,
          capability: "workflow.resolve_conflict",
        }
      : {
          id: "apply_changes",
          label: candidate.changeSet.operations.length > 0
            ? `应用 ${candidate.changeSet.operations.length} 项修改`
            : "应用修改",
          target,
          owner: "candidate_workspace_action_bar",
          enabled: true,
          reason: undefined,
          capability: "workflow.apply_candidate",
        };
  }

  if (candidateState !== "none") return null;

  const run = selectWorkflowRunView(state, facts);
  const draftTarget = {
    kind: "saved_draft" as const,
    workflowId,
    resourceVersion: committed.revision.resourceVersion,
    semanticHash: committed.revision.semanticHash,
  };

  if (run.gateState === "saved_with_issues") {
    const blockingCount = countBlockingDiagnostics(state, facts);
    return {
      id: "fix_issues",
      label: blockingCount > 0 ? `修复 ${blockingCount} 个问题` : "修复问题",
      target: { kind: "workflow_panel", workflowId, panel: "changes" },
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.review_issues",
    };
  }

  // Environment configuration is intentionally absent until a trusted
  // capability projects an exact entry target. Reopening Run is not recovery.
  if (run.gateState === "configuration_required" || run.gateState === "checking") return null;

  if (["unavailable", "unchecked", "stale"].includes(run.gateState)) {
    return {
      id: "check",
      label: "检查工作流",
      target: draftTarget,
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.validate",
    };
  }

  if (run.gateState === "validated") {
    const target = run.draftDryRun.target;
    if (!target) return null;
    return {
      id: "dry_run",
      label: "试运行当前草稿",
      target: {
        kind: "draft_non_production",
        workflowId: target.workflowKey.workflowId,
        resourceVersion: target.basis.resourceVersion,
        semanticHash: target.basis.semanticHash,
        validationBasis: cloneBasis(target.basis),
        environment: target.environment,
      },
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.draft_dry_run",
    };
  }

  const lifecycle = selectLifecycleState(state);
  if (lifecycle === "published") {
    const target = run.publishedRun.enabled ? run.publishedRun.target : undefined;
    if (!target) return null;
    return {
      id: "run_published",
      label: "运行已发布版本",
      target: {
        kind: "published_revision",
        workflowKey: { ...target.workflowKey },
        resourceVersion: target.resourceVersion,
        semanticHash: target.semanticHash,
      },
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.run_published",
    };
  }

  if (!run.canPublish) return null;
  return {
    id: lifecycle === "published_with_draft_changes" ? "publish_changes" : "publish",
    label: lifecycle === "published_with_draft_changes" ? "发布更改" : "发布工作流",
    target: draftTarget,
    owner: "command_bar",
    enabled: true,
    reason: undefined,
    capability: "workflow.publish",
  };
}

export function selectWorkflowLifecycleTraceView(
  state: WorkflowWorkspaceState,
  facts: WorkflowRunGateFacts | undefined,
  saveState: WorkflowSavePresentation,
): WorkflowLifecycleTraceViewModel {
  const run = selectWorkflowRunView(state, facts);
  const lifecycle = selectLifecycleState(state);
  const stages: WorkflowLifecycleTraceStageViewModel[] = [
    saveTraceStage(saveState),
    checkTraceStage(run.gateState),
    draftRunTraceStage(run),
    publishTraceStage(lifecycle, run.gateState),
  ];
  return {
    stages,
    compactLabel: lifecycleCompactLabel(run, lifecycle),
    ariaLabel: stages.map((stage) => `${stage.label}：${stage.description}`).join("；"),
  };
}

export function selectWorkflowWorkspaceView(
  state: WorkflowWorkspaceState,
  now = new Date(),
  facts?: WorkflowRunGateFacts,
) {
  const axes = selectWorkflowWorkspaceAxes(state, now);
  const changeCount = state.candidate?.changeSet.operations.length ?? 0;
  return {
    axes,
    primaryAction: selectWorkflowPrimaryActionViewModel(state, facts),
    issues: validateWorkflowInteractionAxes(axes),
    changeCount,
    applyOutcome: selectApplyOutcome(state),
    runRevision: selectRunRevision(state),
  };
}

function saveTraceStage(state: WorkflowSavePresentation): WorkflowLifecycleTraceStageViewModel {
  if (state === "applying") return { id: "save", label: "保存", state: "in_progress", description: "正在应用" };
  if (state === "unsaved") return { id: "save", label: "保存", state: "blocked", description: "未保存" };
  if (state === "saved_with_issues") return { id: "save", label: "保存", state: "complete", description: "已保存但有问题" };
  return { id: "save", label: "保存", state: "complete", description: "已自动保存" };
}

function checkTraceStage(state: WorkflowRunGateState): WorkflowLifecycleTraceStageViewModel {
  if (state === "checking") return { id: "check", label: "检查", state: "in_progress", description: "正在检查" };
  if (state === "validated" || state === "current") return { id: "check", label: "检查", state: "complete", description: "静态检查通过" };
  if (state === "configuration_required") return { id: "check", label: "检查", state: "complete", description: "检查通过，还需配置" };
  if (state === "saved_with_issues") return { id: "check", label: "检查", state: "blocked", description: "检查发现阻断问题" };
  if (state === "stale") return { id: "check", label: "检查", state: "current", description: "检查已过期" };
  if (state === "unavailable") return { id: "check", label: "检查", state: "blocked", description: "检查依据未同步" };
  return { id: "check", label: "检查", state: "current", description: "待检查" };
}

function draftRunTraceStage(run: WorkflowRunView): WorkflowLifecycleTraceStageViewModel {
  const state = run.gateState;
  if (state === "current") return { id: "draft_run", label: "草稿试运行", state: "complete", description: "试运行通过" };
  if (state === "validated" && run.draftDryRun.enabled) {
    return { id: "draft_run", label: "草稿试运行", state: "current", description: "可试运行" };
  }
  if (state === "validated") {
    return {
      id: "draft_run",
      label: "草稿试运行",
      state: "blocked",
      description: run.environment ? "当前不可试运行" : "运行环境未绑定",
    };
  }
  if (state === "configuration_required") return { id: "draft_run", label: "草稿试运行", state: "blocked", description: "缺少可信环境配置" };
  if (state === "saved_with_issues" || state === "unavailable") {
    return { id: "draft_run", label: "草稿试运行", state: "blocked", description: "当前不可试运行" };
  }
  return { id: "draft_run", label: "草稿试运行", state: "pending", description: "未运行" };
}

function publishTraceStage(
  lifecycle: WorkflowLifecycleState,
  gate: WorkflowRunGateState,
): WorkflowLifecycleTraceStageViewModel {
  if (lifecycle === "published") return { id: "publish", label: "发布", state: "complete", description: "已发布" };
  if (gate === "current") {
    return {
      id: "publish",
      label: "发布",
      state: "current",
      description: lifecycle === "published_with_draft_changes" ? "有更改待发布" : "可发布",
    };
  }
  if (gate === "saved_with_issues" || gate === "configuration_required" || gate === "unavailable") {
    return { id: "publish", label: "发布", state: "blocked", description: "当前不可发布" };
  }
  return {
    id: "publish",
    label: "发布",
    state: "pending",
    description: lifecycle === "published_with_draft_changes" ? "有更改待发布" : "未发布",
  };
}

function lifecycleCompactLabel(
  run: WorkflowRunView,
  lifecycle: WorkflowLifecycleState,
): string {
  const gate = run.gateState;
  if (gate === "current") return lifecycle === "published" ? "已发布 · 可运行" : "试运行通过 · 可发布";
  if (gate === "validated") {
    if (run.draftDryRun.enabled) return "静态检查通过 · 可试运行";
    return run.environment ? "静态检查通过 · 暂不可试运行" : "静态检查通过 · 运行环境未绑定";
  }
  return ({
    unavailable: "检查依据未同步",
    unchecked: "待检查",
    checking: "正在检查",
    stale: "检查已过期",
    configuration_required: "还需配置测试环境",
    saved_with_issues: "检查发现阻断问题",
  } as const)[gate];
}

export function selectCandidateState(state: WorkflowWorkspaceState): WorkflowCandidateState {
  const fact = state.candidate;
  if (!fact) return "none";
  if (fact.changeSet.state === "superseded" || fact.candidate.state === "superseded") return "superseded";
  if (fact.changeSet.state === "conflict" || fact.candidate.state === "conflict") return "conflict";
  if (state.committed && fact.changeSet.base.resourceVersion !== state.committed.revision.resourceVersion) return "stale";
  if (fact.candidate.state === "editing") return "editing";
  if (fact.changeSet.state === "proposed" || fact.candidate.state === "building") return "building";
  return "ready";
}

export function selectCommittedValidation(state: WorkflowWorkspaceState, expectedBasis?: ValidationBasis): WorkflowValidationArtifactFact | undefined {
  const revision = state.committed?.revision;
  if (!revision) return undefined;
  return Object.values(state.validationArtifacts)
    .filter((artifact) => expectedBasis
      ? sameBasis(artifact.basis, expectedBasis)
      : artifact.basis.resourceVersion === revision.resourceVersion && artifact.basis.semanticHash === revision.semanticHash)
    .sort((left, right) => right.issuedAt.localeCompare(left.issuedAt))[0];
}

export function selectCommittedValidationState(state: WorkflowWorkspaceState): WorkflowValidationState {
  return selectCommittedValidation(state)?.status ?? "unknown";
}

export function selectLifecycleState(state: WorkflowWorkspaceState): WorkflowLifecycleState {
  const latestPublished = Object.values(state.lifecycleArtifacts)
    .filter((artifact) => artifact.status === "published")
    .sort((left, right) => right.issuedAt.localeCompare(left.issuedAt))[0];
  if (!latestPublished || !state.committed) return "draft";
  return basisMatchesCommitted(latestPublished, state) ? "published" : "published_with_draft_changes";
}

export function selectApprovalState(state: WorkflowWorkspaceState, candidateState = selectCandidateState(state), now = new Date()): WorkflowApprovalState {
  const approval = state.approval;
  const candidate = state.candidate;
  if (!approval || !candidate || candidateState !== "ready" || approval.candidateId !== candidate.candidate.id || approval.changeSetId !== candidate.changeSet.id || approval.previewHash !== candidate.changeSet.previewHash) return "none";
  if (approval.state === "consumed") return "none";
  if (approval.state === "expired" || (approval.expiresAt && new Date(approval.expiresAt).getTime() <= now.getTime())) return "expired";
  if (approval.state === "rejected") return "rejected";
  return approval.state;
}

export function selectLatestReceipt(state: WorkflowWorkspaceState): WorkflowReceiptFact | undefined {
  const id = state.receiptOrder[state.receiptOrder.length - 1];
  return id ? state.receipts[id] : undefined;
}

export function selectApplyOutcome(state: WorkflowWorkspaceState): "none" | "pending_event" | "committed" | "failed" {
  const receipt = selectLatestReceipt(state);
  if (!receipt) return "none";
  if (["conflict", "rejected"].includes(receipt.status)) return "failed";
  const after = receipt.revisionAfter?.resourceVersion;
  if (!after || state.committed?.revision.resourceVersion !== after) return "pending_event";
  return "committed";
}

export function selectRunRevision(state: WorkflowWorkspaceState): string | undefined {
  return selectLatestPublishedArtifact(state)?.basis.resourceVersion;
}

function selectRunGateState(
  input: {
    currentBasisMatches: boolean;
    currentValidation?: WorkflowValidationArtifactFact;
    currentReceipt?: WorkflowReceiptFact;
    postCheck?: WorkflowRunPostCheckFact;
    hasExplicitFacts: boolean;
    hasAnyGate: boolean;
  },
): WorkflowRunGateState {
  if (!input.hasExplicitFacts) return "unavailable";
  if (!input.currentBasisMatches) return "stale";
  const status = input.postCheck?.status || input.currentReceipt?.status || input.currentValidation?.status;
  if (status === "configuration_required" || status === "saved_with_issues") return status;
  if (input.postCheck && ["dry_run_passed", "completed", "completed_with_warnings"].includes(input.postCheck.status)) return "current";
  if (input.currentReceipt && ["completed", "completed_with_warnings"].includes(input.currentReceipt.status)) return "current";
  if (input.currentValidation?.status === "checking") return "checking";
  if (input.currentValidation && ["valid", "valid_with_warnings"].includes(input.currentValidation.status)) return "validated";
  return input.hasAnyGate ? "stale" : "unchecked";
}

function selectLatestReceiptForBasis(state: WorkflowWorkspaceState, expectedBasis: ValidationBasis): WorkflowReceiptFact | undefined {
  for (let index = state.receiptOrder.length - 1; index >= 0; index -= 1) {
    const receipt = state.receipts[state.receiptOrder[index]];
    if (receipt && sameBasis(receipt.validation.basis, expectedBasis)) return receipt;
  }
  return undefined;
}

function selectLatestPublishedArtifact(state: WorkflowWorkspaceState): WorkflowLifecycleArtifactFact | undefined {
  return Object.values(state.lifecycleArtifacts)
    .filter((artifact) => artifact.status === "published")
    .sort((left, right) => right.issuedAt.localeCompare(left.issuedAt))[0];
}

function sameBasis(left: ValidationBasis, right: ValidationBasis): boolean {
  return left.resourceVersion === right.resourceVersion &&
    left.semanticHash === right.semanticHash &&
    left.catalogVersion === right.catalogVersion &&
    left.actionSchemaDigest === right.actionSchemaDigest &&
    left.referenceSnapshotHash === right.referenceSnapshotHash &&
    left.environmentBindingVersion === right.environmentBindingVersion;
}

function validBasis(value: ValidationBasis): boolean {
  if (!value || typeof value !== "object") return false;
  const keys = Object.keys(value).sort();
  if (keys.join("\u0000") !== BASIS_KEYS.join("\u0000")) return false;
  return BASIS_KEYS.every((key) => typeof value[key] === "string" && value[key].trim().length > 0);
}

function validPostCheck(value: WorkflowRunPostCheckFact): boolean {
  return value.mode === "draft_non_production" && value.checkedAt.trim().length > 0 &&
    ["dry_run_passed", "completed", "completed_with_warnings", "configuration_required", "saved_with_issues"].includes(value.status) &&
    validBasis(value.basis);
}

function cloneBasis(value: ValidationBasis): ValidationBasis {
  return Object.fromEntries(BASIS_KEYS.map((key) => [key, value[key]])) as ValidationBasis;
}

function safeConfiguration(value?: WorkflowRunConfigurationRequirements) {
  return {
    secretRefs: safeIdentifiers(value?.secretRefs),
    environments: safeIdentifiers(value?.environments),
  };
}

function safeIdentifiers(values?: string[]): string[] {
  return [...new Set((Array.isArray(values) ? values : [])
    .map(safeIdentifier)
    .filter((value): value is string => Boolean(value)))]
    .sort();
}

function safeIdentifier(value: unknown): string | undefined {
  return typeof value === "string" && value.length <= 256 && /^[A-Za-z0-9._:/@+\-]+$/.test(value) ? value : undefined;
}

function runDisabledReason(
  gateState: WorkflowRunGateState,
  facts: WorkflowRunGateFacts | undefined,
  candidateBlocksActions: boolean,
  state: WorkflowWorkspaceState,
): string {
  if (!facts) return "运行依据尚未同步";
  if (state.connection.state !== "online" || state.gap) return "连接恢复后才能运行";
  if (candidateBlocksActions) return "先应用或放弃待确认修改";
  if (gateState === "configuration_required") return "完成缺失配置并重新检查";
  if (gateState === "saved_with_issues") return "修复已保存版本的问题并重新检查";
  if (gateState === "stale") return "检查结果已过期，请重新检查";
  if (gateState === "checking") return "正在检查当前版本";
  if (gateState === "unchecked") return "先检查当前草稿";
  return "运行条件尚未满足";
}

function countBlockingDiagnostics(state: WorkflowWorkspaceState, facts?: WorkflowRunGateFacts): number {
  const committed = state.committed?.revision;
  const diagnostics = Object.values(state.validationArtifacts)
    .filter((artifact) => committed &&
      artifact.basis.resourceVersion === committed.resourceVersion &&
      artifact.basis.semanticHash === committed.semanticHash)
    .flatMap((artifact) => artifact.diagnostics);
  if (facts?.postCheck && committed &&
    facts.postCheck.basis.resourceVersion === committed.resourceVersion &&
    facts.postCheck.basis.semanticHash === committed.semanticHash) {
    diagnostics.push(...(facts.postCheck.diagnostics || []));
  }
  return diagnostics.filter((item) => ["error", "fatal", "blocking"].includes(item.severity.toLowerCase())).length;
}

function basisMatchesCommitted(artifact: WorkflowLifecycleArtifactFact, state: WorkflowWorkspaceState): boolean {
  return artifact.basis.resourceVersion === state.committed?.revision.resourceVersion && artifact.basis.semanticHash === state.committed?.revision.semanticHash;
}

const BASIS_KEYS = [
  "actionSchemaDigest",
  "catalogVersion",
  "environmentBindingVersion",
  "referenceSnapshotHash",
  "resourceVersion",
  "semanticHash",
] as const satisfies readonly (keyof ValidationBasis)[];
