import { useAssistantTransportSendCommand, useAssistantTransportState } from "@assistant-ui/react";
import { useMemo } from "react";

import {
  type AiopsTransportCommand,
  createAiopsTransportCommandActions,
  type AiopsTransportCommandActions,
} from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

export function useAiopsTransportCommands(): AiopsTransportCommandActions {
  const sendCommand = useAssistantTransportSendCommand();
  const state = useAssistantTransportState() as AiopsTransportState;
  const sendAiopsCommand = sendCommand as unknown as (command: AiopsTransportCommand) => void;

  return useMemo(() => createAiopsTransportCommandActions(state, sendAiopsCommand), [sendAiopsCommand, state]);
}
