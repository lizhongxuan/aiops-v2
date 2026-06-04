import type {
  AiopsTransportChildAgent,
  AiopsTransportHostMission,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";

type HostSubagentStatusRowProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostSubagentStatusRow({ mission, state, onOpenChildAgent }: HostSubagentStatusRowProps) {
  const childAgents = mission.childAgentIds
    .map((childAgentId) => state.childAgents[childAgentId])
    .filter((agent): agent is AiopsTransportChildAgent => Boolean(agent));

  if (childAgents.length === 0) {
    return null;
  }

  return (
    <div className="border-t border-zinc-200 px-3 py-2">
      <div className="mb-1 text-xs font-medium text-zinc-700">{childAgents.length} 个后台智能体</div>
      <div className="grid min-w-0 gap-1">
        {childAgents.map((childAgent) => (
          <div key={childAgent.id} className="flex min-w-0 items-center gap-2 text-xs text-zinc-600">
            <span className="size-1.5 shrink-0 rounded-full bg-emerald-500" aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate">{formatHostLabel(childAgent)}</span>
            <span className="shrink-0 text-zinc-400">{formatStatus(childAgent.status)}</span>
            <button
              type="button"
              className="shrink-0 rounded-md border border-zinc-200 px-2 py-0.5 text-xs font-medium text-zinc-700 hover:bg-zinc-50"
              onClick={() => onOpenChildAgent?.(childAgent.id)}
            >
              打开
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

function formatHostLabel(childAgent: AiopsTransportChildAgent) {
  const hostName = childAgent.hostDisplayName || childAgent.hostId || "未知主机";
  const address = childAgent.hostAddress || childAgent.hostId;
  return address ? `${hostName}(@${address})` : hostName;
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
      return "待审批";
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
