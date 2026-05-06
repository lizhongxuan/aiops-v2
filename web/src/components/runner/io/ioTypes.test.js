import { describe, expect, it } from "vitest";
import {
  ALLOWED_VALUE_SOURCE_TYPES,
  cloneInputParam,
  createInputParam,
  normalizeInputParams,
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
      value_source: { type: "constant", value: "./restore.sh" },
    };

    const cloned = cloneInputParam(source);

    expect(cloned).toEqual(source);
    expect(cloned).not.toBe(source);
    expect(createInputParam("backup_id")).toMatchObject({
      key: "backup_id",
      type: "string",
      value_source: { type: "constant", value: "" },
    });
  });

  it("validates allowed value source types and duplicate keys", () => {
    expect(ALLOWED_VALUE_SOURCE_TYPES).toEqual(["constant", "variable_reference", "expression"]);
    const params = normalizeInputParams([
      { key: "script", value_source: { type: "constant", value: "./restore.sh" } },
      { key: "script", value_source: { type: "secret_ref", secret_ref: "db/password" } },
    ]);

    expect(validateInputParams(params)).toEqual([
      { code: "duplicate_key", key: "script", message: "输入参数 key 重复" },
      { code: "invalid_value_source", key: "script", message: "value_source 只允许 constant、variable_reference、expression" },
    ]);
  });
});
