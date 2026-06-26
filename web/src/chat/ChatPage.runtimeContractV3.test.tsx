import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { listHostInventory } from "@/api/hostInventory";
import {
  getChildAgentTranscript,
  submitHostOpsApprovalDecision,
} from "@/api/hostOps";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import { resetAiopsTransportStateCacheForTest } from "@/transport/aiopsTransportStateCache";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";
import { ChatPage } from "./ChatPage";

vi.mock("@/api/hostInventory", () => ({
  listHostInventory: vi.fn(),
}));

vi.mock("@/api/hostOps", () => ({
  getChildAgentTranscript: vi.fn(),
  submitHostOpsApprovalDecision: vi.fn(),
}));

describe("ChatPage runtime contract V3", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    HTMLElement.prototype.scrollTo = function scrollTo() {};
    vi.mocked(listHostInventory).mockResolvedValue([]);
    vi.mocked(getChildAgentTranscript).mockReset();
    vi.mocked(submitHostOpsApprovalDecision).mockReset();
    resetAiopsTransportStateCacheForTest();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    resetAiopsTransportStateCacheForTest();
    vi.restoreAllMocks();
    container.remove();
  });

  it("does not imply server-local binding for an empty chat session", async () => {
    const state = createInitialAiopsTransportState(
      "thread-runtime-contract-v3-empty",
    );
    state.sessionId = "sess-runtime-contract-v3-empty";

    await act(async () => {
      root.render(
        <ChatPage
          initialState={state}
          threadId="thread-runtime-contract-v3-empty"
        />,
      );
    });

    expect(container.querySelector("textarea")).not.toBeNull();
    expect(container.textContent).toContain("Hello there");
    expect(container.textContent).not.toContain("要对 server-local 做什么？");
    expect(container.textContent).not.toContain("当前主机");
  });

  it("renders runtime contract V3 timeline markers from transport state", async () => {
    const state = runtimeContractV3State();

    await act(async () => {
      root.render(
        <ChatPage
          initialState={state}
          threadId="thread-runtime-contract-v3"
        />,
      );
    });

    await expandProcessTranscripts(container);

    const text = container.textContent || "";
    for (const marker of REQUIRED_RUNTIME_CONTRACT_V3_MARKERS) {
      expect(text).toContain(marker);
    }

    expect(
      container.querySelector('[data-testid="codex-approval-inline"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="codex-approval-command"]')
        ?.textContent,
    ).toContain("runtime-contract-v3 approval pause marker");
    expect(container.querySelector("textarea")).toBeNull();

    expect(
      container.querySelector('[data-testid="host-ops-status-panel"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="host-child-agent-child-alpha"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="host-child-agent-child-beta"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="host-child-agent-child-gamma"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="context-status-notice"]'),
    ).not.toBeNull();
  });
});

const REQUIRED_RUNTIME_CONTRACT_V3_MARKERS = [
  "approval pause marker",
  "approval denied continuation marker",
  "multi-host child agent timeline marker",
  "context compacted marker",
  "pending input accepted / steer marker",
  "turn cancelled / aborted tool marker",
  "resource lock conflict marker",
];

async function expandProcessTranscripts(container: HTMLElement) {
  const headers = Array.from(
    container.querySelectorAll<HTMLButtonElement>(
      '[data-testid="aiops-process-header"]',
    ),
  );

  await act(async () => {
    for (const header of headers) {
      if (header.getAttribute("aria-expanded") !== "true") {
        header.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      }
    }
  });
}

function runtimeContractV3State(): AiopsTransportState {
  const state = createInitialAiopsTransportState(
    "thread-runtime-contract-v3",
  );
  return {
    ...state,
    sessionId: "sess-runtime-contract-v3",
    status: "blocked",
    currentTurnId: "turn-approval-pause",
    turnOrder: [
      "turn-approval-denied",
      "turn-context-compacted",
      "turn-pending-input",
      "turn-cancelled",
      "turn-resource-lock",
      "turn-approval-pause",
    ],
    turns: {
      "turn-approval-denied": {
        id: "turn-approval-denied",
        status: "completed",
        startedAt: "2026-06-24T01:00:00Z",
        completedAt: "2026-06-24T01:00:08Z",
        user: {
          id: "user-approval-denied",
          text: "Reject the risky restart and continue with read-only evidence.",
          createdAt: "2026-06-24T01:00:00Z",
        },
        process: [
          {
            id: "approval-denied-audit",
            kind: "approval",
            status: "rejected",
            text: "approval denied continuation marker",
            command: "systemctl restart postgresql",
            approvalId: "approval-denied-v3",
          },
        ],
        final: {
          id: "final-approval-denied",
          status: "completed",
          text: "approval denied continuation marker: continued with read-only diagnostics after operator rejection.",
        },
      },
      "turn-context-compacted": {
        id: "turn-context-compacted",
        status: "completed",
        startedAt: "2026-06-24T01:01:00Z",
        completedAt: "2026-06-24T01:01:04Z",
        user: {
          id: "user-context-compacted",
          text: "Continue the long RCA after context compaction.",
          createdAt: "2026-06-24T01:01:00Z",
        },
        contextGovernance: [
          {
            id: "ctx-v3-compacted",
            layer: "L4",
            kind: "context.compaction.completed",
            message:
              "context compacted marker: retained current task, approvals, evidence refs, and child-agent status.",
            referenceIds: ["evidence:pg-timeline", "approval:restart"],
          },
        ],
        final: {
          id: "final-context-compacted",
          status: "completed",
          text: "Context compaction completed and the turn continued.",
        },
      },
      "turn-pending-input": {
        id: "turn-pending-input",
        status: "completed",
        startedAt: "2026-06-24T01:02:00Z",
        completedAt: "2026-06-24T01:02:03Z",
        user: {
          id: "user-pending-input",
          text: "While the turn is running, also inspect inode pressure.",
          createdAt: "2026-06-24T01:02:00Z",
        },
        process: [
          {
            id: "pending-input-steer",
            kind: "system",
            displayKind: "pending_input.accepted",
            status: "completed",
            text: "pending input accepted / steer marker: queued follow-up input into the active regular turn instead of creating a second turn.",
          },
        ],
        final: {
          id: "final-pending-input",
          status: "completed",
          text: "The queued steer was merged into the running turn.",
        },
      },
      "turn-cancelled": {
        id: "turn-cancelled",
        status: "canceled",
        startedAt: "2026-06-24T01:03:00Z",
        completedAt: "2026-06-24T01:03:05Z",
        user: {
          id: "user-cancelled",
          text: "Cancel the current operation.",
          createdAt: "2026-06-24T01:03:00Z",
        },
        process: [
          {
            id: "aborted-tool-marker",
            kind: "tool",
            displayKind: "aiops.tool_aborted/v1",
            status: "rejected",
            text: "turn cancelled / aborted tool marker: active tool call aborted and partial execution risk preserved.",
            outputPreview:
              "partialExecutionRisk=unknown; no completed mutation should be inferred.",
          },
        ],
        final: {
          id: "final-cancelled",
          status: "failed",
          text: "The turn was cancelled by the operator.",
        },
      },
      "turn-resource-lock": {
        id: "turn-resource-lock",
        status: "completed",
        startedAt: "2026-06-24T01:04:00Z",
        completedAt: "2026-06-24T01:04:04Z",
        user: {
          id: "user-resource-lock",
          text: "Try a conflicting service mutation.",
          createdAt: "2026-06-24T01:04:00Z",
        },
        process: [
          {
            id: "resource-lock-conflict",
            kind: "tool",
            displayKind: "resource_lock.conflict",
            status: "failed",
            text: "resource lock conflict marker: mutation denied because service/postgresql is already locked by turn-active.",
            outputPreview:
              "holder=turn-active; scope=host:db-1 service:postgresql; execution_skipped=true",
          },
        ],
        final: {
          id: "final-resource-lock",
          status: "completed",
          text: "The conflicting mutation was not executed.",
        },
      },
      "turn-approval-pause": {
        id: "turn-approval-pause",
        status: "blocked",
        startedAt: "2026-06-24T01:05:00Z",
        updatedAt: "2026-06-24T01:05:02Z",
        user: {
          id: "user-approval-pause",
          text: "@db-alpha @db-beta @db-gamma restart PostgreSQL only after approval.",
          createdAt: "2026-06-24T01:05:00Z",
        },
        process: [
          {
            id: "manager-child-agent-timeline",
            kind: "subagent",
            displayKind: "host_manager.timeline",
            status: "completed",
            text: "multi-host child agent timeline marker: manager spawned alpha, beta, and gamma child agents with per-host status.",
          },
          {
            id: "approval-pause-block",
            kind: "approval",
            status: "blocked",
            text: "approval pause marker: waiting for operator approval before mutation.",
            command: "runtime-contract-v3 approval pause marker: systemctl restart postgresql",
            approvalId: "approval-pause-v3",
          },
        ],
        final: {
          id: "final-approval-pause",
          status: "running",
          text: "Waiting for approval before executing the restart.",
        },
      },
    },
    pendingApprovals: {
      "approval-pause-v3": {
        id: "approval-pause-v3",
        turnId: "turn-approval-pause",
        type: "command",
        status: "blocked",
        command:
          "runtime-contract-v3 approval pause marker: systemctl restart postgresql",
        requestedAt: "2026-06-24T01:05:01Z",
      },
    },
    runtimeLiveness: {
      activeTurns: { "turn-approval-pause": true },
      activeAgents: {
        "child-alpha": true,
        "child-beta": true,
        "child-gamma": true,
      },
      pendingApprovals: { "approval-pause-v3": true },
      pendingUserInputs: { "pending-input-v3": true },
      activeCommandStreams: {},
    },
    activeHostMissionId: "mission-runtime-contract-v3",
    hostMissions: {
      "mission-runtime-contract-v3": {
        id: "mission-runtime-contract-v3",
        turnId: "turn-approval-pause",
        status: "waiting_approval",
        planRequired: true,
        planAccepted: true,
        mentionedHosts: [
          {
            tokenId: "mention-alpha",
            raw: "@db-alpha",
            hostId: "db-alpha",
            address: "10.0.0.11",
            displayName: "alpha",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-beta",
            raw: "@db-beta",
            hostId: "db-beta",
            address: "10.0.0.12",
            displayName: "beta",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-gamma",
            raw: "@db-gamma",
            hostId: "db-gamma",
            address: "10.0.0.13",
            displayName: "gamma",
            source: "inventory",
            resolved: true,
          },
        ],
        childAgentIds: ["child-alpha", "child-beta", "child-gamma"],
        planSteps: [
          {
            id: "plan-child-timeline",
            text: "multi-host child agent timeline marker: collect per-host evidence before mutation.",
            status: "completed",
            childAgentIds: ["child-alpha", "child-beta", "child-gamma"],
            approvalRequired: true,
          },
        ],
      },
    },
    childAgents: {
      "child-alpha": {
        id: "child-alpha",
        missionId: "mission-runtime-contract-v3",
        sessionId: "sess-child-alpha",
        hostId: "db-alpha",
        hostAddress: "10.0.0.11",
        hostDisplayName: "alpha",
        status: "completed",
        task: "collect PostgreSQL role and timeline evidence",
        currentStepTitle: "evidence captured",
      },
      "child-beta": {
        id: "child-beta",
        missionId: "mission-runtime-contract-v3",
        sessionId: "sess-child-beta",
        hostId: "db-beta",
        hostAddress: "10.0.0.12",
        hostDisplayName: "beta",
        status: "running",
        task: "compare replica lag",
        currentStepTitle: "checking replication lag",
      },
      "child-gamma": {
        id: "child-gamma",
        missionId: "mission-runtime-contract-v3",
        sessionId: "sess-child-gamma",
        hostId: "db-gamma",
        hostAddress: "10.0.0.13",
        hostDisplayName: "gamma",
        status: "approval_required",
        task: "wait for coordinated restart approval",
        currentStepTitle: "waiting for approval pause",
      },
    },
  };
}
