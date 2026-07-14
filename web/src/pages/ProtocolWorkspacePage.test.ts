import { describe, expect, it } from "vitest";

import type { AiopsTransportTurn } from "@/transport/aiopsTransportTypes";

import { protocolTurnTranscript } from "./ProtocolWorkspacePage";

describe("ProtocolWorkspacePage canonical transcript", () => {
  it("derives process and final from blockOrder plus blocksById", () => {
    const turn = {
      id: "turn-1",
      status: "completed",
      blockOrder: ["command-1", "final-1"],
      blocksById: {
        "final-1": {
          id: "final-1",
          type: "final_answer",
          kind: "assistant",
          status: "completed",
          text: "检查完成",
        },
        "command-1": {
          id: "command-1",
          type: "command",
          kind: "command",
          status: "completed",
          text: "hostname",
        },
      },
    } as AiopsTransportTurn;

    const transcript = protocolTurnTranscript(turn);

    expect(transcript.process.map((block) => block.id)).toEqual(["command-1"]);
    expect(transcript.final?.text).toBe("检查完成");
  });
});
