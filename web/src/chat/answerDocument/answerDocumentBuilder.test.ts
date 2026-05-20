import { describe, expect, it } from "vitest";

import { buildAnswerDocument } from "./answerDocumentBuilder";

describe("buildAnswerDocument", () => {
  it("inserts a ready chart slot after root cause and before evidence", () => {
    const document = buildAnswerDocument({
      finalText: [
        "根因：外部依赖 external:18090 unknown。",
        "",
        "证据：",
        "- Coroot RCA 查询成功",
      ].join("\n"),
      artifacts: [{ id: "artifact-coroot-net", type: "coroot_chart" }],
      deferredArtifacts: [],
    });

    expect(document.map((node) => node.type)).toEqual([
      "section",
      "artifact_slot",
      "section",
    ]);
    expect(document[1]).toMatchObject({
      type: "artifact_slot",
      slot: {
        state: "ready",
        placement: "after",
        artifact: expect.objectContaining({ id: "artifact-coroot-net" }),
      },
    });
  });

  it("inserts a deferred slot while the chart is delayed", () => {
    const document = buildAnswerDocument({
      finalText: "根因：CPU 使用率升高。",
      artifacts: [],
      deferredArtifacts: [{ id: "artifact-coroot-cpu", type: "coroot_chart" }],
    });

    expect(document).toEqual([
      expect.objectContaining({ type: "section" }),
      expect.objectContaining({
        type: "artifact_slot",
        slot: expect.objectContaining({ state: "deferred" }),
      }),
    ]);
  });
});
