import { describe, expect, it } from "vitest";

import { resolveArtifactSlots } from "./artifactPlacement";
import type { AnswerSection } from "./types";

const sections: AnswerSection[] = [
  { id: "root_cause-0", kind: "root_cause", title: "根因", markdown: "外部依赖 external:18090 unknown。", order: 0 },
  { id: "evidence-1", kind: "evidence", title: "证据", markdown: "- Coroot RCA 查询成功", order: 1 },
];

describe("resolveArtifactSlots", () => {
  it("places a root-cause Coroot chart after the root cause section", () => {
    const slots = resolveArtifactSlots({
      sections,
      artifacts: [
        {
          id: "artifact-coroot-net",
          type: "coroot_chart",
          titleZh: "aiops-host-agent 服务",
          metadata: {
            placement: {
              supports: ["root_cause"],
              topic: "net",
              priority: "primary",
            },
          },
        },
      ],
      deferredArtifacts: [],
    });

    expect(slots).toEqual([
      expect.objectContaining({
        state: "ready",
        placement: "after",
        sectionId: "root_cause-0",
        artifact: expect.objectContaining({ id: "artifact-coroot-net" }),
      }),
    ]);
  });

  it("uses a deferred placeholder at the same semantic position while running", () => {
    const slots = resolveArtifactSlots({
      sections,
      artifacts: [],
      deferredArtifacts: [
        {
          id: "artifact-coroot-net",
          type: "coroot_chart",
          metadata: {
            placement: {
              supports: ["root_cause"],
              topic: "net",
            },
          },
        },
      ],
    });

    expect(slots).toEqual([
      expect.objectContaining({
        state: "deferred",
        placement: "after",
        sectionId: "root_cause-0",
      }),
    ]);
  });

  it("falls back to the end when no semantic section matches", () => {
    const slots = resolveArtifactSlots({
      sections: [{ id: "unknown-0", kind: "unknown", markdown: "普通回答", order: 0 }],
      artifacts: [{ id: "artifact-coroot-cpu", type: "coroot_chart" }],
      deferredArtifacts: [],
    });

    expect(slots[0]).toMatchObject({
      placement: "end",
      sectionId: undefined,
    });
  });
});
