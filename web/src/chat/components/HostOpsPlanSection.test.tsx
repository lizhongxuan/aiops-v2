import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportHostMission } from "@/transport/aiopsTransportTypes";

import { HostOpsPlanSection } from "./HostOpsPlanSection";

describe("HostOpsPlanSection", () => {
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

  it("renders every plan step through the checklist card", async () => {
    await act(async () => {
      root.render(<HostOpsPlanSection mission={sampleMission()} state={createInitialAiopsTransportState("thread")} />);
    });

    expect(container.textContent).toContain("共 3 个步骤，已经完成 1 个");
    expect(container.textContent).toContain("1. 确认主机拓扑");
    expect(container.textContent).toContain("2. 执行主机 A 准备步骤");
    expect(container.textContent).toContain("3. 验证主机 B 状态");
  });

  it("collapses to only the required plan summary", async () => {
    await act(async () => {
      root.render(
        <HostOpsPlanSection
          mission={sampleMission()}
          state={createInitialAiopsTransportState("thread")}
          defaultCollapsed
        />,
      );
    });

    expect(container.textContent?.trim()).toBe("计划共 3 个步骤，已经完成 1 个");
    expect(container.textContent).not.toContain("确认主机拓扑");
  });

  it("opens step details outside the checklist row", async () => {
    await act(async () => {
      root.render(<HostOpsPlanSection mission={sampleMission()} state={createInitialAiopsTransportState("thread")} />);
    });

    const step = container.querySelector('[data-testid="task-checklist-item-step-2"]') as HTMLButtonElement;
    await act(async () => {
      step.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const details = container.querySelector('[data-testid="host-plan-step-detail"]');
    const row = container.querySelector('[data-testid="task-checklist-item-step-2"]');

    expect(details?.textContent).toContain("执行主机 A 准备步骤");
    expect(details?.textContent).toContain("风险：medium");
    expect(details?.textContent).toContain("主机：host-1");
    expect(row?.textContent).not.toContain("风险：medium");
  });
});

function sampleMission(): AiopsTransportHostMission {
  return {
    id: "mission-1",
    turnId: "turn-1",
    status: "running",
    planRequired: true,
    planAccepted: true,
    mentionedHosts: [],
    childAgentIds: [],
    planSteps: [
      { id: "step-1", index: 1, text: "确认主机拓扑", status: "completed", risk: "read_only", hostIds: ["host-1"] },
      { id: "step-2", index: 2, text: "执行主机 A 准备步骤", status: "running", risk: "medium", hostIds: ["host-1"] },
      { id: "step-3", index: 3, text: "验证主机 B 状态", status: "pending", risk: "read_only", hostIds: ["host-2"] },
    ],
  };
}
