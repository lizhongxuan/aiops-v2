package promptcompiler

import "testing"

func TestDefaultPromptSectionContractsCoverKnownSections(t *testing.T) {
	knownSections := []string{
		"system.role",
		"developer.core_rules",
		"tools.index",
		"runtime.policy",
		"protocol.state",
		"context.dynamic_assets",
		"host_task.context",
		"host_agent.runtime_overlay.v1",
		"host_agent.binding.v1",
		"host_agent.assigned_subtask.v1",
		"host_agent.execution_protocol.v1",
		"host_agent.report_contract.v1",
		"host_agent.stop_block_conditions.v1",
		"tool.result.command_output",
	}

	contracts := defaultPromptSectionContracts()
	for _, id := range knownSections {
		contract, ok := contracts[id]
		if !ok {
			t.Fatalf("expected default contract for %q", id)
		}
		if contract.ID != id {
			t.Fatalf("contract ID = %q, want %q", contract.ID, id)
		}
	}

	for id, contract := range defaultPromptSectionContracts() {
		if err := ValidatePromptSectionContract(contract); err != nil {
			t.Fatalf("default contract %q should be valid: %v", id, err)
		}
	}
}

func TestPromptSectionContractRejectsP0WithoutMaxTokens(t *testing.T) {
	contract := LookupPromptSectionContract("system.role")
	contract.MaxTokens = 0

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P0 contract without max tokens to be rejected")
	}
}

func TestPromptSectionContractRejectsP1WithoutCompactSchema(t *testing.T) {
	contract := LookupPromptSectionContract("host_task.context")
	contract.CompactSchema = ""

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P1 contract without compact schema to be rejected")
	}
}

func TestPromptSectionContractRejectsP1WithoutRequiredFields(t *testing.T) {
	contract := LookupPromptSectionContract("host_task.context")
	contract.RequiredFields = nil

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P1 contract without required fields to be rejected")
	}
}

func TestPromptSectionTraceCarriesRetentionMetadata(t *testing.T) {
	compiled := CompiledPrompt{}
	compiled.Dynamic.HostTaskPromptAssets = []string{"assigned host task context"}

	trace := BuildPromptSectionTrace(compiled)
	var hostTask PromptSectionTrace
	for _, section := range trace {
		if section.ID == "host_task.context" {
			hostTask = section
			break
		}
	}

	if hostTask.ID == "" {
		t.Fatal("expected host task prompt section trace")
	}
	if hostTask.RetentionRank != RetentionRankP1 {
		t.Fatalf("retention rank = %q, want %q", hostTask.RetentionRank, RetentionRankP1)
	}
	if hostTask.RetentionClass != RetentionClassSummarize {
		t.Fatalf("retention class = %q, want %q", hostTask.RetentionClass, RetentionClassSummarize)
	}
	if hostTask.CompactSchema == "" {
		t.Fatal("expected compact schema to be carried into trace")
	}
	if hostTask.Redaction == "" {
		t.Fatal("expected redaction policy to be carried into trace")
	}
}
