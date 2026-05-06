import { getGraphUpstreamNodeIds } from "./canvasGraphAdapter";

const SECRET_DISPLAY_VALUE = "******";

const SYSTEM_VARIABLES = [
  { name: "run_id", type: "string", description: "Runner run identifier" },
  { name: "workflow_name", type: "string", description: "Workflow name" },
  { name: "operator", type: "string", description: "Operator account" },
  { name: "timestamp", type: "string", description: "Runtime timestamp" },
];

export function collectRunnerVariables(graph = {}, nodeId = "", options = {}) {
  const upstreamIds = new Set(getGraphUpstreamNodeIds(graph, nodeId));
  const variables = [
    ...collectInputVariables(graph),
    ...collectEnvVariables(graph, options),
    ...collectSystemVariables(),
    ...collectNodeOutputVariables(graph, upstreamIds),
  ];
  return dedupeVariables(variables).sort(sortVariables);
}

export function validateRunnerVariableReferences(graph = {}, nodeId = "", references = []) {
  const availableByExpression = new Map(
    collectRunnerVariables(graph, nodeId).map((variable) => [variable.expression, variable]),
  );
  const issues = [];
  for (const reference of references || []) {
    const selector = normalizeSelector(reference);
    const expression = compileRunnerVariableSelector(selector);
    if (!expression) {
      issues.push({
        severity: "warning",
        code: "invalid_variable_reference",
        expression: "",
        selector,
        message: "变量引用格式无效。",
      });
      continue;
    }
    if (availableByExpression.has(expression)) continue;
    issues.push({
      severity: "warning",
      code: issueCodeForSelector(graph, selector),
      expression,
      selector,
      message: `变量 ${expression} 当前不可用或已被删除。`,
    });
  }
  return issues;
}

export function compileRunnerVariableSelector(selector = {}) {
  const next = normalizeSelector(selector);
  const scope = next.scope;
  const name = cleanKey(next.name);
  if (scope === "node") {
    const nodeId = cleanKey(next.nodeId || next.node_id || next.sourceNodeId);
    if (!nodeId || !name) return "";
    return `node.${nodeId}.${name}`;
  }
  if (["input", "env", "sys"].includes(scope) && name) {
    return `${scope}.${name}`;
  }
  return "";
}

export function parseRunnerVariableExpression(expression = "") {
  const parts = String(expression || "").trim().split(".").filter(Boolean);
  if (parts.length < 2) return null;
  const [scope] = parts;
  if (scope === "node" && parts.length >= 3) {
    return {
      scope: "node",
      nodeId: parts[1],
      name: parts.slice(2).join("."),
    };
  }
  if (["input", "env", "sys"].includes(scope)) {
    return {
      scope,
      name: parts.slice(1).join("."),
    };
  }
  return null;
}

export function normalizeRunnerVariableReference(reference = {}) {
  const selector = normalizeSelector(reference);
  return {
    selector,
    expression: compileRunnerVariableSelector(selector),
  };
}

function collectInputVariables(graph = {}) {
  const variables = [];
  for (const input of normalizeSchemaItems(graph.workflow?.inputs || graph.inputs || [])) {
    variables.push(createVariable({
      scope: "input",
      name: input.key || input.name,
      type: input.type,
      description: input.description || input.label,
      secret: isSecret(input),
      value: input.value ?? input.default,
      required: Boolean(input.required),
    }));
  }

  for (const node of graph.nodes || []) {
    if (node.type !== "start") continue;
    for (const output of normalizeSchemaItems(node.outputs || [])) {
      variables.push(createVariable({
        scope: "input",
        name: output.key || output.name,
        type: output.type,
        description: output.description || output.label,
        secret: isSecret(output),
        sourceNodeId: node.id,
      }));
    }
    for (const input of normalizeSchemaItems(node.inputs || [])) {
      variables.push(createVariable({
        scope: "input",
        name: input.key || input.name,
        type: input.type,
        description: input.description || input.label,
        secret: isSecret(input),
        value: input.value ?? input.default,
        required: Boolean(input.required),
      }));
    }
  }
  return variables;
}

function collectEnvVariables(graph = {}, options = {}) {
  const source = options.env ?? graph.workflow?.env ?? graph.env ?? graph.workflow?.vars?.env ?? [];
  return normalizeEnvItems(source).map((item) => createVariable({
    scope: "env",
    name: item.key,
    type: item.type,
    description: item.description || item.label,
    secret: isSecret(item),
    value: item.value,
  }));
}

function collectSystemVariables() {
  return SYSTEM_VARIABLES.map((item) => createVariable({
    scope: "sys",
    name: item.name,
    type: item.type,
    description: item.description,
  }));
}

function collectNodeOutputVariables(graph = {}, upstreamIds = new Set()) {
  const variables = [];
  for (const node of graph.nodes || []) {
    if (!upstreamIds.has(node.id) || node.type === "start") continue;
    for (const output of normalizeSchemaItems(node.outputs || [])) {
      variables.push(createVariable({
        scope: "node",
        name: output.key || output.name,
        type: output.type,
        description: output.description || output.label,
        secret: isSecret(output),
        value: output.value ?? output.example,
        sourceNodeId: node.id,
      }));
    }
  }
  return variables;
}

function createVariable(input = {}) {
  const scope = normalizeScope(input.scope);
  const name = cleanKey(input.name);
  if (!scope || !name) return null;
  const selector = scope === "node"
    ? { scope, nodeId: cleanKey(input.sourceNodeId || input.nodeId), name }
    : { scope, name };
  const expression = compileRunnerVariableSelector(selector);
  if (!expression) return null;

  const variable = {
    scope,
    name,
    type: input.type || "any",
    expression,
    path: expression,
    selector,
    description: input.description || "",
    required: Boolean(input.required),
  };
  if (selector.nodeId) {
    variable.nodeId = selector.nodeId;
    variable.node_id = selector.nodeId;
    variable.sourceNodeId = selector.nodeId;
  }
  if (input.secret) {
    variable.secret = true;
    variable.masked = true;
    variable.displayValue = SECRET_DISPLAY_VALUE;
  } else if (input.value !== undefined) {
    variable.value = input.value;
    variable.displayValue = String(input.value);
  }
  return variable;
}

function normalizeSelector(reference = {}) {
  if (typeof reference === "string") {
    return parseRunnerVariableExpression(reference) || {};
  }
  if (reference.selector) {
    return normalizeSelector(reference.selector);
  }
  if (reference.path || reference.expression) {
    const parsed = parseRunnerVariableExpression(reference.path || reference.expression);
    if (parsed) return parsed;
  }
  return {
    scope: normalizeScope(reference.scope),
    nodeId: cleanKey(reference.nodeId || reference.node_id || reference.sourceNodeId),
    name: cleanKey(reference.name || reference.key),
  };
}

function normalizeScope(scope = "") {
  switch (String(scope || "").trim()) {
  case "workflow_input":
  case "input":
    return "input";
  case "system":
  case "sys":
    return "sys";
  case "environment":
  case "env":
    return "env";
  case "node_output":
  case "node":
    return "node";
  default:
    return String(scope || "").trim();
  }
}

function issueCodeForSelector(graph = {}, selector = {}) {
  if (selector.scope !== "node") return "missing_variable";
  const node = (graph.nodes || []).find((item) => item.id === selector.nodeId);
  if (!node) return "missing_node";
  const outputExists = normalizeSchemaItems(node.outputs || []).some((output) => (output.key || output.name) === selector.name);
  return outputExists ? "inaccessible_node_output" : "missing_node_output";
}

function normalizeSchemaItems(input = []) {
  if (Array.isArray(input)) {
    return input.filter(Boolean).map((item) => ({ ...item }));
  }
  if (input && typeof input === "object" && input.properties && typeof input.properties === "object") {
    return Object.entries(input.properties).map(([key, value]) => ({ key, ...(value || {}) }));
  }
  if (input && typeof input === "object") {
    return Object.entries(input).map(([key, value]) => {
      if (value && typeof value === "object" && !Array.isArray(value)) return { key, ...value };
      return { key, type: inferValueType(value), value };
    });
  }
  return [];
}

function normalizeEnvItems(input = []) {
  if (Array.isArray(input)) {
    return input.map((item) => ({ key: item.key || item.name, ...item })).filter((item) => cleanKey(item.key));
  }
  if (input && typeof input === "object") {
    return Object.entries(input).map(([key, value]) => {
      if (value && typeof value === "object" && !Array.isArray(value)) return { key, ...value };
      return { key, type: inferValueType(value), value };
    });
  }
  return [];
}

function isSecret(item = {}) {
  const type = String(item.type || "").toLowerCase();
  return Boolean(item.secret || item.masked || item.secret_ref || item.secretRef || type === "secret");
}

function inferValueType(value) {
  if (Array.isArray(value)) return "array";
  if (value === null || value === undefined) return "any";
  if (typeof value === "boolean") return "boolean";
  if (typeof value === "number") return Number.isInteger(value) ? "integer" : "number";
  if (typeof value === "object") return "object";
  return "string";
}

function dedupeVariables(variables = []) {
  const out = new Map();
  for (const variable of variables) {
    if (!variable?.expression) continue;
    out.set(variable.expression, variable);
  }
  return [...out.values()];
}

function sortVariables(a, b) {
  const scopeOrder = { input: 0, env: 1, sys: 2, node: 3 };
  const scopeDiff = (scopeOrder[a.scope] ?? 99) - (scopeOrder[b.scope] ?? 99);
  if (scopeDiff !== 0) return scopeDiff;
  return a.expression.localeCompare(b.expression);
}

function cleanKey(value = "") {
  return String(value || "").trim();
}
