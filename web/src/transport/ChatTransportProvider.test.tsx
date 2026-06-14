import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const resumeRunSpy = vi.fn();
const transportRuntimeSpy = vi.fn(() => ({}));

vi.mock("@assistant-ui/react", () => ({
  AssistantRuntimeProvider: ({ children }: { children: React.ReactNode }) => children,
  useAssistantApi: () => ({
    thread: () => ({
      unstable_resumeRun: resumeRunSpy,
    }),
  }),
  useAssistantState: (selector: (snapshot: { thread: { extras: { state: Record<string, never> } } }) => boolean) =>
    selector({ thread: { extras: { state: {} } } }),
  useAssistantTransportRuntime: (options: unknown) => {
    transportRuntimeSpy(options);
    return {};
  },
}));

import { ChatTransportProvider } from "./ChatTransportProvider";
import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";

describe("ChatTransportProvider", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    resumeRunSpy.mockReset();
    transportRuntimeSpy.mockClear();
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("does not auto-resume a brand-new empty session", async () => {
    const initialState = createInitialAiopsTransportState("thread-empty");
    initialState.sessionId = "sess-empty";

    await act(async () => {
      root.render(
        <ChatTransportProvider autoResume={false} initialState={initialState} threadId="thread-empty">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    expect(resumeRunSpy).not.toHaveBeenCalled();
  });

  it("resumes an existing session when explicitly requested", async () => {
    const initialState = createInitialAiopsTransportState("thread-existing");
    initialState.sessionId = "sess-existing";

    await act(async () => {
      root.render(
        <ChatTransportProvider autoResume initialState={initialState} threadId="thread-existing">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    expect(resumeRunSpy).toHaveBeenCalledTimes(1);
    expect(resumeRunSpy).toHaveBeenCalledWith({});
  });

  it("normalizes legacy initial state before creating the assistant transport runtime", async () => {
    const legacyState = createInitialAiopsTransportState("thread-legacy") as Partial<ReturnType<typeof createInitialAiopsTransportState>>;
    delete legacyState.hostMissions;
    delete legacyState.childAgents;
    delete legacyState.pendingApprovals;
    delete legacyState.mcpSurfaces;
    delete legacyState.artifacts;
    delete legacyState.runtimeLiveness;

    await act(async () => {
      root.render(
        <ChatTransportProvider initialState={legacyState as ReturnType<typeof createInitialAiopsTransportState>} threadId="thread-legacy">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    expect(transportRuntimeSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        initialState: expect.objectContaining({
          hostMissions: {},
          childAgents: {},
          pendingApprovals: {},
          mcpSurfaces: {},
          artifacts: {},
          runtimeLiveness: {
            activeTurns: {},
            activeAgents: {},
            pendingApprovals: {},
            pendingUserInputs: {},
            activeCommandStreams: {},
          },
        }),
      }),
    );
  });
});
