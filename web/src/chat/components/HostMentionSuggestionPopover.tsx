import { Server } from "lucide-react";

import type { HostMentionSuggestion } from "../hostMentionSearch";

export function HostMentionSuggestionPopover({
  id,
  suggestions,
  highlightedIndex,
  onHighlight,
  onSelect,
}: {
  id: string;
  suggestions: HostMentionSuggestion[];
  highlightedIndex: number;
  onHighlight: (index: number) => void;
  onSelect: (suggestion: HostMentionSuggestion) => void;
}) {
  return (
    <div
      id={id}
      role="listbox"
      data-testid="host-mention-suggestion-popover"
      className="overflow-hidden rounded-2xl border border-slate-200 bg-white/95 shadow-[0_18px_50px_rgba(15,23,42,0.10)] backdrop-blur"
    >
      <div className="px-3 pb-1 pt-2 text-xs text-slate-400">主机</div>
      {suggestions.length === 0 ? (
        <div data-testid="host-mention-suggestion-empty" className="px-3 py-3 text-sm text-slate-400">
          没有匹配主机，可继续手动输入
        </div>
      ) : (
        <div className="max-h-[320px] overflow-y-auto px-2 pb-2">
          {suggestions.map((suggestion, index) => {
            const selected = index === highlightedIndex;
            return (
              <button
                key={suggestion.key}
                type="button"
                role="option"
                aria-selected={selected}
                data-testid="host-mention-suggestion-item"
                className={`grid min-h-8 w-full grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-2 rounded-lg px-2 py-1.5 text-left ${
                  selected ? "bg-slate-100" : "bg-transparent hover:bg-slate-50"
                }`}
                onMouseEnter={() => onHighlight(index)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => onSelect(suggestion)}
              >
                <span className="flex h-4 w-4 items-center justify-center rounded bg-slate-100 text-slate-500">
                  <Server className="h-3 w-3" aria-hidden="true" />
                </span>
                <span className="flex min-w-0 items-baseline gap-2">
                  <span className="whitespace-nowrap font-mono text-[13px] text-slate-900">@{suggestion.label}</span>
                  <span className="truncate text-xs text-slate-400">{suggestion.description}</span>
                </span>
                <span className="text-[11px] text-slate-300">{index === 0 ? "Enter" : index + 1}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
