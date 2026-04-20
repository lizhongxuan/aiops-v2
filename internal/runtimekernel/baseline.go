// Package runtimekernel provides business logic baseline validation functions.
// These functions ensure the four-layer semantic separation, workspace state
// isolation, and tool lifecycle as truth source invariants are maintained.
package runtimekernel

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Four-Layer Semantic Separation (Req 12.1)
//
// The four semantic layers are:
//   1. Final          — user-visible assistant text (the "answer")
//   2. Process        — tool invocations, intermediate reasoning
//   3. Blocking       — approvals, evidence gates
//   4. DeepSurface    — workspace orchestration, cards, multi-agent coordination
//
// Each layer has strict boundaries: content from deeper layers must NOT
// leak into shallower layers without explicit projection.
// ---------------------------------------------------------------------------

// SemanticLayer identifies one of the four semantic layers.
type SemanticLayer = string

const (
	// LayerFinal is the user-visible assistant text output.
	LayerFinal SemanticLayer = "final"
	// LayerProcess is tool invocations and intermediate reasoning.
	LayerProcess SemanticLayer = "process"
	// LayerBlocking is approvals and evidence gates.
	LayerBlocking SemanticLayer = "blocking"
	// LayerDeepSurface is workspace orchestration, cards, and multi-agent coordination.
	LayerDeepSurface SemanticLayer = "deep_surface"
)

// validLayers is the set of valid semantic layers.
var validLayers = map[SemanticLayer]bool{
	LayerFinal:       true,
	LayerProcess:     true,
	LayerBlocking:    true,
	LayerDeepSurface: true,
}

// SemanticContent represents content tagged with its semantic layer.
type SemanticContent struct {
	Layer   SemanticLayer
	Content string
	Source  string // originating component (e.g., "projector", "workspace_router")
}

// ---------------------------------------------------------------------------
// ValidateFourLayerSeparation checks that content from deeper layers does not
// leak into shallower layers without explicit projection.
//
// Rules:
//   - Final layer content must NOT contain process/blocking/deep markers
//   - Process layer content must NOT contain deep markers
//   - Blocking layer content must NOT contain deep markers
//   - All layers must be valid
// ---------------------------------------------------------------------------

// ValidateFourLayerSeparation validates that the given content items respect
// the four-layer semantic separation. Returns an error if any violation is found.
func ValidateFourLayerSeparation(contents []SemanticContent) error {
	for _, c := range contents {
		if !validLayers[c.Layer] {
			return fmt.Errorf("baseline: invalid semantic layer %q", c.Layer)
		}

		switch c.Layer {
		case LayerFinal:
			if containsProcessMarkers(c.Content) {
				return fmt.Errorf("baseline: final layer contains process-layer markers")
			}
			if containsBlockingMarkers(c.Content) {
				return fmt.Errorf("baseline: final layer contains blocking-layer markers")
			}
			if containsDeepSurfaceMarkers(c.Content) {
				return fmt.Errorf("baseline: final layer contains deep-surface markers")
			}

		case LayerProcess:
			if containsDeepSurfaceMarkers(c.Content) {
				return fmt.Errorf("baseline: process layer contains deep-surface markers")
			}

		case LayerBlocking:
			if containsDeepSurfaceMarkers(c.Content) {
				return fmt.Errorf("baseline: blocking layer contains deep-surface markers")
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Workspace State Isolation (Req 12.4)
//
// Workspace state (agent status, plan steps, choice state) must NOT flow
// back into the assistant's text body. It can only be surfaced through the
// Projection layer as structured data (cards, snapshots).
// ---------------------------------------------------------------------------

// ValidateWorkspaceStateIsolation checks that workspace state does not flow
// back into assistant text content. Returns an error if a violation is found.
func ValidateWorkspaceStateIsolation(assistantText string) error {
	if containsWorkspaceStateMarkers(assistantText) {
		return fmt.Errorf("baseline: workspace state leaked into assistant text")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tool Lifecycle as Truth Source (Req 12.6)
//
// The tool lifecycle (started → progress → completed/failed) is the single
// source of truth for process state. Tool state must be derived from lifecycle
// events, NOT from parsing assistant text.
// ---------------------------------------------------------------------------

// ToolStateSource identifies where tool state was derived from.
type ToolStateSource string

const (
	// ToolStateFromLifecycle means state was derived from lifecycle events (valid).
	ToolStateFromLifecycle ToolStateSource = "lifecycle"
	// ToolStateFromDisplay means state was derived from display/projection (valid).
	ToolStateFromDisplay ToolStateSource = "display"
	// ToolStateFromAssistantText means state was derived from assistant text (INVALID).
	ToolStateFromAssistantText ToolStateSource = "assistant_text"
)

// ValidateToolStateSource checks that the given source is a valid truth source
// for tool state. Returns an error if the source is not authoritative.
func ValidateToolStateSource(source ToolStateSource) error {
	switch source {
	case ToolStateFromLifecycle, ToolStateFromDisplay:
		return nil
	case ToolStateFromAssistantText:
		return fmt.Errorf("baseline: tool state derived from assistant text is forbidden; use lifecycle events")
	default:
		return fmt.Errorf("baseline: unknown tool state source %q", source)
	}
}

// ToolLifecycleRecord represents a tool state record with its derivation source.
type ToolLifecycleRecord struct {
	ToolCallID string
	ToolName   string
	Status     string // "started", "progress", "completed", "failed"
	Source     ToolStateSource
}

// ValidateToolLifecycleRecords checks that all tool lifecycle records are
// derived from authoritative sources (lifecycle events or display projection).
// Returns an error if any record is derived from a non-authoritative source.
func ValidateToolLifecycleRecords(records []ToolLifecycleRecord) error {
	for _, r := range records {
		if err := ValidateToolStateSource(r.Source); err != nil {
			return fmt.Errorf("baseline: tool %q (call %s): %w", r.ToolName, r.ToolCallID, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Composite Baseline Validation
// ---------------------------------------------------------------------------

// BaselineReport holds the results of a full baseline validation.
type BaselineReport struct {
	Compliant  bool
	Violations []string
}

// ValidateBaseline runs all baseline validations and returns a composite report.
func ValidateBaseline(
	semanticContents []SemanticContent,
	assistantText string,
	toolRecords []ToolLifecycleRecord,
) BaselineReport {
	var violations []string

	if err := ValidateFourLayerSeparation(semanticContents); err != nil {
		violations = append(violations, err.Error())
	}

	if err := ValidateWorkspaceStateIsolation(assistantText); err != nil {
		violations = append(violations, err.Error())
	}

	if err := ValidateToolLifecycleRecords(toolRecords); err != nil {
		violations = append(violations, err.Error())
	}

	return BaselineReport{
		Compliant:  len(violations) == 0,
		Violations: violations,
	}
}

// ---------------------------------------------------------------------------
// Internal marker detection helpers
// ---------------------------------------------------------------------------

// processMarkers indicate raw tool/process output that should not appear in final layer.
var processMarkers = []string{
	"[tool_started]",
	"[tool_progress]",
	"[tool_completed]",
	"[tool_failed]",
	"[internal_reasoning]",
}

// blockingMarkers indicate approval/blocking internals.
var blockingMarkers = []string{
	"[approval_pending]",
	"[approval_decided]",
	"[evidence_required]",
	"[evidence_collected]",
}

// deepSurfaceMarkers indicate workspace orchestration / card data internals.
var deepSurfaceMarkers = []string{
	"[card_data]",
	"[snapshot_data]",
	"[workspace_state]",
	"[mission_state]",
}

// workspaceStateMarkers indicate workspace state leaked into assistant text.
var workspaceStateMarkers = []string{
	"[agent_status:",
	"[plan_step:",
	"[choice_pending]",
	"[task_state:",
	"[agent_instance:",
	"[budget_state:",
	"[mission_progress:",
}

func containsProcessMarkers(s string) bool {
	for _, m := range processMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func containsBlockingMarkers(s string) bool {
	for _, m := range blockingMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func containsDeepSurfaceMarkers(s string) bool {
	for _, m := range deepSurfaceMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func containsWorkspaceStateMarkers(s string) bool {
	for _, m := range workspaceStateMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
