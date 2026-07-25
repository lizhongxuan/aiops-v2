import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createInitialRunState } from "@/components/runner/runStateReducer";
import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";
import { RunnerNodeInspector, type RunnerNodeInspectorAction } from "@/runner/RunnerNodeInspector";

const graph: RunnerGraph = {
  version: "v1",
  workflow: { name: "inspector-fixture" },
  nodes: [
    { id: "start", type: "start", label: "Start", ui: { host_groups: [{ label: "local", hosts: ["local"] }] } },
    {
      id: "script",
      type: "action",
      label: "检查容量",
      inputs: [],
      outputs: [],
      step: { name: "检查容量", action: "script.python", args: { script: "print('before')" }, targets: ["local"] },
    },
    {
      id: "branch",
      type: "condition",
      label: "容量足够",
      condition: { if: "{{steps.script.outputs.free}} > 10", else: true },
      step: { name: "容量足够", action: "condition.evaluate" },
    },
    { id: "end", type: "end", label: "End", ui: {} },
  ],
  edges: [{ id: "script-to-branch", source: "script", target: "branch", kind: "next" }],
};

function click(element: Element | null) {
  if (!element) throw new Error("missing clickable element");
  element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
}

function changeTextarea(element: HTMLTextAreaElement | null, value: string) {
  if (!element) throw new Error("missing textarea");
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(element, value);
  element.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("RunnerNodeInspector", () => {
  let container: HTMLDivElement;
  let root: Root;
  let actions: RunnerNodeInspectorAction[];

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    actions = [];
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  function render(node: RunnerNode) {
    act(() => {
      root.render(
        <RunnerNodeInspector
          node={node}
          graph={graph}
          runState={createInitialRunState()}
          onAction={(action) => actions.push(action)}
        />,
      );
    });
  }

  it("emits a typed apply request and resets the active tab when node identity changes", () => {
    render(graph.nodes[1]);

    expect(container.querySelector('[data-testid="runner-node-panel-tab-io"]')).not.toBeNull();
    act(() => click(container.querySelector('[data-testid="runner-node-panel-tab-io"]')));
    expect(container.querySelector('[data-testid="runner-code-input-variables"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="runner-code-output-variables"]')).not.toBeNull();
    act(() => click(container.querySelector('[data-testid="runner-node-panel-tab-script"]')));
    expect(container.querySelector('[data-testid="runner-node-panel-tab-script"]')?.classList.contains("active")).toBe(true);

    act(() => changeTextarea(container.querySelector('[data-testid="runner-node-script-editor"]'), "print('after')"));
    act(() => click(container.querySelector('[data-testid="runner-node-panel-apply"]')));

    expect(actions).toHaveLength(1);
    expect(actions[0]).toMatchObject({ type: "apply-node", node: { id: "script" } });
    expect((actions[0] as Extract<RunnerNodeInspectorAction, { type: "apply-node" }>).node.step?.args?.script).toBe("print('after')");
    act(() => click(container.querySelector('[data-testid="runner-node-panel-delete"]')));
    expect(actions).toHaveLength(1);
    expect(container.querySelector('[data-testid="runner-node-delete-confirmation"]')).not.toBeNull();
    expect(container.textContent).toContain("检查容量（script）");
    expect(container.textContent).toContain("script-to-branch");
    expect(container.textContent).toContain("容量足够（branch）仍引用此节点");
    expect(container.textContent).toContain("删除完成后可在“历史”中撤销此操作");
    act(() => click(container.querySelector('[data-testid="runner-node-delete-confirm"]')));
    expect(actions[1]).toEqual({ type: "delete-node", nodeId: "script" });

    render(graph.nodes[2]);
    expect(container.querySelector('[data-testid="runner-node-panel-tab-settings"]')?.classList.contains("active")).toBe(true);
    expect(container.querySelector('[data-testid="runner-node-script-editor"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-node-panel-tab-branches"]')).not.toBeNull();
    act(() => click(container.querySelector('[data-testid="runner-node-panel-tab-branches"]')));
    expect(container.querySelector('[data-testid="runner-branch-editor"]')).not.toBeNull();
  });

  it("cancels a destructive deletion without sending a command", () => {
    render(graph.nodes[1]);
    act(() => click(container.querySelector('[data-testid="runner-node-panel-delete"]')));
    act(() => click(container.querySelector('[data-testid="runner-node-delete-cancel"]')));

    expect(actions).toEqual([]);
    expect(container.querySelector('[data-testid="runner-node-delete-confirmation"]')).toBeNull();
  });

  it("deletes an isolated low-risk node directly without unnecessary confirmation", () => {
    const isolated = { id: "isolated", type: "action", label: "临时步骤", step: { action: "script.shell" } };
    act(() => {
      root.render(
        <RunnerNodeInspector
          node={isolated}
          graph={{ ...graph, nodes: [...graph.nodes, isolated] }}
          runState={createInitialRunState()}
          onAction={(action) => actions.push(action)}
        />,
      );
    });
    act(() => click(container.querySelector('[data-testid="runner-node-panel-delete"]')));

    expect(actions).toEqual([{ type: "delete-node", nodeId: "isolated" }]);
    expect(container.querySelector('[data-testid="runner-node-delete-confirmation"]')).toBeNull();
  });

  it("preserves system-node protection and emits only explicit close/run requests", () => {
    render(graph.nodes[0]);

    expect(container.querySelector('[data-testid="runner-node-system-card"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="runner-start-host-groups"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="runner-node-panel-delete"]')).toBeNull();

    act(() => click(container.querySelector('[data-testid="runner-node-panel-run"]')));
    act(() => click(container.querySelector('[data-testid="runner-node-panel-close"]')));
    expect(actions).toEqual([
      { type: "run-node", nodeId: "start" },
      { type: "close" },
    ]);
  });
});
