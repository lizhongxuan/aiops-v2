import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostAgentChecklistSection } from "./HostAgentChecklistSection";

type HostSubagentStatusRowProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostSubagentStatusRow({ mission, state, onOpenChildAgent }: HostSubagentStatusRowProps) {
  return <HostAgentChecklistSection mission={mission} state={state} onOpenChildAgent={onOpenChildAgent} />;
}
