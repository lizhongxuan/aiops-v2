import { useState } from "react";
import { ChevronDownIcon } from "lucide-react";

import { cn } from "@/lib/utils";
import type { AiopsTransportHostMission, AiopsTransportState } from "@/transport/aiopsTransportTypes";

type HostSubagentStatusRowProps = {
  mission: AiopsTransportHostMission;
  state: AiopsTransportState;
  withDivider?: boolean;
  onOpenChildAgent?: (childAgentId: string) => void;
};

export function HostSubagentStatusRow({ mission, state, withDivider = true, onOpenChildAgent }: HostSubagentStatusRowProps) {
  const [collapsed, setCollapsed] = useState(false);
  const childAgentsById = state.childAgents || {};
  const childAgents = (mission.childAgentIds || [])
    .map((childAgentId) => childAgentsById[childAgentId])
    .filter(Boolean);
  const childHostKeys = new Set(childAgents.map((childAgent) => normalizedHostKey(childAgent.hostId || childAgent.hostAddress)));
  const pendingHosts = uniqueMissionMentions(mission).filter((mention) => {
    const key = hostMentionKey(mention);
    return key && !childHostKeys.has(key);
  });
  const totalCount = childAgents.length + pendingHosts.length;

  if (totalCount === 0) {
    return null;
  }

  return (
    <div className={withDivider ? "border-t border-zinc-200" : ""}>
      <section className="min-w-0 px-4 py-1.5 text-[12px]" data-testid="host-subagent-list-card">
        <button
          type="button"
          className="flex w-full min-w-0 items-center gap-2 text-left"
          aria-expanded={!collapsed}
          data-testid="host-subagent-row-toggle"
          onClick={() => setCollapsed((value) => !value)}
        >
          <ChevronDownIcon
            className={cn("size-3 shrink-0 text-zinc-500 transition-transform", collapsed ? "-rotate-90" : "rotate-0")}
            aria-hidden="true"
          />
          <span className="shrink-0 font-semibold text-zinc-700">主机 Agent</span>
          <span className="min-w-0 truncate text-zinc-500">共 {totalCount} 个主机 Agent</span>
        </button>
        {!collapsed ? <ol className="mt-1 grid min-w-0 gap-0.5">
          {childAgents.map((childAgent, index) => (
            <li
              key={childAgent.id}
              className="flex min-h-6 min-w-0 items-center gap-2 rounded-md px-1.5 py-0.5"
              data-testid={`host-child-agent-${childAgent.id}`}
            >
              <span className="w-4 shrink-0 text-right text-[11px] text-zinc-400">
                {index + 1}.
              </span>
              <button
                type="button"
                className={cn(
                  "max-w-[9rem] shrink-0 truncate rounded px-1.5 py-0.5 text-left font-semibold hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400",
                  hostNameTone(index),
                )}
                title={formatVerboseHostLabel(childAgent)}
                aria-label={`${formatCompactHostLabel(childAgent)} 主机详情`}
                data-testid={`host-child-agent-name-${childAgent.id}`}
                onClick={() => onOpenChildAgent?.(childAgent.id)}
              >
                {formatCompactHostLabel(childAgent)}
              </button>
              <span className="min-w-0 flex-1 truncate text-zinc-500">
                {childAgent.currentStepTitle || childAgent.task || "未分配当前步骤"}
              </span>
              <span
                className={cn("shrink-0 rounded px-1.5 py-0.5 font-mono text-[11px] ring-1", statusTone(childAgent.status))}
                data-testid={`host-child-agent-status-${childAgent.id}`}
              >
                {childAgent.status}
              </span>
            </li>
          ))}
          {pendingHosts.map((mention, index) => (
            <li
              key={hostMentionKey(mention)}
              className="flex min-h-6 min-w-0 items-center gap-2 rounded-md px-1.5 py-0.5"
              data-testid={`host-child-agent-pending-${safeTestId(hostMentionKey(mention))}`}
            >
              <span className="w-4 shrink-0 text-right text-[11px] text-zinc-400">
                {childAgents.length + index + 1}.
              </span>
              <span
                className={cn("max-w-[9rem] shrink-0 truncate rounded px-1.5 py-0.5 font-semibold", hostNameTone(childAgents.length + index))}
                title={formatMentionLabel(mention)}
              >
                {formatMentionHandle(mention)}
              </span>
              <span className="min-w-0 flex-1 truncate text-zinc-500">等待分配子 Agent</span>
              <span
                className={cn("shrink-0 rounded px-1.5 py-0.5 font-mono text-[11px] ring-1", statusTone("planned"))}
                data-testid={`host-child-agent-status-pending-${safeTestId(hostMentionKey(mention))}`}
              >
                planned
              </span>
            </li>
          ))}
        </ol> : null}
      </section>
    </div>
  );
}

type MissionMention = {
  hostId?: string;
  address?: string;
  displayName?: string;
  raw?: string;
  resolved?: boolean;
};

type ChildAgentRow = {
  id: string;
  hostId?: string;
  hostAddress?: string;
  hostDisplayName?: string;
  currentStepTitle?: string;
  task?: string;
  status: string;
};

function uniqueMissionMentions(mission: AiopsTransportHostMission): MissionMention[] {
  const mentions = selectMissionMentions(mission);
  const seen = new Set<string>();
  const unique: MissionMention[] = [];
  for (const mention of mentions) {
    if (mention.resolved === false) {
      continue;
    }
    const key = hostMentionKey(mention);
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    unique.push(mention);
  }
  return unique;
}

function selectMissionMentions(mission: AiopsTransportHostMission): MissionMention[] {
  const missionWithMentions = mission as AiopsTransportHostMission & {
    mentions?: unknown;
  };
  const source = Array.isArray(mission.mentionedHosts) ? mission.mentionedHosts : missionWithMentions.mentions;
  return Array.isArray(source) ? source.map((item) => (item && typeof item === "object" ? item as MissionMention : {})) : [];
}

function formatVerboseHostLabel(childAgent: ChildAgentRow) {
  const hostName = childAgent.hostDisplayName || childAgent.hostId || "未知主机";
  const address = childAgent.hostAddress || childAgent.hostId;
  return address ? `${hostName}(@${address})` : hostName;
}

function formatCompactHostLabel(childAgent: ChildAgentRow) {
  if (childAgent.hostAddress) {
    return atHandle(childAgent.hostAddress);
  }
  return childAgent.hostDisplayName || childAgent.hostId || "未知主机";
}

function formatMentionLabel(mention: MissionMention) {
  return mention.displayName || mention.hostId || mention.address || mention.raw || "未知主机";
}

function formatMentionHandle(mention: MissionMention) {
  const value = mention.address || mention.hostId || mention.raw;
  return value ? atHandle(value) : mention.displayName || "未知主机";
}

function atHandle(value: string) {
  const text = value.trim();
  return text.startsWith("@") ? text : `@${text}`;
}

const hostNameTones = [
  "bg-sky-50 text-sky-700 hover:bg-sky-100",
  "bg-emerald-50 text-emerald-700 hover:bg-emerald-100",
  "bg-violet-50 text-violet-700 hover:bg-violet-100",
  "bg-amber-50 text-amber-700 hover:bg-amber-100",
  "bg-rose-50 text-rose-700 hover:bg-rose-100",
  "bg-cyan-50 text-cyan-700 hover:bg-cyan-100",
  "bg-fuchsia-50 text-fuchsia-700 hover:bg-fuchsia-100",
  "bg-lime-50 text-lime-700 hover:bg-lime-100",
  "bg-orange-50 text-orange-700 hover:bg-orange-100",
  "bg-indigo-50 text-indigo-700 hover:bg-indigo-100",
];

function hostNameTone(index: number) {
  return hostNameTones[index % hostNameTones.length];
}

function statusTone(status: string) {
  switch (status) {
    case "running":
      return "bg-sky-50 text-sky-700 ring-sky-200";
    case "waiting":
      return "bg-amber-50 text-amber-700 ring-amber-200";
    case "completed":
      return "bg-emerald-50 text-emerald-700 ring-emerald-200";
    case "failed":
      return "bg-red-50 text-red-700 ring-red-200";
    case "approval_required":
      return "bg-violet-50 text-violet-700 ring-violet-200";
    case "spawning":
      return "bg-indigo-50 text-indigo-700 ring-indigo-200";
    case "cancelled":
      return "bg-slate-50 text-slate-500 ring-slate-200";
    case "planned":
      return "bg-zinc-50 text-zinc-500 ring-zinc-200";
    default:
      return "bg-cyan-50 text-cyan-700 ring-cyan-200";
  }
}

function hostMentionKey(mention: MissionMention) {
  return normalizedHostKey(mention.hostId || mention.address || mention.displayName || mention.raw);
}

function normalizedHostKey(value?: string) {
  return String(value || "").trim().toLowerCase();
}

function safeTestId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}
