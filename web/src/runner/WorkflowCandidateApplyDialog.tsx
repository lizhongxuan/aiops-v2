import { ShieldCheck } from "lucide-react";
import { useEffect, useRef, useState, type RefObject } from "react";

import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { WorkflowCandidateActionBinding } from "@/runner/WorkflowCandidateActionBar";

export const WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT = "aiops:workflow-candidate-apply-confirmed";

export type WorkflowCandidateApplyConfirmation = {
  schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1";
  artifactId: string;
  actionId: string;
  approvalId: string;
  binding: WorkflowCandidateActionBinding;
  changeCount: number;
};

export type WorkflowCandidateApplyDialogProps = {
  confirmation: WorkflowCandidateApplyConfirmation;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (confirmation: WorkflowCandidateApplyConfirmation) => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
};

export function WorkflowCandidateApplyDialog({
  confirmation,
  open,
  onOpenChange,
  onConfirm,
  returnFocusRef,
}: WorkflowCandidateApplyDialogProps) {
  const confirmRef = useRef<HTMLButtonElement>(null);
  const submittedRef = useRef(false);
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    if (!open) return;
    submittedRef.current = false;
    setSubmitted(false);
  }, [open, confirmation]);

  const handleOpenChange = (nextOpen: boolean) => {
    onOpenChange(nextOpen);
    if (!nextOpen && returnFocusRef?.current) {
      // Radix fires close autofocus while the Workspace owner can still be
      // inert. Waiting for the next frame lets React commit the closed state
      // before focus returns to the Apply trigger.
      window.requestAnimationFrame(() => {
        returnFocusRef.current?.focus({ preventScroll: true });
      });
    }
  };

  const confirm = () => {
    if (submittedRef.current) return;
    submittedRef.current = true;
    setSubmitted(true);
    onConfirm(confirmation);
    handleOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-md"
        showCloseButton={false}
        data-testid="workflow-candidate-apply-dialog"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          confirmRef.current?.focus();
        }}
        onEscapeKeyDown={(event) => {
          // The Workflow drawer also listens for Escape on document. Consume
          // the innermost modal Escape here so one key press closes only this
          // Dialog and leaves its Apply owner mounted for focus restoration.
          event.preventDefault();
          event.stopPropagation();
          handleOpenChange(false);
        }}
        onCloseAutoFocus={(event) => {
          if (returnFocusRef?.current) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>确认应用 {confirmation.changeCount} 项修改</DialogTitle>
          <DialogDescription>
            系统会再次核对候选版本、当前画布版本和本次确认。确认前不会修改已保存工作流。
          </DialogDescription>
        </DialogHeader>
        <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3 text-sm leading-6 text-slate-700">
          <span className="inline-flex items-center gap-2 font-medium text-slate-950">
            <ShieldCheck className="h-4 w-4" aria-hidden="true" />
            最终确认
          </span>
          <p className="mt-1">只有当前待审阅版本会被提交；过期或已变化的候选将安全拒绝。</p>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <button
              type="button"
              className="inline-flex min-h-11 items-center justify-center rounded-lg border border-slate-300 bg-white px-4 text-sm font-medium text-slate-800 hover:bg-slate-50"
            >
              取消
            </button>
          </DialogClose>
          <button
            ref={confirmRef}
            type="button"
            data-testid="workflow-candidate-confirm-apply"
            disabled={submitted}
            className="inline-flex min-h-11 items-center justify-center rounded-lg bg-slate-950 px-4 text-sm font-medium text-white disabled:bg-slate-300"
            onClick={confirm}
          >
            {submitted ? "正在确认" : "确认应用"}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function dispatchWorkflowCandidateApplyConfirmation(detail: WorkflowCandidateApplyConfirmation) {
  window.dispatchEvent(new CustomEvent<WorkflowCandidateApplyConfirmation>(
    WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT,
    { detail },
  ));
}

export function readWorkflowCandidateApplyConfirmation(event: Event): WorkflowCandidateApplyConfirmation | undefined {
  const detail = (event as CustomEvent<unknown>).detail;
  if (!detail || typeof detail !== "object" || Array.isArray(detail)) return undefined;
  const value = detail as Record<string, unknown>;
  const bindingValue = value.binding;
  if (value.schemaVersion !== "aiops.workflow.candidate-apply-confirmation.v1"
    || !canonicalString(value.artifactId)
    || !canonicalString(value.actionId)
    || !canonicalString(value.approvalId)
    || !Number.isSafeInteger(value.changeCount)
    || Number(value.changeCount) <= 0
    || !bindingValue
    || typeof bindingValue !== "object"
    || Array.isArray(bindingValue)) {
    return undefined;
  }
  const binding = bindingValue as Record<string, unknown>;
  if (!canonicalString(binding.candidateId)
    || !canonicalString(binding.changeSetId)
    || !canonicalString(binding.previewHash)
    || !canonicalString(binding.expectedRevision)) {
    return undefined;
  }
  return {
    schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
    artifactId: value.artifactId,
    actionId: value.actionId,
    approvalId: value.approvalId,
    binding: {
      candidateId: binding.candidateId,
      changeSetId: binding.changeSetId,
      previewHash: binding.previewHash,
      expectedRevision: binding.expectedRevision,
    },
    changeCount: Number(value.changeCount),
  };
}

function canonicalString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value === value.trim();
}
