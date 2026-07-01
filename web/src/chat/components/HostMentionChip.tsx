import { Server, XIcon } from "lucide-react";

import type { HostMentionCandidate } from "../hostMentions";

export type DisplayHostMention = HostMentionCandidate & {
  hostId?: string;
  displayName?: string;
  resolved?: boolean;
};

export function HostMentionChip({
  mention,
  onRemove,
}: {
  mention: DisplayHostMention;
  onRemove?: () => void;
}) {
  const label = mention.displayName || mention.raw;
  const rawLabel = mention.raw?.trim();
  const showRawLabel = Boolean(rawLabel && rawLabel !== label.trim());

  return (
    <span
      className="inline-flex max-w-full items-center gap-1 rounded-md border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-700"
      data-testid="composer-host-chip"
    >
      <Server className="h-3 w-3 text-slate-500" aria-hidden="true" />
      <span className="truncate font-medium">{label}</span>
      {showRawLabel ? <span className="truncate font-mono text-slate-500">{rawLabel}</span> : null}
      {onRemove ? (
        <button
          type="button"
          className="-mr-1 inline-flex size-5 shrink-0 items-center justify-center rounded text-slate-500 hover:bg-slate-200 hover:text-slate-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
          aria-label={`移除 ${label}`}
          onClick={onRemove}
        >
          <XIcon className="size-3" aria-hidden="true" />
        </button>
      ) : null}
    </span>
  );
}
