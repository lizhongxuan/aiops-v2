import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider as BaseAppShellChromeProvider } from "@/app/AppShellChromeContext";
import { createAiopsQueryClient } from "@/app/queryClient";
import { AppRouter } from "@/router";

function AppShellChromeProvider({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={createAiopsQueryClient()}>
      <BaseAppShellChromeProvider>{children}</BaseAppShellChromeProvider>
    </QueryClientProvider>
  );
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

function keyDown(element: Element, key: string) {
  element.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
}

function canvasNodePosition(container: ParentNode, nodeId: string) {
  const node = container.querySelector(`[data-testid="rf__node-${nodeId}"]`) as HTMLElement | null;
  const transform = node?.style.transform || "";
  const match = transform.match(/translate\(([-\d.]+)px,\s*([-\d.]+)px\)/);
  return {
    x: match ? Number(match[1]) : 0,
    y: match ? Number(match[2]) : 0,
  };
}

describe("RunnerStudioPage Workflow AI", () => {
  let container: HTMLDivElement;
  let root: Root;
  let workflowAiChatClientTurnId = "";
  let workflowAiChatStateFactory: (() => unknown) | undefined;

  beforeEach(() => {
    workflowAiChatClientTurnId = "";
    workflowAiChatStateFactory = undefined;
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
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
      if (path === "/api/v1/chat/message") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : {};
        workflowAiChatClientTurnId = String(body.clientTurnId || "");
        return jsonResponse({
          accepted: true,
          sessionId: "drawer-redis-memory-chat-v2",
          turnId: "turn-chat",
          status: "accepted",
        });
      }
      if (path === "/api/v1/state") {
        if (workflowAiChatStateFactory) {
          return jsonResponse(workflowAiChatStateFactory());
        }
        return jsonResponse({
          schemaVersion: "aiops.transport.v2",
          sessionId: "drawer-redis-memory-chat-v2",
          threadId: "drawer-redis-memory-chat-v2",
          status: "idle",
          currentTurnId: "turn-chat",
          turns: {
            "turn-chat": {
              id: "turn-chat",
              user: { clientTurnId: workflowAiChatClientTurnId, text: "你好" },
              status: "completed",
              final: {
                id: "final-chat",
                text: "你好，我是 Workflow AI。你可以直接和我对话；只有你明确要求创建或修改工作流时，我才会先生成计划。",
                status: "completed",
              },
            },
          },
          turnOrder: ["turn-chat"],
          cards: [],
        });
      }
      if (path === "/api/runner-studio/workflow-ai/sessions") {
        return jsonResponse({ id: "drawer-redis-memory", workflowId: "redis-memory", baseRevision: "rev-1", activeRevision: "rev-1", status: "active" });
      }
      if (path === "/api/runner-studio/workflow-ai/plan") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : {};
        const message = String(body.message || "");
        if (message.includes("记录开始")) {
          return jsonResponse({
            id: "plan-log-node",
            workflowId: "redis-memory",
            message,
            items: [{
              id: "record-start",
              title: "添加“记录开始”日志节点并连接至 Start",
              description: "在 Start 后新增一个短日志节点，记录工作流开始事件并继续向下游传递上下文。",
              goal: "记录工作流开始事件。",
              environment: "读取 Start 输出和当前 Workflow 上下文。",
              nodeLabel: "记录开始",
              nodeType: "action",
              nodeAction: "script.python",
              scriptSummary: "写入开始事件，并输出 workflow_started 标记。",
              validationSummary: "确认输出 workflow_started=true。",
              inputVariables: [{ name: "workflow_context", type: "object" }],
              outputVariables: [{ name: "workflow_started", type: "object" }],
              script: "print({'workflow_started': True})",
              status: "pending",
            }],
          });
        }
        return jsonResponse({
          id: "plan-api",
          workflowId: "redis-memory",
          message: "添加验证步骤",
          items: [
            {
              id: "item-api",
              title: "添加验证步骤",
              description: "在收集节点之后加入验证节点，检查输出是否可用于后续判断。",
              goal: "LLM 目标：验证收集结果是否满足后续判断条件。",
              environment: "LLM 环境：读取 collect 输出和当前 Workflow 上下文。",
              nodeLabel: "验证输出",
              nodeType: "action",
              nodeAction: "script.python",
              scriptSummary: "LLM 脚本：校验 memory_usage 字段并输出 validation_result。",
              validationSummary: "LLM 校验：validation_result.ok 必须为 true。",
              inputVariables: [{ name: "memory_usage", type: "number", required: true }],
              outputVariables: [{ name: "validation_result", type: "object" }],
              script: "# LLM generated validation script\nprint({'validation_result': {'ok': True}})",
              status: "pending",
            },
            { id: "item-audit", title: "记录验证结果", description: "把验证结果写入运行记录，方便事件列表和复盘查看。", status: "pending" },
          ],
        });
      }
      if (path === "/api/runner-studio/workflow-ai/validate") {
        return jsonResponse({ valid: true, errors: [], warnings: [] });
      }
      if (path === "/api/runner-studio/workflow-ai/preview") {
        return jsonResponse({
          patchId: "patch-api",
          graph: {
            ...workflowGraph,
            nodes: workflowGraph.nodes.map((node) => node.id === "check" ? { ...node, ui: { ai_note: "添加验证步骤" } } : node),
          },
          effect: { status: "changed", affectedNodes: ["check"] },
        });
      }
      if (path === "/api/runner-studio/workflow-ai/effect") {
        return jsonResponse({ status: "changed", affectedNodes: ["check"] });
      }
      if (path === "/api/runner-studio/workflow-ai/apply") {
        return jsonResponse({
          patchId: "patch-api",
          workflowId: "redis-memory",
          revisionBefore: "rev-1",
          revisionAfter: "rev-2",
          effect: { status: "changed", affectedNodes: ["check"] },
          undoCheckpoint: { id: "undo-api", patchId: "patch-api", revisionBefore: "rev-1", revisionAfter: "rev-2" },
          describe: { summary: "redis-memory has 3 nodes and 2 edges", nodeCount: 3, edgeCount: 2 },
        });
      }
      if (path === "/api/runner-studio/workflow-ai/undo") {
        return jsonResponse({
          workflowId: "redis-memory",
          revisionBefore: "rev-2",
          revisionAfter: "rev-1",
          undoCheckpoint: { id: "undo-api", patchId: "patch-api" },
          describe: { summary: "redis-memory has 3 nodes and 2 edges", nodeCount: 3, edgeCount: 2 },
        });
      }
      if (path === "/api/runner-studio/workflow-ai/create-draft") {
        return jsonResponse({
          workflowId: "redis-memory-draft",
          revision: "rev-create",
          graph: {
            ...workflowGraph,
            workflow: { name: "redis-memory-draft", title: "每天巡检 Redis 内存" },
          },
          validation: { valid: true, errors: [], warnings: [] },
          describe: { summary: "redis-memory-draft has 3 nodes and 2 edges", nodeCount: 3, edgeCount: 2 },
          published: false,
          executed: false,
        });
      }
      if (path === "/api/runner-studio/workflows/redis-memory") {
        return jsonResponse({ name: "redis-memory" });
      }
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });
    localStorage.clear();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    vi.restoreAllMocks();
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
    const topbar = await waitForElement<HTMLElement>(container, '[data-testid="runner-studio-topbar"]');
    expect(topbar.textContent).toContain("新建工作流");
    expect(topbar.textContent).toContain("draft");
    expect(container.querySelector('[data-testid="runner-studio-canvas"]')).toBeTruthy();
  });

  it("lets users edit the workflow title from the top bar", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const titleButton = await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-workflow-title-display"]');
    expect(titleButton.textContent).toContain("Redis Memory");

    await act(async () => click(titleButton));
    const titleInput = await waitForElement<HTMLInputElement>(container, '[data-testid="runner-workflow-title-input"]');
    await act(async () => {
      changeInput(titleInput, "每天早上8点巡检Redis内存");
      keyDown(titleInput, "Enter");
    });

    expect((await waitForElement(container, '[data-testid="runner-workflow-title-display"]')).textContent).toContain("每天早上8点巡检Redis内存");
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).toContain("未保存");
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

  it("keeps run detail focused and makes verbose sections collapsible", async () => {
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

    await waitForElement(container, '[data-testid="canvas-node-check"]');
    await act(async () => {
      click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-run"]'));
      await new Promise((resolve) => setTimeout(resolve, 800));
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-run-details"]')));
    const historyRow = await waitForElement<HTMLButtonElement>(container, `[data-testid="runner-run-history-row-${runId}"]`);
    expect(historyRow.textContent).toContain("成功");
    expect(historyRow.textContent).not.toContain("success");
    expect(await waitForElement(container, `[data-testid="runner-run-record-status-${runId}"]`)).toBeTruthy();
    await act(async () => click(historyRow));

    const detail = await waitForElement<HTMLElement>(container, '[data-testid="runner-run-detail-panel"]');
    const activeStatus = await waitForElement(container, '[data-testid="runner-run-active-status"]');
    expect(activeStatus.textContent).toContain("成功");
    expect(activeStatus.textContent).not.toContain("success");
    expect(detail.textContent).not.toContain("运行追溯");
    expect(detail.textContent).not.toContain("运行概览");
    expect(container.querySelector('[data-testid="runner-run-traceability"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-run-overview"]')).toBeNull();
    expect(detail.textContent).toContain("节点");
    expect(detail.textContent).toContain("check");
    expect(detail.textContent).toContain("成功");
    const startRunNode = await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-run-node-start"]');
    expect(startRunNode.textContent).toContain("成功");
    expect(startRunNode.textContent).not.toContain("success");
    expect(await waitForElement(container, '[data-testid="runner-run-node-status-icon-start"]')).toBeTruthy();

    const variableInspector = await waitForElement<HTMLDetailsElement>(container, '[data-testid="runner-variable-inspector"]');
    expect(variableInspector.tagName).toBe("DETAILS");
    expect(variableInspector.open).toBe(false);
    expect(variableInspector.querySelector("summary")?.textContent).toContain("变量检查器");

    const runLogs = await waitForElement<HTMLDetailsElement>(container, '[data-testid="runner-run-logs"]');
    expect(runLogs.tagName).toBe("DETAILS");
    expect(runLogs.open).toBe(false);
    expect(runLogs.querySelector("summary")?.textContent).toContain("stdout / stderr / SSE");
    await act(async () => click(startRunNode));
    expect(runLogs.querySelector("summary")?.textContent).toContain("当前节点暂无日志");
    expect(runLogs.textContent).not.toContain("hello");

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-run-node-check"]')));
    expect(runLogs.querySelector("summary")?.textContent).toContain("1 条日志");
    expect(runLogs.textContent).toContain("hello");
  });

  it("opens Workflow AI as a direct chat drawer, confirms the whole plan once, and applies visible graph changes", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const topbar = await waitForElement<HTMLElement>(container, '[data-testid="runner-studio-topbar"]');
    const actionLabels = Array.from(topbar.querySelectorAll("button")).map((button) => button.textContent?.trim());
    expect(actionLabels).not.toContain("事件");
    expect(actionLabels.indexOf("运行详情")).toBeLessThan(actionLabels.indexOf("AI"));
    expect(actionLabels.indexOf("AI")).toBeLessThan(actionLabels.indexOf("更多"));
    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-more"]')));
    expect(container.querySelector('[data-testid="runner-toolbar-more-menu"]')?.textContent).not.toContain("AI");

    const aiButton = await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]');
    expect(aiButton.textContent).toBe("AI");

    await act(async () => click(aiButton));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    expect(drawer.querySelector('[data-testid="workflow-ai-context-card"]')).toBeNull();
    expect(drawer.querySelector('[data-testid="workflow-ai-updated-label"]')?.textContent).toContain("修改");
    expect(drawer.textContent).toContain("你可以直接问我");
    expect(drawer.textContent).not.toContain("涉及修改时我会先生成计划");

    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "添加验证步骤");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));
    expect(await waitForElement(container, '[data-testid="workflow-ai-plan-card"]')).toBeTruthy();
    expect(drawer.textContent).toContain("记录验证结果");
    expect(drawer.textContent).not.toContain("生成一个最小 Workflow patch");
    expect(drawer.textContent).toContain("回复「确认」开始");
    expect(drawer.textContent).not.toContain("确认计划并开始修改");
    expect(container.querySelector('[data-testid="workflow-ai-result-card"]')).toBeNull();
    expect(drawer.textContent).not.toContain("Start");
    expect(drawer.textContent).not.toContain("Apply");

    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "确认");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    expect(await waitForElement(container, '[data-testid="workflow-ai-result-card"]')).toBeTruthy();
    const stepHistory = container.querySelectorAll('[data-testid="workflow-ai-step-history-card"]');
    expect(stepHistory.length).toBeGreaterThanOrEqual(2);
    expect(stepHistory[0].textContent).toContain("完成步骤 1/");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("添加验证步骤");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("LLM 目标：验证收集结果");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("memory_usage:number 必填");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("目标");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("环境");
    expect(Array.from(stepHistory).map((item) => item.textContent).join("\n")).toContain("脚本");
    const generatedNode = await waitForElement(container, '[data-testid="canvas-node-ai-step-item-api"]');
    expect(generatedNode.textContent).toContain("验证输出");
    expect(generatedNode.textContent).not.toContain("添加验证步骤");
    expect(container.querySelector('[data-testid="canvas-node-ai-step-item-api"]')?.className).toContain("ai-highlighted");
    expect(await waitForElement(container, '[data-testid="canvas-node-ai-step-item-audit"]')).toBeTruthy();
    const firstStepPosition = canvasNodePosition(container, "ai-step-item-api");
    const secondStepPosition = canvasNodePosition(container, "ai-step-item-audit");
    expect(secondStepPosition.x).toBeGreaterThan(firstStepPosition.x);
    expect(secondStepPosition.y).toBeGreaterThanOrEqual(firstStepPosition.y);
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).toContain("未保存");
    expect(container.querySelector('[data-testid="workflow-ai-undo-toast"]')).toBeNull();
    const resultCard = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-result-card"]');
    expect(resultCard.textContent).toContain("已完成修改");
    expect(resultCard.textContent).toContain("撤销");

    await act(async () => doubleClick(container.querySelector('[data-testid="canvas-node-ai-step-item-api"]')));
    const nodeTabs = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel-tabs"]');
    expect(nodeTabs.textContent).toContain("脚本");
    expect(nodeTabs.textContent).toContain("输入输出");
    expect(nodeTabs.querySelector("button")?.textContent).toBe("脚本");
    const targetLabelsInput = await waitForElement<HTMLInputElement>(container, '[data-testid="runner-node-target-labels-input"]');
    expect(targetLabelsInput.value).toBe("local");
    await act(async () => click(container.querySelector('[data-testid="runner-node-panel-tab-io"]')));
    const ioTab = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-io-tab"]');
    expect(ioTab.textContent).toContain("输入变量");
    expect(ioTab.textContent).toContain("输出变量");
    expect((ioTab.querySelector('input[aria-label="输入变量名"]') as HTMLInputElement).value).toBe("memory_usage");
    expect((ioTab.querySelector('input[aria-label="输出变量名"]') as HTMLInputElement).value).toBe("validation_result");
    await act(async () => click(container.querySelector('[data-testid="runner-node-panel-tab-script"]')));
    expect(container.querySelector('[data-testid="runner-node-settings"]')).toBeNull();
    const scriptTab = await waitForElement<HTMLElement>(container, '[data-testid="runner-node-script-tab"]');
    expect(scriptTab.textContent).toContain("脚本上下文");
    expect(scriptTab.textContent).not.toContain("查看和编辑当前节点真正执行的脚本内容");
    expect(scriptTab.textContent).not.toContain("策略：");
    const scriptEditor = await waitForElement<HTMLTextAreaElement>(container, '[data-testid="runner-node-script-editor"]');
    expect(scriptEditor.value).toContain("# LLM generated validation script");
    expect(scriptEditor.value).toContain("validation_result");
    expect(scriptEditor.value).not.toContain("PGBACKREST_STANZA");
    expect(scriptEditor.rows).toBe(9);

    expect(container.querySelector('[data-testid="runner-toolbar-events"]')).toBeNull();
    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    await act(async () => click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("事件"))));
    const eventDrawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-event-drawer"]');
    expect(container.querySelector('[data-testid="workflow-ai-drawer"]')).toBeNull();
    expect(eventDrawer.textContent).toContain("workflow.ai.plan.confirmed");
    expect(eventDrawer.textContent).toContain("workflow.ai.step.generating");
    expect(eventDrawer.textContent).toContain("workflow.graph.node.added");
    expect(eventDrawer.textContent).toContain("workflow.graph.edge.added");
    expect(eventDrawer.textContent).toContain("workflow.node.script.generated");
    expect(eventDrawer.textContent).toContain("workflow.ai.step.completed");
    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="workflow-event-filter-ai"]')));
    expect(eventDrawer.textContent).toContain("workflow.graph.node.added");
    expect(eventDrawer.textContent).toContain("workflow.graph.edge.added");
    expect(eventDrawer.textContent).toContain("workflow.node.script.generated");
    const nodeEventRow = Array.from(eventDrawer.querySelectorAll('[data-testid="workflow-event-row"]')).find((row) => row.textContent?.includes("workflow.graph.node.added") && row.textContent?.includes("验证输出"));
    await act(async () => click(nodeEventRow));
    expect(container.querySelector('[data-testid="workflow-event-drawer"]')).toBeNull();
    expect((await waitForElement<HTMLElement>(container, '[data-testid="runner-node-panel-title"]')).textContent).toContain("验证输出");

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    await act(async () => click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("事件"))));
    expect(await waitForElement<HTMLElement>(container, '[data-testid="workflow-event-drawer"]')).toBeTruthy();
    await act(async () => click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("返回 AI"))));
    expect(container.querySelector('[data-testid="workflow-event-drawer"]')).toBeNull();
    expect(await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]')).toBeTruthy();

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    await act(async () => click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "撤销")));
    expect(container.querySelector('[data-testid="workflow-ai-result-card"]')).toBeNull();
    expect(globalThis.fetch).not.toHaveBeenCalledWith(expect.stringContaining("/api/runner-studio/ai/draft"), expect.anything());
  });

  it("cancels a pending workflow ai plan from the composer without changing the graph", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await waitForElement(container, '[data-testid="canvas-node-start"]');
    const initialNodeCount = container.querySelectorAll('[data-testid^="canvas-node-"]').length;
    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "添加验证步骤");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));
    expect(await waitForElement(container, '[data-testid="workflow-ai-plan-card"]')).toBeTruthy();
    expect(container.querySelectorAll('[data-testid^="canvas-node-"]').length).toBe(initialNodeCount);

    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "取消");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("已取消本次计划");
    expect(container.querySelectorAll('[data-testid^="canvas-node-"]').length).toBe(initialNodeCount);
    expect(container.querySelector('[data-testid="canvas-node-ai-step-item-api"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).not.toContain("未保存");
    await act(async () => click(Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("事件"))));
    const eventDrawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-event-drawer"]');
    expect(eventDrawer.textContent).toContain("workflow.ai.plan.cancelled");
    expect(eventDrawer.textContent).not.toContain("workflow.graph.node.added");
  });

  it("answers read-only workflow questions through Workflow AI chat without generating an edit plan", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "解释一下，当前工作流做了什么？");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("你好，我是 Workflow AI");
    expect(answer.textContent).not.toContain("Workflow AI 回复");
    expect(answer.textContent).not.toContain("只读说明");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(drawer.textContent).not.toContain("正在生成修改计划");
    expect(drawer.textContent).not.toContain("确认计划并开始修改");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/chat/message",
      expect.objectContaining({
        body: expect.stringContaining('"source":"workflow_ai_chat"'),
      }),
    );
    const chatRequest = vi.mocked(globalThis.fetch).mock.calls.find(([input]) => String(input) === "/api/v1/chat/message");
    const chatBody = JSON.parse(String(chatRequest?.[1]?.body || "{}"));
    expect(chatBody.content).toContain("当前可见节点：Start、检查内存。");
    expect(chatBody.content).toContain("当前可见连线：Start -> 检查内存。");
    expect(chatBody.content).not.toContain("检查内存 -> End");
    expect(chatBody.content).not.toContain("不要使用 emoji");
    expect(chatBody.content).not.toContain("禁止说");
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.anything(),
    );
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).not.toContain("未保存");
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

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-workflow-title-display"]')));

    expect(await waitForElement(container, '[data-testid="runner-node-script-tab"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="runner-node-settings"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-node-panel-tab-script"]')?.className).toContain("active");
  });

  it("passes exact visible graph context to chat when the workflow has no End node", async () => {
    const startOnlyGraph = {
      ...workflowGraph,
      workflow: { name: "start-only", title: "Start Only" },
      nodes: [workflowGraph.nodes[0]],
      edges: [],
    };
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((input, init) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({
          workflows: [{
            name: "start-only",
            title: "Start Only",
            status: "draft",
            version: "rev-start-only",
            updated_at: "2026-07-06T04:20:00Z",
            graph: startOnlyGraph,
            validation_result: { valid: true, errors: [], warnings: [] },
          }],
        });
      }
      if (path === "/api/runner-studio/actions") return jsonResponse({ items: [] });
      if (path === "/api/v1/chat/message") {
        const body = typeof init?.body === "string" ? JSON.parse(init.body) : {};
        workflowAiChatClientTurnId = String(body.clientTurnId || "");
        return jsonResponse({ accepted: true, sessionId: "drawer-start-only-0-chat-v2", turnId: "turn-chat", status: "accepted" });
      }
      if (path === "/api/v1/state") {
        return jsonResponse({
          schemaVersion: "aiops.transport.v2",
          sessionId: "drawer-start-only-0-chat-v2",
          threadId: "drawer-start-only-0-chat-v2",
          status: "idle",
          currentTurnId: "turn-chat",
          turns: {
            "turn-chat": {
              id: "turn-chat",
              user: { clientTurnId: workflowAiChatClientTurnId, text: "解释一下当前工作流" },
              status: "completed",
              final: { id: "final-chat", text: "当前只有 Start 一个节点，还没有完整编排。", status: "completed" },
            },
          },
          turnOrder: ["turn-chat"],
          cards: [],
        });
      }
      if (path === "/api/runner-studio/workflows/start-only") return jsonResponse({ name: "start-only" });
      return jsonResponse({ error: `unexpected ${path}` }, 404);
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/start-only"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "解释一下当前工作流");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("当前只有 Start 一个节点");
    const chatRequest = vi.mocked(globalThis.fetch).mock.calls.find(([input]) => String(input) === "/api/v1/chat/message");
    const chatBody = JSON.parse(String(chatRequest?.[1]?.body || "{}"));
    expect(chatBody.content).toContain("当前对象：Start Only（draft），1 个节点、0 条连线。");
    expect(chatBody.content).toContain("当前可见节点：Start。");
    expect(chatBody.content).toContain("当前可见连线：无。");
    expect(chatBody.content).toContain("没有列出的节点不要假设存在");
    expect(chatBody.content).not.toContain("End");
    expect(chatBody.content).not.toContain("Start -> End");
  });

  it("handles greetings as normal Workflow AI chat without generating a plan", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "你好");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("你好，我是 Workflow AI");
    expect(answer.textContent).not.toContain("Workflow AI 回复");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(container.textContent).not.toContain("正在生成修改计划");
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).not.toContain("未保存");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/chat/message",
      expect.objectContaining({
        body: expect.stringContaining('"source":"workflow_ai_chat"'),
      }),
    );
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.anything(),
    );
  });

  it("treats negative edit wording as normal chat instead of generating a plan", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "你好，先别修改工作流。");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).not.toContain("Workflow AI 回复");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(container.textContent).not.toContain("正在生成修改计划");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/chat/message",
      expect.objectContaining({
        body: expect.stringContaining('"source":"workflow_ai_chat"'),
      }),
    );
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.anything(),
    );
  });

  it("treats optimization advice questions as chat instead of keyword-forced plan mode", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "你帮我看看这个工作流还有哪些地方可以优化？先给建议。");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).not.toContain("Workflow AI 回复");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/chat/message",
      expect.objectContaining({
        body: expect.stringContaining('"source":"workflow_ai_chat"'),
      }),
    );
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.anything(),
    );
  });

  it("routes simple one-node edits through plan review instead of local direct edits", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "把 Start 后面添加一个日志节点，节点名称叫「记录开始」，不要生成完整运维流程。");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    expect(await waitForElement(container, '[data-testid="workflow-ai-plan-card"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="workflow-ai-result-card"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-ai-step-history-card"]')).toBeNull();
    const generatedNode = Array.from(container.querySelectorAll('[data-testid^="canvas-node-ai-step-"]'))
      .find((node) => node.textContent?.includes("记录开始"));
    expect(generatedNode).toBeFalsy();
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).not.toContain("未保存");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("still generates a plan when an explicit create request says not to edit the canvas directly", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "生成一个 PostgreSQL 备份工作流，先生成计划，不要直接改画布。");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    expect(await waitForElement(container, '[data-testid="workflow-ai-plan-card"]')).toBeTruthy();
    expect(drawer.textContent).toContain("修改计划");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.objectContaining({ method: "POST" }),
    );
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/v1/chat/message",
      expect.anything(),
    );
  });

  it("waits for the matching chat turn instead of using stale assistant cards", async () => {
    let stateCalls = 0;
    workflowAiChatStateFactory = () => {
      stateCalls += 1;
      const base = {
        schemaVersion: "aiops.transport.v2",
        sessionId: "drawer-redis-memory-chat-v2",
        threadId: "drawer-redis-memory-chat-v2",
        status: "idle",
        currentTurnId: "turn-chat",
        turns: {},
        turnOrder: [],
        cards: [
          { id: "old-assistant", type: "AssistantMessageCard", role: "assistant", text: "旧的上一轮回复，不应该显示", message: "旧的上一轮回复，不应该显示" },
        ],
      };
      if (stateCalls < 2) return base;
      return {
        ...base,
        turns: {
          "turn-chat": {
            id: "turn-chat",
            user: { clientTurnId: workflowAiChatClientTurnId, text: "你可以做什么?" },
            status: "completed",
            final: {
              id: "final-chat",
              text: "当前问题的新回复：我可以解释、检查、创建或修改当前 Workflow。",
              status: "completed",
            },
          },
        },
        turnOrder: ["turn-chat"],
      };
    };

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner/redis-memory"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "你可以做什么?");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
    });

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("当前问题的新回复");
    expect(answer.textContent).not.toContain("旧的上一轮回复");
  });

  it("uses the assistant card following the matching Workflow AI user card when turns are absent", async () => {
    workflowAiChatStateFactory = () => ({
      schemaVersion: "aiops.transport.v2",
      sessionId: "drawer-redis-memory-chat-v2",
      threadId: "drawer-redis-memory-chat-v2",
      status: "idle",
      turns: {},
      turnOrder: [],
      cards: [
        { id: "old-user", type: "UserMessageCard", role: "user", clientTurnId: "old-turn", text: "旧问题", message: "旧问题" },
        { id: "old-assistant", type: "AssistantMessageCard", role: "assistant", text: "旧回答", message: "旧回答" },
        { id: `${workflowAiChatClientTurnId}-user`, type: "UserMessageCard", role: "user", clientTurnId: workflowAiChatClientTurnId, text: "你可以做什么?", message: "你可以做什么?" },
        { id: "current-assistant", type: "AssistantMessageCard", role: "assistant", text: "当前 card 回复：我可以解释、检查、创建或修改当前 Workflow。", message: "当前 card 回复：我可以解释、检查、创建或修改当前 Workflow。" },
      ],
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

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "你可以做什么?");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    const answer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-readonly-answer"]');
    expect(answer.textContent).toContain("当前 card 回复");
    expect(answer.textContent).not.toContain("旧回答");
  });

  it("opens create-mode drawer from workflow_ai query and preloads the prompt without auto-submit", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/runner?workflow_ai=create&prompt=%E6%AF%8F%E5%A4%A9%E5%B7%A1%E6%A3%80%20Redis%20%E5%86%85%E5%AD%98"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });

    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    expect(drawer.textContent).toContain("新建 Workflow");
    expect((drawer.querySelector("textarea") as HTMLTextAreaElement).value).toBe("每天巡检 Redis 内存");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/plan",
      expect.anything(),
    );

    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));
    expect(await waitForElement(container, '[data-testid="workflow-ai-plan-card"]')).toBeTruthy();
    expect(drawer.textContent).not.toContain("确认计划并开始修改");
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "确认");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/runner-studio/workflow-ai/create-draft",
      expect.objectContaining({ method: "POST" }),
    );
    expect((await waitForElement(container, '[data-testid="runner-studio-topbar"]')).textContent).toContain("每天巡检 Redis 内存");
  });

  it("does not fabricate a local plan or edit the graph when plan generation fails", async () => {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((input) => {
      const path = String(input);
      if (path === "/api/runner-studio/workflows") {
        return jsonResponse({
          workflows: [{
            name: "redis-memory",
            title: "Redis Memory",
            status: "draft",
            version: "rev-1",
            graph: workflowGraph,
            validation_result: { valid: true, errors: [], warnings: [] },
          }],
        });
      }
      if (path === "/api/runner-studio/actions") {
        return jsonResponse({ items: [] });
      }
      if (path.startsWith("/api/runner-studio/workflow-ai/")) {
        return jsonResponse({ error: "workfloweditor store unavailable" }, 404);
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

    await act(async () => click(await waitForElement<HTMLButtonElement>(container, '[data-testid="runner-toolbar-ai-generate"]')));
    const drawer = await waitForElement<HTMLElement>(container, '[data-testid="workflow-ai-drawer"]');
    await act(async () => {
      changeTextarea(drawer.querySelector("textarea") as HTMLTextAreaElement, "添加验证步骤");
    });
    await act(async () => click(Array.from(drawer.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))));

    expect(await waitForElement(container, '[data-testid="workflow-ai-conflict-card"]')).toBeTruthy();
    expect(drawer.textContent).toContain("计划生成失败");
    expect(drawer.textContent).not.toContain("生成一个最小 Workflow patch");
    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')).toBeNull();
    expect(container.querySelector('[data-testid="canvas-node-ai-step-item-1"]')).toBeNull();
    expect(container.querySelector('[data-testid="runner-save-state"]')?.textContent).not.toContain("未保存");
    expect(drawer.textContent).not.toContain("Start");
    expect(drawer.textContent).not.toContain("Apply");
  });
});
