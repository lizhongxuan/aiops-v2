export const AGENT_EVENT_KINDS = Object.freeze([
  "turn",
  "agent",
  "assistant",
  "tool",
  "approval",
  "artifact",
  "diff",
  "browser",
  "plan",
  "evidence",
  "system",
]);
export const AGENT_EVENT_PHASES = Object.freeze(["requested", "started", "delta", "updated", "completed", "failed", "canceled", "blocked", "resolved"]);
export const AGENT_EVENT_STATUSES = Object.freeze(["queued", "running", "waiting", "blocked", "completed", "failed", "canceled", "skipped"]);
export const AGENT_EVENT_VISIBILITIES = Object.freeze(["primary", "secondary", "debug", "hidden"]);

const kindSet = new Set(AGENT_EVENT_KINDS);
const phaseSet = new Set(AGENT_EVENT_PHASES);
const statusSet = new Set(AGENT_EVENT_STATUSES);
const visibilitySet = new Set(AGENT_EVENT_VISIBILITIES);

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function coerceIsoStamp(value) {
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? new Date(stamp).toISOString() : new Date().toISOString();
}

export function normalizeAgentEvent(input = {}) {
  const source = asObject(input);
  const eventId = compactText(source.eventId);
  const sessionId = compactText(source.sessionId);
  const kind = compactText(source.kind);
  const phase = compactText(source.phase);
  const status = compactText(source.status);
  const visibility = compactText(source.visibility);
  const sourceName = compactText(source.source);
  if (!eventId || !sessionId || !kind || !phase || !status || !visibility || !sourceName) return null;
  if (!kindSet.has(kind) || !phaseSet.has(phase) || !statusSet.has(status) || !visibilitySet.has(visibility)) return null;
  return {
    eventId,
    seq: Number.isFinite(Number(source.seq)) ? Number(source.seq) : 0,
    sessionId,
    threadId: compactText(source.threadId),
    turnId: compactText(source.turnId),
    clientTurnId: compactText(source.clientTurnId),
    agentId: compactText(source.agentId),
    parentAgentId: compactText(source.parentAgentId),
    kind,
    phase,
    status,
    visibility,
    source: sourceName,
    createdAt: coerceIsoStamp(source.createdAt),
    startedAt: compactText(source.startedAt),
    completedAt: compactText(source.completedAt),
    durationMs: Number.isFinite(Number(source.durationMs)) ? Number(source.durationMs) : 0,
    payload: asObject(source.payload),
  };
}
