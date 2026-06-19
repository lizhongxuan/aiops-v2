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
  const hasPlanSteps = selectPlanSteps(mission).length > 0;
  if (!hasPlanSteps && !hasChildAgents(mission, state) && !hasMissionHosts(mission)) {
    return null;
  }

  return (
    <section
      className="mx-auto -mb-8 w-[calc(100%-4rem)] max-w-[44.5rem] min-w-0 overflow-hidden rounded-t-[1.15rem] rounded-b-none border border-b-0 border-slate-200 bg-white pb-8 max-[520px]:w-[calc(100%-2.5rem)]"
      data-testid="host-ops-status-panel"
    >
      <HostOpsPlanSection mission={mission} state={state} />
      <HostSubagentStatusRow mission={mission} state={state} withDivider={hasPlanSteps} onOpenChildAgent={onOpenChildAgent} />
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

function hasChildAgents(mission: AiopsTransportHostMission, state: AiopsTransportState) {
  return (mission.childAgentIds || []).some((childAgentId) => Boolean((state.childAgents || {})[childAgentId]));
}

function hasMissionHosts(mission: AiopsTransportHostMission) {
  return selectMissionMentions(mission).length > 0;
}

function selectPlanSteps(mission: AiopsTransportHostMission) {
  const missionWithPlan = mission as AiopsTransportHostMission & {
    planSteps?: unknown;
    plan?: unknown;
  };
  if (Array.isArray(missionWithPlan.planSteps)) {
    return missionWithPlan.planSteps;
  }
  if (Array.isArray(missionWithPlan.plan)) {
    return missionWithPlan.plan;
  }
  return [];
}

function selectMissionMentions(mission: AiopsTransportHostMission) {
  const missionWithMentions = mission as AiopsTransportHostMission & {
    mentions?: unknown;
  };
  if (Array.isArray(mission.mentionedHosts)) {
    return mission.mentionedHosts;
  }
  if (Array.isArray(missionWithMentions.mentions)) {
    return missionWithMentions.mentions;
  }
  return [];
}
