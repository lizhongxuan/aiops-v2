import { describe, expect, it } from "vitest";
import { buildCodexProcessTranscript } from "./codexProcessTranscript";

describe("buildCodexProcessTranscript", () => {
  it("builds a typed transcript without legacy aggregate or internal pipeline labels", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-1",
      active: false,
      status: "completed",
      elapsedLabel: "13s",
      assistantMessages: [
        { id: "intent-1", text: "我先拉取当前主要指数和板块行情信息，给你一个简短概览。" },
        { id: "final-duplicate", text: "截至 2026年4月29日收盘，A 股明显反弹：\n- 上证指数：+0.71%" },
      ],
      finalText: "截至 2026年4月29日收盘，A 股明显反弹：\n- 上证指数：+0.71%",
      processItems: [
        { id: "internal-1", text: "正在准备上下文（第 1 轮）", displayKind: "runtime.prepare_context", status: "completed" },
        { id: "summary-1", text: "已记录 4 条过程细项", status: "completed" },
        {
          id: "search-1",
          kind: "search",
          displayKind: "browser.search",
          status: "completed",
          inputSummary: "2026-04-29 A股 大盘 上证指数 深证成指 创业板指",
          results: [
            { title: "2026年4月29日 上证指数 收盘", url: "https://finance.example.test/a", snippet: "上证指数收盘上涨" },
          ],
        },
        { id: "reason-1", kind: "reasoning", displayKind: "reasoning.summary", text: "我会再取一次实时/收盘报价源，避免只依赖新闻稿。" },
      ],
    });

    expect(transcript.header).toMatchObject({
      kind: "header",
      text: "已处理 13s",
      status: "completed",
    });
    expect(transcript.blocks.map((block) => block.kind)).toEqual([
      "header",
      "assistant-intent",
      "search-step",
      "reasoning-summary",
      "final-answer",
    ]);
    expect(transcript.blocks.filter((block) => block.kind === "final-answer")).toHaveLength(1);
    expect(JSON.stringify(transcript)).not.toContain("已记录");
    expect(JSON.stringify(transcript)).not.toContain("明细已折叠");
    expect(JSON.stringify(transcript)).not.toContain("处理失败");
    expect(JSON.stringify(transcript)).not.toContain("准备上下文");
    expect(JSON.stringify(transcript)).not.toContain("调用模型");
  });

  it("keeps real command, output preview, search query, results, and approval fields structured", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-host",
      active: true,
      status: "blocked",
      elapsedLabel: "9s",
      processItems: [
        {
          id: "cmd-1",
          kind: "command",
          displayKind: "host.command",
          status: "completed",
          command: "df -h",
          text: "exec_command",
          outputPreview: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
        },
        {
          id: "search-1",
          kind: "search",
          displayKind: "browser.search",
          status: "running",
          inputSummary: "BTC price 2026-04-29",
          queries: ["BTC price 2026-04-29"],
          results: [{ title: "BTC Price", url: "https://example.test/btc" }],
        },
      ],
      approval: {
        id: "approval-1",
        type: "command",
        command: "uptime && vm_stat",
        reason: "需要读取本机资源状态。",
        status: "blocked",
      },
    });

    const command = transcript.blocks.find((block) => block.kind === "command-step");
    const search = transcript.blocks.find((block) => block.kind === "search-step");
    const approval = transcript.blocks.find((block) => block.kind === "inline-approval");

    expect(command).toMatchObject({
      text: "已运行 df -h",
      command: "df -h",
      outputPreview: expect.stringContaining("Filesystem"),
    });
    expect(command.text).not.toBe("exec_command");
    expect(search).toMatchObject({
      text: "正在搜索网页",
      inputSummary: "BTC price 2026-04-29",
      queries: ["BTC price 2026-04-29"],
      results: [{ title: "BTC Price", url: "https://example.test/btc" }],
    });
    expect(approval).toMatchObject({
      text: "等待确认",
      command: "uptime && vm_stat",
      reason: "需要读取本机资源状态。",
      displayKind: "approval.command",
    });
    expect(JSON.stringify(transcript)).not.toContain("exec_command");
  });

  it("uses stable block ids and exposes thinking state without rendering a fold placeholder", () => {
    const input = {
      turnId: "turn-stable",
      active: true,
      status: "running",
      elapsedLabel: "20s",
      processItems: [
        {
          id: "search-1",
          kind: "search",
          displayKind: "browser.search",
          status: "completed",
          inputSummary: "A股 大盘",
        },
      ],
      modelRunning: true,
    };

    const first = buildCodexProcessTranscript(input);
    const second = buildCodexProcessTranscript(input);
    const empty = buildCodexProcessTranscript({
      turnId: "turn-empty",
      active: true,
      status: "running",
      elapsedLabel: "1s",
      modelRunning: true,
      processItems: [],
    });

    expect(second.blocks.map((block) => block.id)).toEqual(first.blocks.map((block) => block.id));
    expect(first.showThinking).toBe(true);
    expect(first.blocks.some((block) => block.text === "正在思考")).toBe(false);
    expect(empty.showThinking).toBe(false);
    expect(empty.blocks.map((block) => block.kind)).not.toContain("reasoning-summary");
  });

  it("deduplicates near-identical assistant narration drafts", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-near-duplicate",
      status: "running",
      assistantMessages: [
        {
          id: "draft-1",
          text: "已拿到三路结果，但数值不一致：CoinGecko 给出约 75,410 美元且 24h +1.7%，Binance 片段约 77,450，美区间存在偏差；CoinMarketCap 没拿到今天快照。下一步我直接抓取 CoinGecko 和 Binance 页面文本，优先核实是否是页面地区/合约类型差异，尽量给你一个可靠...",
        },
        {
          id: "draft-2",
          text: "已拿到三路结果，但数值不一致：CoinGecko 给出约75,410 美元且24h +1.7%，Binance片段约77,450，美区间存在偏差；CoinMarketCap 没拿到今天快照。下一步我直接抓取 CoinGecko 和 Binance 页面文本，优先核实是否是页面地区/合约类型差异，尽量给你一个可靠的当前概况。",
        },
      ],
    });

    const narrationBlocks = transcript.blocks.filter((block) => ["assistant-intent", "assistant-result", "reasoning-summary"].includes(block.kind));
    expect(narrationBlocks).toHaveLength(1);
    expect(narrationBlocks[0].text).toContain("可靠的当前概况");
  });

  it("drops command steps when only a raw tool or generic command label is available", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-raw",
      status: "completed",
      processItems: [
        {
          id: "cmd-raw",
          kind: "command",
          displayKind: "host.command",
          status: "completed",
          text: "已运行命令",
        },
        {
          id: "cmd-tool",
          kind: "command",
          displayKind: "host.command",
          status: "completed",
          text: "exec_command",
        },
      ],
    });

    expect(transcript.blocks.map((block) => block.kind)).not.toContain("command-step");
    expect(JSON.stringify(transcript)).not.toContain("exec_command");
    expect(JSON.stringify(transcript)).not.toContain("已运行命令");
  });

  it("normalizes shell-wrapped commands before deduping command steps", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-shell-wrapper",
      status: "completed",
      processItems: [
        {
          id: "cmd-inner",
          kind: "command",
          displayKind: "host.command",
          status: "completed",
          command: "df -h",
          outputPreview: "Filesystem Size Used",
        },
        {
          id: "cmd-shell",
          kind: "command",
          displayKind: "host.command",
          status: "completed",
          command: "/bin/zsh -lc 'df -h'",
          outputPreview: "Filesystem Size Used",
        },
      ],
    });

    const commands = transcript.blocks.filter((block) => block.kind === "command-step");
    expect(commands).toHaveLength(1);
    expect(commands[0]).toMatchObject({
      text: "已运行 df -h",
      command: "df -h",
    });
    expect(JSON.stringify(transcript)).not.toContain("/bin/zsh -lc");
  });

  it("keeps browser-open activity as a visible transcript step", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-browser-open",
      status: "completed",
      processItems: [
        {
          id: "open-1",
          kind: "tool",
          displayKind: "browser.open",
          status: "completed",
          text: "已浏览网页",
          inputSummary: "https://coinmarketcap.com/currencies/bitcoin/",
        },
      ],
    });

    const browserStep = transcript.blocks.find((block) => block.displayKind === "browser.open");
    expect(browserStep).toMatchObject({
      kind: "file-step",
      text: "已浏览网页",
      inputSummary: "https://coinmarketcap.com/currencies/bitcoin/",
    });
  });
});
