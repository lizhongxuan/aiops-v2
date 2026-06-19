import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AiopsComposer } from "./AiopsComposer";

const mockState = vi.hoisted(() => ({
  sendCommand: vi.fn(),
  composerText: "",
}));

vi.mock("@assistant-ui/react", () => ({
  ComposerPrimitive: {
    Root: ({ children, ...props }: any) => <div {...props}>{children}</div>,
    Input: ({ children }: any) => children,
  },
  useAssistantApi: () => ({ thread: () => ({ cancelRun: vi.fn() }) }),
  useAssistantTransportSendCommand: () => mockState.sendCommand,
  useAssistantTransportState: () => ({ pendingApprovals: {}, activeCommandStreams: {}, runtimeLiveness: {}, currentTurnId: "" }),
  useComposer: () => ({ text: mockState.composerText }),
  useComposerRuntime: () => ({
    getState: () => ({ text: mockState.composerText }),
    setText: (value: string) => {
      mockState.composerText = value;
    },
  }),
  useThread: () => false,
}));

vi.mock("@/transport/aiopsTransportConverter", () => ({ isAiopsTransportRunning: () => false }));
vi.mock("@/transport/useAiopsTransportCommands", () => ({ useAiopsTransportCommands: () => ({ stop: vi.fn(), submitApproval: vi.fn() }) }));
vi.mock("./SessionTargetContext", () => ({ useSessionTargetContext: () => ({ metadata: {}, hostId: "" }) }));
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

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  it("opens suggestions for @, filters by name/ip, and inserts selected host", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@");
    expect(container.querySelector('[data-testid="host-mention-suggestion-popover"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')).toHaveLength(3);

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
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(input.value).toBe("@10.0.0.8 ");

    await typeInComposer(input, `${input.value}检查状态`);
    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
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

  it("does not send unresolved tool mentions as host-ops metadata", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@coroot 分析环境A的A服务为什么异常");
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')).toBeNull();

    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command).toEqual(
      expect.objectContaining({
        type: "add-message",
        message: expect.objectContaining({
          metadata: expect.not.objectContaining({ "aiops.hostops.mentions": expect.any(String) }),
        }),
      }),
    );
  });

  it("renders resolved host mentions inline inside the chat input without a separate selected-host row", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "这是@120.77.239.90主机,查看@10.0.0.8内存情况");

    expect(container.querySelector('[data-testid="composer-host-list"]')).toBeNull();
    const overlay = container.querySelector('[data-testid="composer-inline-host-overlay"]') as HTMLDivElement | null;
    expect(overlay?.textContent).toContain("这是@120.77.239.90主机,查看@10.0.0.8内存情况");

    const inlineMentions = Array.from(container.querySelectorAll('[data-testid="composer-inline-host-mention"]'));
    expect(inlineMentions.map((element) => element.textContent)).toEqual(["@120.77.239.90", "@10.0.0.8"]);
    expect(inlineMentions[0]?.className).toContain("bg-");
    expect(input.value).toBe("这是@120.77.239.90主机,查看@10.0.0.8内存情况");

    await act(async () => {
      container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
    expect(command).toEqual(
      expect.objectContaining({
        type: "add-message",
        message: expect.objectContaining({
          metadata: expect.objectContaining({ "aiops.hostops.clientDetectedMultiHost": "true" }),
        }),
      }),
    );
    expect(input.value).toBe("");
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).toBeNull();
  });

  it("does not match hostname, id, sshUser, labels, or status", async () => {
    await renderComposer(root);
    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

    await typeInComposer(input, "@ignored");
    expect(container.querySelector('[data-testid="host-mention-suggestion-empty"]')).not.toBeNull();
  });
});

async function renderComposer(root: Root) {
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
  await flushMicrotasks();
}

async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

function sampleHosts() {
  return [
    { id: "host-a", name: "pg-primary", ip: "120.77.239.90", status: "online", hostname: "ignored-hostname", sshUser: "ignored-user", labels: { role: "ignored" } },
    { id: "host-b", name: "redis", ip: "10.0.0.8", status: "online" },
    { id: "host-c", name: "api", address: "10.0.0.9", status: "offline" },
  ];
}
