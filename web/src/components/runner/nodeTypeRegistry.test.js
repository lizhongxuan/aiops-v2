import { describe, expect, it } from "vitest";
import {
  getActionDefaultPorts,
  getNodeCanvasMeta,
  getNodePorts,
  getNodeTypeDefinition,
  isActionAllowedAfterPort,
} from "./nodeTypeRegistry";

describe("nodeTypeRegistry", () => {
  it("derives semantic node metadata and ports from Runner actions", () => {
    const condition = { id: "gate", type: "condition", step: { action: "condition.branch" } };
    const approval = { id: "approval", type: "approval", step: { action: "approval.wait" } };
    const aggregator = { id: "aggregate", type: "variable_aggregator", step: { action: "variable.aggregate" } };

    expect(getNodeTypeDefinition(condition)).toMatchObject({
      key: "condition",
      label: "条件分支",
      iconText: "IF",
    });
    expect(getNodePorts(condition).outputs.map((port) => port.id)).toEqual(["if", "else"]);
    expect(getNodePorts(approval).outputs.map((port) => port.id)).toEqual(["approved", "rejected"]);
    expect(getNodeTypeDefinition(aggregator)).toMatchObject({
      key: "variable-aggregator",
      label: "变量聚合",
      iconText: "VAR",
    });
    expect(getNodePorts(aggregator).outputs.map((port) => port.id)).toEqual(["next"]);
  });

  it("preserves explicit graph ports over registry defaults", () => {
    const node = {
      id: "custom",
      type: "action",
      step: { action: "shell.run" },
      ports: {
        inputs: [{ id: "custom-in", label: "Custom In" }],
        outputs: [{ id: "custom-out", label: "Custom Out" }],
      },
    };

    expect(getNodePorts(node)).toEqual({
      inputs: [{ id: "custom-in", label: "Custom In" }],
      outputs: [{ id: "custom-out", label: "Custom Out" }],
    });
  });

  it("treats end nodes as terminal nodes with no outgoing port", () => {
    const end = { id: "end", type: "end", label: "End" };

    expect(getNodeTypeDefinition(end)).toMatchObject({
      key: "end",
      label: "结束",
    });
    expect(getNodePorts(end)).toEqual({
      inputs: [{ id: "in", label: "输入" }],
      outputs: [],
    });
  });

  it("uses action catalog default ports before hardcoded node definitions", () => {
    const ports = getActionDefaultPorts({
      action: "cmd.run",
      default_ports: {
        inputs: [{ id: "catalog-in", label: "Catalog In" }],
        outputs: [{ id: "catalog-out", label: "Catalog Out" }],
      },
    });

    expect(ports).toEqual({
      inputs: [{ id: "catalog-in", label: "Catalog In" }],
      outputs: [{ id: "catalog-out", label: "Catalog Out" }],
    });
  });

  it("filters follow-up actions by source port semantics", () => {
    const actions = [
      { action: "cmd.run", label: "Command" },
      { action: "notify.send", label: "Notify" },
      { action: "approval.wait", label: "Approval" },
    ];

    expect(actions.filter((action) => isActionAllowedAfterPort(action, "failure")).map((action) => action.action)).toEqual([
      "notify.send",
      "approval.wait",
    ]);
    expect(getNodeCanvasMeta({ id: "cmd", step: { action: "cmd.run" } })).toMatchObject({
      label: "Command",
      action: "cmd.run",
    });
  });
});
