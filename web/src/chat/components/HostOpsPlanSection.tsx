import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

type HostOpsPlanSectionProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
};

type HostPlanStep = {
  id?: string;
  title?: string;
  text?: string;
  status?: string;
};

type MissionWithPlanSteps = AiopsTransportHostMission & {
  planSteps?: HostPlanStep[];
  plan?: HostPlanStep[];
};

const completedStatuses = new Set(["completed", "done", "success"]);

export function HostOpsPlanSection({ mission }: HostOpsPlanSectionProps) {
  const steps = selectPlanSteps(mission);
  const completedCount = steps.filter((step) => completedStatuses.has(String(step.status || "").toLowerCase())).length;

  return (
    <div className="min-w-0 px-3 py-2">
      <div className="flex min-w-0 items-center gap-2 text-xs font-medium text-zinc-700">
        <span className="shrink-0">计划</span>
        <span className="min-w-0 truncate text-zinc-500">
          共 {steps.length} 个任务，已经完成 {completedCount} 个
        </span>
      </div>
      {steps.length > 0 ? (
        <ol className="mt-1 flex min-w-0 gap-2 overflow-hidden text-xs text-zinc-500">
          {steps.slice(0, 5).map((step, index) => (
            <li key={step.id || `${index}-${step.title || step.text || "step"}`} className="min-w-0 truncate">
              {index + 1}. {step.title || step.text || "未命名任务"}
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  );
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
