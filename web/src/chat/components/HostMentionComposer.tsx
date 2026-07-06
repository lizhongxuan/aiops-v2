import { Textarea } from "@/components/ui/textarea";

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
  return (
    <div className="grid gap-2">
      <div className="relative min-h-12">
        <HostMentionInlineOverlay text={value} mentions={mentions} caretIndex={mentions.length ? value.length : null} />
        <Textarea
          value={value}
          rows={1}
          spellCheck={false}
          className={[
            "relative z-10 min-h-12 resize-none bg-transparent text-[16px] leading-7 md:text-[16px]",
            mentions.length > 0
              ? "text-transparent caret-transparent selection:bg-sky-200/70"
              : "",
          ]
            .filter(Boolean)
            .join(" ")}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      </div>
    </div>
  );
}
