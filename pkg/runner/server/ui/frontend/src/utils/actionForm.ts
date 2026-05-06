import type { ActionSpec, JsonSchema, WorkflowDefinition, WorkflowHandler, WorkflowNode, WorkflowStep } from "../types/workflow";

export type ActionFieldKind = "string" | "multiline" | "boolean" | "string-array" | "env" | "json";

export interface ActionArgField {
  key: string;
  title: string;
  description?: string;
  kind: ActionFieldKind;
  required: boolean;
}

export interface ActionFieldIssue {
  field: string;
  severity: "error" | "warning";
  code: string;
  message: string;
  suggestion: string;
}

export interface TargetOption {
  label: string;
  value: string;
  targetType: "host" | "group" | "adhoc" | "local";
  capabilities?: string[];
}

const commonActionSchemas: Record<string, JsonSchema> = {
  "cmd.run": {
    type: "object",
    required: ["cmd"],
    properties: {
      cmd: { type: "string", title: "Command", description: "Command passed to /bin/sh -c." },
      dir: { type: "string", title: "Working directory" },
      env: envObjectSchema(),
    },
  },
  "shell.run": {
    type: "object",
    required: ["script"],
    properties: {
      script: { type: "string", title: "Script", description: "Shell script content." },
      dir: { type: "string", title: "Working directory" },
      env: envObjectSchema(),
      export_vars: { type: "boolean", title: "Export variables", description: "Parse RUNNER_EXPORT_* lines from stdout." },
    },
  },
  "script.shell": scriptSchema("Shell script"),
  "script.python": scriptSchema("Python script"),
};

export function findActionSpec(actions: ActionSpec[], action?: string): ActionSpec | undefined {
  if (!action) return undefined;
  return actions.find((spec) => spec.action === action);
}

export function getActionArgFields(spec: ActionSpec | undefined, action?: string): ActionArgField[] {
  const schema = actionSchema(spec, action);
  const properties = schema?.properties || {};
  const required = new Set([...(schema?.required || []), ...(spec?.required_args || [])]);

  return Object.entries(properties).map(([key, property]) => ({
    key,
    title: property.title || humanizeKey(key),
    description: property.description,
    kind: fieldKind(key, property),
    required: required.has(key),
  }));
}

export function validateActionArgs(spec: ActionSpec | undefined, action: string | undefined, args: Record<string, unknown> | undefined): ActionFieldIssue[] {
  const schema = actionSchema(spec, action);
  const properties = schema?.properties || {};
  const required = new Set([...(schema?.required || []), ...(spec?.required_args || [])]);
  const values = args || {};
  const issues: ActionFieldIssue[] = [];

  for (const key of required) {
    if (isEmptyArg(values[key])) {
      issues.push({
        field: key,
        severity: "error",
        code: "required_arg_missing",
        message: `${humanizeKey(key)} is required.`,
        suggestion: `Provide ${key} before validating, dry-running, or running this workflow.`,
      });
    }
  }

  for (const [key, value] of Object.entries(values)) {
    if (value === undefined || value === null) continue;
    const property = properties[key];
    if (!property) {
      if (schema?.additionalProperties === false) {
        issues.push({
          field: key,
          severity: "warning",
          code: "unknown_arg",
          message: `${humanizeKey(key)} is not defined by the action schema.`,
          suggestion: "Remove this field or update the action catalog schema.",
        });
      }
      continue;
    }
    issues.push(...validateJSONSchemaValue(key, value, property));
  }

  if ((action === "script.shell" || action === "script.python") && isEmptyArg(values.script_ref) && isEmptyArg(values.script)) {
    issues.push({
      field: "script_ref",
      severity: "error",
      code: "script_reference_missing",
      message: "Stored script or inline script is required.",
      suggestion: "Set script_ref for a managed script, or provide inline script content.",
    });
  }

  return issues;
}

export function getTargetOptions(workflow: WorkflowDefinition | null | undefined, currentTargets: string[] = []): TargetOption[] {
  const inventory = inventoryRecord(workflow);
  const groups = recordValue(inventory.groups) || {};
  const hosts = recordValue(inventory.hosts) || {};
  const options = new Map<string, TargetOption>();

  options.set("local", { label: "local", value: "local", targetType: "local" });
  for (const [name, value] of Object.entries(groups)) {
    options.set(name, {
      label: `${name} (group)`,
      value: name,
      targetType: "group",
      capabilities: capabilitiesFromInventoryItem(value),
    });
  }
  for (const [name, value] of Object.entries(hosts)) {
    options.set(name, {
      label: `${name} (host)`,
      value: name,
      targetType: "host",
      capabilities: capabilitiesFromInventoryItem(value),
    });
  }
  for (const target of currentTargets) {
    if (!target || options.has(target)) continue;
    options.set(target, { label: `${target} (custom)`, value: target, targetType: "adhoc" });
  }

  return [...options.values()].sort((left, right) => targetTypeRank(left.targetType) - targetTypeRank(right.targetType) || left.value.localeCompare(right.value));
}

export function validateTargets(workflow: WorkflowDefinition | null | undefined, action: string | undefined, targets: string[]): ActionFieldIssue[] {
  const normalizedTargets = normalizeStringList(targets);
  if (normalizedTargets.length === 0) {
    return [
      {
        field: "targets",
        severity: "warning",
        code: "target_missing",
        message: "Targets are not explicit.",
        suggestion: "Select inventory hosts or groups so dry-run and runtime scope are predictable.",
      },
    ];
  }

  const optionsByValue = new Map(getTargetOptions(workflow, normalizedTargets).map((option) => [option.value, option]));
  const issues: ActionFieldIssue[] = [];
  for (const target of normalizedTargets) {
    const option = optionsByValue.get(target);
    if (!option || option.targetType === "adhoc") {
      issues.push({
        field: "targets",
        severity: "warning",
        code: "target_not_in_inventory",
        message: `Target ${target} is not declared in inventory.`,
        suggestion: "Declare the host or group in workflow inventory before production runs.",
      });
      continue;
    }
    if (!action || option.targetType === "local" || !option.capabilities?.length) continue;
    if (!option.capabilities.includes(action)) {
      issues.push({
        field: "targets",
        severity: "warning",
        code: "capability_mismatch",
        message: `Target ${target} does not advertise ${action}.`,
        suggestion: "Choose a target with matching capabilities, or update the agent capability registry.",
      });
    }
  }
  return issues;
}

export function nodeAction(node: WorkflowNode | null | undefined): string {
  if (!node) return "cmd.run";
  if (node.type === "handler") return node.handler?.action || "cmd.run";
  return node.step?.action || "cmd.run";
}

export function nodeExecutableName(node: WorkflowNode | null | undefined): string {
  if (!node) return "";
  if (node.type === "handler") return node.handler?.name || node.handler_name || node.id;
  return node.step?.name || node.step_name || node.id;
}

export function nodeArgs(node: WorkflowNode | null | undefined): Record<string, unknown> | undefined {
  if (!node) return undefined;
  if (node.type === "handler") return node.handler?.args;
  return node.step?.args;
}

export function createStepPatch(node: WorkflowNode, patch: Partial<WorkflowStep | WorkflowHandler>): Partial<WorkflowNode> {
  if (node.type === "handler") {
    return createHandlerPatch(node, patch);
  }
  const baseStep = ensureStep(node);
  const step = compactStep({
    ...baseStep,
    ...patch,
    args: patch.args === undefined ? baseStep.args : patch.args,
  });
  return {
    step,
    step_name: step.name || node.step_name,
  };
}

export function createArgPatch(node: WorkflowNode, key: string, value: unknown): Partial<WorkflowNode> {
  if (node.type === "handler") {
    const handler = ensureHandler(node);
    return createHandlerPatch(node, {
      args: compactRecord({
        ...(handler.args || {}),
        [key]: value,
      }),
    });
  }
  const step = ensureStep(node);
  return createStepPatch(node, {
    args: compactRecord({
      ...(step.args || {}),
      [key]: value,
    }),
  });
}

export function createSubflowPatch(node: WorkflowNode, patch: { workflow_name?: string; vars?: Record<string, unknown> }): Partial<WorkflowNode> {
  const baseStep = ensureStep(node);
  const workflowName = patch.workflow_name === undefined ? readSubflowWorkflowName(node) : patch.workflow_name.trim();
  const vars = patch.vars === undefined ? readSubflowVars(node) : patch.vars;
  const hasVars = Object.keys(vars).length > 0;
  const step = compactStep({
    ...baseStep,
    action: "workflow.run",
    args: compactRecord({
      ...(baseStep.args || {}),
      workflow: workflowName || undefined,
      vars: hasVars ? vars : undefined,
    }),
  });
  const subflow = compactObject({
    ...(node.subflow || {}),
    workflow_name: workflowName || undefined,
    vars: hasVars ? vars : undefined,
  }) as NonNullable<WorkflowNode["subflow"]>;
  return {
    step,
    step_name: step.name || node.step_name,
    subflow: Object.keys(subflow).length > 0 ? subflow : undefined,
  };
}

export function replaceStepFromJSON(node: WorkflowNode, value: string): Partial<WorkflowNode> {
  const parsed = JSON.parse(value) as WorkflowStep | WorkflowHandler;
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(node.type === "handler" ? "Handler JSON must be an object." : "Step JSON must be an object.");
  }
  return createStepPatch(node, parsed);
}

export function readStepTargets(step: WorkflowStep | undefined): string[] {
  if (!step) return [];
  if (Array.isArray(step.targets)) return normalizeStringList(step.targets);
  if (Array.isArray(step.target)) return normalizeStringList(step.target);
  if (typeof step.target === "string") return normalizeStringList([step.target]);
  return [];
}

export function readSubflowWorkflowName(node: WorkflowNode | null | undefined): string {
  if (!node) return "";
  const args = nodeArgs(node);
  return firstString(node.subflow?.workflow_name, args?.workflow);
}

export function readSubflowVars(node: WorkflowNode | null | undefined): Record<string, unknown> {
  if (!node) return {};
  const args = nodeArgs(node);
  return recordValue(node.subflow?.vars) || recordValue(args?.vars) || {};
}

export function parseListText(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function normalizeStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}

export function objectToEnvText(value: unknown): string {
  if (!isRecord(value)) return "";
  return Object.entries(value)
    .map(([key, item]) => `${key}=${String(item ?? "")}`)
    .join("\n");
}

export function envTextToObject(value: string): Record<string, string> {
  const entries: Record<string, string> = {};
  for (const rawLine of value.split("\n")) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const splitAt = line.indexOf("=");
    const key = (splitAt === -1 ? line : line.slice(0, splitAt)).trim();
    if (!key) continue;
    entries[key] = splitAt === -1 ? "" : line.slice(splitAt + 1);
  }
  return entries;
}

export function formatJSON(value: unknown): string {
  return JSON.stringify(value ?? {}, null, 2);
}

function ensureStep(node: WorkflowNode): WorkflowStep {
  return {
    name: node.step?.name || node.step_name || node.id,
    action: node.step?.action || "cmd.run",
    args: {},
    ...node.step,
  };
}

function ensureHandler(node: WorkflowNode): WorkflowHandler {
  return {
    name: node.handler?.name || node.handler_name || node.id,
    action: node.handler?.action || "cmd.run",
    args: {},
    ...node.handler,
  };
}

function createHandlerPatch(node: WorkflowNode, patch: Partial<WorkflowStep | WorkflowHandler>): Partial<WorkflowNode> {
  const baseHandler = ensureHandler(node);
  const handler = compactHandler({
    ...baseHandler,
    name: typeof patch.name === "string" ? patch.name : baseHandler.name,
    action: typeof patch.action === "string" ? patch.action : baseHandler.action,
    args: patch.args === undefined ? baseHandler.args : patch.args,
  });
  return {
    handler,
    handler_name: handler.name || node.handler_name,
  };
}

function compactStep(step: WorkflowStep): WorkflowStep {
  return compactObject(step) as WorkflowStep;
}

function compactHandler(handler: WorkflowHandler): WorkflowHandler {
  return compactObject(handler) as WorkflowHandler;
}

function compactRecord<T extends Record<string, unknown>>(value: T): T {
  return compactObject(value) as T;
}

function compactObject(value: object): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value)) {
    if (item === undefined || item === null) continue;
    if (Array.isArray(item) && item.length === 0) continue;
    if (isRecord(item) && Object.keys(item).length === 0) continue;
    out[key] = item;
  }
  return out;
}

function fieldKind(key: string, schema: JsonSchema): ActionFieldKind {
  if (key === "env") return "env";
  if (schema.type === "boolean") return "boolean";
  if (schema.type === "array" && schema.items?.type === "string") return "string-array";
  if (schema.type === "object") return "json";
  if (key === "script" || key === "cmd") return "multiline";
  return "string";
}

function actionSchema(spec: ActionSpec | undefined, action?: string): JsonSchema | undefined {
  return spec?.args_schema || (action ? commonActionSchemas[action] : undefined);
}

function inventoryRecord(workflow: WorkflowDefinition | null | undefined): Record<string, unknown> {
  return recordValue(workflow?.inventory) || {};
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return isRecord(value) ? value : undefined;
}

function capabilitiesFromInventoryItem(value: unknown): string[] | undefined {
  const item = recordValue(value);
  const vars = recordValue(item?.vars);
  return firstStringArray(vars?.capabilities, vars?.runner_capabilities, item?.capabilities, item?.runner_capabilities);
}

function firstStringArray(...values: unknown[]): string[] | undefined {
  for (const value of values) {
    const list = normalizeStringList(value);
    if (list.length > 0) return list;
  }
  return undefined;
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function targetTypeRank(type: TargetOption["targetType"]): number {
  if (type === "local") return 0;
  if (type === "group") return 1;
  if (type === "host") return 2;
  return 3;
}

function validateJSONSchemaValue(field: string, value: unknown, schema: JsonSchema): ActionFieldIssue[] {
  const issues: ActionFieldIssue[] = [];
  const type = Array.isArray(schema.type) ? schema.type.find((item) => item !== "null") : schema.type;

  if (schema.enum?.length && !schema.enum.some((item) => JSON.stringify(item) === JSON.stringify(value))) {
    issues.push({
      field,
      severity: "error",
      code: "enum_mismatch",
      message: `${humanizeKey(field)} must be one of ${schema.enum.map(String).join(", ")}.`,
      suggestion: "Choose one of the allowed values from the action schema.",
    });
  }

  if (type === "string") {
    if (typeof value !== "string") {
      issues.push(typeIssue(field, "string"));
    } else if (schema.minLength !== undefined && value.length < schema.minLength) {
      issues.push({
        field,
        severity: "error",
        code: "min_length",
        message: `${humanizeKey(field)} is too short.`,
        suggestion: `Use at least ${schema.minLength} characters.`,
      });
    }
  } else if (type === "boolean" && typeof value !== "boolean") {
    issues.push(typeIssue(field, "boolean"));
  } else if (type === "array") {
    if (!Array.isArray(value)) {
      issues.push(typeIssue(field, "array"));
    } else if (schema.items?.type === "string" && value.some((item) => typeof item !== "string")) {
      issues.push({
        field,
        severity: "error",
        code: "array_item_type",
        message: `${humanizeKey(field)} only accepts strings.`,
        suggestion: "Remove non-string values from the list.",
      });
    }
  } else if (type === "object") {
    if (!isRecord(value)) {
      issues.push(typeIssue(field, "object"));
    } else if (schema.additionalProperties && typeof schema.additionalProperties === "object") {
      const valueType = Array.isArray(schema.additionalProperties.type)
        ? schema.additionalProperties.type.find((item) => item !== "null")
        : schema.additionalProperties.type;
      if (valueType === "string") {
        for (const [key, item] of Object.entries(value)) {
          if (typeof item !== "string") {
            issues.push({
              field,
              severity: "warning",
              code: "object_value_type",
              message: `${field}.${key} should be a string.`,
              suggestion: "Convert the value to a string before saving.",
            });
          }
        }
      }
    }
  } else if ((type === "number" || type === "integer") && typeof value !== "number") {
    issues.push(typeIssue(field, type));
  }

  return issues;
}

function typeIssue(field: string, expected: string): ActionFieldIssue {
  return {
    field,
    severity: "error",
    code: "type_mismatch",
    message: `${humanizeKey(field)} must be ${expected}.`,
    suggestion: "Fix the value type in the field or Advanced JSON.",
  };
}

function isEmptyArg(value: unknown): boolean {
  return value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0);
}

function humanizeKey(key: string): string {
  return key
    .split("_")
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(" ");
}

function scriptSchema(scriptTitle: string): JsonSchema {
  return {
    type: "object",
    properties: {
      script_ref: { type: "string", title: "Stored script" },
      script: { type: "string", title: scriptTitle },
      args: { type: "array", title: "Arguments", items: { type: "string" } },
      dir: { type: "string", title: "Working directory" },
      env: envObjectSchema(),
      export_vars: { type: "boolean", title: "Export variables" },
    },
  };
}

function envObjectSchema(): JsonSchema {
  return {
    type: "object",
    title: "Environment",
    additionalProperties: { type: "string" },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
