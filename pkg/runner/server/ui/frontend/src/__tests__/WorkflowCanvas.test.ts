// @vitest-environment jsdom
import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it, vi } from "vitest";
import WorkflowCanvas from "../components/WorkflowCanvas.vue";
import type { WorkflowGraph } from "../types/workflow";

vi.mock("@vue-flow/core", () => ({
  SelectionMode: { Partial: "partial", Full: "full" },
  VueFlow: defineComponent({
    name: "VueFlow",
    inheritAttrs: false,
    props: {
      nodes: { type: Array, default: () => [] },
      edges: { type: Array, default: () => [] },
      nodesDraggable: Boolean,
      nodesConnectable: Boolean,
      elementsSelectable: Boolean,
      fitViewOnInit: Boolean,
      selectionKeyCode: { type: [String, Boolean, Array], default: undefined },
      selectionMode: { type: String, default: undefined },
      nodeClassName: { type: Function, default: undefined },
    },
    emits: ["node-click", "node-drag-stop", "connect", "pane-click"],
    template: `
      <div
        class="vue-flow-stub"
        v-bind="$attrs"
        :data-draggable="String(nodesDraggable)"
        :data-connectable="String(nodesConnectable)"
        :data-selectable="String(elementsSelectable)"
        :data-fit-view="String(fitViewOnInit)"
        :data-selection-key="String(selectionKeyCode)"
        :data-selection-mode="String(selectionMode)"
      >
        <slot />
      </div>
    `,
  }),
  useVueFlow: () => ({
    screenToFlowCoordinate: ({ x, y }: { x: number; y: number }) => ({ x: x + 10, y: y + 20 }),
  }),
}));

vi.mock("@vue-flow/background", () => ({
  Background: defineComponent({ name: "Background", template: '<div class="background-stub" />' }),
}));

vi.mock("@vue-flow/controls", () => ({
  Controls: defineComponent({ name: "Controls", template: '<div class="controls-stub" />' }),
}));

vi.mock("@vue-flow/minimap", () => ({
  MiniMap: defineComponent({ name: "MiniMap", template: '<div class="minimap-stub" />' }),
}));

vi.mock("lucide-vue-next", () => {
  const Icon = defineComponent({ name: "IconStub", template: "<span />" });
  return {
    ClipboardPaste: Icon,
    Copy: Icon,
    GitBranch: Icon,
    LayoutGrid: Icon,
    Redo2: Icon,
    Trash2: Icon,
    Undo2: Icon,
  };
});

const graph: WorkflowGraph = {
  version: "v1",
  workflow: { version: "v0.1", name: "canvas-test" },
  nodes: [
    { id: "start", type: "start", label: "Start", position: { x: 80, y: 120 } },
    { id: "run", type: "action", label: "Run command", position: { x: 280, y: 120 }, step: { name: "run", action: "cmd.run" } },
    { id: "end", type: "end", label: "End", position: { x: 520, y: 120 } },
  ],
  edges: [
    { id: "start-run", source: "start", target: "run", kind: "next" },
    { id: "run-end", source: "run", target: "end", kind: "success" },
  ],
};

describe("WorkflowCanvas", () => {
  it("enables production canvas affordances through Vue Flow", () => {
    const wrapper = mountCanvas();
    const flow = wrapper.find(".vue-flow-stub");

    expect(flow.attributes("data-draggable")).toBe("true");
    expect(flow.attributes("data-connectable")).toBe("true");
    expect(flow.attributes("data-selectable")).toBe("true");
    expect(flow.attributes("data-fit-view")).toBe("true");
    expect(flow.attributes("data-selection-key")).toBe("undefined");
    expect(flow.attributes("data-selection-mode")).toBe("partial");
    expect(wrapper.find(".controls-stub").exists()).toBe(true);
    expect(wrapper.find(".minimap-stub").exists()).toBe(true);
  });

  it("emits graph editing events for node selection, movement, connection, drag-drop, and toolbar actions", async () => {
    const wrapper = mountCanvas({ selectedNodeId: "run", canUndo: true, canRedo: true, canPaste: true });
    const flow = wrapper.findComponent({ name: "VueFlow" });

    flow.vm.$emit("node-click", { node: { id: "run" } });
    flow.vm.$emit("node-drag-stop", { node: { id: "run", position: { x: 340, y: 180 } } });
    flow.vm.$emit("connect", { source: "run", target: "end" });
    flow.vm.$emit("pane-click");

    const actionTransfer = dataTransfer({
      "application/runner-action": "cmd.run",
    });
    await wrapper.find(".vue-flow-stub").trigger("dragover", { dataTransfer: actionTransfer });
    await wrapper.find(".vue-flow-stub").trigger("drop", { clientX: 100, clientY: 120, dataTransfer: actionTransfer });

    const controlTransfer = dataTransfer({
      "application/runner-node-type": "join",
    });
    await wrapper.find(".vue-flow-stub").trigger("drop", { clientX: 220, clientY: 260, dataTransfer: controlTransfer });

    await wrapper.find('button[title="Undo"]').trigger("click");
    await wrapper.find('button[title="Redo"]').trigger("click");
    await wrapper.find('button[title="Copy selected node"]').trigger("click");
    await wrapper.find('button[title="Paste node"]').trigger("click");
    await wrapper.find('button[title="Auto layout"]').trigger("click");
    await wrapper.find('button[title="Delete selected node"]').trigger("click");

    expect(wrapper.emitted("select-node")).toEqual([["run"], [null]]);
    expect(wrapper.emitted("update-node-position")).toEqual([["run", { x: 340, y: 180 }]]);
    expect(wrapper.emitted("connect-nodes")).toEqual([["run", "end"]]);
    expect(wrapper.emitted("add-action")).toEqual([["cmd.run", { x: 110, y: 140 }]]);
    expect(wrapper.emitted("add-control-node")).toEqual([["join", { x: 230, y: 280 }]]);
    expect(wrapper.emitted("undo")).toHaveLength(1);
    expect(wrapper.emitted("redo")).toHaveLength(1);
    expect(wrapper.emitted("copy-selected")).toHaveLength(1);
    expect(wrapper.emitted("paste-node")).toHaveLength(1);
    expect(wrapper.emitted("auto-layout")).toHaveLength(1);
    expect(wrapper.emitted("delete-selected")).toHaveLength(1);
  });
});

function mountCanvas(overrides: Partial<InstanceType<typeof WorkflowCanvas>["$props"]> = {}) {
  return mount(WorkflowCanvas, {
    props: {
      graph,
      selectedNodeId: null,
      canUndo: false,
      canRedo: false,
      canPaste: false,
      ...overrides,
    },
  });
}

function dataTransfer(values: Record<string, string>): DataTransfer {
  return {
    types: Object.keys(values),
    dropEffect: "none",
    getData: (type: string) => values[type] || "",
  } as unknown as DataTransfer;
}
