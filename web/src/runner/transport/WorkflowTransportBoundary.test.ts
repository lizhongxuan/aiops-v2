import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

import {
  emitWorkflowTransportResume,
  readWorkflowTransportSession,
  readWorkflowTransportResumeEvent,
  WORKFLOW_TRANSPORT_RESUME_EVENT,
  WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
  WorkflowTransportBoundary,
  workflowTransportIdentity,
  writeWorkflowTransportSession,
} from "./WorkflowTransportBoundary";

const mocks = vi.hoisted(() => ({
  resumeRun: vi.fn(),
  state: {} as AiopsTransportState,
}));

vi.mock("@assistant-ui/react", () => ({
  useAssistantApi: () => ({ thread: () => ({ unstable_resumeRun: mocks.resumeRun }) }),
  useAssistantTransportState: () => mocks.state,
}));

vi.mock("@/transport/ChatTransportProvider", () => ({
  ChatTransportProvider: ({ children }: { children?: ReactNode }) => children,
}));

vi.mock("./WorkflowTransportActivityContext", () => ({
  WorkflowTransportActivityProvider: ({ children }: { children?: ReactNode }) => children,
}));

describe("WorkflowTransportBoundary helpers", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    mocks.resumeRun.mockReset();
    mocks.state = transportState();
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    window.sessionStorage.clear();
  });

  it("isolates transport cache, thread, and resume pointer by workflow", () => {
    expect(workflowTransportIdentity("checkout flow")).toEqual({
      threadId: "workflow:checkout flow",
      cacheScope: "workflow:checkout flow",
      storageKey: "aiops.workflow.transport.session.checkout%20flow",
    });
  });

  it("persists only the opaque session id needed by AssistantTransport resume", () => {
    const key = workflowTransportIdentity("workflow-a").storageKey;
    writeWorkflowTransportSession(key, " session-a ");
    expect(readWorkflowTransportSession(key)).toBe("session-a");
    expect(window.sessionStorage.length).toBe(1);
  });

  it("resumes only the exact workflow, thread, and existing AssistantTransport session", async () => {
    await act(async () => {
      root.render(createElement(WorkflowTransportBoundary, { workflowId: "workflow-a" }, createElement("div", null, "workspace")));
    });
    const exact = {
      schemaVersion: WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
      workflowId: "workflow-a",
      threadId: "workflow:workflow-a",
      sessionId: "session-a",
      turnId: "turn-approval-a",
      approvalId: "approval-a",
    } as const;

    await act(async () => emitWorkflowTransportResume(exact));
    expect(mocks.resumeRun).toHaveBeenCalledTimes(1);
    expect(mocks.resumeRun).toHaveBeenCalledWith({});

    for (const detail of [
      { ...exact, workflowId: "workflow-b" },
      { ...exact, threadId: "workflow:workflow-b" },
      { ...exact, sessionId: "session-b" },
      { ...exact, turnId: " turn-approval-a" },
    ]) {
      await act(async () => window.dispatchEvent(new CustomEvent(WORKFLOW_TRANSPORT_RESUME_EVENT, { detail })));
    }
    expect(mocks.resumeRun).toHaveBeenCalledTimes(1);
  });

  it("rejects expanded resume events and carries no approval decision or ActionToken", () => {
    const exact = {
      schemaVersion: WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
      workflowId: "workflow-a",
      threadId: "workflow:workflow-a",
      sessionId: "session-a",
      turnId: "turn-approval-a",
      approvalId: "approval-a",
    } as const;
    const event = new CustomEvent(WORKFLOW_TRANSPORT_RESUME_EVENT, { detail: exact });
    expect(readWorkflowTransportResumeEvent(event)).toEqual(exact);
    expect(readWorkflowTransportResumeEvent(new CustomEvent(WORKFLOW_TRANSPORT_RESUME_EVENT, {
      detail: { ...exact, actionToken: "forged" },
    }))).toBeUndefined();
    expect(JSON.stringify(exact)).not.toMatch(/actionToken|decision|apply_patch/);
  });
});

function transportState(): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "session-a",
    threadId: "workflow:workflow-a",
    status: "idle",
    turns: {}, turnOrder: [], pendingApprovals: {}, mcpSurfaces: {}, artifacts: {},
    runtimeLiveness: {
      activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {},
    },
    hostMissions: {}, childAgents: {}, seq: 1, updatedAt: "2026-07-22T00:00:00Z",
  };
}
