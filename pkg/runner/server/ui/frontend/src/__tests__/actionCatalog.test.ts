import { describe, expect, it } from "vitest";
import type { ActionSpec, WorkflowGraph } from "../types/workflow";
import { createActionNodeFromSpec, filterActionCatalog } from "../utils/actionCatalog";

const graph: WorkflowGraph = {
  version: "v1",
  workflow: { version: "v0.1", name: "demo" },
  nodes: [
    { id: "start", type: "start", label: "Start", position: { x: 80, y: 160 } },
    { id: "cmd-run", type: "action", position: { x: 320, y: 160 }, step_name: "cmd-run" },
  ],
  edges: [],
};

describe("action catalog helpers", () => {
  it("filters actions and keeps deterministic category groups", () => {
    const actions: ActionSpec[] = [
      { action: "shell.run", title: "Shell Script", category: "script", risk: "high" },
      { action: "cmd.run", title: "Command", category: "command", risk: "medium" },
      { action: "script.python", title: "Stored Python Script", category: "script", description: "Run Python script" },
    ];

    expect(filterActionCatalog(actions, "script")).toEqual([
      {
        category: "script",
        actions: [
          expect.objectContaining({ action: "shell.run" }),
          expect.objectContaining({ action: "script.python" }),
        ],
      },
    ]);
  });

  it("creates graph nodes from spec defaults and schema fallback values", () => {
    const spec: ActionSpec = {
      action: "custom.deploy",
      title: "Deploy",
      category: "release",
      risk: "high",
      node_type: "action",
      defaults: { command: "deploy --check" },
      args_schema: {
        type: "object",
        required: ["command", "environment"],
        properties: {
          command: { type: "string" },
          environment: { type: "string", default: "staging" },
          dry_run: { type: "boolean", default: true },
        },
      },
    };

    const node = createActionNodeFromSpec(spec, graph, { x: 500, y: 240 });

    expect(node).toMatchObject({
      id: "custom-deploy",
      type: "action",
      label: "Deploy",
      position: { x: 500, y: 240 },
      step_name: "custom-deploy",
      step: {
        id: "custom-deploy",
        name: "custom-deploy",
        action: "custom.deploy",
        args: {
          command: "deploy --check",
          environment: "staging",
          dry_run: true,
        },
      },
    });
  });
});
