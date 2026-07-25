import type {
  ValidationBasis,
  WorkflowCandidateV2,
  WorkflowChangeSetV2,
  WorkflowDomainEventV2,
  WorkflowJsonValue,
  WorkflowKey,
  WorkflowRevision,
  WorkflowValidationResult,
} from "./workflowMcpContracts";
import type {
  WorkflowConnectionState,
  WorkflowTaskState,
} from "./workflowInteractionContract";

export const WORKFLOW_WORKSPACE_STATE_VERSION = "aiops.workflow.workspace.v1" as const;

export type WorkflowCommittedSnapshotFact = {
  workflowKey: WorkflowKey;
  revision: WorkflowRevision;
  graph: WorkflowJsonValue;
  sequence: number;
  loadedAt: string;
};

export type WorkflowCandidateFact = {
  candidate: WorkflowCandidateV2;
  changeSet: WorkflowChangeSetV2;
  projection?: WorkflowJsonValue;
};

export type WorkflowSelectionFact = {
  nodeIds: string[];
  edgeIds: string[];
  anchorId?: string;
};

export type WorkflowViewportFact = { x: number; y: number; zoom: number };

export type WorkflowEnvironmentFact = {
  name?: string;
  bindingVersion: string;
  capabilityVersions?: string[];
};

export type WorkflowReceiptStatus =
  | "verifying"
  | "completed"
  | "completed_with_warnings"
  | "configuration_required"
  | "saved_with_issues"
  | "no_effect"
  | "conflict"
  | "rejected";

export type WorkflowReceiptFact = {
  transactionId: string;
  changeSetId: string;
  workflowKey: WorkflowKey;
  status: WorkflowReceiptStatus;
  revisionBefore: WorkflowRevision;
  revisionAfter?: WorkflowRevision;
  validation: WorkflowValidationResult;
  receivedAt: string;
};

export type WorkflowValidationArtifactFact = {
  artifactId: string;
  basis: ValidationBasis;
  status: "checking" | "valid" | "valid_with_warnings" | "configuration_required" | "saved_with_issues";
  diagnostics: WorkflowValidationResult["diagnostics"];
  issuedAt: string;
};

export type WorkflowLifecycleArtifactFact = {
  artifactId: string;
  basis: ValidationBasis;
  status: "draft" | "published";
  issuedAt: string;
};

export type WorkflowApprovalFact = {
  approvalId: string;
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
  state: "requested" | "approved" | "rejected" | "expired" | "consumed";
  expiresAt?: string;
};

export type WorkflowEventGap = { expectedSequence: number; receivedSequence: number };

export type WorkflowWorkspaceState = {
  schemaVersion: typeof WORKFLOW_WORKSPACE_STATE_VERSION;
  workflowKey?: WorkflowKey;
  committed?: WorkflowCommittedSnapshotFact;
  candidate?: WorkflowCandidateFact;
  selection: WorkflowSelectionFact;
  viewport?: WorkflowViewportFact;
  receipts: Record<string, WorkflowReceiptFact>;
  receiptOrder: string[];
  events: Record<string, WorkflowDomainEventV2>;
  eventOrder: string[];
  bufferedEvents: Record<number, WorkflowDomainEventV2>;
  lastSequence: number;
  gap?: WorkflowEventGap;
  validationArtifacts: Record<string, WorkflowValidationArtifactFact>;
  lifecycleArtifacts: Record<string, WorkflowLifecycleArtifactFact>;
  approval?: WorkflowApprovalFact;
  task: { state: WorkflowTaskState; turnId?: string; errorCode?: string };
  connection: { state: WorkflowConnectionState; changedAt?: string };
  runTarget: "draft" | "published";
};

export type WorkflowWorkspaceAction =
  | { type: "snapshot.loaded"; snapshot: WorkflowCommittedSnapshotFact }
  | { type: "candidate.received"; candidate: WorkflowCandidateFact }
  | { type: "candidate.cleared"; candidateId?: string }
  | { type: "selection.changed"; selection: WorkflowSelectionFact }
  | { type: "viewport.changed"; viewport: WorkflowViewportFact }
  | { type: "receipt.received"; receipt: WorkflowReceiptFact }
  | { type: "event.received"; event: WorkflowDomainEventV2 }
  | { type: "validation.received"; artifact: WorkflowValidationArtifactFact }
  | { type: "lifecycle.received"; artifact: WorkflowLifecycleArtifactFact }
  | { type: "approval.changed"; approval?: WorkflowApprovalFact }
  | { type: "task.changed"; task: WorkflowWorkspaceState["task"] }
  | { type: "connection.changed"; connection: WorkflowWorkspaceState["connection"] }
  | { type: "run-target.changed"; target: WorkflowWorkspaceState["runTarget"] };
