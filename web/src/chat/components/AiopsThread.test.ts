import { describe, expect, it } from "vitest";

import { assistantMessageRenderedFinalText, assistantTranscriptFromContent, groupAssistantTranscriptBlocks, isNearThreadBottom, mergeAssistantArtifactRuns, orderedAssistantTurnBlocks } from "./AiopsThread";
import type { AiopsTransportBlock, AiopsTransportTurn } from "@/transport/aiopsTransportTypes";

describe("AiopsThread auto-scroll helpers", () => {
  it("treats a viewport close to the bottom as sticky", () => {
    expect(isNearThreadBottom({ scrollTop: 890, clientHeight: 100, scrollHeight: 1000 })).toBe(true);
  });

  it("does not auto-stick when the user has scrolled up into history", () => {
    expect(isNearThreadBottom({ scrollTop: 200, clientHeight: 100, scrollHeight: 1000 })).toBe(false);
  });
});

describe("AiopsThread canonical transcript", () => {
  it("uses blockOrder as the only rendering order", () => {
    const turn = {
      id: "turn-1",
      status: "completed",
      blockOrder: ["commentary-1", "final-1"],
      blocksById: {
        "final-1": {
          id: "final-1",
          type: "final_answer",
          kind: "assistant",
          phase: "final_answer",
          status: "completed",
          text: "最终结论",
        },
        "commentary-1": {
          id: "commentary-1",
          type: "commentary",
          kind: "assistant",
          phase: "commentary",
          status: "completed",
          text: "先检查",
        },
      },
    } as AiopsTransportTurn;

    expect(orderedAssistantTurnBlocks(turn).map((block) => block.id)).toEqual(["commentary-1", "final-1"]);
  });

  it("reads ordered transcript blocks from AssistantTransport data parts", () => {
    const transcript = assistantTranscriptFromContent([
      {
        type: "data",
        name: "aiops.transport.turn",
        data: { id: "turn-1", status: "completed" },
      },
      {
        type: "data",
        name: "aiops.transport.block",
        data: { id: "commentary-1", type: "commentary" },
      },
      {
        type: "data",
        name: "aiops.transport.block",
        data: { id: "final-1", type: "final_answer" },
      },
      { type: "text", text: "最终结论" },
    ]);

    expect(transcript.turn).toMatchObject({
      id: "turn-1",
      status: "completed",
    });
    expect(transcript.blocks.map((block) => block.id)).toEqual(["commentary-1", "final-1"]);
  });

  it("preserves visible process, artifact, later commentary, and final order", () => {
    const groups = groupAssistantTranscriptBlocks([
      {
        id: "reasoning-1",
        type: "reasoning",
        kind: "reasoning",
        status: "completed",
        text: "分析",
      },
      {
        id: "tool-1",
        type: "tool",
        kind: "tool",
        status: "completed",
        text: "检查",
      },
      {
        id: "artifact-1",
        type: "artifact",
        kind: "tool",
        status: "completed",
        text: "图表",
      },
      {
        id: "commentary-2",
        type: "commentary",
        kind: "assistant",
        phase: "commentary",
        status: "completed",
        text: "图表之后继续检查",
      },
      {
        id: "final-1",
        type: "final_answer",
        kind: "assistant",
        status: "completed",
        text: "结论",
      },
    ] as AiopsTransportBlock[]);

    expect(groups).toHaveLength(4);
    expect(groups[0]).toMatchObject({
      type: "process",
      blocks: [{ id: "reasoning-1" }, { id: "tool-1" }],
    });
    expect(groups[1]).toMatchObject({
      type: "block",
      block: { id: "artifact-1" },
    });
    expect(groups[2]).toMatchObject({
      type: "process",
      blocks: [{ id: "commentary-2" }],
    });
    expect(groups[3]).toMatchObject({
      type: "block",
      block: { id: "final-1" },
    });
  });

  it("merges a contiguous ops manual artifact run without reading legacy turn fields", () => {
    const blocks = mergeAssistantArtifactRuns([
      {
        id: "search-1", type: "artifact", kind: "tool", status: "completed", artifact: {
          id: "search-1", type: "ops_manual_search_result", inlineData: {
            ops_manual_flow_id: "flow-1",
            manuals: [{ manual: { id: "manual-1" }, bound_workflow_id: "workflow-1" }],
          },
        },
      },
      {
        id: "params-1", type: "artifact", kind: "tool", status: "completed", artifact: {
          id: "params-1", type: "ops_manual_param_resolution", inlineData: {
            ops_manual_flow_id: "flow-1", manual_id: "manual-1", workflow_id: "workflow-1",
          },
        },
      },
    ] as AiopsTransportBlock[]);

    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toMatchObject({
      id: "search-1",
      artifact: {
        id: "params-1",
        type: "ops_manual_search_result",
        inlineData: { original_search_artifact_id: "search-1" },
      },
    });
  });

  it("preserves the canonical collision-safe block id when artifact ids overlap", () => {
    const blocks = mergeAssistantArtifactRuns([
      {
        id: "artifact:shared-id",
        type: "artifact",
        kind: "tool",
        status: "completed",
        artifact: { id: "shared-id", type: "verification_result" },
      },
    ] as AiopsTransportBlock[]);

    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.id).toBe("artifact:shared-id");
    expect(blocks[0]?.artifact?.id).toBe("shared-id");
  });
});

describe("assistant message final text", () => {
  it("prefers transport finalText over stale assistant content", () => {
    const text = assistantMessageRenderedFinalText(
      [{ type: "text", text: "让我查看一下这台主机的基本信息。" }],
      { finalText: "" },
    );

    expect(text).toBe("");
  });

  it("does not infer evidence or artifacts from final text lines", () => {
    const finalText = [
        "已确认：",
        '- {"categoryCounts":{"application":25,"control-plane":14,"monitoring":10},"evidenceRefs":["ev-services"]}',
        '- {"evidenceRefs":["ev-incidents"],"incidents":[{"application":"rabbitmq-server"}]}',
        "",
        "仍缺少：",
        "- read_mcp_resource 未成功返回证据；不能当作已检查结果。",
        "- read_mcp_resource 未成功返回证据；不能当作已检查结果。",
        "",
        "下一步只读检查：",
        "1. 重新读取或替代核对 read_mcp_resource 对应的只读证据。",
      ].join("\n");
    const text = assistantMessageRenderedFinalText([], {
      finalText,
    });

    expect(text).toBe(finalText);
    expect(text.match(/read_mcp_resource 未成功返回证据/g)).toHaveLength(2);
  });

  it("does not classify final visibility from tool-process vocabulary", () => {
    const finalText = "让我直接读取证据引用： read_context_artifact with the evidence IDs: Let me try reading the evidence refs directly.";
    const text = assistantMessageRenderedFinalText([], {
      finalText,
    });

    expect(text).toBe(finalText);
  });

  it("does not classify final visibility from product-specific vocabulary", () => {
    const finalText = "SERVICE_NAME=rabbitmq-server。让我获取RCA上下文。".repeat(8);
    const text = assistantMessageRenderedFinalText([], {
      finalText,
    });

    expect(text).toBe(finalText);
  });
});
