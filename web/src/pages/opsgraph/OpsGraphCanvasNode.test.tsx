import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsGraphCanvasNode } from "./OpsGraphCanvasNode";

vi.mock("@xyflow/react", () => ({
  Handle: ({ type }: { type: string }) => <span data-testid={`handle-${type}`} />,
  Position: { Left: "left", Right: "right" },
}));

describe("OpsGraphCanvasNode", () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

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

  it("selects the exact node when the custom card body is clicked", () => {
    const onSelect = vi.fn();

    act(() => {
      root.render(
        <OpsGraphCanvasNode
          id="middleware.pg"
          data={{
            label: "order-postgres",
            node: { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres" },
            onSelect,
            topologyMeta: { iconLabel: "PG", tone: "database", typeLabel: "Postgres", chips: [] },
          }}
        />,
      );
    });

    const card = container.querySelector(".opsgraph-node-card");
    expect(card).toBeTruthy();

    act(() => {
      card?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onSelect).toHaveBeenCalledWith("middleware.pg");
  });

  it("renders a graphic node icon instead of the old abbreviation badge", () => {
    act(() => {
      root.render(
        <OpsGraphCanvasNode
          id="middleware.pg"
          data={{
            label: "order-postgres",
            node: { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres" },
            topologyMeta: { iconLabel: "PG", tone: "database", typeLabel: "Postgres", chips: [] },
          }}
        />,
      );
    });

    expect(container.querySelector(".opsgraph-node-icon svg")).toBeTruthy();
    expect(container.querySelector(".opsgraph-node-icon")?.textContent).not.toContain("PG");
  });
});
