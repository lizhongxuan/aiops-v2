import { AlertTriangle, GitBranch, Server, X } from "lucide-react";

import type { AiopsSpecialInputContext, AiopsSpecialInputGrant, AiopsSpecialInputRoleBinding } from "@/transport/aiopsTransportTypes";

export function SpecialInputContextBar({
  context,
  onClear,
  onConfirm,
}: {
  context?: AiopsSpecialInputContext;
  onClear?: () => void;
  onConfirm?: () => void;
}) {
  if (!hasSpecialInputContext(context)) {
    return null;
  }
  const activeGrant = context.activeGrant;
  const candidateCount = context.candidateFacts?.length || 0;
  const pendingCount = context.pendingConfirmations?.length || 0;
  const roles = buildRoleBindingGroups(context.roleBindings || []);
  const conflictCount = context.conflicts?.length || 0;

  return (
    <div
      data-testid="special-input-context-bar"
      className="flex flex-wrap items-center gap-2 border-b border-slate-100 pb-3 text-xs text-slate-600"
    >
      {activeGrant ? <ActiveGrantChip grant={activeGrant} /> : null}
      {roles.slice(0, 3).map((group) => (
        <RoleBindingGroupChip key={group.key} group={group} />
      ))}
      {conflictCount > 0 ? (
        <span className="inline-flex h-7 items-center gap-1 rounded-md border border-red-200 bg-red-50 px-2 font-medium text-red-700">
          <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
          角色冲突 {conflictCount}
        </span>
      ) : null}
      {candidateCount > 0 ? (
        <span className="inline-flex h-7 items-center gap-1 rounded-md border border-amber-200 bg-amber-50 px-2 font-medium text-amber-700">
          <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
          低信任候选 {candidateCount}
        </span>
      ) : null}
      {pendingCount > 0 ? (
        <span className="inline-flex h-7 items-center gap-1 rounded-md border border-sky-200 bg-sky-50 px-2 font-medium text-sky-700">
          需要确认 {pendingCount}
        </span>
      ) : null}
      {pendingCount > 0 && onConfirm ? (
        <button
          type="button"
          className="inline-flex h-7 items-center rounded-md border border-sky-200 bg-white px-2 font-medium text-sky-700 hover:bg-sky-50"
          onClick={onConfirm}
        >
          确认
        </button>
      ) : null}
      {onClear ? (
        <button
          type="button"
          aria-label="清除特殊输入上下文"
          title="清除特殊输入上下文"
          className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 hover:bg-slate-50 hover:text-slate-900"
          onClick={onClear}
        >
          <X className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

function ActiveGrantChip({ grant }: { grant: AiopsSpecialInputGrant }) {
  const label = grant.display || grant.resourceId || grant.canonicalKey || "当前目标";
  return (
    <span className="inline-flex h-7 min-w-0 max-w-full items-center gap-1 rounded-md border border-sky-100 bg-sky-50 px-2 font-medium text-sky-700">
      <Server className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </span>
  );
}

type RoleBindingGroup = {
  key: string;
  environmentKey?: string;
  clusterKey?: string;
  bindings: AiopsSpecialInputRoleBinding[];
};

function RoleBindingGroupChip({ group }: { group: RoleBindingGroup }) {
  const prefix = [group.environmentKey, group.clusterKey].filter(Boolean).join(" / ");
  const summary = group.bindings
    .map((binding) => {
      const role = binding.roleKey || binding.runtimeName || "role";
      const target = binding.display || binding.resourceId || binding.bindingHash || "";
      return target ? `${role} -> ${target}` : role;
    })
    .join(", ");
  const label = prefix ? `${prefix}: ${summary}` : summary;
  return (
    <span className="inline-flex h-7 min-w-0 max-w-full items-center gap-1 rounded-md border border-slate-200 bg-white px-2 font-medium text-slate-600">
      <GitBranch className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </span>
  );
}

function buildRoleBindingGroups(bindings: AiopsSpecialInputRoleBinding[]): RoleBindingGroup[] {
  const groups = new Map<string, RoleBindingGroup>();
  for (const binding of bindings) {
    const key = [binding.environmentKey || "", binding.clusterKey || ""].join("\u0000");
    const group = groups.get(key) || {
      key,
      environmentKey: binding.environmentKey,
      clusterKey: binding.clusterKey,
      bindings: [],
    };
    group.bindings.push(binding);
    groups.set(key, group);
  }
  return Array.from(groups.values()).sort((left, right) =>
    left.key.localeCompare(right.key),
  );
}

function hasSpecialInputContext(context?: AiopsSpecialInputContext) {
  if (!context) {
    return false;
  }
  return Boolean(
    context.activeGrant ||
      context.visibleFacts?.length ||
      context.candidateFacts?.length ||
      context.suspendedGrants?.length ||
      context.roleBindings?.length ||
      context.conflicts?.length ||
      context.pendingConfirmations?.length,
  );
}
