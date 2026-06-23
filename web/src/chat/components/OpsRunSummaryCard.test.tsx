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

describe("OpsRunSummaryCard", () => {
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

  it("renders the current chat ops run summary", async () => {
    const state = createInitialAiopsTransportState("thread-opsrun");
    state.opsRun = {
      id: "opsrun-turn-1",
      source: "chat",
      status: "working",
      title: "主机A跟主机B上PG不同步",
      targetSummary: "主机A/主机B PG 与主机C pg_mon",
      evidenceCount: 3,
      currentStep: "正在只读采集 PG 同步证据",
    };

    await act(async () => {
      root.render(<OpsRunSummaryCard state={state} />);
    });

    expect(
      container.querySelector('[data-testid="ops-run-summary-card"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain("主机A跟主机B上PG不同步");
    expect(container.textContent).toContain("处理中");
    expect(container.textContent).toContain("AI Chat");
    expect(container.textContent).toContain("正在只读采集 PG 同步证据");
    expect(container.textContent).toContain("3 条证据");
    expect(container.textContent).toContain("主机A/主机B PG 与主机C pg_mon");
    expect(container.textContent).not.toContain("生成 Case");
  });

  it("does not render without an ops run", async () => {
    await act(async () => {
      root.render(
        <OpsRunSummaryCard
          state={createInitialAiopsTransportState("thread-empty")}
        />,
      );
    });

    expect(
      container.querySelector('[data-testid="ops-run-summary-card"]'),
    ).toBeNull();
    expect(container.textContent).toBe("");
  });

  it("creates an archive case only after a completed ops run action is clicked", async () => {
    vi.mocked(archiveOpsRunCase).mockResolvedValue({
      case: { id: "case-001" },
    });
    const state = createInitialAiopsTransportState("thread-archive");
    state.opsRun = {
      id: "opsrun-turn-archive",
      sessionId: "thread-archive",
      turnId: "turn-archive",
      source: "chat",
      status: "completed",
      title: "PG 不同步修复",
      currentStep: "已整理诊断和执行记录",
    };

    await act(async () => {
      root.render(<OpsRunSummaryCard state={state} />);
    });

    expect(archiveOpsRunCase).not.toHaveBeenCalled();
    const button = buttonByText("生成 Case");
    expect(button).not.toBeNull();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(archiveOpsRunCase).toHaveBeenCalledWith("opsrun-turn-archive", {
      sessionId: "thread-archive",
      turnId: "turn-archive",
      title: "PG 不同步修复",
      summary: "已整理诊断和执行记录",
    });
    expect(container.textContent).toContain("已生成 Case：case-001");
  });

  it("creates run record and experience candidates from completed ops runs", async () => {
    vi.mocked(createOpsRunRunRecord).mockResolvedValue({
      id: "run-record-001",
    });
    vi.mocked(createOpsRunExperienceCandidates).mockResolvedValue({
      items: [{ id: "exp-001" }],
    });
    const state = createInitialAiopsTransportState("thread-record");
    state.opsRun = {
      id: "opsrun-record",
      source: "chat",
      status: "completed",
      title: "运维处理",
    };

    await act(async () => {
      root.render(<OpsRunSummaryCard state={state} />);
    });

    await act(async () => {
      buttonByText("生成 Run Record")?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(createOpsRunRunRecord).toHaveBeenCalledWith("opsrun-record", {
      sessionId: undefined,
      turnId: undefined,
      title: "运维处理",
      summary: undefined,
    });
    expect(container.textContent).toContain(
      "已生成 Run Record：run-record-001",
    );

    await act(async () => {
      buttonByText("生成经验候选")?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(createOpsRunExperienceCandidates).toHaveBeenCalledWith(
      "opsrun-record",
      {
        sessionId: undefined,
        turnId: undefined,
        title: "运维处理",
        summary: undefined,
      },
    );
    expect(container.textContent).toContain("已生成 1 条经验候选");
  });

  function buttonByText(text: string) {
    return Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes(text),
    );
  }
});
