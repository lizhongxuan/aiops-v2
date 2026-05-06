export const ALLOWED_VALUE_SOURCE_TYPES = ["constant", "variable_reference", "expression"];

export function createInputParam(key = "input") {
  return {
    key,
    label: "",
    type: "string",
    required: false,
    description: "",
    value_source: { type: "constant", value: "" },
  };
}

export function cloneInputParam(param = {}) {
  return {
    ...createInputParam(param.key || "input"),
    ...JSON.parse(JSON.stringify(param || {})),
    value_source: normalizeValueSource(param.value_source),
  };
}

export function normalizeValueSource(source = {}) {
  const type = source.type || "constant";
  if (type === "variable_reference") {
    return {
      type,
      variable: source.variable || null,
    };
  }
  if (type === "expression") {
    return {
      type,
      expression: source.expression || "",
    };
  }
  return {
    type,
    value: source.value ?? "",
  };
}

export function normalizeInputParams(params = []) {
  return Array.isArray(params) ? params.map((param) => cloneInputParam(param)) : [];
}

export function validateInputParams(params = []) {
  const issues = [];
  const seen = new Set();
  for (const param of normalizeInputParams(params)) {
    const key = String(param.key || "").trim();
    if (!key) {
      issues.push({ code: "missing_key", key, message: "输入参数 key 不能为空" });
      continue;
    }
    if (seen.has(key)) {
      issues.push({ code: "duplicate_key", key, message: "输入参数 key 重复" });
    }
    seen.add(key);
    if (!ALLOWED_VALUE_SOURCE_TYPES.includes(param.value_source?.type)) {
      issues.push({
        code: "invalid_value_source",
        key,
        message: "value_source 只允许 constant、variable_reference、expression",
      });
    }
  }
  return issues;
}

export function variablePath(variable = {}) {
  if (variable.path) return variable.path;
  if (variable.scope === "node_output") return `nodes.${variable.node_id}.outputs.${variable.name}`;
  if (variable.scope && variable.name) return `${variable.scope}.${variable.name}`;
  return "";
}
