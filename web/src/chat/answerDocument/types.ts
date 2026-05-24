import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

export type AnswerSectionKind =
  | "summary"
  | "root_cause"
  | "evidence"
  | "impact"
  | "next_steps"
  | "unknown";

export type AnswerSection = {
  id: string;
  kind: AnswerSectionKind;
  title?: string;
  markdown: string;
  order: number;
};

export type ArtifactPlacementHint = {
  supports?: AnswerSectionKind[];
  preferredAfter?: AnswerSectionKind[];
  preferredBefore?: AnswerSectionKind[];
  topic?: "cpu" | "memory" | "net" | "instances" | "logs" | "slo" | "topology";
  priority?: "primary" | "secondary";
  service?: string;
  reason?: string;
};

export type ArtifactSlotState = "deferred" | "ready";

export type ArtifactSlot = {
  id: string;
  state: ArtifactSlotState;
  placement: "after" | "before" | "end";
  sectionId?: string;
  artifact?: AiopsTransportAgentUiArtifact;
  deferredArtifacts?: AiopsTransportAgentUiArtifact[];
};

export type AnswerDocumentNode =
  | { type: "section"; section: AnswerSection }
  | { type: "artifact_slot"; slot: ArtifactSlot };
