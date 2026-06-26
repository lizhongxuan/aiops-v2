import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { UserMessageBubble } from "./AiopsThread";

describe("UserMessageBubble", () => {
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

  it("renders long user input as stable full text without an ellipsis preview", async () => {
    const text = "我需要先分析一个线上运维问题，而不是立即执行命令。请根据日志、依赖和最近变更判断可能原因，并列出下一步只读排查证据，这段输入比较长也必须完整展示。";

    await act(async () => {
      root.render(<UserMessageBubble text={text} />);
    });

    const bubble = container.firstElementChild as HTMLElement | null;
    expect(bubble?.textContent).toBe(text);
    expect(bubble?.textContent).not.toContain("...");
    expect(bubble?.className).toContain("whitespace-pre-wrap");
    expect(bubble?.className).toContain("break-words");
    expect(bubble?.className).toContain("text-[15px]");
  });
});
