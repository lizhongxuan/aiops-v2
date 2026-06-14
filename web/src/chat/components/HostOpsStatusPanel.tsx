import { useAssistantTransportState } from "@assistant-ui/react";

import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostOpsPlanSection } from "./HostOpsPlanSection";
import { HostSubagentStatusRow } from "./HostSubagentStatusRow";

type HostOpsStatusPanelProps = {
  state?: AiopsTransportState;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostOpsStatusPanel({ state: stateProp, onOpenChildAgent }: HostOpsStatusPanelProps) {
  if (stateProp) {
    return <HostOpsStatusPanelView state={stateProp} onOpenChildAgent={onOpenChildAgent} />;
  }

  return <ConnectedHostOpsStatusPanel onOpenChildAgent={onOpenChildAgent} />;
}

function ConnectedHostOpsStatusPanel({ onOpenChildAgent }: Pick<HostOpsStatusPanelProps, "onOpenChildAgent">) {
  const state = useAssistantTransportState() as AiopsTransportState;

  return <HostOpsStatusPanelView state={state} onOpenChildAgent={onOpenChildAgent} />;
}

function HostOpsStatusPanelView({ state, onOpenChildAgent }: Required<Pick<HostOpsStatusPanelProps, "state">> & Pick<HostOpsStatusPanelProps, "onOpenChildAgent">) {
  const mission = selectActiveHostMission(state);

  if (!mission) {
    return null;
  }

  return (
    <section
      className="mx-auto -mb-8 w-[calc(100%-2rem)] max-w-[46.5rem] overflow-hidden rounded-t-[1.15rem] rounded-b-none border border-b-0 border-slate-200 bg-white pb-8"
      data-testid="host-ops-status-panel"
    >
      <HostOpsPlanSection mission={mission} state={state} />
      <HostSubagentStatusRow mission={mission} state={state} onOpenChildAgent={onOpenChildAgent} />
    </section>
  );
}

export function selectActiveHostMission(state: AiopsTransportState): AiopsTransportHostMission | undefined {
  const hostMissions = state.hostMissions || {};
  if (state.activeHostMissionId) {
    return hostMissions[state.activeHostMissionId];
  }

  return Object.values(hostMissions).find((mission) =>
    !["completed", "failed", "cancelled"].includes(String(mission.status)),
  );
}
