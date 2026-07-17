import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ProcessTranscript, groupConsecutiveBlocks, getMergedSummaryText, stripHtml } from "./ProcessTranscript";
import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

function makeBlock(overrides: Partial<AiopsProcessBlock> & { id: string; kind: AiopsProcessBlock["kind"] }): AiopsProcessBlock {
  return {
    status: "completed",
    text: overrides.text || `block-${overrides.id}`,
    ...overrides,
  };
}

describe("ProcessTranscript", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  async function expandProcessTranscript() {
    const header = container.querySelector('[data-testid="aiops-process-header"]');
    expect(header).toBeTruthy();
    await act(async () => {
      header?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
  }

  it("does not render final answer markdown before terminal completion", async () => {
    const markdown = "检查结果如下：\n\n1. **Nginx 正常**\n2. **CPU 负载稳定**";

    await act(async () => {
      root.render(<ProcessTranscript process={[]} turnStatus="working" finalText={markdown} />);
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeTruthy();
    expect(container.textContent).toContain("处理中");
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-final-text"]')).toBeNull();
    expect(container.textContent).not.toContain("Nginx 正常");

    await act(async () => {
      root.render(<ProcessTranscript process={[]} turnStatus="completed" finalText={markdown} />);
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeNull();
    expect(container.querySelectorAll("ol li")).toHaveLength(2);
    expect(container.querySelector("strong")?.textContent).toBe("Nginx 正常");
    expect(container.textContent).not.toContain("**Nginx 正常**");
  });

  it("can hide final text when another renderer owns the answer document", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[]}
          turnStatus="completed"
          finalText="根因：外部依赖异常。"
          renderFinalText={false}
        />,
      );
    });

    expect(container.textContent).not.toContain("外部依赖异常");
  });

  it("does not repeat completed final answer blocks when another renderer owns the answer document", async () => {
    const final = "已启动 nginx 容器，映射主机端口 1234 -> 容器端口 80。";
    const process = [
      makeBlock({
        id: "assistant-prelude",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.process",
        text: "我先检查端口，然后启动容器。",
      }),
      makeBlock({
        id: "cmd-docker-run",
        kind: "command",
        status: "completed",
        command: "docker run -d --name nginx -p 1234:80 nginx:latest",
        outputPreview: "container-id",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          finalText={final}
          renderFinalText={false}
        />,
      );
    });
    await expandProcessTranscript();

    const text = container.textContent || "";
    expect(text).toContain("我先检查端口，然后启动容器。");
    expect(text).toContain("已运行 docker run -d --name nginx -p 1234:80 nginx:latest");
    expect(text).not.toContain("verification completion gate");
    expect(text).not.toContain(final);
  });

  it("keeps assistant, command, and approval rows in their process order", async () => {
    const process = [
      makeBlock({
        id: "assistant-before-command",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "先检查网络配置。",
      }),
      makeBlock({
        id: "cmd-ip-addr",
        kind: "command",
        status: "failed",
        command: "ip addr show",
        outputPreview: "needs_host_agent",
      }),
      makeBlock({
        id: "assistant-before-approval",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "命令不可用，改为请求只读日志检查。",
      }),
      makeBlock({
        id: "approval-read-log",
        kind: "approval",
        status: "blocked",
        text: "需要审批：读取 /var/log",
        approvalId: "approval-read-log",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="blocked" />);
    });

    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    const assistantBeforeCommand = processText.indexOf("先检查网络配置");
    const command = processText.indexOf("运行失败 ip addr show");
    const assistantBeforeApproval = processText.indexOf("改为请求只读日志检查");
    const approval = processText.indexOf("需要审批：读取 /var/log");

    expect(assistantBeforeCommand).toBeGreaterThanOrEqual(0);
    expect(command).toBeGreaterThan(assistantBeforeCommand);
    expect(assistantBeforeApproval).toBeGreaterThan(command);
    expect(approval).toBeGreaterThan(assistantBeforeApproval);
  });

  it("keeps preserved final output out of the process transcript when a raw stream error is present", async () => {
    const rawError = "failed to receive stream chunk: context deadline exceeded";
    const preservedAnswer = [
      "Now I have enough context from the PostgreSQL PITR documentation and pg_auto_failover operations guide.",
      "",
      "根因：pgBackRest 恢复主机A后 promote 产生了新的 timeline 分支；归档仓库中仍保留旧集群的历史 timeline 文件。",
      "",
      "机制：从节点执行 pg_autoctl create postgres 后会通过 pg_basebackup 和 restore_command 获取 WAL；如果 recovery_target_timeline=latest 跟随了更高的历史 timeline，就会出现 timeline 分叉不兼容。",
      "",
      "下一步：先核对主机 A 和主机 B 的 pg_controldata timeline，再检查恢复残留配置和归档 timeline history。",
    ].join("\n");
    const process = [
      makeBlock({
        id: "assistant-prelude-search",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "I'll search for relevant documentation to supplement the analysis.",
      }),
      makeBlock({
        id: "web-search",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        inputSummary: "pg_auto_failover pgBackRest timeline divergence",
        results: [
          {
            title: "PostgreSQL continuous archiving",
            url: "https://www.postgresql.org/docs/current/continuous-archiving.html",
          },
        ],
      }),
      makeBlock({
        id: "stream-error",
        kind: "system",
        status: "failed",
        text: rawError,
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="failed" finalText={preservedAnswer} />);
    });
    await expandProcessTranscript();

    const visibleText = container.textContent || "";
    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(visibleText).toContain("pgBackRest 恢复主机A后 promote");
    expect(processText).toContain("I'll search for relevant documentation");
    expect(processText).not.toContain("Now I have enough context");
    expect(processText).not.toContain("pgBackRest 恢复主机A后 promote");
  });

  it("keeps running assistant commentary in process without promoting it to final text", async () => {
    const commentary = "我会先核对恢复后的主节点 timeline，再决定是否需要进一步检查。";
    const process = [
      makeBlock({
        id: "assistant-commentary",
        kind: "assistant",
        status: "running",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "streaming",
        text: commentary,
      }),
      makeBlock({
        id: "wait-after-draft",
        kind: "reasoning",
        status: "running",
        text: "正在等待模型返回",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.querySelector('[data-testid="aiops-live-answer-text"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-final-text"]')).toBeNull();
    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(processText).toContain("恢复后的主节点 timeline");
    expect(processText).toContain("正在等待模型返回");
  });

  it("shows at most one typed model prelude per iteration", async () => {
    const process = [
      makeBlock({
        id: "model-prelude-first",
        kind: "assistant",
        status: "completed",
        phase: "commentary",
        streamState: "complete",
        commentarySource: "model_prelude",
        iteration: 2,
        text: "本轮先采集只读证据。",
      }),
      makeBlock({
        id: "model-prelude-duplicate",
        kind: "assistant",
        status: "completed",
        phase: "commentary",
        streamState: "complete",
        commentarySource: "model_prelude",
        iteration: 2,
        text: "同一轮重复说明不应出现。",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const bodyText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(bodyText).toContain("本轮先采集只读证据。");
    expect(bodyText).not.toContain("同一轮重复说明不应出现。");
  });

  it("keeps final output stable when assistant_message final becomes completed", async () => {
    const running = [
      makeBlock({
        id: "assistant-commentary-0",
        kind: "assistant",
        status: "running",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "streaming",
        text: "我正在整理最终结论。",
      }),
    ];
    const completed: AiopsProcessBlock[] = [];

    await act(async () => {
      root.render(<ProcessTranscript process={running} turnStatus="working" />);
    });
    expect(container.querySelector('[data-testid="aiops-live-answer-text"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent).toContain("我正在整理最终结论");
    expect(container.querySelector('[data-testid="aiops-final-text"]')).toBeNull();

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={completed}
          turnStatus="completed"
          finalText="第一段分析。最终结论。"
        />,
      );
    });
    const text = container.textContent || "";
    expect(text).toContain("第一段分析。最终结论。");
    expect((text.match(/第一段分析。最终结论。/g) || []).length).toBe(1);
    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeNull();
  });

  it("does not rely on superseded final candidate blocks in the process transcript", async () => {
    const oldDraft = [
      "根因（置信度：中）",
      "这是一段已经被后续模型输出替换的候选答案，里面包含很多机制分析和下一步检查。",
      "不应该继续作为过程大段文本展示。",
    ].join("\n");
    const process = [
      makeBlock({
        id: "assistant-progress",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "我会基于新证据重新整理结论。",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(processText).not.toContain("已修订一次候选答案");
    expect(processText).not.toContain(oldDraft);
    expect(processText).not.toContain("这是一段已经被后续模型输出替换");
    expect(processText).toContain("我会基于新证据重新整理结论");
  });

  it("renders the typed normalized stream error without inspecting raw error strings", async () => {
    const answer = "根因：已生成的分析内容应该保留。";
    const process = [
      makeBlock({
        id: "stream-error",
        kind: "system",
        displayKind: "runtime.error",
        status: "failed",
        text: "模型流中断，已保留已生成内容",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="failed" finalText={answer} />);
    });
    await expandProcessTranscript();

    expect(container.querySelector('[data-testid="aiops-final-text"]')?.textContent).toContain("已生成的分析内容应该保留");
    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(processText).toContain("模型流中断，已保留已生成内容");
    expect(processText).not.toContain("failed to receive stream chunk");
  });

  it("renders route progress summary in user-readable process text", async () => {
    const process = [
      makeBlock({
        id: "route-summary",
        kind: "system",
        status: "completed",
        displayKind: "route.summary",
        text: "已识别为证据分析；不会执行主机命令；优先检索官方资料",
        outputPreview: "不会执行主机命令；优先检索官方资料",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const text = container.textContent || "";
    expect(text).toContain("已识别为证据分析");
    expect(text).toContain("不会执行主机命令");
    expect(text).toContain("优先检索官方资料");
  });

  it("renders the upstream typed safety decision without rescanning final prose", async () => {
    const safeDecision = "安全策略已阻止该高风险操作。";
    const process = [
      makeBlock({
        id: "visible-system",
        kind: "system",
        status: "completed",
        displayKind: "safety.decision",
        text: safeDecision,
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" finalText={safeDecision} />);
    });
    await expandProcessTranscript();

    const text = container.textContent || "";
    expect(text).toContain(safeDecision);
  });

  it("renders final answer text one step smaller without changing tool transcript text", async () => {
    const process = [
      makeBlock({
        id: "cmd-font-baseline",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          finalText="正文回答应该比之前小一号。"
        />,
      );
    });
    await expandProcessTranscript();

    expect(container.querySelector('[data-testid="aiops-final-text"]')?.className).toContain("text-[15px]");
    expect(container.querySelector('[data-testid="aiops-final-text"]')?.className).toContain("leading-7");
    expect(container.querySelector('[data-testid="aiops-command-row-cmd-font-baseline"]')?.className).toContain("text-[14px]");
    expect(container.querySelector('[data-testid="aiops-command-row-cmd-font-baseline"]')?.className).toContain("leading-6");
  });

  it("keeps the running process header without exposing an uncommitted final draft", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[]}
          turnStatus="working"
          turnStartedAt="2026-05-07T10:00:00Z"
          finalText="我先复查主机当前的 CPU、内存、磁盘和负载情况，再给你一个最新快照。"
        />,
      );
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeTruthy();
    expect(container.textContent).toContain("处理中");
    expect(container.textContent).not.toContain("我先复查主机当前的 CPU、内存、磁盘和负载情况");
    expect(container.querySelector('[data-testid="aiops-final-text"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
  });

  it("renders agent steps as a collapsible process flow when process blocks are absent", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[]}
          turnStatus="completed"
          agentSteps={[
            {
              id: "step-search",
              kind: "tool_search",
              status: "completed",
              title: "搜索可用工具",
              toolName: "tool_search",
              inputSummary: "checkout service metrics",
            },
            {
              id: "step-tool",
              kind: "tool_call",
              status: "completed",
              title: "读取 Coroot 指标",
              toolName: "coroot.service_metrics",
              toolCallId: "call-coroot-1",
              outputSummary: "p95 latency high",
              targetRefs: ["service:checkout"],
              evidenceRefs: ["evidence-coroot-1"],
            },
          ]}
        />,
      );
    });
    await expandProcessTranscript();

    expect(container.textContent).toContain("搜索可用工具");
    expect(container.textContent).toContain("读取 Coroot 指标");
    expect(container.textContent).toContain("coroot.service_metrics");
    expect(container.textContent).toContain("已完成");
  });

  it("preserves skill_search arguments in tool progress text", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "skill-search-render",
              kind: "tool",
              displayKind: "skill_search",
              text: "skill_search mode=search query=synthetic diagnosis",
            }),
          ]}
          turnStatus="completed"
        />,
      );
    });

    await expandProcessTranscript();

    expect(container.textContent).toContain("skill_search mode=search query=synthetic diagnosis");
    expect(container.textContent).not.toContain("网页搜索");
  });

  it("shows the active web search query in the running search summary", async () => {
    const process = [
      makeBlock({
        id: "search-1",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        text: "正在搜索网页",
        inputSummary: "BTC 行情",
        queries: ["BTC 行情"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.textContent).toContain("正在搜索网页：BTC 行情");
    expect(container.textContent).toContain("BTC 行情");
  });

  it("keeps the running web search row stable while showing the active query in details", async () => {
    const process = [
      makeBlock({
        id: "search-stable-running",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        text: "正在搜索网页",
        inputSummary: "pg_autoctl standby timeline higher than primary",
        queries: ["pg_autoctl standby timeline higher than primary"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const toggle = container.querySelector('[data-testid="aiops-search-toggle"]');
    expect(toggle?.textContent).toContain("网页搜索 1 次");
    expect(toggle?.textContent).not.toContain("正在搜索网页");
    expect(container.querySelector('[data-testid="aiops-search-details"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-search-running-status"]')?.textContent).toContain(
      "正在搜索网页：pg_autoctl standby timeline higher than primary",
    );
    expect(container.querySelector('[data-testid="aiops-inline-status-indicator"]')).toBeNull();
  });

  it("does not render spinning indicators while a turn is processing", async () => {
    const process = [
      makeBlock({
        id: "search-2",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        text: "正在搜索网页",
        inputSummary: "BTC 行情",
        queries: ["BTC 行情"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.textContent).toContain("处理中");
    expect(container.querySelector(".animate-spin")).toBeNull();
  });

  it("hides internal LLM endpoints and raw tool payloads from search details", async () => {
    const process = [
      makeBlock({
        id: "search-3",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        foldGroupId: "lookup-btc",
        foldGroupKind: "web_lookup",
        inputSummary: "BTC price today USD",
        outputPreview:
          'failed: Post "http://127.0.0.1:8317/v1/responses": context deadline exceeded (Client.Timeout exceeded while awaiting headers)',
      }),
      makeBlock({
        id: "search-4",
        kind: "tool",
        status: "completed",
        displayKind: "browse_url",
        foldGroupId: "lookup-btc",
        foldGroupKind: "web_lookup",
        inputSummary: "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd",
        outputPreview:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978}}","url":"https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"}',
      }),
      makeBlock({
        id: "search-5",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        foldGroupId: "lookup-btc",
        foldGroupKind: "web_lookup",
        inputSummary: "Searching the web",
        outputPreview: '{"contentType":"application/json; charset=utf-8","text":"raw response body"}',
      }),
      makeBlock({
        id: "search-6",
        kind: "tool",
        status: "completed",
        displayKind: "browser.open",
        foldGroupId: "lookup-btc",
        foldGroupKind: "web_lookup",
        inputSummary:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978}}","url":"https://api.coingecko.com/api/v3/simple/price?ids=bitcoin\\u0026vs_currencies=usd"}',
        outputPreview:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978',
      }),
      makeBlock({
        id: "search-7",
        kind: "search",
        status: "completed",
        foldGroupId: "lookup-btc",
        foldGroupKind: "web_lookup",
        text: "BTC price today USD",
        queries: ["BTC price today USD"],
        results: [
          {
            title: "Bitcoin Price: BTC/USD Live Price Chart, Market Cap & News Today",
            url: "https://www.coingecko.com/en/coins/bitcoin",
            snippet: '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978}}"}',
          },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    expect(container.textContent).toContain("网页搜索 5 次 · 找到 2 个来源");
    const searchButton = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("网页搜索"),
    );
    expect(searchButton).toBeTruthy();

    await act(async () => {
      searchButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const text = container.textContent || "";
    expect(text).toContain("https://api.coingecko.com/api/v3/simple/price");
    expect(text).toContain("https://www.coingecko.com/en/coins/bitcoin");
    expect(text).not.toContain("Bitcoin Price: BTC/USD Live Price Chart");
    expect(text).not.toContain("127.0.0.1:8317");
    expect(text).not.toContain("/v1/responses");
    expect(text).not.toContain("contentType");
    expect(text).not.toContain('"bitcoin"');

    const rows = Array.from(container.querySelectorAll('[data-testid="aiops-search-detail-row-toggle"]'));
    expect(rows[1]?.querySelector('[data-testid="aiops-search-detail-chevron"]')).toBeNull();
    expect(text).not.toContain("检索内容：");
    expect(text).not.toContain("检索词：");
    expect(text).not.toContain("摘要：");
  });

  it("lists every web search source as an expandable row without omitted summaries", async () => {
    const process = [
      makeBlock({
        id: "search-compact-1",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        foldGroupId: "lookup-compact",
        foldGroupKind: "web_lookup",
        inputSummary: "pg_auto_failover standby timeline higher than primary",
        queries: ["pg_auto_failover standby timeline higher than primary"],
        results: [
          { title: "pg_auto_failover operations", url: "https://pg-auto-failover.readthedocs.io/en/main/operations.html", snippet: "Monitor and failover operations." },
          { title: "pg_auto_failover state machine", url: "https://pg-auto-failover.readthedocs.io/en/main/failover-state-machine.html", snippet: "State transitions for nodes." },
          { title: "pg_auto_failover FAQ", url: "https://pg-auto-failover.readthedocs.io/en/main/faq.html", snippet: "Common operational questions." },
        ],
      }),
      makeBlock({
        id: "search-compact-2",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        foldGroupId: "lookup-compact",
        foldGroupKind: "web_lookup",
        inputSummary: "pgBackRest restore timeline recovery_target_timeline latest",
        queries: ["pgBackRest restore timeline recovery_target_timeline latest"],
        results: [
          { title: "pgBackRest restore", url: "https://pgbackrest.org/user-guide.html#restore", snippet: "Restore guidance." },
          { title: "pgBackRest command reference", url: "https://pgbackrest.org/command.html#command-restore", snippet: "Restore options." },
          { title: "PostgreSQL PITR", url: "https://www.postgresql.org/docs/current/continuous-archiving.html", snippet: "PITR and timeline history." },
          { title: "PostgreSQL recovery_target_timeline", url: "https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE", snippet: "Timeline target setting." },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const label = container.querySelector('[data-testid="aiops-search-toggle"]')?.textContent || "";
    expect(label).toContain("网页搜索 2 次");
    expect(label).toContain("找到 7 个来源");
    expect(label).not.toContain("网页搜索 17 项");

    expect(container.querySelector('[data-testid="aiops-search-details"]')).toBeNull();
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const details = container.querySelector('[data-testid="aiops-search-details"]');
    expect(details?.textContent).toContain("https://pg-auto-failover.readthedocs.io/en/main/operations.html");
    expect(details?.textContent).toContain("https://www.postgresql.org/docs/current/runtime-config-wal.html");
    expect(details?.textContent).not.toContain("pg_auto_failover operations");
    expect(details?.textContent).not.toContain("PostgreSQL recovery_target_timeline");
    expect(details?.textContent).not.toContain("检索词是系统自动生成的搜索关键词");
    expect(details?.textContent).not.toContain("检索词：pg_auto_failover standby timeline higher than primary");
    expect(details?.textContent).not.toContain("参考来源：");
    expect(details?.textContent).not.toContain("已省略");
    expect(details?.textContent).not.toContain("query:");
    expect(details?.querySelectorAll('[data-testid="aiops-search-detail-line"]').length).toBe(7);

    const firstSource = details?.querySelector('[data-testid="aiops-search-detail-row-toggle"]');
    const firstChevron = firstSource?.querySelector('[data-testid="aiops-search-detail-chevron"]');
    expect(firstChevron?.getAttribute("class")).toContain("mt-[5px]");
    await act(async () => {
      firstSource?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    const expanded = details?.querySelector('[data-testid="aiops-search-detail-expanded"]')?.textContent || "";
    expect(expanded).toContain("Monitor and failover operations.");
    expect(expanded).not.toContain("检索内容：");
    expect(expanded).not.toContain("检索词：");
    expect(expanded).not.toContain("摘要：");
  });

  it("renders web_search open operations inside the web lookup group", async () => {
    const process = [
      makeBlock({
        id: "open-docs",
        kind: "search",
        status: "completed",
        displayKind: "web_search",
        text: "https://www.postgresql.org/docs/current/",
        inputSummary: "https://www.postgresql.org/docs/current/",
        queries: ["https://www.postgresql.org/docs/current/"],
        results: [
          {
            title: "PostgreSQL docs",
            url: "https://www.postgresql.org/docs/current/",
            snippet: "Readable PostgreSQL documentation.",
            fetched: true,
            text: "Full bounded text is not rendered in the process transcript.",
          },
        ],
        foldGroupKind: "web_lookup",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.textContent).toContain("网页搜索 1 次");
    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.textContent).toContain("找到 1 个来源");
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const details = container.querySelector('[data-testid="aiops-search-details"]');
    expect(details?.textContent).toContain("https://www.postgresql.org/docs/current/");
    expect(details?.textContent).not.toContain("Full bounded text");
  });

  it("shows fetched and failed read states only in expanded web lookup details", async () => {
    const process = [
      makeBlock({
        id: "search-fetch-status",
        kind: "search",
        status: "completed",
        displayKind: "web_search",
        text: "postgres docs",
        queries: ["postgres docs"],
        results: [
          {
            title: "PostgreSQL docs",
            url: "https://www.postgresql.org/docs/current/",
            snippet: "Readable PostgreSQL documentation.",
            fetched: true,
            text: "Full bounded text is hidden.",
          },
          {
            title: "Example blog",
            url: "https://example.com/post",
            fetchError: "blocked by policy",
          },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).not.toContain("已读取正文");
    const rows = Array.from(container.querySelectorAll('[data-testid="aiops-search-detail-row-toggle"]'));
    await act(async () => {
      rows[0]?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(container.querySelector('[data-testid="aiops-search-detail-expanded"]')?.textContent).toContain("已读取正文");

    await act(async () => {
      rows[1]?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    const expandedText = Array.from(container.querySelectorAll('[data-testid="aiops-search-detail-expanded"]'))
      .map((node) => node.textContent || "")
      .join("\n");
    expect(expandedText).toContain("读取失败：blocked by policy");
    expect(expandedText).not.toContain("Full bounded text is hidden.");
  });

  it("uses compact text and reduced indent for running search labels and details", async () => {
    const process = [
      makeBlock({
        id: "search-font",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        inputSummary: "2026-05-07 BTC price today USD",
        queries: ["2026-05-07 BTC price today USD"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.className).toContain("text-[14px]");
    expect(container.querySelector('[data-testid="aiops-search-details"]')).toBeNull();
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("text-[14px]");
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("pl-3");
    const searchToggle = container.querySelector('[data-testid="aiops-search-toggle"]');
    const searchIcon = container.querySelector('[data-testid="aiops-search-icon"]');
    expect(searchIcon).toBeTruthy();
    expect(searchToggle?.firstElementChild).toBe(searchIcon);
  });

  it("keeps model wait reasoning trace-only", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "model-wait",
              kind: "reasoning",
              status: "running",
              text: "正在等待模型返回",
            }),
          ]}
          turnStatus="working"
        />,
      );
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-model-wait-pill"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-model-wait-icon"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-inline-status-indicator"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
    expect(container.textContent).not.toContain("正在等待模型返回");
  });

  it("wraps long searched urls instead of truncating them", async () => {
    const url =
      "https://www.coingecko.com/en/coins/bitcoin?utm_source=aiops&include_market_cap=true&include_24hr_change=true";
    const process = [
      makeBlock({
        id: "search-long-url",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        inputSummary: "BTC current price USD",
        queries: ["BTC current price USD"],
        results: [{ url }],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const row = container.querySelector('[data-testid="aiops-search-detail-row-toggle"]');
    expect(row?.textContent).toContain(url);
    const urlText = row?.querySelector("span:nth-child(2)");
    expect(urlText?.className).toContain("break-all");
    expect(urlText?.className).not.toContain("truncate");
  });

  it("keeps web search details collapsed after completion while the turn is still running", async () => {
    const runningProcess = [
      makeBlock({
        id: "search-stable",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        inputSummary: "pg_autoctl create postgres standby pgBackRest restore timeline",
        queries: ["pg_autoctl create postgres standby pgBackRest restore timeline"],
        results: [
          {
            title: "pg_auto_failover operations",
            url: "https://pg-auto-failover.readthedocs.io/en/main/operations.html",
            snippet: "pg_autoctl create postgres starts a local PostgreSQL instance and joins the monitor.",
          },
        ],
      }),
    ];
    const completedProcess = [
      {
        ...runningProcess[0],
        status: "completed" as const,
      },
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={runningProcess} turnStatus="working" />);
    });
    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector('[data-testid="aiops-search-details"]')).toBeNull();

    await act(async () => {
      root.render(<ProcessTranscript process={completedProcess} turnStatus="working" />);
    });

    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.getAttribute("aria-expanded")).toBe("false");
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    const details = container.querySelector('[data-testid="aiops-search-details"]')?.textContent || "";
    expect(details).toContain("https://pg-auto-failover.readthedocs.io/en/main/operations.html");
    expect(details).not.toContain("pg_auto_failover operations");
    expect(details).not.toContain("pg_autoctl create postgres starts");

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-detail-row-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    const expanded = container.querySelector('[data-testid="aiops-search-detail-expanded"]')?.textContent || "";
    expect(expanded).toContain("pg_autoctl create postgres starts a local PostgreSQL instance");
    expect(expanded).not.toContain("检索内容：");
    expect(expanded).not.toContain("检索词：");
    expect(expanded).not.toContain("摘要：");
  });

  it("starts completed web search details collapsed while the turn is still running", async () => {
    const process = [
      makeBlock({
        id: "search-completed-active-turn",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        inputSummary: "pg_auto_failover monitor standby failure",
        queries: ["pg_auto_failover monitor standby failure"],
        results: [
          {
            title: "pg_auto_failover failover state machine",
            url: "https://pg-auto-failover.readthedocs.io/en/main/failover-state-machine.html",
            snippet: "The monitor tracks nodes and assigns states for primary and secondary nodes.",
          },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector('[data-testid="aiops-search-details"]')).toBeNull();
    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    const details = container.querySelector('[data-testid="aiops-search-details"]')?.textContent || "";
    expect(details).toContain("https://pg-auto-failover.readthedocs.io/en/main/failover-state-machine.html");
    expect(details).not.toContain("pg_auto_failover failover state machine");

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-detail-row-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    const expanded = container.querySelector('[data-testid="aiops-search-detail-expanded"]')?.textContent || "";
    expect(expanded).toContain("The monitor tracks nodes and assigns states");
    expect(expanded).not.toContain("检索内容：");
    expect(expanded).not.toContain("检索词：");
    expect(expanded).not.toContain("摘要：");
  });

  it("keeps streaming assistant prelude before the following search block", async () => {
    const prelude = "我将先通过实时网页搜索核实BTC当前行情与主要价格来源，然后返回简洁结果并附上来源。";
    const process = [
      makeBlock({
        id: "assistant-prelude",
        kind: "assistant",
        status: "running",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "streaming",
        text: prelude,
      }),
      makeBlock({
        id: "search-after-prelude",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        text: "正在搜索网页",
        inputSummary: "BTC current price USD 24h change",
        queries: ["BTC current price USD 24h change"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" finalText={prelude} />);
    });

    const bodyText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    const preludeIndex = bodyText.indexOf(prelude);
    const searchIndex = bodyText.indexOf("正在搜索网页：BTC current price USD 24h change");
    expect(preludeIndex).toBeGreaterThanOrEqual(0);
    expect(searchIndex).toBeGreaterThan(preludeIndex);
    expect(container.querySelectorAll('[data-testid="aiops-final-text"]')).toHaveLength(0);
  });

  it("renders assistant commentary between folded tool groups without pulling final text into process", async () => {
    const finalText = "结论：CPU 当前负载正常，未发现持续高负载。";
    const process = [
      makeBlock({
        id: "assistant-search",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "我会先检索可用工具并确认适合的只读检查能力，再继续获取证据。",
      }),
      makeBlock({
        id: "tool-search-1",
        kind: "tool",
        displayKind: "tool_search",
        foldGroupKind: "web_lookup",
        text: "tool_search",
        inputSummary: "host CPU monitoring status check server local",
      }),
      makeBlock({
        id: "assistant-command",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "我会先执行只读命令获取证据，再根据输出给出结论。",
      }),
      makeBlock({
        id: "cmd-cpu",
        kind: "command",
        foldGroupKind: "command",
        command: "top -l 1 | head",
        outputPreview: "CPU usage: 10% user, 15% sys, 75% idle",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" finalText={finalText} />);
    });
    await expandProcessTranscript();

    const processText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(processText).toContain("检索可用工具");
    expect(processText).toContain("执行只读命令");
    expect(processText).toContain("已运行 top -l 1 | head");
    expect(processText).not.toContain(finalText);
    expect(container.querySelector('[data-testid="aiops-final-text"]')?.textContent).toContain(finalText);
  });

  it("keeps stale substantial assistant drafts out of the running process timeline", async () => {
    const draft = [
      "基于 PostgreSQL timeline 机制原理和 pgBackRest 恢复流程，我现在可以给出完整分析。",
      "",
      "根因（置信度：中）",
      "主机B 的 timeline 比主机A 更高，导致 standby 加入时 WAL lineage 校验失败。",
      "",
      "机制链条：",
      "1. pgBackRest 恢复后 PostgreSQL 会 promote 并创建新的 timeline 分支。",
      "2. pg_auto_failover 只记录 monitor 中的节点角色，不会自动修复 timeline 分叉。",
      "3. standby 通过 primary_conninfo 跟随 primary 时会校验 WAL 历史，timeline 不兼容会中断复制。",
    ].join("\n");
    const process = [
      makeBlock({
        id: "assistant-plan",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.process",
        text: "我会先核对 PostgreSQL timeline 和 pgBackRest 恢复机制，再给出结论。",
      }),
      makeBlock({
        id: "search-docs",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        inputSummary: "PostgreSQL PITR timeline pgBackRest restore",
        queries: ["PostgreSQL PITR timeline pgBackRest restore"],
        results: [
          {
            title: "PostgreSQL Continuous Archiving",
            url: "https://www.postgresql.org/docs/current/continuous-archiving.html",
          },
        ],
      }),
      makeBlock({
        id: "wait-after-draft",
        kind: "reasoning",
        status: "running",
        text: "正在等待模型返回",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const bodyText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    expect(bodyText).toContain("我会先核对 PostgreSQL timeline");
    expect(bodyText).toContain("网页搜索 1 次");
    expect(bodyText).toContain("正在等待模型返回");
    expect(bodyText).not.toContain("根因（置信度：中）");
    expect(bodyText).not.toContain("主机B 的 timeline 比主机A 更高");
    const renderedAssistantText = Array.from(container.querySelectorAll('[data-testid="aiops-assistant-progress-text"]'))
      .map((node) => node.textContent || "")
      .join("\n");
    expect(renderedAssistantText).toContain("我会先核对 PostgreSQL timeline");
    expect(renderedAssistantText).not.toContain("根因（置信度：中）");
  });

  it("points the search disclosure arrow down while expanded", async () => {
    const process = [
      makeBlock({
        id: "search-chevron",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        inputSummary: "BTC 行情",
        queries: ["BTC 行情"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const chevron = container.querySelector('[data-testid="aiops-search-chevron"]');
    expect(chevron?.getAttribute("class")).toContain("-rotate-90");

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(chevron?.getAttribute("class")).toContain("rotate-0");
  });

  it("shows command details for merged command groups", async () => {
    const process = [
      makeBlock({
        id: "cmd-1",
        kind: "command",
        foldGroupId: "commands-1",
        foldGroupKind: "command",
        command: "kubectl get pods -n prod",
        text: "kubectl get pods",
      }),
      makeBlock({
        id: "cmd-2",
        kind: "command",
        status: "blocked",
        foldGroupId: "commands-1",
        foldGroupKind: "command",
        command: "kubectl rollout restart deployment/api -n prod",
        text: "kubectl rollout restart deployment/api",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    const text = container.textContent || "";
    expect(text).toContain("已运行 2 条命令");
    expect(text).toContain("kubectl get pods -n prod");
    expect(text).toContain("kubectl rollout restart deployment/api -n prod");
    expect(text).toContain("等待审核");
  });

  it("renders plan steps instead of compact plan summary", async () => {
    const process = [
      makeBlock({
        id: "plan-1",
        kind: "plan",
        displayKind: "plan",
        status: "completed",
        text: "plan updated: active (1/4 in_progress)",
        steps: [
          { id: "metrics", text: "查询 Redis RSS 与 used_memory 指标", status: "in_progress" },
          { id: "events", text: "读取最近 30 分钟 Kubernetes events", status: "pending" },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.textContent).not.toContain("plan updated: active");
    expect(container.textContent).toContain("查询 Redis RSS 与 used_memory 指标");
    expect(container.textContent).toContain("读取最近 30 分钟 Kubernetes events");
  });

  it("shows mock rows without leaking internal evidence refs", async () => {
    const process = [
      makeBlock({
        id: "tool-mock-evidence",
        kind: "tool",
        displayKind: "coroot.metrics",
        text: "rss/used_memory ratio is 1.8",
        mock: true,
        evidenceRefs: ["evidence:redis:rss", "evidence:redis:events"],
      }),
      makeBlock({
        id: "reasoning-separator",
        kind: "reasoning",
        text: "继续核对 Kubernetes events。",
      }),
      makeBlock({
        id: "cmd-mock-evidence",
        kind: "command",
        command: "kubectl get events -n prod",
        mock: true,
        evidenceRefs: ["evidence:k8s:events"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    expect(container.querySelector('[data-testid="aiops-tool-row-tool-mock-evidence"]')?.textContent).toContain("Mock");
    expect(container.querySelector('[data-testid="aiops-command-row-cmd-mock-evidence"]')?.textContent).toContain("Mock");
    expect(container.textContent).not.toContain("evidence:redis:rss");
    expect(container.textContent).not.toContain("evidence:redis:events");
    expect(container.textContent).not.toContain("evidence:k8s:events");
  });

  it("lets expanded command rows in merged groups grow without clipping sibling rows", async () => {
    const process = [
      makeBlock({
        id: "cmd-grow-1",
        kind: "command",
        foldGroupId: "commands-grow",
        foldGroupKind: "command",
        status: "completed",
        command: "ifconfig en0",
        text: "ifconfig en0",
        outputPreview: [
          "en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500",
          "options=6460<TSO4,TSO6,CHANNEL_IO,PARTIAL_CSUM,ZEROINVERT_CSUM>",
          "ether 4e:61:3a:2f:f3:bd",
          "inet 172.20.219.37 netmask 0xffffff00 broadcast 172.20.219.255",
          "status: active",
        ].join("\n"),
      }),
      makeBlock({
        id: "cmd-grow-2",
        kind: "command",
        foldGroupId: "commands-grow",
        foldGroupKind: "command",
        status: "failed",
        command: "ifconfig en0 down",
        text: "ifconfig en0 down",
        outputPreview: "ifconfig: down: permission denied",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    const mergedDetails = container.querySelector('[data-testid="aiops-merged-command-details"]');
    expect(mergedDetails).toBeTruthy();
    expect(mergedDetails?.className).not.toContain("max-h-64");
    expect(mergedDetails?.className).not.toContain("overflow-y-auto");

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-grow-1"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.querySelector('[data-testid="aiops-terminal-card-cmd-grow-1"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-command-row-cmd-grow-2"]')).toBeTruthy();
  });

  it("does not render approval controls inside the process transcript", async () => {
    const decisions: Array<{ id: string; decision: "accept" | "reject" }> = [];
    const process = [
      makeBlock({
        id: "cmd-approval",
        kind: "command",
        status: "blocked",
        command: "ifconfig en0 down",
        approvalId: "approval-1",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="blocked"
          onApprovalDecision={(id, decision) => decisions.push({ id, decision })}
        />,
      );
    });

    expect(container.textContent).toContain("ifconfig en0 down");
    expect(container.textContent).toContain("等待审核");
    expect(container.textContent).not.toContain("批准");
    expect(container.textContent).not.toContain("拒绝");
    expect(container.querySelector('[data-testid="aiops-inline-approval-approval-1"]')).toBeNull();
    expect(decisions).toEqual([]);
  });

  it("keeps rejected approval blocks as readonly audit trail", async () => {
    const decisions: Array<{ id: string; decision: "accept" | "reject" }> = [];
    const process = [
      makeBlock({
        id: "approval-rejected",
        kind: "approval",
        status: "rejected",
        text: "需要执行高风险命令",
        command: "systemctl restart postgresql",
        approvalId: "approval-1",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          onApprovalDecision={(id, decision) => decisions.push({ id, decision })}
        />,
      );
    });

    await expandProcessTranscript();

    expect(container.textContent).toContain(
      "已拒绝，将基于已有证据继续分析",
    );
    expect(container.textContent).toContain("systemctl restart postgresql");
    expect(
      Array.from(container.querySelectorAll("button")).map((button) =>
        button.textContent?.trim(),
      ),
    ).toEqual(["已处理"]);
    expect(container.querySelector('[data-testid="aiops-inline-approval-approval-1"]')).toBeNull();
    expect(decisions).toEqual([]);
  });

  it("uses turn timestamps for completed elapsed time and starts process details collapsed", async () => {
    const process = [
      makeBlock({
        id: "cmd-duration",
        kind: "command",
        command: "uptime",
        outputPreview: "up 22 days",
        updatedAt: "2026-05-07T10:00:01Z",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnCompletedAt="2026-05-07T10:00:12Z"
        />,
      );
    });

    expect(container.textContent).toContain("已处理 12s");
    const header = container.querySelector('[data-testid="aiops-process-header"]');
    expect(header?.tagName).toBe("BUTTON");
    expect(header?.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
  });

  it("renders only Coroot blocks admitted by the upstream presentation policy", async () => {
    const process = [
      makeBlock({
        id: "tool-coroot-broad",
        kind: "tool",
        status: "completed",
        source: "coroot_list_services",
        text: "coroot_list_services",
      }),
      makeBlock({
        id: "tool-coroot-incidents",
        kind: "tool",
        status: "completed",
        source: "coroot_incidents",
        text: "coroot_incidents",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });

    await expandProcessTranscript();

    expect(container.querySelector('[data-testid="aiops-tool-row-tool-coroot-broad"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-tool-row-tool-coroot-reused"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-tool-row-tool-coroot-incidents"]')).toBeTruthy();
    expect(container.textContent).not.toContain("covered_by_prior_broad_query");
    expect(container.textContent).not.toContain('{"status":"warning"}');
  });

  it("formats long elapsed times with hours minutes and seconds", async () => {
    const process = [
      makeBlock({
        id: "cmd-long-duration",
        kind: "command",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnCompletedAt="2026-05-07T13:04:20Z"
        />,
      );
    });

    expect(container.textContent).toContain("已处理 3h 4m 20s");
    expect(container.textContent).not.toContain("11060s");
  });

  it("shows blocked command turns as waiting instead of running", async () => {
    const process = [
      makeBlock({
        id: "cmd-blocked-launchctl",
        kind: "command",
        status: "blocked",
        command: "launchctl print system/com.docker.helper",
        approvalId: "evidence-1",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="blocked"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnUpdatedAt="2026-05-07T10:15:49Z"
        />,
      );
    });

    expect(container.textContent).toContain("等待审核 15m 49s");
    expect(container.textContent).toContain("等待审核 launchctl print system/com.docker.helper");
    expect(container.textContent).not.toContain("处理中");
    expect(container.textContent).not.toContain("正在运行");
    expect(container.querySelector('[data-testid="aiops-command-status-cmd-blocked-launchctl"]')).toBeNull();
  });

  it("keeps final answer visible while the completed process details are folded", async () => {
    const process = [
      makeBlock({
        id: "reason-folded-final",
        kind: "reasoning",
        text: "我先检查本机状态。",
      }),
      makeBlock({
        id: "cmd-folded-final",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnCompletedAt="2026-05-07T10:00:12Z"
          finalText="本机当前运行正常。"
        />,
      );
    });

    expect(container.textContent).toContain("已处理 12s");
    expect(container.textContent).toContain("本机当前运行正常。");
    expect(container.textContent).not.toContain("我先检查本机状态。");
    expect(container.textContent).not.toContain("已运行 uptime");

    await expandProcessTranscript();

    const text = container.textContent || "";
    const reasoning = text.indexOf("我先检查本机状态。");
    const command = text.indexOf("已运行 uptime");
    const final = text.indexOf("本机当前运行正常。");
    expect(reasoning).toBeGreaterThanOrEqual(0);
    expect(command).toBeGreaterThan(reasoning);
    expect(final).toBeGreaterThan(command);
  });

  it("adds hover/focus disclosure chevrons to collapsible labels", async () => {
    const process = [
      makeBlock({
        id: "cmd-chevron-1",
        kind: "command",
        foldGroupId: "commands-chevron",
        foldGroupKind: "command",
        status: "completed",
        command: "pwd",
        outputPreview: "/tmp",
      }),
      makeBlock({
        id: "cmd-chevron-2",
        kind: "command",
        foldGroupId: "commands-chevron",
        foldGroupKind: "command",
        status: "completed",
        command: "date",
        outputPreview: "Fri May 8",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });

    expect(container.querySelector('[data-testid="aiops-process-header-chevron"]')?.getAttribute("class")).toContain("group-hover:opacity-100");
    await expandProcessTranscript();
    expect(container.querySelector('[data-testid="aiops-merged-command-chevron"]')?.getAttribute("class")).toContain("group-hover:opacity-100");
    await act(async () => {
      container.querySelector('[data-testid="aiops-merged-command-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(container.querySelector('[data-testid="aiops-command-chevron-cmd-chevron-1"]')?.getAttribute("class")).toContain("group-hover:opacity-100");
  });

  it("keeps command disclosure chevron next to the command label and removes success status text", async () => {
    const process = [
      makeBlock({
        id: "cmd-label-chevron",
        kind: "command",
        status: "completed",
        command: "pwd",
        outputPreview: "/tmp",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    const row = container.querySelector('[data-testid="aiops-command-row-cmd-label-chevron"]');
    const labelRegion = container.querySelector('[data-testid="aiops-command-label-region-cmd-label-chevron"]');
    expect(row?.textContent).toBe("已运行 pwd");
    expect(labelRegion?.textContent).toBe("已运行 pwd");
    const commandIcon = labelRegion?.querySelector('[data-testid="aiops-command-icon-cmd-label-chevron"]');
    expect(commandIcon).toBeTruthy();
    expect(labelRegion?.firstElementChild).toBe(commandIcon);
    expect(labelRegion?.querySelector('[data-testid="aiops-command-chevron-cmd-label-chevron"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-command-status-cmd-label-chevron"]')).toBeNull();
  });

  it("renders final answer text inside the same transcript flow", async () => {
    const process = [
      makeBlock({
        id: "reason-before-final",
        kind: "reasoning",
        text: "我先检查本机状态。",
      }),
      makeBlock({
        id: "cmd-before-final",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
    ];
    const props = {
      process,
      turnStatus: "completed",
      turnStartedAt: "2026-05-07T10:00:00Z",
      turnCompletedAt: "2026-05-07T10:00:12Z",
      finalText: "本机当前运行正常。",
    } as unknown as Parameters<typeof ProcessTranscript>[0];

    await act(async () => {
      root.render(<ProcessTranscript {...props} />);
    });
    await expandProcessTranscript();

    const text = container.textContent || "";
    const reasoning = text.indexOf("我先检查本机状态。");
    const command = text.indexOf("已运行 uptime");
    const final = text.indexOf("本机当前运行正常。");
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).not.toBeNull();
    expect(reasoning).toBeGreaterThanOrEqual(0);
    expect(command).toBeGreaterThan(reasoning);
    expect(final).toBeGreaterThan(command);
  });

  it("renders assistant process text inline without duplicating fallback final text", async () => {
    const process = [
      makeBlock({
        id: "reason-before-inline-final",
        kind: "reasoning",
        text: "我先检查本机状态。",
      }),
      makeBlock({
        id: "assistant-inline-final",
        kind: "assistant",
        status: "completed",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "我会用 uptime 核对主机运行状态。",
      }),
      makeBlock({
        id: "cmd-after-inline-final",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" finalText="本机当前运行正常。" />);
    });
    await expandProcessTranscript();

    const text = container.textContent || "";
    expect(text.indexOf("我先检查本机状态。")).toBeGreaterThanOrEqual(0);
    expect(text.indexOf("我会用 uptime 核对主机运行状态。")).toBeGreaterThan(text.indexOf("我先检查本机状态。"));
    expect(text).toContain("已运行 uptime");
    expect(text.match(/本机当前运行正常。/g) || []).toHaveLength(1);
    expect(container.querySelectorAll('[data-testid="aiops-final-text"]')).toHaveLength(1);
  });

  it("folds process assistant text under the elapsed-time header while keeping the final answer visible", async () => {
    const process = [
      makeBlock({
        id: "assistant-before-commands",
        kind: "assistant",
        status: "completed",
        text: "我先在本机读取当前工作路径。",
      }),
      makeBlock({
        id: "cmd-after-assistant",
        kind: "command",
        status: "completed",
        command: "pwd",
        outputPreview: "/Users/lizhongxuan/Desktop/aiops/aiops-v2",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnCompletedAt="2026-05-07T10:00:25Z"
          finalText="当前工作路径已确认。"
        />,
      );
    });

    expect(container.textContent).toContain("已处理 25s");
    expect(container.textContent).toContain("当前工作路径已确认。");
    expect(container.textContent).not.toContain("我先在本机读取当前工作路径。");
    expect(container.textContent).not.toContain("已运行 pwd");

    await expandProcessTranscript();

    expect(container.textContent).toContain("我先在本机读取当前工作路径。");
    expect(container.textContent).toContain("已运行 pwd");
    const header = container.querySelector('[data-testid="aiops-process-header"]');
    expect(header?.tagName).toBe("BUTTON");
  });

  it("keeps mixed tool actions separate without crossing assistant text", async () => {
    const process = [
      makeBlock({ id: "file-a", kind: "file", text: "Read README.md" }),
      makeBlock({
        id: "search-a",
        kind: "tool",
        displayKind: "web_search",
        text: "Searching the web",
        inputSummary: "BTC 行情",
        queries: ["BTC 行情"],
      }),
      makeBlock({
        id: "cmd-a",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
      makeBlock({ id: "assistant-break", kind: "assistant", text: "拿到第一批证据。" }),
      makeBlock({ id: "file-b", kind: "file", text: "Read package.json" }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    const text = container.textContent || "";
    const firstFile = text.indexOf("Read README.md");
    const command = text.indexOf("uptime");
    const reasoning = text.indexOf("拿到第一批证据。");
    const laterFile = text.indexOf("Read package.json");
    expect(firstFile).toBeGreaterThanOrEqual(0);
    expect(container.querySelector('[data-testid="aiops-search-toggle"]')).toBeTruthy();
    expect(command).toBeGreaterThan(firstFile);
    expect(container.querySelector('[data-testid="aiops-merged-mixed-icon"]')).toBeNull();
    expect(reasoning).toBeGreaterThan(command);
    expect(laterFile).toBeGreaterThan(reasoning);
  });

  it("expands a single running generic tool and collapses its output after completion", async () => {
    const runningProcess = [
      makeBlock({
        id: "tool-running-output",
        kind: "tool",
        status: "running",
        displayKind: "mcp.action",
        text: "Collect host metrics",
        outputPreview: "cpu idle 72%",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={runningProcess} turnStatus="working" />);
    });

    expect(container.textContent).toContain("Collect host metrics");
    expect(container.textContent).toContain("cpu idle 72%");

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            {
              ...runningProcess[0],
              status: "completed",
            },
          ]}
          turnStatus="completed"
        />,
      );
    });

    expect(container.textContent).not.toContain("Collect host metrics");
    expect(container.textContent).not.toContain("cpu idle 72%");
    await expandProcessTranscript();
    expect(container.textContent).toContain("Collect host metrics");
    expect(container.textContent).not.toContain("cpu idle 72%");
  });

  it("does not keep a canceled turn running because of historical blocked blocks", async () => {
    const process = [
      makeBlock({
        id: "cmd-blocked-before-stop",
        kind: "command",
        status: "blocked",
        command: "sw_vers",
      }),
    ];

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="canceled"
          turnStartedAt="2026-05-07T10:00:00Z"
          turnUpdatedAt="2026-05-07T10:00:08Z"
        />,
      );
    });

    expect(container.textContent).toContain("已处理 8s");
    expect(container.textContent).not.toContain("处理中");
  });

  it("shows command text first and keeps completed terminal output collapsed", async () => {
    const process = [
      makeBlock({
        id: "cmd-output-1",
        kind: "command",
        status: "completed",
        command: "uptime",
        text: "uptime",
        outputPreview: "22:38 up 22 days, 8:23, 1 user",
      }),
      makeBlock({
        id: "cmd-output-2",
        kind: "command",
        status: "completed",
        command: "sysctl -n hw.ncpu",
        text: "sysctl -n hw.ncpu",
        outputPreview: "10",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    expect(container.textContent).toContain("uptime");
    expect(container.textContent).toContain("sysctl -n hw.ncpu");
    expect(container.textContent).not.toContain("22:38 up 22 days");

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-output-1"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("22:38 up 22 days");
  });

  it("renders a single completed command as one collapsed Codex-style summary line", async () => {
    const process = [
      makeBlock({
        id: "cmd-native-card",
        kind: "command",
        status: "completed",
        command: "git diff -- internal/appui/transport_projector.go",
        outputPreview: "diff --git a/internal/appui/transport_projector.go b/internal/appui/transport_projector.go",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    expect(container.textContent).toContain("已运行 git diff -- internal/appui/transport_projector.go");
    expect(container.textContent).not.toContain("Shell");
    expect(container.textContent).not.toContain("$ git diff -- internal/appui/transport_projector.go");
    expect(container.textContent).not.toContain("✓ 成功");
    expect(container.querySelector('[data-testid="aiops-terminal-card-cmd-native-card"]')).toBeNull();
    expect(container.textContent).not.toContain("diff --git");

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-native-card"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("Shell");
    expect(container.textContent).toContain("✓ 成功");
    expect(container.textContent).toContain("$ git diff -- internal/appui/transport_projector.go");
    expect(container.querySelector('[data-testid="aiops-command-output-cmd-native-card"]')?.className).toContain("bg-slate-100");
    expect(container.textContent).toContain("diff --git");
  });

  it("does not offer automatic ops manual generation after reusable completed operation evidence", async () => {
    const process = [
      makeBlock({
        id: "cmd-manual-cta-1",
        kind: "command",
        status: "completed",
        command: "docker ps --filter name=redis",
        outputPreview: "aiops-redis",
      }),
      makeBlock({
        id: "cmd-manual-cta-2",
        kind: "command",
        status: "completed",
        command: "docker exec aiops-redis redis-cli INFO memory",
        outputPreview: "used_memory_rss:123456",
      }),
    ];
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={process}
          turnStatus="completed"
          finalText="本次验证状态：已验证，结论基于当前主机与 Redis 容器实时只读结果；未执行任何变更操作。"
        />,
      );
    });

    expect(container.querySelector('[data-testid="aiops-generate-ops-manual-from-chat"]')).toBeNull();
    expect(container.textContent).not.toContain("本次对话可沉淀为运维手册");
  });

  it("keeps long terminal output inside a bounded scroll area", async () => {
    const process = [
      makeBlock({
        id: "cmd-long-output",
        kind: "command",
        status: "completed",
        command: "ps -arc -o rss,pid,comm",
        text: "ps -arc -o rss,pid,comm",
        outputPreview: Array.from({ length: 40 }, (_, index) => `${index + 1} 12345 process-${index + 1}`).join("\n"),
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-long-output"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const terminalCard = container.querySelector('[data-testid="aiops-terminal-card-cmd-long-output"]');
    const terminalOutput = container.querySelector('[data-testid="aiops-command-output-cmd-long-output"]');
    expect(terminalCard?.className).toContain("max-h-72");
    expect(terminalCard?.className).toContain("overflow-hidden");
    expect(terminalOutput?.className).toContain("min-h-0");
    expect(terminalOutput?.className).toContain("max-h-[12rem]");
    expect(terminalOutput?.className).toContain("flex-1");
    expect(terminalOutput?.className).toContain("overflow-y-auto");
    expect(container.textContent).toContain("40 12345 process-40");
  });

  it("shows only command stdout or failed stderr from structured output previews", async () => {
    const process = [
      makeBlock({
        id: "cmd-json-stdout",
        kind: "command",
        status: "completed",
        command: "hostname",
        hostId: "remote-120-77-239-90",
        text: "hostname",
        outputPreview: JSON.stringify({
          command: "hostname",
          status: "ok",
          stdout: "host-a\n",
          stderr: "",
          exitCode: 0,
          tool: "exec_command",
        }),
      }),
      makeBlock({
        id: "cmd-json-stderr",
        kind: "command",
        status: "failed",
        command: "uptime",
        text: "uptime",
        outputPreview: JSON.stringify({
          command: "uptime",
          status: "error",
          stdout: "partial output\n",
          stderr: "uptime: command not found\n",
          exitCode: 127,
          tool: "exec_command",
        }),
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-json-stdout"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      container.querySelector('[data-testid="aiops-command-row-cmd-json-stderr"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const stdout = container.querySelector('[data-testid="aiops-command-output-cmd-json-stdout"]');
    const stderr = container.querySelector('[data-testid="aiops-command-output-cmd-json-stderr"]');
    expect(container.querySelector('[data-testid="aiops-command-host-cmd-json-stdout"]')?.textContent).toBe(
      "120.77.239.90",
    );
    expect(container.querySelector('[data-testid="aiops-command-row-cmd-json-stdout"]')?.getAttribute("aria-label")).toBe(
      "终端命令：在 120.77.239.90 执行 hostname",
    );
    expect(stdout?.textContent).toBe("host-a");
    expect(stderr?.textContent).toBe("uptime: command not found");
    expect(container.textContent).not.toContain('"stdout"');
    expect(container.textContent).not.toContain('"tool"');
    expect(container.textContent).not.toContain("partial output");
  });

  it("does not render output summary as terminal output without an output preview", async () => {
    const process = [
      makeBlock({
        id: "cmd-summary-only",
        kind: "command",
        status: "completed",
        command: "ps -axo pid,ppid,%mem,rss,state,comm",
        text: "ps -axo pid,ppid,%mem,rss,state,comm",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-summary-only"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("$ ps -axo pid,ppid,%mem,rss,state,comm");
    expect(container.querySelector('[data-testid="aiops-command-output-cmd-summary-only"]')).toBeNull();
  });

  it("preserves process order across reasoning, command, reasoning, and search blocks", async () => {
    const process = [
      makeBlock({ id: "reason-1", kind: "reasoning", text: "先确认目标" }),
      makeBlock({
        id: "cmd-order",
        kind: "command",
        status: "completed",
        command: "uptime",
        outputPreview: "up 22 days",
      }),
      makeBlock({ id: "reason-2", kind: "reasoning", text: "命令后概述" }),
      makeBlock({
        id: "search-order",
        kind: "tool",
        status: "running",
        displayKind: "web_search",
        inputSummary: "BTC 行情",
        queries: ["BTC 行情"],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    const text = container.textContent || "";
    const first = text.indexOf("先确认目标");
    const command = text.indexOf("已运行 uptime");
    const second = text.indexOf("命令后概述");
    const search = text.indexOf("正在搜索网页：BTC 行情");
    expect(first).toBeGreaterThanOrEqual(0);
    expect(command).toBeGreaterThan(first);
    expect(second).toBeGreaterThan(command);
    expect(search).toBeGreaterThan(second);
  });

  it("keeps externalized tool evidence internal in normal chat", async () => {
    const process = [
      makeBlock({
        id: "tool-spilled",
        kind: "tool",
        status: "completed",
        displayKind: "logs_query",
        text: "logs_query",
        outputPreview: "summary only",
        materializationTier: "large",
        externalReferences: [
          {
            id: "spill-1",
            kind: "blob",
            title: "nginx raw logs",
            summary: "raw log summary",
          },
        ],
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });

    const header = container.querySelector('[data-testid="aiops-process-header"]');
    expect(header?.getAttribute("aria-expanded")).toBe("false");

    await expandProcessTranscript();
    await act(async () => {
      container.querySelector('[data-testid="aiops-tool-row-tool-spilled"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(container.textContent).toContain("logs_query");
    expect(container.textContent).toContain("summary only");
    expect(container.textContent).not.toContain("结果较大，仅显示摘要。");
    expect(container.textContent).not.toContain("已外溢");
    expect(container.textContent).not.toContain("原始证据");
    expect(container.textContent).not.toContain("查看原始证据");
    expect(container.textContent).not.toContain("nginx raw logs");
    expect(container.textContent).not.toContain("raw log summary");
  });

  it("keeps assistant narration in order around grouped commands and grouped searches", async () => {
    const process = [
      makeBlock({
        id: "assistant-next",
        kind: "assistant",
        status: "completed",
        text: "接下来我要检查运行环境和最近任务状态。",
      }),
      makeBlock({
        id: "cmd-order-1",
        kind: "command",
        foldGroupId: "commands-order",
        foldGroupKind: "command",
        status: "completed",
        command: "pwd",
        outputPreview: "/Users/lizhongxuan/Desktop/aiops-v2",
      }),
      makeBlock({
        id: "cmd-order-2",
        kind: "command",
        foldGroupId: "commands-order",
        foldGroupKind: "command",
        status: "completed",
        command: "git status --short",
        outputPreview: "",
      }),
      makeBlock({
        id: "assistant-after-commands",
        kind: "assistant",
        status: "completed",
        text: "命令结果已经拿到，我会继续核对相关页面信息。",
      }),
      makeBlock({
        id: "search-order-1",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        foldGroupId: "lookup-order",
        foldGroupKind: "web_lookup",
        inputSummary: "aiops-v2 AssistantTransport 顺序",
        queries: ["aiops-v2 AssistantTransport 顺序"],
      }),
      makeBlock({
        id: "search-order-2",
        kind: "tool",
        status: "completed",
        displayKind: "browse_url",
        foldGroupId: "lookup-order",
        foldGroupKind: "web_lookup",
        inputSummary: "https://example.com/aiops-v2-order",
      }),
      makeBlock({
        id: "assistant-after-search",
        kind: "assistant",
        status: "completed",
        text: "页面也确认过了，最终回答会基于上面的命令和搜索结果。",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();

    expect(container.querySelector('[data-testid="aiops-assistant-progress-text"]')?.className).not.toContain("px-1");
    const mergedCommandIcon = container.querySelector('[data-testid="aiops-merged-command-icon"]');
    const searchIcon = container.querySelector('[data-testid="aiops-search-icon"]');
    expect(mergedCommandIcon).toBeTruthy();
    expect(mergedCommandIcon?.parentElement?.firstElementChild).toBe(mergedCommandIcon);
    expect(searchIcon).toBeTruthy();
    expect(searchIcon?.parentElement?.firstElementChild).toBe(searchIcon);
    await act(async () => {
      container.querySelector('[data-testid="aiops-merged-command-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    const bodyText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    const next = bodyText.indexOf("接下来我要检查运行环境和最近任务状态。");
    const commandSummary = bodyText.indexOf("已运行 2 条命令");
    const firstCommand = bodyText.indexOf("已运行 pwd");
    const secondCommand = bodyText.indexOf("已运行 git status --short");
    const afterCommands = bodyText.indexOf("命令结果已经拿到，我会继续核对相关页面信息。");
    const searchSummary = bodyText.indexOf("网页搜索 2 次 · 找到 1 个来源");
    const afterSearch = bodyText.indexOf("页面也确认过了，最终回答会基于上面的命令和搜索结果。");

    expect(next).toBeGreaterThanOrEqual(0);
    expect(commandSummary).toBeGreaterThan(next);
    expect(firstCommand).toBeGreaterThan(commandSummary);
    expect(secondCommand).toBeGreaterThan(firstCommand);
    expect(afterCommands).toBeGreaterThan(secondCommand);
    expect(searchSummary).toBeGreaterThan(afterCommands);
    expect(afterSearch).toBeGreaterThan(searchSummary);

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const expandedBodyText =
      container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    const expandedSearchSummary = expandedBodyText.indexOf("网页搜索 2 次 · 找到 1 个来源");
    const searchedPageRow = expandedBodyText.indexOf("https://example.com/aiops-v2-order");
    const expandedAfterSearch = expandedBodyText.indexOf("页面也确认过了，最终回答会基于上面的命令和搜索结果。");
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("pl-3");
    expect(searchedPageRow).toBeGreaterThan(expandedSearchSummary);
    expect(expandedAfterSearch).toBeGreaterThan(searchedPageRow);

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-detail-row-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(container.querySelector('[data-testid="aiops-search-detail-expanded"]')).toBeNull();
  });

  it("keeps running command terminal output visible", async () => {
    const process = [
      makeBlock({
        id: "cmd-running-output",
        kind: "command",
        status: "running",
        command: "top -l 1",
        text: "top -l 1",
        outputPreview: "Processes: 808 total",
      }),
      makeBlock({
        id: "cmd-completed-output",
        kind: "command",
        status: "completed",
        command: "uptime",
        text: "uptime",
        outputPreview: "up 22 days",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="working" />);
    });

    expect(container.textContent).toContain("top -l 1");
    expect(container.textContent).toContain("Processes: 808 total");
    expect(container.textContent).toContain("uptime");
    expect(container.textContent).not.toContain("up 22 days");
    expect(container.querySelector('[data-testid="aiops-command-status-cmd-running-output"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-terminal-card-cmd-running-output"]')?.textContent).not.toContain("正在运行");
  });

  it("collapses terminal output when a running command completes", async () => {
    const runningProcess = [
      makeBlock({
        id: "cmd-transition-output",
        kind: "command",
        status: "running",
        command: "hostname",
        text: "hostname",
        outputPreview: "server-local",
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={runningProcess} turnStatus="working" />);
    });

    expect(container.textContent).toContain("server-local");

    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            {
              ...runningProcess[0],
              status: "completed",
            },
          ]}
          turnStatus="completed"
        />,
      );
    });
    await expandProcessTranscript();
    expect(container.textContent).toContain("hostname");
    expect(container.textContent).not.toContain("server-local");
  });

  it("truncates long structured tool output instead of replacing it with a fixed message", async () => {
    const longJson = `{"chartReports":[{"name":"Instances","widgets":[{"chart":{"ctx":{"from":1779194700000,"step":30000},"series":[{"name":"${"aiops-host-agent".repeat(18)}","data":[${Array(60).fill(2).join(",")}]}]}}]}]}`;
    const process = [
      makeBlock({
        id: "tool-long-json",
        kind: "tool",
        status: "completed",
        text: "Coroot chartReports",
        outputPreview: longJson,
      }),
    ];

    await act(async () => {
      root.render(<ProcessTranscript process={process} turnStatus="completed" />);
    });
    await expandProcessTranscript();
    await act(async () => {
      container.querySelector('[data-testid="aiops-tool-row-tool-long-json"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const output = container.querySelector('[data-testid="aiops-tool-output-tool-long-json"]');
    expect(output?.textContent).toContain("chartReports");
    expect(output?.textContent).toContain("aiops-host-agent");
    expect(output?.textContent).toContain("...");
    expect(output?.textContent).not.toBe("结果较大，仅显示摘要。");
    expect((output?.textContent || "").length).toBeLessThanOrEqual(720);
    expect(output?.className).toContain("max-w-full");
    expect(output?.className).toContain("overflow-hidden");
    expect(output?.className).toContain("break-words");
  });
});

describe("groupConsecutiveBlocks", () => {
  it("returns empty array for empty input", () => {
    expect(groupConsecutiveBlocks([])).toEqual([]);
  });

  it("uses typed fold identity instead of visible text to group tool blocks", () => {
    const groups = groupConsecutiveBlocks([
      makeBlock({
        id: "tool-a",
        kind: "tool",
        foldGroupId: "action-a",
        foldGroupKind: "tool",
        toolCallId: "call-a",
        text: "相同的可见说明",
      }),
      makeBlock({
        id: "tool-b-1",
        kind: "tool",
        foldGroupId: "action-b",
        foldGroupKind: "tool",
        toolCallId: "call-b-1",
        text: "相同的可见说明",
      }),
      makeBlock({
        id: "tool-b-2",
        kind: "mcp",
        foldGroupId: "action-b",
        foldGroupKind: "tool",
        toolCallId: "call-b-2",
        text: "不同的可见说明",
      }),
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0]).toMatchObject({ kind: "single", block: { id: "tool-a" } });
    expect(groups[1]).toMatchObject({
      kind: "merged",
      mergedKind: "tool",
      blocks: [{ id: "tool-b-1" }, { id: "tool-b-2" }],
    });
  });

  it("groups runtime tool intent commentary with every explicitly linked tool", () => {
    const groups = groupConsecutiveBlocks([
      makeBlock({
        id: "commentary-action",
        kind: "assistant",
        phase: "commentary",
        commentarySource: "runtime_tool_intent",
        toolCallIds: ["call-file", "call-mcp"],
        foldGroupId: "action-1",
        foldGroupKind: "tool",
        text: "采集两类只读证据。",
      }),
      makeBlock({
        id: "file-action",
        kind: "file",
        toolCallId: "call-file",
        foldGroupId: "action-1",
        foldGroupKind: "tool",
        text: "读取配置",
      }),
      makeBlock({
        id: "mcp-action",
        kind: "mcp",
        toolCallId: "call-mcp",
        foldGroupId: "action-1",
        foldGroupKind: "tool",
        text: "读取 MCP 资源",
      }),
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      kind: "merged",
      mergedKind: "tool",
      blocks: [{ id: "commentary-action" }, { id: "file-action" }, { id: "mcp-action" }],
    });
  });

  it("does not merge adjacent same-kind tools when their typed fold ids differ", () => {
    const groups = groupConsecutiveBlocks([
      makeBlock({ id: "search-a", kind: "tool", foldGroupId: "lookup-a", foldGroupKind: "web_lookup" }),
      makeBlock({ id: "search-b", kind: "tool", foldGroupId: "lookup-b", foldGroupKind: "web_lookup" }),
    ]);

    expect(groups).toHaveLength(2);
    expect(groups.every((group) => group.kind === "single")).toBe(true);
  });

  it("keeps a single reasoning block as a single group", () => {
    const blocks = [makeBlock({ id: "r1", kind: "reasoning", text: "thinking" })];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("single");
  });

  it("keeps a single tool block as a single group (no merge for count=1)", () => {
    const blocks = [makeBlock({ id: "f1", kind: "file", text: "read file.ts" })];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("single");
  });

  it("does not merge file blocks into tool summary groups", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "f3", kind: "file", text: "read c.ts" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(3);
    expect(groups.every((group) => group.kind === "single")).toBe(true);
  });

  it("does NOT merge across reasoning blocks", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "r1", kind: "reasoning", text: "let me think" }),
      makeBlock({ id: "f3", kind: "file", text: "read c.ts" }),
      makeBlock({ id: "f4", kind: "file", text: "read d.ts" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(5);
    expect(groups.every((group) => group.kind === "single")).toBe(true);
  });

  it("does not merge adjacent mixed tool kinds into one process group", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "c1", kind: "command", foldGroupId: "commands", foldGroupKind: "command", text: "npm test" }),
      makeBlock({ id: "c2", kind: "command", foldGroupId: "commands", foldGroupKind: "command", text: "npm build" }),
      makeBlock({ id: "c3", kind: "command", foldGroupId: "commands", foldGroupKind: "command", text: "npm lint" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(3);
    expect(groups[0].kind).toBe("single");
    expect(groups[1].kind).toBe("single");
    expect(groups[2].kind).toBe("merged");
    if (groups[2].kind === "merged") {
      expect(groups[2].mergedKind).toBe("command");
      expect(groups[2].blocks).toHaveLength(3);
    }
  });

  it("does not merge command groups across assistant commentary", () => {
    const groups = groupConsecutiveBlocks([
      makeBlock({ id: "cmd-1", kind: "command", foldGroupKind: "command", command: "uptime" }),
      makeBlock({
        id: "assistant-between-commands",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        text: "第一条命令结果已拿到，我会继续执行下一条只读命令。",
      }),
      makeBlock({ id: "cmd-2", kind: "command", foldGroupKind: "command", command: "top -l 1" }),
    ]);

    expect(groups).toHaveLength(3);
    expect(groups[0]).toMatchObject({ kind: "single" });
    expect(groups[1]).toMatchObject({ kind: "single" });
    expect(groups[2]).toMatchObject({ kind: "single" });
  });

  it("merges only consecutive search blocks and keeps separated searches apart", () => {
    const blocks = [
      makeBlock({ id: "s1", kind: "tool", displayKind: "web_search", foldGroupId: "lookup-a", foldGroupKind: "web_lookup", text: "search a" }),
      makeBlock({ id: "s2", kind: "tool", displayKind: "web_search", foldGroupId: "lookup-a", foldGroupKind: "web_lookup", text: "search b" }),
      makeBlock({ id: "r1", kind: "reasoning", text: "middle" }),
      makeBlock({ id: "s3", kind: "tool", displayKind: "web_search", text: "search c" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(3);
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].mergedKind).toBe("search");
      expect(groups[0].blocks).toHaveLength(2);
    }
    expect(groups[1].kind).toBe("single");
    expect(groups[2].kind).toBe("single");
  });

  it("does not classify skill discovery tools as web search blocks", () => {
    const blocks = [
      makeBlock({ id: "skill-search", kind: "tool", displayKind: "skill_search", text: "skill_search mode=search" }),
      makeBlock({ id: "skill-read", kind: "tool", displayKind: "skill_read", text: "skill_read skill=synthetic.triage" }),
    ];

    const groups = groupConsecutiveBlocks(blocks);

    expect(groups).toHaveLength(2);
    expect(groups.every((group) => group.kind === "single")).toBe(true);
  });

  it("uses explicit backend foldGroupKind without merging unrelated MCP blocks", () => {
    const blocks = [
      makeBlock({ id: "s1", kind: "tool", displayKind: "web_search", foldGroupId: "lookup-a", foldGroupKind: "web_lookup", text: "search a" }),
      makeBlock({ id: "s2", kind: "tool", displayKind: "browse_url", foldGroupId: "lookup-a", foldGroupKind: "web_lookup", text: "open a" }),
      makeBlock({ id: "mcp", kind: "mcp", displayKind: "read_mcp_resource", text: "read resource" }),
      makeBlock({ id: "c1", kind: "command", foldGroupId: "commands-a", foldGroupKind: "command", text: "pwd" }),
      makeBlock({ id: "c2", kind: "command", foldGroupId: "commands-a", foldGroupKind: "command", text: "uptime" }),
    ];

    const groups = groupConsecutiveBlocks(blocks);

    expect(groups).toHaveLength(3);
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].mergedKind).toBe("search");
      expect(groups[0].blocks.map((block) => block.id)).toEqual(["s1", "s2"]);
    }
    expect(groups[1].kind).toBe("single");
    if (groups[1].kind === "single") {
      expect(groups[1].block.kind).toBe("mcp");
    }
    expect(groups[2].kind).toBe("merged");
    if (groups[2].kind === "merged") {
      expect(groups[2].mergedKind).toBe("command");
      expect(groups[2].blocks.map((block) => block.id)).toEqual(["c1", "c2"]);
    }
  });

  it("handles mixed reasoning and tool blocks correctly", () => {
    const blocks = [
      makeBlock({ id: "r1", kind: "reasoning", text: "start" }),
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "r2", kind: "reasoning", text: "middle" }),
      makeBlock({ id: "t1", kind: "tool", text: "call api" }),
      makeBlock({ id: "t2", kind: "tool", text: "call api2" }),
      makeBlock({ id: "t3", kind: "tool", text: "call api3" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(6);
    expect(groups[0].kind).toBe("single"); // reasoning
    expect(groups[1].kind).toBe("single"); // single file (only 1)
    expect(groups[2].kind).toBe("single"); // reasoning
    expect(groups[3].kind).toBe("single"); // tool
    expect(groups[4].kind).toBe("single"); // tool
    expect(groups[5].kind).toBe("single"); // tool
  });
});

describe("MergedToolSummary", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("replaces familiar command tool names with terminal icons while preserving custom tool names", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "command-action-title",
              kind: "assistant",
              phase: "commentary",
              commentarySource: "model_prelude",
              toolCallIds: ["call-uptime", "call-disk", "call-legacy", "call-custom"],
              foldGroupId: "command-action",
              foldGroupKind: "tool",
              text: "分别执行两个只读命令，再调用业务探针。",
            }),
            makeBlock({
              id: "tool-uptime",
              kind: "tool",
              displayKind: "terminal",
              source: "exec_command",
              inputSummary: "uptime",
              text: "exec_command uptime",
              toolCallId: "call-uptime",
              foldGroupId: "command-action",
              foldGroupKind: "tool",
              outputPreview: JSON.stringify({
                schemaVersion: "aiops.terminal/v1",
                tool: "exec_command",
                hostId: "remote-120-77-239-90",
                command: "uptime",
                status: "ok",
                exitCode: 0,
                stdout: "up 246 days\n",
                stderr: "",
              }),
            }),
            makeBlock({
              id: "tool-disk",
              kind: "tool",
              displayKind: "terminal.command",
              source: "exec_command",
              inputSummary: "df -h /",
              text: "exec_command df -h /",
              toolCallId: "call-disk",
              foldGroupId: "command-action",
              foldGroupKind: "tool",
              outputPreview: JSON.stringify({
                schemaVersion: "aiops.terminal/v1",
                tool: "exec_command",
                hostId: "remote-120-77-239-90",
                command: "df -h /",
                status: "ok",
                exitCode: 0,
                stdout: "Filesystem Size Used Avail Use% Mounted on\n/dev/vda3 40G 31G 6.4G 83% /\n",
                stderr: "",
              }),
            }),
            makeBlock({
              id: "tool-legacy",
              kind: "tool",
              displayKind: "exec_command",
              text: "exec_command hostname",
              toolCallId: "call-legacy",
              foldGroupId: "command-action",
              foldGroupKind: "tool",
            }),
            makeBlock({
              id: "tool-custom",
              kind: "tool",
              displayKind: "business.probe",
              source: "exec_command_runner",
              inputSummary: "checkout",
              text: "exec_command_runner checkout",
              toolCallId: "call-custom",
              foldGroupId: "command-action",
              foldGroupKind: "tool",
            }),
          ]}
          turnStatus="completed"
        />,
      );
    });

    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    await act(async () => {
      container.querySelector('[data-testid="aiops-merged-tool-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const uptimeRow = container.querySelector('[data-testid="aiops-command-row-tool-uptime"]');
    const diskRow = container.querySelector('[data-testid="aiops-command-row-tool-disk"]');
    const legacyRow = container.querySelector('[data-testid="aiops-command-row-tool-legacy"]');
    const customRow = container.querySelector('[data-testid="aiops-tool-row-tool-custom"]');
    expect(uptimeRow?.textContent).toContain("uptime");
    expect(diskRow?.textContent).toContain("df -h /");
    expect(uptimeRow?.textContent).not.toContain("exec_command");
    expect(diskRow?.textContent).not.toContain("exec_command");
    expect(legacyRow?.textContent).toContain("hostname");
    expect(legacyRow?.textContent).not.toContain("exec_command");
    expect(uptimeRow?.getAttribute("aria-label")).toBe("终端命令：在 120.77.239.90 执行 uptime，已完成");
    expect(diskRow?.getAttribute("aria-label")).toBe("终端命令：在 120.77.239.90 执行 df -h /，已完成");
    expect(container.querySelector('[data-testid="aiops-command-icon-tool-uptime"]')?.getAttribute("aria-hidden")).toBe("true");
    expect(container.querySelector('[data-testid="aiops-command-icon-tool-disk"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-command-icon-tool-legacy"]')).toBeTruthy();
    const uptimeHost = container.querySelector('[data-testid="aiops-command-host-tool-uptime"]');
    const uptimeCommand = container.querySelector('[data-testid="aiops-command-text-tool-uptime"]');
    expect(uptimeHost?.textContent).toBe("120.77.239.90");
    expect(uptimeCommand?.textContent).toBe("uptime");
    expect(
      ((uptimeHost?.compareDocumentPosition(uptimeCommand as Node) || 0) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
    ).toBe(true);
    expect(customRow?.textContent).toContain("exec_command_runner checkout");
    expect(container.querySelector('[data-testid="aiops-tool-icon-tool-custom"]')).toBeNull();

    await act(async () => {
      uptimeRow?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      diskRow?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.querySelector('[data-testid="aiops-command-output-tool-uptime"]')?.textContent).toBe("up 246 days");
    expect(container.querySelector('[data-testid="aiops-command-output-tool-disk"]')?.textContent).toBe(
      "Filesystem Size Used Avail Use% Mounted on\n/dev/vda3 40G 31G 6.4G 83% /",
    );
    expect(container.querySelector('[data-testid="aiops-terminal-card-tool-uptime"]')?.textContent).toContain(
      "Shell · 120.77.239.90",
    );
    expect(container.textContent).not.toContain('"stdout"');
    expect(container.textContent).not.toContain('"schemaVersion"');
  });

  it("shows failed exec tool stderr in a terminal without leaking its JSON envelope", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "tool-failed-command",
              kind: "tool",
              displayKind: "host.command",
              source: "exec_command",
              status: "failed",
              inputSummary: "systemctl status missing.service",
              text: "exec_command systemctl status missing.service",
              outputPreview: JSON.stringify({
                schemaVersion: "aiops.terminal/v1",
                tool: "exec_command",
                hostId: "remote-120-77-239-90",
                command: "systemctl status missing.service",
                status: "error",
                exitCode: 4,
                stdout: "partial output\n",
                stderr: "Unit missing.service could not be found.\n",
              }),
            }),
          ]}
          turnStatus="completed"
        />,
      );
    });
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-tool-failed-command"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.querySelector('[data-testid="aiops-command-host-tool-failed-command"]')?.textContent).toBe(
      "120.77.239.90",
    );
    expect(container.querySelector('[data-testid="aiops-command-output-tool-failed-command"]')?.textContent).toBe(
      "Unit missing.service could not be found.",
    );
    expect(container.textContent).not.toContain("partial output");
    expect(container.textContent).not.toContain('"stderr"');
    expect(container.textContent).not.toContain('"tool":"exec_command"');
  });

  it("does not replace explicit business or file mutation tool names with familiar icons", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "mutation-action-title",
              kind: "assistant",
              phase: "commentary",
              commentarySource: "model_prelude",
              toolCallIds: ["call-business", "call-patch"],
              foldGroupId: "mutation-action",
              foldGroupKind: "tool",
              text: "检查业务探针并更新配置。",
            }),
            makeBlock({
              id: "tool-business",
              kind: "tool",
              displayKind: "business.probe",
              source: "exec_command",
              inputSummary: "checkout",
              text: "exec_command checkout",
              toolCallId: "call-business",
              foldGroupId: "mutation-action",
              foldGroupKind: "tool",
            }),
            makeBlock({
              id: "file-patch",
              kind: "file",
              displayKind: "file.diff",
              source: "apply_patch",
              text: "更新 config.yaml",
              toolCallId: "call-patch",
              foldGroupId: "mutation-action",
              foldGroupKind: "tool",
            }),
          ]}
          turnStatus="completed"
        />,
      );
    });

    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    await act(async () => {
      container.querySelector('[data-testid="aiops-merged-tool-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.querySelector('[data-testid="aiops-tool-row-tool-business"]')?.textContent).toContain(
      "exec_command checkout",
    );
    expect(container.querySelector('[data-testid="aiops-tool-icon-tool-business"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-tool-row-tool-business"]')?.getAttribute("aria-label")).toBeNull();
    expect(container.querySelector('[data-testid="aiops-tool-row-file-patch"]')?.textContent).toContain(
      "更新 config.yaml",
    );
    expect(container.querySelector('[data-testid="aiops-tool-icon-file-patch"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-tool-row-file-patch"]')?.getAttribute("aria-label")).toBeNull();
  });

  it("uses typed model prelude commentary as the action title instead of a tool row", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "model-prelude-action",
              kind: "assistant",
              phase: "commentary",
              commentarySource: "model_prelude",
              toolCallIds: ["call-file"],
              foldGroupId: "action-file",
              foldGroupKind: "tool",
              text: "先读取配置文件，再核对字段。",
            }),
            makeBlock({
              id: "file-action",
              kind: "file",
              displayKind: "file.read",
              toolCallId: "call-file",
              foldGroupId: "action-file",
              foldGroupKind: "tool",
              text: "读取 config.yaml",
            }),
          ]}
          turnStatus="completed"
        />,
      );
    });

    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect((container.textContent?.match(/先读取配置文件，再核对字段。/g) || [])).toHaveLength(1);

    await act(async () => {
      container.querySelector('[data-testid="aiops-merged-tool-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(container.querySelectorAll('[data-testid^="aiops-tool-row-"]')).toHaveLength(1);
    expect(container.textContent).toContain("读取 config.yaml");
    expect(container.querySelector('[data-testid="aiops-tool-icon-file-action"]')).toBeTruthy();
    expect(container.querySelector('[data-testid="aiops-tool-row-file-action"]')?.getAttribute("aria-label")).toBe(
      "文件读取：读取 config.yaml，已完成",
    );
  });

  it("shows merged tool call details by default", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({ id: "tool-coroot-apps", kind: "tool", text: "coroot.applications" }),
            makeBlock({
              id: "tool-coroot-health",
              kind: "tool",
              text: "Summary: warning services",
              source: "coroot_list_services",
              inputSummary: '{"status":"warning"}',
            }),
            makeBlock({ id: "tool-coroot-rca", kind: "tool", text: "coroot.rca" }),
          ]}
          turnStatus="completed"
        />,
      );
    });

    const header = container.querySelector('[data-testid="aiops-process-header"]');
    expect(header).toBeTruthy();
    await act(async () => {
      header?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).not.toContain("已调用 3 个工具");
    expect(container.querySelector('[data-testid="aiops-merged-tool-details"]')).toBeNull();
    expect(container.querySelectorAll('[data-testid^="aiops-tool-row-"]')).toHaveLength(3);
    expect(container.textContent).toContain("coroot.applications");
    expect(container.textContent).toContain("coroot_list_services");
    expect(container.textContent).not.toContain('{"status":"warning"}');
    expect(container.textContent).toContain("coroot.rca");
  });

  it("labels failed web search attempts as retrieval failures, not execution failures", async () => {
    await act(async () => {
      root.render(
        <ProcessTranscript
          process={[
            makeBlock({
              id: "search-failed",
              kind: "tool",
              status: "failed",
              displayKind: "web_search",
              inputSummary: "PostgreSQL timeline divergence",
            }),
            makeBlock({
              id: "mcp-resources",
              kind: "tool",
              status: "completed",
              displayKind: "list_mcp_resources",
              text: "list_mcp_resources",
            }),
          ]}
          turnStatus="working"
        />,
      );
    });

    expect(container.textContent).toContain("检索失败 PostgreSQL timeline divergence");
    expect(container.textContent).not.toContain("执行失败 PostgreSQL timeline divergence");
  });
});

describe("getMergedSummaryText", () => {
  it("returns file summary with count", () => {
    expect(getMergedSummaryText("file", 6)).toBe("已探索 6 个文件");
  });

  it("returns command summary with count", () => {
    expect(getMergedSummaryText("command", 3)).toBe("已运行 3 条命令");
  });

  it("returns tool summary with count", () => {
    expect(getMergedSummaryText("tool", 4)).toBe("已调用 4 个工具");
  });

  it("returns mcp summary with count (same as tool)", () => {
    expect(getMergedSummaryText("mcp", 2)).toBe("已调用 2 个工具");
  });

  it("returns fallback for unknown kind", () => {
    expect(getMergedSummaryText("unknown", 5)).toBe("已处理 5 个操作");
  });
});

describe("stripHtml", () => {
  it("returns empty string for empty input", () => {
    expect(stripHtml("")).toBe("");
  });

  it("returns non-HTML text unchanged", () => {
    expect(stripHtml("hello world")).toBe("hello world");
    expect(stripHtml("some plain text with <no> html detection")).toBe(
      "some plain text with <no> html detection",
    );
  });

  it("strips HTML tags from DOCTYPE content", () => {
    const input = "<!DOCTYPE html><html><body><p>Hello</p></body></html>";
    const result = stripHtml(input);
    expect(result).not.toContain("<");
    expect(result).not.toContain(">");
    expect(result).toContain("Hello");
  });

  it("strips HTML tags from <html> content", () => {
    const input = "<html><head><title>Test</title></head><body><div>Content</div></body></html>";
    const result = stripHtml(input);
    expect(result).not.toContain("<");
    expect(result).not.toContain(">");
    expect(result).toContain("Test");
    expect(result).toContain("Content");
  });

  it("collapses whitespace after stripping", () => {
    const input = "<!DOCTYPE html><html><body>  <p>  spaced   out  </p>  </body></html>";
    const result = stripHtml(input);
    // No consecutive spaces should remain
    expect(result).not.toMatch(/\s{2,}/);
  });

  it("truncates to 200 chars with '…' for long HTML content", () => {
    const longContent = "<!DOCTYPE html><html><body>" + "<p>word </p>".repeat(100) + "</body></html>";
    const result = stripHtml(longContent);
    expect(result.length).toBeLessThanOrEqual(201); // 200 chars + "…"
    expect(result).toMatch(/…$/);
  });

  it("handles leading whitespace before DOCTYPE", () => {
    const input = "   <!DOCTYPE html><html><body><p>Hello</p></body></html>";
    const result = stripHtml(input);
    expect(result).not.toContain("<");
    expect(result).toContain("Hello");
  });
});
