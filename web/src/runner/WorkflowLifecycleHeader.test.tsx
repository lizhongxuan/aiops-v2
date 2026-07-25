import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workspace/workflowWorkspaceReducer";
import type { WorkflowRunGateFacts } from "./workspace/workflowWorkspaceSelectors";
import type { WorkflowCandidateFact, WorkflowWorkspaceState } from "./workspace/workflowWorkspaceTypes";
import { deriveWorkflowLifecycleHeaderView, WorkflowLifecycleHeader } from "./WorkflowLifecycleHeader";

const key = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, layoutHash: `layout-${value}` });
const basis = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" });

function loaded(value = "rv-1"): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(createWorkflowWorkspaceState(), {
    type: "snapshot.loaded",
    snapshot: { workflowKey: key, revision: revision(value), graph: {}, sequence: 1, loadedAt: "2026-07-21T00:00:00Z" },
  });
}

function facts(value = "rv-1", postCheck: WorkflowRunGateFacts["postCheck"] = undefined): WorkflowRunGateFacts {
  return { currentBasis: basis(value), environment: { name: "staging", bindingVersion: "env-1" }, ...(postCheck ? { postCheck } : {}) };
}

function currentFacts(value = "rv-1") {
  return facts(value, { basis: basis(value), mode: "draft_non_production", status: "dry_run_passed", checkedAt: "2026-07-21T00:00:02Z" });
}

function candidate(): WorkflowCandidateFact {
  return {
    candidate: { id: "candidate-1", workflowKey: key, currentChangeSetId: "change-1", currentRevision: 1, currentPreviewHash: "preview-1", state: "ready", expiresAt: "2030-01-01T00:00:00Z" },
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2", id: "change-1", workflowKey: key, candidateId: "candidate-1", candidateRevision: 1,
      actor: "agent", intent: "更新两个步骤", base: revision("rv-1"),
      operations: [{ op: "delete_edge", edgeId: "edge-1" }, { op: "update_node", nodeId: "node-1", set: { label: "检查" } }],
      previewHash: "preview-1", validation: { basis: basis("rv-1"), valid: true, diagnostics: [] }, state: "previewed",
      createdAt: "2026-07-21T00:00:00Z", expiresAt: "2030-01-01T00:00:00Z",
    },
  };
}

describe("WorkflowLifecycleHeader", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("maps unchecked, blocking, validated and ready states to one exact lifecycle action", () => {
    expect(deriveWorkflowLifecycleHeaderView(loaded(), facts()).action).toMatchObject({ id: "check", label: "检查工作流" });

    const diagnostics = [
      { code: "E_ONE", severity: "error", message: "sensitive-one", nodeId: "node-1" },
      { code: "E_TWO", severity: "blocking", message: "sensitive-two", nodeId: "node-2" },
    ];
    const blocked = facts("rv-1", { basis: basis("rv-1"), mode: "draft_non_production", status: "saved_with_issues", checkedAt: "2026-07-21T00:00:03Z", diagnostics });
    expect(deriveWorkflowLifecycleHeaderView(loaded(), blocked).action).toMatchObject({ id: "fix_issues", label: "修复 2 个问题" });

    let validated = loaded();
    validated = workflowWorkspaceReducer(validated, { type: "validation.received", artifact: { artifactId: "valid-1", basis: basis("rv-1"), status: "valid", diagnostics: [], issuedAt: "2026-07-21T00:00:01Z" } });
    expect(deriveWorkflowLifecycleHeaderView(validated, facts()).action).toMatchObject({ id: "dry_run", label: "试运行当前草稿", enabled: true });
    expect(deriveWorkflowLifecycleHeaderView(loaded(), currentFacts()).action).toMatchObject({ id: "publish", label: "发布工作流", enabled: true });
  });

  it("distinguishes published clean from published dirty and never runs the draft", () => {
    let published = workflowWorkspaceReducer(loaded(), { type: "lifecycle.received", artifact: { artifactId: "published-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    expect(deriveWorkflowLifecycleHeaderView(published, currentFacts()).action).toMatchObject({ id: "run_published", label: "运行已发布版本" });

    published = workflowWorkspaceReducer(published, { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:01:00Z" } });
    expect(deriveWorkflowLifecycleHeaderView(published, currentFacts("rv-2")).action).toMatchObject({ id: "publish_changes", label: "发布更改" });
    expect(deriveWorkflowLifecycleHeaderView(published, currentFacts("rv-2")).publishedRevision).toBe("rv-1");
  });

  it("always delegates the active Candidate primary action to the review bar", async () => {
    const state = workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() });
    const onAction = vi.fn();
    await act(async () => root.render(<WorkflowLifecycleHeader state={state} facts={currentFacts()} onAction={onAction} />));
    expect(container.querySelector('[data-testid="workflow-primary-action"]')).toBeNull();
    expect(container.textContent).toContain("主操作位于审阅栏");

    expect(container.querySelector('[data-testid="workflow-primary-action"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-lifecycle-header"]')?.getAttribute("data-candidate-primary-owner"))
      .toBe("candidate_workspace_action_bar");
    expect(onAction).not.toHaveBeenCalled();
  });

  it.each([
    ["auto_saved", "已自动保存"],
    ["unsaved", "未保存"],
    ["applying", "正在应用"],
    ["saved_with_issues", "已保存但有问题"],
  ] as const)("shows %s without a permanent Save action", async (saveState, label) => {
    await act(async () => root.render(<WorkflowLifecycleHeader state={loaded()} facts={facts()} saveState={saveState} onAction={() => undefined} />));
    expect(container.querySelector('[data-testid="workflow-save-status"]')?.textContent).toContain(label);
    expect([...container.querySelectorAll("button")].some((button) => button.textContent?.trim() === "保存")).toBe(false);
  });

  it("renders identity, authoritative status and the action in one topbar control", async () => {
    await act(async () => root.render(
      <WorkflowLifecycleHeader
        state={loaded()}
        facts={facts("rv-2")}
        title="Redis Memory"
        environment="staging"
        placement="topbar"
        onAction={() => undefined}
      />,
    ));

    const header = container.querySelector('[data-testid="workflow-lifecycle-header"]');
    expect(header?.getAttribute("data-placement")).toBe("topbar");
    expect(header?.textContent).toContain("已自动保存");
    expect(header?.textContent).toContain("检查已过期");
    expect(header?.textContent).toContain("检查工作流");
    expect(header?.textContent).toContain("Redis Memory");
    expect(header?.textContent).not.toContain("staging");
  });

  it("does not render an unregistered lifecycle action", async () => {
    await act(async () => root.render(
      <WorkflowLifecycleHeader
        state={loaded()}
        facts={facts()}
        canHandleAction={() => false}
        onAction={vi.fn()}
      />,
    ));
    expect(container.querySelector('[data-testid="workflow-primary-action"]')).toBeNull();
    expect(container.textContent).toContain("当前操作暂不可用");
  });

  it("stops showing applying once the committed snapshot reaches the receipt revision", () => {
    let state = workflowWorkspaceReducer(loaded(), {
      type: "receipt.received",
      receipt: {
        transactionId: "txn-1",
        changeSetId: "change-1",
        workflowKey: key,
        status: "verifying",
        revisionBefore: revision("rv-1"),
        revisionAfter: revision("rv-2"),
        validation: { basis: basis("rv-2"), valid: true, diagnostics: [] },
        receivedAt: "2026-07-21T00:00:00Z",
      },
    });
    expect(deriveWorkflowLifecycleHeaderView(state, facts()).save.state).toBe("applying");
    state = workflowWorkspaceReducer(state, {
      type: "snapshot.loaded",
      snapshot: { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:01Z" },
    });
    expect(deriveWorkflowLifecycleHeaderView(state, facts("rv-2")).save.state).toBe("auto_saved");
  });
});
