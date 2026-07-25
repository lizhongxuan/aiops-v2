import { Bot, PanelRightClose, Play, Settings2, Sparkles, X } from "lucide-react";
import { useEffect, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export { WORKFLOW_CANVAS_LOCATE_EVENT } from "./workflowCanvasEvents";

export type WorkflowContextDockTab = "ai" | "configuration" | "changes" | "run";
export type WorkflowContextDockLegacyTab = "review" | "history";
type WorkflowContextDockRequestedTab = WorkflowContextDockTab | WorkflowContextDockLegacyTab;
export type WorkflowContextDockSlots = Partial<Record<WorkflowContextDockRequestedTab, ReactNode>>;
export type WorkflowMobileMode = "canvas" | "ai" | "changes" | "run";

export const WORKFLOW_CONTEXT_DOCK_OPEN_EVENT = "aiops:workflow-context-dock:open";
export const WORKFLOW_CONTEXT_DOCK_CLOSE_EVENT = "aiops:workflow-context-dock:close";
export const WORKFLOW_CANVAS_RESIZE_EVENT = "aiops:workflow-canvas:resize";

const MIN_DOCK_WIDTH = 320;
const MAX_DOCK_WIDTH = 640;
const MIN_CANVAS_WIDTH = 480;
const SEPARATOR_WIDTH = 8;
const DEFAULT_DOCK_WIDTH = 420;

export type WorkflowContextDockProps = {
  outline?: ReactNode;
  canvas: ReactNode;
  slots: WorkflowContextDockSlots;
  configurationFocusTargetId?: string;
  activation?: { tab: WorkflowContextDockRequestedTab; key: string };
  initialTab?: WorkflowContextDockRequestedTab;
  initialWorkflowDrawerOpen?: boolean;
  initialDockWidth?: number;
  viewportWidth?: number;
  generationActive?: boolean;
  changedObjectCount?: number;
  onTabChange?: (tab: WorkflowContextDockTab) => void;
  onDockWidthChange?: (width: number) => void;
  onLocateChanges?: () => void;
};

export function requestWorkflowContextDockTab(tab: WorkflowContextDockRequestedTab) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(WORKFLOW_CONTEXT_DOCK_OPEN_EVENT, { detail: { tab } }));
}

export function closeWorkflowContextDock() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(WORKFLOW_CONTEXT_DOCK_CLOSE_EVENT));
}

export function dockWidthBounds(viewportWidth: number, outlineWidth = 0) {
  const available = Math.max(0, Math.floor(viewportWidth) - Math.max(0, outlineWidth) - SEPARATOR_WIDTH - MIN_CANVAS_WIDTH);
  return { min: MIN_DOCK_WIDTH, max: Math.max(MIN_DOCK_WIDTH, Math.min(MAX_DOCK_WIDTH, available)) };
}

export function WorkflowContextDock({
  canvas,
  slots,
  configurationFocusTargetId,
  activation,
  initialTab = "ai",
  initialWorkflowDrawerOpen = false,
  initialDockWidth = DEFAULT_DOCK_WIDTH,
  viewportWidth,
  onTabChange,
  onDockWidthChange,
}: WorkflowContextDockProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const workflowDrawerRef = useRef<HTMLElement | null>(null);
  const workflowDrawerTriggerRef = useRef<HTMLElement | null>(null);
  const workflowDrawerFocusFrameRef = useRef<number>();
  const draggingRef = useRef(false);
  const activePointerIdRef = useRef<number | null>(null);
  const configurationTriggerRef = useRef<HTMLElement | null>(null);
  const hasAiWorkspace = slots.ai !== undefined && slots.ai !== null;
  const hasNodeWorkspace = slots.configuration !== undefined && slots.configuration !== null;
  const availableTabs = hasAiWorkspace ? WORKFLOW_TABS : WORKFLOW_TABS_WITHOUT_AI;
  const availableMobileModes = hasAiWorkspace ? MOBILE_MODES : MOBILE_MODES.filter((mode) => mode.id !== "ai");
  const normalizedInitialTab = canonicalWorkflowTab(initialTab);
  const effectiveInitialTab = availableTabs.some((tab) => tab.id === normalizedInitialTab) ? normalizedInitialTab : availableTabs[0].id;
  const [measuredWidth, setMeasuredWidth] = useState(() => viewportWidth || (typeof window !== "undefined" ? window.innerWidth : 1280));
  const responsiveWidth = viewportWidth ?? (typeof window !== "undefined" ? window.innerWidth : measuredWidth);
  const isMobile = responsiveWidth <= MOBILE_WORKSPACE_BREAKPOINT;
  const bounds = dockWidthBounds(viewportWidth || measuredWidth);
  const [dockWidth, setDockWidth] = useState(() => clamp(initialDockWidth, bounds));
  const [activeTab, setActiveTab] = useState<WorkflowContextDockTab>(effectiveInitialTab);
  const [workflowDrawerOpen, setWorkflowDrawerOpen] = useState(initialWorkflowDrawerOpen);
  const [mobileMode, setMobileMode] = useState<WorkflowMobileMode>("canvas");
  const [mobileConfigurationOpen, setMobileConfigurationOpen] = useState(false);
  const [mobileConfigurationRequested, setMobileConfigurationRequested] = useState(false);
  const currentDockWidth = clamp(dockWidth, bounds);

  const openMobileConfiguration = () => {
    const target = findConfigurationFocusTarget(configurationFocusTargetId) || document.activeElement;
    configurationTriggerRef.current = target instanceof HTMLElement ? target : null;
    setMobileMode("canvas");
    setMobileConfigurationOpen(true);
  };

  const closeMobileConfiguration = () => {
    setMobileConfigurationRequested(false);
    setMobileConfigurationOpen(false);
    const target = findConfigurationFocusTarget(configurationFocusTargetId) || configurationTriggerRef.current;
    window.requestAnimationFrame(() => {
      if (!(target instanceof HTMLElement) || !target.isConnected) return;
      if (target.tabIndex < 0) target.tabIndex = 0;
      target.focus({ preventScroll: true });
    });
  };

  const rememberWorkflowDrawerTrigger = () => {
    const activeElement = document.activeElement;
    if (activeElement instanceof HTMLElement && !workflowDrawerRef.current?.contains(activeElement)) {
      workflowDrawerTriggerRef.current = activeElement;
    }
  };

  const closeWorkflowDrawer = (restoreFocus = true) => {
    setWorkflowDrawerOpen(false);
    if (!restoreFocus) return;
    const target = workflowDrawerTriggerRef.current;
    if (workflowDrawerFocusFrameRef.current !== undefined) {
      window.cancelAnimationFrame(workflowDrawerFocusFrameRef.current);
    }
    workflowDrawerFocusFrameRef.current = window.requestAnimationFrame(() => {
      workflowDrawerFocusFrameRef.current = undefined;
      const fallback = document.querySelector<HTMLElement>('[data-testid="workflow-workbench-trigger"]');
      const focusTarget = target?.isConnected ? target : fallback;
      focusTarget?.focus({ preventScroll: true });
    });
  };

  useEffect(() => () => {
    if (workflowDrawerFocusFrameRef.current !== undefined) {
      window.cancelAnimationFrame(workflowDrawerFocusFrameRef.current);
    }
  }, []);

  const selectTab = (requestedTab: WorkflowContextDockRequestedTab, captureTrigger = false) => {
    const tab = canonicalWorkflowTab(requestedTab);
    if (tab === "configuration") {
      onTabChange?.("configuration");
      if (isMobile) {
        if (hasNodeWorkspace) {
          setMobileConfigurationRequested(false);
          openMobileConfiguration();
        } else {
          setMobileConfigurationRequested(true);
        }
      } else {
        closeWorkflowDrawer(false);
      }
      return;
    }
    if (!availableTabs.some((item) => item.id === tab)) return;
    if (captureTrigger) rememberWorkflowDrawerTrigger();
    setActiveTab(tab);
    setWorkflowDrawerOpen(true);
    onTabChange?.(tab);
    if (!isMobile) return;
    setMobileConfigurationRequested(false);
    setMobileConfigurationOpen(false);
    setMobileMode(tab);
  };

  useEffect(() => {
    const open = (event: Event) => {
      const tab = (event as CustomEvent<{ tab?: WorkflowContextDockRequestedTab }>).detail?.tab;
      if (tab) selectTab(tab, true);
    };
    const close = () => closeWorkflowDrawer(true);
    window.addEventListener(WORKFLOW_CONTEXT_DOCK_OPEN_EVENT, open);
    window.addEventListener(WORKFLOW_CONTEXT_DOCK_CLOSE_EVENT, close);
    return () => {
      window.removeEventListener(WORKFLOW_CONTEXT_DOCK_OPEN_EVENT, open);
      window.removeEventListener(WORKFLOW_CONTEXT_DOCK_CLOSE_EVENT, close);
    };
  });

  useEffect(() => {
    if (activation?.key) selectTab(activation.tab);
  }, [activation?.key, activation?.tab]);

  useEffect(() => {
    if (!isMobile) {
      setMobileConfigurationRequested(false);
      setMobileConfigurationOpen(false);
    }
  }, [isMobile]);

  useEffect(() => {
    if (!isMobile && hasNodeWorkspace) return;
    draggingRef.current = false;
    activePointerIdRef.current = null;
  }, [hasNodeWorkspace, isMobile]);

  useEffect(() => {
    const stopWindowDrag = (event: PointerEvent) => {
      if (activePointerIdRef.current !== null && activePointerIdRef.current !== event.pointerId) return;
      draggingRef.current = false;
      activePointerIdRef.current = null;
    };
    const stopWindowDragOnBlur = () => {
      draggingRef.current = false;
      activePointerIdRef.current = null;
    };
    window.addEventListener("pointerup", stopWindowDrag);
    window.addEventListener("pointercancel", stopWindowDrag);
    window.addEventListener("blur", stopWindowDragOnBlur);
    return () => {
      window.removeEventListener("pointerup", stopWindowDrag);
      window.removeEventListener("pointercancel", stopWindowDrag);
      window.removeEventListener("blur", stopWindowDragOnBlur);
    };
  }, []);

  useEffect(() => {
    if (!isMobile || !mobileConfigurationRequested || !hasNodeWorkspace) return;
    setMobileConfigurationRequested(false);
    openMobileConfiguration();
  }, [hasNodeWorkspace, isMobile, mobileConfigurationRequested]);

  useEffect(() => {
    if (!availableTabs.some((tab) => tab.id === activeTab)) setActiveTab(availableTabs[0].id);
    if (!hasAiWorkspace && mobileMode === "ai") setMobileMode("canvas");
  }, [activeTab, availableTabs, hasAiWorkspace, mobileMode]);

  useEffect(() => {
    if (!isMobile || !mobileConfigurationOpen) return undefined;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      closeMobileConfiguration();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [configurationFocusTargetId, isMobile, mobileConfigurationOpen]);

  useEffect(() => {
    if (isMobile || !workflowDrawerOpen) return undefined;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      closeWorkflowDrawer(true);
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [isMobile, workflowDrawerOpen]);

  useEffect(() => {
    const element = rootRef.current;
    if (!element || typeof ResizeObserver === "undefined") return undefined;
    let resizeFrame: number | undefined;
    const observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width;
      if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame);
      resizeFrame = window.requestAnimationFrame(() => {
        if (width && width > 0 && viewportWidth === undefined) setMeasuredWidth(width);
        emitCanvasResize(!isMobile && hasNodeWorkspace ? currentDockWidth : 0, "observer");
      });
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
      if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame);
    };
  }, [currentDockWidth, hasNodeWorkspace, isMobile, viewportWidth]);

  useEffect(() => {
    if (dockWidth !== currentDockWidth) setDockWidth(currentDockWidth);
    const occupiedWidth = !isMobile && hasNodeWorkspace ? currentDockWidth : 0;
    onDockWidthChange?.(occupiedWidth);
    emitCanvasResize(occupiedWidth, "dock");
  }, [currentDockWidth, dockWidth, hasNodeWorkspace, isMobile, onDockWidthChange]);

  const updateWidth = (value: number) => setDockWidth(clamp(value, bounds));
  const onSeparatorKeyDown = (event: ReactKeyboardEvent) => {
    const step = event.shiftKey ? 40 : 16;
    let next: number | undefined;
    if (event.key === "ArrowLeft") next = currentDockWidth + step;
    if (event.key === "ArrowRight") next = currentDockWidth - step;
    if (event.key === "Home") next = bounds.min;
    if (event.key === "End") next = bounds.max;
    if (next === undefined) return;
    event.preventDefault();
    const boundedNext = clamp(next, bounds);
    if (boundedNext === currentDockWidth && (event.key === "ArrowLeft" || event.key === "ArrowRight")) {
      updateWidth(currentDockWidth + (event.key === "ArrowLeft" ? -step : step));
      return;
    }
    updateWidth(boundedNext);
  };
  const onSeparatorPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || event.isPrimary === false) return;
    if (event.pointerType === "mouse" && (event.buttons & 1) === 0) return;
    draggingRef.current = true;
    activePointerIdRef.current = event.pointerId;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };
  const onSeparatorPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current || activePointerIdRef.current !== event.pointerId) return;
    if ((event.buttons & 1) === 0) {
      draggingRef.current = false;
      activePointerIdRef.current = null;
      return;
    }
    const rect = rootRef.current?.getBoundingClientRect();
    const right = rect?.right || (viewportWidth || measuredWidth);
    updateWidth(right - event.clientX);
  };
  const stopSeparatorDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (activePointerIdRef.current !== null && activePointerIdRef.current !== event.pointerId) return;
    draggingRef.current = false;
    activePointerIdRef.current = null;
    if (!event.currentTarget.hasPointerCapture || event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture?.(event.pointerId);
    }
  };
  const onSeparatorLostPointerCapture = () => {
    draggingRef.current = false;
    activePointerIdRef.current = null;
  };

  const style = {
    "--workflow-node-sidebar-width": `${currentDockWidth}px`,
  } as CSSProperties;
  const onWorkflowTabValueChange = (value: string) => {
    const tab = availableTabs.find((item) => item.id === value);
    if (tab) selectTab(tab.id);
  };
  const selectMobileMode = (mode: WorkflowMobileMode) => {
    setMobileConfigurationRequested(false);
    setMobileConfigurationOpen(false);
    setMobileMode(mode);
    if (mode !== "canvas") selectTab(mode);
  };

  return (
    <div
      ref={rootRef}
      className={`workflow-context-layout ${isMobile ? "is-mobile" : ""} ${!isMobile && hasNodeWorkspace ? "with-node-sidebar" : ""}`}
      style={style}
      data-testid="workflow-context-layout"
      data-mobile-mode={isMobile ? mobileMode : undefined}
    >
      {isMobile ? (
        <nav className="workflow-context-mobile-tabs" role="tablist" aria-label="移动端工作区">
          {availableMobileModes.map(({ id, label }) => (
            <button
              key={id}
              id={`workflow-mobile-mode-${id}`}
              type="button"
              role="tab"
              aria-selected={mobileMode === id}
              aria-controls={id === "canvas" ? "workflow-context-canvas-pane" : `workflow-context-panel-${id}`}
              tabIndex={mobileMode === id ? 0 : -1}
              data-testid={`workflow-mobile-mode-${id}`}
              onClick={() => selectMobileMode(id)}
            >
              {label}
            </button>
          ))}
        </nav>
      ) : null}
      <div
        id="workflow-context-canvas-pane"
        role={isMobile ? "tabpanel" : undefined}
        aria-labelledby={isMobile ? "workflow-mobile-mode-canvas" : undefined}
        className="workflow-context-canvas-pane"
        data-testid="workflow-context-canvas-pane"
      >
        {canvas}
      </div>
      {!isMobile && hasNodeWorkspace ? (
        <>
          <div
            role="separator"
            aria-label="调整画布与节点侧栏宽度"
            aria-orientation="vertical"
            aria-controls="workflow-node-sidebar"
            aria-valuemin={bounds.min}
            aria-valuemax={bounds.max}
            aria-valuenow={currentDockWidth}
            tabIndex={0}
            className="workflow-context-separator"
            data-testid="workflow-node-sidebar-separator"
            onKeyDown={onSeparatorKeyDown}
            onPointerDown={onSeparatorPointerDown}
            onPointerMove={onSeparatorPointerMove}
            onPointerUp={stopSeparatorDrag}
            onPointerCancel={stopSeparatorDrag}
            onLostPointerCapture={onSeparatorLostPointerCapture}
          />
          <aside
            id="workflow-node-sidebar"
            className="workflow-node-sidebar"
            aria-label="节点操作"
            aria-hidden={workflowDrawerOpen}
            inert={workflowDrawerOpen}
            data-testid="workflow-node-sidebar"
          >
            <header className="workflow-node-sidebar-label"><Settings2 className="h-4 w-4" aria-hidden="true" /><strong>节点操作</strong></header>
            <div className="workflow-node-sidebar-body">{slots.configuration}</div>
          </aside>
        </>
      ) : null}
      <aside
        ref={workflowDrawerRef}
        id="workflow-context-dock"
        className="workflow-context-dock"
        aria-label="工作流面板"
        aria-hidden={!isMobile && !workflowDrawerOpen}
        hidden={!isMobile && !workflowDrawerOpen}
        data-testid="workflow-context-dock"
        data-scope="workflow"
      >
        <Tabs
          value={activeTab}
          onValueChange={onWorkflowTabValueChange}
          className="workflow-context-tabs-root"
        >
          <header className="workflow-context-drawer-tabs-row">
            <TabsList
              variant="line"
              className="workflow-context-tabs"
              aria-label="工作流面板"
            >
              {availableTabs.map(({ id, label, Icon }) => (
                <TabsTrigger
                  key={id}
                  value={id}
                  id={`workflow-context-tab-${id}`}
                  aria-controls={`workflow-context-panel-${id}`}
                  data-testid={`workflow-context-tab-${id}`}
                >
                  <Icon className="h-4 w-4" aria-hidden="true" /><span>{label}</span>
                </TabsTrigger>
              ))}
            </TabsList>
            {!isMobile ? (
              <button
                type="button"
                data-testid="workflow-context-drawer-close"
                aria-label="收起工作流侧栏"
                aria-controls="workflow-context-dock"
                aria-expanded="true"
                onClick={() => closeWorkflowDrawer(true)}
              ><PanelRightClose className="h-4 w-4" aria-hidden="true" /></button>
            ) : null}
          </header>
          <div className="workflow-context-panels">
            {availableTabs.map(({ id, label }) => {
              const inactive = activeTab !== id;
              return (
                <TabsContent
                  key={id}
                  value={id}
                  forceMount
                  id={`workflow-context-panel-${id}`}
                  aria-labelledby={`workflow-context-tab-${id}`}
                  aria-label={label}
                  aria-hidden={inactive}
                  hidden={inactive}
                  inert={inactive}
                  className="workflow-context-panel"
                  data-testid={`workflow-context-panel-${id}`}
                >
                  {workflowPanelSlot(slots, id) ?? <p className="workflow-context-empty">此区域暂无内容。</p>}
                </TabsContent>
              );
            })}
          </div>
        </Tabs>
      </aside>
      {isMobile && mobileConfigurationOpen && slots.configuration ? (
        <section
          role="dialog"
          aria-modal="true"
          aria-labelledby="workflow-mobile-configuration-title"
          className="workflow-mobile-configuration-sheet"
          data-testid="workflow-mobile-configuration-sheet"
        >
          <header>
            <h2 id="workflow-mobile-configuration-title">节点配置</h2>
            <button type="button" aria-label="关闭配置" onClick={closeMobileConfiguration}><X aria-hidden="true" /></button>
          </header>
          <div className="workflow-mobile-configuration-body">{slots.configuration}</div>
        </section>
      ) : null}
    </div>
  );
}

function emitCanvasResize(dockWidth: number, source: "dock" | "observer") {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(WORKFLOW_CANVAS_RESIZE_EVENT, { detail: { dockWidth, source } }));
}

function clamp(value: number, bounds: { min: number; max: number }) {
  return Math.min(bounds.max, Math.max(bounds.min, Math.round(value)));
}

const WORKFLOW_TABS = [
  { id: "ai", label: "AI 协作", Icon: Bot },
  { id: "changes", label: "变更", Icon: Sparkles },
  { id: "run", label: "运行", Icon: Play },
] as const;

const WORKFLOW_TABS_WITHOUT_AI = WORKFLOW_TABS.filter((tab) => tab.id !== "ai");

const MOBILE_WORKSPACE_BREAKPOINT = 820;
const MOBILE_MODES = [
  { id: "canvas", label: "画布" },
  { id: "ai", label: "AI" },
  { id: "changes", label: "变更" },
  { id: "run", label: "运行" },
] as const;

function canonicalWorkflowTab(tab: WorkflowContextDockRequestedTab): WorkflowContextDockTab {
  if (tab === "review") return "changes";
  if (tab === "history") return "run";
  return tab;
}

function workflowPanelSlot(slots: WorkflowContextDockSlots, tab: Exclude<WorkflowContextDockTab, "configuration">) {
  if (tab === "ai") return slots.ai;
  if (tab === "run") return slots.run;
  const review = slots.changes ?? slots.review;
  if (slots.changes !== undefined || slots.history === undefined) return review;
  return (
    <>
      {review}
      <div data-testid="workflow-legacy-durable-history">{slots.history}</div>
    </>
  );
}

function findConfigurationFocusTarget(nodeId?: string): HTMLElement | null {
  if (!nodeId || typeof document === "undefined") return null;
  const testId = `canvas-node-${nodeId}`;
  return [...document.querySelectorAll<HTMLElement>('[data-testid^="canvas-node-"]')]
    .find((element) => element.dataset.testid === testId) || null;
}
