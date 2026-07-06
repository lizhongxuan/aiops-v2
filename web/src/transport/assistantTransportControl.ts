import type { AssistantTransportCommand } from "@assistant-ui/react";

import type { AiopsTransportState } from "./aiopsTransportTypes";
import { createInitialAiopsTransportState, isCurrentAiopsTransportState, normalizeAiopsTransportState } from "./aiopsTransportRuntime";
import { toUserFacingTransportErrorMessage } from "./transportErrorMessage";

export async function postAssistantTransportCommand(
  state: AiopsTransportState,
  command: AssistantTransportCommand,
) {
  const response = await fetchAssistantTransport("/api/v1/assistant/transport", {
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
    throw new Error(toUserFacingTransportErrorMessage(text || `assistant transport request failed with status ${response.status}`));
  }
  return text;
}

export async function fetchAssistantTransportResumeState(sessionId: string, threadId = sessionId) {
  const state = createInitialAiopsTransportState(threadId);
  state.sessionId = sessionId;
  state.threadId = threadId;
  const response = await fetchAssistantTransport("/api/v1/assistant/resume", {
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
    throw new Error(toUserFacingTransportErrorMessage(text || `assistant resume request failed with status ${response.status}`));
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
    if (isCurrentAiopsTransportState(fullState)) {
      return normalizeAiopsTransportState(fullState);
    }
  }
  return null;
}

async function fetchAssistantTransport(input: RequestInfo | URL, init: RequestInit): Promise<Response> {
  try {
    return await fetch(input, init);
  } catch (error) {
    throw new Error(toUserFacingTransportErrorMessage(error));
  }
}
