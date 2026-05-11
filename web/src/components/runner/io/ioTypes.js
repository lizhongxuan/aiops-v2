export const ALLOWED_VALUE_SOURCE_TYPES = ["literal", "variable", "expression", "secret", "env"];

export function createInputParam(key = "input") {
  return {
    key,
    label: "",
    type: "string",
    required: false,
    description: "",
    value_source: { type: "literal", value: "" },
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
  const rawType = source.type || "literal";
  const type = rawType === "constant" ? "literal" : rawType === "variable_reference" ? "variable" : rawType === "secret_ref" ? "secret" : rawType;
  if (type === "variable") {
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
  if (type === "secret") {
    return {
      type,
      secret_ref: source.secret_ref || source.secretRef || "",
    };
  }
  if (type === "env") {
    return {
      type,
      env_key: source.env_key || source.envKey || "",
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
        message: "value_source 只允许 literal、variable、expression、secret、env",
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

export function variableToValueSource(variable = {}) {
  const name = String(variable.name || variable.key || "").trim();
  if (!name) return { type: "literal", value: "" };
  const scope = backendVariableScope(variable.scope);
  const ref = { scope, name };
  const nodeId = String(variable.nodeId || variable.node_id || variable.sourceNodeId || "").trim();
  if (nodeId && ["node_output", "approval", "subflow"].includes(scope)) {
    ref.node_id = nodeId;
  }
  return {
    type: "variable",
    variable: ref,
  };
}

export function valueSourceLabel(source = {}) {
  const normalized = normalizeValueSource(source);
  if (normalized.type === "variable") {
    const variable = normalized.variable || {};
    const parts = [variable.scope, variable.node_id, variable.name].filter(Boolean);
    return parts.join(".");
  }
  if (normalized.type === "expression") {
    return normalized.expression || "";
  }
  if (normalized.value === undefined || normalized.value === null) return "";
  if (typeof normalized.value === "object") return JSON.stringify(normalized.value);
  return String(normalized.value);
}

function backendVariableScope(scope = "") {
  switch (String(scope || "").trim()) {
  case "input":
  case "workflow_input":
    return "workflow_input";
  case "sys":
  case "system":
    return "system";
  case "env":
  case "workflow_var":
  case "environment":
    return "workflow_var";
  case "node":
  case "node_output":
    return "node_output";
  case "approval":
    return "approval";
  case "subflow":
    return "subflow";
  case "inventory":
    return "inventory";
  default:
    return String(scope || "").trim() || "workflow_var";
  }
}
