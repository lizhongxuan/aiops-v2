import { describe, expect, it } from "vitest";
import {
  collectRunnerVariables,
  compileRunnerVariableSelector,
  parseRunnerVariableExpression,
  validateRunnerVariableReferences,
} from "./runnerVariables";

const graph = {
  version: "v1",
  workflow: {
    name: "pg-restore",
    inputs: [
      { key: "backup_id", type: "string", required: true, description: "Backup identifier" },
      { key: "db_password", type: "secret", secret: true, default: "super-secret-input" },
    ],
    env: {
      PGDATA: "/data/pg",
      PGPASSWORD: { type: "secret", secret: true, value: "super-secret-env" },
    },
  },
  nodes: [
    {
      id: "start",
      type: "start",
      outputs: [{ key: "ticket_id", type: "string" }],
    },
    {
      id: "restore",
      type: "action",
      step: { action: "shell.run" },
      outputs: [
        { key: "restore_lsn", type: "string", description: "Restored WAL LSN" },
        { key: "restore_token", type: "secret", secret: true, example: "super-secret-node" },
      ],
    },
    {
      id: "verify",
      type: "action",
      step: { action: "cmd.run" },
      outputs: [{ key: "duration_ms", type: "number" }],
    },
    {
      id: "notify",
      type: "notify",
      step: { action: "notify.send" },
    },
  ],
  edges: [
    { id: "start-restore", source: "start", target: "restore", kind: "next" },
    { id: "restore-verify", source: "restore", target: "verify", kind: "next" },
    { id: "verify-notify", source: "verify", target: "notify", kind: "next" },
  ],
};

describe("runnerVariables", () => {
  it("collects only global and upstream variables for the current node", () => {
    const variables = collectRunnerVariables(graph, "verify");
    const expressions = variables.map((variable) => variable.expression);

    expect(expressions).toEqual(expect.arrayContaining([
      "input.backup_id",
      "input.ticket_id",
      "env.PGDATA",
      "sys.run_id",
      "node.restore.restore_lsn",
    ]));
    expect(expressions).not.toContain("node.verify.duration_ms");
    expect(expressions).not.toContain("node.notify.anything");

    const restoreLsn = variables.find((variable) => variable.expression === "node.restore.restore_lsn");
    expect(restoreLsn).toMatchObject({
      selector: { scope: "node", nodeId: "restore", name: "restore_lsn" },
      sourceNodeId: "restore",
      type: "string",
    });
  });

  it("reports stale references when an upstream output is removed", () => {
    const next = {
      ...graph,
      nodes: graph.nodes.map((node) => (node.id === "restore" ? { ...node, outputs: [] } : node)),
    };

    const issues = validateRunnerVariableReferences(next, "verify", [
      { scope: "node", nodeId: "restore", name: "restore_lsn" },
      { scope: "input", name: "backup_id" },
    ]);

    expect(issues).toEqual([
      expect.objectContaining({
        code: "missing_node_output",
        expression: "node.restore.restore_lsn",
        severity: "warning",
      }),
    ]);
  });

  it("masks secret variables without leaking raw values", () => {
    const variables = collectRunnerVariables(graph, "verify");
    const json = JSON.stringify(variables);

    expect(json).not.toContain("super-secret-input");
    expect(json).not.toContain("super-secret-env");
    expect(json).not.toContain("super-secret-node");
    expect(variables.find((variable) => variable.expression === "input.db_password")).toMatchObject({
      secret: true,
      displayValue: "******",
    });
    expect(variables.find((variable) => variable.expression === "env.PGPASSWORD")).toMatchObject({
      secret: true,
      displayValue: "******",
    });
    expect(variables.find((variable) => variable.expression === "node.restore.restore_token")).toMatchObject({
      secret: true,
      displayValue: "******",
    });
  });

  it("attaches last-run values to visible variables without unmasking secrets", () => {
    const variables = collectRunnerVariables(graph, "verify", {
      runState: {
        runId: "run-1",
        status: "success",
        variables: {
          inputs: [{ key: "backup_id", value: "backup-42" }],
          outputs: [{ nodeId: "restore", key: "restore_lsn", value: "0/16B6C50" }],
          exports: [{ key: "PGDATA", value: "/runtime/pg" }],
          nodeResults: [],
        },
        nodes: {
          restore: {
            nodeId: "restore",
            result: {
              restore_token: "runtime-secret-node",
            },
          },
        },
      },
    });

    expect(variables.find((variable) => variable.expression === "input.backup_id")).toMatchObject({
      lastValue: "backup-42",
      displayValue: "backup-42",
    });
    expect(variables.find((variable) => variable.expression === "node.restore.restore_lsn")).toMatchObject({
      lastValue: "0/16B6C50",
      displayValue: "0/16B6C50",
    });
    expect(variables.find((variable) => variable.expression === "node.restore.restore_token")).toMatchObject({
      secret: true,
      displayValue: "******",
    });
  });

  it("compiles and parses structured selectors as Runner variable expressions", () => {
    expect(compileRunnerVariableSelector({ scope: "input", name: "backup_id" })).toBe("input.backup_id");
    expect(compileRunnerVariableSelector({ scope: "env", name: "PGDATA" })).toBe("env.PGDATA");
    expect(compileRunnerVariableSelector({ scope: "sys", name: "run_id" })).toBe("sys.run_id");
    expect(compileRunnerVariableSelector({ scope: "node", nodeId: "restore", name: "restore_lsn" })).toBe(
      "node.restore.restore_lsn",
    );
    expect(parseRunnerVariableExpression("node.restore.restore_lsn")).toEqual({
      scope: "node",
      nodeId: "restore",
      name: "restore_lsn",
    });
  });
});
