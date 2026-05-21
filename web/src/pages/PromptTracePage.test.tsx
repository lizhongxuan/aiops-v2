import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PromptTracePage } from "./PromptTracePage";

const traceJson = {
  schemaVersion: 1,
  kind: "runtime_model_input",
  sessionId: "sess-1",
  turnId: "turn-1",
  caseId: "case-checkout-1",
  iteration: 1,
  createdAt: "2026-05-12T09:12:00+08:00",
  visibleTools: ["coroot.query_latency"],
  promptFingerprint: {
    stableHash: "stable-hash",
    developerHash: "developer-hash",
    toolRegistryHash: "tools-hash",
  },
  modelInput: [
    {
      index: 0,
      providerRole: "system",
      semanticRole: "system",
      promptLayer: "system",
      content: "System guard token=sk-page-system",
    },
    {
      index: 1,
      providerRole: "system",
      semanticRole: "developer",
      promptLayer: "developer",
      content: "Developer guard password=page-dev-pass",
    },
    {
      index: 2,
      providerRole: "user",
      semanticRole: "user",
      promptLayer: "conversation",
      content: "检查 checkout p95 延迟并生成图表 api_key=page-user-key",
    },
    {
      index: 3,
      providerRole: "assistant",
      semanticRole: "assistant",
      promptLayer: "conversation",
      content: "我会查询 Coroot 指标。secret=page-output-secret",
      toolCalls: [
        {
          id: "tool-call-coroot",
          type: "function",
          function: { name: "coroot.query_latency" },
          llmRequestId: "llm-request-1",
        },
      ],
    },
    {
      index: 4,
      providerRole: "tool",
      semanticRole: "tool_result",
      promptLayer: "conversation",
      toolCallId: "tool-call-coroot",
      content: "checkout p95=2800ms cookie=page-cookie",
    },
  ],
  llmRequests: [
    {
      id: "llm-request-1",
      request_body: {
        messages: [
          { role: "system", content: "System guard token=sk-page-request" },
          { role: "developer", content: "Developer guard secret=page-request-secret" },
          { role: "user", content: "用户请求 password=page-user-pass" },
        ],
      },
      retrieval_context: "Coroot context cookie=page-context-cookie",
      output: "图表已生成 api key=page-output-key",
      error: "",
      usage: { prompt_tokens: 21, completion_tokens: 8, total_tokens: 29 },
      duration_ms: 456,
      tool_messages: [{ content: "tool response request body={\"token\":\"page-body-token\"}" }],
    },
  ],
  artifacts: {
    "coroot-checkout-latency-chart": {
      artifact_id: "coroot-checkout-latency-chart",
      type: "coroot_chart",
      title: "Checkout p95 延迟图",
    },
  },
  agentUiArtifacts: [
    {
      artifact_id: "coroot-checkout-latency-chart",
      metadata: {
        llmRequestId: "llm-request-1",
        toolCallId: "tool-call-coroot",
        evidence_ref: "ev-coroot-latency",
        case_id: "case-checkout-1",
        redactionStatus: "redacted",
      },
    },
  ],
};

const traceTwoJson = {
  ...traceJson,
  turnId: "turn-2",
  createdAt: "2026-05-12T09:11:00+08:00",
  modelInput: traceJson.modelInput.map((message) => (
    message.providerRole === "user"
      ? { ...message, content: "修复 PG 集群主从复制延迟" }
      : message
  )),
};

let activeTraceList: unknown[];
let activeFiles: Record<string, unknown>;

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL) {
  const url = String(input);
  if (url.includes("/api/v1/debug/model-input-traces/file")) {
    const path = new URL(url, "http://localhost").searchParams.get("path") || "";
    const content = activeFiles[path] || traceJson;
    return jsonResponse({ content: JSON.stringify(content) });
  }
  if (url.includes("/api/v1/debug/model-input-traces")) {
    return jsonResponse({
      rootDir: ".data/model-input-traces",
      selectedId: "trace-1",
      traces: activeTraceList,
    });
  }
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 8; index += 1) {
      await Promise.resolve();
    }
  });
}

describe("PromptTracePage", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    activeTraceList = [
      {
        id: "trace-1",
        sessionId: "sess-1",
        turnId: "turn-1",
        caseId: "case-checkout-1",
        iteration: 1,
        jsonPath: ".data/model-input-traces/sess-1/turn-1/iteration-001.json",
        markdownPath: ".data/model-input-traces/sess-1/turn-1/iteration-001.md",
        relativePath: "sess-1/turn-1/iteration-001.json",
        createdAt: "2026-05-12T09:12:00+08:00",
        userPromptPreview: "检查 checkout p95 延迟",
        llmRequestCount: 1,
        usage: { promptTokens: 21, completionTokens: 8, totalTokens: 29 },
        averageDurationMs: 456,
        promptFingerprint: traceJson.promptFingerprint,
      },
    ];
    activeFiles = {
      ".data/model-input-traces/sess-1/turn-1/iteration-001.json": traceJson,
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(mockFetch as typeof fetch);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("keeps Prompt Trace lists compact and opens LLM details in a dialog", async () => {
    await act(async () => {
      root.render(<PromptTracePage />);
    });
    await flush();

    expect(container.textContent).not.toContain("模型请求列表");
    expect(container.textContent).not.toContain("选中请求详情");
    expect(container.textContent).not.toContain("先选择会话，再选择用户请求，最后查看该请求触发的所有 LLM Prompt、工具和 Agent-to-UI 来源。");
    expect(container.textContent).toContain("会话");
    expect(container.textContent).toContain("用户请求");
    expect(container.textContent).toContain("LLM 请求");
    expect(container.textContent).toContain("历史会话");
    expect(container.textContent).not.toContain("会话列表");
    expect(container.textContent).not.toContain("每个会话可包含多次用户请求。");
    expect(container.textContent).toContain("用户请求列表");
    expect(container.textContent).toContain("选择某次用户发出的对话请求。");
    expect(container.textContent).toContain("LLM 请求列表");
    expect(container.textContent).not.toContain("LLM 请求详情");
    expect(container.textContent).not.toContain("当前 LLM 请求");
    expect(container.textContent).not.toContain("Agent-to-UI 来源");
    expect(container.textContent).not.toContain("System Prompt");
    expect(container.querySelector('[data-testid="prompt-trace-scroll"]')?.className).toContain("overflow-x-auto");
    expect(container.querySelector('[data-testid="prompt-trace-scroll"]')?.className).toContain("overflow-y-hidden");
    expect(container.querySelector('[data-testid="prompt-trace-layout"]')?.className).toContain("grid-cols-[minmax(180px,240px)_minmax(220px,300px)_minmax(260px,1fr)]");
    expect(container.querySelector('[data-testid="prompt-trace-layout"]')?.className).not.toContain("xl:grid-cols");
    expect(container.querySelector('[data-testid="prompt-trace-layout"]')?.className).toContain("min-w-[720px]");
    expect(container.querySelector('[data-testid="prompt-trace-layout"]')?.className).toContain("overflow-hidden");
    expect(container.querySelector('[data-testid="prompt-trace-llm-list"]')?.className).toContain("min-w-0");
    expect(container.querySelector('[data-testid="prompt-trace-llm-list"]')?.className).toContain("overflow-auto");
    expect(container.querySelector('[data-testid="prompt-trace-llm-list"]')?.className).toContain("flex");
    expect(container.querySelector('[data-testid="prompt-trace-llm-list"]')?.className).not.toContain("grid");

    const sessionButton = container.querySelector('[data-testid="prompt-trace-session-card"]') as HTMLButtonElement | null;
    const userRequestButton = container.querySelector('[data-testid="prompt-trace-turn-card"]') as HTMLButtonElement | null;
    const llmRequestButton = container.querySelector('[data-testid="prompt-trace-llm-card"]') as HTMLButtonElement | null;
    expect(sessionButton?.className).toContain("h-28");
    expect(userRequestButton?.className).toContain("h-28");
    expect(llmRequestButton?.className).toContain("h-20");
    expect(llmRequestButton?.className).not.toContain("h-28");
    expect(sessionButton?.className).toContain("overflow-hidden");
    expect(userRequestButton?.className).toContain("overflow-hidden");
    expect(llmRequestButton?.className).toContain("overflow-hidden");
    expect(sessionButton?.getAttribute("title")).toContain("sess-1");
    expect(sessionButton?.getAttribute("title")).toContain("Case case-checkout-1");
    expect(sessionButton?.textContent).not.toContain("Case case-checkout-1");
    expect(sessionButton?.textContent).toContain("检查 checkout p95 延迟");
    expect(container.querySelector('[data-testid="prompt-trace-session-title"]')?.textContent).not.toContain("sess-1");
    expect(userRequestButton?.getAttribute("title")).toContain("检查 checkout p95 延迟");
    expect(userRequestButton?.getAttribute("title")).not.toContain("LLM 请求");
    expect(userRequestButton?.getAttribute("title")).not.toContain("Turn turn-1");
    expect(userRequestButton?.textContent).not.toContain("LLM 请求");
    expect(userRequestButton?.textContent).not.toContain("Turn turn-1");
    expect(userRequestButton?.textContent).toContain("turn-1");
    expect(userRequestButton?.textContent).toContain("Token 29");
    expect(userRequestButton?.textContent).toContain("平均 456ms");
    expect(llmRequestButton?.getAttribute("title")).toContain("sess-1/turn-1/iteration-001.json");
    expect(llmRequestButton?.getAttribute("title")).toContain("Token 29");
    expect(llmRequestButton?.getAttribute("title")).toContain("平均响应 456ms");
    expect(llmRequestButton?.getAttribute("title")).not.toContain("LLM 请求 1");
    expect(llmRequestButton?.getAttribute("title")).not.toContain("iteration 1");
    expect(llmRequestButton?.textContent).not.toContain("LLM 请求");
    expect(llmRequestButton?.textContent).not.toContain("iteration 1");
    expect(llmRequestButton?.textContent).toContain("Token 29");
    expect(llmRequestButton?.textContent).toContain("456ms");
    expect(llmRequestButton?.textContent).not.toContain("查看详情");
    expect(container.querySelector('[data-testid="prompt-trace-session-title"]')?.className).toContain("line-clamp-2");
    expect(container.querySelector('[data-testid="prompt-trace-turn-preview"]')?.className).toContain("line-clamp-2");
    expect(container.querySelector('[data-testid="prompt-trace-llm-path"]')?.className).toContain("truncate");
    expect(container.querySelector('[data-testid="prompt-trace-session-title"]')?.getAttribute("style") || "").toContain("-webkit-line-clamp: 2");
    expect(container.querySelector('[data-testid="prompt-trace-turn-preview"]')?.getAttribute("style") || "").toContain("-webkit-line-clamp: 2");

    const llmButton = container.querySelector('[data-testid="prompt-trace-llm-card"]') as HTMLButtonElement | null;
    expect(llmButton).toBeTruthy();
    await act(async () => {
      llmButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain("LLM 请求详情");
    expect(document.body.querySelector('[role="dialog"]')?.textContent).not.toContain("Agent-to-UI 来源");
    expect(document.body.querySelector('[role="dialog"]')?.textContent).not.toContain("当前链路");
    expect(Array.from(document.body.querySelectorAll('[role="dialog"] button')).map((button) => button.textContent)).not.toContain("来源");
    expect(document.body.textContent).toContain("Messages");
    expect(document.body.textContent).toContain("Tools");
    expect(document.body.textContent).toContain("Prompt chars");
    expect(document.body.textContent).toContain("Total tokens");
    expect(document.body.textContent).toContain("Avg response");
    expect(document.body.textContent).toContain("LLM 返回内容");
    expect(document.body.textContent).toContain("图表已生成");
    expect(document.body.textContent).toContain("prompt 21 / completion 8 / total 29");
    expect(document.body.textContent).toContain("456 ms");
    expect(document.body.textContent).not.toContain("coroot-checkout-latency-chart");
    expect(document.body.textContent).not.toContain("工具调用 coroot.query_latency");
    expect(document.body.textContent).not.toContain("EvidenceRef ev-coroot-latency");
    expect(document.body.textContent).toContain("已脱敏");
    expect(Array.from(document.body.querySelectorAll("button")).some((button) => button.textContent === "Raw")).toBe(true);
    expect(Array.from(container.querySelectorAll("textarea,input,[contenteditable='true']"))).toHaveLength(1);
    expect(container.innerHTML).not.toContain("sk-page-request");
    expect(container.innerHTML).not.toContain("page-request-secret");
    expect(container.innerHTML).not.toContain("page-user-pass");
    expect(container.innerHTML).not.toContain("page-body-token");
    expect(container.innerHTML).not.toContain("page-context-cookie");
    expect(container.innerHTML).not.toContain("page-output-key");
    expect(container.innerHTML).not.toContain("page-cookie");
  });

  it("keeps each user request preview bound to its own turn", async () => {
    activeTraceList = [
      {
        id: "trace-1",
        sessionId: "sess-1",
        turnId: "turn-1",
        caseId: "case-checkout-1",
        iteration: 1,
        createdAt: "2026-05-12T09:12:00+08:00",
        jsonPath: ".data/model-input-traces/sess-1/turn-1/iteration-001.json",
        markdownPath: ".data/model-input-traces/sess-1/turn-1/iteration-001.md",
        relativePath: "sess-1/turn-1/iteration-001.json",
        userPromptPreview: "检查 checkout p95 延迟",
        promptFingerprint: traceJson.promptFingerprint,
      },
      {
        id: "trace-2",
        sessionId: "sess-1",
        turnId: "turn-2",
        caseId: "case-checkout-1",
        iteration: 1,
        createdAt: "2026-05-12T09:11:00+08:00",
        jsonPath: ".data/model-input-traces/sess-1/turn-2/iteration-001.json",
        markdownPath: ".data/model-input-traces/sess-1/turn-2/iteration-001.md",
        relativePath: "sess-1/turn-2/iteration-001.json",
        userPromptPreview: "修复 PG 集群主从复制延迟",
        promptFingerprint: traceJson.promptFingerprint,
      },
    ];
    activeFiles = {
      ".data/model-input-traces/sess-1/turn-1/iteration-001.json": traceJson,
      ".data/model-input-traces/sess-1/turn-2/iteration-001.json": traceTwoJson,
    };

    await act(async () => {
      root.render(<PromptTracePage />);
    });
    await flush();

    const previewsBefore = Array.from(container.querySelectorAll('[data-testid="prompt-trace-turn-preview"]')).map((node) => node.textContent || "");
    expect(previewsBefore).toEqual(["检查 checkout p95 延迟", "修复 PG 集群主从复制延迟"]);

    const secondTurn = Array.from(container.querySelectorAll('[data-testid="prompt-trace-turn-card"]'))[1] as HTMLButtonElement;
    await act(async () => {
      secondTurn.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    const previewsAfter = Array.from(container.querySelectorAll('[data-testid="prompt-trace-turn-preview"]')).map((node) => node.textContent || "");
    expect(previewsAfter).toEqual(["检查 checkout p95 延迟", "修复 PG 集群主从复制延迟"]);
  });

  it("sorts LLM requests by request order inside the selected turn", async () => {
    activeTraceList = [
      {
        id: "trace-iter-2",
        sessionId: "sess-1",
        turnId: "turn-1",
        iteration: 2,
        createdAt: "2026-05-12T09:13:00+08:00",
        jsonPath: ".data/model-input-traces/sess-1/turn-1/iteration-002.json",
        relativePath: "sess-1/turn-1/iteration-002.json",
        userPromptPreview: "检查 checkout p95 延迟",
        promptFingerprint: traceJson.promptFingerprint,
      },
      {
        id: "trace-iter-1",
        sessionId: "sess-1",
        turnId: "turn-1",
        iteration: 1,
        createdAt: "2026-05-12T09:12:00+08:00",
        jsonPath: ".data/model-input-traces/sess-1/turn-1/iteration-001.json",
        relativePath: "sess-1/turn-1/iteration-001.json",
        userPromptPreview: "检查 checkout p95 延迟",
        promptFingerprint: traceJson.promptFingerprint,
      },
    ];
    activeFiles = {
      ".data/model-input-traces/sess-1/turn-1/iteration-001.json": traceJson,
      ".data/model-input-traces/sess-1/turn-1/iteration-002.json": { ...traceJson, iteration: 2 },
    };

    await act(async () => {
      root.render(<PromptTracePage />);
    });
    await flush();

    const llmPaths = Array.from(container.querySelectorAll('[data-testid="prompt-trace-llm-path"]')).map((node) => node.textContent || "");
    expect(llmPaths).toEqual([
      "sess-1/turn-1/iteration-001.json",
      "sess-1/turn-1/iteration-002.json",
    ]);
  });
});
