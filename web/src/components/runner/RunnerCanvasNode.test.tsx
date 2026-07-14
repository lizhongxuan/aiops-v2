import { ReactFlowProvider } from "@xyflow/react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { RunnerCanvasNode } from "./RunnerCanvasNode";
import "./runnerStudio.css";

describe("RunnerCanvasNode", () => {
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

  it("shows a visible failure message on failed nodes", async () => {
    await act(async () => {
      root.render(
        <ReactFlowProvider>
          <RunnerCanvasNode
            {...({
              id: "start",
              selected: false,
              data: {
                label: "Start",
                meta: { iconText: "IN" },
                ports: { inputs: [], outputs: [] },
                runState: {
                  status: "failed",
                  label: "失败",
                  message: "未找到可执行目标：节点没有绑定目标标签 local",
                },
              },
            } as any)}
          />
        </ReactFlowProvider>,
      );
    });

    const node = container.querySelector('[data-testid="canvas-node-start"]');
    expect(node?.textContent).toContain("失败");
    const message = container.querySelector('[data-testid="canvas-node-start-failure-message"]');
    expect(message?.textContent).toContain("未找到可执行目标");
  });
});
