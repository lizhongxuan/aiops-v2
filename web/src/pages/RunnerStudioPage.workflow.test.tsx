import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider as BaseAppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";
import { createAiopsQueryClient } from "@/app/queryClient";
import { RunnerStudioPage, workflowOperationForCandidateGraphChange } from "@/pages/RunnerStudioPage";
import { workflowCommandForGraphChange } from "@/runner/WorkflowEditorShell";
import { WorkflowTransportBoundary } from "@/runner/transport/WorkflowTransportBoundary";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import { resetAiopsTransportStateCacheForTest, setCachedAiopsTransportState } from "@/transport/aiopsTransportStateCache";
import { AppRouter } from "@/router";

function AppShellChromeProvider({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={createAiopsQueryClient()}>
      <BaseAppShellChromeProvider>{children}</BaseAppShellChromeProvider>
    </QueryClientProvider>
  );
}

function DirectPageChromeProvider({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={createAiopsQueryClient()}>
      <BaseAppShellChromeProvider>
        <RegisteredHeader />
        {children}
      </BaseAppShellChromeProvider>
    </QueryClientProvider>
  );
}

function RegisteredHeader() {
  const { headerContent } = useAppShellChrome();
  return <>{headerContent}</>;
}

const workflowGraph = {
  version: "v1",
  workflow: { name: "redis-memory", title: "Redis Memory" },
  nodes: [
    { id: "start", type: "start", label: "Start", position: { x: 80, y: 160 }, ports: [{ id: "next", type: "output", label: "下一步" }], ui: {} },
    { id: "check", type: "action", label: "检查内存", position: { x: 360, y: 160 }, ports: [{ id: "in", type: "input" }, { id: "next", type: "output" }], ui: {}, step: { name: "检查内存", action: "script.python", args: { script: "print('ok')" } } },
    { id: "end", type: "end", label: "End", position: { x: 680, y: 160 }, ports: [{ id: "in", type: "input" }], ui: {} },
  ],
  edges: [
    { id: "start-check", source: "start", source_port: "next", target: "check", target_port: "in", kind: "next" },
    { id: "check-end", source: "check", source_port: "next", target: "end", target_port: "in", kind: "next" },
  ],
};

const PUBLISHED_RESOURCE_VERSION = `sha256:${"1".repeat(64)}`;
const PUBLISHED_SEMANTIC_HASH = `sha256:${"2".repeat(64)}`;
const CURRENT_RESOURCE_VERSION = `sha256:${"3".repeat(64)}`;
const CURRENT_SEMANTIC_HASH = `sha256:${"4".repeat(64)}`;

function workflowFixture(name: string, title = name, status = "draft", updatedAt = "2026-07-06T04:20:00Z") {
  return {
    name,
    title,
    status,
    version: `rev-${name}`,
    updated_at: updatedAt,
    graph: { ...workflowGraph, workflow: { name, title } },
    validation_result: { valid: true, errors: [], warnings: [] },
  };
}

function cacheWorkflowCandidate(
  workflowId = "redis-memory",
  operations: Array<{ op: "update_node"; nodeId: string; set: Record<string, string> }> = [
    { op: "update_node", nodeId: "check", set: { label: "候选容量检查" } },
  ],
) {
  const workflowKey = { tenantId: "tenant-a", workflowId };
  const state = createInitialAiopsTransportState(`workflow:${workflowId}`);
  state.sessionId = "session-candidate-switch";
  state.updatedAt = "2026-07-21T08:00:00Z";
  state.turnOrder = ["turn-candidate-switch"];
  state.turns["turn-candidate-switch"] = {
    id: "turn-candidate-switch",
    status: "completed",
    workflowContext: {
      workflowId,
      revision: { resourceVersion: "rev-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      selection: { nodeIds: [], edgeIds: [] },
      environment: { name: "staging", bindingVersion: "env-1" },
    },
    blockOrder: ["candidate-artifact-block"],
    blocksById: {
      "candidate-artifact-block": {
        id: "candidate-artifact-block",
        type: "artifact",
        kind: "assistant",
        status: "completed",
        artifact: {
          id: "candidate-artifact-switch",
          type: "workflow_patch_preview",
          renderer: "workflow.change.preview.v2",
          schemaVersion: "aiops.workflow.edit.v2",
          inlineData: {
            schemaVersion: "aiops.workflow.edit.v2",
            isError: false,
            content: [{ type: "text", text: "更新检查节点" }],
            structuredContent: {
              kind: "preview",
              changeSet: {
                schemaVersion: "aiops.workflow.edit.v2",
                id: "change-switch-1",
                workflowKey,
                candidateId: "candidate-switch-1",
                candidateRevision: 1,
                actor: "agent",
                intent: "更新检查节点",
                base: { resourceVersion: "rev-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
                operations,
                previewHash: "preview-switch-1",
                validation: { kind: "validation", basis: workflowBasis(), valid: true, diagnostics: [] },
                state: "previewed",
                createdAt: "2026-07-21T07:59:00Z",
                expiresAt: "2026-07-22T07:59:00Z",
              },
              candidate: {
                id: "candidate-switch-1",
                workflowKey,
                currentChangeSetId: "change-switch-1",
                currentRevision: 1,
                currentPreviewHash: "preview-switch-1",
                state: "ready",
                expiresAt: "2026-07-22T07:59:00Z",
              },
              diff: { touchedPaths: ["nodes/check/label"] },
            },
          },
        },
      },
    },
  };
  setCachedAiopsTransportState(`workflow:${workflowId}`, state);
}

function jsonResponse(payload: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (key: string) => key.toLowerCase() === "content-type" ? "application/json" : "" },
    text: () => Promise.resolve(JSON.stringify(payload)),
  } as Response);
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function waitForElement<T extends Element>(container: ParentNode, selector: string): Promise<T> {
  for (let index = 0; index < 30; index += 1) {
    const element = container.querySelector<T>(selector);
    if (element) return element;
    await flush();
  }
  throw new Error(`Missing element: ${selector}`);
}

function click(element: Element | null | undefined) {
  if (!element) throw new Error("Missing clickable element");
  element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
}

function clickRadixTab(element: Element | null | undefined) {
  if (!element) throw new Error("Missing tab element");
  element.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0, buttons: 1 }));
  element.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
  element.dispatchEvent(new MouseEvent("click", { bubbles: true, button: 0 }));
}

function doubleClick(element: Element | null | undefined) {
  if (!element) throw new Error("Missing double-clickable element");
  element.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
}

function changeTextarea(textarea: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  setter?.call(textarea, value);
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

function changeInput(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function changeSelect(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}

describe("RunnerStudioPage workflow UI", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    if (!globalThis.CSS) Object.defineProperty(globalThis, "CSS", { configurable: true, value: {} });
    if (!globalThis.CSS.escape) Object.defineProperty(globalThis.CSS, "escape", { configurable: true, value: (value: string) => String(value).replace(/[^a-zA-Z0-9_-]/g, "\\$&") });
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({
          workflows: [{
            name: "redis-memory",
            title: "Redis Memory",
            status: "draft",
            version: "rev-1",
            updated_at: "2026-07-06T04:20:00Z",
            graph: workflowGraph,
            validation_result: { valid: true, errors: [], warnings: [] },
          }],
        });
      }
      if (path === "/api/runner-studio/actions") {
        return jsonResponse({ items: [] });
      }
      if (path === "/api/runner-studio/workflows/redis-memory") {
        return jsonResponse({ name: "redis-memory" });
      }
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });
    resetAiopsTransportStateCacheForTest();
    localStorage.clear();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    vi.restoreAllMocks();
    resetAiopsTransportStateCacheForTest();
    container.remove();
    localStorage.clear();
  });

  it("shows workflow last modified time and removes the row after confirmed delete", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const library = await waitForElement<HTMLElement>(container, '[data-testid="runner-workflow-library"]');
    expect(library.textContent).toContain("Redis Memory");
    expect(library.textContent).toContain("最后修改");
    expect(library.textContent).toContain("2026");

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-delete-workflow-redis-memory"]')));
    const confirmDialog = await waitForElement<HTMLElement>(container, '[data-testid="workflow-delete-confirm"]');
    expect(confirmDialog.textContent).toContain("Redis Memory");
    await act(async () => click(confirmDialog.querySelector('[data-testid="workflow-delete-confirm-submit"]')));
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/runner-studio/workflows/redis-memory",
      expect.objectContaining({ method: "DELETE" }),
    );
    expect(container.querySelector('[data-testid="runner-delete-workflow-redis-memory"]')).toBeNull();
    expect(container.textContent).toContain("暂无工作流");
  });

  it("filters workflows from the top bar and paginates the library", async () => {
    const workflows = [
      ...Array.from({ length: 7 }, (_, index) => workflowFixture(`pg-backup-nightly-${index + 1}`, `PG Backup Nightly ${index + 1}`)),
      ...Array.from({ length: 6 }, (_, index) => workflowFixture(`redis-memory-check-${index + 1}`, `Redis Memory Check ${index + 1}`)),
    ];
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((input) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") return jsonResponse({ workflows });
      if (path === "/api/runner-studio/actions") return jsonResponse({ items: [] });
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const library = await waitForElement<HTMLElement>(container, '[data-testid="runner-workflow-library"]');
    expect(library.textContent).toContain("PG Backup Nightly 1");
    expect(library.textContent).toContain("PG Backup Nightly 6");
    expect(library.textContent).not.toContain("PG Backup Nightly 7");
    expect(library.textContent).toContain("第 1 / 3 页");

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-workflow-page-next"]')));
    expect(library.textContent).toContain("PG Backup Nightly 7");
    expect(library.textContent).not.toContain("PG Backup Nightly 1");
    expect(library.textContent).toContain("第 2 / 3 页");

    const searchInput = await waitForElement<HTMLInputElement>(container, '[data-testid="runner-workflow-search"]');
    await act(async () => changeInput(searchInput, "redis memory"));
    expect(library.textContent).toContain("Redis Memory Check 1");
    expect(library.textContent).toContain("Redis Memory Check 6");
    expect(library.textContent).not.toContain("PG Backup Nightly 7");
    expect(library.textContent).toContain("第 1 / 1 页");
    expect(library.textContent).toContain("6 个结果");
  });

  it("creates a new workflow directly from the top bar without opening the manager", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    expect(container.textContent).not.toContain("管理工作流");
    const createButton = await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-create-workflow"]');
    expect(createButton.textContent).toContain("新建工作流");

    await act(async () => click(createButton));

    expect(container.querySelector('[data-testid="workflow-manager-modal"]')).toBeNull();
    const commandBar = await waitForElement<HTMLElement>(container, '[data-testid="workflow-command-bar"]');
    expect(commandBar.textContent).toContain("新建工作流");
    expect(commandBar.textContent).not.toContain("draft");
    expect(commandBar.querySelector('[data-testid="runner-save-state"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-studio-canvas"]')).toBeTruthy();
  });

  it("does not revive AI draft generation from the legacy create query", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner?workflow_ai=create&prompt=legacy"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    expect(await waitForElement(container, '[data-testid="runner-create-workflow"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="workflow-ai-drawer"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-ai-transport-panel"]')).toBeNull();
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input).includes("/workflow-ai/"))).toBe(false);
  });

  it("renders the route workflow as static Command Bar identity with one Back action", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const commandBar = await waitForElement<HTMLElement>(container, '[data-testid="workflow-command-bar"]');
    expect(commandBar.textContent).toContain("Redis Memory");
    expect(container.querySelector('[data-testid="runner-workflow-title-input"]')).toBeNull();
    expect(commandBar.querySelectorAll('[data-testid="runner-back-to-library"]')).toHaveLength(1);

    await act(async () => click(commandBar.querySelector('[data-testid="runner-back-to-library"]')));
    expect(await waitForElement(container, '[data-testid="runner-workflow-library"]')).toBeTruthy();
  });

  it("adds a default local target to existing runnable nodes that were missing targets", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => doubleClick(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const targetLabelsInput = await waitForElement<HTMLInputElement>(container, '[data-testid="runner-node-target-labels-input"]');
    expect(targetLabelsInput.value).toBe("local");
  });

  it("keeps a node run in the current-session Run inspector instead of a separate history panel", async () => {
    const runId = "run-detail-ui";
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({
          workflows: [{
            name: "redis-memory",
            title: "Redis Memory",
            status: "draft",
            version: "rev-1",
            updated_at: "2026-07-06T04:20:00Z",
            graph: workflowGraph,
            validation_result: { valid: true, errors: [], warnings: [] },
          }],
        });
      }
      if (path === "/api/runner-studio/actions") return jsonResponse({ items: [] });
      if (path === "/api/runner-studio/workflows/redis-memory/graph") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : {};
        return jsonResponse({
          name: "redis-memory",
          title: "Redis Memory",
          status: "draft",
          version: "rev-2",
          graph: body.graph || workflowGraph,
          validation_result: { valid: true, errors: [], warnings: [] },
        });
      }
      if (path === "/api/runner-studio/runs") {
        return jsonResponse({ run_id: runId, status: "running" });
      }
      if (path === `/api/runner-studio/runs/${runId}/events/history`) {
        return jsonResponse([
          { type: "run_start", run_id: runId, status: "running", timestamp: "2026-07-08T03:00:00Z" },
          {
            type: "host_result",
            run_id: runId,
            step: "check",
            node_id: "check",
            status: "success",
            message: "打印hello",
            output: { stdout: "hello\n", stderr: "" },
          },
          { type: "step_finish", run_id: runId, step: "check", node_id: "check", status: "success" },
          { type: "run_finish", run_id: runId, status: "success", timestamp: "2026-07-08T03:00:01Z" },
        ]);
      }
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => doubleClick(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const inspector = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel"]');
    await act(async () => {
      click(inspector.querySelector('[data-testid="runner-node-panel-run"]'));
      await new Promise((resolve) => setTimeout(resolve, 800));
    });

    const runRequest = vi.mocked(globalThis.fetch).mock.calls.find(([input]) => String(input) === "/api/runner-studio/runs");
    expect(JSON.parse(String(runRequest?.[1]?.body || "{}"))).toMatchObject({
      workflow_name: "redis-memory",
      node_id: "check",
      run_scope: "single_node",
    });

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    await act(async () => click(await waitForElement(container, '[data-testid="workflow-context-tab-run"]')));
    const runPanel = await waitForElement<HTMLElement>(container, '[data-testid="workflow-context-panel-run"]');
    const sessionInspector = await waitForElement<HTMLElement>(runPanel, '[data-testid="workflow-run-inspector"]');
    expect(sessionInspector.textContent).toContain("当前会话");
    expect(sessionInspector.textContent).toContain("节点 check");
    expect(runPanel.querySelector('[data-testid="workflow-history-panel"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-run-history-panel"]')).toBeNull();

    const sessionRow = await waitForElement<HTMLButtonElement>(sessionInspector, "ol button");
    await act(async () => click(sessionRow));
    const detail = await waitForElement<HTMLElement>(sessionInspector, '[data-testid="workflow-run-details"]');
    expect(detail.textContent).toContain("check");
    expect(detail.textContent).toContain("打印hello");
  });

  it("retries a failed current-session run with the exact original target", async () => {
    const runIds = ["run-retry-source", "run-retry-result"];
    const runRequests: Record<string, unknown>[] = [];
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({
          workflows: [workflowFixture("redis-memory", "Redis Memory")],
        });
      }
      if (path === "/api/runner-studio/actions") return jsonResponse({ items: [] });
      if (path === "/api/runner-studio/workflows/redis-memory/graph") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : {};
        return jsonResponse({
          ...workflowFixture("redis-memory", "Redis Memory"),
          version: "rev-2",
          graph: body.graph || workflowGraph,
        });
      }
      if (path === "/api/runner-studio/runs") {
        runRequests.push(JSON.parse(String(init?.body || "{}")));
        const runId = runIds[Math.min(runRequests.length - 1, runIds.length - 1)];
        return jsonResponse({ run_id: runId, status: "running" });
      }
      if (path === `/api/runner-studio/runs/${runIds[0]}/events/history`) {
        return jsonResponse([
          { type: "run_start", run_id: runIds[0], status: "running", timestamp: "2026-07-08T03:00:00Z" },
          { type: "node.failed", run_id: runIds[0], node_id: "check", status: "failed", error: "remote unavailable", timestamp: "2026-07-08T03:00:01Z" },
          { type: "run.failed", run_id: runIds[0], status: "failed", error: "remote unavailable", timestamp: "2026-07-08T03:00:01Z" },
        ]);
      }
      if (path === `/api/runner-studio/runs/${runIds[1]}/events/history`) {
        return jsonResponse([
          { type: "run_start", run_id: runIds[1], status: "running", timestamp: "2026-07-08T03:01:00Z" },
          { type: "step_finish", run_id: runIds[1], node_id: "check", status: "success", timestamp: "2026-07-08T03:01:01Z" },
          { type: "run_finish", run_id: runIds[1], status: "success", timestamp: "2026-07-08T03:01:01Z" },
        ]);
      }
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => doubleClick(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const inspector = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel"]');
    await act(async () => {
      click(inspector.querySelector('[data-testid="runner-node-panel-run"]'));
      await new Promise((resolve) => setTimeout(resolve, 800));
    });

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    await act(async () => click(await waitForElement(container, '[data-testid="workflow-context-tab-run"]')));
    const sessionInspector = await waitForElement<HTMLElement>(container, '[data-testid="workflow-run-inspector"]');
    await act(async () => click(await waitForElement(sessionInspector, "ol button")));
    const retry = await waitForElement<HTMLButtonElement>(sessionInspector, '[data-testid="workflow-run-retry"]');

    await act(async () => {
      click(retry);
      await new Promise((resolve) => setTimeout(resolve, 800));
    });

    expect(runRequests).toHaveLength(2);
    expect(runRequests[1]).toEqual(runRequests[0]);
    expect(runRequests[1]).toMatchObject({
      workflow_name: "redis-memory",
      node_id: "check",
      run_scope: "single_node",
      triggered_by: "ui",
      risk_acknowledged: true,
    });
  });

  it("opens only the authoritative AssistantTransport panel without changing the graph", async () => {
    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    const beforeNodeIds = Array.from(container.querySelectorAll('[data-testid^="canvas-node-"]'))
      .map((node) => node.getAttribute("data-testid"));
    const aiButton = await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-workbench-trigger"]');
    await act(async () => click(aiButton));

    const panel = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-transport-panel"]');
    expect(container.querySelectorAll('[data-testid="workflow-context-dock"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="workflow-context-tab-ai"]')?.getAttribute("aria-selected")).toBe("true");
    expect(panel.textContent).toContain("AI 协作");
    expect(container.querySelector('[data-testid="workflow-ai-drawer"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-event-drawer"]')).toBeNull();
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input).includes("/workflow-ai/"))).toBe(false);
    expect(Array.from(container.querySelectorAll('[data-testid^="canvas-node-"]')).map((node) => node.getAttribute("data-testid")))
      .toEqual(beforeNodeIds);

    await act(async () => click(container.querySelector('[data-testid="workflow-context-drawer-close"]')));
    expect(container.querySelector('[data-testid="workflow-ai-transport-panel"]')).not.toBeNull();
    expect((container.querySelector('[data-testid="workflow-context-dock"]') as HTMLElement).hidden).toBe(true);
    expect(container.querySelector('[data-testid="workflow-workbench-trigger"]')).not.toBeNull();
  });

  it("locks Workflow AI to the workflow opened by the route instead of offering a second workflow selector", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    const otherGraph = { ...workflowGraph, workflow: { name: "other-workflow", title: "Other Workflow" } };
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({ workflows: [workflowFixture("redis-memory", "Redis Memory"), workflowFixture("other-workflow", "Other Workflow")] });
      }
      if (path === "/api/runner-studio/workflows/other-workflow/graph") return jsonResponse({ graph: otherGraph });
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });
    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });
    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    const panel = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-transport-panel"]');
    expect(panel.querySelector('[data-testid="workflow-scope-workflow"]')).toBeNull();
    expect(panel.querySelector('[aria-label="本次发送范围"]')).toBeNull();
    expect((await waitForElement(container, '[data-testid="workflow-command-bar"]')).textContent).toContain("Redis Memory");
  });

  it("requires an explicit keep or discard choice before switching away from an active Candidate", async () => {
    cacheWorkflowCandidate();
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    const otherGraph = { ...workflowGraph, workflow: { name: "other-workflow", title: "Other Workflow" } };
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({ workflows: [workflowFixture("redis-memory", "Redis Memory"), workflowFixture("other-workflow", "Other Workflow")] });
      }
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflows/other-workflow/graph") return jsonResponse({ graph: otherGraph });
      if (path === "/api/runner-studio/workflow-candidate-actions" && init?.method === "POST") {
        return jsonResponse({
          kind: "candidate_discarded",
          schemaVersion: "aiops.workflow.edit.v2",
          candidate: {
            id: "candidate-switch-1",
            workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
            currentChangeSetId: "change-switch-1",
            currentRevision: 1,
            currentPreviewHash: "preview-switch-1",
            state: "abandoned",
            expiresAt: "2026-07-22T07:59:00Z",
          },
        });
      }
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
              <Route path="/runner" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    await waitForElement(container, '[data-testid="workflow-change-review"]');
    const backButton = await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-back-to-library"]');
    await act(async () => click(backButton));
    const dialog = await waitForElement<HTMLElement>(container, '[data-testid="workflow-candidate-switch-confirm"]');
    expect(dialog.textContent).toContain("保留还是放弃");
    expect(dialog.textContent).toContain("工作流列表");
    expect(container.querySelector('[data-testid="workflow-command-bar"]')?.textContent).toContain("Redis Memory");

    await act(async () => click(dialog.querySelector('[data-testid="workflow-candidate-switch-keep"]')));
    expect(container.querySelector('[data-testid="workflow-candidate-switch-confirm"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-command-bar"]')?.textContent).toContain("Redis Memory");
    expect(container.querySelector('[data-testid="workflow-change-review"]')).toBeTruthy();

    await act(async () => click(await waitForElement(container, '[data-testid="runner-back-to-library"]')));
    const discardDialog = await waitForElement<HTMLElement>(container, '[data-testid="workflow-candidate-switch-confirm"]');
    await act(async () => {
      click(discardDialog.querySelector('[data-testid="workflow-candidate-switch-discard"]'));
      await flush();
    });
    expect(await waitForElement(container, '[data-testid="runner-workflow-library"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="workflow-command-bar"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-candidate-switch-confirm"]')).toBeNull();

    const discardCalls = vi.mocked(globalThis.fetch).mock.calls.filter(([input]) => String(input) === "/api/runner-studio/workflow-candidate-actions");
    expect(discardCalls).toHaveLength(1);
    expect(JSON.parse(String(discardCalls[0][1]?.body))).toEqual({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "redis-memory",
      candidateId: "candidate-switch-1",
      candidateRevision: 1,
      changeSetId: "change-switch-1",
      previewHash: "preview-switch-1",
      expected: { resourceVersion: "rev-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      idempotencyKey: expect.any(String),
      action: "discard",
    });
  });

  it("keeps Changes navigation visible without an outline and keeps node operations separate from workflow tabs", async () => {
    cacheWorkflowCandidate("redis-memory", [
      { op: "update_node", nodeId: "check", set: { label: "候选容量检查" } },
      { op: "update_node", nodeId: "end", set: { label: "候选结束节点" } },
    ]);

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    const review = await waitForElement<HTMLElement>(container, '[data-testid="workflow-change-review"]');
    const changesTab = await waitForElement<HTMLElement>(container, '[data-testid="workflow-context-tab-changes"]');
    expect(changesTab.getAttribute("aria-selected")).toBe("true");

    await act(async () => click(review.querySelector('[data-testid="workflow-review-next"]')));
    expect(review.textContent).toContain("2 / 2");
    expect(changesTab.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="workflow-context-outline"]')).toBeNull();

    await act(async () => click(review.querySelector('[data-testid="workflow-review-previous"]')));
    expect(review.textContent).toContain("1 / 2");
    expect(changesTab.getAttribute("aria-selected")).toBe("true");

    await act(async () => click(review.querySelector("ol li:nth-child(2) button")));
    expect(review.textContent).toContain("2 / 2");
    expect(changesTab.getAttribute("aria-selected")).toBe("true");

    await act(async () => click(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-changes"]')?.getAttribute("aria-selected")).toBe("true");
  });

  it("fails closed when the server omits or malforms the Workflow AI capability while keeping manual editing available", async () => {
    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowCapabilities: { workflow_agent_ui: true } } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    expect(await waitForElement(container, '[data-testid="runner-studio-canvas"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="workflow-workbench-trigger"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-ai"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-ai-transport-panel"]')).toBeNull();
    const commandBar = await waitForElement<HTMLElement>(container, '[data-testid="workflow-command-bar"]');
    expect(container.querySelector('[data-testid="workflow-primary-action"]')).toBeNull();
    expect(commandBar.textContent).toContain("检查依据未同步");
  });

  it("keeps node configuration independent from the collapsible workflow drawer and preserves its draft", async () => {
    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });
    const workflowDrawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-context-dock"]');
    expect(container.querySelectorAll('[data-testid="workflow-context-dock"]')).toHaveLength(1);
    expect(workflowDrawer.hidden).toBe(true);
    await act(async () => doubleClick(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const inspector = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel"]');
    const nodeSidebar = await waitForElement<HTMLElement>(container, '[data-testid="workflow-node-sidebar"]');
    expect(container.querySelector('[data-testid="runner-node-panel-modal"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(nodeSidebar.getAttribute("aria-hidden")).not.toBe("true");
    expect(workflowDrawer.hidden).toBe(true);
    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-tab-script"]')));
    const editor = await waitForElement<HTMLTextAreaElement>(inspector, '[data-testid="runner-node-script-editor"]');
    await act(async () => changeTextarea(editor, "print('dock draft')"));

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    expect(workflowDrawer.hidden).toBe(false);
    expect(container.querySelector('[data-testid="workflow-context-tab-ai"]')?.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="runner-node-panel"]')).not.toBeNull();
    expect(nodeSidebar.getAttribute("aria-hidden")).toBe("true");
    expect(nodeSidebar.hasAttribute("inert")).toBe(true);
    expect((container.querySelector('[data-testid="runner-node-script-editor"]') as HTMLTextAreaElement).value).toBe("print('dock draft')");

    await act(async () => click(container.querySelector('[data-testid="workflow-context-drawer-close"]')));
    expect(workflowDrawer.hidden).toBe(true);
    expect(nodeSidebar.getAttribute("aria-hidden")).not.toBe("true");
    expect(nodeSidebar.hasAttribute("inert")).toBe(false);
    expect((container.querySelector('[data-testid="runner-node-script-editor"]') as HTMLTextAreaElement).value).toBe("print('dock draft')");

    await act(async () => {
      click(inspector.querySelector('[data-testid="runner-node-panel-run"]'));
      await flush();
    });
    expect(workflowDrawer.hidden).toBe(true);
    expect(container.querySelector('[data-testid="workflow-context-tab-run"]')?.getAttribute("aria-selected")).not.toBe("true");

    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-close"]')));
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-node-panel"]')).toBeNull();

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    expect(workflowDrawer.hidden).toBe(false);
    await act(async () => clickRadixTab(await waitForElement(container, '[data-testid="workflow-context-tab-run"]')));
    expect(container.querySelector('[data-testid="workflow-context-tab-run"]')?.getAttribute("aria-selected")).toBe("true");
    expect(container.querySelector('[data-testid="runner-run-drawer"]')).toBeNull();
  });

  it("locates a blocking diagnostic and focuses its exact Inspector field", async () => {
    const projected = {
      ...workspaceProjection(),
      validationArtifacts: [{
        artifactId: "validation-script-required",
        basis: workflowBasis(),
        status: "saved_with_issues" as const,
        diagnostics: [{
          code: "ACTION_ARGUMENT_REQUIRED",
          severity: "error",
          message: "raw server validation detail",
          nodeId: "check",
          field: "args.script",
        }],
        issuedAt: "2026-07-21T00:00:02Z",
      }],
    };
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      if (String(input) === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${String(input)}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: projected, workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-context-tab-changes"]')));
    const issue = await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-validation-issue-committed-0"]');
    expect(issue.dataset.workflowNodeId).toBe("check");
    expect(issue.dataset.workflowField).toBe("args.script");
    await act(async () => {
      click(issue);
      await new Promise<void>((resolve) => window.requestAnimationFrame(() => window.requestAnimationFrame(() => resolve())));
    });

    const editor = await waitForElement<HTMLTextAreaElement>(container, '[data-testid="runner-node-script-editor"]');
    expect(container.querySelector('[data-testid="workflow-context-tab-configuration"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-node-sidebar"]')).not.toBeNull();
    expect((container.querySelector('[data-testid="workflow-context-dock"]') as HTMLElement).hidden).toBe(true);
    expect(container.querySelector('[data-testid="runner-node-panel-title"]')?.textContent).toContain("检查内存");
    expect(container.querySelector('[data-testid="runner-node-panel-tab-script"]')?.className).toContain("active");
    expect(editor.dataset.workflowDiagnosticFocus).toBe("args.script");
    expect(document.activeElement).toBe(editor);
  });

  it("renders authoritative lifecycle facts and submits dry-run without sending the Graph", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflow-dry-run") return jsonResponse({ status: "dry_run_passed" });
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    const header = await waitForElement<HTMLElement>(container, '[data-testid="workflow-lifecycle-header"]');
    const commandBar = await waitForElement<HTMLElement>(header, '[data-testid="workflow-command-bar"]');
    expect(header.dataset.placement).toBe("topbar");
    expect(container.querySelector('[data-testid="runner-studio-shell"] [data-testid="workflow-lifecycle-header"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-studio-shell"] .runner-studio-workspace')?.classList.contains("with-lifecycle-header")).toBe(false);
    expect(commandBar.textContent).toContain("Redis Memory");
    expect(commandBar.textContent).toContain("静态检查通过");
    expect(container.querySelectorAll('[data-testid="workflow-command-bar"]')).toHaveLength(1);
    expect(container.querySelectorAll('[data-testid="workflow-primary-action"]')).toHaveLength(1);
    expect(container.querySelector('[data-testid="runner-toolbar-save"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-toolbar-run"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-toolbar-run-details"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-toolbar-ai-generate"]')).toBeNull();
    expect(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')).toBeTruthy();
    const primary = await waitForElement<HTMLButtonElement>(header, '[data-testid="workflow-primary-action"]');
    expect(primary.dataset.action).toBe("dry_run");
    await act(async () => {
      click(primary);
      await flush();
    });

    const request = vi.mocked(globalThis.fetch).mock.calls.find(([input]) => String(input) === "/api/runner-studio/workflow-dry-run");
    expect(request).toBeTruthy();
    const body = JSON.parse(String(request?.[1]?.body || "{}"));
    expect(body).toEqual({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "redis-memory",
      expectedBasis: workflowBasis(),
      environment: "staging",
    });
    expect(body).not.toHaveProperty("graph");
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input).includes("/workflows/graph/dry-run"))).toBe(false);
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input) === "/api/runner-studio/workflow-published-run")).toBe(false);
  });

  it("checks only the exact saved revision projected into the Primary target", async () => {
    const projected = {
      ...workspaceProjection(),
      validationArtifacts: [],
    };
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflows/redis-memory/validate") {
        return jsonResponse({ valid: true, status: "validated", validated_graph_hash: "semantic-1" });
      }
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: projected, workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    const primary = await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-primary-action"]');
    expect(primary.dataset.action).toBe("check");
    await act(async () => {
      click(primary);
      await flush();
    });

    const request = vi.mocked(globalThis.fetch).mock.calls.find(([input]) =>
      String(input) === "/api/runner-studio/workflows/redis-memory/validate");
    const body = JSON.parse(String(request?.[1]?.body || "{}"));
    expect(body).toEqual({
      expected_resource_version: "rev-1",
      expected_semantic_hash: "semantic-1",
    });
    expect(body).not.toHaveProperty("graph");
  });

  it("keeps More limited to low-frequency non-lifecycle actions", async () => {
    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    const trigger = await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-action-menu-trigger"]');
    await act(async () => {
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
      await flush();
    });
    const menu = await waitForElement<HTMLElement>(document, '[data-testid="workflow-action-menu"]');
    const actionIds = [...menu.querySelectorAll<HTMLElement>("[data-action]")]
      .map((item) => item.dataset.action);

    expect(actionIds.length).toBeGreaterThan(0);
    expect(actionIds.every((id) => ["import", "export", "manual", "shortcuts", "duplicate", "delete"].includes(String(id)))).toBe(true);
    expect(actionIds).not.toEqual(expect.arrayContaining(["validate", "dry-run", "run", "variables", "publish"]));
    expect(menu.textContent).not.toContain("检查工作流");
    expect(menu.textContent).not.toContain("试运行");
    expect(menu.textContent).not.toContain("发布工作流");
    expect(menu.textContent).not.toContain("运行已发布版本");
  });

  it("runs only the exact immutable published target and never sends the current draft Graph", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflow-published-run") return jsonResponse({ run_id: "run-published-1", status: "running" }, 202);
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: publishedWorkspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    await act(async () => click(await waitForElement(container, '[data-testid="workflow-context-tab-run"]')));
    const publishedTarget = await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-run-published"]');
    await act(async () => {
      click(publishedTarget);
      await flush();
    });

    const request = vi.mocked(globalThis.fetch).mock.calls.find(([input]) =>
      String(input) === "/api/runner-studio/workflow-published-run");
    expect(request).toBeTruthy();
    expect(request?.[1]?.method).toBe("POST");
    const body = JSON.parse(String(request?.[1]?.body || "{}"));
    expect(body.target).toEqual({
      workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
      resourceVersion: PUBLISHED_RESOURCE_VERSION,
      semanticHash: PUBLISHED_SEMANTIC_HASH,
    });
    expect(body.target.resourceVersion).not.toBe(CURRENT_RESOURCE_VERSION);
    expect(body).not.toHaveProperty("graph");
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input) === "/api/runner-studio/workflow-dry-run")).toBe(false);
    expect(vi.mocked(globalThis.fetch).mock.calls.some(([input]) => String(input) === "/api/runner-studio/runs")).toBe(false);
  });

  it("retries a failed published run against the same immutable revision target", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    const runIds = ["run-published-failed", "run-published-retry"];
    const publishedRequests: Record<string, unknown>[] = [];
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflow-published-run") {
        publishedRequests.push(JSON.parse(String(init?.body || "{}")));
        return jsonResponse({ run_id: runIds[Math.min(publishedRequests.length - 1, 1)], status: "running" }, 202);
      }
      if (path === `/api/runner-studio/runs/${runIds[0]}/events/history`) {
        return jsonResponse([
          { type: "run_start", run_id: runIds[0], status: "running", timestamp: "2026-07-08T03:00:00Z" },
          { type: "run.failed", run_id: runIds[0], status: "failed", error: "remote unavailable", timestamp: "2026-07-08T03:00:01Z" },
        ]);
      }
      if (path === `/api/runner-studio/runs/${runIds[1]}/events/history`) {
        return jsonResponse([
          { type: "run_start", run_id: runIds[1], status: "running", timestamp: "2026-07-08T03:01:00Z" },
          { type: "run_finish", run_id: runIds[1], status: "success", timestamp: "2026-07-08T03:01:01Z" },
        ]);
      }
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: publishedWorkspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement(container, '[data-testid="workflow-workbench-trigger"]')));
    await act(async () => click(await waitForElement(container, '[data-testid="workflow-context-tab-run"]')));
    await act(async () => {
      click(await waitForElement(container, '[data-testid="workflow-run-published"]'));
      await new Promise((resolve) => setTimeout(resolve, 800));
    });
    const sessionInspector = await waitForElement<HTMLElement>(container, '[data-testid="workflow-run-inspector"]');
    await act(async () => click(await waitForElement(sessionInspector, "ol button")));
    await act(async () => {
      click(await waitForElement(sessionInspector, '[data-testid="workflow-run-retry"]'));
      await new Promise((resolve) => setTimeout(resolve, 800));
    });

    expect(publishedRequests).toHaveLength(2);
    expect(publishedRequests[1]?.target).toEqual(publishedRequests[0]?.target);
    expect(publishedRequests[1]?.target).toEqual({
      workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
      resourceVersion: PUBLISHED_RESOURCE_VERSION,
      semanticHash: PUBLISHED_SEMANTIC_HASH,
    });
    expect(publishedRequests[1]).not.toHaveProperty("graph");
    expect(publishedRequests[1]?.idempotency_key).not.toBe(publishedRequests[0]?.idempotency_key);
  });

  it("keeps the script tab active when the selected node is refreshed", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => doubleClick(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    await waitForElement(container, '[data-testid="runner-node-panel"]');
    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-node-panel-tab-script"]')));
    expect(await waitForElement(container, '[data-testid="runner-node-script-tab"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="runner-node-settings"]')).toBeNull();

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="canvas-node-check"]')));

    expect(await waitForElement(container, '[data-testid="runner-node-script-tab"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="runner-node-settings"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-node-panel-tab-script"]')?.className).toContain("active");
  });

  it("submits Inspector edits as typed commands and waits for authoritative reconciliation", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflow-commands") {
        return jsonResponse({
          receipt: {
            transactionId: "txn-inspector-1",
            changeSetId: "change-inspector-1",
            workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
            status: "verifying",
            revisionBefore: { resourceVersion: "rev-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
            revisionAfter: { resourceVersion: "rev-2", semanticHash: "semantic-2", layoutHash: "layout-1" },
            validation: {
              basis: {
                resourceVersion: "rev-2",
                semanticHash: "semantic-2",
                catalogVersion: "catalog-1",
                actionSchemaDigest: "actions-1",
                referenceSnapshotHash: "refs-1",
                environmentBindingVersion: "env-1",
              },
              valid: true,
              diagnostics: [],
            },
          },
        });
      }
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const inspector = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel"]');
    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-tab-script"]')));
    const editor = await waitForElement<HTMLTextAreaElement>(inspector, '[data-testid="runner-node-script-editor"]');
    await act(async () => changeTextarea(editor, "print('server authoritative')"));
    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-apply"]')));
    await flush();

    const commandCall = vi.mocked(globalThis.fetch).mock.calls.find(([input]) => String(input) === "/api/runner-studio/workflow-commands");
    expect(commandCall).toBeTruthy();
    const body = JSON.parse(String(commandCall?.[1]?.body || "{}"));
    expect(body).toMatchObject({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "redis-memory",
      expectedRevision: "rev-1",
      command: { kind: "update_node", nodeId: "check", nodePatch: { step: { args: { script: "print('server authoritative')" } } } },
    });
    expect(body).not.toHaveProperty("graph");
    expect((inspector.querySelector('[data-testid="runner-node-script-editor"]') as HTMLTextAreaElement).value).toBe("print('server authoritative')");
    expect((await waitForElement(container, '[data-testid="workflow-save-status"]')).textContent).toContain("正在应用");
    expect(container.querySelector('[data-testid="runner-workspace-command-state"]')).toBeNull();
  });

  it("keeps the Inspector draft and reports unsaved when the typed command fails", async () => {
    const fallbackFetch = vi.mocked(globalThis.fetch).getMockImplementation();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows/redis-memory/graph") return jsonResponse({ graph: workflowGraph });
      if (path === "/api/runner-studio/workflow-commands") {
        return jsonResponse({ errorCode: "STALE_REVISION", message: "workflow changed", retryHint: "reload" }, 409);
      }
      return fallbackFetch?.(input, init) || jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <DirectPageChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <Routes>
              <Route path="/runner/:workflowName" element={<WorkflowTransportBoundary workflowId="redis-memory"><RunnerStudioPage appSnapshot={{ config: { workflowWorkspace: workspaceProjection(), workflowCapabilities: workflowCapabilities() } }} /></WorkflowTransportBoundary>} />
            </Routes>
          </MemoryRouter>
        </DirectPageChromeProvider>,
      );
    });
    await act(async () => click(await waitForElement(container, '[data-testid="canvas-node-check"]')));
    const inspector = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel"]');
    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-tab-script"]')));
    const editor = await waitForElement<HTMLTextAreaElement>(inspector, '[data-testid="runner-node-script-editor"]');
    await act(async () => changeTextarea(editor, "print('keep my draft')"));
    await act(async () => click(inspector.querySelector('[data-testid="runner-node-panel-apply"]')));
    await flush();

    expect((inspector.querySelector('[data-testid="runner-node-script-editor"]') as HTMLTextAreaElement).value).toBe("print('keep my draft')");
    const toast = await waitForElement(container, '[data-testid="workflow-command-error-toast"]');
    expect(toast.textContent).toContain("修改未保存");
    expect(toast.textContent).toContain("workflow changed");
    expect(container.querySelector('[data-testid="runner-workspace-command-state"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-save-status"]')?.textContent).toContain("未保存");
    await act(async () => click(toast.querySelector('[aria-label="关闭错误提示"]')));
    expect(container.querySelector('[data-testid="workflow-command-error-toast"]')).toBeNull();
    expect(vi.mocked(globalThis.fetch).mock.calls.filter(([input, init]) =>
      String(input).endsWith("/graph") && String(input).includes("redis-memory") && ["PUT", "POST"].includes(String(init?.method || "GET")),
    )).toHaveLength(0);
  });
});

describe("WorkflowEditorShell authoritative manual commands", () => {
  it("translates one drag-stop graph update into one typed move command", () => {
    const next = {
      ...workflowGraph,
      nodes: workflowGraph.nodes.map((node) => node.id === "check"
        ? { ...node, position: { x: 440, y: 220 } }
        : node),
    };

    expect(workflowCommandForGraphChange(workflowGraph, next)).toEqual({
      kind: "move",
      layout: { nodes: [{ nodeId: "check", position: { x: 440, y: 220 } }] },
    });
  });

  it("translates edits and connections without leaking canvas UI/runtime fields", () => {
    const edited = {
      ...workflowGraph,
      nodes: workflowGraph.nodes.map((node) => node.id === "check"
        ? { ...node, label: "检查 Redis 内存", ui: { selected: true }, state: { status: "running" } }
        : node),
    };
    expect(workflowCommandForGraphChange(workflowGraph, edited)).toEqual({
      kind: "update_node",
      nodeId: "check",
      nodePatch: { label: "检查 Redis 内存" },
    });

    const connected = {
      ...workflowGraph,
      edges: [...workflowGraph.edges, {
        id: "start-end-success",
        source: "start",
        source_port: "success",
        target: "end",
        target_port: "in",
        kind: "success",
        state: { status: "selected" },
      }],
    };
    expect(workflowCommandForGraphChange(workflowGraph, connected)).toEqual({
      kind: "connect_edge",
      edge: {
        id: "start-end-success",
        source: "start",
        sourcePort: "success",
        target: "end",
        targetPort: "in",
        kind: "success",
      },
    });
  });

  it("maps add, cascade delete, and disconnect to their typed server commands", () => {
    const extraNode = { id: "notify", type: "action", label: "Notify", position: { x: 520, y: 320 }, step: { name: "notify", action: "script.shell", args: {} }, ui: { selected: true } };
    expect(workflowCommandForGraphChange(workflowGraph, { ...workflowGraph, nodes: [...workflowGraph.nodes, extraNode] })).toEqual({
      kind: "add_node",
      node: { id: "notify", type: "action", label: "Notify", position: { x: 520, y: 320 }, step: { name: "notify", action: "script.shell", args: {} } },
    });

    expect(workflowCommandForGraphChange(workflowGraph, {
      ...workflowGraph,
      nodes: workflowGraph.nodes.filter((node) => node.id !== "check"),
      edges: workflowGraph.edges.filter((edge) => edge.source !== "check" && edge.target !== "check"),
    })).toEqual({ kind: "delete_node", nodeId: "check", cascadeEdges: true });

    expect(workflowCommandForGraphChange(workflowGraph, { ...workflowGraph, edges: workflowGraph.edges.slice(1) }))
      .toEqual({ kind: "disconnect_edge", edgeId: "start-check" });
  });

  it("fails closed for compound graph replacement instead of inventing local success", () => {
    const compound = {
      ...workflowGraph,
      nodes: workflowGraph.nodes.filter((node) => node.id !== "check" && node.id !== "start"),
      edges: [],
    };

    expect(() => workflowCommandForGraphChange(workflowGraph, compound))
      .toThrow("single typed workflow command");
  });
});

describe("RunnerStudioPage Candidate canvas revisions", () => {
  it("maps a Candidate parameter edit to one canonical update_node operation", () => {
    const next = {
      ...workflowGraph,
      nodes: workflowGraph.nodes.map((node) => node.id === "check"
        ? { ...node, step: { ...node.step, args: { script: "df -h" } } }
        : node),
    };
    expect(workflowOperationForCandidateGraphChange(workflowGraph, next)).toEqual({
      op: "update_node",
      nodeId: "check",
      set: { step: { name: "检查内存", action: "script.python", args: { script: "df -h" } } },
    });
  });

  it("maps Candidate drag-stop to one canonical update_layout operation", () => {
    const next = {
      ...workflowGraph,
      nodes: workflowGraph.nodes.map((node) => node.id === "check"
        ? { ...node, position: { x: 480, y: 260 } }
        : node),
    };
    expect(workflowOperationForCandidateGraphChange(workflowGraph, next)).toEqual({
      op: "update_layout",
      layout: { nodes: [{ nodeId: "check", position: { x: 480, y: 260 } }] },
    });
  });

  it("maps Candidate connection and reconnection to canonical edge operations", () => {
    const connected = {
      ...workflowGraph,
      edges: [...workflowGraph.edges, {
        id: "start-end-success", source: "start", source_port: "success", target: "end", target_port: "in", kind: "success",
      }],
    };
    expect(workflowOperationForCandidateGraphChange(workflowGraph, connected)).toEqual({
      op: "add_edge",
      edge: { id: "start-end-success", source: "start", sourcePort: "success", target: "end", targetPort: "in", kind: "success" },
    });

    const reconnected = {
      ...workflowGraph,
      edges: workflowGraph.edges.map((edge) => edge.id === "check-end" ? { ...edge, target: "start" } : edge),
    };
    expect(workflowOperationForCandidateGraphChange(workflowGraph, reconnected)).toEqual({
      op: "update_edge",
      edgeId: "check-end",
      set: { target: "start" },
    });
  });
});

function workspaceProjection() {
  return {
    schemaVersion: "aiops.workflow.workspace.v1" as const,
    workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
    committed: {
      workflowKey: { tenantId: "tenant-a", workflowId: "redis-memory" },
      revision: { resourceVersion: "rev-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
      graph: workflowGraph,
      sequence: 1,
      loadedAt: "2026-07-21T00:00:00Z",
    },
    events: [],
    lastSequence: 1,
    runGateFacts: {
      currentBasis: workflowBasis(),
      environment: { name: "staging", bindingVersion: "env-1" },
    },
    validationArtifacts: [{
      artifactId: "validation-rev-1",
      basis: workflowBasis(),
      status: "valid" as const,
      diagnostics: [],
      issuedAt: "2026-07-21T00:00:01Z",
    }],
    durableHistory: [{
      source: "durable_transaction" as const,
      transactionId: "txn-rev-1",
      businessLabel: "更新工作流",
      status: "completed" as const,
      occurredAt: "2026-07-21T00:00:01Z",
      actorLabel: "用户",
      touchedObjectCount: 1,
      reversible: true,
    }],
  };
}

function publishedWorkspaceProjection() {
  const projection = workspaceProjection();
  const currentBasis = {
    ...workflowBasis(),
    resourceVersion: CURRENT_RESOURCE_VERSION,
    semanticHash: CURRENT_SEMANTIC_HASH,
  };
  const publishedBasis = {
    ...workflowBasis(),
    resourceVersion: PUBLISHED_RESOURCE_VERSION,
    semanticHash: PUBLISHED_SEMANTIC_HASH,
  };
  return {
    ...projection,
    committed: {
      ...projection.committed,
      revision: {
        resourceVersion: CURRENT_RESOURCE_VERSION,
        semanticHash: CURRENT_SEMANTIC_HASH,
        layoutHash: "layout-2",
      },
      sequence: 2,
    },
    lastSequence: 2,
    runGateFacts: {
      currentBasis,
      environment: { name: "staging", bindingVersion: "env-1" },
      postCheck: {
        basis: currentBasis,
        mode: "draft_non_production" as const,
        status: "dry_run_passed" as const,
        checkedAt: "2026-07-21T00:01:00Z",
      },
    },
    validationArtifacts: [{
      artifactId: "validation-rev-2",
      basis: currentBasis,
      status: "valid" as const,
      diagnostics: [],
      issuedAt: "2026-07-21T00:01:00Z",
    }],
    lifecycleArtifacts: [{
      artifactId: "published-rev-1",
      basis: publishedBasis,
      status: "published" as const,
      issuedAt: "2026-07-21T00:00:30Z",
    }],
  };
}

function workflowBasis() {
  return {
    resourceVersion: "rev-1",
    semanticHash: "semantic-1",
    catalogVersion: "catalog-1",
    actionSchemaDigest: "actions-1",
    referenceSnapshotHash: "refs-1",
    environmentBindingVersion: "env-1",
  };
}

function workflowCapabilities() {
  return {
    schemaVersion: "aiops.workflow.rollout.v1",
    stage: "internal_apply",
    saltVersion: "workflow-rollout-2026-07-v1",
    rolloutPercentage: 100,
    bucketBasisPoints: 1724,
    decision: "allow_list",
    capabilities: {
      workflow_agent_ui: true,
      workflow_mcp_read: true,
      workflow_mcp_preview: true,
      workflow_mcp_mutation: true,
      workflow_delete: false,
      workflow_publish: false,
      workflow_execute: false,
      workflow_domain_events: true,
      workflow_plan_ir: false,
      workflow_elk_layout: false,
      workflow_edit_grants: false,
      workflow_external_mcp: false,
      workflow_mcp_apps: false,
    },
  };
}
