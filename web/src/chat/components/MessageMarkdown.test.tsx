import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { MessageMarkdown } from "./MessageMarkdown";

describe("MessageMarkdown", () => {
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

  it("renders markdown emphasis and ordered lists as semantic HTML", async () => {
    await act(async () => {
      root.render(
        <MessageMarkdown
          text={"如果你愿意，我可以继续帮你看：\n\n1. **K线/支撑压力位**\n2. **最近 24h 爆仓和资金费率**"}
        />,
      );
    });

    expect(container.querySelectorAll("ol li")).toHaveLength(2);
    expect(container.querySelector("strong")?.textContent).toBe("K线/支撑压力位");
    expect(container.textContent).not.toContain("**K线/支撑压力位**");
  });

  it("escapes raw HTML instead of injecting it", async () => {
    await act(async () => {
      root.render(<MessageMarkdown text={'<img src=x onerror="alert(1)"> **safe**'} />);
    });

    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("<img");
    expect(container.querySelector("strong")?.textContent).toBe("safe");
  });

  it("shows long URLs as copyable summaries instead of full text", async () => {
    const writes: string[] = [];
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          writes.push(value);
        },
      },
    });
    const url = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd&include_market_cap=true";

    await act(async () => {
      root.render(<MessageMarkdown text={`来源：\n- CoinGecko API: ${url}`} />);
    });

    const link = container.querySelector("a");
    expect(link?.textContent).toBe("api.coingecko.com /api/v3/simple/price");
    expect(container.textContent).not.toContain("include_market_cap=true");

    await act(async () => {
      link?.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    });

    expect(writes).toEqual([url]);
  });

  it("keeps a source label and its URL in one compact paragraph", async () => {
    await act(async () => {
      root.render(<MessageMarkdown text={"来源：\n\nhttps://www.coinbase.com/price/bitcoin"} />);
    });

    const paragraphs = container.querySelectorAll("p");
    expect(paragraphs).toHaveLength(1);
    expect(paragraphs[0].textContent).toBe("来源： www.coinbase.com /price/bitcoin");
  });

  it("falls back to selection copy when clipboard api rejects", async () => {
    const execCalls: string[] = [];
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("denied");
        },
      },
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: (command: string) => {
        execCalls.push(command);
        return true;
      },
    });

    await act(async () => {
      root.render(<MessageMarkdown text="https://api.coingecko.com/api/v3/simple/price?ids=bitcoin" />);
    });

    await act(async () => {
      container.querySelector("a")?.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    });

    expect(execCalls).toContain("copy");
  });

  it("renders unix timestamp fields as readable local time", async () => {
    await act(async () => {
      root.render(<MessageMarkdown text="数据更新时间：CoinGecko 返回字段 `last_updated_at=1778157569`" />);
    });

    expect(container.textContent).toContain("last_updated_at=2026-05-07 20:39:29 GMT+8");
    expect(container.textContent).not.toContain("1778157569");
  });

  it("renders generic timestamp labels as readable local time", async () => {
    await act(async () => {
      root.render(<MessageMarkdown text={"数据源返回时间戳：1778167247\n更新时间 timestamp: 1778167247\nUnix 时间戳 `1778167247`\n数据更新时间：Unix `1778167247`"} />);
    });

    expect(container.textContent).toContain("数据源返回时间戳：2026-05-07 23:20:47 GMT+8");
    expect(container.textContent).toContain("timestamp: 2026-05-07 23:20:47 GMT+8");
    expect(container.textContent).toContain("Unix 时间戳 2026-05-07 23:20:47 GMT+8");
    expect(container.textContent).toContain("数据更新时间：Unix 2026-05-07 23:20:47 GMT+8");
    expect(container.textContent).not.toContain("1778167247");
  });
});
