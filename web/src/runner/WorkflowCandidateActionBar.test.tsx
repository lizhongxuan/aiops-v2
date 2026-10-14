import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WorkflowCandidateActionBar,
  type WorkflowCandidateActionBinding,
  type WorkflowCandidateApprovalFact,
} from "./WorkflowCandidateActionBar";

const binding: WorkflowCandidateActionBinding = {
  candidateId: "candidate-7",
  changeSetId: "change-7",
  previewHash: "preview-7",
  expectedRevision: "rv-12",
};

const approval = (status: WorkflowCandidateApprovalFact["status"], overrides: Partial<WorkflowCandidateApprovalFact> = {}): WorkflowCandidateApprovalFact => ({
  status,
  binding,
  approvalId: "approval-7",
  expiresAt: "2030-07-21T00:00:00Z",
  ...overrides,
});

describe("WorkflowCandidateActionBar", () => {
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

  it("binds Apply to all four candidate identity fields and prevents double submission while pending", async () => {
    let resolveApply: (() => void) | undefined;
    const onApply = vi.fn(() => new Promise<void>((resolve) => { resolveApply = resolve; }));
    await act(async () => root.render(
      <WorkflowCandidateActionBar binding={binding} approval={approval("requested")} changeCount={3} onApply={onApply} />,
    ));
    const button = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    await act(async () => {
      button.click();
      button.click();
    });
    expect(onApply).toHaveBeenCalledTimes(1);
    expect(onApply).toHaveBeenCalledWith({ ...binding, approvalId: "approval-7" });
    expect(button.disabled).toBe(true);
    expect(container.textContent).toContain("正在应用 3 项修改");
    await act(async () => resolveApply?.());
  });

  it("marks a pending decision and disables only the duplicate Apply entry point", async () => {
    const onApply = vi.fn();
    const onDiscard = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateActionBar binding={binding} approval={approval("decision_pending")} changeCount={2} onApply={onApply} onDiscard={onDiscard} />,
    ));
    expect(container.textContent).toContain("确认已提交，正在等待服务端审批结果");
    const apply = container.querySelector('[data-testid="workflow-candidate-apply"]') as HTMLButtonElement;
    const discard = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("放弃修改")) as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    expect(apply.textContent).toContain("审批中");
    expect(discard.disabled).toBe(false);
    await act(async () => discard.click());
    expect(onDiscard).toHaveBeenCalledWith(binding);
    expect(onApply).not.toHaveBeenCalled();
  });

  it.each(["expired", "revoked", "conflict"] as const)("offers one recovery action for %s", async (status) => {
    const onRecover = vi.fn();
    const onApply = vi.fn();
    const onDiscard = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateActionBar binding={binding} approval={approval(status)} changeCount={2} onApply={onApply} onDiscard={onDiscard} onRecover={onRecover} />,
    ));
    const actions = container.querySelectorAll("button");
    expect(actions).toHaveLength(1);
    expect(actions[0].textContent).toContain("恢复待确认修改");
    await act(async () => (actions[0] as HTMLButtonElement).click());
    expect(onRecover).toHaveBeenCalledWith({ reason: status, binding });
    expect(onApply).not.toHaveBeenCalled();
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it("fails closed on mismatched approval identity and exposes only recovery", async () => {
    const onRecover = vi.fn();
    const onApply = vi.fn();
    const mismatched = approval("approved", { binding: { ...binding, previewHash: "older-preview" } });
    await act(async () => root.render(
      <WorkflowCandidateActionBar binding={binding} approval={mismatched} changeCount={1} onApply={onApply} onRecover={onRecover} />,
    ));
    expect(container.textContent).toContain("确认信息与当前修改不一致");
    expect(container.querySelectorAll("button")).toHaveLength(1);
    await act(async () => (container.querySelector("button") as HTMLButtonElement).click());
    expect(onRecover).toHaveBeenCalledWith({ reason: "conflict", binding });
    expect(onApply).not.toHaveBeenCalled();
  });

  it("fails closed when an approval-required state has no server approval id", async () => {
    const onApply = vi.fn();
    const onRecover = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateActionBar
        binding={binding}
        approval={approval("requested", { approvalId: undefined })}
        changeCount={1}
        onApply={onApply}
        onRecover={onRecover}
      />,
    ));
    expect(container.textContent).toContain("确认信息与当前修改不一致");
    expect(container.querySelector('[data-testid="workflow-candidate-apply"]')).toBeNull();
  });

  it("derives expiry from the server deadline and does not apply an expired approval", async () => {
    const onRecover = vi.fn();
    const onApply = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateActionBar
        binding={binding}
        approval={approval("requested", { expiresAt: "2026-07-21T00:00:00Z" })}
        changeCount={1}
        now={() => new Date("2026-07-21T00:00:01Z")}
        onApply={onApply}
        onRecover={onRecover}
      />,
    ));
    expect(container.textContent).toContain("确认已过期");
    expect(container.querySelector('[data-testid="workflow-candidate-apply"]')).toBeNull();
  });

  it("queries an existing receipt after a lost Apply response and never replays Apply", async () => {
    const onApply = vi.fn();
    const onQueryReceipt = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateActionBar
        binding={binding}
        approval={approval("apply_response_lost", { receiptQuery: { idempotencyKey: "apply-7", transactionId: "txn-7" } })}
        changeCount={2}
        onApply={onApply}
        onQueryReceipt={onQueryReceipt}
      />,
    ));
    expect(container.textContent).toContain("保存响应未确认");
    expect(container.textContent).toContain("只查询已有回执，不会再次应用");
    expect(container.querySelector('[data-testid="workflow-candidate-apply"]')).toBeNull();
    const query = container.querySelector('[data-testid="workflow-candidate-query-receipt"]') as HTMLButtonElement;
    await act(async () => query.click());
    expect(onQueryReceipt).toHaveBeenCalledWith({ binding, idempotencyKey: "apply-7", transactionId: "txn-7" });
    expect(onApply).not.toHaveBeenCalled();
  });
});
