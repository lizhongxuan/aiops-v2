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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

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

  it("uses one font size for running search labels and details", async () => {
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

    expect(container.querySelector('[data-testid="aiops-search-toggle"]')?.className).toContain("text-[15px]");
    expect(container.querySelector('[data-testid="aiops-search-details"]')?.className).toContain("text-[15px]");
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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    const text = container.textContent || "";
    expect(text).toContain("已运行 2 条命令");
    expect(text).toContain("kubectl get pods -n prod");
    expect(text).toContain("kubectl rollout restart deployment/api -n prod");
    expect(text).toContain("等待审核");
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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

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

  it("uses turn timestamps for completed elapsed time and starts collapsed", async () => {
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
    expect(container.querySelector('[data-testid="aiops-process-transcript-body"]')).toBeNull();
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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("已运行 git diff -- internal/appui/transport_projector.go");
    expect(container.textContent).not.toContain("Shell");
    expect(container.textContent).not.toContain("$ git diff -- internal/appui/transport_projector.go");
    expect(container.textContent).toContain("✓ 成功");
    expect(container.querySelector('[data-testid="aiops-terminal-card-cmd-native-card"]')).toBeNull();
    expect(container.textContent).not.toContain("diff --git");

    await act(async () => {
      container.querySelector('[data-testid="aiops-command-row-cmd-native-card"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("Shell");
    expect(container.textContent).toContain("$ git diff -- internal/appui/transport_projector.go");
    expect(container.querySelector('[data-testid="aiops-command-output-cmd-native-card"]')?.className).toContain("bg-slate-100");
    expect(container.textContent).toContain("diff --git");
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
    await act(async () => {
      container.querySelector('[data-testid="aiops-process-header"]')?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("hostname");
    expect(container.textContent).not.toContain("server-local");
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

  it("does NOT merge different kinds together", () => {
    const blocks = [
      makeBlock({ id: "f1", kind: "file", text: "read a.ts" }),
      makeBlock({ id: "f2", kind: "file", text: "read b.ts" }),
      makeBlock({ id: "c1", kind: "command", text: "npm test" }),
      makeBlock({ id: "c2", kind: "command", text: "npm build" }),
      makeBlock({ id: "c3", kind: "command", text: "npm lint" }),
    ];
    const groups = groupConsecutiveBlocks(blocks);
    expect(groups).toHaveLength(2);
    // First: merged file group
    expect(groups[0].kind).toBe("merged");
    if (groups[0].kind === "merged") {
      expect(groups[0].mergedKind).toBe("file");
      expect(groups[0].blocks).toHaveLength(2);
    }
    // Second: merged command group
    expect(groups[1].kind).toBe("merged");
    if (groups[1].kind === "merged") {
      expect(groups[1].mergedKind).toBe("command");
      expect(groups[1].blocks).toHaveLength(3);
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
    expect(getMergedSummaryText("file", 6)).toBe("📂 已探索 6 个文件");
  });

  it("returns command summary with count", () => {
    expect(getMergedSummaryText("command", 3)).toBe("已运行 3 条命令");
  });

  it("returns tool summary with count", () => {
    expect(getMergedSummaryText("tool", 4)).toBe("⚙️ 已调用 4 个工具");
  });

  it("returns mcp summary with count (same as tool)", () => {
    expect(getMergedSummaryText("mcp", 2)).toBe("⚙️ 已调用 2 个工具");
  });

  it("returns fallback for unknown kind", () => {
    expect(getMergedSummaryText("unknown", 5)).toBe("⚙️ 已处理 5 个操作");
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
