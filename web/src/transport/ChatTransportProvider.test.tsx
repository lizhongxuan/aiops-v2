import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const resumeRunSpy = vi.fn();
const transportRuntimeSpy = vi.fn(() => ({}));
let transportStateForHook: unknown = undefined;

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
  useAssistantTransportState: () => transportStateForHook,
}));

import { ChatTransportProvider } from "./ChatTransportProvider";
import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";
import {
  getCachedAiopsTransportState,
  resetAiopsTransportStateCacheForTest,
} from "./aiopsTransportStateCache";
import type { AiopsTransportState } from "./aiopsTransportTypes";

type TransportRuntimeOptions = {
  onError: (
    error: Error,
    context: {
      updateState: (updater: (state: AiopsTransportState) => AiopsTransportState) => void;
    },
  ) => void;
};

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
    transportStateForHook = undefined;
    resetAiopsTransportStateCacheForTest();
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

  it("drops incomplete initial state instead of migrating old transport data", async () => {
    const staleState = createInitialAiopsTransportState("thread-stale") as Partial<ReturnType<typeof createInitialAiopsTransportState>>;
    delete staleState.hostMissions;
    delete staleState.childAgents;
    delete staleState.pendingApprovals;
    delete staleState.mcpSurfaces;
    delete staleState.artifacts;
    delete staleState.runtimeLiveness;

    await act(async () => {
      root.render(
        <ChatTransportProvider initialState={staleState as ReturnType<typeof createInitialAiopsTransportState>} threadId="thread-current">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    expect(transportRuntimeSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        initialState: expect.objectContaining({
          sessionId: "",
          threadId: "thread-current",
          turns: {},
          turnOrder: [],
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

  it("writes transport state updates to the in-memory cache", async () => {
    const state = createInitialAiopsTransportState("thread-cache-provider");
    state.sessionId = "sess-cache-provider";
    state.turnOrder = ["turn-1"];
    transportStateForHook = state;

    await act(async () => {
      root.render(
        <ChatTransportProvider cacheScope="single_host" initialState={state} threadId="thread-cache-provider">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    expect(getCachedAiopsTransportState("single_host")).toMatchObject({
      sessionId: "sess-cache-provider",
      threadId: "thread-cache-provider",
      turnOrder: ["turn-1"],
    });
  });

  it("localizes network errors and clears model wait state when the transport fails", async () => {
    const state = createInitialAiopsTransportState("thread-error-provider");
    state.sessionId = "sess-error-provider";
    state.status = "working";
    state.currentTurnId = "turn-error";
    state.turnOrder = ["turn-error"];
    state.turns = {
      "turn-error": {
        id: "turn-error",
        status: "working",
        process: [
          {
            id: "wait-model",
            kind: "reasoning",
            status: "running",
            text: "正在等待模型返回",
          },
        ],
      },
    };

    await act(async () => {
      root.render(
        <ChatTransportProvider initialState={state} threadId="thread-error-provider">
          <div>chat</div>
        </ChatTransportProvider>,
      );
    });

    const options = transportRuntimeSpy.mock.calls[0]?.[0] as TransportRuntimeOptions;
    let nextState = state;
    options.onError(new Error("network error"), {
      updateState(updater) {
        nextState = updater(nextState);
      },
    });

    expect(nextState.lastError).toBe("网络异常,请检查后重试");
    expect(nextState.turns["turn-error"]?.process?.[0]).toMatchObject({
      id: "wait-model",
      status: "failed",
      text: "模型调用失败",
    });
  });
});
