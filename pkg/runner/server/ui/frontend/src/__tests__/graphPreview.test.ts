import { describe, expect, it } from "vitest";
import type { WorkflowGraph } from "../types/workflow";
import { formatGraphJSON, graphPreviewText, toYamlLike } from "../utils/graphPreview";

const graph: WorkflowGraph = {
  version: "v1",
  workflow: {
    version: "v0.1",
    name: "demo",
  },
  nodes: [
    {
      id: "run",
      type: "action",
      position: { x: 10, y: 20 },
      step: {
        name: "run",
        action: "cmd.run",
        targets: ["local"],
        args: { cmd: "echo ok" },
      },
    },
  ],
  edges: [],
};

describe("graph preview helpers", () => {
  it("prefers compiled yamlPreview when present", () => {
    expect(graphPreviewText(graph, "name: compiled\n")).toBe("name: compiled");
  });

  it("falls back to a YAML-like graph preview", () => {
    const preview = graphPreviewText(graph, "");

    expect(preview).toContain("version: v1");
    expect(preview).toContain("workflow:");
    expect(preview).toContain("nodes:");
    expect(preview).toContain("cmd: \"echo ok\"");
  });

  it("formats graph JSON for the advanced editor", () => {
    expect(JSON.parse(formatGraphJSON(graph))).toMatchObject({ version: "v1", workflow: { name: "demo" } });
  });

  it("quotes scalar values only when needed", () => {
    expect(toYamlLike({ plain: "local", spaced: "echo ok" })).toBe('plain: local\nspaced: "echo ok"');
  });
});
