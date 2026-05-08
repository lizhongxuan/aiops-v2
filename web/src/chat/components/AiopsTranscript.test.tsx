import { act, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AiopsTranscript, type AiopsTranscriptBlock } from "./AiopsTranscript";
import { getAssistantAiopsTranscriptMeta } from "./AiopsThread";

const { approvalDecision } = vi.hoisted(() => ({
  approvalDecision: vi.fn(),
}));

vi.mock("@/transport/useAiopsTransportCommands", () => ({
  useAiopsTransportCommands: () => ({
    approvalDecision,
  }),
}));

describe("AiopsTranscript", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    approvalDecision.mockClear();
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

  it("renders text, tool, and text in blockOrder order", async () => {
    await renderTranscript({
      turnStatus: "completed",
      blockOrder: ["text-1", "cmd-1", "text-2"],
      blocksById: {
        "text-1": textBlock("text-1", "我先检查。"),
        "cmd-1": commandBlock("cmd-1", {
          summary: "已运行 pwd",
          command: "pwd",
          stdout: "/tmp\n",
        }),
        "text-2": textBlock("text-2", "检查完成。"),
      },
    });

    const text = transcriptText();
    expect(text.indexOf("我先检查。")).toBeLessThan(text.indexOf("已运行 pwd"));
    expect(text.indexOf("已运行 pwd")).toBeLessThan(text.indexOf("检查完成。"));
  });

  it("expands running command and collapses completed command by default", async () => {
    await renderTranscript({
      turnStatus: "working",
      blocks: [
        commandBlock("cmd-1", {
          status: "running",
          summary: "正在运行 npm test",
          command: "npm test",
          stdout: "running\n",
        }),
      ],
    });

    expect(queryByTestId("aiops-terminal-card-cmd-1")).not.toBeNull();
    expect(transcriptText()).toContain("running");

    await renderTranscript({
      turnStatus: "completed",
      blocks: [
        commandBlock("cmd-1", {
          status: "completed",
          summary: "已运行 npm test",
          command: "npm test",
          stdout: "running\n",
        }),
      ],
    });

    expect(queryByTestId("aiops-terminal-card-cmd-1")).toBeNull();
    expect(transcriptText()).toContain("已运行 npm test");
  });

  it("renders non-command tool details as muted inline rows instead of a terminal card", async () => {
    await renderTranscript({
      turnStatus: "working",
      blocks: [
        {
          id: "search-1",
          type: "tool",
          tool: {
            toolKind: "search",
            title: "Search",
            summary: "正在搜索 transport 文件夹中的文件",
            status: "running",
            output: {
              stdout: "",
              stderr: "",
              text: "Searched for AssistantTransport|assistant-ui in aiops-v2\nRead README.md\nRead package.json\n",
              truncated: false,
            },
          },
        },
      ],
    });

    expect(queryByTestId("aiops-terminal-card-search-1")).toBeNull();
    expect(transcriptText()).toContain("Searched for AssistantTransport|assistant-ui in aiops-v2");
    const detailRows = Array.from(container.querySelectorAll("[data-testid='aiops-tool-detail-row']"));
    expect(detailRows.some((row) => row.textContent === "Read README.md" && row.className.includes("text-slate-400"))).toBe(true);
  });

  it("allows a completed command to be manually expanded in place", async () => {
    await renderTranscript({
      turnStatus: "completed",
      blocks: [
        commandBlock("cmd-1", {
          summary: "已运行命令",
          command: "sed -n '1,220p' SKILL.md",
          stdout: "name: using-superpowers\n",
        }),
      ],
    });

    expect(queryByTestId("aiops-terminal-card-cmd-1")).toBeNull();
    clickButtonByText("已运行命令");

    expect(queryByTestId("aiops-terminal-card-cmd-1")).not.toBeNull();
    expect(transcriptText()).toContain("$ sed -n '1,220p' SKILL.md");
  });

  it("keeps failed commands expanded by default", async () => {
    await renderTranscript({
      turnStatus: "failed",
      blocks: [
        commandBlock("cmd-1", {
          status: "failed",
          summary: "运行 npm test 失败",
          command: "npm test",
          stderr: "Error: failed\n",
        }),
      ],
    });

    expect(queryByTestId("aiops-terminal-card-cmd-1")).not.toBeNull();
    expect(transcriptText()).toContain("Error: failed");
  });

  it("renders thinking as a muted inline status", async () => {
    await renderTranscript({
      turnStatus: "working",
      blocks: [{ id: "thinking-1", type: "thinking", thinking: { status: "running" } }],
    });

    expect(transcriptText()).toContain("正在思考");
  });

  it("expands aggregate details without rendering child command terminal cards", async () => {
    await renderTranscript({
      turnStatus: "completed",
      blockOrder: ["agg-1"],
      blocksById: {
        "agg-1": {
          id: "agg-1",
          type: "aggregate",
          aggregate: {
            summary: "已运行 2 条命令",
            status: "completed",
            childBlockIds: ["cmd-1", "cmd-2"],
            counts: { command: 2 },
          },
        },
        "cmd-1": commandBlock("cmd-1", { summary: "已运行 pwd", command: "pwd", stdout: "/tmp\n" }),
        "cmd-2": commandBlock("cmd-2", { summary: "已运行 whoami", command: "whoami", stdout: "aiops\n" }),
      },
    });

    clickButtonByText("已运行 2 条命令");

    expect(transcriptText()).toContain("已运行 pwd");
    expect(transcriptText()).toContain("已运行 whoami");
    expect(queryByTestId("aiops-terminal-card-cmd-1")).toBeNull();
    expect(queryByTestId("aiops-terminal-card-cmd-2")).toBeNull();
  });

  it("calls approvalDecision with the single frontend approval vocabulary", async () => {
    await renderTranscript({
      turnStatus: "blocked",
      blocks: [
        {
          id: "approval-1",
          type: "approval",
          approval: {
            approvalId: "approval-1",
            title: "需要确认",
            summary: "要执行这个命令，需要你确认吗？",
            command: "kubectl rollout restart deploy/payment-api",
            status: "pending",
          },
        },
      ],
    });

    clickButtonByText("同意");
    expect(approvalDecision).toHaveBeenCalledWith("approval-1", "approve");

    clickButtonByText("拒绝");
    expect(approvalDecision).toHaveBeenCalledWith("approval-1", "deny");
  });

  it("extracts assistant transcript data from metadata.custom.aiops", () => {
    const meta = getAssistantAiopsTranscriptMeta({
      content: [{ type: "text", text: "legacy final answer" }],
      metadata: {
        custom: {
          aiops: {
            turnId: "turn-1",
            turnStatus: "completed",
            blocks: [textBlock("text-1", "来自 custom aiops")],
          },
        },
      },
    });

    expect(meta.turnId).toBe("turn-1");
    expect(meta.turnStatus).toBe("completed");
    expect(meta.blocks.map((block) => block.id)).toEqual(["text-1"]);
  });

  async function renderTranscript(props: ComponentProps<typeof AiopsTranscript>) {
    await act(async () => {
      root.render(<AiopsTranscript {...props} />);
    });
  }

  function transcriptText() {
    return queryByTestId("aiops-transcript")?.textContent || "";
  }

  function queryByTestId(testId: string) {
    return container.querySelector(`[data-testid="${testId}"]`);
  }

  function clickButtonByText(text: string) {
    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes(text));
    if (!button) {
      throw new Error(`No button containing ${text}`);
    }
    act(() => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
  }
});

function textBlock(id: string, text: string): AiopsTranscriptBlock {
  return {
    id,
    type: "text",
    text: { role: "assistant", text, status: "completed" },
  };
}

function commandBlock(
  id: string,
  overrides: {
    status?: "queued" | "running" | "completed" | "failed" | "blocked" | "rejected";
    summary?: string;
    command?: string;
    stdout?: string;
    stderr?: string;
  },
): AiopsTranscriptBlock {
  return {
    id,
    type: "tool",
    tool: {
      toolKind: "command",
      title: "Shell",
      summary: overrides.summary || "已运行命令",
      status: overrides.status || "completed",
      command: overrides.command || "pwd",
      output: {
        stdout: overrides.stdout || "",
        stderr: overrides.stderr || "",
        text: `${overrides.stdout || ""}${overrides.stderr || ""}`,
        truncated: false,
      },
    },
  };
}
