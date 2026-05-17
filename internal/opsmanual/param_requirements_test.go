package opsmanual

import "testing"

func TestBuildParamRequirementsMergesManualWorkflowAndLegacyFields(t *testing.T) {
	manual := OpsManual{
		ID:          "manual-pg-backup",
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-pg-backup"},
		Operation:   OperationProfile{TargetType: "postgresql", Action: "backup"},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"pg_isready"},
		},
		ParameterRules: map[string]ParameterRule{
			"backup_path": {Required: true, Source: "user", Validation: "path"},
		},
		PreflightProbe: PreflightProbe{RequiredOutputs: []string{"ssh_access", "pg_isready", "backup_path_writable"}},
		Metadata: map[string]any{
			"param_requirements": []any{
				map[string]any{
					"id":             "backup_path",
					"label":          "备份路径",
					"type":           "path",
					"required":       true,
					"ask_user_when":  []any{"no_candidate"},
					"resolver_hints": []any{"conversation"},
				},
			},
		},
	}
	workflowParams := []ParamRequirement{
		{ID: "backup_path", Label: "Workflow backup path", Type: "text", Required: true},
		{ID: "retention_days", Label: "保留天数", Type: "number", Required: false},
	}

	requirements := BuildParamRequirements(manual, workflowParams)
	byID := requirementsByID(requirements)
	if byID["target_host"].Type != "host_ref" || !byID["target_host"].Required {
		t.Fatalf("target_host = %#v, want required host_ref", byID["target_host"])
	}
	if byID["target_instance"].Type != "resource_ref" || byID["target_instance"].UIControl != "select" {
		t.Fatalf("target_instance = %#v, want resource_ref select", byID["target_instance"])
	}
	if byID["backup_path"].Type != "path" || byID["backup_path"].Label != "备份路径" || !containsValue(byID["backup_path"].AskUserWhen, "no_candidate") {
		t.Fatalf("backup_path = %#v, want manual metadata to win", byID["backup_path"])
	}
	if _, ok := byID["ssh_access"]; ok {
		t.Fatalf("required preflight outputs should not become user params: %#v", byID)
	}
	if byID["retention_days"].Label != "保留天数" {
		t.Fatalf("retention_days = %#v, want workflow param retained", byID["retention_days"])
	}
}

func TestNormalizeParamType(t *testing.T) {
	cases := map[string]string{
		"target_instance":   "resource_ref",
		"redis_instance":    "resource_ref",
		"target_host":       "host_ref",
		"backup_path":       "path",
		"password":          "secret_ref",
		"symptom_or_metric": "text",
	}
	for input, want := range cases {
		if got := NormalizeParamType(input, ""); got != want {
			t.Fatalf("NormalizeParamType(%q) = %q, want %q", input, got, want)
		}
	}
}

func requirementsByID(requirements []ParamRequirement) map[string]ParamRequirement {
	out := map[string]ParamRequirement{}
	for _, requirement := range requirements {
		out[requirement.ID] = requirement
	}
	return out
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
