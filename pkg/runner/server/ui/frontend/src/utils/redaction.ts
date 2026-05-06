const REDACTED_VALUE = "[redacted]";
const SENSITIVE_KEY_PATTERN = /(password|passwd|secret|token|api[_-]?key|apikey|access[_-]?key|private[_-]?key|credential|authorization)/i;

export function redactSensitiveValues(value: unknown): unknown {
  return redactValue(value, "");
}

export function stringifyRedacted(value: unknown): string {
  return JSON.stringify(redactSensitiveValues(value), null, 2);
}

function redactValue(value: unknown, key: string): unknown {
  if (SENSITIVE_KEY_PATTERN.test(key)) {
    return REDACTED_VALUE;
  }
  if (Array.isArray(value)) {
    return value.map((item) => redactValue(item, ""));
  }
  if (!isRecord(value)) {
    return value;
  }
  return Object.fromEntries(Object.entries(value).map(([childKey, childValue]) => [childKey, redactValue(childValue, childKey)]));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
