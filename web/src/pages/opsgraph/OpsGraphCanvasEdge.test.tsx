import { act } from "react";
import type React from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsGraphCanvasEdge } from "./OpsGraphCanvasEdge";

vi.mock("@xyflow/react", () => ({
  BaseEdge: ({ interactionWidth, markerEnd, path }: { interactionWidth?: number; markerEnd?: string; path: string }) => (
    <div data-interaction-width={interactionWidth || ""} data-marker-end={markerEnd || ""} data-path={path} />
  ),
  EdgeLabelRenderer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  getBezierPath: () => ["M0 0 C40 0 60 40 100 40", 50, 20],
}));

describe("OpsGraphCanvasEdge", () => {
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

  it("selects the relationship when its label is clicked", () => {
    const onSelectRelationship = vi.fn();

    act(() => {
      root.render(
        <OpsGraphCanvasEdge
          id="e1"
          source="service.order-api"
          target="middleware.pg"
          sourceX={0}
          sourceY={0}
          targetX={100}
          targetY={40}
          sourcePosition={"right" as any}
          targetPosition={"left" as any}
          label="依赖"
          data={{
            relationship: { id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" },
            onSelectRelationship,
          }}
        />,
      );
    });

    const label = container.querySelector("button");
    expect(label?.textContent).toBe("依赖");
    expect(label?.style.pointerEvents).toBe("all");
    expect(label?.style.zIndex).toBe("20");

    act(() => {
      label?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onSelectRelationship).toHaveBeenCalledWith("e1");
  });

  it("draws lane-offset edges with separated paths and hit areas", () => {
    act(() => {
      root.render(
        <OpsGraphCanvasEdge
          id="e1"
          source="service.checkout"
          target="middleware.redis"
          sourceX={0}
          sourceY={0}
          targetX={100}
          targetY={40}
          sourcePosition={"right" as any}
          targetPosition={"left" as any}
          label="依赖"
          interactionWidth={26}
          data={{
            laneOffset: 18,
            relationship: { id: "e1", from: "service.checkout", type: "depends_on", to: "middleware.redis" },
          }}
        />,
      );
    });

    const edgePath = container.querySelector("[data-path]");
    const label = container.querySelector("button");
    expect(edgePath?.getAttribute("data-interaction-width")).toBe("26");
    expect(edgePath?.getAttribute("data-path")).toContain("18");
    expect(label?.style.transform).toContain("38px");
  });
});
