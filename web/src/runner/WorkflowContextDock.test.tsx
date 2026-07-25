import { act, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WORKFLOW_CANVAS_RESIZE_EVENT,
  WorkflowContextDock,
  closeWorkflowContextDock,
  dockWidthBounds,
  requestWorkflowContextDockTab,
} from "./WorkflowContextDock";

function StatefulSlot({ label, onMount }: { label: string; onMount: () => void }) {
  const [value, setValue] = useState("");
  useState(() => onMount());
  return (
    <label>
      {label}
      <input
        data-testid={`slot-input-${label}`}
        value={value}
        onChange={(event) => setValue(event.target.value)}
      />
    </label>
  );
}

function setInputValue(input: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function dispatchPointer(
  target: HTMLElement,
  type: string,
  {
    pointerId = 1,
    clientX = 0,
    button = 0,
    buttons = 0,
    isPrimary = true,
    pointerType = "mouse",
  }: {
    pointerId?: number;
    clientX?: number;
    button?: number;
    buttons?: number;
    isPrimary?: boolean;
    pointerType?: string;
  } = {},
) {
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, clientX, button, buttons });
  Object.defineProperties(event, {
    pointerId: { value: pointerId },
    isPrimary: { value: isPrimary },
    pointerType: { value: pointerType },
  });
  target.dispatchEvent(event);
}

describe("WorkflowContextDock", () => {
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
    vi.restoreAllMocks();
  });

  it("separates the persistent node sidebar from the collapsed workflow drawer", async () => {
    const mounts = Object.fromEntries(["AI", "配置", "审阅", "运行", "历史"].map((label) => [label, vi.fn()]));
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<div data-testid="canvas-content">Canvas</div>}
        slots={{
          ai: <StatefulSlot label="AI" onMount={mounts.AI} />,
          configuration: <StatefulSlot label="配置" onMount={mounts.配置} />,
          review: <StatefulSlot label="审阅" onMount={mounts.审阅} />,
          run: <StatefulSlot label="运行" onMount={mounts.运行} />,
          history: <StatefulSlot label="历史" onMount={mounts.历史} />,
        }}
      />,
    ));

    const layout = container.querySelector('[data-testid="workflow-context-layout"]') as HTMLElement;
    const nodeSidebar = container.querySelector('[data-testid="workflow-node-sidebar"]') as HTMLElement;
    const workflowDrawer = container.querySelector('[data-testid="workflow-context-dock"]') as HTMLElement;
    expect(layout.classList.contains("with-node-sidebar")).toBe(true);
    expect(nodeSidebar.getAttribute("aria-label")).toBe("节点操作");
    expect(nodeSidebar.textContent).toContain("配置");
    expect(workflowDrawer.hidden).toBe(true);
    expect(workflowDrawer.getAttribute("aria-label")).toBe("工作流面板");
    expect(container.querySelector('[data-testid="workflow-context-drawer-open"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect([...container.querySelectorAll('[role="tab"][data-testid^="workflow-context-tab-"]')].map((tab) => tab.textContent)).toEqual([
      "AI 协作",
      "变更",
      "运行",
    ]);
    expect(container.querySelectorAll('[data-testid^="workflow-context-panel-"]')).toHaveLength(3);
    for (const mount of Object.values(mounts)) expect(mount).toHaveBeenCalledTimes(1);
  });

  it("uses the shared line Tabs primitive with automatic roving-focus activation", async () => {
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<div>Canvas</div>}
        slots={{ ai: <div>AI</div>, review: <div>Review</div>, run: <div>Run</div> }}
        initialWorkflowDrawerOpen
      />,
    ));

    const tabs = container.querySelector('[data-slot="tabs"]');
    const tabList = container.querySelector('[data-slot="tabs-list"]');
    const aiTab = container.querySelector('[data-testid="workflow-context-tab-ai"]') as HTMLButtonElement;
    const changesTab = container.querySelector('[data-testid="workflow-context-tab-changes"]') as HTMLButtonElement;
    expect(tabs).not.toBeNull();
    expect(tabList?.getAttribute("data-variant")).toBe("line");
    expect(aiTab.getAttribute("data-slot")).toBe("tabs-trigger");
    expect(container.querySelector('[data-testid="workflow-context-panel-ai"]')?.getAttribute("data-slot")).toBe("tabs-content");

    aiTab.focus();
    await act(async () => {
      aiTab.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
      await new Promise((resolve) => window.setTimeout(resolve, 0));
    });
    expect(document.activeElement).toBe(changesTab);
    expect(changesTab.getAttribute("aria-selected")).toBe("true");
  });

  it("opens the workflow drawer over an inert node sidebar and preserves both surfaces' drafts", async () => {
    const aiMount = vi.fn();
    const nodeMount = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={(
          <button
            type="button"
            data-testid="workflow-workbench-trigger"
            onClick={() => requestWorkflowContextDockTab("ai")}
          >
            工作台
          </button>
        )}
        slots={{
          ai: <StatefulSlot label="AI" onMount={aiMount} />,
          configuration: <StatefulSlot label="节点" onMount={nodeMount} />,
          review: <div>Review</div>,
          run: <div>Run</div>,
          history: <div>History</div>,
        }}
      />,
    ));

    const launcher = container.querySelector('[data-testid="workflow-workbench-trigger"]') as HTMLButtonElement;
    launcher.focus();
    await act(async () => launcher.click());
    const drawer = container.querySelector('[data-testid="workflow-context-dock"]') as HTMLElement;
    const nodeSidebar = container.querySelector('[data-testid="workflow-node-sidebar"]') as HTMLElement;
    expect(drawer.hidden).toBe(false);
    expect(nodeSidebar.getAttribute("aria-hidden")).toBe("true");
    expect(nodeSidebar.hasAttribute("inert")).toBe(true);

    await act(async () => {
      setInputValue(container.querySelector('[data-testid="slot-input-AI"]') as HTMLInputElement, "未发送的 AI 输入");
      setInputValue(container.querySelector('[data-testid="slot-input-节点"]') as HTMLInputElement, "节点草稿");
    });
    await act(async () => (container.querySelector('[data-testid="workflow-context-tab-changes"]') as HTMLButtonElement)
      .dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 })));
    expect(container.querySelector('[data-testid="workflow-context-panel-ai"]')?.hasAttribute("hidden")).toBe(true);
    expect(container.querySelector('[data-testid="workflow-context-panel-ai"]')?.hasAttribute("inert")).toBe(true);

    await act(async () => (container.querySelector('[data-testid="workflow-context-drawer-close"]') as HTMLButtonElement).click());
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)));
    expect(drawer.hidden).toBe(true);
    expect(nodeSidebar.getAttribute("aria-hidden")).toBe("false");
    expect(nodeSidebar.hasAttribute("inert")).toBe(false);
    expect(document.activeElement).toBe(launcher);

    await act(async () => launcher.click());
    await act(async () => (container.querySelector('[data-testid="workflow-context-tab-ai"]') as HTMLButtonElement)
      .dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 })));
    expect((container.querySelector('[data-testid="slot-input-AI"]') as HTMLInputElement).value).toBe("未发送的 AI 输入");
    expect((container.querySelector('[data-testid="slot-input-节点"]') as HTMLInputElement).value).toBe("节点草稿");
    expect(aiMount).toHaveBeenCalledTimes(1);
    expect(nodeMount).toHaveBeenCalledTimes(1);
  });

  it("opens workflow tabs through events and treats configuration as a node-surface request", async () => {
    const onTabChange = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<button type="button" data-testid="open-review">Open review</button>}
        slots={{ ai: <p>Chat</p>, configuration: <p>Node config</p>, review: <p>Review</p>, run: <p>Run</p> }}
        initialWorkflowDrawerOpen
        onTabChange={onTabChange}
      />,
    ));
    const trigger = container.querySelector('[data-testid="open-review"]') as HTMLButtonElement;
    trigger.focus();
    await act(async () => requestWorkflowContextDockTab("review"));
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(false);
    expect(container.querySelector('[data-testid="workflow-context-tab-changes"]')?.getAttribute("aria-selected")).toBe("true");

    await act(async () => requestWorkflowContextDockTab("configuration"));
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(true);
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).not.toBeNull();
    expect(onTabChange).toHaveBeenLastCalledWith("configuration");

    await act(async () => requestWorkflowContextDockTab("history"));
    expect(container.querySelector('[data-testid="workflow-context-tab-run"]')?.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(false);
    await act(async () => closeWorkflowContextDock());
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(true);
  });

  it("closes the desktop workflow drawer with Escape and restores its trigger focus", async () => {
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<button type="button" data-testid="canvas-trigger">Canvas trigger</button>}
        slots={{ ai: <div>AI</div>, review: <div>Review</div> }}
      />,
    ));
    const trigger = container.querySelector('[data-testid="canvas-trigger"]') as HTMLButtonElement;
    trigger.focus();
    await act(async () => requestWorkflowContextDockTab("review"));
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(false);
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)));
    expect(container.querySelector('[data-testid="workflow-context-dock"]')?.hasAttribute("hidden")).toBe(true);
    expect(document.activeElement).toBe(trigger);
  });

  it("omits the node sidebar and its separator when no node configuration exists", async () => {
    const onDockWidthChange = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        viewportWidth={1280}
        canvas={<div>Canvas</div>}
        slots={{ ai: <div>AI</div>, review: <div>Review</div>, run: <div>Run</div>, history: <div>History</div> }}
        onDockWidthChange={onDockWidthChange}
      />,
    ));
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-node-sidebar-separator"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-layout"]')?.classList.contains("with-node-sidebar")).toBe(false);
    expect(onDockWidthChange).toHaveBeenLastCalledWith(0);
  });

  it("provides a keyboard separator only for the bounded desktop node sidebar", async () => {
    expect(dockWidthBounds(1440)).toEqual({ min: 320, max: 640 });
    expect(dockWidthBounds(1280)).toEqual({ min: 320, max: 640 });
    expect(dockWidthBounds(1024)).toEqual({ min: 320, max: 536 });
    const onDockWidthChange = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<div>Canvas</div>}
        slots={{ configuration: <div>Node config</div> }}
        viewportWidth={1024}
        onDockWidthChange={onDockWidthChange}
      />,
    ));
    const separator = container.querySelector('[data-testid="workflow-node-sidebar-separator"]') as HTMLElement;
    expect(separator.getAttribute("role")).toBe("separator");
    expect(separator.getAttribute("aria-controls")).toBe("workflow-node-sidebar");
    expect(separator.getAttribute("aria-valuemin")).toBe("320");
    expect(separator.getAttribute("aria-valuemax")).toBe("536");
    expect(separator.getAttribute("aria-valuenow")).toBe("420");
    await act(async () => separator.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true })));
    expect(separator.getAttribute("aria-valuenow")).toBe("436");
    await act(async () => separator.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true })));
    expect(separator.getAttribute("aria-valuenow")).toBe("536");
    await act(async () => separator.dispatchEvent(new KeyboardEvent("keydown", { key: "Home", bubbles: true })));
    expect(separator.getAttribute("aria-valuenow")).toBe("320");
    expect(onDockWidthChange).toHaveBeenCalledWith(320);
  });

  it("changes width only during an active primary-pointer drag and self-heals missed releases", async () => {
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<div>Canvas</div>}
        slots={{ configuration: <div>Node config</div> }}
        viewportWidth={1024}
      />,
    ));
    const separator = container.querySelector('[data-testid="workflow-node-sidebar-separator"]') as HTMLElement;
    expect(separator.getAttribute("aria-valuenow")).toBe("420");

    await act(async () => dispatchPointer(separator, "pointermove", { pointerId: 7, clientX: 504, buttons: 0 }));
    expect(separator.getAttribute("aria-valuenow")).toBe("420");

    await act(async () => {
      dispatchPointer(separator, "pointerdown", { pointerId: 7, clientX: 604, buttons: 1 });
      dispatchPointer(separator, "pointermove", { pointerId: 7, clientX: 544, buttons: 1 });
    });
    expect(separator.getAttribute("aria-valuenow")).toBe("480");

    await act(async () => dispatchPointer(separator, "pointermove", { pointerId: 7, clientX: 520, buttons: 0 }));
    expect(separator.getAttribute("aria-valuenow")).toBe("480");
    await act(async () => dispatchPointer(separator, "pointermove", { pointerId: 7, clientX: 500, buttons: 1 }));
    expect(separator.getAttribute("aria-valuenow")).toBe("480");

    await act(async () => {
      dispatchPointer(separator, "pointerdown", { pointerId: 8, clientX: 544, buttons: 1 });
      dispatchPointer(separator, "pointermove", { pointerId: 8, clientX: 560, buttons: 1 });
    });
    expect(separator.getAttribute("aria-valuenow")).toBe("464");
    await act(async () => dispatchPointer(separator, "lostpointercapture", { pointerId: 8 }));
    await act(async () => dispatchPointer(separator, "pointermove", { pointerId: 8, clientX: 500, buttons: 1 }));
    expect(separator.getAttribute("aria-valuenow")).toBe("464");
  });

  it("clears a drag when the node sidebar unmounts before pointerup", async () => {
    const renderDock = (configuration?: ReactNode) => (
      <WorkflowContextDock
        canvas={<div>Canvas</div>}
        slots={{ configuration }}
        viewportWidth={1024}
      />
    );
    await act(async () => root.render(renderDock(<div>Node config</div>)));
    let separator = container.querySelector('[data-testid="workflow-node-sidebar-separator"]') as HTMLElement;
    await act(async () => dispatchPointer(separator, "pointerdown", { pointerId: 9, clientX: 604, buttons: 1 }));

    await act(async () => root.render(renderDock()));
    expect(container.querySelector('[data-testid="workflow-node-sidebar-separator"]')).toBeNull();
    await act(async () => root.render(renderDock(<div>Node config restored</div>)));
    separator = container.querySelector('[data-testid="workflow-node-sidebar-separator"]') as HTMLElement;
    await act(async () => dispatchPointer(separator, "pointermove", { pointerId: 9, clientX: 520, buttons: 1 }));
    expect(separator.getAttribute("aria-valuenow")).toBe("420");
  });

  it("emits node-sidebar occupancy as canvas resize evidence", async () => {
    let observerCallback: ResizeObserverCallback | undefined;
    globalThis.ResizeObserver = class ResizeObserver {
      constructor(callback: ResizeObserverCallback) { observerCallback = callback; }
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    const resize = vi.fn();
    window.addEventListener(WORKFLOW_CANVAS_RESIZE_EVENT, resize);
    await act(async () => root.render(
      <WorkflowContextDock canvas={<div>Canvas</div>} slots={{ configuration: <div>Node config</div> }} viewportWidth={1280} />,
    ));
    const separator = container.querySelector('[data-testid="workflow-node-sidebar-separator"]') as HTMLElement;
    await act(async () => separator.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true })));
    await act(async () => observerCallback?.([], {} as ResizeObserver));
    expect(resize).toHaveBeenCalledWith(expect.objectContaining({ detail: expect.objectContaining({ dockWidth: 436 }) }));
    window.removeEventListener(WORKFLOW_CANVAS_RESIZE_EVENT, resize);
  });

  it("keeps mobile workflow modes separate from the modal node configuration sheet", async () => {
    const aiMount = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        viewportWidth={390}
        configurationFocusTargetId="check"
        canvas={<button type="button" data-testid="canvas-node-check">Check</button>}
        slots={{
          ai: <StatefulSlot label="移动 AI" onMount={aiMount} />,
          configuration: <div data-testid="mobile-configuration-content">节点配置</div>,
          review: <div>候选审阅</div>,
          run: <div>运行状态</div>,
          history: <div>历史记录</div>,
        }}
      />,
    ));
    expect([...container.querySelectorAll('[role="tab"][data-testid^="workflow-mobile-mode-"]')].map((tab) => tab.textContent)).toEqual([
      "画布",
      "AI",
      "变更",
      "运行",
    ]);
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-node-sidebar-separator"]')).toBeNull();

    await act(async () => (container.querySelector('[data-testid="workflow-mobile-mode-ai"]') as HTMLButtonElement).click());
    const input = container.querySelector('[data-testid="slot-input-移动 AI"]') as HTMLInputElement;
    await act(async () => setInputValue(input, "保留草稿"));
    await act(async () => (container.querySelector('[data-testid="workflow-mobile-mode-canvas"]') as HTMLButtonElement).click());
    await act(async () => (container.querySelector('[data-testid="workflow-mobile-mode-ai"]') as HTMLButtonElement).click());
    expect((container.querySelector('[data-testid="slot-input-移动 AI"]') as HTMLInputElement).value).toBe("保留草稿");
    expect(aiMount).toHaveBeenCalledTimes(1);

    const node = container.querySelector('[data-testid="canvas-node-check"]') as HTMLButtonElement;
    node.focus();
    await act(async () => requestWorkflowContextDockTab("configuration"));
    const sheet = container.querySelector('[data-testid="workflow-mobile-configuration-sheet"]');
    expect(sheet?.getAttribute("aria-modal")).toBe("true");
    expect(sheet?.textContent).toContain("节点配置");
    await act(async () => (sheet?.querySelector('[aria-label="关闭配置"]') as HTMLButtonElement).click());
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)));
    expect(document.activeElement).toBe(node);
  });

  it("honors a mobile node-configuration request that arrives before the inspector mounts", async () => {
    const renderDock = (configuration?: ReactNode) => (
      <WorkflowContextDock
        viewportWidth={390}
        configurationFocusTargetId="late-node"
        canvas={<button type="button" data-testid="canvas-node-late-node">Late node</button>}
        slots={{ ai: <div>AI</div>, configuration, review: <div>Review</div>, run: <div>Run</div> }}
      />
    );
    await act(async () => root.render(renderDock()));

    await act(async () => requestWorkflowContextDockTab("configuration"));
    expect(container.querySelector('[data-testid="workflow-mobile-configuration-sheet"]')).toBeNull();

    await act(async () => root.render(renderDock(<div data-testid="late-node-configuration">Late configuration</div>)));
    expect(container.querySelector('[data-testid="workflow-mobile-configuration-sheet"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="late-node-configuration"]')).not.toBeNull();
  });

  it("falls back to Changes when AI is unavailable without exposing configuration as a workflow tab", async () => {
    await act(async () => root.render(
      <WorkflowContextDock
        viewportWidth={390}
        canvas={<div>Canvas</div>}
        slots={{ configuration: <div>配置</div>, review: <div>审阅</div>, run: <div>运行</div>, history: <div>历史</div> }}
      />,
    ));
    expect(container.querySelector('[data-testid="workflow-context-tab-ai"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-mobile-mode-ai"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-changes"]')?.getAttribute("aria-selected")).toBe("true");
  });

  it("canonicalizes legacy review and history activation without rendering legacy tabs", async () => {
    await act(async () => root.render(
      <WorkflowContextDock
        canvas={<div>Canvas</div>}
        slots={{ ai: <div>AI</div>, review: <div>待审阅候选</div>, run: <div>运行中心</div> }}
        initialWorkflowDrawerOpen
      />,
    ));

    await act(async () => requestWorkflowContextDockTab("review"));
    expect(container.querySelector('[data-testid="workflow-context-tab-changes"]')?.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="workflow-context-panel-changes"]')?.textContent).toContain("待审阅候选");

    await act(async () => requestWorkflowContextDockTab("history"));
    expect(container.querySelector('[data-testid="workflow-context-tab-run"]')?.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="workflow-context-tab-review"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-history"]')).toBeNull();
  });

  it("keeps the legacy outline and Follow AI props compatible without rendering either surface", async () => {
    const onLocateChanges = vi.fn();
    await act(async () => root.render(
      <WorkflowContextDock
        outline={<nav data-testid="outline-slot">结构大纲</nav>}
        canvas={<div>Canvas</div>}
        slots={{ ai: <div>AI</div> }}
        generationActive
        changedObjectCount={3}
        onLocateChanges={onLocateChanges}
      />,
    ));
    expect(container.querySelector('[data-testid="workflow-context-outline"]')).toBeNull();
    expect(container.querySelector('[data-testid="outline-slot"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-outline-collapse"]')).toBeNull();
    expect(container.textContent).not.toContain("跟随 AI");
    expect(container.textContent).not.toContain("定位 3 处变更");
    expect(container.querySelector('[data-testid="workflow-follow-ai"]')).toBeNull();
    expect(onLocateChanges).not.toHaveBeenCalled();
  });
});
