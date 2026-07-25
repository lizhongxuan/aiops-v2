import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowOutline } from "./WorkflowOutline";

describe("WorkflowOutline", () => {
  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => { globalThis.IS_REACT_ACT_ENVIRONMENT = true; container = document.createElement("div"); document.body.appendChild(container); root = createRoot(container); });
  afterEach(() => { act(() => root.unmount()); container.remove(); });

  it("uses business names, types and states for keyboard selection without reading internal ids", async () => {
    const onSelectNode = vi.fn();
    const internalId = "550e8400-e29b-41d4-a716-446655440000";
    await act(async () => root.render(<WorkflowOutline
      graph={{ nodes: [{ id: internalId, type: "action", label: "检查 Redis 容量", position: { x: 0, y: 0 } }], edges: [] }}
      selectedNodeId={internalId}
      nodeStatus={{ [internalId]: "running" }}
      nodeValidation={{ [internalId]: "warning" }}
      onSelectNode={onSelectNode}
    />));
    const option = container.querySelector('[role="option"]') as HTMLButtonElement;
    expect(option.getAttribute("aria-label")).toBe("检查 Redis 容量，执行步骤，运行中，有检查提醒");
    expect(option.getAttribute("aria-label")).not.toContain(internalId);
    expect(option.getAttribute("aria-selected")).toBe("true");
    await act(async () => option.click());
    expect(onSelectNode).toHaveBeenCalledWith(internalId);
  });
});
