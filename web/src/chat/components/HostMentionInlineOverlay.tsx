import { Activity, BookOpen, GitBranch, Server, Wrench } from "lucide-react";

import { cn } from "@/lib/utils";

import type { HostMentionCandidate, SpecialAiMentionCandidate } from "../hostMentions";

type HostMentionInlineOverlayVariant = "chat" | "default";

type HostMentionInlineOverlayProps = {
  text: string;
  mentions: InlineMention[];
  variant?: HostMentionInlineOverlayVariant;
};

export type ResourceInlineMentionCandidate = {
  tokenId: string;
  raw: string;
  value: string;
  start: number;
  end: number;
  source: "ops_resource";
  kind: "ops_manual" | "ops_graph";
  displayName: string;
};

export type InlineMention = HostMentionCandidate | SpecialAiMentionCandidate | ResourceInlineMentionCandidate;

type HostMentionTextSegment =
  | { type: "text"; text: string; key: string }
  | { type: "mention"; text: string; key: string; mention: InlineMention };

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
            data-testid={inlineMentionTestId(segment.mention)}
            data-mention-kind={inlineMentionKind(segment.mention)}
            data-layout-text={segment.text}
            className={cn(
              "aiops-inline-mention-anchor align-baseline",
              segment.mention.source === "ai_tool" || segment.mention.source === "ops_resource"
                ? "bg-blue-50 text-blue-700"
                : "bg-sky-50 text-sky-700",
            )}
          >
            <span
              data-testid="composer-inline-mention-visual"
              className={cn(
                "aiops-inline-mention-visual max-w-max rounded-md px-0.5 font-medium",
                segment.mention.source === "ai_tool" || segment.mention.source === "ops_resource"
                  ? "bg-blue-50 text-blue-700"
                  : "bg-sky-50 text-sky-700",
              )}
            >
              <InlineMentionIcon mention={segment.mention} />
              <span className="whitespace-nowrap">{inlineMentionLabel(segment.text, segment.mention)}</span>
            </span>
          </span>
        ) : (
          <span key={segment.key}>{segment.text}</span>
        ),
      )}
    </div>
  );
}

function buildHostMentionTextSegments(text: string, mentions: InlineMention[]): HostMentionTextSegment[] {
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
      mention,
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

function InlineMentionIcon({ mention }: { mention: InlineMention }) {
  const className = "h-3.5 w-3.5 shrink-0";
  if (mention.source === "ops_resource") {
    return mention.kind === "ops_graph"
      ? <GitBranch className={className} aria-hidden="true" />
      : <BookOpen className={className} aria-hidden="true" />;
  }
  if (mention.source !== "ai_tool") {
    return <Server className={className} aria-hidden="true" />;
  }
  if (mention.value === "coroot") {
    return <Activity className={className} aria-hidden="true" />;
  }
  if (mention.value === "ops_graph") {
    return <GitBranch className={className} aria-hidden="true" />;
  }
  if (mention.value === "ops_manuals" || mention.value === "ops_manus") {
    return <BookOpen className={className} aria-hidden="true" />;
  }
  return <Wrench className={className} aria-hidden="true" />;
}

function inlineMentionLabel(text: string, mention: InlineMention) {
  if (mention.source === "ops_resource") {
    return mention.displayName || mention.value || text.replace(/^@/, "");
  }
  if (mention.source !== "ai_tool") {
    const label = mention.value === "server-local" ? "local" : mention.value;
    return label || text.replace(/^@/, "");
  }
  if (mention.value === "coroot") return "Coroot";
  if (mention.value === "ops_graph") return "OpsGraph";
  if (mention.value === "ops_manuals" || mention.value === "ops_manus") return "运维手册";
  return text.replace(/^@/, "");
}

function inlineMentionTestId(mention: InlineMention) {
  if (mention.source === "ai_tool") return "composer-inline-special-mention";
  if (mention.source === "ops_resource") return "composer-inline-resource-mention";
  return "composer-inline-host-mention";
}

function inlineMentionKind(mention: InlineMention) {
  if (mention.source === "ai_tool") return "ai_tool";
  if (mention.source === "ops_resource") return mention.kind;
  return "host";
}
