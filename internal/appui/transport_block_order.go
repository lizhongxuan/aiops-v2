package appui

import (
	"strconv"
	"strings"
)

// canonicalTransportBlockAccumulator records the position where a transcript
// block first becomes visible. Later updates replace the block payload without
// moving its id, so tool completion and liveness updates cannot reorder Chat.
type canonicalTransportBlockAccumulator struct {
	order            []string
	blocks           map[string]AiopsTransportBlock
	artifactBlockIDs map[string]string
}

func newCanonicalTransportBlockAccumulator() *canonicalTransportBlockAccumulator {
	return &canonicalTransportBlockAccumulator{
		blocks:           map[string]AiopsTransportBlock{},
		artifactBlockIDs: map[string]string{},
	}
}

func upsertCanonicalTransportBlock(order *[]string, blocks map[string]AiopsTransportBlock, block AiopsTransportBlock) {
	id := strings.TrimSpace(block.ID)
	if id == "" {
		return
	}
	if _, exists := blocks[id]; !exists {
		*order = append(*order, id)
	}
	blocks[id] = block
}

func (acc *canonicalTransportBlockAccumulator) observeTurn(turn AiopsTransportTurn) {
	for _, process := range turn.Process {
		acc.upsert(canonicalProcessTransportBlock(process))
	}
	for idx := range turn.AgentUIArtifacts {
		acc.upsertArtifact(turn.AgentUIArtifacts[idx])
	}
	if turn.Final != nil && strings.TrimSpace(turn.Final.ID) != "" {
		acc.upsert(canonicalFinalTransportBlock(turn, *turn.Final))
	}
}

func (acc *canonicalTransportBlockAccumulator) reconcileTurn(turn AiopsTransportTurn) {
	acc.observeTurn(turn)
	visible := make(map[string]struct{}, len(turn.Process)+len(turn.AgentUIArtifacts)+1)
	for _, process := range turn.Process {
		if id := strings.TrimSpace(process.ID); id != "" {
			visible[id] = struct{}{}
		}
	}
	for idx := range turn.AgentUIArtifacts {
		artifactID := strings.TrimSpace(turn.AgentUIArtifacts[idx].ID)
		if id := acc.artifactBlockIDs[artifactID]; id != "" {
			visible[id] = struct{}{}
		}
	}
	if turn.Final != nil {
		if id := strings.TrimSpace(turn.Final.ID); id != "" {
			visible[id] = struct{}{}
		}
	}

	nextOrder := make([]string, 0, len(visible))
	for _, id := range acc.order {
		if _, ok := visible[id]; !ok {
			delete(acc.blocks, id)
			continue
		}
		nextOrder = append(nextOrder, id)
	}
	acc.order = nextOrder
}

func (acc *canonicalTransportBlockAccumulator) upsert(block AiopsTransportBlock) {
	id := strings.TrimSpace(block.ID)
	if id == "" {
		return
	}
	if existing, collision := acc.blocks[id]; collision && existing.Type == AiopsTransportBlockTypeArtifact && block.Type != AiopsTransportBlockTypeArtifact {
		acc.relocateArtifact(id, existing)
	}
	upsertCanonicalTransportBlock(&acc.order, acc.blocks, block)
}

func (acc *canonicalTransportBlockAccumulator) upsertArtifact(artifact AiopsTransportAgentUIArtifact) {
	artifactID := strings.TrimSpace(artifact.ID)
	if artifactID == "" {
		return
	}
	blockID := acc.artifactBlockIDs[artifactID]
	if blockID == "" {
		blockID = artifactID
		if existing, collision := acc.blocks[blockID]; collision && existing.Type != AiopsTransportBlockTypeArtifact {
			blockID = acc.availableArtifactBlockID(artifactID)
		}
		acc.artifactBlockIDs[artifactID] = blockID
	}
	block := canonicalArtifactTransportBlock(blockID, artifact)
	upsertCanonicalTransportBlock(&acc.order, acc.blocks, block)
}

func (acc *canonicalTransportBlockAccumulator) relocateArtifact(currentID string, block AiopsTransportBlock) {
	artifactID := currentID
	if block.Artifact != nil && strings.TrimSpace(block.Artifact.ID) != "" {
		artifactID = strings.TrimSpace(block.Artifact.ID)
	}
	nextID := acc.availableArtifactBlockID(artifactID)
	delete(acc.blocks, currentID)
	block.ID = nextID
	for idx, id := range acc.order {
		if id == currentID {
			acc.order[idx] = nextID
			break
		}
	}
	acc.blocks[nextID] = block
	acc.artifactBlockIDs[artifactID] = nextID
}

func (acc *canonicalTransportBlockAccumulator) availableArtifactBlockID(artifactID string) string {
	base := "artifact:" + strings.TrimSpace(artifactID)
	if _, exists := acc.blocks[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + ":" + strconv.Itoa(suffix)
		if _, exists := acc.blocks[candidate]; !exists {
			return candidate
		}
	}
}

func canonicalProcessTransportBlock(process AiopsProcessBlock) AiopsTransportBlock {
	blockType := AiopsTransportBlockType(process.Kind)
	if process.Kind == AiopsTransportProcessKindAssistant && process.Phase == "commentary" {
		blockType = AiopsTransportBlockTypeCommentary
	}
	return AiopsTransportBlock{Type: blockType, AiopsProcessBlock: process}
}

func canonicalFinalTransportBlock(turn AiopsTransportTurn, final AiopsTransportFinal) AiopsTransportBlock {
	return AiopsTransportBlock{
		Type: AiopsTransportBlockTypeFinalAnswer,
		AiopsProcessBlock: AiopsProcessBlock{
			ID:          final.ID,
			Kind:        AiopsTransportProcessKindAssistant,
			DisplayKind: "assistant.message",
			Phase:       "final_answer",
			StreamState: finalStreamState(final.Status),
			Status:      mapFinalStatusToTransportProcessStatus(final.Status),
			Text:        final.Text,
			DurationMs:  final.DurationMs,
			UpdatedAt:   firstNonEmptyString(turn.CompletedAt, turn.UpdatedAt),
		},
		FinalContract: &final,
	}
}

func canonicalArtifactTransportBlock(id string, artifact AiopsTransportAgentUIArtifact) AiopsTransportBlock {
	return AiopsTransportBlock{
		Type: AiopsTransportBlockTypeArtifact,
		AiopsProcessBlock: AiopsProcessBlock{
			ID:        id,
			Kind:      AiopsTransportProcessKindTool,
			Status:    mapArtifactStatusToTransportProcessStatus(artifact.Status),
			Text:      firstNonEmptyString(artifact.SummaryZh, artifact.Summary, artifact.TitleZh, artifact.Title),
			UpdatedAt: artifact.UpdatedAt,
		},
		Artifact: &artifact,
	}
}

// projectCanonicalTransportBlocks is a compatibility boundary for callers
// that only have a completed turn. ProjectTurnSnapshot uses the same
// accumulator incrementally while each TurnItem is projected.
func projectCanonicalTransportBlocks(turn AiopsTransportTurn) ([]string, map[string]AiopsTransportBlock) {
	acc := newCanonicalTransportBlockAccumulator()
	acc.reconcileTurn(turn)
	return acc.order, acc.blocks
}
