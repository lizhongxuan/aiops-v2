import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowApprovalCard } from "./WorkflowApprovalCard";

describe("WorkflowApprovalCard", () => {
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

  it("renders one pending confirmation and emits an exact typed decision", async () => {
    const onDecision = vi.fn();
    await act(async () => root.render(
      <WorkflowApprovalCard
        approval={{ id: "approval-1", turnId: "turn-1", status: "pending", reason: "删除步骤 database", targetSummary: "步骤 database 及 2 条连线", expectedEffect: "保存新版本" }}
        blockText="需要确认"
        onDecision={onDecision}
      />,
    ));
    expect(container.textContent).toContain("需要你确认");
    expect(container.textContent).toContain("步骤 database 及 2 条连线");
    expect(container.textContent).not.toContain("pending");
    await act(async () => (container.querySelector('[data-testid="workflow-approval-accept"]') as HTMLButtonElement).click());
    expect(onDecision).toHaveBeenCalledWith("approval-1", "accept");
  });

  it("keeps a Candidate-owned approval non-interactive because final confirmation belongs to the Dialog", async () => {
    const onDecision = vi.fn();
    await act(async () => root.render(
      <WorkflowApprovalCard
        approval={{ id: "approval-1", turnId: "turn-1", status: "pending", reason: "应用当前候选" }}
        decisionOwner="candidate-dialog"
        onDecision={onDecision}
      />,
    ));
    expect(container.textContent).toContain("请在候选修改操作栏完成最终确认");
    expect(container.querySelector('[data-testid="workflow-approval-accept"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-approval-reject"]')).toBeNull();
    expect(onDecision).not.toHaveBeenCalled();
  });

  it.each([
    ["accepted", "已确认，正在继续"],
    ["rejected", "已拒绝，本次修改不会应用"],
    ["canceled", "已取消，本次修改不会应用"],
    ["expired", "确认已过期，请重新生成待确认修改"],
  ])("renders the %s state as stable Chinese user copy", async (status, copy) => {
    await act(async () => root.render(
      <WorkflowApprovalCard approval={{ id: "approval-1", status }} blockText="协议内容" />,
    ));
    expect(container.textContent).toContain(copy);
    expect(container.querySelectorAll("button")).toHaveLength(0);
    expect(container.textContent).not.toContain(status);
  });
});
