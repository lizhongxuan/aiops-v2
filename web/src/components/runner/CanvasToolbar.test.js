import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import CanvasToolbar from "./CanvasToolbar.vue";

const actions = [
  { action: "cmd.run", label: "Command" },
  { action: "shell.run", label: "Shell Script" },
];

describe("CanvasToolbar", () => {
  it("renders catalog actions as draggable palette items", async () => {
    const wrapper = mount(CanvasToolbar, { props: { actions } });
    const transfer = {
      setData: vi.fn(),
      effectAllowed: "",
    };

    expect(wrapper.find('[data-testid="catalog-action-shell-run"]').exists()).toBe(false);
    await wrapper.get('[data-testid="runner-node-picker-toggle"]').trigger("click");
    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("dragstart", {
      dataTransfer: transfer,
    });

    expect(wrapper.text()).toContain("Command");
    expect(wrapper.text()).toContain("Shell Script");
    expect(transfer.setData).toHaveBeenCalledWith("application/runner-action", JSON.stringify(actions[1]));
    expect(transfer.effectAllowed).toBe("copy");
  });

  it("emits an add action event when a palette item is clicked", async () => {
    const wrapper = mount(CanvasToolbar, { props: { actions } });

    await wrapper.get('[data-testid="runner-node-picker-toggle"]').trigger("click");
    await wrapper.get('[data-testid="catalog-action-cmd-run"]').trigger("click");

    expect(wrapper.emitted("add-action")?.[0]).toEqual([actions[0]]);
  });

  it("emits a fullscreen toggle from the canvas toolbar", async () => {
    const wrapper = mount(CanvasToolbar, { props: { actions, fullscreen: false } });

    await wrapper.get('[data-testid="runner-canvas-fullscreen-toggle"]').trigger("click");

    expect(wrapper.emitted("toggle-fullscreen")?.[0]).toEqual([]);
  });
});
