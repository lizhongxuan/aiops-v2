import { isAiopsTransportRunning } from "@/transport/aiopsTransportConverter";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

export type StopDispatchTarget = "transport" | "runtime";

export function resolveStopDispatchTarget(
  state: AiopsTransportState,
  threadIsRunning: boolean,
): StopDispatchTarget {
  if (isAiopsTransportRunning(state) && Boolean(state.currentTurnId)) {
    return "transport";
  }
  if (threadIsRunning && Boolean(state.sessionId)) {
    return "transport";
  }
  return threadIsRunning ? "runtime" : "transport";
}
