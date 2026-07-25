import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkflowChangeSetV2 } from "./workspace/workflowMcpContracts";
import { WorkflowChangeReview, type WorkflowReviewFieldDiff } from "./WorkflowChangeReview";

const changeSet: WorkflowChangeSetV2 = {
  schemaVersion: "aiops.workflow.edit.v2",
  id: "change-7",
  workflowKey: { tenantId: "tenant-a", workflowId: "workflow-a" },
  candidateId: "candidate-7",
  candidateRevision: 7,
  actor: "agent",
  intent: "更新超时并移除废弃路径",
  base: { resourceVersion: "rv-12", semanticHash: "semantic-12", layoutHash: "layout-12" },
  operations: [
    { op: "delete_node", nodeId: "node-deprecated", cascadeEdges: true },
    { op: "update_node", nodeId: "node-check", set: { step: { timeout: 30 } } },
    { op: "delete_edge", edgeId: "edge-fallback" },
  ],
  previewHash: "preview-7",
  validation: {
    basis: { resourceVersion: "rv-12", semanticHash: "semantic-12", catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" },
    valid: false,
    diagnostics: [
      { code: "NODE_WARNING", severity: "warning", message: "检查超时设置", nodeId: "node-check", field: "step.timeout" },
      { code: "EDGE_BLOCKED", severity: "error", message: "provider-secret-do-not-render", edgeId: "edge-fallback" },
    ],
  },
  state: "previewed",
  createdAt: "2026-07-21T00:00:00Z",
  expiresAt: "2026-07-21T01:00:00Z",
};

const fieldDiffs: WorkflowReviewFieldDiff[] = [{
  operationIndex: 1,
  path: "/nodes/node-check/step/timeout",
  before: 10,
  after: 30,
}];

describe("WorkflowChangeReview", () => {
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

  it("renders the typed operation trail, unapplied status and exact high-risk delete targets", async () => {
    await act(async () => root.render(<WorkflowChangeReview changeSet={changeSet} fieldDiffs={fieldDiffs} />));

    expect(container.textContent).toContain("尚未应用");
    expect(container.querySelector('[data-testid="workflow-change-summary"]')?.textContent).toContain("修改 1");
    expect(container.querySelector('[data-testid="workflow-change-summary"]')?.textContent).toContain("删除 2");
    expect(container.querySelector('[data-testid="workflow-change-summary"]')?.textContent).toContain("问题 2");
    expect(container.textContent).toContain("1. 删除节点 node-deprecated");
    expect(container.textContent).toContain("2. 更新节点 node-check");
    expect(container.textContent).toContain("3. 删除连线 edge-fallback");
    expect(container.textContent).toContain("高风险");
    expect(container.textContent).toContain("将删除节点 node-deprecated");
    expect(container.textContent).toContain("将删除连线 edge-fallback");
    expect(container.textContent).not.toContain("删除若干对象");
  });

  it("navigates previous/next operations and focuses the exact node or edge", async () => {
    const onFocusNode = vi.fn();
    const onFocusEdge = vi.fn();
    await act(async () => root.render(
      <WorkflowChangeReview changeSet={changeSet} fieldDiffs={fieldDiffs} onFocusNode={onFocusNode} onFocusEdge={onFocusEdge} />,
    ));

    const next = container.querySelector('[data-testid="workflow-review-next"]') as HTMLButtonElement;
    await act(async () => next.click());
    expect(container.textContent).toContain("/nodes/node-check/step/timeout");
    expect(container.textContent).toContain("10");
    expect(container.textContent).toContain("30");
    expect(onFocusNode).toHaveBeenLastCalledWith("node-check");

    await act(async () => next.click());
    expect(onFocusEdge).toHaveBeenLastCalledWith("edge-fallback");
    expect((container.querySelector('[data-testid="workflow-review-next"]') as HTMLButtonElement).disabled).toBe(true);

    const previous = container.querySelector('[data-testid="workflow-review-previous"]') as HTMLButtonElement;
    await act(async () => previous.click());
    expect(onFocusNode).toHaveBeenLastCalledWith("node-check");
  });

  it("focuses the stable first payload node for a multi-node layout operation", async () => {
    const onFocusNode = vi.fn();
    const layoutChangeSet: WorkflowChangeSetV2 = {
      ...changeSet,
      operations: [
        changeSet.operations[1],
        {
          op: "update_layout",
          layout: {
            nodes: [
              { nodeId: "node-layout-primary", position: { x: 120, y: 80 } },
              { nodeId: "node-layout-secondary", position: { x: 360, y: 80 } },
            ],
          },
        },
      ],
    };
    const layoutDiffs: WorkflowReviewFieldDiff[] = [
      { operationIndex: 1, path: "/nodes/node-layout-primary/position/x", before: 100, after: 120 },
      { operationIndex: 1, path: "/nodes/node-layout-secondary/position/x", before: 340, after: 360 },
    ];
    await act(async () => root.render(
      <WorkflowChangeReview changeSet={layoutChangeSet} fieldDiffs={layoutDiffs} onFocusNode={onFocusNode} />,
    ));

    await act(async () => (container.querySelector('[data-testid="workflow-review-next"]') as HTMLButtonElement).click());

    expect(onFocusNode).toHaveBeenLastCalledWith("node-layout-primary");
    expect(container.textContent).toContain("/nodes/node-layout-primary/position/x");
  });

  it("shows validation risk and lets diagnostics focus their typed object", async () => {
    const onFocusNode = vi.fn();
    const onFocusEdge = vi.fn();
    await act(async () => root.render(
      <WorkflowChangeReview changeSet={changeSet} onFocusNode={onFocusNode} onFocusEdge={onFocusEdge} />,
    ));

    expect(container.textContent).toContain("检查未通过");
    expect(container.textContent).toContain("NODE_WARNING");
    expect(container.textContent).toContain("EDGE_BLOCKED");
    expect(container.textContent).not.toContain("provider-secret-do-not-render");
    const diagnostics = container.querySelectorAll<HTMLButtonElement>("[data-workflow-diagnostic]");
    await act(async () => diagnostics[0].click());
    await act(async () => diagnostics[1].click());
    expect(onFocusNode).toHaveBeenCalledWith("node-check");
    expect(onFocusEdge).toHaveBeenCalledWith("edge-fallback");
  });

  it("redacts sensitive field values while preserving a secretRef identifier", async () => {
    const sensitiveDiffs: WorkflowReviewFieldDiff[] = [
      { operationIndex: 1, path: "/nodes/node-check/step/password", before: "old-secret", after: "new-secret" },
      { operationIndex: 1, path: "/nodes/node-check/step/secretRef", before: "secret://workflow/old", after: "secret://workflow/new" },
    ];
    await act(async () => root.render(<WorkflowChangeReview changeSet={changeSet} fieldDiffs={sensitiveDiffs} initialOperationIndex={1} />));
    expect(container.textContent).not.toContain("old-secret");
    expect(container.textContent).not.toContain("new-secret");
    expect(container.textContent).toContain("••••••");
    expect(container.textContent).toContain("secret://workflow/new");
  });

  it("separates pending review from durable saved changes without adding candidate actions", async () => {
    await act(async () => root.render(
      <WorkflowChangeReview
        changeSet={changeSet}
        savedChanges={<div data-testid="durable-history-slot">服务端持久修改</div>}
      />,
    ));

    expect([...container.querySelectorAll('[role="tab"]')].map((tab) => tab.textContent)).toEqual([
      "待审阅 3",
      "已保存修改",
    ]);
    expect(container.querySelector('[data-testid="workflow-candidate-apply"]')).toBeNull();
    await act(async () => (container.querySelector('[data-testid="workflow-changes-tab-saved"]') as HTMLButtonElement).click());
    expect(container.querySelector('[data-testid="durable-history-slot"]')).not.toBeNull();
    expect(container.textContent).not.toContain("当前会话");
  });
});
