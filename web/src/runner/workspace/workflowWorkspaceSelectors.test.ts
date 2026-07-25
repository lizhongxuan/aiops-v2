import { describe, expect, it } from "vitest";
import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workflowWorkspaceReducer";
import {
  selectApplyOutcome,
  selectCandidateState,
  selectCommittedValidationState,
  selectRunRevision,
  selectWorkflowLifecycleTraceView,
  selectWorkflowPrimaryActionViewModel,
  selectWorkflowRunView,
  selectWorkflowWorkspaceView,
  type WorkflowRunGateFacts,
} from "./workflowWorkspaceSelectors";
import type { WorkflowSavePresentation } from "./workflowInteractionContract";
import type { WorkflowCandidateFact, WorkflowReceiptFact, WorkflowWorkspaceState } from "./workflowWorkspaceTypes";

const key = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (value: string) => ({ resourceVersion: value, semanticHash: `sha256:${value}`, layoutHash: `sha256:layout-${value}` });
const basis = (value: string) => ({ resourceVersion: value, semanticHash: `sha256:${value}`, catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" });

const runFacts = (value = "rv-1", overrides: Partial<WorkflowRunGateFacts> = {}): WorkflowRunGateFacts => ({
  currentBasis: basis(value),
  environment: { name: "staging", bindingVersion: "env-1" },
  postCheck: {
    basis: basis(value),
    mode: "draft_non_production",
    status: "dry_run_passed",
    checkedAt: "2026-07-21T00:00:02Z",
  },
  ...overrides,
});

function loaded(): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-1"), graph: { nodes: [], edges: [] }, sequence: 1, loadedAt: "2026-07-21T00:00:00Z" } });
}

function validated(): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(loaded(), {
    type: "validation.received",
    artifact: {
      artifactId: "valid-current",
      basis: basis("rv-1"),
      status: "valid",
      diagnostics: [],
      issuedAt: "2026-07-21T00:00:01Z",
    },
  });
}

function candidate(base = "rv-1", state: WorkflowCandidateFact["changeSet"]["state"] = "previewed"): WorkflowCandidateFact {
  return {
    candidate: { id: "candidate-1", workflowKey: key, currentChangeSetId: "change-1", currentRevision: 1, currentPreviewHash: "preview-1", state: state === "conflict" ? "conflict" : "ready", expiresAt: "2030-01-01T00:00:00Z" },
    changeSet: { schemaVersion: "aiops.workflow.edit.v2", id: "change-1", workflowKey: key, candidateId: "candidate-1", candidateRevision: 1, actor: "agent", intent: "change", base: revision(base), operations: [{ op: "delete_edge", edgeId: "edge-1" }], previewHash: "preview-1", validation: { basis: basis(base), valid: true, diagnostics: [] }, state, createdAt: "2026-07-21T00:00:00Z", expiresAt: "2030-01-01T00:00:00Z" },
  };
}

function receipt(): WorkflowReceiptFact {
  return { transactionId: "txn-1", changeSetId: "change-1", workflowKey: key, status: "verifying", revisionBefore: revision("rv-1"), revisionAfter: revision("rv-2"), validation: { basis: basis("rv-2"), valid: true, diagnostics: [] }, receivedAt: "2026-07-21T00:00:00Z" };
}

function published(): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(loaded(), {
    type: "lifecycle.received",
    artifact: {
      artifactId: "publish-1",
      basis: basis("rv-1"),
      status: "published",
      issuedAt: "2026-07-21T00:00:00Z",
    },
  });
}

function publishedWithDraftChanges(): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(published(), {
    type: "snapshot.loaded",
    snapshot: {
      workflowKey: key,
      revision: revision("rv-2"),
      graph: {},
      sequence: 2,
      loadedAt: "2026-07-21T00:00:01Z",
    },
  });
}

type PrimaryActionFixture = {
  id: `UX-HDR-${string}`;
  state: () => WorkflowWorkspaceState;
  facts: () => WorkflowRunGateFacts;
  save: WorkflowSavePresentation;
  actionId: string | null;
  owner: "command_bar" | "candidate_workspace_action_bar" | null;
  saveStage: "complete" | "blocked";
};

const primaryActionFixtures: PrimaryActionFixture[] = [
  {
    id: "UX-HDR-001",
    state: loaded,
    facts: () => runFacts("rv-1", { postCheck: undefined }),
    save: "auto_saved",
    actionId: "check",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-002",
    state: validated,
    facts: () => runFacts("rv-1", {
      postCheck: {
        ...runFacts("rv-1").postCheck!,
        status: "configuration_required",
        configuration: { environments: ["staging"] },
      },
    }),
    save: "auto_saved",
    actionId: null,
    owner: null,
    saveStage: "complete",
  },
  {
    id: "UX-HDR-003",
    state: validated,
    facts: () => runFacts("rv-1", { postCheck: undefined }),
    save: "auto_saved",
    actionId: "dry_run",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-004",
    state: loaded,
    facts: runFacts,
    save: "auto_saved",
    actionId: "publish",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-005",
    state: published,
    facts: runFacts,
    save: "auto_saved",
    actionId: "run_published",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-006",
    state: publishedWithDraftChanges,
    facts: () => runFacts("rv-2"),
    save: "auto_saved",
    actionId: "publish_changes",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-007",
    state: () => workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() }),
    facts: runFacts,
    save: "auto_saved",
    actionId: "apply_changes",
    owner: "candidate_workspace_action_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-008",
    state: loaded,
    facts: () => runFacts("rv-1", { postCheck: undefined }),
    save: "auto_saved",
    actionId: "check",
    owner: "command_bar",
    saveStage: "complete",
  },
  {
    id: "UX-HDR-009",
    state: loaded,
    facts: () => runFacts("rv-1", { postCheck: undefined }),
    save: "unsaved",
    actionId: "check",
    owner: "command_bar",
    saveStage: "blocked",
  },
  {
    id: "UX-HDR-010",
    state: () => workflowWorkspaceReducer(loaded(), { type: "connection.changed", state: "offline" }),
    facts: runFacts,
    save: "auto_saved",
    actionId: null,
    owner: null,
    saveStage: "complete",
  },
];

describe("workflowWorkspaceSelectors", () => {
  it.each(primaryActionFixtures)("$id freezes the named Primary truth-table fixture", (fixture) => {
    const state = fixture.state();
    const facts = fixture.facts();
    const action = selectWorkflowPrimaryActionViewModel(state, facts);
    expect(action?.id ?? null).toBe(fixture.actionId);
    expect(action?.owner ?? null).toBe(fixture.owner);
    expect(action ? 1 : 0).toBeLessThanOrEqual(1);
    expect(selectWorkflowLifecycleTraceView(state, facts, fixture.save).stages[0].state).toBe(fixture.saveStage);
  });

  it("does not let Candidate validation light up committed validation", () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() });
    expect(selectCommittedValidationState(state)).toBe("unknown");
    state = workflowWorkspaceReducer(state, { type: "validation.received", artifact: { artifactId: "stale", basis: basis("rv-old"), status: "valid", diagnostics: [], issuedAt: "2026-07-21T00:00:00Z" } });
    expect(selectCommittedValidationState(state)).toBe("unknown");
    state = workflowWorkspaceReducer(state, { type: "validation.received", artifact: { artifactId: "current", basis: basis("rv-1"), status: "valid_with_warnings", diagnostics: [], issuedAt: "2026-07-21T00:00:01Z" } });
    expect(selectCommittedValidationState(state)).toBe("valid_with_warnings");
  });

  it("derives stale, conflict and supersede from typed revision facts", () => {
    expect(selectCandidateState(workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate("rv-old") }))).toBe("stale");
    expect(selectCandidateState(workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate("rv-1", "conflict") }))).toBe("conflict");
    const superseded = candidate(); superseded.changeSet.state = "superseded";
    expect(selectCandidateState(workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: superseded }))).toBe("superseded");
  });

  it("reports apply unknown outcome until the committed snapshot catches up", () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "receipt.received", receipt: receipt() });
    expect(selectApplyOutcome(state)).toBe("pending_event");
    expect(selectWorkflowWorkspaceView(state).axes.taskState).toBe("verifying");
    state = workflowWorkspaceReducer(state, { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z" } });
    expect(selectApplyOutcome(state)).toBe("committed");
  });

  it("keeps published and draft run targets explicit", () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "lifecycle.received", artifact: { artifactId: "publish-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    expect(selectWorkflowWorkspaceView(state).axes.lifecycleState).toBe("published");
    state = workflowWorkspaceReducer(state, { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z" } });
    expect(selectWorkflowWorkspaceView(state).axes.lifecycleState).toBe("published_with_draft_changes");
    expect(selectRunRevision(state)).toBe("rv-1");
    state = workflowWorkspaceReducer(state, { type: "run-target.changed", target: "published" });
    expect(selectWorkflowWorkspaceView(state).runRevision).toBe("rv-1");
  });

  it("UI-025 fails the publish gate when any complete ValidationBasis dimension is stale", () => {
    const fields = Object.keys(basis("rv-2")) as Array<keyof ReturnType<typeof basis>>;
    for (const field of fields) {
      let state = workflowWorkspaceReducer(loaded(), {
        type: "snapshot.loaded",
        snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z" },
      });
      const currentBasis = basis("rv-2");
      const staleBasis = { ...currentBasis, [field]: `${currentBasis[field]}-stale` };
      const view = selectWorkflowRunView(state, runFacts("rv-2", {
        currentBasis,
        postCheck: { ...runFacts("rv-2").postCheck!, basis: staleBasis },
      }));
      expect(view.gateState, field).toBe("stale");
      expect(view.canPublish, field).toBe(false);
      expect(view.draftDryRun.enabled, field).toBe(false);
    }
  });

  it("UI-026 accepts a server-reissued layout-only gate for the new resourceVersion", () => {
    let state = workflowWorkspaceReducer(loaded(), {
      type: "snapshot.loaded",
      snapshot: {
        workflowKey: key,
        revision: { resourceVersion: "rv-layout", semanticHash: "sha256:rv-1", layoutHash: "sha256:layout-new" },
        graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z",
      },
    });
    const reissued = { ...basis("rv-1"), resourceVersion: "rv-layout" };
    state = workflowWorkspaceReducer(state, {
      type: "validation.received",
      artifact: { artifactId: "layout-reissued", basis: reissued, status: "valid", diagnostics: [], issuedAt: "2026-07-21T00:00:02Z" },
    });
    const view = selectWorkflowRunView(state, runFacts("rv-1", {
      currentBasis: reissued,
      postCheck: { ...runFacts("rv-1").postCheck!, basis: reissued },
    }));
    expect(view.gateState).toBe("current");
    expect(view.canPublish).toBe(true);
  });

  it("UI-027 keeps a committed graph visible but closes publish and run for saved_with_issues", () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "lifecycle.received", artifact: { artifactId: "publish-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    state = workflowWorkspaceReducer(state, {
      type: "receipt.received",
      receipt: { ...receipt(), status: "saved_with_issues", validation: { basis: basis("rv-2"), valid: false, diagnostics: [{ code: "POST_CHECK_FAILED", severity: "error", message: "provider detail must stay out of the Run panel" }] } },
    });
    state = workflowWorkspaceReducer(state, { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z" } });
    const view = selectWorkflowRunView(state, runFacts("rv-2", { postCheck: undefined }));
    expect(view.gateState).toBe("saved_with_issues");
    expect(view.canPublish).toBe(false);
    expect(view.publishedRun).toMatchObject({ enabled: false, target: { resourceVersion: "rv-1" } });
  });

  it("requires explicit current basis and environment facts instead of inventing a runnable target", () => {
    const state = loaded();
    const view = selectWorkflowRunView(state);
    expect(view.gateState).toBe("unavailable");
    expect(view.canPublish).toBe(false);
    expect(view.draftDryRun).toMatchObject({ enabled: false });
    expect(view.publishedRun.enabled).toBe(false);
    expect(view.publishedRun.target).toBeUndefined();
  });

  it("derives one command-bar action with an exact saved draft target", () => {
    const action = selectWorkflowPrimaryActionViewModel(validated(), runFacts("rv-1", { postCheck: undefined }));
    expect(action).toEqual({
      id: "dry_run",
      label: "试运行当前草稿",
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.draft_dry_run",
      target: {
        kind: "draft_non_production",
        workflowId: "workflow-a",
        resourceVersion: "rv-1",
        semanticHash: "sha256:rv-1",
        validationBasis: basis("rv-1"),
        environment: "staging",
      },
    });
  });

  it("delegates Candidate actions and fails closed for stale, offline and untrusted environment configuration", () => {
    const withCandidate = workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() });
    expect(selectWorkflowPrimaryActionViewModel(withCandidate, runFacts())).toMatchObject({
      id: "apply_changes",
      owner: "candidate_workspace_action_bar",
      target: {
        kind: "candidate",
        workflowId: "workflow-a",
        candidateId: "candidate-1",
        changeSetId: "change-1",
        previewHash: "preview-1",
        expectedRevision: "rv-1",
      },
    });

    const offline = workflowWorkspaceReducer(loaded(), { type: "connection.changed", state: "offline" });
    expect(selectWorkflowPrimaryActionViewModel(offline, runFacts())).toBeNull();

    expect(selectWorkflowPrimaryActionViewModel(
      loaded(),
      runFacts("rv-1", {
        postCheck: {
          ...runFacts("rv-1").postCheck!,
          status: "configuration_required",
          configuration: { environments: ["staging"] },
        },
      }),
    )).toBeNull();
  });

  it("binds a published action only to the immutable published revision", () => {
    const published = workflowWorkspaceReducer(loaded(), {
      type: "lifecycle.received",
      artifact: {
        artifactId: "publish-1",
        basis: basis("rv-1"),
        status: "published",
        issuedAt: "2026-07-21T00:00:00Z",
      },
    });
    expect(selectWorkflowPrimaryActionViewModel(published, runFacts())).toEqual({
      id: "run_published",
      label: "运行已发布版本",
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.run_published",
      target: {
        kind: "published_revision",
        workflowKey: key,
        resourceVersion: "rv-1",
        semanticHash: "sha256:rv-1",
      },
    });
  });

  it("projects the four lifecycle stages from the same current basis without exposing a full hash", () => {
    const trace = selectWorkflowLifecycleTraceView(validated(), runFacts("rv-1", { postCheck: undefined }), "auto_saved");
    expect(trace.stages.map((stage) => stage.id)).toEqual(["save", "check", "draft_run", "publish"]);
    expect(trace.stages.map((stage) => stage.state)).toEqual(["complete", "complete", "current", "pending"]);
    expect(trace.compactLabel).toBe("静态检查通过 · 可试运行");
    expect(JSON.stringify(trace)).not.toContain("sha256:rv-1");
  });

  it("does not claim that a validated draft can run when no trusted environment target exists", () => {
    const facts = runFacts("rv-1", {
      postCheck: undefined,
      environment: { name: "", bindingVersion: "env-1" },
    });
    const trace = selectWorkflowLifecycleTraceView(validated(), facts, "auto_saved");
    expect(selectWorkflowPrimaryActionViewModel(validated(), facts)).toBeNull();
    expect(trace.compactLabel).toBe("静态检查通过 · 运行环境未绑定");
    expect(trace.stages.find((stage) => stage.id === "draft_run")).toMatchObject({
      state: "blocked",
      description: "运行环境未绑定",
    });
  });

  it("rejects a post-check that is not explicitly draft_non_production", () => {
    const forged = runFacts("rv-1");
    forged.postCheck = { ...forged.postCheck!, mode: "production" as "draft_non_production" };
    const view = selectWorkflowRunView(loaded(), forged);
    expect(view.gateState).toBe("stale");
    expect(view.canPublish).toBe(false);
  });

  it("derives approval expiry, event-gap reconnection and one primary action", () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() });
    state = workflowWorkspaceReducer(state, { type: "approval.changed", approval: { approvalId: "approval-1", candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1", state: "approved", expiresAt: "2026-07-21T00:00:00Z" } });
    state = workflowWorkspaceReducer(state, { type: "event.received", event: { schemaVersion: "aiops.workflow.edit.v2", eventId: "event-3", sequence: 3, type: "candidate.observed", workflowKey: key, revision: revision("rv-1"), actor: "system", payload: {}, occurredAt: "2026-07-21T00:00:00Z" } });
    const view = selectWorkflowWorkspaceView(state, new Date("2026-07-21T00:00:01Z"));
    expect(view.axes.approvalState).toBe("expired");
    expect(view.axes.connectionState).toBe("reconnecting");
    expect(view.primaryAction).toBeNull();
  });

  it.each(["W0", "W1", "W2", "W3", "W4", "W5", "W6"])("keeps fixture %s legal under the six-axis contract", (fixtureId) => {
    let state = loaded();
    if (fixtureId === "W5") state = workflowWorkspaceReducer(state, { type: "validation.received", artifact: { artifactId: fixtureId, basis: basis("rv-1"), status: "configuration_required", diagnostics: [], issuedAt: "2026-07-21T00:00:00Z" } });
    const view = selectWorkflowWorkspaceView(state);
    expect(view.issues).toEqual([]);
    expect(view.primaryAction).toBeTruthy();
  });
});
