package runtimekernel

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultAggregateToolResultRatio = 18
	minAggregateToolResultTokens    = 2000
)

type AggregateToolResultBudgetInput struct {
	SessionID            string
	TurnID               string
	Iteration            int
	Results              []ToolResult
	MaxAggregateTokens   int
	CurrentNonToolTokens int
	Thresholds           ContextBudgetThresholds
	Externalize          func(ToolResult) (ExternalReference, error)
	CreatedAt            time.Time
}

type AggregateToolResultBudgetResult struct {
	Results            []ToolResult
	Events             []ContextGovernanceEvent
	Applied            bool
	BeforeTokens       int
	AfterTokens        int
	MaxAggregateTokens int
	ReferenceIDs       []string
}

func ApplyAggregateToolResultBudget(input AggregateToolResultBudgetInput) AggregateToolResultBudgetResult {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	results := cloneToolResults(input.Results)
	maxTokens := aggregateToolResultBudgetTokens(input)
	before := estimateToolResultsTokens(results)
	after := before
	out := AggregateToolResultBudgetResult{
		Results:            results,
		BeforeTokens:       before,
		AfterTokens:        after,
		MaxAggregateTokens: maxTokens,
	}
	if len(results) == 0 || maxTokens <= 0 || before <= maxTokens {
		return out
	}

	candidates := aggregateBudgetCandidates(results)
	for _, candidate := range candidates {
		if after <= maxTokens {
			break
		}
		idx := candidate.index
		ref, err := aggregateBudgetExternalRef(input, results[idx], createdAt)
		if err != nil {
			continue
		}
		originalTokens := EstimateTokens(Message{Role: "tool", Content: results[idx].Content})
		results[idx] = externalizedAggregateToolResult(results[idx], ref)
		after -= originalTokens
		after += EstimateTokens(Message{Role: "tool", Content: results[idx].Content})
		out.ReferenceIDs = append(out.ReferenceIDs, ref.ID)
	}

	out.Results = results
	out.AfterTokens = after
	out.Applied = len(out.ReferenceIDs) > 0
	if out.Applied {
		out.Events = []ContextGovernanceEvent{BuildContextGovernanceEvent(ContextGovernanceEvent{
			ID:           fmt.Sprintf("ctxgov-%s-%d-l1-aggregate-tool-budget", input.TurnID, input.Iteration),
			Layer:        ContextGovernanceLayerL1,
			Kind:         "tool_result.aggregate_budget_applied",
			SessionID:    input.SessionID,
			TurnID:       input.TurnID,
			Iteration:    input.Iteration,
			Message:      fmt.Sprintf("工具结果聚合预算已应用，before=%d after=%d max=%d", before, after, maxTokens),
			Budget:       input.Thresholds,
			ReferenceIDs: append([]string(nil), out.ReferenceIDs...),
			CreatedAt:    createdAt,
		})}
	}
	return out
}

type aggregateBudgetCandidate struct {
	index  int
	tokens int
}

func aggregateToolResultBudgetTokens(input AggregateToolResultBudgetInput) int {
	if input.MaxAggregateTokens > 0 {
		return input.MaxAggregateTokens
	}
	effective := input.Thresholds.EffectiveContextWindow
	if effective <= 0 {
		effective = input.Thresholds.MaxContextTokens - input.Thresholds.ReservedOutputTokens
	}
	if effective <= 0 {
		effective = DefaultMaxTokens
	}
	ratioBudget := effective * defaultAggregateToolResultRatio / 100
	remainingBudget := effective - input.Thresholds.ReservedOutputTokens - input.CurrentNonToolTokens - input.Thresholds.BlockingLimit
	maxTokens := ratioBudget
	if remainingBudget > 0 && remainingBudget < maxTokens {
		maxTokens = remainingBudget
	}
	if maxTokens < minAggregateToolResultTokens {
		maxTokens = minAggregateToolResultTokens
	}
	return maxTokens
}

func estimateToolResultsTokens(results []ToolResult) int {
	total := 0
	for _, result := range results {
		total += EstimateTokens(Message{Role: "tool", Content: result.Content})
	}
	return total
}

func aggregateBudgetCandidates(results []ToolResult) []aggregateBudgetCandidate {
	candidates := make([]aggregateBudgetCandidate, 0, len(results))
	for i, result := range results {
		if !isAggregateBudgetExternalizable(result) {
			continue
		}
		candidates = append(candidates, aggregateBudgetCandidate{
			index:  i,
			tokens: EstimateTokens(Message{Role: "tool", Content: result.Content}),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].tokens > candidates[j].tokens
	})
	return candidates
}

func isAggregateBudgetExternalizable(result ToolResult) bool {
	if strings.TrimSpace(result.Error) != "" {
		return false
	}
	if result.Spilled || len(result.ExternalReferences) > 0 {
		return false
	}
	if strings.TrimSpace(result.Content) == "" {
		return false
	}
	return true
}

func aggregateBudgetExternalRef(input AggregateToolResultBudgetInput, result ToolResult, createdAt time.Time) (ExternalReference, error) {
	if input.Externalize != nil {
		return input.Externalize(result)
	}
	id := "agg-spill-" + digestContent(result.ToolCallID + "|" + result.Content)[:12]
	return ExternalReference{
		ID:          id,
		SessionID:   input.SessionID,
		TurnID:      input.TurnID,
		Iteration:   input.Iteration,
		Kind:        string(ToolResultReferenceKindBlob),
		URI:         "store://tool-result-aggregate/" + id,
		Title:       result.ToolCallID,
		Summary:     fallbackSummary(result.Summary, result.Content, defaultMaxInlineResultBytes),
		ContentType: detectResultContentType(result.Content),
		Digest:      digestContent(result.Content),
		Bytes:       int64(len(result.Content)),
		CreatedAt:   createdAt,
	}, nil
}

func externalizedAggregateToolResult(result ToolResult, ref ExternalReference) ToolResult {
	result.Spilled = true
	result.Summary = fallbackSummary(result.Summary, result.Content, defaultMaxInlineResultBytes)
	result.OriginalBytes = int64(len(result.Content))
	result.Content = fmt.Sprintf("Summary: %s\nExternal ref: %s.", result.Summary, externalReferenceLabel(ref))
	result.InlineBytes = int64(len(result.Content))
	result.MaterializationTier = string(toolResultTierLarge)
	result.References = appendToolResultReferences(result.References, toolResultReferenceFromExternalRef(ref))
	appendExternalReferences(&result.ExternalReferences, ref)
	return result
}

func cloneToolResults(results []ToolResult) []ToolResult {
	out := make([]ToolResult, len(results))
	for i := range results {
		out[i] = results[i]
		out[i].References = append([]ToolResultReference(nil), results[i].References...)
		out[i].ExternalReferences = append([]ExternalReference(nil), results[i].ExternalReferences...)
	}
	return out
}
