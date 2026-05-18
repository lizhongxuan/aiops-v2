package opsmanual

import (
	"strings"
	"testing"
)

func TestGenerateCandidateFromWorkflowDraftUsesWorkflowMetadataAndYAML(t *testing.T) {
	candidate, err := GenerateCandidateFromWorkflowDraft(WorkflowDraftInput{
		WorkflowID:      "wf-redis-restart",
		WorkflowVersion: "v7",
		StorageURI:      "file://workflows/redis-restart.yaml",
		YAML: strings.TrimSpace(`
version: v7
name: Redis Restart
description: Restart Redis after memory pressure validation.
vars:
  target_instance:
    required: true
  redis_password:
    required: true
x_ops_manual:
  manual_family_id: redis-restart
  target_type: redis
  action: restart
  middleware: redis
  execution_surface: ssh
  risk_level: high
  validation:
    - redis responds to PING
  cannot_use_when:
    - target instance unknown
`),
		Metadata: map[string]any{"owner": "sre"},
	})
	if err != nil {
		t.Fatalf("GenerateCandidateFromWorkflowDraft() error = %v", err)
	}

	manual := candidate.ProposedManual
	if candidate.SourceType != "workflow_draft" {
		t.Fatalf("SourceType = %q, want workflow_draft", candidate.SourceType)
	}
	if manual.Status != ManualStatusDraft {
		t.Fatalf("Status = %q, want draft", manual.Status)
	}
	if manual.WorkflowRef.WorkflowID != "wf-redis-restart" || manual.WorkflowRef.WorkflowVersion != "v7" || manual.WorkflowRef.WorkflowDigest == "" {
		t.Fatalf("WorkflowRef = %#v, want id/version/digest", manual.WorkflowRef)
	}
	if manual.ManualFamilyID != "redis-restart" || manual.Operation.TargetType != "redis" || manual.Operation.Action != "restart" {
		t.Fatalf("manual = %#v, want metadata-derived family/operation", manual)
	}
	if manual.ParameterRules["target_instance"].Required != true || manual.ParameterRules["redis_password"].Required != true {
		t.Fatalf("ParameterRules = %#v, want YAML vars imported as required rules", manual.ParameterRules)
	}
	if len(manual.Validation) != 1 || len(manual.CannotUseWhen) != 1 {
		t.Fatalf("manual gates = %#v / %#v, want validation and cannot_use_when", manual.Validation, manual.CannotUseWhen)
	}
}

func TestGenerateCandidateFromAIChatWithoutWorkflowIDRequiresWorkflowDraft(t *testing.T) {
	result, err := GenerateCandidateFromAIChat(AIChatCandidateRequest{
		Message: "帮我生成一个重启 Redis 的运维手册",
		Frame: OperationFrame{
			Target:    OperationTarget{Type: "redis"},
			Operation: OperationProfile{Action: "restart"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateCandidateFromAIChat() error = %v", err)
	}
	if !result.RequiresWorkflowDraft {
		t.Fatalf("RequiresWorkflowDraft = false, want true")
	}
	if result.Reason != "requires_workflow_draft" {
		t.Fatalf("Reason = %q, want requires_workflow_draft", result.Reason)
	}
}

func TestGenerateAdaptationCandidateKeepsManualFamilyID(t *testing.T) {
	base := redisMemoryManual()
	base.ID = "manual-redis-memory"
	base.ManualFamilyID = "redis-memory"
	base.WorkflowRef = WorkflowRef{WorkflowID: "wf-redis-memory", WorkflowVersion: "v1", WorkflowDigest: "sha256:old"}

	candidate, err := GenerateAdaptationCandidate(base, AdaptationCandidateRequest{
		VariantID:   "k8s",
		TitleSuffix: "Kubernetes variant",
		WorkflowRef: WorkflowRef{WorkflowID: "wf-redis-memory-k8s", WorkflowVersion: "v1"},
		Applicability: ApplicabilityProfile{
			Platform:         []string{"kubernetes"},
			ExecutionSurface: []string{"kubectl"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateAdaptationCandidate() error = %v", err)
	}
	manual := candidate.ProposedManual
	if manual.ManualFamilyID != "redis-memory" {
		t.Fatalf("ManualFamilyID = %q, want redis-memory", manual.ManualFamilyID)
	}
	if manual.ID == base.ID || manual.Status != ManualStatusDraft {
		t.Fatalf("manual id/status = %q/%q, want new draft variant", manual.ID, manual.Status)
	}
	if manual.WorkflowRef.WorkflowID != "wf-redis-memory-k8s" {
		t.Fatalf("WorkflowRef = %#v, want adapted workflow ref", manual.WorkflowRef)
	}
	if got := manual.Applicability.ExecutionSurface; len(got) != 1 || got[0] != "kubectl" {
		t.Fatalf("ExecutionSurface = %#v, want kubectl", got)
	}
}

func TestConvertScriptImportToWorkflowDraftBuildsMinimalDraft(t *testing.T) {
	draft, err := ConvertScriptImportToWorkflowDraft(ScriptImportRequest{
		ScriptName: "redis-memory-check.sh",
		Script:     "redis-cli INFO memory",
		Metadata: map[string]any{
			"target_type": "redis",
			"action":      "diagnose",
		},
	})
	if err != nil {
		t.Fatalf("ConvertScriptImportToWorkflowDraft() error = %v", err)
	}
	if draft.WorkflowID == "" || !strings.Contains(draft.YAML, "redis-memory-check") || !strings.Contains(draft.YAML, "redis-cli INFO memory") {
		t.Fatalf("draft = %#v, want minimal workflow draft carrying script", draft)
	}
	if draft.Metadata["source_type"] != "script_import" {
		t.Fatalf("Metadata[source_type] = %#v, want script_import", draft.Metadata["source_type"])
	}
}
