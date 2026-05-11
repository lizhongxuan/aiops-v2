import { describe, expect, it } from "vitest";
import {
  ALLOWED_VALUE_SOURCE_TYPES,
  cloneInputParam,
  createInputParam,
  normalizeInputParams,
  valueSourceLabel,
  variableToValueSource,
  validateInputParams,
} from "./ioTypes";

describe("ioTypes", () => {
  it("normalizes supported input param fields without mutating source", () => {
    const source = {
      key: "script",
      label: "Script",
      type: "string",
      required: true,
      description: "script body",
      value_source: { type: "literal", value: "./restore.sh" },
    };

    const cloned = cloneInputParam(source);

    expect(cloned).toEqual(source);
    expect(cloned).not.toBe(source);
    expect(createInputParam("backup_id")).toMatchObject({
      key: "backup_id",
      type: "string",
      value_source: { type: "literal", value: "" },
    });
  });

  it("validates allowed value source types and duplicate keys", () => {
    expect(ALLOWED_VALUE_SOURCE_TYPES).toEqual(["literal", "variable", "expression", "secret", "env"]);
    const params = normalizeInputParams([
      { key: "script", value_source: { type: "literal", value: "./restore.sh" } },
      { key: "script", value_source: { type: "unsupported", value: "db/password" } },
    ]);

    expect(validateInputParams(params)).toEqual([
      { code: "duplicate_key", key: "script", message: "输入参数 key 重复" },
      { code: "invalid_value_source", key: "script", message: "value_source 只允许 literal、variable、expression、secret、env" },
    ]);
  });

  it("converts runner variable candidates into backend graph value sources", () => {
    expect(variableToValueSource({ scope: "input", name: "host" })).toEqual({
      type: "variable",
      variable: { scope: "workflow_input", name: "host" },
    });
    expect(variableToValueSource({ scope: "sys", name: "run_id" })).toEqual({
      type: "variable",
      variable: { scope: "system", name: "run_id" },
    });
    expect(variableToValueSource({ scope: "node", nodeId: "precheck", name: "exit_code" })).toEqual({
      type: "variable",
      variable: { scope: "node_output", node_id: "precheck", name: "exit_code" },
    });
  });

  it("renders value source labels for literals, expression, and variable references", () => {
    expect(valueSourceLabel({ type: "literal", value: "prod" })).toBe("prod");
    expect(valueSourceLabel({ type: "expression", expression: "node.precheck.exit_code == 0" })).toBe("node.precheck.exit_code == 0");
    expect(valueSourceLabel({ type: "variable", variable: { scope: "node_output", node_id: "precheck", name: "exit_code" } })).toBe("node_output.precheck.exit_code");
    expect(valueSourceLabel({ type: "constant", value: "legacy" })).toBe("legacy");
    expect(valueSourceLabel({ type: "variable_reference", variable: { scope: "system", name: "run_id" } })).toBe("system.run_id");
  });
});
