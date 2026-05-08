import { describe, expect, it } from "vitest";

import type { AssistantTransportConnectionMetadata } from "@assistant-ui/react";

import {
  createAiopsTransportConverter,
  isAiopsTransportRunning,
} from "./aiopsTransportConverter";
import type { AiopsTransportState } from "./aiopsTransportTypes";

function metadata(overrides = {}): AssistantTransportConnectionMetadata {
  return {
    pendingCommands: [],
    isSending: false,
    toolStatuses: {},
    ...overrides,
  };
}

function createState(): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "sess-1",
    threadId: "thread-1",
    status: "idle",
    currentTurnId: "turn-1",
    turns: {
      "turn-1": {
        id: "turn-1",
        status: "completed",
        startedAt: "2026-05-06T00:00:00Z",
        completedAt: "2026-05-06T00:00:05Z",
        user: {
          id: "user-1",
          text: "Investigate payment-api saturation",
          createdAt: "2026-05-06T00:00:00Z",
        },
        blockOrder: ["text-1", "cmd-1", "text-2"],
        blocksById: {
          "text-1": {
            id: "text-1",
            type: "text",
            text: { role: "assistant", text: "I will check the service first.", status: "completed" },
          },
          "cmd-1": {
            id: "cmd-1",
            type: "tool",
            tool: {
              toolKind: "command",
              title: "Shell",
              summary: "已运行 systemctl status payment-api",
              status: "completed",
              command: "systemctl status payment-api",
              output: { stdout: "", stderr: "", text: "", truncated: false },
            },
          },
          "text-2": {
            id: "text-2",
            type: "text",
            text: { role: "assistant", text: "payment-api is healthy after restart.", status: "completed" },
          },
        },
      },
    },
    turnOrder: ["turn-1"],
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
    seq: 3,
    updatedAt: "2026-05-06T00:00:05Z",
  };
}

describe("aiopsTransportConverter", () => {
  it("maps ordered turns into user and assistant messages without parsing markdown", () => {
    const state = createState();
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const assistant = result.messages[1];
    const aiops = assistant?.metadata?.custom?.aiops as {
      blocks: { id: string }[];
      blockOrder: string[];
      blocksById: Record<string, { id: string }>;
    } | undefined;

    expect(result.messages).toHaveLength(2);
    expect(result.messages[0]).toMatchObject({
      role: "user",
      id: "user-1",
      content: [{ type: "text", text: "Investigate payment-api saturation" }],
    });
    expect(assistant).toMatchObject({
      role: "assistant",
      id: "turn-1:assistant",
      content: [],
      status: { type: "complete", reason: "stop" },
    });
    expect(assistant?.metadata?.custom).toMatchObject({
      source: "aiops.transport.assistant",
      aiops: {
        turnId: "turn-1",
        turnStatus: "completed",
        turnStartedAt: "2026-05-06T00:00:00Z",
        turnCompletedAt: "2026-05-06T00:00:05Z",
      },
    });
    expect(aiops?.blocks.map((block) => block.id)).toEqual(["text-1", "cmd-1", "text-2"]);
    expect(aiops?.blockOrder).toEqual(["text-1", "cmd-1", "text-2"]);
    expect(Object.keys(aiops?.blocksById || {})).toEqual(["text-1", "cmd-1", "text-2"]);
    expect(assistant?.content).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ text: "systemctl status payment-api" })]),
    );
  });

  it("passes full blocksById so aggregate details can resolve child blocks", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      blockOrder: ["agg-1", "text-2"],
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
        "cmd-1": {
          id: "cmd-1",
          type: "tool",
          tool: {
            toolKind: "command",
            title: "Shell",
            summary: "已运行 pwd",
            status: "completed",
            command: "pwd",
            output: { stdout: "/tmp\n", stderr: "", text: "/tmp\n", truncated: false },
          },
        },
        "cmd-2": {
          id: "cmd-2",
          type: "tool",
          tool: {
            toolKind: "command",
            title: "Shell",
            summary: "已运行 whoami",
            status: "completed",
            command: "whoami",
            output: { stdout: "aiops\n", stderr: "", text: "aiops\n", truncated: false },
          },
        },
        "text-2": {
          id: "text-2",
          type: "text",
          text: { role: "assistant", text: "检查完成。", status: "completed" },
        },
      },
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const aiops = result.messages[1]?.metadata?.custom?.aiops as {
      blocks: Array<{ id: string }>;
      blockOrder: string[];
      blocksById: Record<string, { id: string }>;
    };

    expect(aiops.blocks.map((block) => block.id)).toEqual(["agg-1", "text-2"]);
    expect(aiops.blockOrder).toEqual(["agg-1", "text-2"]);
    expect(Object.keys(aiops.blocksById)).toEqual(["agg-1", "cmd-1", "cmd-2", "text-2"]);
  });

  it("keeps assistant message id stable while transcript text streams in", () => {
    const state = createState();
    const converter = createAiopsTransportConverter();
    const runningState: AiopsTransportState = {
      ...state,
      status: "working",
      turns: {
        ...state.turns,
        "turn-1": {
          ...state.turns["turn-1"],
          status: "working",
          blockOrder: [],
          blocksById: {},
        },
      },
    };
    const streamingState: AiopsTransportState = {
      ...runningState,
      turns: {
        ...runningState.turns,
        "turn-1": {
          ...runningState.turns["turn-1"],
          blockOrder: ["text-1"],
          blocksById: {
            "text-1": {
              id: "text-1",
              type: "text",
              text: { role: "assistant", text: "partial", status: "streaming" },
            },
          },
        },
      },
    };

    const before = converter(runningState, metadata());
    const after = converter(streamingState, metadata());
    const aiops = after.messages[1]?.metadata?.custom?.aiops as { blocks: Array<{ text?: { text: string } }> };

    expect(before.messages[1]?.id).toBe("turn-1:assistant");
    expect(after.messages[1]?.id).toBe("turn-1:assistant");
    expect(after.messages[1]?.content).toEqual([]);
    expect(aiops.blocks[0]?.text?.text).toBe("partial");
  });

  it("adds optimistic pending add-message commands without mutating source state", () => {
    const state = createState();
    const converter = createAiopsTransportConverter();

    const result = converter(
      state,
      metadata({
        pendingCommands: [
          {
            type: "add-message",
            message: {
              role: "user",
              parts: [{ type: "text", text: "Check recent deploy logs" }],
            },
          },
        ],
        isSending: true,
      }),
    );

    expect(result.messages.at(-1)).toMatchObject({
      role: "user",
      content: [{ type: "text", text: "Check recent deploy logs" }],
    });
    expect(state.turnOrder).toEqual(["turn-1"]);
    expect(result.isRunning).toBe(true);
  });

  it("renders a running assistant placeholder for submitted turns without blocks yet", () => {
    const state = createState();
    state.status = "working";
    state.turns["turn-1"] = {
      id: "turn-1",
      status: "submitted",
      startedAt: "2026-05-06T00:00:00Z",
      user: {
        id: "user-1",
        text: "看A股行情",
        createdAt: "2026-05-06T00:00:00Z",
      },
      blockOrder: [],
      blocksById: {},
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());

    expect(result.messages).toHaveLength(2);
    expect(result.messages[0]).toMatchObject({
      role: "user",
      content: [{ type: "text", text: "看A股行情" }],
    });
    expect(result.messages[1]).toMatchObject({
      role: "assistant",
      content: [],
      status: { type: "running" },
      metadata: {
        custom: {
          aiops: {
            turnId: "turn-1",
            turnStatus: "submitted",
            blocks: [],
          },
        },
      },
    });
  });

  it("puts failed turn errors into transcript block metadata instead of final text", () => {
    const state = createState();
    state.status = "failed";
    state.lastError = "backend unavailable";
    state.turns["turn-1"] = {
      id: "turn-1",
      status: "failed",
      startedAt: "2026-05-06T00:00:00Z",
      user: {
        id: "user-1",
        text: "检查服务",
        createdAt: "2026-05-06T00:00:00Z",
      },
      blockOrder: [],
      blocksById: {},
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const assistant = result.messages[1];
    const aiops = assistant?.metadata?.custom?.aiops as {
      blocks: Array<{ id: string; type: string; text?: { text: string } }>;
      blockOrder: string[];
      blocksById: Record<string, { id: string; type: string; text?: { text: string } }>;
    };

    expect(assistant?.content).toEqual([]);
    expect(aiops.blocks).toEqual([
      expect.objectContaining({
        type: "text",
        text: expect.objectContaining({ text: "backend unavailable", status: "completed" }),
      }),
    ]);
    expect(aiops.blockOrder).toEqual(["turn-1:error"]);
    expect(aiops.blocksById["turn-1:error"]?.text?.text).toBe("backend unavailable");
  });

  it("treats working and blocked transport states as running", () => {
    const state = createState();

    expect(isAiopsTransportRunning(state)).toBe(false);
    expect(isAiopsTransportRunning({ ...state, status: "working" })).toBe(true);
    expect(
      isAiopsTransportRunning({
        ...state,
        status: "blocked",
        runtimeLiveness: {
          ...state.runtimeLiveness,
          pendingApprovals: { "approval-1": true },
        },
      }),
    ).toBe(true);
  });
});
