import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WORKFLOW_CHANGE_ACTION_EVENT,
  type WorkflowChangeActionEventDetail,
} from "@/components/chat/WorkflowChangeArtifact";
import { RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT } from "@/components/runner/RunnerCanvas";
import type { WorkflowWorkspaceContextValue } from "@/runner/workspace/WorkflowWorkspaceProvider";
import { parseWorkflowTransportArtifact } from "@/runner/workspace/workflowCandidateArtifact";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";
import {
  WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT,
  type WorkflowCandidateApplyConfirmation,
} from "./WorkflowCandidateApplyDialog";
import { WorkflowCandidateWorkspaceActionBar } from "./WorkflowCandidateWorkspaceActionBar";

const mocks = vi.hoisted(() => ({
  state: {} as AiopsTransportState,
  workspace: {} as WorkflowWorkspaceContextValue,
  sendTransportCommand: vi.fn(),
}));

vi.mock("@assistant-ui/react", () => ({
  useAssistantTransportState: () => mocks.state,
  useAssistantTransportSendCommand: () => mocks.sendTransportCommand,
}));

vi.mock("@/runner/workspace/WorkflowWorkspaceProvider", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/runner/workspace/WorkflowWorkspaceProvider")>();
  return { ...actual, useWorkflowWorkspace: () => mocks.workspace };
});

const workflowKey = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" };
const binding = {
  candidateId: "candidate-1",
  changeSetId: "change-1",
  previewHash: "preview-1",
  expectedRevision: "rv-1",
};

describe("WorkflowCandidateWorkspaceActionBar", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    mocks.workspace = workspace();
    mocks.state = transportState();
    vi.clearAllMocks();
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    document.body.querySelectorAll('[data-slot="dialog-portal"]').forEach((node) => node.remove());
  });

  async function render() {
    await act(async () => root.render(<WorkflowCandidateWorkspaceActionBar />));
  }

  it("opens one final-confirmation Dialog for the canonical Apply, makes its owner inert, and delegates Discard", async () => {
    const applyAction = {
      id: "apply-candidate-1",
      intent: "workflow.apply_candidate",
      label: "应用修改",
      target: { ...binding, approvalId: "approval-1" },
    };
    const artifact = candidateArtifact([applyAction]);
    mocks.workspace = workspace(artifact);
    mocks.state = transportState({
      status: "blocked",
      currentTurnId: "turn-1",
      turnOrder: ["turn-1"],
      turns: {
        "turn-1": workflowTurn(artifact, [
          ["artifact-1", artifactBlock(artifact)],
          ["approval-block-1", approvalBlock("approval-1")],
        ]),
      },
      pendingApprovals: {
        "approval-1": { id: "approval-1", turnId: "turn-1", status: "pending", reason: "应用当前候选" },
      },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-1": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    const observed: WorkflowCandidateApplyConfirmation[] = [];
    const listener = (event: Event) => observed.push((event as CustomEvent<WorkflowCandidateApplyConfirmation>).detail);
    window.addEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);

    await render();
    expect(container.textContent).toContain("1 项修改尚未应用");
    expect(container.textContent).not.toContain("workflow.apply_candidate");
    expect(container.textContent).not.toContain("changeSetId");

    await act(async () => {
      (container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).click();
    });
    const owner = container.querySelector('[data-testid="workflow-candidate-workspace-actions"]') as HTMLElement;
    const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
    expect(dialog).not.toBeNull();
    expect(owner.hasAttribute("inert")).toBe(true);
    expect(owner.getAttribute("aria-hidden")).toBe("true");
    expect(observed).toEqual([]);

    const confirm = document.body.querySelector('[data-testid="workflow-candidate-confirm-apply"]') as HTMLButtonElement;
    await act(async () => {
      confirm.click();
      confirm.click();
    });
    expect(observed).toEqual([{
      schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
      artifactId: "preview-artifact-1",
      actionId: applyAction.id,
      approvalId: "approval-1",
      binding,
      changeCount: 1,
    }]);
    await act(async () => {
      await new Promise((resolve) => window.requestAnimationFrame(resolve));
    });
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    expect(owner.hasAttribute("inert")).toBe(false);
    expect(document.activeElement).toBe(container.querySelector('[data-testid="workflow-candidate-apply"]'));

    await act(async () => {
      ([...container.querySelectorAll("button")].find((button) => button.textContent?.includes("放弃修改")) as HTMLButtonElement).click();
    });
    expect(mocks.workspace.discardCandidate).toHaveBeenCalledTimes(1);
    window.removeEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);
  });

  it("routes the exact Canvas keyboard Apply command through the same final-confirmation Dialog", async () => {
    const action = canonicalApplyAction();
    const artifact = candidateArtifact([action]);
    mocks.workspace = workspace(artifact);
    mocks.state = authorizationState(artifact);
    const observed: WorkflowCandidateApplyConfirmation[] = [];
    const listener = (event: Event) => observed.push((event as CustomEvent<WorkflowCandidateApplyConfirmation>).detail);
    window.addEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);
    await render();

    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", ...binding },
    })));
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();
    expect(observed).toEqual([]);

    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", ...binding, previewHash: "forged-preview" },
    })));
    await act(async () => {
      (document.body.querySelector('[data-testid="workflow-candidate-confirm-apply"]') as HTMLButtonElement).click();
    });
    expect(observed).toEqual([expect.objectContaining({
      artifactId: "preview-artifact-1",
      actionId: action.id,
      binding,
    })]);
    window.removeEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);
  });

  it("requests approval exactly once for a transport Candidate before any live approval and disables while requested", async () => {
    const artifact = candidateArtifact([]);
    const current = workspace(artifact);
    let resolveApproval!: (value: unknown) => void;
    const requestApproval = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveApproval = resolve; }));
    current.requestCandidateApplyApproval = requestApproval;
    mocks.workspace = current;
    mocks.state = transportState();

    await render();
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    expect(apply).not.toBeNull();
    expect(apply.disabled).toBe(false);
    await act(async () => {
      apply.click();
      apply.click();
      window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
        detail: { command: "apply_candidate", ...binding },
      }));
      await Promise.resolve();
    });
    expect(requestApproval).toHaveBeenCalledTimes(1);
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();

    mocks.workspace = {
      ...current,
      candidateApplyApproval: {
        status: "requested",
        ...binding,
        candidateRevision: 1,
        sessionId: "session-1",
        turnId: "turn-requested",
        approvalId: "approval-requested",
        requestedAt: "2026-07-21T08:00:00Z",
      },
    };
    await act(async () => {
      resolveApproval({ status: "requested" });
      await Promise.resolve();
    });
    await render();
    expect((container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).disabled).toBe(true);
    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", ...binding },
    })));
    expect(requestApproval).toHaveBeenCalledTimes(1);
  });

  it("keeps a transport Candidate fail-closed when the approval request handler is unavailable", async () => {
    const artifact = candidateArtifact([]);
    mocks.workspace = { ...workspace(artifact), requestCandidateApplyApproval: undefined };
    mocks.state = transportState();

    await render();
    expect((container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).disabled).toBe(true);
    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", ...binding },
    })));
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
  });

  it("requests server-owned approval for a History Candidate and never emits a fake Apply", async () => {
    const artifact = historyCandidateArtifact();
    const current = workspace(artifact);
    const requestApproval = vi.fn().mockResolvedValue({ status: "requested" });
    current.requestCandidateApplyApproval = requestApproval;
    mocks.workspace = current;
    mocks.state = transportState();
    const applyEvents = vi.fn();
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);

    await render();
    expect(container.querySelector('[data-testid="workflow-history-approval-guide"]')?.textContent).toContain("请求确认后");
    const request = container.querySelector('[data-testid="workflow-candidate-request-approval"]') as HTMLButtonElement;
    expect(request.textContent).toContain("请求应用确认");
    expect(request.disabled).toBe(false);
    await act(async () => request.click());
    expect(requestApproval).toHaveBeenCalledTimes(1);
    expect(applyEvents).not.toHaveBeenCalled();

    const historyBinding = {
      candidateId: `history-${"a".repeat(64)}`,
      changeSetId: "history-change-1",
      previewHash: "history-preview-1",
    };
    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", ...historyBinding },
    })));
    expect(requestApproval).toHaveBeenCalledTimes(2);
    expect(applyEvents).not.toHaveBeenCalled();

    mocks.workspace = {
      ...current,
      candidateApplyApproval: {
        status: "requested",
        ...historyBinding,
        candidateRevision: 1,
        expectedRevision: "rv-1",
        sessionId: "session-a",
        turnId: "turn-history-1",
        approvalId: "approval-history-1",
        requestedAt: "2026-07-21T08:00:00Z",
      },
    };
    await render();
    expect(container.querySelector('[data-testid="workflow-history-approval-guide"]')?.textContent).toContain("在本操作栏完成最终确认");
    expect((container.querySelector('[data-testid="workflow-candidate-request-approval"]') as HTMLButtonElement).disabled).toBe(true);
    expect(container.textContent).not.toContain("workflow.apply_candidate");
    window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);
  });

  it("requests revised-Agent approval only after explicit Apply and coalesces a double click", async () => {
    const artifact = revisedAgentCandidateArtifact();
    const current = workspace(artifact);
    let resolveApproval!: (value: unknown) => void;
    const requestApproval = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveApproval = resolve; }));
    current.requestCandidateApplyApproval = requestApproval;
    mocks.workspace = current;
    mocks.state = transportState();
    const applyEvents = vi.fn();
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);

    await render();
    expect(requestApproval).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="workflow-revised-approval-guide"]')?.textContent).toContain("最新候选版本");
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    expect(apply.disabled).toBe(false);
    await act(async () => {
      apply.click();
      apply.click();
      await Promise.resolve();
    });
    expect(requestApproval).toHaveBeenCalledTimes(1);
    expect(applyEvents).not.toHaveBeenCalled();

    await act(async () => {
      resolveApproval({ status: "requested" });
      await Promise.resolve();
    });
    window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);
  });

  it("routes exact revised-Agent keyboard Apply to approval and ignores stale bindings", async () => {
    const artifact = revisedAgentCandidateArtifact();
    const current = workspace(artifact);
    const requestApproval = vi.fn().mockResolvedValue({ status: "requested" });
    current.requestCandidateApplyApproval = requestApproval;
    mocks.workspace = {
      ...current,
      candidateApplyApproval: {
        status: "requested",
        candidateId: "candidate-1",
        candidateRevision: 2,
        changeSetId: "change-2",
        previewHash: "preview-2",
        expectedRevision: "rv-1",
        sessionId: "session-1",
        turnId: "turn-old",
        approvalId: "approval-old",
        requestedAt: "2026-07-21T08:00:00Z",
      },
    };
    mocks.state = transportState();
    await render();

    // A v2 approval cannot authorize the current v3 Candidate.
    expect(container.querySelector('[data-testid="workflow-revised-approval-guide"]')?.textContent).not.toContain("等待 AI 面板确认");
    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", candidateId: "candidate-1", changeSetId: "change-3", previewHash: "preview-3" },
    })));
    expect(requestApproval).toHaveBeenCalledTimes(1);

    await act(async () => window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
      detail: { command: "apply_candidate", candidateId: "candidate-1", changeSetId: "change-2", previewHash: "preview-2" },
    })));
    expect(requestApproval).toHaveBeenCalledTimes(1);
  });

  it.each([
    {
      name: "History",
      createArtifact: historyCandidateArtifact,
      candidateRevision: 1,
      currentBinding: {
        candidateId: `history-${"a".repeat(64)}`,
        changeSetId: "history-change-1",
        previewHash: "history-preview-1",
        expectedRevision: "rv-1",
      },
    },
    {
      name: "revised",
      createArtifact: revisedAgentCandidateArtifact,
      candidateRevision: 3,
      currentBinding: {
        candidateId: "candidate-1",
        changeSetId: "change-3",
        previewHash: "preview-3",
        expectedRevision: "rv-1",
      },
    },
  ])("opens the final Dialog for an exact transport-owned pending approval of a $name Candidate", async ({
    createArtifact,
    candidateRevision,
    currentBinding,
  }) => {
    const artifact = createArtifact();
    const action = {
      id: `apply-${currentBinding.changeSetId}`,
      intent: "workflow.apply_candidate",
      label: "应用修改",
      target: { ...currentBinding, approvalId: "approval-1" },
    };
    artifact.actions = [action];
    const current = workspace(artifact);
    current.candidateApplyApproval = {
      status: "requested",
      ...currentBinding,
      candidateRevision,
      sessionId: "session-1",
      turnId: "turn-1",
      approvalId: "approval-1",
      requestedAt: "2026-07-21T08:00:00Z",
    };
    mocks.workspace = current;
    mocks.state = authorizationState(artifact);
    const observed: WorkflowCandidateApplyConfirmation[] = [];
    const listener = (event: Event) => observed.push((event as CustomEvent<WorkflowCandidateApplyConfirmation>).detail);
    window.addEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);

    await render();
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    expect(apply).not.toBeNull();
    expect(apply.disabled).toBe(false);
    await act(async () => apply.click());
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();

    await act(async () => {
      (document.body.querySelector('[data-testid="workflow-candidate-confirm-apply"]') as HTMLButtonElement).click();
    });
    expect(observed).toEqual([expect.objectContaining({
      artifactId: artifact.id,
      actionId: action.id,
      approvalId: "approval-1",
      binding: currentBinding,
    })]);
    window.removeEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, listener);
  });

  it("authorizes an exact server-controlled approval without a Candidate artifact and sends its owned turn decision", async () => {
    const artifact = revisedAgentCandidateArtifact();
    const currentBinding = {
      candidateId: "candidate-1",
      changeSetId: "change-3",
      previewHash: "preview-3",
      expectedRevision: "rv-1",
    };
    mocks.workspace = {
      ...workspace(artifact),
      candidateApplyApproval: controlledApproval(currentBinding, 3),
    };
    mocks.state = controlledAuthorizationState();
    const artifactConfirmation = vi.fn();
    window.addEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, artifactConfirmation);

    await render();
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    expect(apply.disabled).toBe(false);
    await act(async () => apply.click());
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();
    await act(async () => {
      (document.body.querySelector('[data-testid="workflow-candidate-confirm-apply"]') as HTMLButtonElement).click();
    });

    expect(mocks.sendTransportCommand).toHaveBeenCalledTimes(1);
    expect(mocks.sendTransportCommand).toHaveBeenCalledWith({
      type: "aiops.approval-decision",
      sessionId: "session-1",
      turnId: "turn-controlled",
      approvalId: "approval-controlled",
      decision: "accept",
    });
    expect(artifactConfirmation).not.toHaveBeenCalled();
    window.removeEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, artifactConfirmation);
  });

  it.each([
    {
      name: "response session differs from the transport",
      approval: { sessionId: "session-forged" },
      transport: {},
    },
    {
      name: "response Candidate digest differs from the current Candidate",
      approval: { previewHash: "preview-forged" },
      transport: {},
    },
    {
      name: "response Candidate revision differs from the current Candidate",
      approval: { candidateRevision: 2 },
      transport: {},
    },
    {
      name: "server approval is expired",
      approval: { expiresAt: "2020-01-01T00:00:00Z" },
      transport: {},
    },
    {
      name: "controlled turn is not current",
      approval: {},
      transport: { currentTurnId: "turn-old" },
    },
    {
      name: "approval is not live",
      approval: {},
      transport: {
        runtimeLiveness: {
          activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
        },
      },
    },
    {
      name: "another live approval competes",
      approval: {},
      transport: {
        pendingApprovals: {
          "approval-controlled": { id: "approval-controlled", turnId: "turn-controlled", status: "blocked" },
          "approval-competing": { id: "approval-competing", turnId: "turn-controlled", status: "blocked" },
        },
        runtimeLiveness: {
          activeTurns: {}, activeAgents: {},
          pendingApprovals: { "approval-controlled": true, "approval-competing": true },
          pendingUserInputs: {}, activeCommandStreams: {},
        },
      },
    },
    {
      name: "ordered approval block is already settled",
      approval: {},
      transport: {
        turns: { "turn-controlled": controlledTurn("completed") },
      },
    },
  ])("keeps server-controlled Apply fail-closed when $name", async ({ approval, transport }) => {
    const artifact = revisedAgentCandidateArtifact();
    mocks.workspace = {
      ...workspace(artifact),
      candidateApplyApproval: {
        ...controlledApproval({
          candidateId: "candidate-1",
          changeSetId: "change-3",
          previewHash: "preview-3",
          expectedRevision: "rv-1",
        }, 3),
        ...approval,
      },
    };
    mocks.state = controlledAuthorizationState(transport as Partial<AiopsTransportState>);

    await render();
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    if (!apply.disabled) {
      await act(async () => apply.click());
    }
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    expect(mocks.sendTransportCommand).not.toHaveBeenCalled();
  });

  it("closes an invalidated controlled Dialog and returns focus to the stable Workbench trigger", async () => {
    const artifact = revisedAgentCandidateArtifact();
    const currentBinding = {
      candidateId: "candidate-1",
      changeSetId: "change-3",
      previewHash: "preview-3",
      expectedRevision: "rv-1",
    };
    mocks.workspace = {
      ...workspace(artifact),
      candidateApplyApproval: controlledApproval(currentBinding, 3),
    };
    mocks.state = controlledAuthorizationState();
    const workbench = document.createElement("button");
    workbench.dataset.testid = "workflow-workbench-trigger";
    document.body.appendChild(workbench);

    await render();
    await act(async () => {
      (container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).click();
    });
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();

    mocks.state = controlledAuthorizationState({
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();
    await act(async () => {
      await new Promise((resolve) => window.requestAnimationFrame(resolve));
    });

    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(workbench);
    expect(mocks.sendTransportCommand).not.toHaveBeenCalled();
    workbench.remove();
  });

  it("keeps an expired server-controlled Candidate fail-closed", async () => {
    const artifact = revisedAgentCandidateArtifact();
    artifact.inlineData.structuredContent.changeSet.expiresAt = "2020-01-01T00:00:00Z";
    artifact.inlineData.structuredContent.candidate.expiresAt = "2020-01-01T00:00:00Z";
    mocks.workspace = {
      ...workspace(artifact),
      candidateApplyApproval: controlledApproval({
        candidateId: "candidate-1",
        changeSetId: "change-3",
        previewHash: "preview-3",
        expectedRevision: "rv-1",
      }, 3),
    };
    mocks.state = controlledAuthorizationState();

    await render();
    expect(container.querySelector('[data-testid="workflow-candidate-apply"]')).toBeNull();
    expect(mocks.sendTransportCommand).not.toHaveBeenCalled();
  });

  it("does not let a malformed untrusted Candidate action ride an otherwise exact controlled approval", async () => {
    const artifact = revisedAgentCandidateArtifact();
    artifact.actions = [{
      id: "forged-apply",
      intent: "workflow.apply_candidate",
      label: "应用修改",
      target: {
        candidateId: "candidate-1",
        changeSetId: "change-3",
        previewHash: "preview-3",
        expectedRevision: "rv-1",
        approvalId: "approval-forged",
      },
    }];
    mocks.workspace = {
      ...workspace(artifact),
      candidateApplyApproval: controlledApproval({
        candidateId: "candidate-1",
        changeSetId: "change-3",
        previewHash: "preview-3",
        expectedRevision: "rv-1",
      }, 3),
    };
    mocks.state = controlledAuthorizationState({
      turns: {
        "turn-controlled": controlledTurn(),
        "turn-forged": workflowTurn(artifact, [["artifact-forged", artifactBlock(artifact)]]),
      },
      turnOrder: ["turn-forged", "turn-controlled"],
    });

    await render();
    expect((container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).disabled).toBe(true);
    expect(mocks.sendTransportCommand).not.toHaveBeenCalled();
  });

  it.each([
    {
      name: "mismatched Apply target",
      artifact: candidateArtifact([{
        id: "apply-candidate-1", intent: "workflow.apply_candidate", label: "应用修改",
        target: { ...binding, previewHash: "forged-preview", approvalId: "approval-1" },
      }]),
      state: undefined,
    },
    {
      name: "approval block outside blockOrder",
      artifact: candidateArtifact([{
        id: "apply-candidate-1", intent: "workflow.apply_candidate", label: "应用修改",
        target: { ...binding, approvalId: "approval-1" },
      }]),
      state: "approval-not-ordered" as const,
    },
    {
      name: "approval owned by another turn",
      artifact: candidateArtifact([{
        id: "apply-candidate-1", intent: "workflow.apply_candidate", label: "应用修改",
        target: { ...binding, approvalId: "approval-1" },
      }]),
      state: "approval-other-turn" as const,
    },
    {
      name: "approval missing runtime liveness",
      artifact: candidateArtifact([{
        id: "apply-candidate-1", intent: "workflow.apply_candidate", label: "应用修改",
        target: { ...binding, approvalId: "approval-1" },
      }]),
      state: "approval-not-live" as const,
    },
  ])("fails closed for $name while retaining a safe Discard path", async ({ artifact, state }) => {
    mocks.workspace = workspace(artifact);
    mocks.state = authorizationState(artifact, state);
    const observed = vi.fn();
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, observed);

    await render();
    expect(container.querySelector('[data-testid="workflow-candidate-recovery-hint"]')?.textContent).toContain("等待确认步骤");
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    await act(async () => apply.click());
    expect(observed).not.toHaveBeenCalled();

    await act(async () => {
      ([...container.querySelectorAll("button")].find((button) => button.textContent?.includes("放弃修改")) as HTMLButtonElement).click();
    });
    expect(mocks.workspace.discardCandidate).toHaveBeenCalledTimes(1);
    window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, observed);
  });

  it("fails closed when two distinct canonical artifacts compete for the current binding", async () => {
    const first = candidateArtifact([canonicalApplyAction()], "preview-artifact-1");
    const second = candidateArtifact([canonicalApplyAction()], "preview-artifact-2");
    mocks.workspace = workspace(first);
    mocks.state = transportState({
      turnOrder: ["turn-1", "turn-2"],
      turns: {
        "turn-1": workflowTurn(first, [["artifact-1", artifactBlock(first)], ["approval-1", approvalBlock("approval-1")]]),
        "turn-2": workflowTurn(second, [["artifact-2", artifactBlock(second)], ["approval-2", approvalBlock("approval-1")]]),
      },
      pendingApprovals: { "approval-1": { id: "approval-1", turnId: "turn-2", status: "pending" } },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-1": true }, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });

    await render();
    expect(container.querySelector('[data-testid="workflow-candidate-recovery-hint"]')).not.toBeNull();
    expect((container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement).disabled).toBe(true);
  });

  it("renders nothing without a current Workspace candidate", async () => {
    mocks.workspace = workspace();
    await render();
    expect(container.innerHTML).toBe("");
  });
});

function canonicalApplyAction() {
  return {
    id: "apply-candidate-1",
    intent: "workflow.apply_candidate",
    label: "应用修改",
    target: { ...binding, approvalId: "approval-1" },
  };
}

function authorizationState(artifact: ReturnType<typeof candidateArtifact>, mode?: "approval-not-ordered" | "approval-other-turn" | "approval-not-live") {
  const ordered = mode === "approval-not-ordered"
    ? [["artifact-1", artifactBlock(artifact)] as const]
    : [["artifact-1", artifactBlock(artifact)] as const, ["approval-block-1", approvalBlock("approval-1")] as const];
  const turn = workflowTurn(artifact, ordered);
  if (mode === "approval-not-ordered") {
    turn.blocksById = { ...turn.blocksById, "approval-block-1": approvalBlock("approval-1") };
  }
  return transportState({
    status: "blocked",
    turnOrder: ["turn-1"],
    turns: { "turn-1": turn },
    pendingApprovals: {
      "approval-1": { id: "approval-1", turnId: mode === "approval-other-turn" ? "turn-2" : "turn-1", status: "pending" },
    },
    runtimeLiveness: {
      activeTurns: {}, activeAgents: {}, pendingApprovals: mode === "approval-not-live" ? {} : { "approval-1": true },
      pendingUserInputs: {}, activeCommandStreams: {},
    },
  });
}

function controlledApproval(
  currentBinding: typeof binding,
  candidateRevision: number,
): NonNullable<WorkflowWorkspaceContextValue["candidateApplyApproval"]> {
  return {
    status: "requested",
    ...currentBinding,
    candidateRevision,
    sessionId: "session-1",
    turnId: "turn-controlled",
    approvalId: "approval-controlled",
    expiresAt: "2030-01-01T00:00:00Z",
    requestedAt: new Date().toISOString(),
  };
}

function controlledTurn(approvalStatus: "blocked" | "completed" = "blocked") {
  return {
    id: "turn-controlled",
    status: "blocked" as const,
    workflowContext: {
      workflowId: "workflow-a",
      revision,
      selection: { nodeIds: [], edgeIds: [] },
      environment: { bindingVersion: "env-1" },
    },
    blockOrder: ["approval-block-controlled"],
    blocksById: {
      "approval-block-controlled": {
        id: "approval-block-controlled",
        type: "approval" as const,
        kind: "approval" as const,
        status: approvalStatus,
        approvalId: "approval-controlled",
        text: "确认应用",
      },
    },
  };
}

function controlledAuthorizationState(overrides: Partial<AiopsTransportState> = {}) {
  return transportState({
    status: "blocked",
    currentTurnId: "turn-controlled",
    turnOrder: ["turn-controlled"],
    turns: { "turn-controlled": controlledTurn() },
    pendingApprovals: {
      "approval-controlled": {
        id: "approval-controlled",
        turnId: "turn-controlled",
        status: "blocked",
      },
    },
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: { "approval-controlled": true },
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    ...overrides,
  });
}

function transportState(overrides: Partial<AiopsTransportState> = {}): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "session-1",
    threadId: "workflow:workflow-a",
    status: "idle",
    turns: {}, turnOrder: [], pendingApprovals: {}, mcpSurfaces: {}, artifacts: {},
    runtimeLiveness: {
      activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
    },
    hostMissions: {}, childAgents: {}, seq: 1, updatedAt: "2026-07-21T08:00:00Z",
    ...overrides,
  };
}

function workflowTurn(
  _artifact: ReturnType<typeof candidateArtifact>,
  blocks: ReadonlyArray<readonly [string, ReturnType<typeof artifactBlock> | ReturnType<typeof approvalBlock>]>,
) {
  return {
    id: blocks.some(([id]) => id.includes("2")) ? "turn-2" : "turn-1",
    status: "blocked" as const,
    workflowContext: {
      workflowId: "workflow-a", revision, selection: { nodeIds: [], edgeIds: [] }, environment: { bindingVersion: "env-1" },
    },
    blockOrder: blocks.map(([id]) => id),
    blocksById: Object.fromEntries(blocks),
  };
}

function artifactBlock(artifact: ReturnType<typeof candidateArtifact>) {
  return { id: `block-${artifact.id}`, type: "artifact" as const, kind: "assistant" as const, status: "completed" as const, artifact };
}

function approvalBlock(approvalId: string) {
  return { id: `block-${approvalId}`, type: "approval" as const, kind: "approval" as const, status: "blocked" as const, approvalId, text: "确认应用" };
}

function workspace(artifact?: ReturnType<typeof candidateArtifact>): WorkflowWorkspaceContextValue {
  const parsed = artifact ? parseWorkflowTransportArtifact(artifact, {
    workflowKey, committedRevision: revision, receivedAt: "2026-07-21T08:00:00Z",
  }) : undefined;
  const candidate = parsed?.kind === "candidate" ? parsed.fact : undefined;
  return {
    state: {
      schemaVersion: "aiops.workflow.workspace.v1", workflowKey,
      committed: { workflowKey, revision, graph: { nodes: [], edges: [] }, sequence: 1, loadedAt: "2026-07-21T08:00:00Z" },
      candidate,
      selection: { nodeIds: [], edgeIds: [] }, receipts: {}, receiptOrder: [], events: {}, eventOrder: [], bufferedEvents: {},
      lastSequence: 1, validationArtifacts: {}, lifecycleArtifacts: {}, task: { state: "idle" }, connection: { state: "online" }, runTarget: "draft",
    },
    commands: {} as WorkflowWorkspaceContextValue["commands"], pendingCommands: [],
    reconcile: vi.fn(), reconcileTransportFact: vi.fn(), refresh: vi.fn(), clearUnsavedError: vi.fn(),
    undoHistory: vi.fn(), redoHistory: vi.fn(), clearHistoryError: vi.fn(), reviseCandidate: vi.fn(),
    discardCandidate: vi.fn().mockResolvedValue(undefined), clearCandidateActionError: vi.fn(), setSelection: vi.fn(), setViewport: vi.fn(),
    requestCandidateApplyApproval: vi.fn().mockResolvedValue({ status: "requested" }),
  };
}

function candidateArtifact(actions: Array<Record<string, unknown>>, id = "preview-artifact-1") {
  return {
    id, type: "workflow_patch_preview", renderer: "workflow.change.preview.v2", schemaVersion: "aiops.workflow.edit.v2",
    inlineData: {
      schemaVersion: "aiops.workflow.edit.v2", isError: false, content: [{ type: "text", text: "候选修改已生成" }],
      structuredContent: {
        kind: "preview",
        changeSet: {
          schemaVersion: "aiops.workflow.edit.v2", id: "change-1", workflowKey, candidateId: "candidate-1", candidateRevision: 1,
          actor: "agent", intent: "更新检查节点", base: revision,
          operations: [{ op: "update_node", nodeId: "check", set: { label: "容量检查" } }], previewHash: "preview-1",
          validation: {
            kind: "validation",
            basis: {
              resourceVersion: "rv-1", semanticHash: "semantic-1", catalogVersion: "catalog-1",
              actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1",
            },
            valid: true, diagnostics: [],
          },
          state: "previewed", createdAt: "2026-07-21T07:59:00Z", expiresAt: "2030-01-01T00:00:00Z",
        },
        candidate: {
          id: "candidate-1", workflowKey, currentChangeSetId: "change-1", currentRevision: 1,
          currentPreviewHash: "preview-1", state: "ready", expiresAt: "2030-01-01T00:00:00Z",
        },
        diff: { touchedPaths: ["nodes/check/label"] },
      },
    },
    actions,
  };
}

function historyCandidateArtifact() {
  const artifact = candidateArtifact([]);
  const content = artifact.inlineData.structuredContent;
  const candidateId = `history-${"a".repeat(64)}`;
  content.changeSet = {
    ...content.changeSet,
    id: "history-change-1",
    candidateId,
    actor: "user",
    intent: "undo workflow transaction txn-17",
    previewHash: "history-preview-1",
  };
  content.candidate = {
    ...content.candidate,
    id: candidateId,
    currentChangeSetId: "history-change-1",
    currentPreviewHash: "history-preview-1",
  };
  return artifact;
}

function revisedAgentCandidateArtifact() {
  const artifact = candidateArtifact([]);
  const content = artifact.inlineData.structuredContent;
  content.changeSet = {
    ...content.changeSet,
    id: "change-3",
    candidateRevision: 3,
    parentChangeSetId: "change-2",
    actor: "user",
    intent: "用户调整候选节点参数",
    previewHash: "preview-3",
  };
  content.candidate = {
    ...content.candidate,
    currentChangeSetId: "change-3",
    currentRevision: 3,
    currentPreviewHash: "preview-3",
  };
  return artifact;
}
