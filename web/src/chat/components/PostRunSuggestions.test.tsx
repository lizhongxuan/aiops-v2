import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  archiveOpsRunCase,
  createOpsRunExperienceCandidates,
  createOpsRunRunRecord,
} from "@/api/chatOpsRuns";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";

import { OpsRunSummaryCard } from "./OpsRunSummaryCard";

vi.mock("@/api/chatOpsRuns", () => ({
  archiveOpsRunCase: vi.fn(),
  createOpsRunExperienceCandidates: vi.fn(),
  createOpsRunRunRecord: vi.fn(),
}));

describe("PostRunSuggestions", () => {
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
    vi.clearAllMocks();
  });

  it("does not show post-run action buttons without backend suggestions", async () => {
    const state = createInitialAiopsTransportState("thread-no-suggestions");
    state.opsRun = {
      id: "opsrun-chat-only",
      source: "chat",
      status: "completed",
      title: "普通问答",
    };

    await act(async () => {
      root.render(<OpsRunSummaryCard state={state} />);
    });

    expect(container.textContent).not.toContain("生成 Run Record");
    expect(container.textContent).not.toContain("生成处理记录");
    expect(container.textContent).not.toContain("生成经验候选");
    expect(container.textContent).not.toContain("生成 Case");
  });

  it("deduplicates saved-record suggestions and reuses existing archive APIs", async () => {
    vi.mocked(archiveOpsRunCase).mockResolvedValue({ case: { id: "case-1" } });
    vi.mocked(createOpsRunRunRecord).mockResolvedValue({ id: "record-1" });
    vi.mocked(createOpsRunExperienceCandidates).mockResolvedValue({
      items: [{ id: "exp-1" }],
    });
    const state = createInitialAiopsTransportState("thread-suggestions");
    state.opsRun = {
      id: "opsrun-reusable",
      source: "chat",
      status: "completed",
      title: "修复 redis 主从复制异常",
      postRunSuggestions: [
        { type: "run_record", label: "生成 Run Record" },
        { type: "processing_record", label: "生成处理记录" },
        { type: "experience_candidate", label: "生成经验候选" },
        { type: "case", label: "生成 Case" },
      ],
    } as typeof state.opsRun;

    await act(async () => {
      root.render(<OpsRunSummaryCard state={state} />);
    });

    expect(buttonByText("生成 Run Record")).toBeUndefined();
    expect(buttonByText("生成处理记录")).toBeTruthy();
    expect(buttonByText("生成经验候选")).toBeTruthy();
    expect(buttonByText("生成 Case")).toBeTruthy();

    await act(async () => {
      buttonByText("生成处理记录")?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(createOpsRunRunRecord).toHaveBeenCalledWith("opsrun-reusable", {
      sessionId: undefined,
      turnId: undefined,
      title: "修复 redis 主从复制异常",
      summary: undefined,
    });
    expect(container.textContent).toContain("已生成处理记录：record-1");

    await act(async () => {
      buttonByText("生成经验候选")?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(createOpsRunExperienceCandidates).toHaveBeenCalled();

    await act(async () => {
      buttonByText("生成 Case")?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(archiveOpsRunCase).toHaveBeenCalled();
  });

  function buttonByText(text: string) {
    return Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes(text),
    );
  }
});
