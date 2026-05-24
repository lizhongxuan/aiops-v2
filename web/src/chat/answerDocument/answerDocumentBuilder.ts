import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

import { resolveArtifactSlots } from "./artifactPlacement";
import { parseAnswerSections } from "./sectionParser";
import type { AnswerDocumentNode, ArtifactSlot } from "./types";

type BuildAnswerDocumentInput = {
  finalText: string;
  artifacts: AiopsTransportAgentUiArtifact[];
  deferredArtifacts: AiopsTransportAgentUiArtifact[];
};

export function buildAnswerDocument(input: BuildAnswerDocumentInput): AnswerDocumentNode[] {
  const sections = parseAnswerSections(input.finalText.trim());
  const slots = resolveArtifactSlots({
    sections,
    artifacts: input.artifacts,
    deferredArtifacts: input.deferredArtifacts,
  });
  const nodes: AnswerDocumentNode[] = [];

  for (const section of sections) {
    for (const slot of slotsForSection(slots, "before", section.id)) {
      nodes.push({ type: "artifact_slot", slot });
    }
    nodes.push({ type: "section", section });
    for (const slot of slotsForSection(slots, "after", section.id)) {
      nodes.push({ type: "artifact_slot", slot });
    }
  }

  for (const slot of slots.filter((item) => item.placement === "end" || !item.sectionId)) {
    nodes.push({ type: "artifact_slot", slot });
  }

  return nodes;
}

function slotsForSection(slots: ArtifactSlot[], placement: "before" | "after", sectionId: string) {
  return slots.filter((slot) => slot.placement === placement && slot.sectionId === sectionId);
}
