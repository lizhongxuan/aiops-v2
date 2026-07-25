import { describe, expect, it, vi } from "vitest";

import type { WorkflowPrimaryActionViewModel } from "./workspace/workflowInteractionContract";
import {
  createWorkflowStudioActionRegistry,
  WorkflowStudioActionUnavailableError,
} from "./workflowStudioActionRegistry";

const publishedAction: WorkflowPrimaryActionViewModel = {
  id: "run_published",
  label: "运行已发布版本",
  owner: "command_bar",
  enabled: true,
  reason: undefined,
  capability: "workflow.run_published",
  target: {
    kind: "published_revision",
    workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
    resourceVersion: "rv-published",
    semanticHash: "sha256:published",
  },
};

describe("workflowStudioActionRegistry", () => {
  it("registers an action only when capability and handler both exist", () => {
    const handler = vi.fn();
    expect(createWorkflowStudioActionRegistry({
      capabilities: { "workflow.run_published": false },
      handlers: { run_published: handler },
    }).canExecute(publishedAction)).toBe(false);
    expect(createWorkflowStudioActionRegistry({
      capabilities: { "workflow.run_published": true },
      handlers: {},
    }).canExecute(publishedAction)).toBe(false);
    expect(createWorkflowStudioActionRegistry({
      capabilities: { "workflow.run_published": true },
      handlers: { run_published: handler },
    }).canExecute(publishedAction)).toBe(true);
  });

  it("fails closed instead of falling back to the legacy graph runner", async () => {
    const registry = createWorkflowStudioActionRegistry({
      capabilities: { "workflow.run_published": true },
      handlers: {},
    });
    await expect(registry.execute(publishedAction)).rejects.toBeInstanceOf(WorkflowStudioActionUnavailableError);
  });

  it("deduplicates the same in-flight action across responsive presenters", async () => {
    let release!: () => void;
    const pending = new Promise<void>((resolve) => { release = resolve; });
    const handler = vi.fn(() => pending);
    const registry = createWorkflowStudioActionRegistry({
      capabilities: { "workflow.run_published": true },
      handlers: { run_published: handler },
    });

    const desktop = registry.execute(publishedAction);
    const mobile = registry.execute(publishedAction);
    expect(handler).toHaveBeenCalledTimes(1);
    release();
    await expect(Promise.all([desktop, mobile])).resolves.toEqual([undefined, undefined]);
  });

  it("keeps Undo and Redo bound to one exact transaction target", async () => {
    const undo = vi.fn();
    const registry = createWorkflowStudioActionRegistry({
      capabilities: { "workflow.undo": true },
      handlers: { undo },
    });
    const target = {
      kind: "history_transaction" as const,
      workflowId: "workflow-a",
      action: "undo" as const,
      targetTransactionId: "txn-17",
      expected: {
        resourceVersion: "rv-17",
        semanticHash: "semantic-17",
        layoutHash: "layout-17",
      },
      idempotencyKey: "undo-txn-17-rv-17",
    };
    await registry.execute({
      id: "undo",
      label: "撤销",
      owner: "command_bar",
      enabled: true,
      reason: undefined,
      capability: "workflow.undo",
      target,
    });
    expect(undo).toHaveBeenCalledWith(target);
  });
});
