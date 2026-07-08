import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowAiPermissionDialog } from "./WorkflowAiPermissionDialog";

describe("WorkflowAiPermissionDialog", () => {
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

  it("shows semantic confirmation and rejects on Escape", async () => {
    const onConfirm = vi.fn();
    const onReject = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiPermissionDialog
          open
          patch={{ id: "patch", summary: "Rename", operations: [{ op: "update_node" }] }}
          onConfirm={onConfirm}
          onReject={onReject}
        />,
      );
    });

    const text = container.textContent || "";
    for (const label of ["修改", "影响", "风险", "校验", "可撤销"]) {
      expect(text).toContain(label);
    }
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    });
    expect(onReject).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
