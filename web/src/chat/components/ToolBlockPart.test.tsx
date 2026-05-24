import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

import { ToolBlockPart } from "./ToolBlockPart";

describe("ToolBlockPart", () => {
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

  it("shows bounded externalized tool preview without exposing external details", async () => {
    await act(async () => {
      root.render(<ToolBlockPart block={externalizedToolBlock()} />);
    });

    expect(container.textContent).toContain("logs.search");
    expect(container.textContent).toContain("Large log output externalized.");
    expect(container.textContent).not.toContain("结果较大，仅显示摘要。");
    expect(container.textContent).not.toContain("已外溢");
    expect(container.textContent).not.toContain("原始日志摘要");
    expect(container.textContent).not.toContain("查看原始证据");
    expect(container.textContent).not.toContain("原始 22000 bytes");
    expect(container.textContent).not.toContain("内联 900 bytes");
    expect(container.textContent).not.toContain("spill-1");
  });
});

function externalizedToolBlock(): AiopsProcessBlock {
  return {
    id: "tool-large-result",
    kind: "tool",
    displayKind: "logs.search",
    status: "completed",
    text: "logs.search",
    outputPreview: "Large log output externalized.",
    materializationTier: "large",
    originalBytes: 22000,
    inlineBytes: 900,
    externalReferences: [
      {
        id: "spill-1",
        kind: "blob",
        title: "nginx raw logs",
        summary: "原始日志摘要",
      },
    ],
  };
}
