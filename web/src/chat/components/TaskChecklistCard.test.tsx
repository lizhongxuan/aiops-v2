import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TaskChecklistCard } from "./TaskChecklistCard";
import { buildTaskChecklistViewModel } from "./taskChecklistViewModel";

describe("TaskChecklistCard", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("renders an expanded checklist with stable numbering and status icons", async () => {
    const onItemClick = vi.fn();

    await act(async () => {
      root.render(
        <TaskChecklistCard
          title="计划"
          items={[
            { id: "step-c", index: 3, title: "收集主机信息", status: "completed" },
            { id: "step-a", index: 1, title: "确认变更窗口", status: "running" },
            { id: "step-b", index: 2, title: "等待审批", status: "blocked" },
          ]}
          onItemClick={onItemClick}
        />,
      );
    });

    expect(container.textContent).toContain("计划");
    expect(container.textContent).toContain("共 3 个步骤，已经完成 1 个");
    expect(container.querySelector('[data-testid="task-checklist-card"]')?.className).toContain("text-[12px]");
    expect(container.textContent).toContain("3. 收集主机信息");
    expect(container.textContent).toContain("1. 确认变更窗口");
    expect(container.textContent).toContain("2. 等待审批");
    expect(container.querySelector('[data-status="completed"]')).not.toBeNull();
    expect(container.querySelector('[data-status="running"]')).not.toBeNull();
    expect(container.querySelector('[data-status="blocked"]')).not.toBeNull();

    const item = container.querySelector('[data-testid="task-checklist-item-step-a"]') as HTMLButtonElement;
    await act(async () => {
      item.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onItemClick).toHaveBeenCalledWith(expect.objectContaining({ id: "step-a" }));
  });

  it("collapses to a summary row without rendering checklist items", async () => {
    await act(async () => {
      root.render(
        <TaskChecklistCard
          title="计划"
          defaultCollapsed
          items={[
            { id: "step-1", title: "第一步", status: "completed" },
            { id: "step-2", title: "第二步", status: "pending" },
          ]}
        />,
      );
    });

    expect(container.textContent).toContain("共 2 个步骤，已经完成 1 个");
    expect(container.textContent).not.toContain("1. 第一步");
    expect(container.querySelector('[aria-expanded="false"]')).not.toBeNull();

    const toggle = container.querySelector('[data-testid="task-checklist-toggle"]') as HTMLButtonElement;
    await act(async () => {
      toggle.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).toContain("1. 第一步");
    expect(container.querySelector('[aria-expanded="true"]')).not.toBeNull();
  });

  it("counts completed-compatible statuses in the view model", () => {
    const viewModel = buildTaskChecklistViewModel([
      { id: "a", title: "A", status: "completed" },
      { id: "b", title: "B", status: "done" },
      { id: "c", title: "C", status: "success" },
      { id: "d", title: "D", status: "failed" },
    ]);

    expect(viewModel.totalCount).toBe(4);
    expect(viewModel.completedCount).toBe(3);
  });
});
