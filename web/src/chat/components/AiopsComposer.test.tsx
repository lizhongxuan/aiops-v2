import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AiopsComposer } from "./AiopsComposer";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

const mockState = vi.hoisted(() => ({
  sendCommand: vi.fn(),
  composerText: "",
  threadRunning: false,
  transportRunning: false,
  transportState: undefined as AiopsTransportState | undefined,
}));
const mockWorkspace = vi.hoisted(() => ({
  composerFocusNonce: 0,
  composerDisabledReason: "",
  llmLabel: "GPT-5.4",
}));

let activeRoot: Root | undefined;

vi.mock("@assistant-ui/react", () => ({
  ComposerPrimitive: {
    Root: ({ children, ...props }: any) => <div {...props}>{children}</div>,
    Input: ({ children }: any) => children,
  },
  useAssistantApi: () => ({ thread: () => ({ cancelRun: vi.fn() }) }),
  useAssistantTransportSendCommand: () => mockState.sendCommand,
  useAssistantTransportState: () => mockState.transportState ?? { pendingApprovals: {}, activeCommandStreams: {}, runtimeLiveness: {}, currentTurnId: "" },
  useComposer: () => ({ text: mockState.composerText }),
  useComposerRuntime: () => ({
    getState: () => ({ text: mockState.composerText }),
    setText: (value: string) => {
      mockState.composerText = value;
    },
  }),
  useThread: () => mockState.threadRunning,
}));

vi.mock("@/transport/aiopsTransportConverter", () => ({ isAiopsTransportRunning: () => mockState.transportRunning }));
vi.mock("./SessionTargetContext", () => ({
  useSessionTargetContext: () => ({
    metadata: {
      "aiops.target.kind": "host",
      "aiops.target.hostId": "server-local",
      "aiops.target.label": "server-local",
    },
    hostId: "server-local",
  }),
}));
vi.mock("./SessionWorkspaceContext", () => ({
  useSessionWorkspaceContext: () => mockWorkspace,
}));

describe("AiopsComposer host mention fuzzy search", () => {
  let container: HTMLDivElement;
  let root: Root;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mockState.composerText = "";
    mockState.threadRunning = false;
    mockState.transportRunning = false;
    mockState.transportState = undefined;
    mockWorkspace.composerFocusNonce = 0;
    mockWorkspace.composerDisabledReason = "";
    mockWorkspace.llmLabel = "GPT-5.4";
    mockState.sendCommand.mockReset();
    fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/v1/hosts")) {
        return new Response(JSON.stringify({ items: sampleHosts() }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (url.includes("/api/v1/ops-manuals") && !url.includes("/candidates") && !url.includes("/run-records")) {
        return new Response(JSON.stringify({ items: sampleOpsManuals() }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (url.endsWith("/api/v1/opsgraph/graphs")) {
        return new Response(JSON.stringify({ graphs: sampleOpsGraphs() }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await flushMicrotasks();
    await act(async () => {
      root.unmount();
    });
    await flushMicrotasks();
    container.remove();
    activeRoot = undefined;
    vi.unstubAllGlobals();
  });

  it("opens a category menu for @ before showing host suggestions", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    expect(container.querySelector('[data-testid="host-mention-suggestion-popover"]')).not.toBeNull();
    const rootItems = Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'));
    expect(rootItems.map((item) => item.textContent)).toEqual([
      expect.stringContaining("主机"),
      expect.stringContaining("监控"),
      expect.stringContaining("关系图谱"),
      expect.stringContaining("运维手册"),
    ]);
    expect(container.textContent).not.toContain("@server-local");
    expect(container.textContent).not.toContain("@host-a");
    expect(container.textContent).not.toContain("@add_workflow");
    expect(container.textContent).not.toContain("add_workflow");

    await act(async () => {
      rootItems[0]?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(input.value).toBe("@host-");
    expect(container.querySelector('[data-testid="host-mention-suggestion-item"]')?.textContent).toContain("local");

    await typeInComposer(input, "@host-pg");
    expect(container.textContent).toContain("pg-primary");
    expect(container.textContent).not.toContain("@redis");

    await act(async () => {
      container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(input.value).toBe("@120.77.239.90 ");
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toContain("120.77.239.90");
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).not.toContain("@");
  });

  it("uses attached chat composer styling without extra top gutter", async () => {
    await renderComposer(root);

    const shell = container.querySelector('[data-testid="aiops-composer-shell"]') as HTMLDivElement;
    const composerFrame = Array.from(shell.querySelectorAll("div")).find((element) =>
      String(element.className).includes("max-w-[49.5rem]"),
    );
    const composerRoot = Array.from(shell.querySelectorAll("div")).find((element) =>
      String(element.className).includes("rounded-[1.5rem]"),
    );

    expect(shell.className).toContain("pt-0");
    expect(composerFrame?.className).toContain("max-w-[49.5rem]");
    expect(composerRoot?.className).toContain("relative");
    expect(composerRoot?.className).toContain("z-10");
    expect(composerRoot?.className).toContain("rounded-[1.5rem]");
  });

  it("does not render the LLM setup prompt below the composer", async () => {
    mockWorkspace.composerDisabledReason = "请先在设置中配置 LLM";
    mockWorkspace.llmLabel = "LLM 未配置";

    await renderComposer(root);

    expect(container.textContent).toContain("LLM 未配置");
    expect(container.textContent).not.toContain("请先在设置中配置 LLM");
    expect((container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement).disabled).toBe(true);
  });

  it("uses keyboard navigation and keeps send behavior after insertion", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@host-redis");
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(input.value).toBe("@10.0.0.8 ");

    await typeInComposer(input, `${input.value}检查状态`);
    const sendButton = container.querySelector('[data-testid="omnibar-primary-action"]') as HTMLButtonElement;
    expect(sendButton.disabled).toBe(false);
    await act(async () => {
      sendButton.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(mockState.sendCommand).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "add-message",
        message: expect.objectContaining({
          metadata: expect.objectContaining({ "aiops.hostops.clientDetectedMultiHost": "false" }),
        }),
      }),
    );
  });

  it("does not attach implicit hostId or host target metadata for a plain advisory question", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "帮我分析 PostgreSQL checkpoint 频繁的常见原因");
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command.message).not.toHaveProperty("hostId");
    expect(command.message.metadata).not.toEqual(
      expect.objectContaining({
        "aiops.target.hostId": "server-local",
        "aiops.hostops.mentions": expect.any(String),
      }),
    );
  });

  it("attaches server-local only when the user explicitly mentions @local", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@local 帮我只读检查 uname");
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command.message.hostId).toBe("server-local");
    expect(JSON.parse(command.message.metadata["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@local",
        value: "server-local",
        hostId: "server-local",
        source: "local_alias",
      }),
    ]);
  });

  it("highlights special tool mentions without sending them as host-ops metadata", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@coroot @ops_graph @ops_manus 分析环境A的A服务为什么异常");
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')).toBeNull();
    const specialMentions = Array.from(
      container.querySelectorAll('[data-testid="composer-inline-special-mention"]'),
    ).map((element) => element.textContent);
    expect(specialMentions).toEqual(["Coroot", "OpsGraph", "运维手册"]);

    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command).toEqual(
      expect.objectContaining({
        type: "add-message",
        message: expect.objectContaining({
          metadata: expect.objectContaining({
            "aiops.coroot.explicitRCA": "true",
            "aiops.opsGraph.explicitMention": "true",
            "aiops.opsManuals.explicitMention": "true",
            enableTool: "search_ops_manuals",
            enableToolPack: "opsgraph,ops_manual_flow",
          }),
        }),
      }),
    );
    expect(command.message.metadata).not.toEqual(
      expect.objectContaining({ "aiops.hostops.mentions": expect.any(String) }),
    );
  });

  it("highlights typed host mentions while keeping editable textarea text", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "这是@120.77.239.90主机,查看@10.0.0.8内存情况");

    expect(container.querySelector('[data-testid="composer-host-list"]')).toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).not.toBeNull();
    const highlightedMentions = Array.from(
      container.querySelectorAll('[data-testid="composer-inline-host-mention"]'),
    ).map((element) => element.textContent);
    expect(highlightedMentions).toEqual(["120.77.239.90", "10.0.0.8"]);
    expect(input.className).toContain("text-transparent");
    expect(input.value).toBe("这是@120.77.239.90主机,查看@10.0.0.8内存情况");
  });

  it("uses overlay-owned caret whenever host mention overlay is active", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;
    const value = "@local查看";

    await typeInComposer(input, value);

    expect(input.className).toContain("caret-transparent");
    const overlayCaret = container.querySelector('[data-testid="composer-inline-caret"]');
    expect(overlayCaret).not.toBeNull();
    expect(overlayCaret?.getAttribute("data-caret-index")).toBe(String(value.length));

    await act(async () => {
      input.setSelectionRange(6, 6);
      input.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(input.className).toContain("caret-transparent");
    expect(container.querySelector('[data-testid="composer-inline-caret"]')?.getAttribute("data-caret-index")).toBe("6");

    await act(async () => {
      input.setSelectionRange(value.length, value.length);
      input.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(input.className).toContain("caret-transparent");
    expect(container.querySelector('[data-testid="composer-inline-caret"]')?.getAttribute("data-caret-index")).toBe(String(value.length));
  });

  it("does not restore stale host mention overlay from composer state after submit", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;
    const submittedText = "@120.77.239.90 查看主机CPU情况";

    await typeInComposer(input, submittedText);
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("120.77.239.90");

    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    mockState.composerText = submittedText;
    mockState.transportRunning = true;
    await act(async () => {
      root.render(<AiopsComposer variant="chat" />);
    });
    await flushMicrotasks();

    expect(input.value).toBe("");
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')).toBeNull();
  });

  it("clears host mention overlay when submit-on-enter starts a run", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;
    const submittedText = "@120.77.239.90 查看主机CPU情况";

    await typeInComposer(input, submittedText);
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("120.77.239.90");

    mockState.composerText = submittedText;
    mockState.transportRunning = true;
    await act(async () => {
      root.render(<AiopsComposer variant="chat" />);
    });
    await flushMicrotasks();

    expect(input.value).toBe("");
    expect(mockState.composerText).toBe("");
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')).toBeNull();
  });

  it("submits pending approval decisions through the transport command", async () => {
    const state = createInitialAiopsTransportState("sess-approval");
    state.sessionId = "sess-approval";
    state.threadId = "sess-approval";
    state.status = "blocked";
    state.currentTurnId = "turn-approval";
    state.pendingApprovals = {
      "approval-1": {
        id: "approval-1",
        turnId: "turn-approval",
        status: "blocked",
        command: "systemctl restart nginx",
        risk: "high",
        source: "ai_chat_direct",
        targetSummary: "host:host-a",
      },
    };
    state.runtimeLiveness.pendingApprovals = { "approval-1": true };
    mockState.transportState = state;

    await renderComposer(root);

    expect(container.querySelector('[data-testid="codex-approval-inline"]')).not.toBeNull();
    await act(async () => {
      Array.from(container.querySelectorAll("button"))
        .find((button) => button.textContent?.trim() === "提交")
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(mockState.sendCommand).toHaveBeenCalledWith({
      type: "aiops.approval-decision",
      sessionId: "sess-approval",
      turnId: "turn-approval",
      approvalId: "approval-1",
      decision: "accept",
    });
    expect(container.textContent).toContain("已提交确认，正在继续执行");
  });

  it("does not match hostname, id, sshUser, labels, or status", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@host-ignored");
    expect(container.querySelector('[data-testid="host-mention-suggestion-empty"]')).not.toBeNull();
  });

  it("sends structured mention metadata when selecting a host suggestion", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@host-pg");
    await act(async () => {
      container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    await typeInComposer(input, `${input.value}检查状态`);
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    const structured = JSON.parse(command.message.metadata["aiops.input.mentions.v1"]);
    expect(structured).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({
          kind: "host",
          path: "host://host-a",
          source: "selection",
          rawText: "@120.77.239.90",
          payload: expect.objectContaining({
            hostId: "host-a",
            address: "120.77.239.90",
            displayName: "pg-primary",
          }),
        }),
      ],
    });
    expect(JSON.parse(command.message.metadata["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@120.77.239.90",
        hostId: "host-a",
        source: "inventory",
        resolved: true,
      }),
    ]);
    expect(command.message.hostId).toBe("host-a");
  });

  it("sends hidden host metadata when selecting server-local from the host list", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("主机"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(input.value).toBe("@host-");

    await act(async () => {
      container
        .querySelector('[data-testid="host-mention-suggestion-item"]')
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await typeInComposer(input, `${input.value}查看 CPU 情况`);
    await act(async () => {
      container
        .querySelector('[data-testid="omnibar-primary-action"]')
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command.message.hostId).toBe("server-local");
    expect(JSON.parse(command.message.metadata["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@local",
        value: "server-local",
        hostId: "server-local",
        address: "server-local",
        displayName: "server-local",
        source: "local_alias",
        resolved: true,
      }),
    ]);
  });

  it("submits typed host mention before Chinese punctuation with host metadata", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@120.77.239.90。现在只读看根分区和 inode，不要改");
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command).toEqual(
      expect.objectContaining({
        type: "add-message",
        message: expect.objectContaining({
          hostId: "host-a",
          metadata: expect.objectContaining({
            "aiops.hostops.clientDetectedMultiHost": "false",
            "aiops.hostops.mentions": expect.any(String),
          }),
        }),
      }),
    );
    expect(JSON.parse(command.message.metadata["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@120.77.239.90",
        hostId: "host-a",
        address: "120.77.239.90",
        resolved: true,
      }),
    ]);
  });

  it("restores composer input after transport fails even if assistant-ui still reports running", async () => {
    const state = createInitialAiopsTransportState("sess-failed-recovery");
    state.sessionId = "sess-failed-recovery";
    state.status = "failed";
    state.currentTurnId = "turn-failed";
    state.lastError = "模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。";
    state.turnOrder = ["turn-failed"];
    state.turns = {
      "turn-failed": {
        id: "turn-failed",
        status: "failed",
        user: {
          id: "turn-failed:user",
          text: "@120.77.239.90 查看 inode",
        },
      },
    };
    mockState.transportState = state;
    mockState.threadRunning = true;

    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    expect(input.disabled).toBe(false);
    await typeInComposer(input, "看一下 CPU");
    const sendButton = container.querySelector('[data-testid="omnibar-primary-action"]') as HTMLButtonElement;
    expect(sendButton.disabled).toBe(false);
  });

  it("does not send stale structured mention metadata after the selected token is deleted", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@host-pg");
    await act(async () => {
      container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    await typeInComposer(input, "检查状态");
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command.message.metadata).not.toHaveProperty("aiops.input.mentions.v1");
    expect(command.message.metadata).not.toHaveProperty("aiops.hostops.mentions");
    expect(command.message).not.toHaveProperty("hostId");
  });

  it("sends structured capability metadata when selecting Coroot through the monitor category", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("监控"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(input.value).toBe("@monitor-");
    expect(container.textContent).toContain("Coroot");
    await act(async () => {
      container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    await typeInComposer(input, `${input.value}分析 checkout`);
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(JSON.parse(command.message.metadata["aiops.input.mentions.v1"])).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({
          kind: "capability",
          path: "capability://coroot",
          rawText: "@Coroot",
          source: "selection",
        }),
      ],
    });
    expect(command.message.metadata).toMatchObject({
      "aiops.coroot.explicitRCA": "true",
      "aiops.coroot.rcaDisplayAllowed": "true",
    });
    expect(command.message).not.toHaveProperty("hostId");
  });

  it("opens a concrete ops manual list instead of inserting the ops manuals capability directly", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("运维手册"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(input.value).toBe("@manual-");
    expect(container.querySelector('[data-testid="host-mention-suggestion-level"]')?.textContent).toContain("选择运维手册");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.textContent).not.toContain("ops_manuals");

    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("Redis 内存压力排障"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(input.value).toBe("@manual-manual-redis-memory ");
    expect(container.querySelector('[data-testid="composer-inline-resource-mention"]')?.textContent).toBe("Redis 内存压力排障");

    await typeInComposer(input, `${input.value}按手册分析`);
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(JSON.parse(command.message.metadata["aiops.input.mentions.v1"])).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({
          kind: "ops_manual",
          path: "ops-manual://manual-redis-memory",
          rawText: "@manual-manual-redis-memory",
          source: "selection",
          payload: expect.objectContaining({
            manualId: "manual-redis-memory",
            workflowId: "workflow-redis-memory",
            title: "Redis 内存压力排障",
          }),
        }),
      ],
    });
    expect(command.message.metadata).toMatchObject({
      "aiops.opsManuals.explicitMention": "true",
      opsManualManualId: "manual-redis-memory",
      opsManualWorkflowId: "workflow-redis-memory",
      opsManualManualTitle: "Redis 内存压力排障",
      enableTool: "search_ops_manuals",
      enableToolPack: "ops_manual_flow",
    });
    expect(command.message).not.toHaveProperty("hostId");
  });

  it("opens a concrete ops graph list instead of inserting the ops graph capability directly", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("关系图谱"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(input.value).toBe("@opsgraph-");
    expect(container.querySelector('[data-testid="host-mention-suggestion-level"]')?.textContent).toContain("选择关系图谱");
    expect(container.textContent).toContain("生产服务图谱");
    expect(container.textContent).not.toContain("ops_graph");

    await act(async () => {
      Array.from(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]'))
        .find((item) => item.textContent?.includes("生产服务图谱"))
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(input.value).toBe("@opsgraph-graph.prod ");
    expect(container.querySelector('[data-testid="composer-inline-resource-mention"]')?.textContent).toBe("生产服务图谱");

    await typeInComposer(input, `${input.value}分析依赖`);
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(JSON.parse(command.message.metadata["aiops.input.mentions.v1"])).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({
          kind: "ops_graph",
          path: "ops-graph://graph.prod",
          rawText: "@opsgraph-graph.prod",
          source: "selection",
          payload: expect.objectContaining({
            graphId: "graph.prod",
            name: "生产服务图谱",
          }),
        }),
      ],
    });
    expect(command.message.metadata).toMatchObject({
      "aiops.opsGraph.explicitMention": "true",
      "aiops.opsGraph.graphId": "graph.prod",
      enableToolPack: "opsgraph",
    });
    expect(command.message).not.toHaveProperty("hostId");
  });
});

async function renderComposer(root: Root) {
  activeRoot = root;
  await act(async () => {
    root.render(<AiopsComposer variant="chat" />);
  });
  await flushMicrotasks();
}

async function typeInComposer(input: HTMLTextAreaElement, value: string) {
  await act(async () => {
    input.value = value;
    mockState.composerText = value;
    input.setSelectionRange(value.length, value.length);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keyup", { key: value.at(-1) || "", bubbles: true }));
  });
  if (activeRoot) {
    await act(async () => {
      activeRoot?.render(<AiopsComposer variant="chat" />);
    });
  }
  mockState.composerText = value;
  await flushMicrotasks();
}

async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    if (typeof window.requestAnimationFrame === "function") {
      await new Promise((resolve) => window.requestAnimationFrame(() => resolve(undefined)));
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function sampleHosts() {
  return [
    { id: "server-local", name: "server-local", address: "server-local", status: "online" },
    { id: "host-a", name: "pg-primary", ip: "120.77.239.90", status: "online", hostname: "ignored-hostname", sshUser: "ignored-user", labels: { role: "ignored" } },
    { id: "host-b", name: "redis", ip: "10.0.0.8", status: "online" },
    { id: "host-c", name: "api", address: "10.0.0.9", status: "offline" },
  ];
}

function sampleOpsManuals() {
  return [
    {
      id: "manual-redis-memory",
      title: "Redis 内存压力排障",
      status: "verified",
      workflow_ref: { workflow_id: "workflow-redis-memory" },
      operation: { target_type: "redis", action: "diagnose" },
      applicability: { middleware: "redis", os: ["linux"], platform: ["ecs"], execution_surface: ["ssh"] },
      run_record_summary: {},
    },
    {
      id: "manual-pg-backup",
      title: "PostgreSQL 备份恢复",
      status: "verified",
      workflow_ref: { workflow_id: "workflow-pg-backup" },
      operation: { target_type: "postgresql", action: "restore" },
      applicability: { middleware: "postgresql" },
      run_record_summary: {},
    },
  ];
}

function sampleOpsGraphs() {
  return [
    {
      id: "graph.prod",
      name: "生产服务图谱",
      environment: "prod",
      isDefault: true,
      nodeCount: 12,
      relationshipCount: 18,
      issueCount: 0,
    },
    {
      id: "graph.eval",
      name: "演练图谱",
      environment: "eval",
      isDefault: false,
      nodeCount: 3,
      relationshipCount: 2,
      issueCount: 1,
    },
  ];
}
