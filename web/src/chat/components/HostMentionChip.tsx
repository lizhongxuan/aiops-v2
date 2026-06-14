import { Server } from "lucide-react";

import type { HostMentionCandidate } from "../hostMentions";

export function HostMentionChip({ mention }: { mention: HostMentionCandidate }) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-md border border-slate-200 bg-slate-50 px-2 py-1 font-mono text-xs text-slate-700">
      <Server className="h-3 w-3 text-slate-500" aria-hidden="true" />
      <span className="truncate">{mention.raw}</span>
    </span>
  );
}
