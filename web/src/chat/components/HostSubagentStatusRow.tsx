import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostAgentChecklistSection } from "./HostAgentChecklistSection";

type HostSubagentStatusRowProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  withDivider?: boolean;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostSubagentStatusRow({ mission, state, withDivider = true, onOpenChildAgent }: HostSubagentStatusRowProps) {
  return <HostAgentChecklistSection mission={mission} state={state} withDivider={withDivider} onOpenChildAgent={onOpenChildAgent} />;
}
