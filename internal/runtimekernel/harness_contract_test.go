package runtimekernel

import (
	"testing"
	"time"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
)

func TestBuildHarnessTurnTraceIncludesRouteTargetToolSurfaceAndContext(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	snapshot := &TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "sess-1",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Metadata: map[string]string{
			"aiops.route.mode":     "host_bound_ops",
			"profile":              "host_worker",
			"aiops.target.binding": "host",
		},
		ToolSurfaceSnapshot: &ToolSurfaceSnapshotRef{
			Fingerprint:        "sha256:test",
			ToolNames:          []string{"exec_command"},
			PolicySnapshotHash: "sha256:policy",
			CreatedAt:          now,
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	step := RuntimeStepContext{
		Turn: RuntimeTurnContext{
			SessionID:   "sess-1",
			TurnID:      "turn-1",
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			Route:       RuntimeRouteSnapshot{Route: "host_bound_ops", HostID: "host-a", Profile: "host_worker"},
			Profile:     "host_worker",
			HostID:      "host-a",
			Model:       modelrouter.ModelCapabilities{Provider: "zhipu", Model: "glm-5.1"},
			Metadata: map[string]string{
				"aiops.route.mode":     "host_bound_ops",
				"aiops.target.binding": "host",
			},
		},
		Iteration: 0,
		Compiled: promptcompiler.CompiledPrompt{
			Fingerprint: promptcompiler.PromptFingerprint{StableHash: "sha256:prompt"},
		},
		ToolSurface: RuntimeToolRouterSnapshot{
			ModelVisibleTools: []string{"exec_command"},
			DispatchableTools: []string{"exec_command"},
			Fingerprint:       "sha256:test",
			PolicyHash:        "sha256:policy",
		},
	}
	final := FinalEvidenceVerification{
		Action:     FinalEvidenceActionAllow,
		Confidence: FinalEvidenceConfidenceLow,
		State: FinalEvidenceState{
			TargetBound:        true,
			ExecCommandAllowed: true,
		},
	}

	trace := BuildHarnessTurnTrace(snapshot, step, final)
	if err := trace.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if trace.SchemaVersion != "aiops.harness.turn.v1" {
		t.Fatalf("schemaVersion = %q", trace.SchemaVersion)
	}
	if trace.Route.Mode != "host_bound_ops" || trace.Route.Profile != "host_worker" {
		t.Fatalf("route = %#v", trace.Route)
	}
	if trace.Target.Binding != "host" || !trace.Target.Verified || len(trace.Target.Refs) != 1 || trace.Target.Refs[0] != "host:host-a" {
		t.Fatalf("target = %#v", trace.Target)
	}
	if trace.ToolSurface.Fingerprint != "sha256:test" {
		t.Fatalf("tool surface fingerprint = %q", trace.ToolSurface.Fingerprint)
	}
	if len(trace.ToolSurface.Visible) != 1 || trace.ToolSurface.Visible[0] != "exec_command" {
		t.Fatalf("visible tools = %#v", trace.ToolSurface.Visible)
	}
	if len(trace.ToolSurface.Dispatchable) != 1 || trace.ToolSurface.Dispatchable[0] != "exec_command" {
		t.Fatalf("dispatchable tools = %#v", trace.ToolSurface.Dispatchable)
	}
	if trace.Context.PromptHash != "sha256:prompt" {
		t.Fatalf("prompt hash = %q", trace.Context.PromptHash)
	}
	if trace.Final.Status != "unknown" || trace.Final.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("final = %#v", trace.Final)
	}
}

func TestBuildHarnessTurnTraceIncludesFinalVerification(t *testing.T) {
	trace := BuildHarnessTurnTrace(&TurnSnapshot{
		ID:          "turn-final",
		SessionID:   "sess-final",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleCompleted,
		ResumeState: TurnResumeStateNone,
		Metadata:    map[string]string{"aiops.route.mode": "chat_advisory"},
	}, RuntimeStepContext{
		Turn: RuntimeTurnContext{
			SessionID:   "sess-final",
			TurnID:      "turn-final",
			SessionType: SessionTypeWorkspace,
			Mode:        ModeChat,
			Route:       RuntimeRouteSnapshot{Route: "chat_advisory", Profile: "advisor"},
			Profile:     "advisor",
		},
	}, FinalEvidenceVerification{
		Action:     FinalEvidenceActionDowngrade,
		Confidence: FinalEvidenceConfidenceLow,
		Reasons:    []string{"failed_tool_requires_lower_confidence"},
		State: FinalEvidenceState{
			FailedTools: []FailedToolImpact{{ToolName: "exec_command", FailureClass: "needs_host_agent"}},
		},
	})

	if trace.Final.Status != "partial" {
		t.Fatalf("final status = %q, want partial", trace.Final.Status)
	}
	if trace.Final.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("final confidence = %q", trace.Final.Confidence)
	}
	if len(trace.Final.FailedToolImpacts) != 1 || trace.Final.FailedToolImpacts[0].FailureClass != "needs_host_agent" {
		t.Fatalf("failed tool impacts = %#v", trace.Final.FailedToolImpacts)
	}
	if len(trace.Final.Limitations) == 0 {
		t.Fatalf("limitations should include verification reasons")
	}
}

func TestHarnessTurnTraceValidateRejectsMissingSessionTurnOrSchema(t *testing.T) {
	valid := BuildHarnessTurnTrace(&TurnSnapshot{
		ID:          "turn-valid",
		SessionID:   "sess-valid",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
	}, RuntimeStepContext{}, FinalEvidenceVerification{})
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid trace failed validation: %v", err)
	}

	for name, mutate := range map[string]func(*HarnessTurnTrace){
		"schema":  func(t *HarnessTurnTrace) { t.SchemaVersion = "" },
		"session": func(t *HarnessTurnTrace) { t.SessionID = "" },
		"turn":    func(t *HarnessTurnTrace) { t.TurnID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			next := valid
			mutate(&next)
			if err := next.Validate(); err == nil {
				t.Fatalf("Validate() error = nil")
			}
		})
	}
}

func TestBuildHarnessTurnTraceUsesSessionTargetSnapshotRefs(t *testing.T) {
	target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:           []string{"host-kme-b2c1b82d"},
		SourceTurnID:      "turn-target",
		SourceMentionIDs:  []string{"mention-1"},
		ExpiresAfterTurns: 6,
		Confidence:        1,
	})
	trace := BuildHarnessTurnTrace(&TurnSnapshot{
		ID:          "turn-target",
		SessionID:   "sess-target",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
	}, RuntimeStepContext{
		Turn: RuntimeTurnContext{SessionID: "sess-target", TurnID: "turn-target", HostID: "host-kme-b2c1b82d"},
	}, FinalEvidenceVerification{
		State: FinalEvidenceState{TargetBound: true},
	})
	trace.Target = HarnessTargetTraceFromSessionTarget(target, trace.Target)
	if trace.Target.Binding != "host" || len(trace.Target.Refs) != 1 || trace.Target.Refs[0] != "host:host-kme-b2c1b82d" {
		t.Fatalf("target = %#v", trace.Target)
	}
}
