import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApprovalBlockPart } from "./ApprovalBlockPart";

const approvalDecision = vi.fn();

vi.mock("@/transport/useAiopsTransportCommands", () => ({
  useAiopsTransportCommands: () => ({
    approvalDecision,
  }),
}));

describe("ApprovalBlockPart", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    approvalDecision.mockReset();
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

  it("shows approval target and risk details without changing approval commands", async () => {
    await act(async () => {
      root.render(
        <ApprovalBlockPart
          block={{
            id: "approval-block-1",
            kind: "approval",
            status: "blocked",
            text: "需要确认后执行命令",
            command: "systemctl restart postgresql",
            approvalId: "approval-1",
            source: "ai_chat_direct",
            targetSummary: "host:pg-a；service:postgresql",
            risk: "high",
            riskSummary: "风险等级：high；会重启数据库服务",
            expectedEffect: "重启 PostgreSQL 服务",
            rollback: "如失败则恢复原服务状态并停止后续动作",
            validation: "确认服务 active 并检查 5432 端口",
          }}
        />,
      );
    });

    expect(container.textContent).toContain("目标");
    expect(container.textContent).toContain("host:pg-a；service:postgresql");
    expect(container.textContent).toContain("风险等级：high");
    expect(container.textContent).toContain("AI Chat");
    expect(container.textContent).toContain("重启 PostgreSQL 服务");
    expect(container.textContent).toContain("恢复原服务状态");
    expect(container.textContent).toContain("确认服务 active 并检查 5432 端口");

    const approveButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("Approve"),
    );
    await act(async () => {
      approveButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(approvalDecision).toHaveBeenCalledWith("approval-1", "accept");
  });
});
