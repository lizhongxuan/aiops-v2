import { describe, expect, it, vi } from "vitest";

import type { WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";
import {
  createWorkflowSurfaceFocusDetail,
  dispatchWorkflowSurfaceFocus,
  readWorkflowSurfaceFocusDetail,
  resolveWorkflowOperationIndex,
  workflowOperationTarget,
  WORKFLOW_SURFACE_FOCUS_EVENT,
} from "./workflowSurfaceFocus";

const operations: WorkflowOperation[] = [
  { op: "update_node", nodeId: "check", set: { label: "容量检查" } },
  { op: "update_edge", edgeId: "edge-check-alert", set: { condition: "failed" } },
  { op: "update_node", nodeId: "check", set: { label: "最终检查" } },
];

describe("workflow surface focus contract", () => {
  it("uses an exact typed operation index and falls back to the first matching object", () => {
    expect(resolveWorkflowOperationIndex(operations, { nodeId: "check" }, 2)).toBe(2);
    expect(resolveWorkflowOperationIndex(operations, { nodeId: "check" }, 1)).toBe(0);
    expect(resolveWorkflowOperationIndex(operations, { edgeId: "edge-check-alert" })).toBe(1);
    expect(workflowOperationTarget(operations[1])).toEqual({ edgeId: "edge-check-alert" });
  });

  it("dispatches and accepts only the versioned canonical event shape", () => {
    const received = vi.fn();
    window.addEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, received);
    const detail = createWorkflowSurfaceFocusDetail({
      requestId: "focus-1",
      source: "canvas",
      workflowId: "workflow-a",
      target: { nodeId: "check" },
      candidate: { candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" },
      operationIndex: 2,
    });
    dispatchWorkflowSurfaceFocus(detail);
    window.removeEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, received);

    expect(received).toHaveBeenCalledTimes(1);
    expect(readWorkflowSurfaceFocusDetail(received.mock.calls[0][0])).toEqual(detail);
    expect(readWorkflowSurfaceFocusDetail(new CustomEvent(WORKFLOW_SURFACE_FOCUS_EVENT, { detail: {
      ...detail,
      target: { nodeId: "check", edgeId: "edge-check-alert" },
    } }))).toBeUndefined();
    expect(readWorkflowSurfaceFocusDetail(new CustomEvent(WORKFLOW_SURFACE_FOCUS_EVENT, { detail: {
      ...detail,
      candidate: { candidateId: "candidate-1" },
    } }))).toBeUndefined();
  });
});
