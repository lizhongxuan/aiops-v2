import type { WorkflowGraph } from "../types/workflow";

export function graphPreviewText(graph: WorkflowGraph | null, yamlPreview: string): string {
  const compiled = yamlPreview.trim();
  if (compiled) return compiled;
  if (!graph) return "";
  return toYamlLike(graph);
}

export function toYamlLike(value: unknown, indent = 0): string {
  if (Array.isArray(value)) {
    if (value.length === 0) return "[]";
    return value.map((item) => formatArrayItem(item, indent)).join("\n");
  }

  if (isPlainObject(value)) {
    const entries = Object.entries(value).filter(([, item]) => item !== undefined);
    if (entries.length === 0) return "{}";
    return entries
      .map(([key, item]) => {
        if (isPlainObject(item) || Array.isArray(item)) {
          return `${spaces(indent)}${key}:\n${toYamlLike(item, indent + 2)}`;
        }
        return `${spaces(indent)}${key}: ${formatScalar(item)}`;
      })
      .join("\n");
  }

  return formatScalar(value);
}

export function formatGraphJSON(graph: WorkflowGraph | null): string {
  return JSON.stringify(graph ?? {}, null, 2);
}

function formatNestedValue(value: unknown, indent: number): string {
  if (isPlainObject(value) || Array.isArray(value)) {
    const rendered = toYamlLike(value, indent);
    return rendered.includes("\n") ? `\n${rendered}` : rendered;
  }
  return formatScalar(value);
}

function formatArrayItem(value: unknown, indent: number): string {
  const prefix = `${spaces(indent)}- `;
  if (isPlainObject(value)) {
    const rendered = toYamlLike(value, indent + 2);
    const [firstLine, ...rest] = rendered.split("\n");
    return `${prefix}${firstLine.trimStart()}${rest.length ? `\n${rest.join("\n")}` : ""}`;
  }
  if (Array.isArray(value)) {
    return `${spaces(indent)}-\n${toYamlLike(value, indent + 2)}`;
  }
  return `${prefix}${formatScalar(value)}`;
}

function formatScalar(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (typeof value !== "string") return JSON.stringify(value);
  if (value === "") return '""';
  if (/^[A-Za-z0-9_./:@${}-]+$/.test(value)) return value;
  return JSON.stringify(value);
}

function spaces(count: number): string {
  return " ".repeat(count);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
