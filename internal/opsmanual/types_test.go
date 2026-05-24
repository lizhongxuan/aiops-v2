package opsmanual

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpsManualJSONUsesStableSnakeCaseFields(t *testing.T) {
	manual := OpsManual{
		ID:             "manual-redis-memory",
		ManualFamilyID: "redis-rca",
		Title:          "Redis memory triage",
		Status:         ManualStatusVerified,
		WorkflowRef: WorkflowRef{
			WorkflowID:     "workflow-redis-memory",
			WorkflowDigest: "sha256:abc",
		},
		Operation: OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss"},
		},
		Preconditions:    []string{"can connect to Redis"},
		Validation:       []string{"used_memory_rss no longer rises"},
		CannotUseWhen:    []string{"target instance unknown"},
		DocumentMarkdown: "Redis memory pressure manual",
	}
	raw, err := json.Marshal(manual)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"manual_family_id", "workflow_ref", "target_type", "required_context", "cannot_use_when"} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want field %s", text, want)
		}
	}
}

func TestDecisionStateCanonicalValues(t *testing.T) {
	cases := map[DecisionState]string{
		DecisionDirectExecute: "direct_execute",
		DecisionNeedInfo:      "need_info",
		DecisionAdapt:         "adapt",
		DecisionReference:     "reference_only",
		DecisionNoMatch:       "no_match",
	}
	for state, want := range cases {
		if string(state) != want {
			t.Fatalf("state %v = %q, want %q", state, string(state), want)
		}
	}
	if DecisionDirect != DecisionDirectExecute {
		t.Fatalf("DecisionDirect = %q, want %q", DecisionDirect, DecisionDirectExecute)
	}
	if DecisionNeedMoreInfo != DecisionNeedInfo {
		t.Fatalf("DecisionNeedMoreInfo = %q, want %q", DecisionNeedMoreInfo, DecisionNeedInfo)
	}
}

func TestOpsManualEnhancedContractRoundTrips(t *testing.T) {
	manual := OpsManual{
		ID:     "manual-k8s-pod-crashloop",
		Title:  "Kubernetes Pod CrashLoopBackOff RCA",
		Status: ManualStatusVerified,
		Tags:   []string{"k8s", "pod", "crashloopbackoff"},
		RetrievalProfile: RetrievalProfile{
			Aliases: map[string][]string{
				"kubernetes_pod": {"pod", "k8s pod", "容器组"},
				"rca_or_repair":  {"排障", "诊断", "CrashLoopBackOff"},
			},
			Keywords:         []string{"restart", "oom", "events"},
			NegativeKeywords: []string{"mysql", "postgresql", "no restart"},
			EmbeddingText:    "Kubernetes Pod CrashLoopBackOff OOMKilled diagnostics",
			MinScore: ScoreThresholds{
				Candidate:     0.55,
				DirectExecute: 0.82,
			},
		},
		RunnableConditions: RunnableConditions{
			RequiredParams:      []string{"namespace", "pod_name"},
			AllowedEnvironments: []string{"prod"},
			MaxRiskLevel:        "medium",
			RequiresApproval:    false,
		},
		PreflightProbe: PreflightProbe{
			ID:              "check_pod_status_and_rbac",
			Type:            "runner_node",
			Action:          "check_pod_status_and_rbac",
			ReadOnly:        true,
			TimeoutSeconds:  15,
			RequiredOutputs: []string{"pod_exists", "rbac_read_ok"},
		},
		RiskPolicy: RiskPolicy{
			BlastRadius:          "single_workload",
			DataMutation:         false,
			ServiceRestart:       "conditional",
			ApprovalRequiredWhen: []string{"production", "risk_level>=high"},
		},
		FallbackGuide: FallbackGuide{
			Mode:        "react_steps",
			MarkdownRef: "document_markdown#降级排障指南",
			Steps:       []string{"读取事件", "检查上次退出原因"},
		},
		Verification: VerificationProfile{
			LastVerifiedAt:        "2026-05-15T00:00:00Z",
			VerifiedBy:            "ops-review",
			RequiredPreflightPlan: true,
			RequiredRunnerDryRun:  true,
		},
	}

	raw, err := json.Marshal(manual)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"retrieval_profile", "runnable_conditions", "preflight_probe", "risk_policy", "fallback_guide", "verification"} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want field %s", text, want)
		}
	}
	var restored OpsManual
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.PreflightProbe.ID != "check_pod_status_and_rbac" || restored.RetrievalProfile.MinScore.DirectExecute != 0.82 || !restored.Verification.RequiredPreflightPlan {
		t.Fatalf("restored enhanced fields = %#v", restored)
	}
}

func TestOpsManualDiagnosisProfileRoundTrips(t *testing.T) {
	manual := OpsManual{
		ID:     "manual-redis-diagnosis",
		Title:  "Redis connection diagnosis",
		Status: ManualStatusVerified,
		Diagnosis: DiagnosisProfile{
			ApplicableSymptoms:       []string{"connection_timeout", "ping_failed"},
			NotApplicableWhen:        []string{"target_instance_unknown"},
			AllowedEvidenceSources:   []string{"redis-cli", "metrics", "read-only inventory"},
			RecommendedEvidenceOrder: []string{"confirm scope", "ping", "info", "metrics"},
			KeyJudgmentRules:         []string{"timeout is a symptom, not a root cause"},
			CommonMisdiagnoses:       []string{"treating policy_blocked lsof as no listener"},
			ConfidenceCriteria:       []string{"high requires direct redis evidence and checked refuting evidence"},
			ConservativeWording:      []string{"evidence is insufficient to confirm a root cause"},
			ApprovalRequiredActions:  []string{"restart", "config_write"},
			MinimumRiskNextSteps:     []string{"collect read-only redis INFO"},
		},
		DocumentMarkdown: "Redis diagnosis manual",
	}
	raw, err := json.Marshal(manual)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"diagnosis",
		"applicable_symptoms",
		"allowed_evidence_sources",
		"recommended_evidence_order",
		"key_judgment_rules",
		"common_misdiagnoses",
		"confidence_criteria",
		"conservative_wording",
		"approval_required_actions",
		"minimum_risk_next_steps",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want field %s", text, want)
		}
	}
	var restored OpsManual
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !contains(restored.Diagnosis.CommonMisdiagnoses, "treating policy_blocked lsof as no listener") {
		t.Fatalf("restored diagnosis = %#v", restored.Diagnosis)
	}
}

func TestPreflightResultContractRoundTrips(t *testing.T) {
	result := PreflightResult{
		Status:       PreflightStatusPassed,
		Ready:        true,
		Reason:       "all probes passed",
		ManualID:     "manual-k8s-pod-crashloop",
		WorkflowID:   "workflow-k8s-pod-rca",
		NextAction:   "confirm_execution",
		Evidence:     []PreflightEvidence{{Name: "rbac_read_ok", Status: "passed", Value: true}},
		CheckedAt:    "2026-05-15T00:00:00Z",
		ArtifactType: "ops_manual_preflight_result",
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restored PreflightResult
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.Status != PreflightStatusPassed || !restored.Ready || len(restored.Evidence) != 1 {
		t.Fatalf("restored preflight result = %#v", restored)
	}
}

func TestPreflightResultCarriesExecutionPlanContract(t *testing.T) {
	result := PreflightResult{
		Status:         PreflightStatusPassed,
		Ready:          true,
		NextAction:     "confirm_execution",
		WorkflowDigest: "sha256:workflow",
		ExecutionPlan: PreflightExecutionPlan{
			Summary:          "将对 redis-01 执行 2 个步骤",
			TargetHosts:      []string{"redis-01"},
			ActionsUsed:      []string{"builtin.tcp_ping", "script.shell"},
			RequiresApproval: false,
			RiskLevel:        "medium",
			Warnings: []PreflightPlanWarning{{
				Code:       "dry_run_variable",
				Field:      "steps.vars",
				Message:    "variable \"window\" is referenced before it is defined",
				Suggestion: "Define the variable before running.",
			}},
		},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var restored PreflightResult
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.NextAction != "confirm_execution" || restored.WorkflowDigest != "sha256:workflow" {
		t.Fatalf("restored = %#v", restored)
	}
	if len(restored.ExecutionPlan.TargetHosts) != 1 || restored.ExecutionPlan.TargetHosts[0] != "redis-01" {
		t.Fatalf("execution plan not preserved: %#v", restored.ExecutionPlan)
	}
	if restored.ExecutionPlan.Warnings[0].Code != "dry_run_variable" {
		t.Fatalf("warnings not preserved: %#v", restored.ExecutionPlan.Warnings)
	}
}

func TestParamResolutionTypesRoundTrip(t *testing.T) {
	result := ParamResolutionResult{
		Status:     ParamResolutionAmbiguous,
		ManualID:   "manual-redis-rca",
		WorkflowID: "workflow-redis-rca",
		OperationFrame: OperationFrame{
			ObjectType:    "redis",
			OperationType: "rca_or_repair",
		},
		Graph: ParamResolutionGraph{
			Nodes: []ParamResolutionNode{
				{
					ID: "target_host",
					Requirement: ParamRequirement{
						ID:            "target_host",
						Label:         "目标主机",
						Type:          "host_ref",
						Required:      true,
						DefaultSource: "selected_host",
						ResolverHints: []string{"selected_host"},
					},
					Status: "resolved",
					Resolved: &ResolvedParam{
						ID:         "target_host",
						Value:      "server-local",
						Source:     "selected_host",
						Confidence: 0.95,
					},
				},
				{
					ID: "redis_instance",
					Requirement: ParamRequirement{
						ID:        "redis_instance",
						Label:     "Redis 实例",
						Type:      "resource_ref",
						Required:  true,
						DependsOn: []string{"target_host"},
					},
					Status: "ambiguous",
					Ambiguous: &AmbiguousParam{
						ParamRequirement: ParamRequirement{ID: "redis_instance", Type: "resource_ref", Required: true},
						Reason:           "multiple candidates",
						Candidates: []ParamCandidate{
							{Value: "docker:redis-a", Label: "redis-a", Source: "docker", Confidence: 0.9},
							{Value: "docker:redis-b", Label: "redis-b", Source: "docker", Confidence: 0.9},
						},
					},
				},
			},
			Edges: []ParamResolutionEdge{{From: "target_host", To: "redis_instance"}},
		},
		ResolvedParams: []ResolvedParam{{ID: "target_host", Value: "server-local", Source: "selected_host", Confidence: 0.95}},
		AmbiguousParams: []AmbiguousParam{{
			ParamRequirement: ParamRequirement{ID: "redis_instance", Type: "resource_ref", Required: true},
			Reason:           "multiple candidates",
			Candidates:       []ParamCandidate{{Value: "docker:redis-a", Label: "redis-a"}},
		}},
		Fields: []ParamResolutionFormField{{
			ID:         "redis_instance",
			Label:      "Redis 实例",
			Type:       "resource_ref",
			Required:   true,
			UIControl:  "select",
			Candidates: []ParamCandidate{{Value: "docker:redis-a", Label: "redis-a"}},
		}},
		NextAction:   "ask_user",
		ArtifactType: "ops_manual_param_resolution",
		CreatedAt:    "2026-05-17T00:00:00Z",
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"resolved_params", "ambiguous_params", "default_source", "ui_control", "artifact_type"} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want field %s", text, want)
		}
	}
	var restored ParamResolutionResult
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.Status != ParamResolutionAmbiguous || restored.Graph.Nodes[1].Ambiguous.Candidates[0].Value != "docker:redis-a" {
		t.Fatalf("restored = %#v", restored)
	}
}

func TestCloneParamResolutionDoesNotShareSlices(t *testing.T) {
	result := ParamResolutionResult{
		Graph: ParamResolutionGraph{Nodes: []ParamResolutionNode{{
			ID:           "target_host",
			Requirement:  ParamRequirement{ID: "target_host", DependsOn: []string{"root"}},
			Dependencies: []string{"root"},
		}}},
		ResolvedParams: []ResolvedParam{{ID: "target_host", Value: "server-local", Metadata: map[string]any{"a": "b"}}},
		Fields:         []ParamResolutionFormField{{ID: "target_host", Candidates: []ParamCandidate{{Value: "server-local"}}}},
	}
	cloned := cloneParamResolutionResult(result)
	result.Graph.Nodes[0].Dependencies[0] = "mutated"
	result.Graph.Nodes[0].Requirement.DependsOn[0] = "mutated"
	result.ResolvedParams[0].Metadata["a"] = "mutated"
	result.Fields[0].Candidates[0].Value = "mutated"
	if cloned.Graph.Nodes[0].Dependencies[0] != "root" ||
		cloned.Graph.Nodes[0].Requirement.DependsOn[0] != "root" ||
		cloned.ResolvedParams[0].Metadata["a"] != "b" ||
		cloned.Fields[0].Candidates[0].Value != "server-local" {
		t.Fatalf("clone shared mutable data: %#v", cloned)
	}
}
