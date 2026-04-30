import { describe, expect, it } from "vitest";
import { cleanAssistantDisplayText } from "./workspaceViewModel";

describe("cleanAssistantDisplayText", () => {
  it("removes empty market source placeholders", () => {
    const text = cleanAssistantDisplayText(
      "今天 BTC 大致在 7.62万-7.64万美元附近；来源： - -\n\nCoinMarketCap 显示 24 小时涨幅约 1%。",
      "assistant",
    );

    expect(text).toContain("今天 BTC 大致在");
    expect(text).toContain("CoinMarketCap");
    expect(text).not.toMatch(/来源[:：]\s*[-—–]/u);
  });

  it("removes empty punctuation left by missing market fields", () => {
    const text = cleanAssistantDisplayText(
      "今天A股整体偏弱，三大指数：，上证指数跌约 0.4%，创业板指小幅上涨。\nCoinGecko：：这次检索没有直接给出今天的 24h 高低。",
      "assistant",
    );

    expect(text).toContain("三大指数：上证指数");
    expect(text).toContain("CoinGecko：这次检索");
    expect(text).not.toContain("：，");
    expect(text).not.toContain("：：");
  });
});
