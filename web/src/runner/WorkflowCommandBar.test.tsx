import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  WorkflowLifecycleTraceViewModel,
  WorkflowPrimaryActionViewModel,
} from "./workspace/workflowInteractionContract";
import { WorkflowCommandBar } from "./WorkflowCommandBar";

const trace: WorkflowLifecycleTraceViewModel = {
  compactLabel: "待检查",
  ariaLabel: "保存：已保存；检查：待检查；草稿试运行：未运行；发布：未发布",
  stages: [
    { id: "save", label: "保存", state: "complete", description: "已保存" },
    { id: "check", label: "检查", state: "current", description: "待检查" },
    { id: "draft_run", label: "草稿试运行", state: "pending", description: "未运行" },
    { id: "publish", label: "发布", state: "pending", description: "未发布" },
  ],
};

const primary: WorkflowPrimaryActionViewModel = {
  id: "check",
  label: "检查工作流",
  owner: "command_bar",
  enabled: true,
  reason: undefined,
  capability: "workflow.validate",
  target: {
    kind: "saved_draft",
    workflowId: "workflow-a",
    resourceVersion: "rv-1",
    semanticHash: "sha256:rv-1",
  },
};

describe("WorkflowCommandBar", () => {
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

  it("keeps identity, one trace and one registered Primary on one command surface", async () => {
    const onPrimary = vi.fn();
    await act(async () => root.render(
      <WorkflowCommandBar
        title="8-redis"
        lifecycleLabel="草稿"
        trace={trace}
        primaryAction={primary}
        onPrimaryAction={onPrimary}
        onOpenWorkbench={vi.fn()}
      />,
    ));

    expect(container.querySelector('[data-testid="workflow-command-bar"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="workflow-lifecycle-trace"]')).toHaveLength(1);
    expect(container.querySelectorAll('[data-testid="workflow-primary-action"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="workflow-workbench-trigger"]')).not.toBeNull();
    expect([...container.querySelectorAll("button")].some((button) => button.textContent?.trim() === "保存")).toBe(false);

    await act(async () => (container.querySelector('[data-testid="workflow-primary-action"]') as HTMLButtonElement).click());
    expect(onPrimary).toHaveBeenCalledWith(primary);
  });

  it("renders no Primary for a Candidate-owned action", async () => {
    await act(async () => root.render(
      <WorkflowCommandBar
        title="8-redis"
        lifecycleLabel="草稿"
        trace={trace}
        primaryAction={{ ...primary, id: "apply_changes", label: "应用 1 项修改", owner: "candidate_workspace_action_bar", capability: "workflow.apply_candidate", target: { kind: "candidate", workflowId: "workflow-a", candidateId: "candidate-1", changeSetId: "change-1", previewHash: "preview-1", expectedRevision: "rv-1" } }}
        onPrimaryAction={vi.fn()}
      />,
    ));
    expect(container.querySelector('[data-testid="workflow-primary-action"]')).toBeNull();
  });
});
