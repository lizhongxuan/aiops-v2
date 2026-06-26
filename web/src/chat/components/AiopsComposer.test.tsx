import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AiopsComposer } from "./AiopsComposer";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

const mockState = vi.hoisted(() => ({
  sendCommand: vi.fn(),
  composerText: "",
  transportRunning: false,
  transportState: undefined as AiopsTransportState | undefined,
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
  useThread: () => false,
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
  useSessionWorkspaceContext: () => ({ composerFocusNonce: 0, composerDisabledReason: "", llmLabel: "GPT-5.4" }),
}));

describe("AiopsComposer host mention fuzzy search", () => {
  let container: HTMLDivElement;
  let root: Root;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    mockState.composerText = "";
    mockState.transportRunning = false;
    mockState.transportState = undefined;
    mockState.sendCommand.mockReset();
    fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/api/v1/hosts")) {
        return new Response(JSON.stringify({ items: sampleHosts() }), { status: 200, headers: { "Content-Type": "application/json" } });
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

  it("opens suggestions for @, filters by name/ip, and inserts selected host", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    expect(container.querySelector('[data-testid="host-mention-suggestion-popover"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')).toHaveLength(4);
    expect(container.querySelector('[data-testid="host-mention-suggestion-item"]')?.textContent).toContain("@local");

    await typeInComposer(input, "@pg");
    expect(container.textContent).toContain("@pg-primary");
    expect(container.textContent).not.toContain("@redis");

    await act(async () => {
      container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(input.value).toBe("@120.77.239.90 ");
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

  it("uses keyboard navigation and keeps send behavior after insertion", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
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
    expect(specialMentions).toEqual(["@coroot", "@ops_graph", "@ops_manus"]);

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
    expect(highlightedMentions).toEqual(["@120.77.239.90", "@10.0.0.8"]);
    expect(input.className).toContain("text-transparent");
    expect(input.value).toBe("这是@120.77.239.90主机,查看@10.0.0.8内存情况");
  });

  it("does not restore stale host mention overlay from composer state after submit", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;
    const submittedText = "@120.77.239.90 查看主机CPU情况";

    await typeInComposer(input, submittedText);
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("@120.77.239.90");

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
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("@120.77.239.90");

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

    await typeInComposer(input, "@ignored");
    expect(container.querySelector('[data-testid="host-mention-suggestion-empty"]')).not.toBeNull();
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
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function sampleHosts() {
  return [
    { id: "host-a", name: "pg-primary", ip: "120.77.239.90", status: "online", hostname: "ignored-hostname", sshUser: "ignored-user", labels: { role: "ignored" } },
    { id: "host-b", name: "redis", ip: "10.0.0.8", status: "online" },
    { id: "host-c", name: "api", address: "10.0.0.9", status: "offline" },
  ];
}
