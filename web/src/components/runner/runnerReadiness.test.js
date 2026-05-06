import { describe, expect, it } from "vitest";
import { checkRunnerWorkflowReadiness } from "./runnerReadiness";

const actions = [
  {
    action: "shell.run",
    input_schema: {
      type: "object",
      required: ["script"],
      properties: {
        script: { type: "string" },
        env: { type: "object" },
      },
    },
  },
];

describe("runnerReadiness", () => {
  it("treats missing failure next step as an exit path", () => {
    const graph = {
      workflow: { name: "host-check" },
      nodes: [
        {
          id: "check",
          type: "action",
          step: { name: "check", action: "shell.run", args: { script: "uptime" } },
          ports: [
            { id: "next", type: "output" },
            { id: "failure", type: "output" },
          ],
        },
      ],
      edges: [],
    };

    const result = checkRunnerWorkflowReadiness({ workflow: { name: "host-check" }, graph, actions });

    expect(result.ready).toBe(true);
    expect(result.blockers).toEqual([]);
    expect(result.infos).toContainEqual(expect.objectContaining({ code: "failure_defaults_to_exit_path" }));
  });

  it("blocks missing names, unknown actions, missing required args, and dangling edges", () => {
    const graph = {
      workflow: { name: "" },
      nodes: [
        { id: "a", type: "action", step: { name: "dup", action: "shell.run", args: {} } },
        { id: "b", type: "action", step: { name: "dup", action: "missing.action", args: {} } },
      ],
      edges: [{ id: "bad", source: "a", target: "ghost", source_port: "next", target_port: "in" }],
    };

    const result = checkRunnerWorkflowReadiness({ workflow: { name: "" }, graph, actions });

    expect(result.ready).toBe(false);
    expect(result.blockers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ code: "workflow_name_missing" }),
        expect.objectContaining({ code: "duplicate_node_name", nodeId: "b" }),
        expect.objectContaining({ code: "required_arg_missing", nodeId: "a", field: "script" }),
        expect.objectContaining({ code: "unknown_action", nodeId: "b" }),
        expect.objectContaining({ code: "dangling_edge", edgeId: "bad" }),
      ]),
    );
  });
});
