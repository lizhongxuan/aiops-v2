import { cn } from "@/lib/utils";

import type { HostMentionCandidate } from "../hostMentions";

type HostMentionInlineOverlayVariant = "chat" | "default";

type HostMentionInlineOverlayProps = {
  text: string;
  mentions: HostMentionCandidate[];
  variant?: HostMentionInlineOverlayVariant;
};

type HostMentionTextSegment =
  | { type: "text"; text: string; key: string }
  | { type: "mention"; text: string; key: string };

export function HostMentionInlineOverlay({ text, mentions, variant = "chat" }: HostMentionInlineOverlayProps) {
  const segments = buildHostMentionTextSegments(text, mentions);
  if (!text || segments.every((segment) => segment.type !== "mention")) {
    return null;
  }

  return (
    <div
      aria-hidden="true"
      data-testid="composer-inline-host-overlay"
      className={cn(
        "pointer-events-none absolute inset-0 z-0 overflow-hidden whitespace-pre-wrap break-words text-left text-slate-900",
        variant === "chat" ? "min-h-12 max-h-40 px-3 py-2 text-[16px] leading-7 md:text-[16px]" : "min-h-11 max-h-44 px-2.5 py-2 text-sm leading-6",
      )}
    >
      {segments.map((segment) =>
        segment.type === "mention" ? (
          <span
            key={segment.key}
            data-testid="composer-inline-host-mention"
            className="rounded-md bg-sky-50 px-1 font-semibold text-sky-700 ring-1 ring-sky-200"
          >
            {segment.text}
          </span>
        ) : (
          <span key={segment.key}>{segment.text}</span>
        ),
      )}
    </div>
  );
}

function buildHostMentionTextSegments(text: string, mentions: HostMentionCandidate[]): HostMentionTextSegment[] {
  const segments: HostMentionTextSegment[] = [];
  let cursor = 0;
  const orderedMentions = [...mentions]
    .filter((mention) => mention.raw && mention.start >= 0 && mention.end > mention.start && mention.end <= text.length)
    .sort((a, b) => a.start - b.start || a.end - b.end);

  for (const mention of orderedMentions) {
    if (mention.start < cursor) {
      continue;
    }
    if (mention.start > cursor) {
      segments.push({
        type: "text",
        text: text.slice(cursor, mention.start),
        key: `text-${cursor}-${mention.start}`,
      });
    }
    segments.push({
      type: "mention",
      text: text.slice(mention.start, mention.end),
      key: `mention-${mention.start}-${mention.end}-${mention.raw}`,
    });
    cursor = mention.end;
  }

  if (cursor < text.length) {
    segments.push({
      type: "text",
      text: text.slice(cursor),
      key: `text-${cursor}-${text.length}`,
    });
  }

  return segments;
}
