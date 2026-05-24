import type { AssistantTransportCommand } from "@assistant-ui/react";

import type { AiopsTransportState } from "./aiopsTransportTypes";
import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";

export async function postAssistantTransportCommand(
  state: AiopsTransportState,
  command: AssistantTransportCommand,
) {
  const response = await fetch("/api/v1/assistant/transport", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/plain",
    },
    body: JSON.stringify({
      state,
      threadId: state.threadId || state.sessionId,
      commands: [command],
    }),
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || `assistant transport request failed with status ${response.status}`);
  }
  return text;
}

export async function fetchAssistantTransportResumeState(sessionId: string, threadId = sessionId) {
  const state = createInitialAiopsTransportState(threadId);
  state.sessionId = sessionId;
  state.threadId = threadId;
  const response = await fetch("/api/v1/assistant/resume", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/plain",
    },
    body: JSON.stringify({
      state,
      threadId,
      commands: [],
    }),
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || `assistant resume request failed with status ${response.status}`);
  }
  return parseAssistantTransportResumeState(text);
}

export function parseAssistantTransportResumeState(text: string): AiopsTransportState | null {
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("aui-state:")) {
      continue;
    }
    const raw = trimmed.slice("aui-state:".length);
    const ops = JSON.parse(raw) as Array<{ type?: string; path?: unknown[]; value?: unknown }>;
    const fullState = ops.find((op) => op?.type === "set" && Array.isArray(op.path) && op.path.length === 0)?.value;
    if (isAiopsTransportState(fullState)) {
      return fullState;
    }
  }
  return null;
}

function isAiopsTransportState(value: unknown): value is AiopsTransportState {
  return Boolean(
    value &&
      typeof value === "object" &&
      (value as { schemaVersion?: unknown }).schemaVersion === "aiops.transport.v2" &&
      typeof (value as { sessionId?: unknown }).sessionId === "string" &&
      typeof (value as { threadId?: unknown }).threadId === "string",
  );
}
