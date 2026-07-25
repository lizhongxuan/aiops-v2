import { act, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WorkflowCandidateApplyDialog,
  type WorkflowCandidateApplyConfirmation,
} from "./WorkflowCandidateApplyDialog";

const confirmation: WorkflowCandidateApplyConfirmation = {
  schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
  artifactId: "artifact-1",
  actionId: "apply-1",
  approvalId: "approval-1",
  binding: {
    candidateId: "candidate-1",
    changeSetId: "change-1",
    previewHash: "preview-1",
    expectedRevision: "rv-1",
  },
  changeCount: 2,
};

describe("WorkflowCandidateApplyDialog", () => {
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
    document.body.querySelectorAll('[data-slot="dialog-portal"]').forEach((node) => node.remove());
    vi.restoreAllMocks();
  });

  it("owns final confirmation, closes on Escape, and restores focus to the Candidate owner", async () => {
    function Harness() {
      const [open, setOpen] = useState(true);
      const ownerRef = useRef<HTMLButtonElement>(null);
      return (
        <>
          <button ref={ownerRef} type="button">应用候选修改</button>
          <WorkflowCandidateApplyDialog
            confirmation={confirmation}
            open={open}
            onOpenChange={setOpen}
            onConfirm={vi.fn()}
            returnFocusRef={ownerRef}
          />
        </>
      );
    }

    await act(async () => root.render(<Harness />));
    const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
    expect(dialog).not.toBeNull();
    expect(dialog.textContent).toContain("确认应用 2 项修改");
    expect(dialog.contains(document.activeElement)).toBe(true);
    const outerDrawerEscape = vi.fn();
    const closeOuterDrawer = (event: KeyboardEvent) => {
      if (event.key === "Escape") outerDrawerEscape();
    };
    document.addEventListener("keydown", closeOuterDrawer);

    await act(async () => {
      dialog.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      await Promise.resolve();
    });

    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    expect(outerDrawerEscape).not.toHaveBeenCalled();
    expect(document.activeElement?.textContent).toBe("应用候选修改");
    document.removeEventListener("keydown", closeOuterDrawer);
  });

  it("emits the frozen typed confirmation only once under a double click", async () => {
    const onConfirm = vi.fn();
    await act(async () => root.render(
      <WorkflowCandidateApplyDialog
        confirmation={confirmation}
        open
        onOpenChange={vi.fn()}
        onConfirm={onConfirm}
      />,
    ));
    const confirm = document.body.querySelector('[data-testid="workflow-candidate-confirm-apply"]') as HTMLButtonElement;
    await act(async () => {
      confirm.click();
      confirm.click();
    });
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledWith(confirmation);
  });

  it("restores focus after the explicit Cancel close has committed and removed owner inertness", async () => {
    function Harness() {
      const [open, setOpen] = useState(true);
      const ownerRef = useRef<HTMLButtonElement>(null);
      return (
        <>
          <div inert={open}>
            <button ref={ownerRef} type="button" data-testid="candidate-owner">应用候选修改</button>
          </div>
          <WorkflowCandidateApplyDialog
            confirmation={confirmation}
            open={open}
            onOpenChange={setOpen}
            onConfirm={vi.fn()}
            returnFocusRef={ownerRef}
          />
        </>
      );
    }

    await act(async () => root.render(<Harness />));
    const callbacks: FrameRequestCallback[] = [];
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callbacks.push(callback);
      return callbacks.length;
    });
    const owner = container.querySelector('[data-testid="candidate-owner"]') as HTMLButtonElement;
    const cancel = [...document.body.querySelectorAll("button")]
      .find((button) => button.textContent === "取消") as HTMLButtonElement;

    await act(async () => cancel.click());

    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).not.toBe(owner);
    expect(callbacks.length).toBeGreaterThan(0);

    await act(async () => {
      callbacks.splice(0).forEach((callback) => callback(performance.now()));
    });
    expect(document.activeElement).toBe(owner);
  });
});
