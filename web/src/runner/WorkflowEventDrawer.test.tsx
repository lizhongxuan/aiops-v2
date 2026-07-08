import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowEventDrawer } from "./WorkflowEventDrawer";

describe("WorkflowEventDrawer", () => {
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

  it("shows newest events first and can return to Workflow AI", async () => {
    const onBackToAi = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowEventDrawer
          open
          events={[
            { id: "old", type: "workflow.ai.chat", actor: "assistant", summary: "旧事件", createdAt: "2026-07-07T08:00:00Z" },
            { id: "new", type: "workflow.graph.node.added", actor: "tool", summary: "新事件", createdAt: "2026-07-07T08:01:00Z" },
          ]}
          onBackToAi={onBackToAi}
        />,
      );
    });

    const rows = Array.from(container.querySelectorAll('[data-testid="workflow-event-row"]'));
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("新事件");
    expect(rows[1].textContent).toContain("旧事件");

    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("返回 AI"))?.click();
    });
    expect(onBackToAi).toHaveBeenCalledTimes(1);
  });
});
