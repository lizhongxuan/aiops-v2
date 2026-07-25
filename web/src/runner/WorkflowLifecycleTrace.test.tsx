import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { WorkflowLifecycleTraceViewModel } from "./workspace/workflowInteractionContract";
import { WorkflowLifecycleTrace } from "./WorkflowLifecycleTrace";

const view: WorkflowLifecycleTraceViewModel = {
  compactLabel: "静态检查通过 · 可试运行",
  ariaLabel: "保存：已保存；检查：静态检查通过；草稿试运行：可试运行；发布：未发布",
  stages: [
    { id: "save", label: "保存", state: "complete", description: "已保存" },
    { id: "check", label: "检查", state: "complete", description: "静态检查通过" },
    { id: "draft_run", label: "草稿试运行", state: "current", description: "可试运行" },
    { id: "publish", label: "发布", state: "pending", description: "未发布" },
  ],
};

describe("WorkflowLifecycleTrace", () => {
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

  it("renders one accessible lifecycle statement and the complete four-stage visual trace", async () => {
    await act(async () => root.render(<WorkflowLifecycleTrace view={view} />));
    const trace = container.querySelector('[data-testid="workflow-lifecycle-trace"]');
    expect(trace?.getAttribute("aria-label")).toBe(view.ariaLabel);
    expect(container.querySelectorAll("[data-lifecycle-stage]")).toHaveLength(4);
    expect(container.textContent).toContain(view.compactLabel);
  });
});
