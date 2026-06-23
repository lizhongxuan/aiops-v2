import { describe, expect, it } from "vitest";

import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";

import {
  buildCorootMentionMetadata,
  buildOpsManualParamFormSubmit,
  resolveStopDispatchTarget,
} from "./aiopsComposerActions";

describe("aiopsComposerActions", () => {
  it("prefers transport stop for an active transport turn even when assistant-ui reports running", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      currentTurnId: "turn-1",
      status: "working" as const,
      runtimeLiveness: {
        activeTurns: { "turn-1": true },
        activeAgents: {},
        pendingApprovals: {},
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
    };

    expect(resolveStopDispatchTarget(state, true)).toBe("transport");
  });

  it("uses transport stop as soon as the session exists, even before currentTurnId is projected", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      status: "idle" as const,
    };

    expect(resolveStopDispatchTarget(state, true)).toBe("transport");
  });

  it("falls back to runtime cancel only when no transport session exists yet", () => {
    const state = createInitialAiopsTransportState("thread-1");

    expect(resolveStopDispatchTarget(state, true)).toBe("runtime");
  });

  it("builds structured ops manual parameter form submissions without lossy prose", () => {
    const result = buildOpsManualParamFormSubmit({
      artifactId: "artifact-param",
      manualId: "manual-redis-rca-ssh",
      workflowId: "workflow-redis-rca-ssh",
      params: { redis_instance: "docker:aiops-redis" },
    });

    expect(result.text).toBe(
      "已提交运维手册参数：redis_instance=docker:aiops-redis",
    );
    expect(result.metadata).toMatchObject({
      opsManualAction: "submit_ops_manual_param_form",
      sourceArtifactId: "artifact-param",
      opsManualManualId: "manual-redis-rca-ssh",
      opsManualWorkflowId: "workflow-redis-rca-ssh",
      opsManualParamsJson: '{"redis_instance":"docker:aiops-redis"}',
    });
    expect(result.text).not.toContain("��");
  });

  it("adds Coroot RCA metadata only for explicit @Coroot mentions", () => {
    expect(
      buildCorootMentionMetadata("请 @Coroot 分析 checkout 根因"),
    ).toMatchObject({
      "aiops.coroot.explicitRCA": "true",
      "aiops.coroot.rcaDisplayAllowed": "true",
    });
    expect(
      buildCorootMentionMetadata("@coroot checkout 服务异常"),
    ).toMatchObject({
      "aiops.coroot.explicitRCA": "true",
      "aiops.coroot.rcaDisplayAllowed": "true",
    });
    expect(buildCorootMentionMetadata("请采集 Coroot 指标作为证据")).toEqual(
      {},
    );
  });
});
