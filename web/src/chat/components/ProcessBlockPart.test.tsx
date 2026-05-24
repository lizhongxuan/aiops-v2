import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

import { ProcessBlockPart } from "./ProcessBlockPart";

describe("ProcessBlockPart", () => {
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

  it("labels compaction process blocks without flooding the transcript", async () => {
    await act(async () => {
      root.render(<ProcessBlockPart block={contextCompactionBlock()} />);
    });

    expect(container.textContent).toContain("上下文压缩");
    expect(container.textContent).toContain("已保留当前任务和关键证据。");
  });
});

function contextCompactionBlock(): AiopsProcessBlock {
  return {
    id: "context-compaction",
    kind: "system",
    displayKind: "context.compaction",
    status: "running",
    text: "已保留当前任务和关键证据。",
  };
}
