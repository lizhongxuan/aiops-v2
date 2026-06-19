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
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("mx-auto");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("w-[calc(100%-4rem)]");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("max-w-[44.5rem]");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("-mb-8");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("rounded-b-none");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).toContain("border-b-0");
    expect(container.querySelector('[data-testid="host-ops-status-panel"]')?.className).not.toContain("shadow-[");
    expect(container.textContent).toContain("共 5 个步骤，已经完成 0 个");
    expect(container.textContent).toContain("共 3 个主机 Agent");
    expect(container.textContent).toContain("@1.1.1.1");
    expect(container.textContent).not.toContain("Franklin(@1.1.1.1)");

    expect(container.textContent).not.toContain("打开");
    expect(container.querySelector('[data-testid="host-subagent-list-card"]')?.className).toContain("px-4");
    expect(container.querySelector('[data-testid="host-subagent-list-card"]')?.className).toContain("py-1.5");
    expect(container.querySelector('[data-testid="host-subagent-list-card"]')?.className).toContain("text-[12px]");

    await act(async () => {
      const hostName = container.querySelector('[data-testid="host-child-agent-name-child-1"]') as HTMLButtonElement;
      hostName.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(openChildAgent).toHaveBeenCalledWith("child-1");
  });

  it("collapses host-bound child agent rows and keeps each row in one line", async () => {
    await act(async () => {
      root.render(<HostOpsStatusPanel state={sampleHostOpsState()} onOpenChildAgent={vi.fn()} />);
    });

    const toggle = container.querySelector('[data-testid="host-subagent-row-toggle"]') as HTMLButtonElement;
    expect(toggle).not.toBeNull();
    expect(toggle.getAttribute("aria-expanded")).toBe("true");

    const firstRow = container.querySelector('[data-testid="host-child-agent-child-1"]');
    expect(firstRow?.className).toContain("flex");
    expect(firstRow?.className).toContain("min-h-6");
    expect(firstRow?.textContent).toContain("@1.1.1.1");
    expect(firstRow?.textContent).toContain("执行主机 A 准备步骤");
    expect(firstRow?.textContent).toContain("running");

    await act(async () => {
      toggle.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector('[data-testid="host-child-agent-child-1"]')).toBeNull();
    expect(container.textContent).toContain("主机 Agent");
    expect(container.textContent).toContain("共 3 个主机 Agent");
  });

  it("colors adjacent host names differently and opens details by clicking the host name", async () => {
    const openChildAgent = vi.fn();

    await act(async () => {
      root.render(<HostOpsStatusPanel state={sampleTenHostOpsState()} onOpenChildAgent={openChildAgent} />);
    });

    const hostNameButtons = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[data-testid^="host-child-agent-name-child-"]'),
    );
    expect(hostNameButtons).toHaveLength(10);
    expect(new Set(hostNameButtons.map((button) => button.className)).size).toBe(10);
    expect(hostNameButtons.every((button) => !button.className.includes("text-zinc-900"))).toBe(true);
    expect(container.textContent).not.toContain("打开");

    await act(async () => {
      hostNameButtons[0].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(openChildAgent).toHaveBeenCalledWith("child-1");
  });

  it("colors host agent status badges by status", async () => {
    await act(async () => {
      root.render(<HostOpsStatusPanel state={sampleStatusHostOpsState()} onOpenChildAgent={vi.fn()} />);
    });

    const badges = Array.from(container.querySelectorAll<HTMLElement>('[data-testid^="host-child-agent-status-"]'));
    expect(badges).toHaveLength(5);
    expect(new Set(badges.map((badge) => badge.className)).size).toBe(5);
    expect(container.querySelector('[data-testid="host-child-agent-status-child-running"]')?.className).toContain("text-sky-700");
    expect(container.querySelector('[data-testid="host-child-agent-status-child-waiting"]')?.className).toContain("text-amber-700");
    expect(container.querySelector('[data-testid="host-child-agent-status-child-completed"]')?.className).toContain("text-emerald-700");
    expect(container.querySelector('[data-testid="host-child-agent-status-child-failed"]')?.className).toContain("text-red-700");
    expect(container.querySelector('[data-testid="host-child-agent-status-child-approval"]')?.className).toContain("text-violet-700");
  });

  it("renders one host-bound child agent status item per mission host", async () => {
    await act(async () => {
      root.render(
        <HostOpsStatusPanel
          state={{
            ...createInitialAiopsTransportState("thread-hostops-child-rows"),
            hostMissions: {
              "mission-1": {
                id: "mission-1",
                status: "running",
                mentions: [
                  { hostId: "host-a", displayName: "主机A", resolved: true },
                  { hostId: "host-b", displayName: "主机B", resolved: true },
                ],
                childAgentIds: ["child-host-a", "child-host-b"],
              },
            },
            activeHostMissionId: "mission-1",
            childAgents: {
              "child-host-a": {
                id: "child-host-a",
                hostId: "host-a",
                hostDisplayName: "主机A",
                status: "running",
              },
              "child-host-b": {
                id: "child-host-b",
                hostId: "host-b",
                hostDisplayName: "主机B",
                status: "waiting",
              },
            },
          } as AiopsTransportState}
        />,
      );
    });

    expect(container.textContent).toContain("主机A");
    expect(container.textContent).toContain("主机B");
    expect(container.querySelector('[data-testid="host-child-agent-child-host-a"]')?.textContent).toContain("running");
    expect(container.querySelector('[data-testid="host-child-agent-child-host-b"]')?.textContent).toContain("waiting");
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

  it("does not render an empty plan-only panel", async () => {
    const state = sampleHostOpsState();
    const mission = state.hostMissions["mission-1"];
    mission.planSteps = [];
    mission.mentionedHosts = [];
    mission.childAgentIds = [];
    state.childAgents = {};

    await act(async () => {
      root.render(<HostOpsStatusPanel state={state} />);
    });

    expect(container.querySelector('[data-testid="host-ops-status-panel"]')).toBeNull();
    expect(container.textContent).not.toContain("共 0 个步骤");
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
          { id: "step-1", title: "确认目标环境", status: "pending" },
          { id: "step-2", title: "执行主机 A 准备步骤", status: "pending" },
          { id: "step-3", title: "执行主机 B 配置步骤", status: "pending" },
          { id: "step-4", title: "执行辅助节点检查", status: "pending" },
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
        task: "执行主机 A 准备步骤",
      },
      "child-2": {
        id: "child-2",
        missionId: "mission-1",
        sessionId: "session-child-2",
        hostId: "host-2",
        hostAddress: "1.1.1.2",
        hostDisplayName: "Harriet",
        status: "running",
        task: "执行主机 B 配置步骤",
      },
      "child-3": {
        id: "child-3",
        missionId: "mission-1",
        sessionId: "session-child-3",
        hostId: "host-3",
        hostAddress: "1.1.1.3",
        hostDisplayName: "Grace",
        status: "waiting",
        task: "执行辅助节点检查",
      },
    },
  };
}

function sampleTenHostOpsState(): AiopsTransportState {
  const base = sampleHostOpsState();
  const mentionedHosts = Array.from({ length: 10 }, (_, index) => {
    const hostNumber = index + 1;
    return {
      tokenId: `mention-${hostNumber}`,
      raw: `@1.1.1.${hostNumber}`,
      hostId: `host-${hostNumber}`,
      address: `1.1.1.${hostNumber}`,
      displayName: `Host ${hostNumber}`,
      source: "inventory",
      resolved: true,
    };
  });
  const childAgents = Object.fromEntries(
    mentionedHosts.map((host, index) => {
      const hostNumber = index + 1;
      return [
        `child-${hostNumber}`,
        {
          id: `child-${hostNumber}`,
          missionId: "mission-1",
          hostId: host.hostId,
          hostAddress: host.address,
          hostDisplayName: host.displayName,
          status: "running",
          task: `执行主机 ${hostNumber} 步骤`,
        },
      ];
    }),
  );
  return {
    ...base,
    hostMissions: {
      "mission-1": {
        ...base.hostMissions["mission-1"],
        mentionedHosts,
        childAgentIds: mentionedHosts.map((_, index) => `child-${index + 1}`),
      },
    } as AiopsTransportState["hostMissions"],
    childAgents: childAgents as AiopsTransportState["childAgents"],
  };
}

function sampleStatusHostOpsState(): AiopsTransportState {
  const statuses = [
    ["child-running", "running"],
    ["child-waiting", "waiting"],
    ["child-completed", "completed"],
    ["child-failed", "failed"],
    ["child-approval", "approval_required"],
  ] as const;
  const base = sampleHostOpsState();
  return {
    ...base,
    hostMissions: {
      "mission-1": {
        ...base.hostMissions["mission-1"],
        mentionedHosts: statuses.map(([id], index) => ({
          tokenId: `mention-${id}`,
          raw: `@10.0.0.${index + 1}`,
          hostId: `host-${id}`,
          address: `10.0.0.${index + 1}`,
          displayName: id,
          source: "inventory",
          resolved: true,
        })),
        childAgentIds: statuses.map(([id]) => id),
      },
    } as AiopsTransportState["hostMissions"],
    childAgents: Object.fromEntries(
      statuses.map(([id, status], index) => [
        id,
        {
          id,
          missionId: "mission-1",
          hostId: `host-${id}`,
          hostAddress: `10.0.0.${index + 1}`,
          hostDisplayName: id,
          status,
          task: `${status} 状态验证`,
        },
      ]),
    ) as AiopsTransportState["childAgents"],
  };
}
