import { cn } from "@/lib/utils";
import type {
  AiopsTransportChildAgent,
  AiopsTransportHostMission,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";

import { TaskChecklistCard } from "./TaskChecklistCard";

type HostAgentChecklistSectionProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  defaultCollapsed?: boolean;
  withDivider?: boolean;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostAgentChecklistSection({
  mission,
  state,
  defaultCollapsed = false,
  withDivider = true,
  onOpenChildAgent,
}: HostAgentChecklistSectionProps) {
  const childAgents = mission.childAgentIds
    .map((childAgentId) => state.childAgents[childAgentId])
    .filter((agent): agent is AiopsTransportChildAgent => Boolean(agent));

  if (childAgents.length === 0) {
    return null;
  }

  return (
    <div className={cn(withDivider && "border-t border-zinc-200")}>
      <TaskChecklistCard
        title="主机 Agent"
        items={childAgents.map((childAgent, index) => ({
          id: childAgent.id,
          index: index + 1,
          title: formatHostLabel(childAgent),
          description: childAgent.currentStepTitle || childAgent.task || "未分配当前步骤",
          status: mapChildAgentStatus(childAgent.status),
          meta: formatStatus(childAgent.status),
          actionLabel: "打开",
          testId: `host-subagent-status-row-${childAgent.id}`,
        }))}
        defaultCollapsed={defaultCollapsed}
        summaryText={(totalCount) => `共 ${totalCount} 个主机 Agent`}
        onItemClick={(item) => onOpenChildAgent?.(item.id)}
      />
    </div>
  );
}

function formatHostLabel(childAgent: AiopsTransportChildAgent) {
  const hostName = childAgent.hostDisplayName || childAgent.hostId || "未知主机";
  const address = childAgent.hostAddress || childAgent.hostId;
  return address ? `${hostName}(@${address})` : hostName;
}

function mapChildAgentStatus(status: string) {
  switch (status) {
    case "spawning":
    case "running":
      return "running";
    case "approval_required":
      return "blocked";
    case "completed":
      return "completed";
    case "failed":
      return "failed";
    case "cancelled":
    case "canceled":
      return "cancelled";
    case "planned":
    case "waiting":
    default:
      return "pending";
  }
}

function formatStatus(status: string) {
  switch (status) {
    case "planned":
      return "已计划";
    case "spawning":
      return "启动中";
    case "running":
      return "运行中";
    case "waiting":
      return "等待中";
    case "approval_required":
      return "等待审批";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return status;
  }
}
