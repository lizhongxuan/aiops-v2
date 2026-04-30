import { describe, expect, it } from "vitest";
import { formatMainChatTurns } from "./chatTurnFormatter";

describe("formatMainChatTurns", () => {
  it("keeps concise assistant process narration in completed single-host process details", () => {
    const turns = formatMainChatTurns({
      hideLiveProcessDetails: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        items: [
          {
            id: "search-1",
            kind: "search",
            text: "已搜索网页",
            status: "completed",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看今天BTC行情",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-draft-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我将先核实今天 BTC 的最新价格、24 小时涨跌和日内区间。",
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-draft-2",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我继续放宽到权威财经媒体和交易所来源。",
          createdAt: "2026-04-29T00:00:02Z",
        },
        {
          id: "assistant-final",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "今天 BTC 行情简要如下：价格约 $76.2k-$76.4k。",
          createdAt: "2026-04-29T00:00:03Z",
        },
      ],
    });

    expect(turns).toHaveLength(1);
    expect(turns[0].finalMessage.card.text).toContain("今天 BTC 行情简要如下");
    expect(turns[0].processItems.map((item) => item.text)).toEqual([
      "我将先核实今天 BTC 的最新价格、24 小时涨跌和日内区间。",
      "我继续放宽到权威财经媒体和交易所来源。",
      "已搜索网页",
    ]);
    expect(turns[0].summary).not.toContain("过程说明");
  });

  it("filters final-like assistant drafts from single-host process details", () => {
    const turns = formatMainChatTurns({
      hideLiveProcessDetails: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        items: [
          {
            id: "search-1",
            kind: "search",
            processKind: "search",
            text: "已搜索网页（BTC price）",
            status: "completed",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看今天BTC行情",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-final-like",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "今天 BTC 行情如下：\n- 现价：$76,300\n- 24小时涨跌：+1.2%\n来源：CoinGecko",
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-final",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "今天 BTC 行情简要如下：现价约 $76,300。",
          createdAt: "2026-04-29T00:00:02Z",
        },
      ],
    });

    expect(turns[0].processItems.map((item) => item.text)).toEqual([
      "已搜索网页（BTC price）",
    ]);
    expect(JSON.stringify(turns[0].processItems)).not.toContain("现价：$76,300");
  });

  it("deduplicates assistant process narration across projection and message sources", () => {
    const text = "我将检查这台主机的 CPU、内存、磁盘和负载信息。";
    const turns = formatMainChatTurns({
      hideLiveProcessDetails: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        items: [
          {
            id: "assistant-projection-1",
            kind: "assistant",
            text,
            status: "completed",
          },
          {
            id: "cmd-1",
            kind: "command",
            processKind: "command",
            text: "已运行 uptime",
            command: "uptime",
            output: "17:40 up 1 day",
            status: "completed",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看主机资源",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-draft-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text,
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-final",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "主机资源整体正常，负载不高。",
          createdAt: "2026-04-29T00:00:03Z",
        },
      ],
    });

    expect(turns[0].processItems.filter((item) => item.text === text)).toHaveLength(1);
    expect(turns[0].processItems.map((item) => item.text)).toContain("已运行 uptime");
  });

  it("does not generate collapsed bookkeeping summaries for completed single-host process details", () => {
    const turns = formatMainChatTurns({
      hideLiveProcessDetails: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        items: [
          {
            id: "search-1",
            kind: "search",
            processKind: "search",
            text: "已搜索网页（2026-04-29 A股 大盘 上证指数 深证成指 创业板指）",
            status: "completed",
          },
          {
            id: "search-2",
            kind: "search",
            processKind: "search",
            text: "已搜索网页（2026-04-29 A股 涨跌家数 成交额）",
            status: "completed",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看A股情况",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-final",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "今天A股整体偏强，上证、深证和创业板均上涨。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:10Z",
        },
      ],
    });

    expect(turns).toHaveLength(1);
    expect(turns[0].processItems.map((item) => item.text)).toEqual([
      "已搜索网页（2026-04-29 A股 大盘 上证指数 深证成指 创业板指）",
      "已搜索网页（2026-04-29 A股 涨跌家数 成交额）",
    ]);
    expect(turns[0].summary).toBe("");
    expect(JSON.stringify(turns[0])).not.toContain("明细已折叠");
    expect(JSON.stringify(turns[0])).not.toMatch(/已记录\s*\d+\s*条过程/u);
  });

  it("removes empty source placeholders from assistant final answers", () => {
    const turns = formatMainChatTurns({
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看今天BTC行情",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-final",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "今天 BTC 行情简要如下：；来源： - -\n\nCoinGecko：$76,199.25\n\n来源：CoinGecko；CoinMarketCap",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:01Z",
        },
      ],
    });

    expect(turns[0].finalMessage.card.text).toContain("今天 BTC 行情简要如下");
    expect(turns[0].finalMessage.card.text).not.toContain("来源： -");
    expect(turns[0].finalMessage.card.text).not.toContain("：；");
    expect(turns[0].finalMessage.card.text).toContain("来源：CoinGecko；CoinMarketCap");
  });

  it("preserves active process command and output for expandable command rows", () => {
    const turns = formatMainChatTurns({
      turnActive: true,
      hideLiveProcessDetails: false,
      activeProcess: {
        turnKeys: ["turn-1"],
        items: [
          {
            id: "cmd-1",
            kind: "command",
            processKind: "command",
            text: "已运行 df -h",
            command: "df -h",
            output: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
            status: "completed",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看主机资源",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
      ],
    });

    expect(turns[0].processItems[0]).toMatchObject({
      kind: "command",
      processKind: "command",
      text: "已运行 df -h",
      command: "df -h",
      output: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
    });
    const commandBlock = turns[0].processTranscript.blocks.find((block) => block.kind === "command-step");
    expect(commandBlock).toMatchObject({
      text: "已运行 df -h",
      command: "df -h",
      outputPreview: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
    });
  });

  it("keeps active single-host process rows in the transcript instead of relying on LiveStatusCard", () => {
    const turns = formatMainChatTurns({
      turnActive: true,
      hideLiveProcessDetails: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        phase: "thinking",
        items: [
          {
            id: "search-1",
            kind: "search",
            displayKind: "browser.search",
            status: "running",
            text: "正在搜索网页",
            inputSummary: "2026-04-29 A股 大盘",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看A股情况",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
      ],
    });

    const searchBlock = turns[0].processTranscript.blocks.find((block) => block.kind === "search-step");
    expect(searchBlock).toMatchObject({
      text: "正在搜索网页",
      inputSummary: "2026-04-29 A股 大盘",
    });
    expect(JSON.stringify(turns[0].processTranscript)).not.toContain("Working for");
  });

  it("does not inject assistant process narration twice into the transcript", () => {
    const turns = formatMainChatTurns({
      turnActive: false,
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看A股情况",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-intent-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我先拉取当前主要指数和板块行情信息，给你一个简短概览。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-final-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "截至 2026年4月29日收盘，A 股明显反弹。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:10Z",
        },
      ],
      activeProcess: {
        turnKeys: ["turn-1"],
        phase: "completed",
        items: [
          {
            id: "search-1",
            kind: "search",
            displayKind: "browser.search",
            status: "completed",
            text: "已搜索网页",
            inputSummary: "2026-04-29 A股 大盘",
          },
        ],
      },
    });

    const transcript = turns[0].processTranscript;
    expect(transcript.blocks.filter((block) => block.kind === "assistant-intent")).toHaveLength(1);
    expect(transcript.blocks.filter((block) => block.text === "我先拉取当前主要指数和板块行情信息，给你一个简短概览。")).toHaveLength(1);
    expect(transcript.blocks.map((block) => block.kind)).toContain("search-step");
  });

  it("deduplicates assistant narration between message drafts and activity projection in the transcript", () => {
    const text = "我将用只读系统命令检查这台主机的时间、运行时长、CPU、内存和磁盘概况，然后汇总当前状态。";
    const turns = formatMainChatTurns({
      turnActive: false,
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "看下主机的情况",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-intent-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text,
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-final-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "主机当前运行稳定，CPU 负载不高。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:12Z",
        },
      ],
      activeProcess: {
        turnKeys: ["turn-1"],
        phase: "completed",
        items: [
          {
            id: "assistant-projection-1",
            kind: "assistant",
            displayKind: "assistant.message",
            status: "completed",
            text,
          },
          {
            id: "cmd-1",
            kind: "command",
            displayKind: "host.command",
            processKind: "command",
            status: "completed",
            text: "已运行 uptime",
            command: "uptime",
            output: "00:00 up 14 days",
          },
        ],
      },
    });

    const transcript = turns[0].processTranscript;
    expect(transcript.blocks.filter((block) => block.text === text)).toHaveLength(1);
    expect(transcript.blocks.filter((block) => block.kind === "reasoning-summary" && block.text === text)).toHaveLength(0);
    expect(transcript.blocks.map((block) => block.kind)).toContain("command-step");
  });

  it("does not let stale activeProcess 0s override completed turn duration", () => {
    const turns = formatMainChatTurns({
      turnActive: false,
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "看下主机的情况",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-intent-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我将用只读系统命令检查这台主机的时间、运行时长、CPU、内存和磁盘概况，然后汇总当前状态。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:01Z",
        },
        {
          id: "assistant-final-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "主机当前运行稳定，CPU 负载不高。",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:12Z",
        },
      ],
      activeProcess: {
        turnKeys: ["turn-1"],
        phase: "completed",
        elapsedLabel: "0s",
        items: [
          {
            id: "cmd-1",
            kind: "command",
            displayKind: "host.command",
            processKind: "command",
            status: "completed",
            text: "已运行 uptime",
            command: "uptime",
          },
        ],
      },
    });

    expect(turns[0].processTranscript.header.text).toBe("已处理 12s");
    expect(turns[0].processLabel).toBe("已处理 12s");
  });

  it("keeps bottom thinking active while the final assistant message is still streaming", () => {
    const turns = formatMainChatTurns({
      turnActive: true,
      activeProcess: {
        turnKeys: ["turn-1"],
        phase: "executing",
        elapsedLabel: "56s",
        items: [
          {
            id: "search-1",
            kind: "search",
            displayKind: "browser.search",
            status: "completed",
            text: "已搜索网页",
            inputSummary: "BTC 今日行情",
          },
        ],
      },
      conversationCards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "看下 BTC 怎么样了",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:00Z",
        },
        {
          id: "assistant-final-streaming",
          type: "AssistantMessageCard",
          role: "assistant",
          status: "inProgress",
          text: "BTC 价格：$75,410.87\n\n24 小时涨跌：+1.7%",
          turnId: "turn-1",
          createdAt: "2026-04-29T00:00:10Z",
        },
      ],
    });

    expect(turns[0].hasActiveFinalMessage).toBe(true);
    expect(turns[0].processTranscript.showThinking).toBe(true);
    expect(turns[0].processTranscript.blocks.some((block) => block.text === "正在思考")).toBe(false);
  });
});
