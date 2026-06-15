import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createSession,
  fetchHosts,
  fetchLlmConfig,
  fetchSessions,
} from "@/pages/settingsApi";
import { fetchAssistantTransportResumeState } from "@/transport/assistantTransportControl";

import { SessionContextBar } from "./SessionContextBar";
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
          selectedHostId: "server-local",
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

    expect(createSession).toHaveBeenCalledWith("single_host", "server-local");
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
  });
});

function ContextProbe() {
  const context = useSessionWorkspaceContext();
  return (
    <div data-testid="session-context-probe">
      active={context.activeSessionId}; reason={context.composerDisabledReason}
    </div>
  );
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}
