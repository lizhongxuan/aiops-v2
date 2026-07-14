package promptcompiler

import (
	"fmt"
	"strings"
)

type PromptSectionContract struct {
	ID              string
	Layer           string
	Source          string
	Purpose         string
	Stability       string
	RetentionClass  string
	RetentionRank   string
	MaxTokens       int
	CompactSchema   string
	RequiredFields  []string
	ExternalizeRule string
	InvalidateOn    []string
	RedactionPolicy string
	TraceRequired   bool
}

func LookupPromptSectionContract(sectionID string) PromptSectionContract {
	id := strings.TrimSpace(sectionID)
	if contract, ok := defaultPromptSectionContracts()[id]; ok {
		return contract
	}
	return PromptSectionContract{
		ID:              id,
		Layer:           "dynamic",
		Source:          "unknown",
		Purpose:         "fallback prompt section",
		Stability:       "dynamic",
		RetentionClass:  RetentionClassDropIfStale,
		RetentionRank:   RetentionRankP3,
		MaxTokens:       512,
		CompactSchema:   "fallback_summary_v1",
		RedactionPolicy: "redact_sensitive",
		TraceRequired:   true,
	}
}

func ValidatePromptSectionContract(contract PromptSectionContract) error {
	id := strings.TrimSpace(contract.ID)
	if id == "" {
		return fmt.Errorf("prompt section contract id is required")
	}
	if strings.TrimSpace(contract.RetentionRank) == "" {
		return fmt.Errorf("prompt section contract %q retention rank is required", id)
	}
	if strings.TrimSpace(contract.RetentionClass) == "" {
		return fmt.Errorf("prompt section contract %q retention class is required", id)
	}
	if strings.TrimSpace(contract.RedactionPolicy) == "" {
		return fmt.Errorf("prompt section contract %q redaction policy is required", id)
	}
	if contract.TraceRequired && strings.TrimSpace(contract.CompactSchema) == "" {
		return fmt.Errorf("prompt section contract %q compact schema is required when trace is required", id)
	}
	if requiresBoundedRetention(contract) && contract.MaxTokens <= 0 {
		return fmt.Errorf("prompt section contract %q max tokens is required for %s", id, contract.RetentionRank)
	}
	if requiresBoundedRetention(contract) && strings.TrimSpace(contract.CompactSchema) == "" {
		return fmt.Errorf("prompt section contract %q compact schema is required for %s", id, contract.RetentionRank)
	}
	if contract.RetentionRank == RetentionRankP1 && len(contract.RequiredFields) == 0 {
		return fmt.Errorf("prompt section contract %q required fields are required for %s", id, contract.RetentionRank)
	}
	return nil
}

func requiresBoundedRetention(contract PromptSectionContract) bool {
	switch contract.RetentionRank {
	case RetentionRankP0, RetentionRankP1:
		return true
	default:
		return false
	}
}

func defaultPromptSectionContracts() map[string]PromptSectionContract {
	return map[string]PromptSectionContract{
		"base.contract": {
			ID:              "base.contract",
			Layer:           "base",
			Source:          "base",
			Purpose:         "minimal non-negotiable runtime contract",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       256,
			CompactSchema:   "base_contract_v1",
			RequiredFields:  []string{"truthfulness", "evidence_boundary", "visible_tools", "failure_semantics", "task_depth", "mutation_gate"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"runtime.state": {
			ID:              "runtime.state",
			Layer:           "runtime",
			Source:          "runtime",
			Purpose:         "current mode, profile, mutation boundary, scope, and dynamic runtime flags",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "runtime_state_v1",
			RequiredFields:  []string{"mode", "profile", "mutation", "host_scope", "evidence"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"profile.advisor": {
			ID:              "profile.advisor",
			Layer:           "profile",
			Source:          "profile",
			Purpose:         "advisor profile behavior only",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       384,
			CompactSchema:   "profile_advisor_v1",
			RequiredFields:  []string{"profile"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"profile.evidence_rca": {
			ID:              "profile.evidence_rca",
			Layer:           "profile",
			Source:          "profile",
			Purpose:         "evidence RCA profile behavior only",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       384,
			CompactSchema:   "profile_evidence_rca_v1",
			RequiredFields:  []string{"profile"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"profile.host_worker": {
			ID:              "profile.host_worker",
			Layer:           "profile",
			Source:          "profile",
			Purpose:         "host worker profile behavior only",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       384,
			CompactSchema:   "profile_host_worker_v1",
			RequiredFields:  []string{"profile"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"profile.host_manager": {
			ID:              "profile.host_manager",
			Layer:           "profile",
			Source:          "profile",
			Purpose:         "host manager profile behavior only",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       384,
			CompactSchema:   "profile_host_manager_v1",
			RequiredFields:  []string{"profile"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"profile.workflow_agent": {
			ID:              "profile.workflow_agent",
			Layer:           "profile",
			Source:          "profile",
			Purpose:         "workflow editor profile behavior only",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "profile_workflow_agent_v1",
			RequiredFields:  []string{"profile"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"tool.surface": {
			ID:              "tool.surface",
			Layer:           "tool-surface",
			Source:          "tools",
			Purpose:         "visible tool capability surface",
			Stability:       "stable",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP2,
			MaxTokens:       4096,
			CompactSchema:   "tool_surface_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"dynamic.context": {
			ID:              "dynamic.context",
			Layer:           "dynamic",
			Source:          "dynamic",
			Purpose:         "bounded per-turn dynamic context, protocol state, reminders, and active prompt assets",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP1,
			MaxTokens:       2048,
			CompactSchema:   "dynamic_context_v1",
			RequiredFields:  []string{"runtime_boundaries", "active_assets", "protocol_state"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.runtime_overlay.v1": {
			ID:              "host_agent.runtime_overlay.v1",
			Layer:           "host-task",
			Source:          "host-runtime-profile",
			Purpose:         "host agent full runtime overlay",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       1024,
			CompactSchema:   "host_runtime_overlay_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.binding.v1": {
			ID:              "host_agent.binding.v1",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "bound host, mission, plan step, and risk identity",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "host_binding_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.assigned_subtask.v1": {
			ID:              "host_agent.assigned_subtask.v1",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "current assigned host subtask",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP1,
			MaxTokens:       1024,
			CompactSchema:   "host_assigned_subtask_v1",
			RequiredFields:  []string{"goal", "manager_intent", "constraints", "evidence_requirements", "completion_criteria", "source_message_id"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_task.context": {
			ID:              "host_task.context",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "current assigned host subtask",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP1,
			MaxTokens:       1024,
			CompactSchema:   "host_assigned_subtask_v1",
			RequiredFields:  []string{"goal", "manager_intent", "constraints", "evidence_requirements", "completion_criteria", "source_message_id"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.execution_protocol.v1": {
			ID:              "host_agent.execution_protocol.v1",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "host-bound execution protocol and coordination rules",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "host_execution_protocol_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.stop_block_conditions.v1": {
			ID:              "host_agent.stop_block_conditions.v1",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "host-bound stop and block conditions",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "host_stop_block_conditions_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_agent.report_contract.v1": {
			ID:              "host_agent.report_contract.v1",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "host task report output contract",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       512,
			CompactSchema:   "host_report_contract_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"tool.result.command_output": {
			ID:              "tool.result.command_output",
			Layer:           "tool-result",
			Source:          "tooling",
			Purpose:         "raw command output must be summarized or externalized",
			Stability:       "ephemeral",
			RetentionClass:  RetentionClassExternalize,
			RetentionRank:   RetentionRankP4,
			MaxTokens:       256,
			CompactSchema:   "command_output_ref_v1",
			ExternalizeRule: "externalize_raw_output",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
	}
}
