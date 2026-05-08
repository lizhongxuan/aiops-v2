import type { AssistantTransportCommand } from "@assistant-ui/react";

import type { AiopsTransportState } from "./aiopsTransportTypes";

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
