import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostAgentChecklistSection } from "./HostAgentChecklistSection";

describe("HostAgentChecklistSection", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("renders host agent rows with host, status, and current step", async () => {
    const openChildAgent = vi.fn();
    const state = sampleState();

    await act(async () => {
      root.render(
        <HostAgentChecklistSection
          mission={state.hostMissions["mission-1"]}
          state={state}
          onOpenChildAgent={openChildAgent}
        />,
      );
    });

    expect(container.textContent).toContain("共 2 个主机 Agent");
    expect(container.textContent).toContain("1. Franklin(@1.1.1.1)");
    expect(container.textContent).toContain("执行主机 A 准备步骤");
    expect(container.textContent).toContain("运行中");
    expect(container.textContent).toContain("2. Harriet(@1.1.1.2)");
    expect(container.textContent).toContain("等待审批");

    const row = container.querySelector('[data-testid="host-subagent-status-row-child-2"]') as HTMLButtonElement;
    await act(async () => {
      row.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(openChildAgent).toHaveBeenCalledWith("child-2");
  });

  it("collapses to only the host agent summary", async () => {
    const state = sampleState();

    await act(async () => {
      root.render(
        <HostAgentChecklistSection mission={state.hostMissions["mission-1"]} state={state} defaultCollapsed />,
      );
    });

    expect(container.textContent?.trim()).toBe("主机 Agent共 2 个主机 Agent");
    expect(container.textContent).not.toContain("Franklin");
    expect(container.querySelector('[aria-expanded="false"]')).not.toBeNull();
  });
});

function sampleState(): AiopsTransportState {
  return {
    ...createInitialAiopsTransportState("thread-host-agent-checklist"),
    hostMissions: {
      "mission-1": sampleMission(),
    },
    childAgents: {
      "child-1": {
        id: "child-1",
        missionId: "mission-1",
        sessionId: "session-child-1",
        hostId: "host-1",
        hostAddress: "1.1.1.1",
        hostDisplayName: "Franklin",
        status: "running",
        task: "执行主机 A 准备步骤",
      },
      "child-2": {
        id: "child-2",
        missionId: "mission-1",
        sessionId: "session-child-2",
        hostId: "host-2",
        hostAddress: "1.1.1.2",
        hostDisplayName: "Harriet",
        status: "approval_required",
        task: "执行主机 B 配置步骤",
      },
    },
  };
}

function sampleMission(): AiopsTransportHostMission {
  return {
    id: "mission-1",
    turnId: "turn-1",
    status: "running",
    planRequired: true,
    planAccepted: true,
    mentionedHosts: [],
    childAgentIds: ["child-1", "child-2"],
  };
}
