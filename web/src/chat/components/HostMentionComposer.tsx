import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

import type { DisplayHostMention } from "./HostMentionChip";
import { HostMentionInlineOverlay } from "./HostMentionInlineOverlay";

export function HostMentionComposer({
  value,
  mentions,
  onChange,
}: {
  value: string;
  mentions: DisplayHostMention[];
  onChange: (value: string) => void;
}) {
  const inlineMentions = mentions.filter((mention) => mention.resolved !== false);
  const hasInlineMentions = inlineMentions.length > 0;

  return (
    <div className="grid gap-2">
      <div className="relative min-h-12">
        <HostMentionInlineOverlay text={value} mentions={inlineMentions} variant="chat" />
        <Textarea
          value={value}
          rows={1}
          className={cn(
            "relative z-10 min-h-12 resize-none bg-transparent text-[16px] leading-7 md:text-[16px]",
            hasInlineMentions && "text-transparent caret-slate-950 selection:bg-sky-200/70",
          )}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      </div>
    </div>
  );
}
