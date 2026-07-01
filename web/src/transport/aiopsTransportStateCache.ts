import { normalizeAiopsTransportState } from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

export type AiopsTransportCacheScope = "single_host" | "workspace" | string;

const stateByKey = new Map<string, AiopsTransportState>();
const activeSessionByScope = new Map<AiopsTransportCacheScope, string>();

export function getCachedAiopsTransportState(scope: AiopsTransportCacheScope): AiopsTransportState | null {
  const sessionId = activeSessionByScope.get(scope);
  if (!sessionId) {
    return null;
  }
  const state = stateByKey.get(cacheKey(scope, sessionId));
  return state ? cloneAiopsTransportState(state) : null;
}

export function setCachedAiopsTransportState(scope: AiopsTransportCacheScope, state: Partial<AiopsTransportState> | AiopsTransportState | null | undefined) {
  if (!state) {
    return;
  }
  const normalized = normalizeAiopsTransportState(state, state?.threadId || state?.sessionId || "default");
  const sessionId = normalized.sessionId.trim();
  if (!sessionId) {
    return;
  }
  const threadId = normalized.threadId.trim() || sessionId;
  const cached = normalizeAiopsTransportState({ ...normalized, threadId }, threadId);
  stateByKey.set(cacheKey(scope, sessionId), cloneAiopsTransportState(cached));
  activeSessionByScope.set(scope, sessionId);
}

export function clearCachedAiopsTransportState(scope: AiopsTransportCacheScope, sessionId?: string) {
  const activeSessionId = activeSessionByScope.get(scope);
  const targetSessionId = (sessionId || activeSessionId || "").trim();
  if (!targetSessionId) {
    return;
  }
  stateByKey.delete(cacheKey(scope, targetSessionId));
  if (activeSessionId === targetSessionId) {
    activeSessionByScope.delete(scope);
  }
}

export function resetAiopsTransportStateCacheForTest() {
  stateByKey.clear();
  activeSessionByScope.clear();
}

function cacheKey(scope: AiopsTransportCacheScope, sessionId: string) {
  return `${scope}:${sessionId}`;
}

function cloneAiopsTransportState(state: AiopsTransportState): AiopsTransportState {
  return JSON.parse(JSON.stringify(state)) as AiopsTransportState;
}
