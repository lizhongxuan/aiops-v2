import { act } from "react";
import type { ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ContextStatusNotice } from "./ContextStatusNotice";

describe("ContextStatusNotice", () => {
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

  it("shows L4 compaction message", async () => {
    await renderNotice({
      kind: "context.compaction.started",
      layer: "L4",
      message: "正在压缩上下文，当前任务会继续",
    });

    expect(container.textContent).toContain("正在压缩上下文");
    expect(container.textContent).toContain("L4");
  });

  it("shows L5 too-long status without retry progress", async () => {
    await renderNotice({
      kind: "context.compaction.too_long",
    });

    expect(container.textContent).toContain("上下文过长，已保留关键摘要并进入保守模式");
    expect(container.textContent).not.toContain("正在重试压缩");
    expect(container.textContent).not.toContain("1/3");
  });

  it("shows small context notice", async () => {
    await renderNotice({ kind: "context.small_context.enabled" });

    expect(container.textContent).toContain("当前模型上下文较小");
  });

  it("shows compact summary in an expandable detail", async () => {
    await renderNotice({
      kind: "context.compaction.completed",
      compactSummary: "保留目标、审批状态和关键证据。",
      referenceIds: ["ref-1"],
    });

    expect(container.querySelector("details")).toBeTruthy();
    expect(container.textContent).toContain("查看压缩摘要");
    expect(container.textContent).toContain("保留目标、审批状态和关键证据。");
    expect(container.textContent).toContain("已保留 1 项上下文引用");
    expect(container.textContent).not.toContain("ref-1");
  });

  it("shows failed compaction guidance", async () => {
    await renderNotice({ kind: "context.compaction.failed" });

    expect(container.textContent).toContain("上下文过长或压缩失败，已进入保守模式");
    expect(container.textContent).toContain("必要时可继续基于摘要排查");
    expect(container.querySelector('[role="alert"]')).toBeTruthy();
  });

  it("renders nothing without an event", async () => {
    await act(async () => {
      root.render(<ContextStatusNotice event={null} />);
    });

    expect(container.textContent).toBe("");
  });

  async function renderNotice(event: ComponentProps<typeof ContextStatusNotice>["event"]) {
    await act(async () => {
      root.render(<ContextStatusNotice event={event} />);
    });
  }
});
