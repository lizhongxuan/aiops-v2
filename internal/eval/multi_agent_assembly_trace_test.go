package eval

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestScoreCaseChecksMultiAgentAssemblyTrace(t *testing.T) {
	tracePayload := map[string]any{
		"assembly": map[string]any{
			"agentKind":   "advisor",
			"profile":     "advisor.default",
			"runtimeRole": "profile_only",
			"routeReason": "simple advisory request",
		},
		"resources": map[string]any{
			"verified": []string{"hostA"},
			"rejected": []string{"hostB"},
		},
		"toolSurface": map[string]any{
			"fingerprint":       "surface:abc",
			"visibleTools":      []string{"search_ops_manuals"},
			"dispatchableTools": []string{"search_ops_manuals"},
			"hiddenTools":       []string{"exec_command"},
		},
		"sessionTargets": map[string]any{
			"bindingMode":  "single_host",
			"hostIds":      []string{"hostA"},
			"sourceTurnId": "turn-1",
		},
		"roleBindings": []map[string]any{{
			"resourceId":    "hostA",
			"role":          "pg_primary",
			"conflictState": "none",
		}},
		"finalReport": map[string]any{
			"hostId":       "hostA",
			"boundRole":    "pg_primary",
			"evidenceRefs": []string{"tool:hostA:replication-status"},
		},
		"promptInputTrace": map[string]any{
			"assembly_source":        "runtimekernel.buildModelInputTraceRequest",
			"prompt_compiler_source": "promptcompiler.Compiler",
			"tool_surface_source":    "runtimekernel.applyToolSurfacePolicyToCompileContext",
			"adapter_name":           "eino",
			"toolSurfaceFingerprint": "surface:abc",
			"promptSections": []map[string]any{{
				"id":     "base.contract",
				"source": "promptcompiler.base",
				"hash":   "sha256:section",
			}},
		},
	}
	data, _ := json.Marshal(tracePayload)
	score := ScoreCase(Case{
		ID:       "multi-agent-trace",
		Category: "multi_agent_assembly",
		Expected: Expected{
			ExpectedAssembly: &ExpectedAssemblyTrace{
				AgentKind:           "advisor",
				Profile:             "advisor.default",
				RuntimeRole:         "profile_only",
				RouteReasonContains: "advisory",
			},
			ExpectedResources: &ExpectedResourceTrace{
				VerifiedIDs: []string{"hostA"},
				RejectedIDs: []string{"hostB"},
			},
			ExpectedToolSurface: &ExpectedToolSurfaceTrace{
				VisibleTools:        []string{"search_ops_manuals"},
				DispatchableTools:   []string{"search_ops_manuals"},
				HiddenTools:         []string{"exec_command"},
				RequireSingleSource: true,
			},
			ExpectedSessionTargets: &ExpectedSessionTargetTrace{
				BindingMode:  "single_host",
				HostIDs:      []string{"hostA"},
				SourceTurnID: "turn-1",
			},
			ExpectedRoleBindings: []ExpectedRoleBindingTrace{{
				ResourceID:    "hostA",
				Role:          "pg_primary",
				ConflictState: "none",
			}},
			ExpectedFinalReport: &ExpectedFinalReportTrace{
				HostIDs:      []string{"hostA"},
				Roles:        []string{"pg_primary"},
				EvidenceRefs: []string{"tool:hostA:replication-status"},
			},
			ExpectedTraceExplainability: &ExpectedTraceExplainability{
				RequireAssemblySource:         true,
				RequirePromptCompilerSource:   true,
				RequireToolSurfaceSource:      true,
				RequireAdapterName:            true,
				RequirePromptSectionHashes:    true,
				RequireToolSurfaceFingerprint: true,
			},
		},
	}, RunOutput{
		Answer: "assembly trace verified with concrete evidence and validation command: go test ./internal/eval.",
		TurnItems: []agentstate.TurnItem{{
			ID:        "model-trace",
			Type:      agentstate.TurnItemTypeModelCall,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Kind: "multi_agent_assembly_trace", Summary: "assembly trace", Data: data},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}}})

	for _, check := range score.Checks {
		if !check.Passed {
			t.Fatalf("check %s failed: %#v", check.Name, check)
		}
	}
}

func TestScoreCaseRequiresExpectedFinalReportSignals(t *testing.T) {
	c := Case{
		ID:       "pg-role-final-report",
		Category: "multi_agent_assembly",
		Expected: Expected{
			ExpectedFinalReport: &ExpectedFinalReportTrace{
				HostIDs:      []string{"hostA"},
				Roles:        []string{"pg_primary"},
				EvidenceRefs: []string{"tool:hostA:replication-status"},
			},
		},
	}
	score := ScoreCase(c, RunOutput{Answer: "hostA pg_primary"})
	check := findAssemblyTraceCheck(score.Checks, "expectedFinalReport")
	if check.Passed {
		t.Fatalf("expectedFinalReport passed without evidence ref: %#v", check)
	}

	payload := map[string]any{
		"finalReport": map[string]any{
			"hostId":       "hostA",
			"boundRole":    "pg_primary",
			"evidenceRefs": []string{"tool:hostA:replication-status"},
		},
	}
	data, _ := json.Marshal(payload)
	score = ScoreCase(c, RunOutput{
		Answer: "final report ready with verification command: go test ./internal/eval.",
		TurnItems: []agentstate.TurnItem{{
			ID:        "final-report",
			Type:      agentstate.TurnItemTypeAssistantMessage,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Kind: "host_task_report", Summary: "hostA pg_primary", Data: data},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}},
	})
	check = findAssemblyTraceCheck(score.Checks, "expectedFinalReport")
	if !check.Passed {
		t.Fatalf("expectedFinalReport failed with complete report: %#v", check)
	}
}

func findAssemblyTraceCheck(checks []CheckResult, name string) CheckResult {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return CheckResult{Name: name, Passed: false, Detail: "missing check"}
}
