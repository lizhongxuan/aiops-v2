import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Position, ReactFlowProvider } from "@xyflow/react";

import { RunnerCanvasEdge } from "./RunnerCanvasEdge";

describe("RunnerCanvasEdge", () => {
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

  it("[UI-013] exposes deleted/warning edge delta with visible text and aria", async () => {
    await act(async () => {
      root.render(
        <ReactFlowProvider>
          <svg>
            <RunnerCanvasEdge {...({
              id: "edge-a", sourceX: 0, sourceY: 0, targetX: 200, targetY: 0,
              sourcePosition: Position.Right, targetPosition: Position.Left,
              data: { candidate: { change: "deleted", blocked: "warning", fieldPaths: ["condition"], fieldDiffs: [], messages: ["结束路径缺失"] } },
            } as any)} />
          </svg>
        </ReactFlowProvider>,
      );
    });

    const path = container.querySelector('[data-testid="runner-edge-path-edge-a"]');
    expect(path?.getAttribute("data-candidate-change")).toBe("deleted");
    expect(path?.getAttribute("aria-label")).toContain("候选删除");
    expect(container.textContent).toContain("删除");
    expect(container.textContent).toContain("警告");
  });
});
