import { describe, expect, it } from "vitest";

import { assistantMessageRenderedFinalText, isNearThreadBottom } from "./AiopsThread";

describe("AiopsThread auto-scroll helpers", () => {
  it("treats a viewport close to the bottom as sticky", () => {
    expect(isNearThreadBottom({ scrollTop: 890, clientHeight: 100, scrollHeight: 1000 })).toBe(true);
  });

  it("does not auto-stick when the user has scrolled up into history", () => {
    expect(isNearThreadBottom({ scrollTop: 200, clientHeight: 100, scrollHeight: 1000 })).toBe(false);
  });
});

describe("assistant message final text", () => {
  it("prefers transport finalText over stale assistant content", () => {
    const text = assistantMessageRenderedFinalText(
      [{ type: "text", text: "让我查看一下这台主机的基本信息。" }],
      { finalText: "" },
    );

    expect(text).toBe("");
  });
});
