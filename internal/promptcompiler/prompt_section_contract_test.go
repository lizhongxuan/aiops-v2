package promptcompiler

import "testing"

func TestDefaultPromptSectionContractsCoverKnownSections(t *testing.T) {
	knownSections := []string{
		"base.contract",
		"runtime.state",
		"profile.advisor",
		"profile.evidence_rca",
		"profile.host_worker",
		"profile.host_manager",
		"tool.surface",
		"dynamic.context",
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
	contract := LookupPromptSectionContract("base.contract")
	contract.MaxTokens = 0

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P0 contract without max tokens to be rejected")
	}
}

func TestPromptSectionContractRejectsP1WithoutCompactSchema(t *testing.T) {
	contract := LookupPromptSectionContract("dynamic.context")
	contract.CompactSchema = ""

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P1 contract without compact schema to be rejected")
	}
}

func TestPromptSectionContractRejectsP1WithoutRequiredFields(t *testing.T) {
	contract := LookupPromptSectionContract("dynamic.context")
	contract.RequiredFields = nil

	if err := ValidatePromptSectionContract(contract); err == nil {
		t.Fatal("expected P1 contract without required fields to be rejected")
	}
}

func TestPromptSectionTraceCarriesRetentionMetadata(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{HostTaskPromptAssets: []string{"assigned host task context"}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	trace := BuildPromptSectionTrace(compiled)
	var dynamic PromptSectionTrace
	for _, section := range trace {
		if section.ID == "dynamic.context" {
			dynamic = section
			break
		}
	}

	if dynamic.ID == "" {
		t.Fatal("expected dynamic context prompt section trace")
	}
	if dynamic.RetentionRank != RetentionRankP1 {
		t.Fatalf("retention rank = %q, want %q", dynamic.RetentionRank, RetentionRankP1)
	}
	if dynamic.RetentionClass != RetentionClassSummarize {
		t.Fatalf("retention class = %q, want %q", dynamic.RetentionClass, RetentionClassSummarize)
	}
	if dynamic.CompactSchema == "" {
		t.Fatal("expected compact schema to be carried into trace")
	}
	if dynamic.Redaction == "" {
		t.Fatal("expected redaction policy to be carried into trace")
	}
}
