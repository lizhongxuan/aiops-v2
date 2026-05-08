import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { TerminalOutputCard } from "./TerminalOutputCard";
import type { AiopsTranscriptBlock } from "./AiopsTranscript";

describe("TerminalOutputCard", () => {
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

  it("renders the command and streamed output text in a shallow gray monospace card", async () => {
    await renderCard(commandBlock({
      command: "npm --prefix web test",
      status: "running",
      output: {
        stdout: "stdout first\n",
        stderr: "stderr second\n",
        text: "stdout first\nstderr second\n",
        truncated: false,
      },
    }));

    const card = queryByTestId("aiops-terminal-card-cmd-1");
    expect(card?.className).toContain("bg-slate-100");
    expect(card?.textContent).toContain("$ npm --prefix web test");
    expect(card?.textContent).toContain("stdout first\nstderr second");
    expect(card?.textContent).toContain("运行中");
    expect(card?.querySelector("pre")?.className).toContain("font-mono");
  });

  it("falls back to stdout then stderr and shows truncation and exit status", async () => {
    await renderCard(commandBlock({
      status: "failed",
      exitCode: 2,
      output: {
        stdout: "normal line\n",
        stderr: "error line\n",
        text: "",
        truncated: true,
      },
    }));

    const text = container.textContent || "";
    expect(text.indexOf("normal line")).toBeLessThan(text.indexOf("error line"));
    expect(text).toContain("输出已截断");
    expect(text).toContain("失败 2");
  });

  async function renderCard(block: AiopsTranscriptBlock) {
    await act(async () => {
      root.render(<TerminalOutputCard block={block} />);
    });
  }

  function queryByTestId(testId: string) {
    return container.querySelector(`[data-testid="${testId}"]`);
  }
});

function commandBlock(overrides: Partial<NonNullable<AiopsTranscriptBlock["tool"]>>): AiopsTranscriptBlock {
  return {
    id: "cmd-1",
    type: "tool",
    tool: {
      toolKind: "command",
      title: "Shell",
      summary: "正在运行命令",
      status: "completed",
      command: "pwd",
      output: {
        stdout: "",
        stderr: "",
        text: "",
        truncated: false,
      },
      ...overrides,
    },
  };
}
