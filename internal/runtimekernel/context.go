package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/spanstream"
)

// ---------------------------------------------------------------------------
// Context assembly and trimming for the RuntimeKernel.
// Manages the ContextWindow to ensure UsedTokens does not exceed MaxTokens.
// Trimming preserves the most recent messages (priority: newest first).
// ---------------------------------------------------------------------------

// DefaultMaxTokens is the default context window size if not configured.
const DefaultMaxTokens = 200000

// ContextCompactionPlan describes the split between the compactable prefix and
// the retained suffix of a message history.
type ContextCompactionPlan struct {
	Compacted    bool
	Compactable  []Message
	Retained     []Message
	TrimmedCount int
}

// ContextPipelineOptions controls compaction-time behavior for a turn iteration.
type ContextPipelineOptions struct {
	SessionID         string
	TurnID            string
	Iteration         int
	Compressor        *spanstream.ContextCompressor
	Profile           string
	TargetRefs        []string
	PendingApprovals  []PendingApproval
	PendingEvidence   []PendingEvidence
	RejectedApprovals []RejectedApproval
	ToolPacksLoaded   []string
	BudgetPolicy      ContextBudgetPolicy
}

// ContextPipelineResult contains the compacted view that is safe to show the model.
type ContextPipelineResult struct {
	Messages           []Message
	CompactedSegments  []CompactedSegment
	ExternalReferences []ExternalReference
	GovernanceEvents   []ContextGovernanceEvent
}

// EstimateTokens provides a rough token estimate for a message.
// In production, this would use a proper tokenizer (tiktoken, etc.).
// For now, we use a simple heuristic: ~4 chars per token.
func EstimateTokens(msg Message) int {
	chars := len(msg.Content)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Name) + len(tc.Arguments)
	}
	if msg.ToolResult != nil {
		chars += len(msg.ToolResult.Content)
	}
	tokens := chars / 4
	if tokens < 1 && chars > 0 {
		tokens = 1
	}
	return tokens
}

// TrimContext ensures the context window stays within MaxTokens by removing
// the oldest messages first. The most recent messages are preserved with
// highest priority.
//
// After trimming:
//   - ContextWindow.UsedTokens <= ContextWindow.MaxTokens
//   - ContextWindow.Messages reflects the actual message count
//   - ContextWindow.TruncatedAt records how many messages were removed
func TrimContext(cw *ContextWindow, messages []Message) {
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	// Recalculate total tokens
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)

	// No trimming needed
	if totalTokens <= cw.MaxTokens {
		cw.TruncatedAt = 0
		return
	}

	// Trim from the front (oldest messages) until within budget
	trimmed := 0
	for totalTokens > cw.MaxTokens && trimmed < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[trimmed])
		trimmed++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - trimmed
	cw.TruncatedAt = trimmed
}

// AssembleContext creates a trimmed message slice from the session messages,
// respecting the context window limits. Returns the messages that fit within
// the window (most recent messages preserved).
func AssembleContext(cw *ContextWindow, messages []Message) []Message {
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	// Calculate total tokens
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}

	// If within budget, return all messages
	if totalTokens <= cw.MaxTokens {
		cw.UsedTokens = totalTokens
		cw.Messages = len(messages)
		cw.TruncatedAt = 0
		return messages
	}

	// Trim from front (oldest) to fit within budget
	startIdx := 0
	for totalTokens > cw.MaxTokens && startIdx < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[startIdx])
		startIdx++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - startIdx
	cw.TruncatedAt = startIdx

	return messages[startIdx:]
}

// SplitContextForCompaction prepares the context window for a compaction
// pipeline by returning the oldest messages that should be summarized and the
// newest messages that should remain in the live window.
//
// The returned plan mirrors TrimContext/AssembleContext token accounting, but
// exposes the compactable prefix so a caller can summarize it before
// reattaching the retained suffix.
func SplitContextForCompaction(cw *ContextWindow, messages []Message) ContextCompactionPlan {
	plan := ContextCompactionPlan{
		Compactable: append([]Message(nil), messages...),
		Retained:    append([]Message(nil), messages...),
	}

	if cw == nil {
		plan.Compactable = nil
		plan.TrimmedCount = 0
		plan.Compacted = false
		return plan
	}

	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}

	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)

	if totalTokens <= cw.MaxTokens {
		cw.TruncatedAt = 0
		plan.Compactable = nil
		plan.TrimmedCount = 0
		plan.Compacted = false
		return plan
	}

	startIdx := 0
	for totalTokens > cw.MaxTokens && startIdx < len(messages)-1 {
		totalTokens -= EstimateTokens(messages[startIdx])
		startIdx++
	}

	cw.UsedTokens = totalTokens
	cw.Messages = len(messages) - startIdx
	cw.TruncatedAt = startIdx

	plan.Compactable = append([]Message(nil), messages[:startIdx]...)
	plan.Retained = append([]Message(nil), messages[startIdx:]...)
	plan.TrimmedCount = startIdx
	plan.Compacted = startIdx > 0
	return plan
}

// ApplyContextPipeline upgrades the old trim-only behavior into a compaction-first
// pipeline. It preserves a hard-kept recent suffix, compacts an older prefix into
// a synthetic summary message, and only falls back to additional suffix trimming
// after the summary has been introduced.
func ApplyContextPipeline(ctx context.Context, cw *ContextWindow, messages []Message, opts ContextPipelineOptions) (ContextPipelineResult, error) {
	thresholds := contextPipelineThresholds(cw, opts.BudgetPolicy)
	micro := MicrocompactMessages(messages, MicrocompactOptions{
		SessionID:                  opts.SessionID,
		TurnID:                     opts.TurnID,
		Iteration:                  opts.Iteration,
		SmallContextMode:           thresholds.SmallContextMode,
		PendingEvidenceToolCallIDs: pendingEvidenceToolCallIDs(opts.PendingEvidence),
		ApprovalBlockerToolCallIDs: pendingApprovalToolCallIDs(opts.PendingApprovals),
	})
	governanceEvents := append([]ContextGovernanceEvent(nil), micro.Events...)
	messages = micro.Messages

	plan := SplitContextForCompaction(cw, messages)
	if !plan.Compacted {
		return ContextPipelineResult{Messages: plan.Retained, GovernanceEvents: governanceEvents}, nil
	}

	hardKeepCount := 4
	if len(opts.PendingApprovals) > 0 || len(opts.PendingEvidence) > 0 {
		hardKeepCount = 6
	}
	minRetained := hardKeepCount
	if len(messages) < minRetained {
		minRetained = len(messages)
	}
	if extra := minRetained - len(plan.Retained); extra > 0 && len(plan.Compactable) > 0 {
		if extra > len(plan.Compactable) {
			extra = len(plan.Compactable)
		}
		start := len(plan.Compactable) - extra
		plan.Retained = append(append([]Message(nil), plan.Compactable[start:]...), plan.Retained...)
		plan.Compactable = append([]Message(nil), plan.Compactable[:start]...)
		plan.TrimmedCount = len(plan.Compactable)
	}
	hardKeepReasons := compactHardKeepReasons(plan.Retained, opts, minRetained)
	if len(hardKeepReasons) > 0 {
		governanceEvents = append(governanceEvents, BuildContextGovernanceEvent(ContextGovernanceEvent{
			ID:              fmt.Sprintf("ctxgov-%s-%d-l4-hard-keep", opts.TurnID, opts.Iteration),
			Layer:           ContextGovernanceLayerL4,
			Kind:            "context.compaction.hard_keep",
			SessionID:       opts.SessionID,
			TurnID:          opts.TurnID,
			Iteration:       opts.Iteration,
			Message:         "compact hard keep reasons recorded",
			Budget:          thresholds,
			DroppedGroupIDs: hardKeepReasons,
		}))
	}
	if len(plan.Compactable) == 0 {
		result := append([]Message(nil), plan.Retained...)
		recomputeContextWindow(cw, result)
		return ContextPipelineResult{Messages: result, GovernanceEvents: governanceEvents}, nil
	}

	refs := collectMessageReferences(plan.Compactable)
	summary := heuristicCompactionSummary(plan.Compactable, opts, refs)
	startedEvent := BuildContextGovernanceEvent(ContextGovernanceEvent{
		ID:           fmt.Sprintf("ctxgov-%s-%d-l4-started", opts.TurnID, opts.Iteration),
		Layer:        ContextGovernanceLayerL4,
		Kind:         "context.compaction.started",
		SessionID:    opts.SessionID,
		TurnID:       opts.TurnID,
		Iteration:    opts.Iteration,
		Message:      "正在压缩上下文，当前任务会继续",
		Budget:       thresholds,
		ReferenceIDs: referenceIDs(refs),
	})
	governanceEvents = append(governanceEvents, startedEvent)
	if opts.Compressor != nil {
		compactCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		compressed, err := compressOnce(compactCtx, opts.Compressor, plan.Compactable)
		cancel()
		if err == nil && strings.TrimSpace(compressed) != "" {
			if _, validateErr := ParseCompactSummaryV1(compressed); validateErr == nil {
				summary = compressed
			} else {
				governanceEvents = append(governanceEvents, BuildContextGovernanceEvent(ContextGovernanceEvent{
					ID:           fmt.Sprintf("ctxgov-%s-%d-l4-summary-validation-failed", opts.TurnID, opts.Iteration),
					Layer:        ContextGovernanceLayerL4,
					Kind:         "context.compaction.summary_validation_failed",
					SessionID:    opts.SessionID,
					TurnID:       opts.TurnID,
					Iteration:    opts.Iteration,
					Message:      "上下文压缩摘要未通过结构校验，已使用本地摘要继续",
					Budget:       thresholds,
					ReferenceIDs: referenceIDs(refs),
				}))
			}
		} else if err != nil {
			message := "上下文压缩失败，已使用本地摘要继续"
			layer := ContextGovernanceLayerL4
			if isPromptTooLongError(err) {
				message = "上下文过长，已使用本地摘要继续"
				layer = ContextGovernanceLayerL5
			}
			governanceEvents = append(governanceEvents, BuildContextGovernanceEvent(ContextGovernanceEvent{
				ID:           fmt.Sprintf("ctxgov-%s-%d-l4-failed", opts.TurnID, opts.Iteration),
				Layer:        layer,
				Kind:         "context.compaction.failed",
				SessionID:    opts.SessionID,
				TurnID:       opts.TurnID,
				Iteration:    opts.Iteration,
				Message:      message,
				Budget:       thresholds,
				ReferenceIDs: referenceIDs(refs),
				Timeout:      compactCtx.Err() == context.DeadlineExceeded,
			}))
		}
	}
	segmentID := fmt.Sprintf("cmp-%s-%d-%d", opts.TurnID, opts.Iteration, plan.TrimmedCount)
	summaryCreatedAt := time.Now()
	summaryMsg := NewCompactBoundaryMessage(CompactBoundaryInput{
		SegmentID:          segmentID,
		CompactedTurnStart: 0,
		CompactedTurnEnd:   plan.TrimmedCount - 1,
		PreservedTailCount: len(plan.Retained),
		CreatedAt:          summaryCreatedAt,
	})
	summaryMsg.ID = segmentID + "-summary"
	if len(hardKeepReasons) > 0 {
		if summaryMsg.Metadata == nil {
			summaryMsg.Metadata = map[string]string{}
		}
		summaryMsg.Metadata["hardKeepReasons"] = strings.Join(hardKeepReasons, ",")
	}
	boundaryContent := summaryMsg.Content
	summaryBody := buildSummaryMessage(summary, refs)
	if cw != nil && cw.MaxTokens > 0 && cw.MaxTokens > 16 {
		summaryBody = truncateForBudget(summaryBody, cw.MaxTokens/3)
	}
	summaryMsg.Content = boundaryContent + "\n" + summaryBody

	retained := append([]Message(nil), plan.Retained...)
	resultMessages := append([]Message{summaryMsg}, retained...)
	if cw != nil {
		maxTokens := cw.MaxTokens
		if maxTokens <= 0 {
			maxTokens = DefaultMaxTokens
		}
		totalTokens := 0
		for _, msg := range resultMessages {
			totalTokens += EstimateTokens(msg)
		}
		for totalTokens > maxTokens && len(retained) > minRetained {
			totalTokens -= EstimateTokens(retained[0])
			retained = retained[1:]
		}
		resultMessages = append([]Message{summaryMsg}, retained...)
		recomputeContextWindow(cw, resultMessages)
		cw.TruncatedAt = plan.TrimmedCount
	}

	segment := CompactedSegment{
		ID:                 segmentID,
		SessionID:          opts.SessionID,
		TurnID:             opts.TurnID,
		Iteration:          opts.Iteration,
		StartIndex:         0,
		EndIndex:           plan.TrimmedCount - 1,
		Summary:            summary,
		ReferenceIDs:       referenceIDs(refs),
		ExternalReferences: refs,
		CreatedAt:          summaryCreatedAt,
	}
	governanceEvents = append(governanceEvents, BuildContextGovernanceEvent(ContextGovernanceEvent{
		ID:           fmt.Sprintf("ctxgov-%s-%d-l4-completed", opts.TurnID, opts.Iteration),
		Layer:        ContextGovernanceLayerL4,
		Kind:         "context.compaction.completed",
		SessionID:    opts.SessionID,
		TurnID:       opts.TurnID,
		Iteration:    opts.Iteration,
		Message:      "已整理早期上下文",
		Budget:       thresholds,
		ReferenceIDs: referenceIDs(refs),
		CompactedIDs: []string{segment.ID},
	}))
	return ContextPipelineResult{
		Messages:           resultMessages,
		CompactedSegments:  []CompactedSegment{segment},
		ExternalReferences: refs,
		GovernanceEvents:   governanceEvents,
	}, nil
}

func compressOnce(ctx context.Context, compressor *spanstream.ContextCompressor, messages []Message) (string, error) {
	compressorMessages := messagesForCompressor(messages)
	return compressor.Compress(ctx, nil, compressorMessages)
}

func messagesForCompressor(messages []Message) []spanstream.Message {
	out := make([]spanstream.Message, 0, len(messages))
	for _, msg := range messages {
		content := msg.Content
		if msg.ToolResult != nil {
			content = msg.ToolResult.Content
		}
		out = append(out, spanstream.Message{
			Role:    msg.Role,
			Content: content,
		})
	}
	return out
}

func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, needle := range []string{
		"prompt too long",
		"context length",
		"maximum context",
		"too many tokens",
		"tokens exceed",
		"exceeds context",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func contextPipelineThresholds(cw *ContextWindow, policy ContextBudgetPolicy) ContextBudgetThresholds {
	maxTokens := DefaultMaxTokens
	if cw != nil && cw.MaxTokens > 0 {
		maxTokens = cw.MaxTokens
	}
	if policy.MaxContextTokens <= 0 {
		policy = DefaultContextBudgetPolicy(maxTokens, policy.ModelMaxOutputTokens)
	}
	return policy.Thresholds()
}

func recomputeContextWindow(cw *ContextWindow, messages []Message) {
	if cw == nil {
		return
	}
	if cw.MaxTokens <= 0 {
		cw.MaxTokens = DefaultMaxTokens
	}
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}
	cw.UsedTokens = totalTokens
	cw.Messages = len(messages)
	cw.TruncatedAt = 0
}

func collectMessageReferences(messages []Message) []ExternalReference {
	out := make([]ExternalReference, 0)
	seen := make(map[string]struct{})
	for _, msg := range messages {
		if msg.ToolResult == nil {
			continue
		}
		for _, ref := range msg.ToolResult.ExternalReferences {
			if ref.ID == "" {
				continue
			}
			if _, ok := seen[ref.ID]; ok {
				continue
			}
			seen[ref.ID] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}

func referenceIDs(refs []ExternalReference) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func pendingEvidenceToolCallIDs(items []PendingEvidence) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ToolCallID) != "" {
			ids = append(ids, item.ToolCallID)
		}
	}
	return ids
}

func pendingApprovalToolCallIDs(items []PendingApproval) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ToolCallID) != "" {
			ids = append(ids, item.ToolCallID)
		}
	}
	return ids
}

func compactionTargetRefs(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var refs []string
	if snapshot.LatestCheckpoint != nil {
		refs = append(refs, snapshot.LatestCheckpoint.TargetRefs...)
	}
	for _, approval := range snapshot.PendingApprovals {
		refs = append(refs, approval.TargetRefs...)
		refs = append(refs, approval.ResourceScopes...)
		if strings.TrimSpace(approval.HostID) != "" {
			refs = append(refs, "host:"+strings.TrimSpace(approval.HostID))
		}
	}
	for _, evidence := range snapshot.PendingEvidence {
		if strings.TrimSpace(evidence.ToolName) != "" {
			refs = append(refs, "tool:"+strings.TrimSpace(evidence.ToolName))
		}
	}
	return nonEmptyRuntimeStrings(refs)
}

func compactHardKeepReasons(retained []Message, opts ContextPipelineOptions, minRetained int) []string {
	reasons := make([]string, 0, 5)
	if retainedHasRole(retained, "user") {
		reasons = append(reasons, "recent_user_message")
	}
	if len(opts.PendingApprovals) > 0 {
		reasons = append(reasons, "pending_approval")
	}
	if len(opts.PendingEvidence) > 0 {
		reasons = append(reasons, "pending_evidence")
	}
	if len(retained) > 0 {
		reasons = append(reasons, "active_task")
	}
	if minRetained > 0 {
		reasons = append(reasons, "compact_safety_minimum")
	}
	return reasons
}

func retainedHasRole(messages []Message, role string) bool {
	for _, msg := range messages {
		if msg.Role == role {
			return true
		}
	}
	return false
}

func heuristicCompactionSummary(messages []Message, opts ContextPipelineOptions, refs []ExternalReference) string {
	if len(messages) == 0 {
		return ""
	}
	summary := buildEvidenceHandoffSummary(messages, opts, refs)
	data, err := json.Marshal(summary)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func buildEvidenceHandoffSummary(messages []Message, opts ContextPipelineOptions, refs []ExternalReference) CompactSummaryV1 {
	latestUser := latestCompactUserMessage(messages, opts.TurnID)
	userGoal := latestUser.Quote
	if firstUser := firstCompactUserMessage(messages, opts.TurnID); strings.TrimSpace(firstUser.Quote) != "" {
		userGoal = firstUser.Quote
	}
	if strings.TrimSpace(userGoal) == "" {
		userGoal = "Continue the current task after context compaction."
	}
	if strings.TrimSpace(latestUser.Quote) == "" {
		latestUser = CompactSummaryMessageRefV1{TurnID: firstNonEmptyRuntimeString(opts.TurnID, "compacted-context"), Quote: userGoal}
	}
	facts, artifacts := compactSummaryEvidenceArtifacts(refs)
	if len(facts) == 0 {
		facts = append(facts, CompactSummaryFactV1{
			Statement: "Earlier context was compacted; no direct evidence refs were present in the compacted prefix.",
			SourceRef: firstNonEmptyRuntimeString(opts.TurnID, "compacted-context"),
		})
	}
	decisions := compactSummaryDecisions(messages, opts.TurnID)
	if len(decisions) == 0 {
		decisions = append(decisions, CompactSummaryDecisionV1{
			Decision:  "Continue from compacted context without treating missing raw payloads as confirmed evidence.",
			SourceRef: firstNonEmptyRuntimeString(opts.TurnID, "compacted-context"),
		})
	}
	inferences := compactSummaryInferences(messages, opts.TurnID)
	if len(inferences) == 0 {
		inferences = append(inferences, CompactSummaryInferenceV1{
			Statement:  "No additional inference was preserved from the compacted prefix.",
			Confidence: "low",
			SourceRef:  firstNonEmptyRuntimeString(opts.TurnID, "compacted-context"),
		})
	}
	return CompactSummaryV1{
		SchemaVersion:      CompactSummarySchemaVersionV1,
		UserGoal:           userGoal,
		CurrentProfile:     strings.TrimSpace(opts.Profile),
		TargetRefs:         nonEmptyRuntimeStrings(opts.TargetRefs),
		LatestUserMessages: []CompactSummaryMessageRefV1{latestUser},
		ActiveConstraints: []string{
			"Use compacted evidence refs instead of raw stdout/stderr, manual content, or artifact payload.",
			"Keep observed facts, inferences, pending work, and rejected approvals separate.",
		},
		CurrentTask: CompactSummaryCurrentTaskV1{
			Description:  "Resume the current task from compacted evidence handoff summary.",
			SourceTurnID: firstNonEmptyRuntimeString(opts.TurnID, latestUser.TurnID),
		},
		ConfirmedFacts:    facts,
		Inferences:        inferences,
		OpenQuestions:     []string{"Which remaining next step can be completed with the retained context and available tools?"},
		Decisions:         decisions,
		Artifacts:         artifacts,
		PendingApprovals:  compactSummaryPendingApprovals(opts.PendingApprovals),
		PendingEvidence:   compactSummaryPendingEvidence(opts.PendingEvidence),
		RejectedApprovals: compactSummaryRejectedApprovals(opts.RejectedApprovals),
		ToolPacksLoaded:   nonEmptyRuntimeStrings(opts.ToolPacksLoaded),
		PlanState:         CompactSummaryPlanStateV1{Status: "in_progress"},
		NextStep: CompactSummaryNextStepV1{
			Action:          "Continue with retained context, compacted evidence refs, and the smallest safe next step.",
			SourceTurnID:    firstNonEmptyRuntimeString(opts.TurnID, latestUser.TurnID),
			RecentUserQuote: latestUser.Quote,
		},
	}
}

func firstCompactUserMessage(messages []Message, turnID string) CompactSummaryMessageRefV1 {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		if quote := summarizeSnippet(msg.Content); quote != "" {
			return CompactSummaryMessageRefV1{TurnID: firstNonEmptyRuntimeString(turnID, msg.ID), Quote: quote}
		}
	}
	return CompactSummaryMessageRefV1{}
}

func latestCompactUserMessage(messages []Message, turnID string) CompactSummaryMessageRefV1 {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		if quote := summarizeSnippet(msg.Content); quote != "" {
			return CompactSummaryMessageRefV1{TurnID: firstNonEmptyRuntimeString(turnID, msg.ID), Quote: quote}
		}
	}
	return CompactSummaryMessageRefV1{}
}

func compactSummaryEvidenceArtifacts(refs []ExternalReference) ([]CompactSummaryFactV1, []CompactSummaryArtifactV1) {
	facts := make([]CompactSummaryFactV1, 0, len(refs))
	artifacts := make([]CompactSummaryArtifactV1, 0, len(refs))
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		summary := firstNonEmptyRuntimeString(ref.Summary, ref.Title, "externalized evidence reference")
		facts = append(facts, CompactSummaryFactV1{
			Statement: "Observed evidence was externalized as " + id + ".",
			SourceRef: id,
		})
		artifacts = append(artifacts, CompactSummaryArtifactV1{
			ID:        id,
			SourceRef: firstNonEmptyRuntimeString(ref.URI, ref.FilePath, ref.CardRef, id),
			Summary:   summarizeSnippet(summary),
		})
	}
	return facts, artifacts
}

func compactSummaryDecisions(messages []Message, turnID string) []CompactSummaryDecisionV1 {
	out := make([]CompactSummaryDecisionV1, 0)
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if !strings.Contains(strings.ToLower(content), "decision") {
			continue
		}
		out = append(out, CompactSummaryDecisionV1{Decision: summarizeSnippet(content), SourceRef: firstNonEmptyRuntimeString(msg.ID, turnID)})
	}
	return out
}

func compactSummaryInferences(messages []Message, turnID string) []CompactSummaryInferenceV1 {
	out := make([]CompactSummaryInferenceV1, 0)
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		lower := strings.ToLower(content)
		if !strings.Contains(lower, "inference") && !strings.Contains(lower, "confidence") {
			continue
		}
		out = append(out, CompactSummaryInferenceV1{Statement: summarizeSnippet(content), Confidence: compactSummaryConfidence(lower), SourceRef: firstNonEmptyRuntimeString(msg.ID, turnID)})
	}
	return out
}

func compactSummaryConfidence(lower string) string {
	switch {
	case strings.Contains(lower, "high"):
		return "high"
	case strings.Contains(lower, "medium"):
		return "medium"
	case strings.Contains(lower, "low"):
		return "low"
	default:
		return "unknown"
	}
}

func compactSummaryPendingApprovals(items []PendingApproval) []CompactSummaryPendingItemV1 {
	out := make([]CompactSummaryPendingItemV1, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		out = append(out, CompactSummaryPendingItemV1{ID: id, SourceRef: firstNonEmptyRuntimeString(item.ToolCallID, item.ToolName, item.TurnID)})
	}
	return out
}

func compactSummaryPendingEvidence(items []PendingEvidence) []CompactSummaryPendingItemV1 {
	out := make([]CompactSummaryPendingItemV1, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		out = append(out, CompactSummaryPendingItemV1{ID: id, SourceRef: firstNonEmptyRuntimeString(item.ToolCallID, item.ToolName, item.TurnID)})
	}
	return out
}

func compactSummaryRejectedApprovals(items []RejectedApproval) []CompactSummaryPendingItemV1 {
	out := make([]CompactSummaryPendingItemV1, 0, len(items))
	for _, item := range items {
		id := firstNonEmptyRuntimeString(item.ID, item.ToolCallID, item.ToolName)
		if id == "" {
			continue
		}
		out = append(out, CompactSummaryPendingItemV1{ID: id, SourceRef: firstNonEmptyRuntimeString(item.ToolCallID, item.ToolName, item.Reason)})
	}
	return out
}

func nonEmptyRuntimeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func firstNonEmptyRuntimeString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func summarizeSnippet(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 120
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func buildSummaryMessage(summary string, refs []ExternalReference) string {
	if len(refs) == 0 {
		return "Earlier context summary: " + summary
	}
	return fmt.Sprintf("Earlier context summary: %s Refer to external refs: %s.", summary, strings.Join(referenceIDs(refs), ", "))
}

func truncateForBudget(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return value[:maxChars]
	}
	return value[:maxChars-3] + "..."
}
