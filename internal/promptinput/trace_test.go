package promptinput

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/specialinputmemory"
	"aiops-v2/internal/tooling"
)

func TestPromptInputTraceJSONAndMarkdownExplainSources(t *testing.T) {
	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
			Tools:  promptcompiler.ToolPromptSet{Content: "tool index"},
			Dynamic: promptcompiler.DynamicPromptDelta{
				ProtocolState: promptcompiler.ProtocolPromptState{
					Items: []promptcompiler.ProtocolPromptItem{
						{Kind: "plan", ID: "step-1", Status: "in_progress", Text: "inspect logs"},
					},
				},
			},
		},
		History: []Message{
			{Role: "user", Content: "triage"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "read_logs"}}},
			{Role: "tool", Content: "log output", ToolResult: &ToolResult{ToolCallID: "call-1", Content: "log output"}},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	data, err := json.Marshal(result.Trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{
		`"source":"runtime"`,
		`"semanticRole":"L2_stable_runtime_contract"`,
		`"providerRole":"system"`,
		`"source":"protocol_state"`,
		`"semanticRole":"tool_result"`,
	} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}

	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{
		"# Prompt Input Trace",
		"L0_absolute_system_core",
		"L2_stable_runtime_contract",
		"dynamic.tool_surface",
		"protocol_state",
		"tool_result",
		"provider",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}

func TestPromptInputTraceIncludesSpecialInputWorldState(t *testing.T) {
	worldState := &specialinputmemory.SpecialInputWorldStateSection{
		SchemaVersion: specialinputmemory.SchemaVersion,
		TurnID:        "turn-special",
		ActiveExecutionScope: &specialinputmemory.ExecutionScopeGrantTrace{
			ID:             "grant-host-a",
			ResourceKind:   specialinputmemory.ResourceKindHost,
			ResourceID:     "host-a",
			AllowedActions: []string{specialinputmemory.ActionRead},
			ValidationHash: "hash-a",
		},
		ActiveRoleBindings: []specialinputmemory.RoleBindingTrace{{
			RoleKey:        "pg_primary",
			RuntimeName:    "primary",
			ResourceKind:   specialinputmemory.ResourceKindHost,
			ResourceID:     "host-a",
			EnvironmentKey: "prod",
			ClusterKey:     "orders",
			BindingHash:    "role-hash-a",
		}},
		PendingConfirmations: []specialinputmemory.PendingConfirmation{{ID: "pending-raw", Kind: "target", Reason: "raw_typed_requires_confirmation"}},
		ReadPlan: &specialinputmemory.MemoryReadPlanTrace{
			ActiveGrantID:          "grant-host-a",
			ActiveResourceKind:     specialinputmemory.ResourceKindHost,
			ActiveResourceID:       "host-a",
			PendingConfirmationIDs: []string{"pending-raw"},
		},
		ModelSummary: "active host host-a from previous confirmed mention",
	}

	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{SpecialInputWorldState: worldState})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.SpecialInputWorldState == nil || result.Trace.SpecialInputWorldState.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("SpecialInputWorldState = %#v, want host-a", result.Trace.SpecialInputWorldState)
	}
	worldState.ActiveExecutionScope.AllowedActions[0] = specialinputmemory.ActionMutate
	if result.Trace.SpecialInputWorldState.ActiveExecutionScope.AllowedActions[0] != specialinputmemory.ActionRead {
		t.Fatalf("trace did not clone world state: %#v", result.Trace.SpecialInputWorldState)
	}

	data, err := json.Marshal(result.Trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	if !strings.Contains(string(data), `"specialInputWorldState"`) || !strings.Contains(string(data), `"grant-host-a"`) {
		t.Fatalf("json trace missing special input state:\n%s", string(data))
	}
	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{"## Special Input Memory", "grant-host-a", "pg_primary", "pending-raw"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestPromptInputTraceIncludesOpsContextBudgetMetrics(t *testing.T) {
	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
		},
		Tools: []promptcompiler.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "search_ops_manuals"}},
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "host_read"}},
		},
		Memories:              []MemoryItem{{ID: "mem-1", Text: "prior target"}},
		OpsContextCapsule:     "flow: flow-1\ncurrent_target: redis",
		SessionFactCount:      5,
		LettaHintCount:        2,
		DroppedContextReasons: []string{"letta_hint_limit", "artifact_ref_only"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.OpsContextCapsuleChars == 0 ||
		result.Trace.SessionFactCount != 5 ||
		result.Trace.LettaHintCount != 2 ||
		result.Trace.MemoryItemCount != 1 ||
		!containsString(result.Trace.VisibleOpsManualTools, "search_ops_manuals") ||
		!containsString(result.Trace.DroppedContextReasons, "artifact_ref_only") {
		t.Fatalf("trace metrics = %#v", result.Trace)
	}
	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{"ops_context_capsule_chars", "session_fact_count", "visible_ops_manual_tools", "artifact_ref_only"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}

func TestPromptInputRendersCompactPlanModeAndTaskState(t *testing.T) {
	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			Dynamic: promptcompiler.DynamicPromptDelta{
				ProtocolState: promptcompiler.ProtocolPromptState{
					PlanMode: &promptcompiler.PlanModePromptState{
						State:          "active",
						PlanID:         "plan-synthetic-1",
						ArtifactStatus: "pending_exit_approval",
						ApprovalStatus: "pending_exit_approval",
						ReminderLevel:  "sparse",
					},
					TaskTodo: &promptcompiler.TaskTodoPromptState{
						Items: []promptcompiler.TaskTodoPromptItem{
							{ID: "step-2", Status: "in_progress", Owner: "agent:planner", PendingEvidence: "requires verification"},
							{ID: "step-3", Status: "blocked", BlockedBy: "missing_user_input"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.PlanModeState == nil || result.Trace.PlanModeState.PlanID != "plan-synthetic-1" {
		t.Fatalf("trace plan mode state = %#v", result.Trace.PlanModeState)
	}
	if result.Trace.TaskTodoState == nil || len(result.Trace.TaskTodoState.Items) != 2 {
		t.Fatalf("trace task state = %#v", result.Trace.TaskTodoState)
	}
	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{
		"### Plan Mode State",
		"state: active",
		"plan_id: plan-synthetic-1",
		"approval_status: pending_exit_approval",
		"reminder_level: sparse",
		"### Task/Todo State",
		"in_progress: step-2 owner=agent:planner",
		"blocked: step-3 blocked_by=missing_user_input",
		"pending_evidence: step-2 requires verification",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}

func TestPromptInputRendersCompactVerificationSafetyState(t *testing.T) {
	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{
		VerificationReportRef: "artifact://synthetic/verification-report",
		VerificationStatus:    "PARTIAL",
		CompletionGate: &CompletionGateTrace{
			Decision: "block_success_final",
			Reasons:  []string{"execution_evidence_missing", "partial_requires_blocker"},
		},
		SafetySignals: []SafetySignalTrace{{
			Category: "destructive_workaround",
			Severity: "high",
			Action:   "require_approval",
			Reasons:  []string{"force overwrite requested with secret=synthetic-secret"},
		}},
		UnexpectedStateGate: &UnexpectedStateGateTrace{
			Action:         "block_mutation",
			Sources:        []string{"synthetic_tool_result"},
			AffectedScopes: []string{"synthetic.resource.scope"},
			BlockedAction:  "overwrite",
			Reasons:        []string{"unexpected_state"},
		},
		ApprovalScope: &ApprovalScopeTrace{
			Status:         "pending",
			AllowedActions: []string{"inspect"},
			ResourceScopes: []string{"synthetic.resource.scope"},
			RiskCeiling:    "medium",
			ExpiresAt:      "2026-06-07T10:00:00Z",
			InputHash:      "sha256:synthetic-input",
			Reasons:        []string{"scope does not allow mutation"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.VerificationStatus != "PARTIAL" || result.Trace.CompletionGate == nil || result.Trace.UnexpectedStateGate == nil || result.Trace.ApprovalScope == nil {
		t.Fatalf("verification safety trace missing fields: %#v", result.Trace)
	}

	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{
		"### Verification/Safety State",
		"verification_status: PARTIAL",
		"verification_report_ref: artifact://synthetic/verification-report",
		"completion_gate: block_success_final",
		"safety: destructive_workaround/high action=require_approval",
		"unexpected_state_gate: block_mutation",
		"approval_scope: pending",
		"input_hash=sha256:synthetic-input",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
	for _, forbidden := range []string{"synthetic-secret", "raw evidence"} {
		if strings.Contains(markdown, forbidden) {
			t.Fatalf("compact markdown leaked %q:\n%s", forbidden, markdown)
		}
	}
}

func TestPromptInputRendersGenericityTraceDepthAndCoverage(t *testing.T) {
	result, err := buildCanonicalPromptInputForTest(t, BuildRequest{
		TaskDepth: &TaskDepthTrace{
			Level:              "investigation",
			Reasons:            []string{"cross_resource_evidence"},
			RequiresPlan:       true,
			RequiresEvidence:   true,
			RequiresValidation: true,
		},
		EvidenceCoverage: &EvidenceCoverageTrace{
			Action:             "continue_gathering",
			Coverage:           0.4,
			RequiredDimensions: []string{"plan_context", "tool_evidence", "verification"},
			CoveredDimensions:  []string{"plan_context"},
			MissingDimensions:  []string{"tool_evidence", "verification"},
			OpenQuestions:      []string{"synthetic_resource_a status"},
			VerificationStatus: "PARTIAL",
			Reasons:            []string{"missing_coverage_dimension"},
		},
		GenericityTrace: &GenericityTrace{
			CoreRuleDomainTerms: []string{"synthetic_resource_b"},
			AllowedFixtureTerms: []string{"synthetic_resource_a"},
			AllowedPluginTerms:  []string{"synthetic_component"},
			ResourceIDSource:    "fixture",
			Violations:          []string{"token=synthetic-secret"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.TaskDepth == nil || result.Trace.TaskDepth.Level != "investigation" {
		t.Fatalf("task depth trace = %#v", result.Trace.TaskDepth)
	}
	if result.Trace.EvidenceCoverage == nil || result.Trace.EvidenceCoverage.Action != "continue_gathering" {
		t.Fatalf("coverage trace = %#v", result.Trace.EvidenceCoverage)
	}
	if result.Trace.GenericityTrace == nil || result.Trace.GenericityTrace.ResourceIDSource != "fixture" {
		t.Fatalf("genericity trace = %#v", result.Trace.GenericityTrace)
	}

	data, err := json.Marshal(result.Trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{`"taskDepth"`, `"requiresPlan":true`, `"evidenceCoverage"`, `"genericityTrace"`, `"resourceIdSource":"fixture"`} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}

	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{
		"### Task Depth",
		"task_depth: investigation",
		"required_gates: plan,evidence,validation",
		"### Evidence Coverage",
		"coverage_action: continue_gathering",
		"missing_dimensions: tool_evidence, verification",
		"### Genericity Trace",
		"resource_id_source: fixture",
		"allowed_fixture_terms: synthetic_resource_a",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "synthetic-secret") || !strings.Contains(markdown, "token=[REDACTED]") {
		t.Fatalf("markdown did not redact genericity violation:\n%s", markdown)
	}
}

func TestPromptInputTraceCarriesContextGovernance(t *testing.T) {
	req := BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
		},
		ContextGovernance: []ContextGovernanceTraceItem{{
			Layer:        "L4",
			Kind:         "context.compaction.started",
			Message:      "compacting context",
			Budget:       map[string]int{"autoCompactThreshold": 167000, "blockingLimit": 177000},
			ReferenceIDs: []string{"ref-1", "ref-2"},
			RetryAttempt: 1,
			RetryMax:     3,
		}},
	}
	result, err := buildCanonicalPromptInputForTest(t, req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	req.ContextGovernance[0].Budget["autoCompactThreshold"] = 1
	req.ContextGovernance[0].ReferenceIDs[0] = "mutated"

	got := result.Trace.ContextGovernance
	if len(got) != 1 {
		t.Fatalf("context governance length = %d, want 1", len(got))
	}
	if got[0].Layer != "L4" ||
		got[0].Kind != "context.compaction.started" ||
		got[0].Budget["autoCompactThreshold"] != 167000 ||
		got[0].ReferenceIDs[0] != "ref-1" ||
		got[0].RetryAttempt != 1 ||
		got[0].RetryMax != 3 {
		t.Fatalf("context governance trace = %#v", got[0])
	}
}

func TestPromptInputTraceNormalizesPromptSectionHarnessMetadata(t *testing.T) {
	req := BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
			PromptSections: []promptcompiler.PromptSectionTrace{{
				ID:             "protocol.state",
				Kind:           "system",
				Source:         "compiler",
				Hash:           "sha256:protocol",
				TokensEstimate: 8,
				RetentionRank:  promptcompiler.RetentionRankP0,
				CompactAction:  promptcompiler.CompactActionKeptOriginal,
			}},
		},
	}
	result, err := buildCanonicalPromptInputForTest(t, req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Trace.PromptSections) != 1 {
		t.Fatalf("prompt sections = %#v", result.Trace.PromptSections)
	}
	section := result.Trace.PromptSections[0]
	if section.TokenEstimate != 8 || section.Action != "kept" || section.RetentionRank != promptcompiler.RetentionRankP0 {
		t.Fatalf("prompt section harness metadata = %#v", section)
	}
}

func TestPromptInputRendersAgentSchedulingState(t *testing.T) {
	trace := PromptInputTrace{
		AgentIndexHash: "agent-index-sha256:synthetic",
		AgentIndexEntries: []AgentIndexEntryTrace{{
			Kind:            "explorer",
			Name:            "synthetic.explorer",
			WhenToUse:       "Use for independent read-only evidence checks.",
			CapabilityKinds: []string{"evidence"},
			ResourceTypes:   []string{"generic_resource"},
			OperationKinds:  []string{"read"},
			MaxConcurrent:   2,
			CostClass:       "low",
		}},
		AgentDelegationDecision: &AgentDelegationDecisionTrace{
			Action:         "spawn_new",
			Reason:         "independent_evidence_surface",
			CandidateAgent: "synthetic.explorer",
		},
		AgentAssignmentLint: []AgentAssignmentLintTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "pass",
		}},
		AgentParallelTraceGroups: []AgentParallelTraceGroup{{
			MissionID:      "synthetic-mission",
			RequestedCount: 2,
			SpawnedInTurn:  []string{"synthetic-worker-1", "synthetic-worker-2"},
			Queued:         []string{"synthetic-worker-3"},
			SerialReasons:  []AgentSerialReasonTrace{{AgentID: "synthetic-worker-3", Reason: "budget_exceeded"}},
		}},
		ResourceLocks: []ResourceLockTrace{{
			LeaseID: "lease-synthetic-1",
			AgentID: "synthetic-worker-1",
			Action:  "acquired",
			Key: ResourceLockKeyTrace{
				ResourceType:  "generic_resource",
				ResourceID:    "synthetic-resource",
				OperationKind: "read",
			},
		}},
		AgentFinalGate: &AgentFinalGateDecisionTrace{
			Action:        "require_wait",
			PendingAgents: []string{"synthetic-worker-2"},
			Reasons:       []string{"pending_worker_evidence"},
		},
		AgentNotifications: []AgentNotificationTrace{{
			AgentID:    "synthetic-worker-1",
			Status:     "completed",
			Summary:    "bounded summary",
			ResultRefs: []string{"artifact://synthetic/evidence"},
			Usage:      AgentUsageTrace{ToolCalls: 2},
		}},
		VerificationAgentReport: &VerificationAgentReportTrace{
			Status:        "PASS",
			Summary:       "independent verification passed",
			EvidenceRefs:  []string{"artifact://synthetic/verification"},
			Counterchecks: []string{"checked bounded evidence refs"},
		},
	}

	markdown := RenderMarkdown(trace)
	for _, want := range []string{
		"### Agent Scheduling",
		"agent listing loaded: synthetic.explorer",
		"delegation decision: spawn_new",
		"assignment lint: pass",
		"parallel agents requested",
		"resource lock acquired",
		"pending agent final gate: require_wait",
		"wait_agent notifications: completed",
		"verification agent: PASS",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
