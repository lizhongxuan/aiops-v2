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
		"system.role": {
			ID:              "system.role",
			Layer:           "system",
			Source:          "prompt-compiler",
			Purpose:         "base agent role and environment",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       2048,
			CompactSchema:   "system_role_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"developer.core_rules": {
			ID:              "developer.core_rules",
			Layer:           "developer",
			Source:          "prompt-compiler",
			Purpose:         "core runtime and safety rules",
			Stability:       "stable",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       8192,
			CompactSchema:   "developer_core_rules_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"tools.index": {
			ID:              "tools.index",
			Layer:           "tool-index",
			Source:          "tool-registry",
			Purpose:         "visible tool capability index",
			Stability:       "stable",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP2,
			MaxTokens:       4096,
			CompactSchema:   "tool_index_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"runtime.policy": {
			ID:              "runtime.policy",
			Layer:           "runtime-policy",
			Source:          "runtime-policy",
			Purpose:         "mode and approval policy",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassMustKeep,
			RetentionRank:   RetentionRankP0,
			MaxTokens:       2048,
			CompactSchema:   "runtime_policy_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"protocol.state": {
			ID:              "protocol.state",
			Layer:           "protocol-state",
			Source:          "protocol-state",
			Purpose:         "current plan, todo, approval, and failure state",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP1,
			MaxTokens:       2048,
			CompactSchema:   "protocol_state_v1",
			RequiredFields:  []string{"plan_state", "todo_state", "approval_state", "failure_state"},
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"context.dynamic_assets": {
			ID:              "context.dynamic_assets",
			Layer:           "dynamic-assets",
			Source:          "dynamic-assets",
			Purpose:         "legacy dynamic prompt asset fingerprint",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP2,
			MaxTokens:       1024,
			CompactSchema:   "dynamic_assets_v1",
			RedactionPolicy: "redact_sensitive",
			TraceRequired:   true,
		},
		"host_task.context": {
			ID:              "host_task.context",
			Layer:           "host-task",
			Source:          "host-task",
			Purpose:         "assigned host-bound task context",
			Stability:       "dynamic",
			RetentionClass:  RetentionClassSummarize,
			RetentionRank:   RetentionRankP1,
			MaxTokens:       2048,
			CompactSchema:   "host_task_context_v1",
			RequiredFields:  []string{"goal", "manager_intent", "constraints", "evidence_requirements", "completion_criteria", "source_message_id"},
			ExternalizeRule: "externalize_large_fields",
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
