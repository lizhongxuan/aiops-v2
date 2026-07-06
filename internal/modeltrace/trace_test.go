package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/specialinputmemory"
	"aiops-v2/internal/tooling"
)

func TestWriteLocksJSONAndMarkdownTraceSchema(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:         "runtime_model_input",
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		Iteration:    1,
		VisibleTools: []string{"read_file"},
		Prompt: Prompt{
			StableHash: "stable-hash",
			Dynamic:    "dynamic delta",
		},
		ModelInput: modelrouter.ModelInputItemsFromEinoMessages([]*schema.Message{
			{
				Role:    schema.System,
				Content: "developer instructions",
				Extra: map[string]any{
					"semantic_role": "developer",
					"prompt_layer":  "developer",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if path == "" {
		t.Fatal("Write returned empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		ModelInput    []struct {
			ProviderRole string `json:"providerRole"`
			SemanticRole string `json:"semanticRole"`
			PromptLayer  string `json:"promptLayer"`
		} `json:"modelInput"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal json trace: %v", err)
	}
	if payload.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", payload.SchemaVersion)
	}
	if len(payload.ModelInput) != 1 {
		t.Fatalf("modelInput length = %d, want 1", len(payload.ModelInput))
	}
	msg := payload.ModelInput[0]
	if msg.ProviderRole != "system" || msg.SemanticRole != "developer" || msg.PromptLayer != "developer" {
		t.Fatalf("trace message roles = %#v, want provider=system semantic=developer layer=developer", msg)
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "- Schema: `1`") {
		t.Fatalf("markdown trace missing schema version:\n%s", string(markdown))
	}
}

func TestWriteOmitsFullStablePromptAfterInitialIteration(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-delta",
		TurnID:    "turn-delta",
		Iteration: 1,
		Prompt: Prompt{
			StableHash: "stable-hash",
			Stable:     "large stable prompt body",
			System:     "system role body",
			Developer:  "developer rules body",
			Tools:      "tool registry body",
			Policy:     "runtime policy body",
			Dynamic:    "dynamic delta body",
		},
		ModelInput: modelrouter.ModelInputItemsFromEinoMessages([]*schema.Message{{
			Role:    schema.System,
			Content: "tool registry body",
			Extra: map[string]any{
				"semantic_role": "tool",
				"prompt_layer":  "tool_index",
			},
		}, {
			Role:    schema.User,
			Content: "new user message stays visible",
		}}),
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	var got payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.Prompt.StableHash != "stable-hash" || got.Prompt.Dynamic != "dynamic delta body" {
		t.Fatalf("prompt delta = %#v, want stable hash and dynamic delta", got.Prompt)
	}
	for _, forbidden := range []string{
		got.Prompt.Stable,
		got.Prompt.System,
		got.Prompt.Developer,
		got.Prompt.Tools,
		got.Prompt.Policy,
	} {
		if forbidden != "" {
			t.Fatalf("subsequent trace retained full stable prompt: %#v", got.Prompt)
		}
	}
	if strings.Contains(string(data), "large stable prompt body") || strings.Contains(string(data), "tool registry body") {
		t.Fatalf("trace JSON retained full stable prompt:\n%s", string(data))
	}
	if !strings.Contains(string(data), "new user message stays visible") || !strings.Contains(string(data), "omitted after initial trace") {
		t.Fatalf("trace JSON should keep new messages and replace prompt-layer content:\n%s", string(data))
	}
}

func TestWriteDisabledReturnsEmptyPath(t *testing.T) {
	path, err := WriteWithConfig(Config{Enabled: false, RootDir: t.TempDir()}, Request{Kind: "runtime_model_input"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if path != "" {
		t.Fatalf("Write() path = %q, want empty when disabled", path)
	}
}

func TestWriteIncludesPromptInputTraceAndDiffMarkdown(t *testing.T) {
	dir := t.TempDir()

	diff := promptinput.DiffTrace(
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{Source: "conversation", SemanticRole: "user", Content: "old"}}},
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{Source: "conversation", SemanticRole: "tool_result", ID: "call-1", Content: "token=secret-value"}}},
	)
	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime model input",
		TraceID:   "trace/with spaces",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		PromptInputTrace: promptinput.PromptInputTrace{Items: []promptinput.TraceItem{
			{Source: "memory", SemanticRole: "memory", ID: "mem-1", Content: "prior note"},
		}},
		PromptInputDiff: &diff,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	if !strings.Contains(string(data), "promptInputTrace") || !strings.Contains(string(data), "mem-1") {
		t.Fatalf("json trace missing prompt input trace:\n%s", string(data))
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "## Prompt Input Trace") || !strings.Contains(string(markdown), "memory") {
		t.Fatalf("markdown trace missing prompt input trace:\n%s", string(markdown))
	}

	diffMarkdown, err := os.ReadFile(filepath.Join(filepath.Dir(path), "input.diff.md"))
	if err != nil {
		t.Fatalf("read input.diff.md: %v", err)
	}
	if !strings.Contains(string(diffMarkdown), "tool_result") || strings.Contains(string(diffMarkdown), "secret-value") {
		t.Fatalf("diff markdown missing semantic delta or leaked secret:\n%s", string(diffMarkdown))
	}
}

func TestWriteIncludesSpecialInputWorldStateInPromptTrace(t *testing.T) {
	dir := t.TempDir()
	worldState := &specialinputmemory.SpecialInputWorldStateSection{
		SchemaVersion: specialinputmemory.SchemaVersion,
		ActiveExecutionScope: &specialinputmemory.ExecutionScopeGrantTrace{
			ID:           "grant-host-a",
			ResourceKind: specialinputmemory.ResourceKindHost,
			ResourceID:   "host-a",
		},
		ReadPlan: &specialinputmemory.MemoryReadPlanTrace{
			ActiveGrantID:      "grant-host-a",
			ActiveResourceKind: specialinputmemory.ResourceKindHost,
			ActiveResourceID:   "host-a",
		},
	}

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:                   "runtime_model_input",
		SessionID:              "sess-special",
		TurnID:                 "turn-special",
		SpecialInputWorldState: worldState,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	if !strings.Contains(string(data), `"specialInputWorldState"`) || !strings.Contains(string(data), "grant-host-a") {
		t.Fatalf("json trace missing special input world state:\n%s", string(data))
	}
	var raw struct {
		SpecialInputWorldState *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState"`
		PromptInputTrace       struct {
			SpecialInputWorldState *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState"`
		} `json:"promptInputTrace"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal json trace: %v", err)
	}
	if raw.SpecialInputWorldState == nil || raw.SpecialInputWorldState.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("top-level specialInputWorldState = %#v, want host-a", raw.SpecialInputWorldState)
	}
	if raw.PromptInputTrace.SpecialInputWorldState == nil || raw.PromptInputTrace.SpecialInputWorldState.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("promptInputTrace.specialInputWorldState = %#v, want host-a", raw.PromptInputTrace.SpecialInputWorldState)
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "## Special Input Memory") || !strings.Contains(string(markdown), "grant-host-a") {
		t.Fatalf("markdown missing special input world state:\n%s", string(markdown))
	}
}

func TestWriteIncludesAssemblyBoundaryTraceFields(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-boundary",
		TurnID:    "turn-boundary",
		PromptInputTrace: promptinput.PromptInputTrace{
			AssemblySource:         "runtimekernel.buildModelInputTraceRequest",
			PromptCompilerSource:   "promptcompiler.Compiler",
			ToolSurfaceSource:      "runtimekernel.applyToolSurfacePolicyToCompileContext",
			AdapterName:            "eino",
			ToolSurfaceFingerprint: "surface-boundary",
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	for _, want := range []string{
		`"assembly_source": "runtimekernel.buildModelInputTraceRequest"`,
		`"prompt_compiler_source": "promptcompiler.Compiler"`,
		`"tool_surface_source": "runtimekernel.applyToolSurfacePolicyToCompileContext"`,
		`"adapter_name": "eino"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json trace missing %q:\n%s", want, string(data))
		}
	}
}

func TestWriteIncludesPromptInputTraceBudgetMetrics(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-ops",
		TurnID:    "turn-ops",
		PromptInputTrace: promptinput.PromptInputTrace{
			OpsContextCapsuleChars: 512,
			SessionFactCount:       5,
			LettaHintCount:         2,
			MemoryItemCount:        3,
			VisibleOpsManualTools:  []string{"search_ops_manuals"},
			DroppedContextReasons:  []string{"letta_hint_limit"},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	for _, want := range []string{`"opsContextCapsuleChars": 512`, `"sessionFactCount": 5`, `"visibleOpsManualTools"`, `"letta_hint_limit"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json trace missing %q:\n%s", want, string(data))
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "ops_context_capsule_chars") || !strings.Contains(string(markdown), "letta_hint_limit") {
		t.Fatalf("markdown trace missing budget metrics:\n%s", string(markdown))
	}
}

func TestModelTraceIncludesPlanModeTaskTransitionsAndCompletionGate(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-plan",
		TurnID:    "turn-plan",
		PromptInputTrace: promptinput.PromptInputTrace{
			PlanModeState: &promptinput.PlanModeTraceState{
				State:          "active",
				PlanID:         "plan-synthetic-1",
				ApprovalStatus: "pending_exit_approval",
				ReminderLevel:  "sparse",
			},
			PlanArtifactRef: "artifact://plans/plan-synthetic-1",
			PlanTransitions: []promptinput.PlanTransitionTrace{{
				From: "inactive",
				To:   "active",
			}},
			PlanRequirementDecision: &promptinput.PlanRequirementDecisionTrace{
				Required: true,
				Reason:   "multi_step",
			},
			PlanCompletionGate: &promptinput.PlanCompletionGateTrace{
				Decision: "block",
				Reasons:  []string{"pending_evidence"},
			},
			TaskClaims: []promptinput.TaskClaimTrace{{
				TaskID: "step-2",
				Owner:  "agent:planner",
				Status: "claimed",
			}},
			PlanApprovalScope: &promptinput.PlanApprovalScopeTrace{
				PlanID:         "plan-synthetic-1",
				ApprovedScopes: []string{"internal/promptcompiler"},
			},
			PlanRejectionEvents: []promptinput.PlanRejectionEventTrace{{
				PlanID: "plan-synthetic-1",
				Reason: "scope too broad",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	for _, want := range []string{
		`"planModeState"`,
		`"planArtifactRef": "artifact://plans/plan-synthetic-1"`,
		`"planTransitions"`,
		`"planRequirementDecision"`,
		`"planCompletionGate"`,
		`"taskClaims"`,
		`"planApprovalScope"`,
		`"planRejectionEvents"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json trace missing %q:\n%s", want, string(data))
		}
	}
}

func TestModelTraceRedactsVerificationSafetyTraceFields(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-verification",
		TurnID:    "turn-verification",
		PromptInputTrace: promptinput.PromptInputTrace{
			VerificationReportRef: "artifact://synthetic/verification?api_key=synthetic-secret",
			VerificationStatus:    "FAIL",
			CompletionGate: &promptinput.CompletionGateTrace{
				Decision: "block_success_final",
				Reasons:  []string{"fail_requires_expected_actual"},
			},
			SafetySignals: []promptinput.SafetySignalTrace{{
				Category: "destructive_workaround",
				Severity: "critical",
				Action:   "require_approval",
				Reasons:  []string{"force overwrite included secret=synthetic-secret"},
			}},
			UnexpectedStateGate: &promptinput.UnexpectedStateGateTrace{
				Action:         "block_mutation",
				Sources:        []string{"synthetic_tool_result"},
				AffectedScopes: []string{"synthetic.scope?api_key=synthetic-secret"},
				BlockedAction:  "overwrite",
				Reasons:        []string{"unexpected_state"},
			},
			ApprovalScope: &promptinput.ApprovalScopeTrace{
				Status:         "denied",
				AllowedActions: []string{"inspect"},
				ResourceScopes: []string{"synthetic.scope?api_key=synthetic-secret"},
				RiskCeiling:    "medium",
				InputHash:      "sha256:synthetic-input",
				Reasons:        []string{"scope_mismatch"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{
		`"verificationReportRef"`,
		`"verificationStatus": "FAIL"`,
		`"completionGate"`,
		`"safetySignals"`,
		`"unexpectedStateGate"`,
		`"approvalScope"`,
		`[REDACTED]`,
	} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}
	if strings.Contains(jsonTrace, "synthetic-secret") {
		t.Fatalf("json trace leaked secret:\n%s", jsonTrace)
	}
}

func TestModelTraceIncludesRedactedToolSurfaceTrace(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:         "runtime_model_input",
		SessionID:    "sess-tool-surface",
		TurnID:       "turn-tool-surface",
		VisibleTools: []string{"exec_command", "tool_search", "generic.metrics.read"},
		PromptInputTrace: promptinput.PromptInputTrace{
			DeferredToolDirectory: []promptcompiler.DeferredToolDirectoryEntry{{
				Pack:              "external_metrics",
				Capability:        "metrics",
				Source:            "mcp",
				MCPServerID:       "observability",
				HealthStatus:      "unavailable",
				RequiresHealth:    true,
				RequiresSelect:    true,
				UnavailableReason: "connect https://user:secret-pass@metrics.example.internal/api failed token=tool-secret",
				ToolCount:         4,
			}},
			LoadedToolsDelta: []string{"generic.metrics.read"},
			LoadedPacksDelta: []string{"generic_metrics"},
			ToolSearchEvents: []promptinput.ToolSearchTraceEvent{{
				Mode:       "search",
				Query:      "metrics password=query-secret",
				MatchCount: 1,
				Matches:    []string{"generic.metrics.read"},
				Reason:     "need host resource facts",
			}},
			ToolSelectionEvents: []promptinput.ToolSelectionTraceEvent{{
				Source:      "tool_search.select",
				Reason:      "selected direct read tool",
				LoadedTools: []string{"generic.metrics.read"},
				LoadedPacks: []string{"generic_metrics"},
				NotLoaded:   []string{"external.metrics.read"},
				NotLoadedReasons: map[string]string{
					"external.metrics.read": "mcp_unavailable token=filtered-secret",
				},
			}},
			RejectedToolCalls: []promptinput.RejectedToolCallTraceEvent{{
				ToolName:       "external.metrics.read",
				ErrorType:      "mcp_unavailable",
				Reason:         "server unavailable api_key=rejected-secret",
				RequiredAction: "use direct evidence",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{
		`"toolSurfaceTrace"`,
		`"initialTools"`,
		`"baseRegistryCount": 2`,
		`"deferredFamilies"`,
		`"mcpHealth"`,
		`"loadedTools"`,
		`"loadedPacks"`,
		`"filteredTools"`,
		`"selectedTools"`,
		`"rejectedToolReasons"`,
		`metrics.example.internal`,
		`[REDACTED]`,
	} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}
	for _, forbidden := range []string{"secret-pass", "tool-secret", "query-secret", "filtered-secret", "rejected-secret"} {
		if strings.Contains(jsonTrace, forbidden) {
			t.Fatalf("json trace leaked %q:\n%s", forbidden, jsonTrace)
		}
	}
}

func TestModelTraceRedactsGenericityTraceDepthAndCoverage(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-genericity",
		TurnID:    "turn-genericity",
		PromptInputTrace: promptinput.PromptInputTrace{
			TaskDepth: &promptinput.TaskDepthTrace{
				Level:              "investigation",
				Reasons:            []string{"cross_resource_evidence"},
				RequiresPlan:       true,
				RequiresEvidence:   true,
				RequiresValidation: true,
			},
			EvidenceCoverage: &promptinput.EvidenceCoverageTrace{
				Action:             "continue_gathering",
				MissingDimensions:  []string{"verification"},
				VerificationStatus: "PARTIAL",
				Reasons:            []string{"token=synthetic-secret"},
			},
			GenericityTrace: &promptinput.GenericityTrace{
				AllowedFixtureTerms: []string{"synthetic_resource_a"},
				AllowedPluginTerms:  []string{"synthetic_component"},
				ResourceIDSource:    "fixture",
				Violations:          []string{"password=synthetic-secret"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{`"taskDepth"`, `"requiresPlan": true`, `"evidenceCoverage"`, `"genericityTrace"`, `"resourceIdSource": "fixture"`, `[REDACTED]`} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}
	if strings.Contains(jsonTrace, "synthetic-secret") {
		t.Fatalf("json trace leaked secret:\n%s", jsonTrace)
	}

	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"### Task Depth", "task_depth: investigation", "### Evidence Coverage", "coverage_action: continue_gathering", "### Genericity Trace"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "synthetic-secret") {
		t.Fatalf("markdown trace leaked secret:\n%s", md)
	}
}

func TestWriteIncludesFinalEvidenceState(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-final-evidence",
		TurnID:    "turn-final-evidence",
		FinalEvidenceState: map[string]any{
			"confidence": "low",
			"failedTools": []map[string]any{{
				"toolName":     "synthetic.read",
				"failureClass": "timeout",
				"impact":       "required evidence is missing",
			}},
			"notChecked": []map[string]any{{
				"toolName": "synthetic.deferred.read",
				"reason":   "tool_unloaded",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"finalEvidenceState"`, `"confidence": "low"`, `"failedTools"`, `"notChecked"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "## Final Evidence State") || !strings.Contains(string(markdown), "synthetic.read") {
		t.Fatalf("markdown trace missing final evidence state:\n%s", string(markdown))
	}
}

func TestWriteIncludesContextGovernanceTrace(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-governance",
		TurnID:    "turn-governance",
		PromptInputTrace: promptinput.PromptInputTrace{
			ContextGovernance: []promptinput.ContextGovernanceTraceItem{{
				Layer:               "L4",
				Kind:                "context.compaction.started",
				Message:             "compacting token=plain-token",
				ToolCallID:          "call-logs-1",
				ToolName:            "logs.search",
				MaterializationTier: "large",
				OriginalBytes:       49152,
				InlineBytes:         512,
				Budget:              map[string]int{"autoCompactThreshold": 167000, "blockingLimit": 177000},
				ReferenceIDs:        []string{"ref-1", "artifact-token=plain-token"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"contextGovernance"`, `"layer": "L4"`, `"kind": "context.compaction.started"`, `"toolCallId": "call-logs-1"`, `"toolName": "logs.search"`, `"materializationTier": "large"`, `"originalBytes": 49152`, `"inlineBytes": 512`, `"autoCompactThreshold": 167000`, `"referenceIds"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	if strings.Contains(jsonText, "plain-token") {
		t.Fatalf("json trace leaked secret:\n%s", jsonText)
	}

	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"## Context Governance", "### Budget", "autoCompactThreshold=`167000`", "### External References", "ref-1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "1/3") {
		t.Fatalf("markdown trace should not include retry progress:\n%s", md)
	}
	if strings.Contains(md, "plain-token") {
		t.Fatalf("markdown trace leaked secret:\n%s", md)
	}
}

func TestWriteIncludesResourceDedupeRange(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-resource",
		TurnID:    "turn-resource",
		PromptInputTrace: promptinput.PromptInputTrace{
			ContextGovernance: []promptinput.ContextGovernanceTraceItem{{
				Layer:        "L2",
				Kind:         "resource.dedupe.hit",
				ReferenceIDs: []string{"ref-1"},
				Resource: &promptinput.ResourceTraceItem{
					URI:         "resource://generic",
					Digest:      "sha256:same",
					ContentType: "text/plain",
					Bytes:       128,
					Range: promptinput.ResourceRange{
						Offset: 6,
						Limit:  4,
						Format: "text",
					},
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"resource"`, `"uri": "resource://generic"`, `"offset": 6`, `"limit": 4`, `"kind": "resource.dedupe.hit"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	if strings.Contains(jsonText, "full content should not repeat") {
		t.Fatalf("json trace leaked resource content:\n%s", jsonText)
	}

	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"resource.dedupe.hit", "resource://generic", "offset=6", "limit=4"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "full content should not repeat") {
		t.Fatalf("markdown trace leaked resource content:\n%s", md)
	}
}

func TestWriteIncludesResourceBindingTrace(t *testing.T) {
	dir := t.TempDir()
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a", DisplayName: "db-a"}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-resource-binding",
		TurnID:    "turn-resource-binding",
		PromptInputTrace: promptinput.PromptInputTrace{
			ResourceBindings: []resourcebinding.ResourceBindingSnapshot{binding},
			ResourceRoleBindings: []resourcebinding.ResourceRoleBinding{resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
				ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeDatabase, ID: "pg-a"},
				Role:         "primary",
				RoleAlias:    []string{"primary", "主节点"},
				SourceTurnID: "turn-resource-binding",
			})},
			ResourceCapabilities: []resourcebinding.ResourceCapability{
				resourcebinding.NewResourceCapability(binding, resourcebinding.CapabilityExec, []string{"host.exec"}, resourcebinding.CapabilityOptions{}),
			},
			ResourceEvidenceRefs: []resourcebinding.EvidenceRef{resourcebinding.BuildEvidenceRef(resourcebinding.EvidenceInput{
				ID:          "ev-1",
				ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
				Source:      resourcebinding.EvidenceSourceTool,
				Kind:        resourcebinding.EvidenceKindCommandOutput,
			})},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{
		`"resourceBindings"`,
		`"trustLevel": "verified"`,
		`"resourceCapabilities"`,
		`"host.exec"`,
		`"resourceEvidenceRefs"`,
		`"command_output"`,
		`"resourceRoleBindings"`,
		`"primary"`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}

	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"## Resource Binding Trace", "### Bindings", "### Capabilities", "### Evidence", "host.exec"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
}

func TestWriteIncludesSessionTargetAndRoleConflictTrace(t *testing.T) {
	dir := t.TempDir()
	target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:          []string{"host-a", "host-b"},
		SourceTurnID:     "turn-1",
		SourceMentionIDs: []string{"m1", "m2"},
	})
	conflict := resourcebinding.DetectRoleBindingConflicts([]resourcebinding.ResourceRoleBinding{
		resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}, Role: resourcebinding.RolePGPrimary}),
		resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"}, Role: resourcebinding.RolePGPrimary}),
	})

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-target",
		TurnID:    "turn-target",
		PromptInputTrace: promptinput.PromptInputTrace{
			SessionTargetSnapshot: target,
			RoleBindingConflicts:  conflict,
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"sessionTargetSnapshot"`, `"bindingMode": "multi_host"`, `"roleBindingConflicts"`, `"unique_role_bound_to_multiple_resources"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	for _, want := range []string{"### Session Target", "### Role Conflicts", "multi_host"} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, string(markdown))
		}
	}
}

func TestWriteIncludesAgentAssemblySnapshot(t *testing.T) {
	dir := t.TempDir()
	snapshot := agentassembly.Build(agentassembly.BuildInput{
		AgentKind:         "worker",
		Profile:           "host_worker",
		RuntimeRole:       "host.execute",
		RouteReason:       []string{"aiops.route.mode=host_bound_ops"},
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "host.exec", Description: "execute host command"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "host.exec", Description: "execute host command"}},
	})

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-assembly",
		TurnID:    "turn-assembly",
		PromptInputTrace: promptinput.PromptInputTrace{
			AgentAssemblySnapshot: &snapshot,
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"agentAssemblySnapshot"`, `"agentKind": "worker"`, `"profile": "host_worker"`, `"toolSurface"`, `"host.exec"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "## Agent Assembly Snapshot") || !strings.Contains(string(markdown), "host_worker") {
		t.Fatalf("markdown trace missing assembly snapshot:\n%s", string(markdown))
	}
}

func TestWriteIncludesPromptSectionTraceAndContextUsage(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-sections",
		TurnID:    "turn-sections",
		PromptInputTrace: promptinput.PromptInputTrace{
			PromptSections: []promptcompiler.PromptSectionTrace{{
				ID:             "protocol.state",
				Kind:           "dynamic",
				Source:         "protocol-state",
				Hash:           "sha256:abc",
				Bytes:          32,
				TokensEstimate: 8,
			}},
			ChangedSections: []promptcompiler.ChangedPromptSection{{
				ID:          "protocol.state",
				Reason:      promptcompiler.PromptSectionChangeProtocolStateChanged,
				CurrentHash: "sha256:abc",
			}},
			ContextUsage: promptinput.ContextUsage{
				MaxContextTokens:     1000,
				ReservedOutputTokens: 200,
				EstimatedInputTokens: 20,
				Categories: []promptinput.ContextUsageCategory{{
					Name:           "tool_results",
					Bytes:          800,
					TokensEstimate: 200,
				}},
				TopContributors: []promptinput.ContextContributor{{
					Kind:           "tool_results",
					ID:             "call-1",
					TokensEstimate: 200,
					Bytes:          800,
					Action:         "keep_inline",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"promptSections"`, `"changedSections"`, `"contextUsage"`, `"protocol.state"`, `"tool_results"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"### Prompt Sections", "### Changed Sections", "### Context Usage", "tool_results", "call-1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
}

func TestWriteIncludesPromptFingerprintSummary(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Metadata:  map[string]string{"eval.caseId": "case-1"},
		PromptFingerprint: map[string]string{
			"version":       "prompt-fingerprint-v1",
			"stableHash":    "stable-hash",
			"developerHash": "developer-hash",
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	if !strings.Contains(string(data), `"promptFingerprint"`) || !strings.Contains(string(data), `"developerHash": "developer-hash"`) || !strings.Contains(string(data), `"caseId": "case-1"`) {
		t.Fatalf("json trace missing prompt fingerprint:\n%s", string(data))
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "- Prompt fingerprint: `stable-hash`") || !strings.Contains(string(markdown), "- Eval case: `case-1`") {
		t.Fatalf("markdown trace missing prompt fingerprint summary:\n%s", string(markdown))
	}
}

func TestWriteIncludesDiagnosticTraceAndRedactsSecrets(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		DiagnosticTrace: diagnostics.DiagnosticTrace{
			ScopeHash:        "scope-redis",
			ScopeSummary:     "redis redis://:secret@127.0.0.1:6379/0",
			Hypotheses:       []string{"redis unavailable"},
			ObservedEvidence: []string{"PING timeout"},
			RefutingEvidence: []string{"container is running"},
			MissingEvidence:  []string{"need api key sk-test-value"},
			ToolFailures: []diagnostics.ToolFailure{{
				ToolName: "exec_command",
				Semantic: diagnostics.ToolFailurePolicyBlocked,
				Detail:   "policy blocked token=plain-token",
				Critical: true,
			}},
			ManualBindingID:  "manual-redis",
			Confidence:       diagnostics.ConfidenceLow,
			ConfidenceReason: "sensitive value was present in failed probe",
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"diagnosticTrace"`, `"scopeHash": "scope-redis"`, `"manualBindingId": "manual-redis"`, `"semantic": "policy_blocked"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	for _, forbidden := range []string{"secret", "sk-test-value", "plain-token"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("json trace leaked %q:\n%s", forbidden, jsonText)
		}
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"## Diagnostic Trace", "scope-redis", "redis unavailable", "PING timeout", "policy_blocked", "low"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
	for _, forbidden := range []string{"secret", "sk-test-value", "plain-token"} {
		if strings.Contains(md, forbidden) {
			t.Fatalf("markdown trace leaked %q:\n%s", forbidden, md)
		}
	}
}

func TestWriteRedactsSecretsFromPromptModelInputAndToolCalls(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Prompt: Prompt{
			Dynamic: "## Runtime Environment Context\nCurrentFocus: target=redis dsn=redis://:secret-pass@127.0.0.1:6379/0",
		},
		ModelInput: modelrouter.ModelInputItemsFromEinoMessages([]*schema.Message{
			{Role: schema.User, Content: "连接串 redis://:secret-pass@127.0.0.1:6379/0 帮我排查"},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "host_exec",
					Arguments: `{"cmd":"redis-cli -a secret-pass PING","token":"plain-token"}`,
				},
			}}},
		}),
		PromptInputTrace: promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{
			Source:       "conversation",
			SemanticRole: "user",
			Content:      "redis password=secret-pass",
		}}},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	for _, filePath := range []string{path, strings.TrimSuffix(path, filepath.Ext(path)) + ".md"} {
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s: %v", filePath, err)
		}
		text := string(data)
		for _, forbidden := range []string{"secret-pass", "plain-token"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked %q:\n%s", filePath, forbidden, text)
			}
		}
		if !strings.Contains(text, "[REDACTED]") {
			t.Fatalf("%s missing redaction marker:\n%s", filePath, text)
		}
	}
}
