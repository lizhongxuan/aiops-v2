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

  it("renders streaming final answer markdown immediately and keeps completed markup stable", async () => {
    const markdown = "检查结果如下：\n\n1. **Nginx 正常**\n2. **CPU 负载稳定**";

    await act(async () => {
      root.render(<ProcessTranscript process={[]} turnStatus="working" finalText={markdown} />);
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeTruthy();
    expect(container.textContent).toContain("处理中");
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
    expect(container.querySelectorAll("ol li")).toHaveLength(2);
    expect(container.querySelector("strong")?.textContent).toBe("Nginx 正常");
    expect(container.textContent).not.toContain("**Nginx 正常**");
    const streamingFinalMarkup = container.querySelector('[data-testid="aiops-final-text"]')?.innerHTML;

    await act(async () => {
      root.render(<ProcessTranscript process={[]} turnStatus="completed" finalText={markdown} />);
    });

    expect(container.querySelector('[data-testid="aiops-process-header"]')).toBeNull();
    expect(container.querySelector('[data-testid="aiops-final-text"]')?.innerHTML).toBe(streamingFinalMarkup);
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

  it("keeps the running process header visible when assistant text streams before tool blocks", async () => {
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
    expect(container.textContent).toContain("我先复查主机当前的 CPU、内存、磁盘和负载情况");
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
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

    expect(container.textContent).toContain("正在搜索网页（BTC 行情）");
    expect(container.textContent).toContain("BTC 行情");
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
        inputSummary: "BTC price today USD",
        outputPreview:
          'failed: Post "http://127.0.0.1:8317/v1/responses": context deadline exceeded (Client.Timeout exceeded while awaiting headers)',
      }),
      makeBlock({
        id: "search-4",
        kind: "tool",
        status: "completed",
        displayKind: "browse_url",
        inputSummary: "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd",
        outputPreview:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978}}","url":"https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"}',
      }),
      makeBlock({
        id: "search-5",
        kind: "tool",
        status: "completed",
        displayKind: "web_search",
        inputSummary: "Searching the web",
        outputPreview: '{"contentType":"application/json; charset=utf-8","text":"raw response body"}',
      }),
      makeBlock({
        id: "search-6",
        kind: "tool",
        status: "completed",
        displayKind: "browser.open",
        inputSummary:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978}}","url":"https://api.coingecko.com/api/v3/simple/price?ids=bitcoin\\u0026vs_currencies=usd"}',
        outputPreview:
          '{"contentType":"application/json; charset=utf-8","text":"{\\"bitcoin\\":{\\"usd\\":80978',
      }),
      makeBlock({
        id: "search-7",
        kind: "search",
        status: "completed",
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
    expect(container.textContent).toContain("网页检索 3 项");
    const searchButton = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("网页检索"),
    );
    expect(searchButton).toBeTruthy();

    await act(async () => {
      searchButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const text = container.textContent || "";
    expect(text).toContain("BTC price today USD");
    expect(text).toContain("https://api.coingecko.com/api/v3/simple/price");
    expect(text).toContain("https://www.coingecko.com/en/coins/bitcoin");
    expect(text).not.toContain("Bitcoin Price: BTC/USD Live Price Chart");
    expect(text).not.toContain("127.0.0.1:8317");
    expect(text).not.toContain("/v1/responses");
    expect(text).not.toContain("contentType");
    expect(text).not.toContain('"bitcoin"');
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
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("text-[14px]");
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("pl-3");
    const searchToggle = container.querySelector('[data-testid="aiops-search-toggle"]');
    const searchIcon = container.querySelector('[data-testid="aiops-search-icon"]');
    expect(searchIcon).toBeTruthy();
    expect(searchToggle?.firstElementChild).toBe(searchIcon);
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

    const detail = container.querySelector('[data-testid="aiops-search-details"]')?.children.item(1);
    expect(detail?.textContent).toBe(url);
    expect(detail?.className).toContain("break-all");
    expect(detail?.className).not.toContain("truncate");
  });

  it("keeps streaming assistant prelude before the following search block", async () => {
    const prelude = "我将先通过实时网页搜索核实BTC当前行情与主要价格来源，然后返回简洁结果并附上来源。";
    const process = [
      makeBlock({
        id: "assistant-prelude",
        kind: "assistant",
        status: "running",
        displayKind: "assistant.final",
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
    const searchIndex = bodyText.indexOf("正在搜索网页（BTC current price USD 24h change）");
    expect(preludeIndex).toBeGreaterThanOrEqual(0);
    expect(searchIndex).toBeGreaterThan(preludeIndex);
    expect(container.querySelectorAll('[data-testid="aiops-final-text"]')).toHaveLength(1);
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
    expect(chevron?.getAttribute("class")).toContain("rotate-0");

    await act(async () => {
      container.querySelector('[data-testid="aiops-search-toggle"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(chevron?.getAttribute("class")).toContain("-rotate-90");
  });

  it("shows command details for merged command groups", async () => {
    const process = [
      makeBlock({
        id: "cmd-1",
        kind: "command",
        command: "kubectl get pods -n prod",
        text: "kubectl get pods",
      }),
      makeBlock({
        id: "cmd-2",
        kind: "command",
        status: "blocked",
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

  it("shows mock and evidence refs on tool and command rows", async () => {
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
    expect(container.textContent).toContain("证据");
    expect(container.textContent).toContain("evidence:redis:rss");
    expect(container.textContent).toContain("evidence:redis:events");
    expect(container.textContent).toContain("evidence:k8s:events");
  });

  it("lets expanded command rows in merged groups grow without clipping sibling rows", async () => {
    const process = [
      makeBlock({
        id: "cmd-grow-1",
        kind: "command",
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
        status: "completed",
        command: "pwd",
        outputPreview: "/tmp",
      }),
      makeBlock({
        id: "cmd-chevron-2",
        kind: "command",
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
        text: "本机当前运行正常。",
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
    expect(text.indexOf("本机当前运行正常。")).toBeGreaterThan(text.indexOf("我先检查本机状态。"));
    expect(text.indexOf("已运行 uptime")).toBeGreaterThan(text.indexOf("本机当前运行正常。"));
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

  it("aggregates adjacent mixed tool actions without crossing assistant text", async () => {
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
    const mixed = text.indexOf("已探索 1 个文件,1 次搜索,已运行 1 条命令");
    const reasoning = text.indexOf("拿到第一批证据。");
    const laterFile = text.indexOf("Read package.json");
    expect(mixed).toBeGreaterThanOrEqual(0);
    expect(container.querySelector('[data-testid="aiops-merged-mixed-icon"]')).toBeTruthy();
    expect(reasoning).toBeGreaterThan(mixed);
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
    expect(terminalOutput?.className).toContain("max-h-48");
    expect(terminalOutput?.className).toContain("flex-1");
    expect(terminalOutput?.className).toContain("overflow-y-auto");
    expect(container.textContent).toContain("40 12345 process-40");
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
    const search = text.indexOf("正在搜索网页（BTC 行情）");
    expect(first).toBeGreaterThanOrEqual(0);
    expect(command).toBeGreaterThan(first);
    expect(second).toBeGreaterThan(command);
    expect(search).toBeGreaterThan(second);
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
        status: "completed",
        command: "pwd",
        outputPreview: "/Users/lizhongxuan/Desktop/aiops-v2",
      }),
      makeBlock({
        id: "cmd-order-2",
        kind: "command",
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
        inputSummary: "aiops-v2 AssistantTransport 顺序",
        queries: ["aiops-v2 AssistantTransport 顺序"],
      }),
      makeBlock({
        id: "search-order-2",
        kind: "tool",
        status: "completed",
        displayKind: "browse_url",
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

    expect(container.querySelector('[data-testid="aiops-final-text"]')?.className).not.toContain("px-1");
    const mergedCommandIcon = container.querySelector('[data-testid="aiops-merged-command-icon"]');
    const searchIcon = container.querySelector('[data-testid="aiops-search-icon"]');
    expect(mergedCommandIcon).toBeTruthy();
    expect(mergedCommandIcon?.parentElement?.firstElementChild).toBe(mergedCommandIcon);
    expect(searchIcon).toBeTruthy();
    expect(searchIcon?.parentElement?.firstElementChild).toBe(searchIcon);
    const bodyText = container.querySelector('[data-testid="aiops-process-transcript-body"]')?.textContent || "";
    const next = bodyText.indexOf("接下来我要检查运行环境和最近任务状态。");
    const commandSummary = bodyText.indexOf("已运行 2 条命令");
    const firstCommand = bodyText.indexOf("已运行 pwd");
    const secondCommand = bodyText.indexOf("已运行 git status --short");
    const afterCommands = bodyText.indexOf("命令结果已经拿到，我会继续核对相关页面信息。");
    const searchSummary = bodyText.indexOf("网页检索 2 项");
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
    const expandedSearchSummary = expandedBodyText.indexOf("网页检索 2 项");
    const searchQuery = expandedBodyText.indexOf("aiops-v2 AssistantTransport 顺序");
    const searchedPage = expandedBodyText.indexOf("https://example.com/aiops-v2-order");
    const expandedAfterSearch = expandedBodyText.indexOf("页面也确认过了，最终回答会基于上面的命令和搜索结果。");
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("pl-3");
    expect(searchQuery).toBeGreaterThan(expandedSearchSummary);
    expect(searchedPage).toBeGreaterThan(searchQuery);
    expect(expandedAfterSearch).toBeGreaterThan(searchedPage);
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

  it("keeps long tool output inside the transcript container", async () => {
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
    expect(output?.className).toContain("max-w-full");
    expect(output?.className).toContain("overflow-hidden");
    expect(output?.className).toContain("break-words");
  });
});

describe("groupConsecutiveBlocks", () => {
  it("returns empty array for empty input", () => {
    expect(groupConsecutiveBlocks([])).toEqual([]);
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

  it("merges 3 consecutive file blocks into one merged group", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "f3", kind: "file", text: "read c.ts" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].blocks).toHaveLength(3);
      expect(groups[0].mergedKind).toBe("file");
    }
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
    expect(groups).toHaveLength(3);
    // First group: merged file blocks
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].blocks).toHaveLength(2);
    }
    // Second group: reasoning (single)
    expect(groups[1].kind).toBe("single");
    if (groups[1].kind === "single") {
      expect(groups[1].block.kind).toBe("reasoning");
    }
    // Third group: merged file blocks
    expect(groups[2].kind).toBe("merged");
    if (groups[2].kind === "merged") {
      expect(groups[2].blocks).toHaveLength(2);
    }
  });

  it("merges adjacent mixed tool kinds into one process group", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "c1", kind: "command", text: "npm test" }),
      makeBlock({ id: "c2", kind: "command", text: "npm build" }),
      makeBlock({ id: "c3", kind: "command", text: "npm lint" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].mergedKind).toBe("mixed");
      expect(groups[0].blocks).toHaveLength(5);
    }
  });

  it("merges only consecutive search blocks and keeps separated searches apart", () => {
    const blocks = [
      makeBlock({ id: "s1", kind: "tool", displayKind: "web_search", text: "search a" }),
      makeBlock({ id: "s2", kind: "tool", displayKind: "web_search", text: "search b" }),
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
    expect(groups).toHaveLength(4);
    expect(groups[0].kind).toBe("single"); // reasoning
    expect(groups[1].kind).toBe("single"); // single file (only 1)
    expect(groups[2].kind).toBe("single"); // reasoning
    expect(groups[3].kind).toBe("merged"); // 3 tool blocks
    if (groups[3].kind === "merged") {
      expect(groups[3].mergedKind).toBe("tool");
      expect(groups[3].blocks).toHaveLength(3);
    }
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
