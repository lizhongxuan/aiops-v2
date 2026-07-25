import { ReactFlowProvider } from "@xyflow/react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

  it("[UI-012] exposes candidate change and blocking state with icon, text and aria", async () => {
    await act(async () => {
      root.render(
        <ReactFlowProvider>
          <RunnerCanvasNode
            {...({
              id: "check",
              selected: false,
              data: {
                label: "Check",
                meta: { iconText: "IF" },
                ports: { inputs: [], outputs: [] },
                candidate: {
                  change: "changed",
                  blocked: "conflict",
                  fieldPaths: ["label", "step.args.threshold"],
                  messages: ["字段已被其他修改覆盖"],
                },
              },
            } as any)}
          />
        </ReactFlowProvider>,
      );
    });

    const node = container.querySelector('[data-testid="canvas-node-check"]');
    expect(node?.getAttribute("data-candidate-change")).toBe("changed");
    expect(node?.getAttribute("data-candidate-blocked")).toBe("conflict");
    expect(node?.getAttribute("aria-label")).toContain("候选修改");
    expect(node?.getAttribute("aria-label")).toContain("冲突");
    expect(node?.textContent).toContain("修改");
    expect(node?.textContent).toContain("冲突");
    expect(node?.textContent).toContain("label");
  });

  it("opens Candidate configuration only when the Candidate is explicitly editable", async () => {
    const onOpenConfig = vi.fn();
    const renderNode = async (candidateReadOnly: boolean) => {
      await act(async () => {
        root.render(
          <ReactFlowProvider>
            <RunnerCanvasNode
              {...({
                id: "check",
                selected: true,
                data: {
                  label: "Candidate check",
                  meta: { iconText: "IF" },
                  ports: { inputs: [], outputs: [] },
                  candidate: { change: "changed", fieldPaths: ["step.args"], messages: [] },
                  candidateReadOnly,
                  onOpenConfig,
                },
              } as any)}
            />
          </ReactFlowProvider>,
        );
      });
    };

    await renderNode(true);
    await act(async () => container.querySelector<HTMLElement>('[data-testid="canvas-node-check"]')?.dispatchEvent(new MouseEvent("dblclick", { bubbles: true })));
    expect(onOpenConfig).not.toHaveBeenCalled();

    await renderNode(false);
    await act(async () => container.querySelector<HTMLElement>('[data-testid="canvas-node-check"]')?.dispatchEvent(new MouseEvent("dblclick", { bubbles: true })));
    expect(onOpenConfig).toHaveBeenCalledWith("check");
  });
});
