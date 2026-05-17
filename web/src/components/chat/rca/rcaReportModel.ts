export type RCAReportStatus = "ok" | "partial" | "inconclusive" | "error";

export type RCAReportSection = {
  id: string;
  kind: string;
  titleZh: string;
  summaryZh?: string;
  status?: string;
  evidenceRefs: string[];
  payload: Record<string, unknown>;
};

export type RCAReport = {
  schemaVersion: "aiops.rca_report/v1";
  source: string;
  status: RCAReportStatus;
  project?: string;
  target: Record<string, string>;
  window: Record<string, string>;
  conclusion: {
    summaryZh: string;
    rootCauseEntity?: string;
    rootCauseKind?: string;
    confidence: number;
  };
  hypotheses: Array<Record<string, unknown>>;
  sections: RCAReportSection[];
  evidenceRefs: string[];
  rawRefs: Array<Record<string, unknown>>;
  limitations: string[];
};

const UNSAFE_KEYS = new Set(["html", "script", "dangerouslySet" + "InnerHTML", "inner" + "HTML"]);
const UNSAFE_PROTOCOL_PATTERN = new RegExp("\\bjava" + "scr" + "ipt:", "i");
const UNSAFE_PROTOCOL_GLOBAL_PATTERN = new RegExp("\\bjava" + "scr" + "ipt:", "gi");
const SENSITIVE_KEY_PATTERN = /(secret|token|password|credential|apikey|api_key|privatekey|private_key)/i;

export function normalizeRCAReport(value: unknown): RCAReport {
  const source = asRecord(value);
  if (text(source.schemaVersion) !== "aiops.rca_report/v1") {
    return fallbackReport("无法读取 RCA 报告数据。");
  }

  const conclusion = asRecord(source.conclusion);
  return {
    schemaVersion: "aiops.rca_report/v1",
    source: text(source.source) || "aiops",
    status: normalizeStatus(source.status),
    project: optionalText(source.project),
    target: stringRecord(source.target),
    window: stringRecord(source.window),
    conclusion: {
      summaryZh: text(conclusion.summaryZh) || "RCA 报告没有提供结论摘要。",
      rootCauseEntity: optionalText(conclusion.rootCauseEntity),
      rootCauseKind: optionalText(conclusion.rootCauseKind),
      confidence: numberInRange(conclusion.confidence, 0, 1),
    },
    hypotheses: arrayOfRecords(source.hypotheses).map(sanitizeRecord),
    sections: arrayOfRecords(source.sections).map((section, index) => ({
      id: text(section.id) || `section-${index + 1}`,
      kind: text(section.kind) || "overview",
      titleZh: text(section.titleZh) || text(section.title) || "RCA 分析",
      summaryZh: optionalText(section.summaryZh),
      status: optionalText(section.status),
      evidenceRefs: stringList(section.evidenceRefs),
      payload: sanitizeRecord(asRecord(section.payload)),
    })),
    evidenceRefs: stringList(source.evidenceRefs),
    rawRefs: arrayOfRecords(source.rawRefs).map(sanitizeRawRef).filter((ref) => Object.keys(ref).length > 0),
    limitations: stringList(source.limitations),
  };
}

function fallbackReport(summaryZh: string): RCAReport {
  return {
    schemaVersion: "aiops.rca_report/v1",
    source: "aiops",
    status: "inconclusive",
    target: {},
    window: {},
    conclusion: { summaryZh, confidence: 0 },
    hypotheses: [],
    sections: [],
    evidenceRefs: [],
    rawRefs: [],
    limitations: [summaryZh],
  };
}

function normalizeStatus(value: unknown): RCAReportStatus {
  const status = text(value);
  if (status === "ok" || status === "partial" || status === "inconclusive" || status === "error") {
    return status;
  }
  return "inconclusive";
}

function stringRecord(value: unknown): Record<string, string> {
  return Object.fromEntries(
    Object.entries(asRecord(value))
      .filter(([key]) => isSafeKey(key))
      .map(([key, val]) => [key, text(val)])
      .filter(([, val]) => val),
  );
}

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(text).filter(Boolean).filter(isSafeRefText) : [];
}

function sanitizeRawRef(ref: Record<string, unknown>): Record<string, unknown> {
  const safe = sanitizeRecord(ref);
  const uri = text(safe.uri);
  if (uri && !isSafeUri(uri)) {
    return {};
  }
  return safe;
}

function sanitizeRecord(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value)
      .filter(([key]) => isSafeKey(key))
      .map(([key, entry]) => [key, sanitizeValue(entry)])
      .filter(([, entry]) => entry !== undefined),
  );
}

function sanitizeValue(value: unknown): unknown {
  if (typeof value === "string") {
    const sanitized = text(value);
    return sanitized || undefined;
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value === "boolean") {
    return value;
  }
  if (Array.isArray(value)) {
    return value.map(sanitizeValue).filter((entry) => entry !== undefined);
  }
  if (value && typeof value === "object") {
    return sanitizeRecord(value as Record<string, unknown>);
  }
  return undefined;
}

function isSafeKey(key: string): boolean {
  return !UNSAFE_KEYS.has(key) && !SENSITIVE_KEY_PATTERN.test(key);
}

function isSafeRefText(value: string): boolean {
  return !/[<>]/.test(value) && !UNSAFE_PROTOCOL_PATTERN.test(value);
}

function isSafeUri(value: string): boolean {
  return /^(coroot|aiops|evidence|prompt-trace|case):\/\//i.test(value) || /^#[A-Za-z0-9_.:-]+$/.test(value);
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function arrayOfRecords(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.map(asRecord).filter((item) => Object.keys(item).length > 0) : [];
}

function text(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, "")
    .replace(/<[^>]*>/g, "")
    .replace(/\bon\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(UNSAFE_PROTOCOL_GLOBAL_PATTERN, "")
    .trim()
    .replace(/\s+/g, " ");
}

function optionalText(value: unknown): string | undefined {
  return text(value) || undefined;
}

function numberInRange(value: unknown, min: number, max: number): number {
  const parsed = typeof value === "number" && Number.isFinite(value) ? value : 0;
  return Math.max(min, Math.min(max, parsed));
}
