import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";
import { RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT } from "@/components/runner/RunnerCanvas";
import type { WorkflowWorkspaceContextValue } from "@/runner/workspace/WorkflowWorkspaceProvider";
import { parseWorkflowTransportArtifact } from "@/runner/workspace/workflowCandidateArtifact";
import {
  WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT,
  type WorkflowCandidateApplyConfirmation,
} from "./WorkflowCandidateApplyDialog";
import { WorkflowAiTransportPanel, type WorkflowAiReviewTarget } from "./WorkflowAiTransportPanel";

const mocks = vi.hoisted(() => ({
  state: {} as AiopsTransportState,
  workspace: {} as WorkflowWorkspaceContextValue,
  sendCommand: vi.fn(),
  cancelRun: vi.fn(),
  stop: vi.fn(),
  retry: vi.fn(),
  approvalDecision: vi.fn(),
}));

vi.mock("@assistant-ui/react", () => ({
  useAssistantTransportState: () => mocks.state,
  useAssistantTransportSendCommand: () => mocks.sendCommand,
  useAssistantApi: () => ({ thread: () => ({ cancelRun: mocks.cancelRun }) }),
}));

vi.mock("@/transport/useAiopsTransportCommands", () => ({
  useAiopsTransportCommands: () => ({
    stop: mocks.stop,
    retry: mocks.retry,
    approvalDecision: mocks.approvalDecision,
  }),
}));

vi.mock("@/runner/workspace/WorkflowWorkspaceProvider", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/runner/workspace/WorkflowWorkspaceProvider")>();
  return { ...actual, useWorkflowWorkspace: () => mocks.workspace };
});

function state(overrides: Partial<AiopsTransportState> = {}): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "session-a",
    threadId: "workflow:workflow-a",
    status: "idle",
    turns: {},
    turnOrder: [],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
    },
    hostMissions: {},
    childAgents: {},
    seq: 1,
    updatedAt: "2026-07-21T08:00:00Z",
    ...overrides,
  };
}

function workspace(selection = { nodeIds: ["node-a"], edgeIds: ["edge-a"] }): WorkflowWorkspaceContextValue {
  return {
    state: {
      schemaVersion: "aiops.workflow.workspace.v1",
      workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
      committed: {
        workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
        revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
        graph: { nodes: [], edges: [] }, sequence: 1, loadedAt: "2026-07-21T08:00:00Z",
      },
      selection,
      receipts: {}, receiptOrder: [], events: {}, eventOrder: [], bufferedEvents: {}, lastSequence: 1,
      validationArtifacts: {}, lifecycleArtifacts: {}, task: { state: "idle" },
      connection: { state: "online" }, runTarget: "draft",
    },
    commands: {} as WorkflowWorkspaceContextValue["commands"],
    pendingCommands: [],
    reconcile: vi.fn(), reconcileTransportFact: vi.fn(), refresh: vi.fn(), clearUnsavedError: vi.fn(),
    undoHistory: vi.fn(), redoHistory: vi.fn(), clearHistoryError: vi.fn(), reviseCandidate: vi.fn(),
    discardCandidate: vi.fn().mockResolvedValue(undefined), clearCandidateActionError: vi.fn(),
    setSelection: vi.fn(), setViewport: vi.fn(),
  };
}

const workflowKey = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (resourceVersion: string) => ({ resourceVersion, semanticHash: `semantic-${resourceVersion === "rv-1" ? "1" : "2"}`, layoutHash: `layout-${resourceVersion === "rv-1" ? "1" : "2"}` });
const basis = (resourceVersion: string) => ({
  resourceVersion,
  semanticHash: `semantic-${resourceVersion === "rv-1" ? "1" : "2"}`,
  catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1",
});
const envelope = (structuredContent: unknown, isError = false) => ({
  schemaVersion: "aiops.workflow.edit.v2", isError, content: [{ type: "text", text: "typed result" }], structuredContent,
});

function candidateArtifact(
  actions: Array<Record<string, unknown>> = [],
  identity: {
    artifactId?: string;
    candidateId?: string;
    changeSetId?: string;
    previewHash?: string;
    label?: string;
  } = {},
) {
  const artifactId = identity.artifactId || "preview-artifact-1";
  const candidateId = identity.candidateId || "candidate-1";
  const changeSetId = identity.changeSetId || "change-1";
  const previewHash = identity.previewHash || "preview-1";
  const label = identity.label || "容量检查";
  return {
    id: artifactId, type: "workflow_patch_preview", renderer: "workflow.change.preview.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    inlineData: envelope({
      kind: "preview",
      changeSet: {
        schemaVersion: "aiops.workflow.edit.v2", id: changeSetId, workflowKey, candidateId, candidateRevision: 1,
        actor: "agent", intent: "更新检查节点", base: revision("rv-1"),
        operations: [{ op: "update_node", nodeId: "check", set: { label } }], previewHash,
        validation: { kind: "validation", basis: basis("rv-1"), valid: true, diagnostics: [] }, state: "previewed",
        createdAt: "2026-07-21T07:59:00Z", expiresAt: "2026-07-22T07:59:00Z",
      },
      candidate: {
        id: candidateId, workflowKey, currentChangeSetId: changeSetId, currentRevision: 1,
        currentPreviewHash: previewHash, state: "ready", expiresAt: "2026-07-22T07:59:00Z",
      },
      diff: { touchedPaths: ["nodes/check/label"] },
    }),
    actions,
  };
}

function receiptArtifact() {
  return {
    id: "receipt-artifact-1", type: "workflow_patch_result", renderer: "workflow.change.receipt.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    inlineData: envelope({
      schemaVersion: "aiops.workflow.edit.v2", kind: "receipt", transactionId: "txn-1", changeSetId: "change-1", workflowKey,
      status: "verifying", revisionBefore: revision("rv-1"), revisionAfter: revision("rv-2"),
      delta: { addedNodeIds: [], updatedNodeIds: ["check"], deletedNodeIds: [], addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [] },
      validation: { kind: "validation", basis: basis("rv-2"), valid: true, diagnostics: [] }, auditId: "audit-1",
    }),
  };
}

function validationArtifact() {
  return {
    id: "validation-artifact-1", type: "workflow_patch_validation", renderer: "workflow.change.validation.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    inlineData: envelope({ kind: "validation", basis: basis("rv-1"), valid: true, diagnostics: [] }),
  };
}

describe("WorkflowAiTransportPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    mocks.state = state();
    mocks.workspace = workspace();
    vi.clearAllMocks();
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  async function render(onOpenReview?: (target: WorkflowAiReviewTarget) => void) {
    await act(async () => root.render(
      <WorkflowAiTransportPanel
        workflowId="workflow-a"
        transportThreadId="workflow:workflow-a"
        environment={{ name: "staging", bindingVersion: "env-1", capabilityVersions: ["catalog-2", "catalog-1"] }}
        createClientMessageId={() => "message-fixed"}
        onOpenReview={onOpenReview}
      />,
    ));
  }

  it("uses a compact task-oriented empty state and fails closed without invented presets", async () => {
    await render();

    const empty = container.querySelector('[data-testid="workflow-ai-empty-state"]') as HTMLElement;
    expect(empty.textContent).toContain("想修改什么？");
    expect(empty.textContent).toContain("AI 会先展示待确认修改");
    expect(empty.className).not.toContain("border-dashed");
    expect(container.querySelector('[data-testid="workflow-ai-preset"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-ai-transcript"]')).not.toBeNull();
    expect(container.querySelector("form.workflow-ai-composer")).not.toBeNull();
  });

  it("renders commentary, typed artifact and final in blockOrder without parsing final markdown", async () => {
    mocks.state = state({
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a",
          status: "completed",
          workflowContext: {
            workflowId: "workflow-a",
            revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
            selection: { nodeIds: ["node-a"], edgeIds: [] },
            environment: { name: "staging", bindingVersion: "env-1" },
          },
          blockOrder: ["commentary", "tool", "artifact", "final"],
          blocksById: {
            final: { id: "final", type: "final_answer", kind: "assistant", status: "completed", text: "**保存完成**" },
            commentary: { id: "commentary", type: "commentary", kind: "assistant", status: "completed", text: "正在检查" },
            tool: { id: "tool", type: "tool", kind: "tool", status: "completed", text: "已读取当前工作流" },
            artifact: {
              id: "artifact", type: "artifact", kind: "assistant", status: "completed", text: "预览",
              artifact: {
                id: "preview-1", type: "workflow_patch_preview", renderer: "workflow.change.preview.v2",
                schemaVersion: "aiops.workflow.edit.v2", payload: { object: { label: "发布流程" }, changeSummary: "更新超时" },
              },
            },
          },
        },
      },
    });
    await render();

    expect([...container.querySelectorAll("[data-transport-block-id]")].map((node) => node.getAttribute("data-transport-block-id")))
      .toEqual(["commentary", "tool", "artifact", "final"]);
    expect(container.querySelector('[data-testid="workflow-change-artifact"]')).not.toBeNull();
    expect(container.textContent).toContain("本轮基于：node-a");
    expect(container.textContent).toContain("**保存完成**");
    expect(container.querySelector("strong")).toBeNull();
    const process = container.querySelector('[data-testid="workflow-ai-process-details"]') as HTMLDetailsElement;
    expect(process.open).toBe(false);
  });

  it("freezes workflow, selection and environment into one typed add-message command per send", async () => {
    await render();
    expect(container.querySelector("form.workflow-ai-composer")).not.toBeNull();
    const input = container.querySelector('[data-testid="workflow-ai-input"]') as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(input, "只修改所选步骤");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    const send = container.querySelector('[data-testid="workflow-ai-send"]') as HTMLButtonElement;
    await act(async () => {
      send.click();
      send.click();
    });

    expect(mocks.sendCommand).toHaveBeenCalledTimes(1);
    const command = mocks.sendCommand.mock.calls[0][0];
    expect(command).toMatchObject({
      type: "add-message",
      message: {
        id: "message-fixed",
        role: "user",
        workflowContext: {
          workflowId: "workflow-a",
          revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
          selection: { nodeIds: ["node-a"], edgeIds: ["edge-a"] },
          environment: { name: "staging", bindingVersion: "env-1", capabilityVersions: ["catalog-1", "catalog-2"] },
        },
        parts: [{ type: "text", text: "只修改所选步骤" }],
      },
    });

    mocks.workspace = workspace({ nodeIds: ["node-b"], edgeIds: [] });
    await render();
    expect(command.message.workflowContext.selection.nodeIds).toEqual(["node-a"]);
  });

  it("tracks the current canvas selection for the next turn without rewriting an already submitted turn", async () => {
    await render();
    let input = container.querySelector('[data-testid="workflow-ai-input"]') as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(input, "先检查当前选区");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => (container.querySelector('[data-testid="workflow-ai-send"]') as HTMLButtonElement).click());
    const firstContext = mocks.sendCommand.mock.calls[0][0].message.workflowContext;
    expect(firstContext.selection).toEqual({ nodeIds: ["node-a"], edgeIds: ["edge-a"] });

    mocks.workspace = workspace({ nodeIds: ["node-b"], edgeIds: [] });
    await render();
    input = container.querySelector('[data-testid="workflow-ai-input"]') as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(input, "再只检查新选区");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => (container.querySelector('[data-testid="workflow-ai-send"]') as HTMLButtonElement).click());

    expect(firstContext.selection).toEqual({ nodeIds: ["node-a"], edgeIds: ["edge-a"] });
    expect(mocks.sendCommand.mock.calls[1][0].message.workflowContext.selection).toEqual({ nodeIds: ["node-b"], edgeIds: [] });
  });

  it("does not render scope controls and always binds the route workflow plus trusted environment", async () => {
    const onWorkflowScopeChange = vi.fn();
    const onEnvironmentScopeChange = vi.fn();
    const environments = [
      { id: "staging", label: "预发布", environment: { name: "staging", bindingVersion: "env-1" } },
      { id: "production", label: "生产", environment: { name: "production", bindingVersion: "env-2", capabilityVersions: ["catalog-2"] } },
    ];
    await act(async () => root.render(
      <WorkflowAiTransportPanel
        workflowId="workflow-a"
        workflowOptions={[
          { workflowId: "workflow-a", label: "采购审批" },
          { workflowId: "workflow-b", label: "退款审批" },
        ]}
        environment={{ name: "staging", bindingVersion: "env-1" }}
        environmentOptions={environments}
        transportThreadId="workflow:workflow-a"
        onWorkflowScopeChange={onWorkflowScopeChange}
        onEnvironmentScopeChange={onEnvironmentScopeChange}
      />,
    ));

    expect(container.querySelector('[aria-label="本次发送范围"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-scope-workflow"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-scope-selection"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-scope-environment"]')).toBeNull();
    expect(container.textContent).not.toContain("工作流：");
    expect(container.textContent).not.toContain("范围：");
    expect(container.textContent).not.toContain("环境：");

    const input = container.querySelector('[data-testid="workflow-ai-input"]') as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(input, "检查当前工作流");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => (container.querySelector('[data-testid="workflow-ai-send"]') as HTMLButtonElement).click());

    expect(mocks.sendCommand).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.objectContaining({
        metadata: expect.objectContaining({ workflowId: "workflow-a" }),
        workflowContext: expect.objectContaining({
          workflowId: "workflow-a",
          environment: { name: "staging", bindingVersion: "env-1" },
          selection: { nodeIds: ["node-a"], edgeIds: ["edge-a"] },
        }),
      }),
    }));
    expect(onWorkflowScopeChange).not.toHaveBeenCalled();
    expect(onEnvironmentScopeChange).not.toHaveBeenCalled();
  });

  it("uses typed transport commands for stop, retry and approval without local busy inference", async () => {
    mocks.state = state({
      status: "working",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["approval"],
          blocksById: {
            approval: { id: "approval", type: "approval", kind: "approval", status: "blocked", text: "需要确认", approvalId: "approval-a" },
          },
        },
      },
      pendingApprovals: {
        "approval-a": { id: "approval-a", turnId: "turn-a", status: "pending", reason: "保存修改" },
      },
      runtimeLiveness: {
        activeTurns: { "turn-a": true }, activeAgents: {}, pendingApprovals: { "approval-a": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();
    expect(container.textContent).toContain("正在处理");

    await act(async () => {
      (container.querySelector('[data-testid="workflow-ai-stop"]') as HTMLButtonElement).click();
      (container.querySelector('[data-testid="workflow-approval-accept"]') as HTMLButtonElement).click();
    });
    expect(mocks.cancelRun).toHaveBeenCalledTimes(1);
    expect(mocks.stop).not.toHaveBeenCalled();
    expect(mocks.approvalDecision).toHaveBeenCalledWith("approval-a", "accept");

    mocks.state = state({
      status: "canceled",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: { "turn-a": { id: "turn-a", status: "canceled", blockOrder: [], blocksById: {} } },
    });
    await render();
    expect(container.querySelector('[data-testid="workflow-ai-stop"]')).toBeNull();
    const input = container.querySelector('[data-testid="workflow-ai-input"]') as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(input, "停止后继续下一轮");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect((container.querySelector('[data-testid="workflow-ai-send"]') as HTMLButtonElement).disabled).toBe(false);

    mocks.state = state({ status: "failed", lastError: "500 Internal Server Error", currentTurnId: "turn-failed", turns: {
      "turn-failed": { id: "turn-failed", status: "failed", blockOrder: [], blocksById: {} },
    } });
    await render();
    expect(container.querySelector('[data-testid="workflow-ai-transport-error"]')?.textContent)
      .toContain("服务异常,请稍后重试，画布未发生修改。你可以继续重试。");
    await act(async () => {
      (container.querySelector('[data-testid="workflow-ai-retry"]') as HTMLButtonElement).click();
    });
    expect(mocks.retry).toHaveBeenCalledWith("turn-failed");
  });

  it("renders one approval card when a blocked tool and its explicit approval block coexist", async () => {
    mocks.state = state({
      status: "blocked",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["tool", "approval"],
          blocksById: {
            tool: { id: "tool", type: "tool", kind: "tool", status: "blocked", text: "workflow.apply 等待确认" },
            approval: { id: "approval", type: "approval", kind: "approval", status: "blocked", text: "确认应用", approvalId: "approval-a" },
          },
        },
      },
      pendingApprovals: {
        "approval-a": { id: "approval-a", turnId: "turn-a", status: "pending", reason: "应用当前候选" },
      },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-a": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();

    expect(container.querySelectorAll('[data-testid="workflow-approval-card"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="workflow-approval-card"]')?.closest('[data-transport-block-id]')?.getAttribute("data-transport-block-id"))
      .toBe("approval");
  });

  it("keeps one fallback approval card for a legacy blocked-tool-only turn", async () => {
    mocks.state = state({
      status: "blocked",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["tool"],
          blocksById: {
            tool: { id: "tool", type: "tool", kind: "tool", status: "blocked", text: "workflow.apply 等待确认" },
          },
        },
      },
      pendingApprovals: {
        "approval-a": { id: "approval-a", turnId: "turn-a", status: "pending", reason: "应用当前候选" },
      },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-a": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();

    expect(container.querySelectorAll('[data-testid="workflow-approval-card"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="workflow-approval-card"]')?.closest('[data-transport-block-id]')?.getAttribute("data-transport-block-id"))
      .toBe("tool");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).not.toBeNull();
  });

  it("stops an approval-blocked turn through AssistantTransport and removes live approval actions when canceled", async () => {
    const waiting = state({
      status: "blocked",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["approval"],
          blocksById: {
            approval: { id: "approval", type: "approval", kind: "approval", status: "blocked", text: "等待操作人确认", approvalId: "approval-a" },
          },
        },
      },
      pendingApprovals: {
        "approval-a": { id: "approval-a", turnId: "turn-a", status: "requested", reason: "应用当前候选" },
      },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-a": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    mocks.state = waiting;
    await render();

    expect(container.textContent).toContain("等待你的操作");
    expect(container.querySelector('[data-testid="workflow-ai-stop"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).not.toBeNull();
    await act(async () => (container.querySelector('[data-testid="workflow-ai-stop"]') as HTMLButtonElement).click());
    expect(mocks.cancelRun).toHaveBeenCalledTimes(1);
    expect(mocks.stop).toHaveBeenCalledWith("user canceled pending workflow approval");

    mocks.state = state({
      ...waiting,
      status: "canceled",
      turns: {
        "turn-a": {
          ...waiting.turns["turn-a"],
          status: "canceled",
          blocksById: {
            approval: { ...waiting.turns["turn-a"].blocksById.approval, status: "rejected" },
          },
        },
      },
    });
    await render();

    expect(container.textContent).toContain("已取消");
    expect(container.querySelector('[data-testid="workflow-ai-stop"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-reject"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-card"]')?.getAttribute("data-approval-state")).toBe("canceled");
  });

  it("keeps a resolved approval visible after transport removes it from pending approvals", async () => {
    mocks.state = state({
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "completed", blockOrder: ["approval"], blocksById: {
            approval: { id: "approval", type: "approval", kind: "approval", status: "rejected", text: "用户拒绝本次修改", approvalId: "approval-a" },
          },
        },
      },
      pendingApprovals: {},
    });
    await render();

    expect(container.querySelector('[data-testid="workflow-approval-card"]')?.getAttribute("data-approval-state")).toBe("rejected");
    expect(container.textContent).toContain("已拒绝，本次修改不会应用");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
  });

  it("shows the canonical running approval block as accepted while async resume continues", async () => {
    mocks.state = state({
      status: "working",
      currentTurnId: "turn-a",
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "working", blockOrder: ["approval"], blocksById: {
            approval: { id: "approval", type: "approval", kind: "approval", status: "running", text: "正在应用当前候选", approvalId: "approval-a" },
          },
        },
      },
      pendingApprovals: {},
      runtimeLiveness: {
        activeTurns: { "turn-a": true }, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();

    expect(container.querySelector('[data-testid="workflow-approval-card"]')?.getAttribute("data-approval-state")).toBe("accepted");
    expect(container.textContent).toContain("已确认，正在继续");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
  });

  it("reconciles canonical preview, receipt, and validation facts in ordered blocks only once", async () => {
    const preview = candidateArtifact();
    mocks.state = state({
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "completed",
          workflowContext: {
            workflowId: "workflow-a", revision: revision("rv-1"), selection: { nodeIds: [], edgeIds: [] },
            environment: { bindingVersion: "env-1" },
          },
          blockOrder: ["preview", "preview-replay", "receipt", "validation"],
          blocksById: {
            preview: { id: "preview", type: "artifact", kind: "assistant", status: "completed", artifact: preview },
            "preview-replay": { id: "preview-replay", type: "artifact", kind: "assistant", status: "completed", artifact: structuredClone(preview) },
            receipt: { id: "receipt", type: "artifact", kind: "assistant", status: "completed", artifact: receiptArtifact() },
            validation: { id: "validation", type: "artifact", kind: "assistant", status: "completed", artifact: validationArtifact() },
          },
        },
      },
    });

    await render();
    expect(mocks.workspace.reconcileTransportFact).toHaveBeenCalledTimes(3);
    expect(vi.mocked(mocks.workspace.reconcileTransportFact).mock.calls.map(([fact]) => fact.kind))
      .toEqual(["candidate", "receipt", "validation"]);

    await render();
    expect(mocks.workspace.reconcileTransportFact).toHaveBeenCalledTimes(3);
  });

  it("reconciles a later distinct revision-one Candidate as the active replacement", async () => {
    const first = candidateArtifact([], {
      artifactId: "preview-artifact-retained",
      candidateId: "candidate-retained",
      changeSetId: "change-retained",
      previewHash: "preview-retained",
      label: "retained candidate",
    });
    const replacement = candidateArtifact([], {
      artifactId: "preview-artifact-repreviewed",
      candidateId: "candidate-repreviewed",
      changeSetId: "change-repreviewed",
      previewHash: "preview-repreviewed",
      label: "re-previewed candidate",
    });
    const workflowContext = {
      workflowId: "workflow-a", revision: revision("rv-1"), selection: { nodeIds: ["check"], edgeIds: [] },
      environment: { name: "staging", bindingVersion: "env-1" },
    };
    const firstTurn = {
      id: "turn-retained", status: "completed" as const, workflowContext,
      blockOrder: ["preview-retained"],
      blocksById: {
        "preview-retained": {
          id: "preview-retained", type: "artifact" as const, kind: "assistant" as const,
          status: "completed" as const, artifact: first,
        },
      },
    };

    mocks.state = state({ turnOrder: [firstTurn.id], turns: { [firstTurn.id]: firstTurn } });
    await render();
    expect(mocks.workspace.reconcileTransportFact).toHaveBeenCalledTimes(1);

    const replacementTurn = {
      id: "turn-repreviewed", status: "completed" as const, workflowContext,
      blockOrder: ["preview-repreviewed"],
      blocksById: {
        "preview-repreviewed": {
          id: "preview-repreviewed", type: "artifact" as const, kind: "assistant" as const,
          status: "completed" as const, artifact: replacement,
        },
      },
    };
    mocks.state = state({
      turnOrder: [firstTurn.id, replacementTurn.id],
      turns: { [firstTurn.id]: firstTurn, [replacementTurn.id]: replacementTurn },
      updatedAt: "2026-07-21T08:01:00Z",
    });
    await render();

    const reconciled = vi.mocked(mocks.workspace.reconcileTransportFact).mock.calls.map(([fact]) => fact);
    expect(reconciled).toHaveLength(2);
    expect(reconciled.map((fact) => fact.kind === "candidate" ? {
      candidateId: fact.fact.candidate.id,
      revision: fact.fact.candidate.currentRevision,
      previewHash: fact.fact.changeSet.previewHash,
    } : fact.kind)).toEqual([
      { candidateId: "candidate-retained", revision: 1, previewHash: "preview-retained" },
      { candidateId: "candidate-repreviewed", revision: 1, previewHash: "preview-repreviewed" },
    ]);
  });

  it("routes typed Candidate Apply to the Workspace Dialog and accepts only one exactly bound confirmed event", async () => {
    const onOpenReview = vi.fn<(target: WorkflowAiReviewTarget) => void>();
    const binding = { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" };
    const artifact = candidateArtifact([
      { id: "focus", intent: "workflow.focus_object", label: "定位节点", target: { nodeId: "check" } },
      { id: "legacy-focus", intent: "workflow.focus_object", label: "旧定位", target: { kind: "node", id: "forged" } },
      { id: "review", intent: "workflow.review_change", label: "审阅修改", target: binding },
      { id: "apply", intent: "workflow.apply_candidate", label: "应用修改", target: { ...binding, approvalId: "approval-1" } },
    ]);
    const parsed = parseWorkflowTransportArtifact(artifact, {
      workflowKey,
      committedRevision: revision("rv-1"),
      receivedAt: "2026-07-21T08:00:00Z",
    });
    expect(parsed?.kind).toBe("candidate");
    mocks.workspace.state.candidate = parsed?.kind === "candidate" ? parsed.fact : undefined;
    mocks.state = state({
      status: "blocked", currentTurnId: "turn-a", turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["artifact", "approval"],
          blocksById: {
            artifact: { id: "artifact", type: "artifact", kind: "assistant", status: "completed", artifact },
            approval: { id: "approval", type: "approval", kind: "approval", status: "blocked", approvalId: "approval-1", text: "确认应用" },
          },
        },
      },
      pendingApprovals: { "approval-1": { id: "approval-1", turnId: "turn-a", status: "pending", reason: "应用当前候选" } },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-1": true }, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render(onOpenReview);

    const button = (label: string) => [...container.querySelectorAll("button")].find((item) => item.textContent?.includes(label)) as HTMLButtonElement;
    const focusEvents: unknown[] = [];
    const candidateCommands: unknown[] = [];
    const recordFocus = (event: Event) => focusEvents.push((event as CustomEvent).detail);
    const recordCandidateCommand = (event: Event) => candidateCommands.push((event as CustomEvent).detail);
    window.addEventListener("aiops:workflow-surface-focus", recordFocus);
    window.addEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, recordCandidateCommand);
    await act(async () => {
      button("定位节点").click();
      button("旧定位").click();
      button("审阅修改").click();
      button("应用修改").click();
      button("应用修改").click();
    });
    window.removeEventListener("aiops:workflow-surface-focus", recordFocus);
    window.removeEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, recordCandidateCommand);

    expect(focusEvents).toEqual([expect.objectContaining({
      schemaVersion: "aiops.workflow.surface-focus.v1",
      source: "chat",
      workflowId: "workflow-a",
      target: { nodeId: "check" },
      operationIndex: 0,
      candidate: binding,
    })]);
    expect(mocks.workspace.setSelection).not.toHaveBeenCalled();
    expect(onOpenReview).toHaveBeenCalledWith({ artifactId: "preview-artifact-1", turnId: "turn-a", ...binding });
    expect(candidateCommands).toEqual([
      { command: "apply_candidate", ...binding },
      { command: "apply_candidate", ...binding },
    ]);
    expect(container.textContent).toContain("请在候选修改操作栏完成最终确认");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-reject"]')).toBeNull();
    expect(mocks.approvalDecision).not.toHaveBeenCalled();

    const confirmation: WorkflowCandidateApplyConfirmation = {
      schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
      artifactId: artifact.id,
      actionId: "apply",
      approvalId: "approval-1",
      binding,
      changeCount: 1,
    };
    await act(async () => {
      window.dispatchEvent(new CustomEvent(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, {
        detail: { ...confirmation, binding: { ...binding, previewHash: "forged-preview" } },
      }));
    });
    expect(mocks.approvalDecision).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="workflow-approval-recovery"]')).not.toBeNull();

    await act(async () => {
      window.dispatchEvent(new CustomEvent(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, { detail: confirmation }));
      window.dispatchEvent(new CustomEvent(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, { detail: confirmation }));
    });
    expect(mocks.approvalDecision).toHaveBeenCalledTimes(1);
    expect(mocks.approvalDecision).toHaveBeenCalledWith("approval-1", "accept");
  });

  it("keeps an ambiguous duplicate Candidate Apply approval fail-closed and never falls back to inline accept", async () => {
    const binding = { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" };
    const artifact = candidateArtifact([
      { id: "apply-a", intent: "workflow.apply_candidate", label: "应用修改", target: { ...binding, approvalId: "approval-1" } },
      { id: "apply-b", intent: "workflow.apply_candidate", label: "再次应用", target: { ...binding, approvalId: "approval-1" } },
    ]);
    const parsed = parseWorkflowTransportArtifact(artifact, {
      workflowKey,
      committedRevision: revision("rv-1"),
      receivedAt: "2026-07-21T08:00:00Z",
    });
    expect(parsed?.kind).toBe("candidate");
    mocks.workspace.state.candidate = parsed?.kind === "candidate" ? parsed.fact : undefined;
    mocks.state = state({
      status: "blocked", currentTurnId: "turn-a", turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["artifact", "approval"],
          blocksById: {
            artifact: { id: "artifact", type: "artifact", kind: "assistant", status: "completed", artifact },
            approval: { id: "approval", type: "approval", kind: "approval", status: "blocked", approvalId: "approval-1", text: "确认应用" },
          },
        },
      },
      pendingApprovals: { "approval-1": { id: "approval-1", turnId: "turn-a", status: "pending" } },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-1": true }, pendingUserInputs: {}, activeCommandStreams: {},
      },
    });
    await render();

    expect(container.textContent).toContain("请在候选修改操作栏完成最终确认");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-reject"]')).toBeNull();
    await act(async () => window.dispatchEvent(new CustomEvent(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, {
      detail: {
        schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
        artifactId: artifact.id,
        actionId: "apply-a",
        approvalId: "approval-1",
        binding,
        changeCount: 1,
      } satisfies WorkflowCandidateApplyConfirmation,
    })));
    expect(mocks.approvalDecision).not.toHaveBeenCalled();
  });

  it("locates the exact typed Chat change item when Canvas selects a Candidate ghost", async () => {
    const artifact = candidateArtifact();
    mocks.state = state({
      turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "completed", blockOrder: ["artifact"], blocksById: {
            artifact: { id: "artifact", type: "artifact", kind: "assistant", status: "completed", artifact },
          },
        },
      },
    });
    await render();

    await act(async () => window.dispatchEvent(new CustomEvent("aiops:workflow-surface-focus", { detail: {
      schemaVersion: "aiops.workflow.surface-focus.v1",
      requestId: "canvas-focus-1",
      source: "canvas",
      workflowId: "workflow-a",
      target: { nodeId: "check" },
      candidate: { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" },
      operationIndex: 0,
    } })));

    const focused = container.querySelector('[data-testid="workflow-focused-change"]') as HTMLElement;
    expect(focused).not.toBeNull();
    expect(focused.getAttribute("data-workflow-change-item")).toBe("change-1:0");
    expect(focused.getAttribute("aria-current")).toBe("true");
    expect(focused.textContent).toContain("已定位到第 1 项修改");
  });

  it("fails closed without an exact pending approval and shows one waiting-step recovery hint", async () => {
    const binding = { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1", approvalId: "approval-missing" };
    mocks.state = state({
      status: "blocked", currentTurnId: "turn-a", turnOrder: ["turn-a"],
      turns: {
        "turn-a": {
          id: "turn-a", status: "blocked", blockOrder: ["artifact"],
          blocksById: {
            artifact: { id: "artifact", type: "artifact", kind: "assistant", status: "completed", artifact: candidateArtifact([
              { id: "apply", intent: "workflow.apply_candidate", label: "应用修改", target: binding },
            ]) },
          },
        },
      },
    });
    await render();
    const apply = [...container.querySelectorAll("button")].find((item) => item.textContent?.includes("应用修改")) as HTMLButtonElement;
    await act(async () => {
      apply.click();
      apply.click();
    });

    expect(mocks.approvalDecision).not.toHaveBeenCalled();
    expect(container.querySelectorAll('[data-testid="workflow-approval-recovery"]')).toHaveLength(1);
    expect(container.textContent).toContain("等待确认步骤");
  });

  it("delegates an exactly bound discard to the authoritative Workspace and keeps retry transport-owned", async () => {
    const artifact = candidateArtifact([
      {
        id: "discard", intent: "workflow.discard_candidate", label: "放弃修改",
        target: { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" },
      },
      { id: "retry", intent: "workflow.retry_from_current", label: "重新计算", target: { candidateId: "candidate-1" } },
    ]);
    const parsed = parseWorkflowTransportArtifact(artifact, {
      workflowKey, committedRevision: revision("rv-1"), receivedAt: "2026-07-21T08:00:00Z",
    });
    expect(parsed?.kind).toBe("candidate");
    const retainedCandidate = parsed?.kind === "candidate" ? parsed.fact : undefined;
    mocks.workspace.state.candidate = retainedCandidate;
    mocks.state = state({
      turnOrder: ["turn-a"], turns: {
        "turn-a": { id: "turn-a", status: "failed", blockOrder: ["artifact"], blocksById: {
          artifact: { id: "artifact", type: "artifact", kind: "assistant", status: "completed", artifact },
        } },
      },
    });
    await render();
    const button = (label: string) => [...container.querySelectorAll("button")].find((item) => item.textContent?.includes(label)) as HTMLButtonElement;
    await act(async () => {
      button("放弃修改").click();
      button("重新计算").click();
    });

    expect(mocks.retry).toHaveBeenCalledWith("turn-a");
    expect(mocks.workspace.discardCandidate).toHaveBeenCalledTimes(1);
    expect(mocks.workspace.reconcileTransportFact).toHaveBeenCalledTimes(1);
    expect(mocks.workspace.state.candidate).toBe(retainedCandidate);
  });
});
