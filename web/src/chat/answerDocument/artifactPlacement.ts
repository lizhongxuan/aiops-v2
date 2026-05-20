import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

import type { AnswerSection, AnswerSectionKind, ArtifactPlacementHint, ArtifactSlot } from "./types";

type ResolveArtifactSlotsInput = {
  sections: AnswerSection[];
  artifacts: AiopsTransportAgentUiArtifact[];
  deferredArtifacts: AiopsTransportAgentUiArtifact[];
};

export function resolveArtifactSlots(input: ResolveArtifactSlotsInput): ArtifactSlot[] {
  const activeArtifacts = input.artifacts.filter(isCorootChartArtifact);
  const deferredArtifacts = input.deferredArtifacts.filter(isCorootChartArtifact);
  const sourceArtifacts = activeArtifacts.length ? activeArtifacts : deferredArtifacts;
  if (!sourceArtifacts.length) {
    return [];
  }

  const primary = choosePrimaryArtifact(sourceArtifacts, input.sections);
  const placement = placementForArtifact(primary, input.sections);
  return [{
    id: `slot-${primary.id}`,
    state: activeArtifacts.length ? "ready" : "deferred",
    placement: placement.placement,
    sectionId: placement.sectionId,
    artifact: activeArtifacts.length ? primary : undefined,
    deferredArtifacts: activeArtifacts.length ? undefined : sourceArtifacts,
  }];
}

export function placementHintForArtifact(artifact: AiopsTransportAgentUiArtifact): ArtifactPlacementHint {
  const metadata = asRecord(artifact.metadata);
  const placement = asRecord(metadata.placement);
  return {
    supports: validKinds(stringArray(placement.supports)),
    preferredAfter: validKinds(stringArray(placement.preferredAfter)),
    preferredBefore: validKinds(stringArray(placement.preferredBefore)),
    topic: validTopic(text(placement.topic)),
    priority: validPriority(text(placement.priority)),
    service: text(placement.service),
    reason: text(placement.reason),
  };
}

function choosePrimaryArtifact(artifacts: AiopsTransportAgentUiArtifact[], sections: AnswerSection[]) {
  const rootCauseText = sections.find((section) => section.kind === "root_cause")?.markdown.toLowerCase() || "";
  const preferredTopic = preferredTopicFromText(rootCauseText);
  return artifacts.find((artifact) => placementHintForArtifact(artifact).topic === preferredTopic) ||
    artifacts.find((artifact) => placementHintForArtifact(artifact).priority === "primary") ||
    artifacts[0];
}

function placementForArtifact(artifact: AiopsTransportAgentUiArtifact, sections: AnswerSection[]) {
  const hint = placementHintForArtifact(artifact);
  const afterKind = firstExistingKind(hint.preferredAfter || [], sections) ||
    firstExistingKind(hint.supports || [], sections);
  if (afterKind) {
    return { placement: "after" as const, sectionId: sectionIdForKind(afterKind, sections) };
  }
  const beforeKind = firstExistingKind(hint.preferredBefore || [], sections);
  if (beforeKind) {
    return { placement: "before" as const, sectionId: sectionIdForKind(beforeKind, sections) };
  }
  const inferredKind = inferSectionKindFromArtifact(artifact, sections);
  if (inferredKind) {
    return { placement: "after" as const, sectionId: sectionIdForKind(inferredKind, sections) };
  }
  return { placement: "end" as const, sectionId: undefined };
}

function inferSectionKindFromArtifact(artifact: AiopsTransportAgentUiArtifact, sections: AnswerSection[]) {
  if (artifact.type !== "coroot_chart") {
    return undefined;
  }
  if (sections.some((section) => section.kind === "root_cause")) {
    return "root_cause" as const;
  }
  if (sections.some((section) => section.kind === "evidence")) {
    return "evidence" as const;
  }
  return undefined;
}

function firstExistingKind(kinds: AnswerSectionKind[], sections: AnswerSection[]) {
  return kinds.find((kind) => sections.some((section) => section.kind === kind));
}

function sectionIdForKind(kind: AnswerSectionKind, sections: AnswerSection[]) {
  return sections.find((section) => section.kind === kind)?.id;
}

function preferredTopicFromText(textValue: string) {
  if (/(external|依赖|端口|tcp|连接|18090)/i.test(textValue)) return "net";
  if (/(cpu|cores|delay|throttled)/i.test(textValue)) return "cpu";
  if (/(memory|内存|rss|pagecache|oom)/i.test(textValue)) return "memory";
  if (/(restart|实例|pod|container)/i.test(textValue)) return "instances";
  return "";
}

function isCorootChartArtifact(artifact: AiopsTransportAgentUiArtifact) {
  return artifact.type === "coroot_chart";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function stringArray(value: unknown): string[] {
  if (typeof value === "string") {
    return [text(value)].filter(Boolean);
  }
  return Array.isArray(value) ? value.map(text).filter(Boolean) : [];
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function validKinds(values: string[]): AnswerSectionKind[] | undefined {
  const allowed: AnswerSectionKind[] = ["summary", "root_cause", "evidence", "impact", "next_steps", "unknown"];
  const result = values.filter((value): value is AnswerSectionKind => allowed.includes(value as AnswerSectionKind));
  return result.length ? result : undefined;
}

function validTopic(value: string): ArtifactPlacementHint["topic"] {
  const allowed = ["cpu", "memory", "net", "instances", "logs", "slo", "topology"];
  return allowed.includes(value) ? value as ArtifactPlacementHint["topic"] : undefined;
}

function validPriority(value: string): ArtifactPlacementHint["priority"] {
  return value === "primary" || value === "secondary" ? value : undefined;
}
