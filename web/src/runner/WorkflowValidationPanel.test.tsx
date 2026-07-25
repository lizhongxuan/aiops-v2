import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workspace/workflowWorkspaceReducer";
import type { WorkflowCandidateFact, WorkflowWorkspaceState } from "./workspace/workflowWorkspaceTypes";
import {
  readWorkflowValidationFocusEvent,
  WORKFLOW_VALIDATION_FOCUS_EVENT,
  WorkflowValidationPanel,
} from "./WorkflowValidationPanel";

const key = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, layoutHash: `layout-${value}` });
const basis = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" });

function loaded(): WorkflowWorkspaceState {
  return workflowWorkspaceReducer(createWorkflowWorkspaceState(), {
    type: "snapshot.loaded",
    snapshot: { workflowKey: key, revision: revision("rv-1"), graph: {}, sequence: 1, loadedAt: "2026-07-21T00:00:00Z" },
  });
}

function candidate(): WorkflowCandidateFact {
  return {
    candidate: { id: "candidate-1", workflowKey: key, currentChangeSetId: "change-1", currentRevision: 1, currentPreviewHash: "preview-1", state: "ready", expiresAt: "2030-01-01T00:00:00Z" },
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2", id: "change-1", workflowKey: key, candidateId: "candidate-1", candidateRevision: 1,
      actor: "agent", intent: "修改候选", base: revision("rv-1"), operations: [{ op: "update_node", nodeId: "candidate-node", set: { label: "候选" } }],
      previewHash: "preview-1", state: "previewed", createdAt: "2026-07-21T00:00:00Z", expiresAt: "2030-01-01T00:00:00Z",
      validation: { basis: basis("rv-1"), valid: true, diagnostics: [{ code: "CANDIDATE_OK", severity: "warning", message: "candidate-secret-do-not-render", nodeId: "candidate-node" }] },
    },
  };
}

describe("WorkflowValidationPanel", () => {
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

  it("renders only current committed diagnostics and focuses an exact node and field", async () => {
    const onFocusIssue = vi.fn();
    const onFocusEvent = vi.fn((event: Event) => readWorkflowValidationFocusEvent(event));
    window.addEventListener(WORKFLOW_VALIDATION_FOCUS_EVENT, onFocusEvent);
    let state = workflowWorkspaceReducer(loaded(), { type: "validation.received", artifact: { artifactId: "stale", basis: basis("rv-old"), status: "saved_with_issues", diagnostics: [{ code: "STALE", severity: "error", message: "stale-secret", nodeId: "old-node" }], issuedAt: "2026-07-21T00:00:00Z" } });
    state = workflowWorkspaceReducer(state, { type: "validation.received", artifact: { artifactId: "current", basis: basis("rv-1"), status: "saved_with_issues", diagnostics: [{ code: "REQUIRED_FIELD", severity: "error", message: "raw-secret-do-not-render", nodeId: "check-host", field: "inputs.hostRef" }], issuedAt: "2026-07-21T00:00:01Z" } });
    await act(async () => root.render(<WorkflowValidationPanel state={state} onFocusIssue={onFocusIssue} />));

    const committed = container.querySelector('[data-validation-authority="committed"]') as HTMLElement;
    expect(committed.textContent).toContain("1 个阻断问题");
    expect(committed.textContent).toContain("REQUIRED_FIELD");
    expect(committed.textContent).toContain("节点 check-host · 字段 inputs.hostRef");
    expect(container.textContent).not.toContain("raw-secret-do-not-render");
    expect(container.textContent).not.toContain("stale-secret");
    const issue = committed.querySelector("button") as HTMLButtonElement;
    expect(issue.dataset.workflowNodeId).toBe("check-host");
    expect(issue.dataset.workflowField).toBe("inputs.hostRef");
    await act(async () => issue.click());
    expect(onFocusIssue).toHaveBeenCalledWith({ nodeId: "check-host", field: "inputs.hostRef" });
    expect(onFocusEvent).toHaveReturnedWith({ nodeId: "check-host", field: "inputs.hostRef" });
    window.removeEventListener(WORKFLOW_VALIDATION_FOCUS_EVENT, onFocusEvent);
  });

  it("keeps Candidate validation visibly separate and never represents it as committed authority", async () => {
    const state = workflowWorkspaceReducer(loaded(), { type: "candidate.received", candidate: candidate() });
    await act(async () => root.render(<WorkflowValidationPanel state={state} />));

    const committed = container.querySelector('[data-validation-authority="committed"]') as HTMLElement;
    const candidateGroup = container.querySelector('[data-validation-authority="candidate"]') as HTMLElement;
    expect(committed.textContent).toContain("待检查");
    expect(candidateGroup.textContent).toContain("待应用修改");
    expect(candidateGroup.textContent).toContain("不能解锁已保存版本的发布或运行");
    expect(candidateGroup.textContent).toContain("CANDIDATE_OK");
    expect(container.textContent).not.toContain("candidate-secret-do-not-render");
  });

  it("uses configuration and saved-with-issues copy without conflating save, validation and publish", async () => {
    let state = workflowWorkspaceReducer(loaded(), { type: "validation.received", artifact: { artifactId: "config", basis: basis("rv-1"), status: "configuration_required", diagnostics: [], issuedAt: "2026-07-21T00:00:00Z" } });
    await act(async () => root.render(<WorkflowValidationPanel state={state} />));
    expect(container.textContent).toContain("还需配置");
    expect(container.textContent).toContain("结构已保存");

    state = workflowWorkspaceReducer(state, { type: "validation.received", artifact: { artifactId: "issues", basis: basis("rv-1"), status: "saved_with_issues", diagnostics: [], issuedAt: "2026-07-21T00:00:01Z" } });
    await act(async () => root.render(<WorkflowValidationPanel state={state} />));
    expect(container.textContent).toContain("已保存");
    expect(container.textContent).toContain("不能发布或运行");
    expect(container.textContent).not.toContain("已完成");
  });
});
