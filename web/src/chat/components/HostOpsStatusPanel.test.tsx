import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostOpsStatusPanel } from "./HostOpsStatusPanel";

describe("HostOpsStatusPanel", () => {
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

  it("renders Codex-style compact plan and subagent rows above composer", async () => {
    const openChildAgent = vi.fn();

    await act(async () => {
      root.render(<HostOpsStatusPanel state={sampleHostOpsState()} onOpenChildAgent={openChildAgent} />);
    });

    expect(container.querySelector('[data-testid="host-ops-status-panel"]')).not.toBeNull();
    expect(container.textContent).toContain("共 5 个任务，已经完成 0 个");
    expect(container.textContent).toContain("3 个后台智能体");
    expect(container.textContent).toContain("Franklin(@1.1.1.1)");
    expect(container.textContent).toContain("打开");

    const openButtons = Array.from(container.querySelectorAll("button")).filter((button) =>
      button.textContent?.includes("打开"),
    );
    expect(openButtons).toHaveLength(3);

    await act(async () => {
      openButtons[0].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(openChildAgent).toHaveBeenCalledWith("child-1");
  });

  it("ignores legacy transport states without host operation maps", async () => {
    const legacyState = createInitialAiopsTransportState("legacy-thread") as Partial<AiopsTransportState>;
    delete legacyState.hostMissions;
    delete legacyState.childAgents;

    await act(async () => {
      root.render(<HostOpsStatusPanel state={legacyState as AiopsTransportState} />);
    });

    expect(container.querySelector('[data-testid="host-ops-status-panel"]')).toBeNull();
  });
});

function sampleHostOpsState(): AiopsTransportState {
  return {
    ...createInitialAiopsTransportState("thread-hostops-panel"),
    activeHostMissionId: "mission-1",
    hostMissions: {
      "mission-1": {
        id: "mission-1",
        turnId: "turn-1",
        status: "running",
        planRequired: true,
        planAccepted: true,
        mentionedHosts: [
          {
            tokenId: "mention-1",
            raw: "@1.1.1.1",
            hostId: "host-1",
            address: "1.1.1.1",
            displayName: "Franklin",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-2",
            raw: "@1.1.1.2",
            hostId: "host-2",
            address: "1.1.1.2",
            displayName: "Harriet",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-3",
            raw: "@1.1.1.3",
            hostId: "host-3",
            address: "1.1.1.3",
            displayName: "Grace",
            source: "inventory",
            resolved: true,
          },
        ],
        childAgentIds: ["child-1", "child-2", "child-3"],
        planSteps: [
          { id: "step-1", title: "确认 PostgreSQL 拓扑", status: "pending" },
          { id: "step-2", title: "初始化主库", status: "pending" },
          { id: "step-3", title: "配置从库复制", status: "pending" },
          { id: "step-4", title: "部署监控节点", status: "pending" },
          { id: "step-5", title: "执行最终验证", status: "pending" },
        ],
      },
    } as AiopsTransportState["hostMissions"],
    childAgents: {
      "child-1": {
        id: "child-1",
        missionId: "mission-1",
        sessionId: "session-child-1",
        hostId: "host-1",
        hostAddress: "1.1.1.1",
        hostDisplayName: "Franklin",
        status: "running",
        task: "初始化主库",
      },
      "child-2": {
        id: "child-2",
        missionId: "mission-1",
        sessionId: "session-child-2",
        hostId: "host-2",
        hostAddress: "1.1.1.2",
        hostDisplayName: "Harriet",
        status: "running",
        task: "配置从库复制",
      },
      "child-3": {
        id: "child-3",
        missionId: "mission-1",
        sessionId: "session-child-3",
        hostId: "host-3",
        hostAddress: "1.1.1.3",
        hostDisplayName: "Grace",
        status: "waiting",
        task: "部署监控节点",
      },
    },
  };
}
