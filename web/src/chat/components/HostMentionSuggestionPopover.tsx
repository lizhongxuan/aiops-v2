import { Activity, BookOpen, ChevronRight, GitBranch, Server, Wrench } from "lucide-react";

import type { HostMentionSuggestion } from "../hostMentionSearch";
import type { CapabilityMentionSuggestion, MentionCategorySuggestion, ResourceMentionSuggestion } from "../mentionCatalog";

export type ComposerMentionSuggestion =
  | HostMentionSuggestion
  | CapabilityMentionSuggestion
  | MentionCategorySuggestion
  | ResourceMentionSuggestion;

export function HostMentionSuggestionPopover({
  id,
  suggestions,
  highlightedIndex,
  onHighlight,
  onSelect,
}: {
  id: string;
  suggestions: ComposerMentionSuggestion[];
  highlightedIndex: number;
  onHighlight: (index: number) => void;
  onSelect: (suggestion: ComposerMentionSuggestion) => void;
}) {
  const levelLabel = mentionSuggestionLevelLabel(suggestions);
  return (
    <div
      id={id}
      role="listbox"
      data-testid="host-mention-suggestion-popover"
      className="overflow-hidden rounded-2xl border border-slate-200 bg-white/95 shadow-[0_18px_50px_rgba(15,23,42,0.10)] backdrop-blur"
    >
      <div data-testid="host-mention-suggestion-level" className="px-3 pb-1 pt-2 text-xs text-slate-400">{levelLabel}</div>
      {suggestions.length === 0 ? (
        <div data-testid="host-mention-suggestion-empty" className="px-3 py-3 text-sm text-slate-400">
          没有匹配项，可继续手动输入
        </div>
      ) : (
        <div className="max-h-[320px] overflow-y-auto px-2 pb-2">
          {suggestions.map((suggestion, index) => {
            const selected = index === highlightedIndex;
            const Icon = mentionSuggestionIcon(suggestion);
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
                <span className="flex h-5 w-5 items-center justify-center rounded-md bg-slate-100 text-slate-500">
                  <Icon className="h-3.5 w-3.5" aria-hidden="true" />
                </span>
                <span className="flex min-w-0 items-baseline gap-2">
                  <span className="whitespace-nowrap text-[13px] font-medium text-slate-900">{mentionSuggestionPrimaryLabel(suggestion)}</span>
                  <span className="truncate text-xs text-slate-400">{mentionSuggestionDescription(suggestion)}</span>
                </span>
                <span className="flex items-center gap-1 text-[11px] text-slate-300">
                  {suggestion.kind === "category" ? <ChevronRight className="h-3 w-3" aria-hidden="true" /> : null}
                  {index === 0 ? "Enter" : index + 1}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function mentionSuggestionLevelLabel(suggestions: ComposerMentionSuggestion[]) {
  if (suggestions.some((suggestion) => suggestion.kind === "category")) {
    return "选择类型";
  }
  if (suggestions.some((suggestion) => suggestion.kind === "host")) {
    return "选择主机";
  }
  if (suggestions.some((suggestion) => suggestion.kind === "resource" && suggestion.category === "ops_manuals")) {
    return "选择运维手册";
  }
  if (suggestions.some((suggestion) => suggestion.kind === "resource" && suggestion.category === "ops_graph")) {
    return "选择关系图谱";
  }
  return "选择能力";
}

function mentionSuggestionIcon(suggestion: ComposerMentionSuggestion) {
  if (suggestion.kind === "host") return Server;
  const category = suggestion.kind === "category" ? suggestion.category : suggestion.category;
  if (category === "monitor") return Activity;
  if (category === "ops_graph") return GitBranch;
  if (category === "ops_manuals") return BookOpen;
  return Wrench;
}

function mentionSuggestionPrimaryLabel(suggestion: ComposerMentionSuggestion) {
  if (suggestion.kind === "host") {
    if (suggestion.hostId === "server-local" || suggestion.mention === "@local") {
      return suggestion.label;
    }
    return suggestion.address || suggestion.label;
  }
  if (suggestion.kind === "resource") {
    return suggestion.label;
  }
  return suggestion.label;
}

function mentionSuggestionDescription(suggestion: ComposerMentionSuggestion) {
  if (suggestion.kind === "host") {
    const primary = mentionSuggestionPrimaryLabel(suggestion);
    return [
      suggestion.address && suggestion.address !== primary ? suggestion.address : "",
      suggestion.label !== primary ? suggestion.label : "",
      suggestion.status,
    ]
      .filter(Boolean)
      .join(" · ") || suggestion.description;
  }
  if (suggestion.kind === "resource") {
    return suggestion.description;
  }
  return suggestion.description;
}
