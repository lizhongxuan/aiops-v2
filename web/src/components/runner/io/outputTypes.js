export const ALLOWED_EXTRACT_SOURCE_TYPES = [
  "stdout_text",
  "stdout_jsonpath",
  "stderr_text",
  "exit_code",
  "export_var",
  "approval_result",
  "subflow_output",
];

export function createOutputParam(key = "output") {
  return {
    key,
    type: "string",
    description: "",
    extract_source: { type: "stdout_text", path: "" },
  };
}

export function normalizeExtractSource(source = {}) {
  return {
    type: source.type || "stdout_text",
    path: source.path || source.extract_rule || "",
    expression: source.expression || "",
    value: source.value,
  };
}

export function cloneOutputParam(param = {}) {
  return {
    ...createOutputParam(param.key || "output"),
    ...JSON.parse(JSON.stringify(param || {})),
    extract_source: normalizeExtractSource(param.extract_source),
  };
}

export function normalizeOutputParams(outputs = []) {
  return Array.isArray(outputs) ? outputs.map((output) => cloneOutputParam(output)) : [];
}

export function validateJsonPath(path = "") {
  const value = String(path || "").trim();
  if (!value) return "";
  if (!value.startsWith("$")) return "JSONPath 必须以 $ 开头";
  if (value.includes("..")) return "JSONPath 不能包含空路径段";
  return "";
}

export function validateOutputParams(outputs = []) {
  const issues = [];
  const seen = new Set();
  for (const output of normalizeOutputParams(outputs)) {
    const key = String(output.key || "").trim();
    if (!key) {
      issues.push({ code: "missing_key", key, message: "输出变量 key 不能为空" });
      continue;
    }
    if (seen.has(key)) issues.push({ code: "duplicate_key", key, message: "输出变量 key 重复" });
    seen.add(key);
    if (!ALLOWED_EXTRACT_SOURCE_TYPES.includes(output.extract_source?.type)) {
      issues.push({ code: "invalid_extract_source", key, message: "extract_source 不支持" });
    }
    if (output.extract_source?.type === "stdout_jsonpath") {
      const message = validateJsonPath(output.extract_source.path);
      if (message) issues.push({ code: "invalid_jsonpath", key, message });
    }
  }
  return issues;
}
