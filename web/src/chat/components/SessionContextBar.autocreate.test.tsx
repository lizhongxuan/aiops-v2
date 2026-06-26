import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createSession,
  fetchHosts,
  fetchLlmConfig,
  fetchSessions,
  selectHost,
} from "@/pages/settingsApi";
import { fetchAssistantTransportResumeState } from "@/transport/assistantTransportControl";

import { AppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";
import { SESSION_CONTEXT_TIMEOUT_MS, SessionContextBar } from "./SessionContextBar";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

vi.mock("@/pages/settingsApi", () => ({
  activateSession: vi.fn(),
  createSession: vi.fn(),
  fetchHosts: vi.fn(),
  fetchLlmConfig: vi.fn(),
  fetchSessions: vi.fn(),
  selectHost: vi.fn(),
}));

vi.mock("@/transport/assistantTransportControl", () => ({
  fetchAssistantTransportResumeState: vi.fn(),
}));

describe("SessionContextBar auto-create", () => {
  let container: HTMLDivElement;
  let root: Root;
  const onThreadChange = vi.fn();

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    onThreadChange.mockReset();
    vi.mocked(fetchSessions).mockResolvedValue({ activeSessionId: "", sessions: [] });
    vi.mocked(fetchHosts).mockResolvedValue({ items: [] });
    vi.mocked(fetchLlmConfig).mockResolvedValue({
      provider: "openai",
      model: "gpt-5.4",
      apiKeySet: true,
    });
    vi.mocked(createSession).mockResolvedValue({
      activeSessionId: "session-auto",
      sessions: [
        {
          id: "session-auto",
          kind: "single_host",
          title: "AI 对话",
          selectedHostId: "",
          status: "empty",
          messageCount: 0,
        },
      ],
    });
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("creates a usable single-host session when entering chat with no prior sessions", async () => {
    await act(async () => {
      root.render(
        <SessionContextBar
          activeThreadId="default"
          description="AI Chat"
          kind="single_host"
          newSessionLabel="新建会话"
          onThreadChange={onThreadChange}
          title="单机会话"
        >
          <ContextProbe />
        </SessionContextBar>,
      );
    });

    await act(async () => {
      await flushMicrotasks();
    });

    expect(createSession).toHaveBeenCalledWith("single_host", undefined);
    expect(onThreadChange).toHaveBeenCalledWith(
      "session-auto",
      expect.objectContaining({
        sessionId: "session-auto",
        threadId: "session-auto",
      }),
      false,
    );
    expect(container.textContent).toContain("active=session-auto");
    expect(container.textContent).toContain("reason=");
    expect(container.textContent).not.toContain("请先创建会话");
  });

  it("does not require manual session creation when an initial chat state is already active", async () => {
    await act(async () => {
      root.render(
        <SessionContextBar
          activeThreadId="fixture-session"
          description="AI Chat"
          kind="single_host"
          newSessionLabel="新建会话"
          onThreadChange={onThreadChange}
          skipInitialLoad
          title="单机会话"
        >
          <ContextProbe />
        </SessionContextBar>,
      );
    });

    expect(fetchSessions).not.toHaveBeenCalled();
    expect(createSession).not.toHaveBeenCalled();
    expect(container.textContent).toContain("active=fixture-session");
    expect(container.textContent).toContain("reason=");
    expect(container.textContent).not.toContain("请先创建会话");
  });

  it("does not remount the same active session with an empty state after loading session context", async () => {
    vi.mocked(fetchSessions).mockResolvedValue({
      activeSessionId: "session-existing",
      sessions: [
        {
          id: "session-existing",
          kind: "single_host",
          title: "Existing chat",
          selectedHostId: "server-local",
          status: "working",
          messageCount: 1,
        },
      ],
    });
    vi.mocked(fetchAssistantTransportResumeState).mockResolvedValue(null);

    await act(async () => {
      root.render(
        <SessionContextBar
          activeThreadId="session-existing"
          description="AI Chat"
          kind="single_host"
          newSessionLabel="新建会话"
          onThreadChange={onThreadChange}
          title="单机会话"
        >
          <ContextProbe />
        </SessionContextBar>,
      );
    });

    await act(async () => {
      await flushMicrotasks();
    });

    expect(onThreadChange).not.toHaveBeenCalled();
    expect(container.textContent).toContain("active=session-existing");
    expect(container.textContent).toContain("target=none");
    expect(container.textContent).toContain("未选择执行目标");
    expect(container.textContent).not.toContain("target=host:server-local");
  });

  it("does not keep the composer disabled when terminal session resume times out", async () => {
    vi.useFakeTimers();
    vi.mocked(fetchSessions).mockResolvedValue({
      activeSessionId: "session-terminal",
      sessions: [
        {
          id: "session-terminal",
          kind: "single_host",
          title: "Terminal chat",
          selectedHostId: "",
          status: "completed",
          messageCount: 1,
        },
      ],
    });
    vi.mocked(fetchAssistantTransportResumeState).mockReturnValue(new Promise(() => {}));

    await act(async () => {
      root.render(
        <SessionContextBar
          activeThreadId="default"
          description="AI Chat"
          kind="single_host"
          newSessionLabel="新建会话"
          onThreadChange={onThreadChange}
          title="单机会话"
        >
          <ContextProbe />
        </SessionContextBar>,
      );
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(SESSION_CONTEXT_TIMEOUT_MS);
      await flushMicrotasks();
    });

    expect(fetchAssistantTransportResumeState).toHaveBeenCalledWith("session-terminal");
    expect(container.textContent).toContain("active=session-terminal");
    expect(container.textContent).toContain("reason=;");
    expect(container.textContent).toContain("busy=false");
    expect(container.textContent).toContain("llm=gpt-5.4");
  });

  it("does not register a current-host selector in single-host chat header actions", async () => {
    vi.mocked(fetchSessions).mockResolvedValue({
      activeSessionId: "session-existing",
      sessions: [
        {
          id: "session-existing",
          kind: "single_host",
          title: "Existing chat",
          selectedHostId: "server-local",
          status: "working",
          messageCount: 1,
        },
      ],
    });
    vi.mocked(fetchHosts).mockResolvedValue({
      items: [
        {
          id: "server-local",
          name: "server-local",
          address: "local",
          status: "online",
          terminalCapable: true,
          labels: {},
        },
      ],
    });
    vi.mocked(fetchAssistantTransportResumeState).mockResolvedValue(null);

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <SessionContextBar
            activeThreadId="session-existing"
            description="AI Chat"
            kind="single_host"
            newSessionLabel="新建会话"
            onThreadChange={onThreadChange}
            title="单机会话"
          >
            <ContextProbe />
          </SessionContextBar>
          <HeaderActionsProbe />
        </AppShellChromeProvider>,
      );
    });

    await act(async () => {
      await flushMicrotasks();
    });

    const text = container.textContent || "";
    expect(text).toContain("新建会话");
    expect(text).not.toContain("主机");
    expect(text).not.toContain("server-local");
  });

  it("creates a new AI chat without inheriting the previous selected host", async () => {
    vi.mocked(fetchSessions).mockResolvedValue({
      activeSessionId: "session-existing",
      sessions: [
        {
          id: "session-existing",
          kind: "single_host",
          title: "Existing chat",
          selectedHostId: "server-local",
          status: "completed",
          messageCount: 1,
        },
      ],
    });
    vi.mocked(fetchHosts).mockResolvedValue({
      items: [
        {
          id: "server-local",
          name: "server-local",
          address: "local",
          status: "online",
          terminalCapable: true,
          labels: {},
        },
      ],
    });
    vi.mocked(createSession).mockResolvedValue({
      activeSessionId: "session-new",
      sessions: [
        {
          id: "session-existing",
          kind: "single_host",
          title: "Existing chat",
          selectedHostId: "server-local",
          status: "completed",
          messageCount: 1,
        },
        {
          id: "session-new",
          kind: "single_host",
          title: "AI 对话",
          selectedHostId: "",
          status: "empty",
          messageCount: 0,
        },
      ],
    });

    await act(async () => {
      root.render(
        <SessionContextBar
          activeThreadId="session-existing"
          description="AI Chat"
          kind="single_host"
          newSessionLabel="新建会话"
          onThreadChange={onThreadChange}
          title="单机会话"
        >
          <ContextProbe />
        </SessionContextBar>,
      );
    });
    await act(async () => {
      await flushMicrotasks();
    });
    const button = container.querySelector('[data-testid="create-session"]');
    if (!button) {
      throw new Error("missing new session button");
    }

    await act(async () => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await flushMicrotasks();
    });

    expect(createSession).toHaveBeenLastCalledWith("single_host", undefined);
    expect(selectHost).not.toHaveBeenCalled();
    expect(container.textContent).toContain("未选择执行目标");
    expect(container.textContent).toContain("target=none");
  });
});

function ContextProbe() {
  const context = useSessionWorkspaceContext();
  const target = useSessionTargetContext();
  return (
    <div data-testid="session-context-probe">
      active={context.activeSessionId}; reason={context.composerDisabledReason}; busy={String(context.busy)}; llm=
      {context.llmLabel}; target={target.targetValue}; label={target.targetLabel}
      <button type="button" data-testid="create-session" onClick={context.createSession}>
        new
      </button>
    </div>
  );
}

function HeaderActionsProbe() {
  const { headerActions } = useAppShellChrome();
  return <div data-testid="header-actions-probe">{headerActions}</div>;
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}
