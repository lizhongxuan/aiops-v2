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
    schemaVersion: "aiops.transport.v1",
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
        process: [
          {
            id: "block-1",
            kind: "command",
            status: "completed",
            text: "systemctl status payment-api",
            command: "systemctl status payment-api",
            updatedAt: "2026-05-06T00:00:03Z",
          },
        ],
        final: {
          id: "final-1",
          text: "payment-api is healthy after restart.",
          status: "completed",
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
  it("maps ordered turns into assistant-ui thread messages without parsing markdown", () => {
    const state = createState();
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());

    expect(result.messages).toHaveLength(2);
    expect(result.messages[0]).toMatchObject({
      role: "user",
      id: "user-1",
      content: [{ type: "text", text: "Investigate payment-api saturation" }],
    });
    expect(result.messages[1]).toMatchObject({
      role: "assistant",
      id: "turn-1:assistant",
      content: [{ type: "text", text: "payment-api is healthy after restart." }],
      status: { type: "complete", reason: "stop" },
    });
    expect(result.messages[1]?.metadata?.unstable_state).toMatchObject({
      turnId: "turn-1",
      turnStatus: "completed",
      turnStartedAt: "2026-05-06T00:00:00Z",
      turnCompletedAt: "2026-05-06T00:00:05Z",
      process: [
        expect.objectContaining({
          id: "block-1",
          kind: "command",
          command: "systemctl status payment-api",
        }),
      ],
    });
    expect(result.messages[1]?.content).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ text: "systemctl status payment-api" })]),
    );
  });

  it("keeps assistant message id stable while final text streams in", () => {
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
          final: undefined,
        },
      },
    };
    const streamingState: AiopsTransportState = {
      ...runningState,
      turns: {
        ...runningState.turns,
        "turn-1": {
          ...runningState.turns["turn-1"],
          final: { id: "final-1", text: "partial", status: "running" },
        },
      },
    };

    const before = converter(runningState, metadata());
    const after = converter(streamingState, metadata());

    expect(before.messages[1]?.id).toBe("turn-1:assistant");
    expect(after.messages[1]?.id).toBe("turn-1:assistant");
    expect(after.messages[1]?.content).toEqual([{ type: "text", text: "partial" }]);
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

  it("renders a running assistant placeholder for submitted turns without process yet", () => {
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
      status: { type: "running" },
      metadata: {
        unstable_state: {
          turnId: "turn-1",
          turnStatus: "submitted",
          process: [],
        },
      },
    });
  });

  it("attaches turn Agent-to-UI artifacts to assistant message metadata", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-coroot-latency",
          type: "coroot_chart",
          titleZh: "Coroot 延迟趋势",
          summaryZh: "接口 P95 延迟在 14:03 后升高。",
          caseId: "case-debug-1",
          source: "coroot",
          redactionStatus: "redacted",
          createdAt: "2026-05-12T02:00:00Z",
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());

    expect(result.messages[1]?.metadata?.unstable_state).toMatchObject({
      agentUiArtifacts: [
        expect.objectContaining({
          id: "artifact-coroot-latency",
          type: "coroot_chart",
          titleZh: "Coroot 延迟趋势",
          caseId: "case-debug-1",
        }),
      ],
    });
  });

  it("delays ops manual search artifacts until the assistant turn is terminal", () => {
    const state = createState();
    state.status = "working";
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      status: "working",
      final: { id: "final-1", text: "我先查一下手册。", status: "running" },
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual",
          type: "ops_manual_search_result",
          titleZh: "运维手册检索",
        },
        {
          id: "artifact-coroot-latency",
          type: "coroot_chart",
          titleZh: "Coroot 延迟趋势",
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());

    expect(result.messages[1]?.metadata?.unstable_state).toMatchObject({
      agentUiArtifacts: [
        expect.objectContaining({
          id: "artifact-coroot-latency",
          type: "coroot_chart",
        }),
      ],
    });
    expect(result.messages[1]?.metadata?.unstable_state?.agentUiArtifacts).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ type: "ops_manual_search_result" })]),
    );
  });

  it("shows delayed ops manual search artifacts after the assistant turn completes", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual",
          type: "ops_manual_search_result",
          titleZh: "运维手册检索",
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());

    expect(result.messages[1]?.metadata?.unstable_state).toMatchObject({
      agentUiArtifacts: [
        expect.objectContaining({
          id: "artifact-ops-manual",
          type: "ops_manual_search_result",
        }),
      ],
    });
  });

  it("merges matching ops manual preflight into the search artifact", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual-search",
          type: "ops_manual_search_result",
          titleZh: "运维手册检索",
          inlineData: {
            decision: "direct_execute",
            manuals: [
              {
                manual: { id: "manual-mysql-backup-ssh", title: "MySQL SSH 备份运维手册" },
                bound_workflow_id: "workflow-mysql-backup-ssh",
              },
            ],
          },
        },
        {
          id: "artifact-ops-manual-preflight",
          type: "ops_manual_preflight_result",
          titleZh: "运维手册预检",
          inlineData: {
            status: "passed",
            ready: true,
            manual_id: "manual-mysql-backup-ssh",
            workflow_id: "workflow-mysql-backup-ssh",
            evidence: [{ name: "ssh_access", status: "passed" }],
          },
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const artifacts = result.messages[1]?.metadata?.unstable_state?.agentUiArtifacts;

    expect(artifacts).toHaveLength(1);
    expect(artifacts?.[0]).toMatchObject({
      id: "artifact-ops-manual-search",
      type: "ops_manual_search_result",
      inlineData: {
        merged_preflight_result: expect.objectContaining({
          status: "passed",
          manual_id: "manual-mysql-backup-ssh",
          artifact_id: "artifact-ops-manual-preflight",
        }),
      },
    });
  });

  it("merges search, parameter resolution, and preflight into one ops manual progress artifact", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual-search",
          type: "ops_manual_search_result",
          inlineData: {
            decision: "need_info",
            manuals: [
              {
                manual: { id: "manual-redis-rca-ssh", title: "Redis SSH 排障运维手册" },
                bound_workflow_id: "workflow-redis-rca-ssh",
              },
            ],
          },
        },
        {
          id: "artifact-ops-manual-params",
          type: "ops_manual_param_resolution",
          inlineData: {
            status: "resolved",
            manual_id: "manual-redis-rca-ssh",
            workflow_id: "workflow-redis-rca-ssh",
          },
        },
        {
          id: "artifact-ops-manual-preflight",
          type: "ops_manual_preflight_result",
          inlineData: {
            status: "passed",
            ready: true,
            manual_id: "manual-redis-rca-ssh",
            workflow_id: "workflow-redis-rca-ssh",
          },
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const artifacts = result.messages[1]?.metadata?.unstable_state?.agentUiArtifacts;

    expect(artifacts).toHaveLength(1);
    expect(artifacts?.[0]).toMatchObject({
      id: "artifact-ops-manual-params",
      type: "ops_manual_search_result",
      inlineData: {
        original_search_artifact_id: "artifact-ops-manual-search",
        merged_param_resolution: expect.objectContaining({
          status: "resolved",
          manual_id: "manual-redis-rca-ssh",
          artifact_id: "artifact-ops-manual-params",
        }),
        merged_preflight_result: expect.objectContaining({
          status: "passed",
          artifact_id: "artifact-ops-manual-preflight",
        }),
      },
    });
    expect(artifacts).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "artifact-ops-manual-search" }),
        expect.objectContaining({ id: "artifact-ops-manual-preflight" }),
      ]),
    );
  });

  it("merges legacy parameter resolution when workflow_id carries the search flow id", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual-search",
          type: "ops_manual_search_result",
          inlineData: {
            decision: "need_info",
            ops_manual_flow_id: "flow-search-mysql",
            manuals: [
              {
                manual: { id: "manual-mysql-backup-ssh", title: "MySQL SSH 备份运维手册" },
                bound_workflow_id: "workflow-mysql-backup-ssh",
              },
            ],
          },
        },
        {
          id: "artifact-ops-manual-params",
          type: "ops_manual_param_resolution",
          inlineData: {
            status: "need_user_input",
            ops_manual_flow_id: "flow-regenerated-params",
            manual_id: "manual-mysql-backup-ssh",
            workflow_id: "flow-search-mysql",
            fields: [{ id: "backup_path", label: "备份路径" }],
          },
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const artifacts = result.messages[1]?.metadata?.unstable_state?.agentUiArtifacts;

    expect(artifacts).toHaveLength(1);
    expect(artifacts?.[0]).toMatchObject({
      id: "artifact-ops-manual-params",
      type: "ops_manual_search_result",
      inlineData: {
        original_search_artifact_id: "artifact-ops-manual-search",
        merged_param_resolution: expect.objectContaining({
          artifact_id: "artifact-ops-manual-params",
          manual_id: "manual-mysql-backup-ssh",
          workflow_id: "flow-search-mysql",
        }),
      },
    });
  });

  it("uses ops_manual_flow_id before manual and workflow heuristics when merging preflight results", () => {
    const state = createState();
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      agentUiArtifacts: [
        {
          id: "artifact-ops-manual-search-a",
          type: "ops_manual_search_result",
          inlineData: {
            decision: "direct_execute",
            ops_manual_flow_id: "flow-a",
            manuals: [
              {
                manual: { id: "manual-redis-rca-ssh", title: "Redis SSH 排障运维手册" },
                bound_workflow_id: "workflow-redis-rca-ssh",
              },
            ],
          },
        },
        {
          id: "artifact-ops-manual-search-b",
          type: "ops_manual_search_result",
          inlineData: {
            decision: "direct_execute",
            ops_manual_flow_id: "flow-b",
            manuals: [
              {
                manual: { id: "manual-redis-rca-ssh", title: "Redis SSH 排障运维手册" },
                bound_workflow_id: "workflow-redis-rca-ssh",
              },
            ],
          },
        },
        {
          id: "artifact-ops-manual-preflight-b",
          type: "ops_manual_preflight_result",
          inlineData: {
            status: "passed",
            ready: true,
            ops_manual_flow_id: "flow-b",
            manual_id: "manual-redis-rca-ssh",
            workflow_id: "workflow-redis-rca-ssh",
          },
        },
      ],
    };
    const converter = createAiopsTransportConverter();

    const result = converter(state, metadata());
    const artifacts = result.messages[1]?.metadata?.unstable_state?.agentUiArtifacts;

    expect(artifacts).toHaveLength(2);
    expect(artifacts?.[0]).toMatchObject({
      id: "artifact-ops-manual-search-a",
      inlineData: expect.not.objectContaining({
        merged_preflight_result: expect.anything(),
      }),
    });
    expect(artifacts?.[1]).toMatchObject({
      id: "artifact-ops-manual-search-b",
      inlineData: {
        ops_manual_flow_id: "flow-b",
        merged_preflight_result: expect.objectContaining({
          artifact_id: "artifact-ops-manual-preflight-b",
          ops_manual_flow_id: "flow-b",
        }),
      },
    });
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
