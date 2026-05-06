import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import RunnerCanvas from "./RunnerCanvas.vue";

const graph = {
  version: "v1",
  workflow: { name: "demo" },
  nodes: [
    { id: "start", type: "start", position: { x: 40, y: 140 }, label: "Start" },
    { id: "pre-check", type: "action", position: { x: 260, y: 140 }, step: { name: "pre-check", action: "cmd.run" } },
  ],
  edges: [{ id: "start-pre-check", source: "start", target: "pre-check", kind: "next" }],
};

const actions = [{ action: "shell.run", label: "Shell Script" }];
const followUpActions = [
  { action: "cmd.run", label: "Command" },
  { action: "notify.send", label: "Notify" },
  { action: "approval.wait", label: "Approval" },
  { action: "wait.event", label: "Wait" },
];

describe("RunnerCanvas", () => {
  it("turns a dropped catalog action into a graph node update", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });
    const droppedAction = JSON.stringify(actions[0]);

    await wrapper.get('[data-testid="runner-canvas-dropzone"]').trigger("drop", {
      clientX: 520,
      clientY: 240,
      preventDefault: vi.fn(),
      dataTransfer: {
        getData: vi.fn((type) => (type === "application/runner-action" ? droppedAction : "")),
      },
    });

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.at(-1)).toMatchObject({
      id: "shell-run",
      type: "action",
      step: { action: "shell.run" },
    });
  });

  it("adds a catalog action to the graph when the palette item is clicked", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    expect(wrapper.find('[data-testid="catalog-action-shell-run"]').exists()).toBe(false);
    await wrapper.get('[data-testid="runner-node-picker-toggle"]').trigger("click");
    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.at(-1)).toMatchObject({
      id: "shell-run",
      type: "action",
      position: { x: 680, y: 80 },
      step: { action: "shell.run" },
    });
  });

  it("uses a floating Dify-style node picker instead of a permanent empty sidebar", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    expect(wrapper.get('[data-testid="runner-node-picker-toggle"]').text()).toContain("添加节点");
    expect(wrapper.find('[data-testid="runner-node-picker"]').exists()).toBe(false);

    await wrapper.get('[data-testid="runner-node-picker-toggle"]').trigger("click");

    expect(wrapper.get('[data-testid="runner-node-picker"] input').attributes("placeholder")).toBe("搜索节点");
    expect(wrapper.get('[data-testid="runner-node-picker"]').text()).toContain("Shell Script");
  });

  it("forwards fullscreen toggles from the floating canvas toolbar", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions, fullscreen: false },
    });

    await wrapper.get('[data-testid="runner-canvas-fullscreen-toggle"]').trigger("click");

    expect(wrapper.emitted("toggle-fullscreen")?.[0]).toEqual([]);
  });

  it("emits graph edge updates when nodes are connected", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph: { ...graph, edges: [] }, actions },
    });

    wrapper.findComponent({ name: "VueFlow" }).vm.$emit("connect", {
      source: "start",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    });
    await wrapper.vm.$nextTick();

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges.at(-1)).toMatchObject({
      source: "start",
      target: "pre-check",
      kind: "next",
    });
  });

  it("enables Dify-style canvas affordances through VueFlow", () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    const flow = wrapper.findComponent({ name: "VueFlow" });
    expect(flow.attributes("data-draggable")).toBe("true");
    expect(flow.attributes("data-connectable")).toBe("true");
    expect(flow.attributes("data-fit-view")).toBe("false");
    expect(flow.attributes("data-default-viewport-zoom")).toBe("1");
    expect(flow.attributes("data-max-zoom")).toBe("1.4");
    expect(flow.attributes("data-connection-mode")).toBe("strict");
    expect(flow.attributes("data-connection-radius")).toBe("44");
    expect(flow.attributes("data-connect-on-click")).toBe("true");
    expect(flow.props("edges")[0]).toMatchObject({
      source: "start",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    });
    expect(wrapper.find(".controls-stub").exists()).toBe(true);
    expect(wrapper.find(".minimap-stub").exists()).toBe(true);
  });

  it("updates graph positions when an existing node is dragged on the canvas", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    wrapper.findComponent({ name: "VueFlow" }).vm.$emit("node-drag-stop", {
      node: { id: "pre-check", position: { x: 360, y: 220 } },
    });
    await wrapper.vm.$nextTick();

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.find((node) => node.id === "pre-check").position).toEqual({ x: 360, y: 220 });
  });

  it("creates graph edges from VueFlow drag connections", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph: { ...graph, edges: [] }, actions },
    });

    wrapper.findComponent({ name: "VueFlow" }).vm.$emit("connect", {
      source: "start",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    });
    await wrapper.vm.$nextTick();

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges.at(-1)).toMatchObject({
      source: "start",
      target: "pre-check",
      kind: "next",
    });
  });

  it("shows an actionable validation message instead of silently accepting illegal connections", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    wrapper.findComponent({ name: "VueFlow" }).vm.$emit("connect", {
      source: "pre-check",
      target: "pre-check",
      sourceHandle: "next",
      targetHandle: "in",
    });
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:graph")).toBeUndefined();
    expect(wrapper.get('[data-testid="runner-canvas-validation"]').text()).toContain("不能连接到自身");
  });

  it("renders semantic handles and node metadata from the node registry", async () => {
    const updateNodeInternals = globalThis.__vueFlowUpdateNodeInternalsMock;
    updateNodeInternals.mockClear();
    const semanticGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        { id: "gate", type: "condition", position: { x: 120, y: 120 }, step: { action: "condition.branch" } },
        { id: "approval", type: "approval", position: { x: 360, y: 120 }, step: { action: "approval.wait" } },
      ],
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: semanticGraph, actions },
    });

    expect(wrapper.get('[data-testid="canvas-node-gate"]').text()).toContain("条件分支");
    expect(wrapper.get('[data-testid="node-output-gate-if"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="node-output-gate-else"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="node-output-approval-approved"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="node-output-approval-rejected"]').exists()).toBe(true);
    await wrapper.vm.$nextTick();
    await wrapper.vm.$nextTick();
    expect(updateNodeInternals).toHaveBeenCalledWith(["gate"]);
    expect(updateNodeInternals).toHaveBeenCalledWith(["approval"]);
  });

  it("keeps semantic edge kind in data without rendering a permanent NEXT badge", () => {
    const branchingGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        { id: "gate", type: "condition", position: { x: 120, y: 120 }, step: { action: "condition.branch" } },
        { id: "notify", type: "action", position: { x: 380, y: 120 }, step: { action: "notify.send" } },
      ],
      edges: [{ id: "gate-notify-if", source: "gate", target: "notify", kind: "if", source_port: "if", target_port: "in" }],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: branchingGraph, actions },
    });

    expect(wrapper.find('[data-testid="runner-edge-label-gate-notify-if"]').exists()).toBe(false);
    expect(wrapper.findComponent({ name: "VueFlow" }).props("edges")[0]).toMatchObject({
      type: "runner-edge",
      sourceHandle: "if",
      targetHandle: "in",
      label: "if",
    });
  });

  it("opens edge actions from the line hit area and updates the edge kind", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="runner-edge-hit-start-pre-check"]').trigger("click", {
      clientX: 420,
      clientY: 220,
      preventDefault: vi.fn(),
    });
    expect(wrapper.get('[data-testid="runner-edge-menu"]').text()).toContain("删除连线");

    await wrapper.get('[data-testid="edge-action-kind-failure"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges[0]).toMatchObject({
      id: "start-pre-check",
      kind: "failure",
      source_port: "failure",
    });
  });

  it("closes transient edge and picker menus from the canvas pane", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });
    const flow = wrapper.findComponent({ name: "VueFlow" });

    await wrapper.get('[data-testid="runner-edge-hit-start-pre-check"]').trigger("click", {
      clientX: 420,
      clientY: 220,
      preventDefault: vi.fn(),
    });
    expect(wrapper.find('[data-testid="runner-edge-menu"]').exists()).toBe(true);

    flow.vm.$emit("pane-click");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-testid="runner-edge-menu"]').exists()).toBe(false);

    await wrapper.get('[data-testid="node-output-add-start-next"]').trigger("click", {
      clientX: 420,
      clientY: 220,
    });
    expect(wrapper.find('[data-testid="runner-node-picker"]').exists()).toBe(true);

    flow.vm.$emit("pane-click");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-testid="runner-node-picker"]').exists()).toBe(false);
  });

  it("closes transient menus with Escape", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
      attachTo: document.body,
    });

    await wrapper.get('[data-testid="node-output-add-start-next"]').trigger("click", {
      clientX: 420,
      clientY: 220,
    });
    expect(wrapper.find('[data-testid="runner-node-picker"]').exists()).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-testid="runner-node-picker"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("deletes a graph edge from the edge context menu", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="runner-edge-start-pre-check"]').trigger("contextmenu", {
      clientX: 420,
      clientY: 220,
      preventDefault: vi.fn(),
    });
    expect(wrapper.get('[data-testid="runner-edge-menu"]').text()).toContain("删除连线");

    await wrapper.get('[data-testid="edge-action-delete"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges).toHaveLength(0);
  });

  it("opens edge actions from the edge line and updates the edge kind", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="runner-edge-hit-start-pre-check"]').trigger("click", {
      clientX: 420,
      clientY: 220,
      preventDefault: vi.fn(),
    });

    expect(wrapper.get('[data-testid="runner-edge-menu"]').text()).toContain("删除连线");
    await wrapper.get('[data-testid="edge-action-kind-failure"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges[0]).toMatchObject({
      id: "start-pre-check",
      source: "start",
      target: "pre-check",
      kind: "failure",
      source_port: "failure",
      target_port: "in",
    });
  });

  it("mounts a custom connection preview line while users drag from a handle", () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    expect(wrapper.get('[data-testid="runner-connection-line"]').exists()).toBe(true);
  });

  it("opens a contextual node picker when a dragged connection ends on empty canvas", async () => {
    const disconnectedGraph = {
      ...graph,
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: disconnectedGraph, actions },
    });

    const flow = wrapper.findComponent({ name: "VueFlow" });
    flow.vm.$emit("connect-start", {}, { nodeId: "start" });
    flow.vm.$emit("connect-end", { clientX: 600, clientY: 300 });
    await wrapper.vm.$nextTick();

    expect(wrapper.get('[data-testid="runner-node-picker"]').text()).toContain("Shell Script");

    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.at(-1)).toMatchObject({
      id: "shell-run",
      position: { x: 610, y: 320 },
      step: { action: "shell.run" },
    });
    expect(emittedGraph.edges.at(-1)).toMatchObject({
      source: "start",
      target: "shell-run",
      kind: "next",
    });
  });

  it("opens a plus picker from an output port and auto-connects the selected action", async () => {
    const disconnectedGraph = {
      ...graph,
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: disconnectedGraph, actions },
    });

    await wrapper.get('[data-testid="node-output-add-start-next"]').trigger("click", {
      clientX: 420,
      clientY: 220,
    });

    expect(wrapper.get('[data-testid="runner-node-picker"]').text()).toContain("Shell Script");
    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.at(-1)).toMatchObject({
      id: "shell-run",
      step: { action: "shell.run" },
    });
    expect(emittedGraph.edges.at(-1)).toMatchObject({
      source: "start",
      source_port: "next",
      target: "shell-run",
      kind: "next",
    });
  });

  it("auto-connects a port-plus action to End when the source has no existing next target", async () => {
    const blankGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        { id: "start", type: "start", position: { x: 80, y: 160 }, label: "Start" },
        { id: "end", type: "end", position: { x: 720, y: 160 }, label: "End" },
      ],
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: blankGraph, actions },
    });

    await wrapper.get('[data-testid="node-output-add-start-next"]').trigger("click", {
      clientX: 420,
      clientY: 220,
    });
    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ source: "start", target: "shell-run", source_port: "next", target_port: "in" }),
        expect.objectContaining({ source: "shell-run", target: "end", source_port: "next", target_port: "in" }),
      ]),
    );
  });

  it("inserts a selected action into an existing edge from the edge plus", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="runner-edge-insert-start-pre-check"]').trigger("click", {
      clientX: 420,
      clientY: 220,
    });
    expect(wrapper.get('[data-testid="runner-node-picker"]').text()).toContain("Shell Script");

    await wrapper.get('[data-testid="catalog-action-shell-run"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.edges).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ source: "start", target: "pre-check" })]),
    );
    expect(emittedGraph.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ source: "start", target: "shell-run", source_port: "next", kind: "next" }),
        expect.objectContaining({ source: "shell-run", target: "pre-check", source_port: "next", kind: "next" }),
      ]),
    );
  });

  it("recommends Notify from a failure port and auto-connects the new node", async () => {
    const shellGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        {
          id: "shell-run",
          type: "action",
          position: { x: 120, y: 120 },
          label: "Shell Script",
          step: { name: "Shell Script", action: "shell.run" },
        },
      ],
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: shellGraph, actions: followUpActions },
    });

    const flow = wrapper.findComponent({ name: "VueFlow" });
    flow.vm.$emit("connect-start", {}, { nodeId: "shell-run", handleId: "failure" });
    flow.vm.$emit("connect-end", { clientX: 620, clientY: 320 });
    await wrapper.vm.$nextTick();

    const picker = wrapper.get('[data-testid="runner-node-picker"]');
    expect(picker.text()).toContain("推荐节点");
    expect(picker.text()).toContain("Notify");
    expect(picker.text()).not.toContain("Command");

    await wrapper.get('[data-testid="catalog-action-notify-send"]').trigger("click");

    const emittedGraph = wrapper.emitted("update:graph")?.[0]?.[0];
    expect(emittedGraph.nodes.at(-1)).toMatchObject({
      id: "notify-send",
      step: { action: "notify.send" },
    });
    expect(emittedGraph.edges.at(-1)).toMatchObject({
      source: "shell-run",
      target: "notify-send",
      kind: "failure",
      source_port: "failure",
      target_port: "in",
    });
  });

  it("accepts the real VueFlow connect-start payload shape for empty-canvas picks", async () => {
    const shellGraph = {
      version: "v1",
      workflow: { name: "demo" },
      nodes: [
        {
          id: "shell-run",
          type: "action",
          position: { x: 120, y: 120 },
          label: "Shell Script",
          step: { name: "Shell Script", action: "shell.run" },
        },
      ],
      edges: [],
    };
    const wrapper = mount(RunnerCanvas, {
      props: { graph: shellGraph, actions: followUpActions },
    });

    const flow = wrapper.findComponent({ name: "VueFlow" });
    flow.vm.$emit("connect-start", { event: {}, nodeId: "shell-run", handleId: "failure", handleType: "source" });
    flow.vm.$emit("connect-end", { event: { clientX: 640, clientY: 340 } });
    await wrapper.vm.$nextTick();

    expect(wrapper.get('[data-testid="runner-node-picker"]').text()).toContain("Notify");
  });

  it("single click only updates summary selection and double click opens config", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="canvas-node-pre-check"]').trigger("click");
    await wrapper.get('[data-testid="canvas-node-pre-check"]').trigger("dblclick");

    expect(wrapper.emitted("select-node")?.[0]).toEqual(["pre-check"]);
    expect(wrapper.emitted("open-node-config")?.[0]).toEqual(["pre-check"]);
    expect(wrapper.emitted("update:graph")).toBeUndefined();
  });

  it("opens the node action menu on right click and forwards menu actions", async () => {
    const wrapper = mount(RunnerCanvas, {
      props: { graph, actions },
    });

    await wrapper.get('[data-testid="canvas-node-pre-check"]').trigger("contextmenu", {
      clientX: 240,
      clientY: 160,
      preventDefault: vi.fn(),
    });
    expect(wrapper.get('[data-testid="node-action-menu"]').text()).toContain("AI 修复");

    await wrapper.get('[data-testid="node-action-delete"]').trigger("click");

    expect(wrapper.emitted("node-action")?.[0]).toEqual(["delete", "pre-check"]);
  });
});
