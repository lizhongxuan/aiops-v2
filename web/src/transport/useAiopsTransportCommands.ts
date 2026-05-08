import { useAssistantTransportSendCommand, useAssistantTransportState } from "@assistant-ui/react";
import { useMemo } from "react";

import {
  createAiopsTransportCommandActions,
  type AiopsTransportCommandActions,
} from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

export function useAiopsTransportCommands(): AiopsTransportCommandActions {
  const sendCommand = useAssistantTransportSendCommand();
  const state = useAssistantTransportState() as AiopsTransportState;

  return useMemo(() => createAiopsTransportCommandActions(state, sendCommand), [sendCommand, state]);
}
