import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import RunnerCanvasEdge from "./RunnerCanvasEdge.vue";

describe("RunnerCanvasEdge", () => {
  it("renders the edge path and opens its menu target", async () => {
    const wrapper = mount(RunnerCanvasEdge, {
      props: {
        id: "gate-notify-if",
        sourceX: 10,
        sourceY: 20,
        targetX: 180,
        targetY: 100,
        sourcePosition: "right",
        targetPosition: "left",
        data: {
          kind: "if",
          edge: { id: "gate-notify-if", source: "gate", target: "notify", kind: "if" },
        },
      },
    });

    expect(wrapper.get('[data-testid="runner-edge-gate-notify-if"]').exists()).toBe(true);

    await wrapper.get('[data-testid="runner-edge-hit-gate-notify-if"]').trigger("contextmenu", {
      clientX: 260,
      clientY: 180,
      preventDefault: vi.fn(),
    });

    expect(wrapper.emitted("open-menu")?.[0]?.[0]).toMatchObject({
      edge: { id: "gate-notify-if", source: "gate", target: "notify" },
      x: 260,
      y: 180,
    });
  });
});
