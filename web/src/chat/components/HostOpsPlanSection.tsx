import { useEffect, useState } from "react";

import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { TaskChecklistCard } from "./TaskChecklistCard";

type HostOpsPlanSectionProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  defaultCollapsed?: boolean;
};

type HostPlanStep = {
  id?: string;
  index?: number;
  title?: string;
  text?: string;
  status?: string;
  risk?: string;
  hostIds?: string[];
  childAgentIds?: string[];
  approvalRequired?: boolean;
};

type MissionWithPlanSteps = AiopsTransportHostMission & {
  planSteps?: HostPlanStep[];
  plan?: HostPlanStep[];
};

export function HostOpsPlanSection({ mission, defaultCollapsed = false }: HostOpsPlanSectionProps) {
  const steps = selectPlanSteps(mission);
  const [selectedStepId, setSelectedStepId] = useSelectedStep(steps);
  const selectedStep = steps.find((step, index) => stepId(step, index) === selectedStepId);

  if (steps.length === 0) {
    return null;
  }

  return (
    <div className="min-w-0">
      <TaskChecklistCard
        title="计划"
        items={steps.map((step, index) => ({
          id: stepId(step, index),
          index: step.index ?? index + 1,
          title: step.title || step.text || "未命名步骤",
          status: step.status,
        }))}
        defaultCollapsed={defaultCollapsed}
        onItemClick={(item) => setSelectedStepId(item.id)}
      />
      {selectedStep ? <PlanStepDetail step={selectedStep} /> : null}
    </div>
  );
}

function PlanStepDetail({ step }: { step: HostPlanStep }) {
  const title = step.title || step.text || "未命名步骤";
  return (
    <div
      className="mx-3 mb-2 rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-600"
      data-testid="host-plan-step-detail"
    >
      <div className="font-medium text-zinc-800">{title}</div>
      <div className="mt-1 grid gap-1">
        {step.status ? <div>状态：{step.status}</div> : null}
        {step.risk ? <div>风险：{step.risk}</div> : null}
        {step.hostIds?.length ? <div>主机：{step.hostIds.join(", ")}</div> : null}
        {step.childAgentIds?.length ? <div>Agent：{step.childAgentIds.join(", ")}</div> : null}
        {step.approvalRequired ? <div>需要审核</div> : null}
      </div>
    </div>
  );
}

function useSelectedStep(steps: HostPlanStep[]) {
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  useEffect(() => {
    if (selectedStepId && !steps.some((step, index) => stepId(step, index) === selectedStepId)) {
      setSelectedStepId(null);
    }
  }, [selectedStepId, steps]);
  return [selectedStepId, setSelectedStepId] as const;
}

function selectPlanSteps(mission: AiopsTransportHostMission): HostPlanStep[] {
  const missionWithPlan = mission as MissionWithPlanSteps;
  if (Array.isArray(missionWithPlan.planSteps)) {
    return missionWithPlan.planSteps;
  }
  if (Array.isArray(missionWithPlan.plan)) {
    return missionWithPlan.plan;
  }
  return [];
}

function stepId(step: HostPlanStep, index: number) {
  return step.id || `step-${step.index ?? index + 1}`;
}
