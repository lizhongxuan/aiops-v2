import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowHistoryPanel, type WorkflowDurableHistoryEntry } from "./WorkflowHistoryPanel";

const entry = (overrides: Partial<WorkflowDurableHistoryEntry> = {}): WorkflowDurableHistoryEntry => ({
  source: "durable_transaction", transactionId: "txn-1", businessLabel: "调整容量检查阈值", status: "completed",
  occurredAt: "2026-07-21T08:00:00Z", actorLabel: "AI 助手", touchedObjectCount: 2, reversible: true,
  ...overrides,
});

describe("WorkflowHistoryPanel", () => {
  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => { globalThis.IS_REACT_ACT_ENVIRONMENT = true; container = document.createElement("div"); document.body.appendChild(container); root = createRoot(container); });
  afterEach(() => { act(() => root.unmount()); container.remove(); });

  it("renders compact durable transactions and sends exact Undo/Redo transaction identities", async () => {
    const onUndo = vi.fn();
    const onRedo = vi.fn();
    await act(async () => root.render(<WorkflowHistoryPanel entries={[entry(), entry({ transactionId: "txn-2", businessLabel: "恢复失败分支", redoable: true, reversible: false })]} onUndo={onUndo} onRedo={onRedo} />));
    expect(container.textContent).toContain("只显示服务端持久事务");
    expect(container.querySelectorAll('[data-testid="workflow-history-row"]')).toHaveLength(2);
    expect(container.querySelector('li[data-transaction-id="txn-1"]')).not.toBeNull();
    expect(container.querySelector('li[data-transaction-id="txn-2"]')).not.toBeNull();
    const undo = [...container.querySelectorAll("button")].find((button) => button.textContent?.includes("撤销")) as HTMLButtonElement;
    const txn2 = container.querySelector('[data-transaction-key="txn-2"]') as HTMLElement;
    await act(async () => (txn2.querySelector("summary") as HTMLElement).click());
    const redo = [...txn2.querySelectorAll("button")].find((button) => button.textContent?.includes("重做")) as HTMLButtonElement;
    await act(async () => undo.click());
    await act(async () => redo.click());
    expect(onUndo).toHaveBeenCalledWith("txn-1");
    expect(onRedo).toHaveBeenCalledWith("txn-2");
  });

  it("shows a three-way recovery path for conflicts", async () => {
    const onResolveConflict = vi.fn();
    await act(async () => root.render(<WorkflowHistoryPanel entries={[entry({ status: "conflict", conflict: { baseLabel: "版本 17", currentLabel: "版本 19", candidateLabel: "撤销事务 1" } })]} onResolveConflict={onResolveConflict} />));
    expect(container.textContent).toContain("原始版本");
    expect(container.textContent).toContain("当前版本");
    expect(container.textContent).toContain("待恢复修改");
    const review = [...container.querySelectorAll("button")].find((button) => button.textContent?.includes("审阅三方差异")) as HTMLButtonElement;
    await act(async () => review.click());
    expect(onResolveConflict).toHaveBeenCalledWith("txn-1", "review_three_way");
  });

  it("shows pending and retryable error feedback while preventing duplicate history actions", async () => {
    const onUndo = vi.fn();
    const onDismissError = vi.fn();
    await act(async () => root.render(
      <WorkflowHistoryPanel
        entries={[entry()]}
        pending={{ action: "undo", targetTransactionId: "txn-1" }}
        error={{
          action: "undo",
          targetTransactionId: "txn-1",
          message: "当前版本已变化",
          retryHint: "刷新后重试",
        }}
        onUndo={onUndo}
        onDismissError={onDismissError}
      />,
    ));

    expect(container.querySelector("section")?.getAttribute("aria-busy")).toBe("true");
    expect(container.querySelector('[data-testid="workflow-history-pending"]')?.textContent).toContain("正在准备撤销候选");
    expect(container.querySelector('[data-testid="workflow-history-error"]')?.textContent).toContain("当前版本已变化");
    expect(container.querySelector('[data-testid="workflow-history-error"]')?.textContent).toContain("已保存的工作流未发生修改");
    const undo = [...container.querySelectorAll("button")].find((button) => button.textContent?.includes("撤销")) as HTMLButtonElement;
    expect(undo.disabled).toBe(true);
    await act(async () => undo.click());
    expect(onUndo).not.toHaveBeenCalled();
    await act(async () => (container.querySelector('button[aria-label="关闭事务操作错误"]') as HTMLButtonElement).click());
    expect(onDismissError).toHaveBeenCalledTimes(1);
  });

  it("does not render invalid in-memory-like entries as durable history", async () => {
    await act(async () => root.render(<WorkflowHistoryPanel entries={[entry({ transactionId: "", occurredAt: "not-a-date" })]} />));
    expect(container.textContent).toContain("暂无已确认的持久事务");
  });

  it("keeps one scroll owner and only promotes the newest reversible action across 100 records", async () => {
    const entries = Array.from({ length: 100 }, (_, index) => entry({
      transactionId: `txn-${index}`,
      businessLabel: `更新工作流 ${index}`,
      occurredAt: new Date(Date.UTC(2026, 6, 21, 8, index)).toISOString(),
    })).reverse();
    await act(async () => root.render(<WorkflowHistoryPanel entries={entries} onUndo={vi.fn()} />));

    expect(container.querySelectorAll('[data-testid="workflow-history-row"]')).toHaveLength(100);
    expect(container.querySelectorAll('[data-testid="workflow-history-primary-undo"]')).toHaveLength(1);
    expect(container.querySelectorAll('[data-testid="workflow-history-scroll"]')).toHaveLength(1);
    expect(container.textContent).not.toContain("当前会话");
  });
});
