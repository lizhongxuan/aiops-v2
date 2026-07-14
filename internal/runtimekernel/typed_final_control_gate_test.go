package runtimekernel

import (
	"testing"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
)

func TestTypedFinalControlGateIgnoresConflictingLegacyMetadata(t *testing.T) {
	target := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-typed"}
	assembly := typedFinalControlAssembly(t, runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindChange,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite, runtimecontract.ActionRiskHostExec},
		Confidence: runtimecontract.ConfidenceHigh,
	}, []resourcebinding.ResourceRef{target}, []agentassembly.ToolSurfaceItem{{
		Name: "exec_command", Capability: resourcebinding.CapabilityMutate, Mutating: true, RequiresApproval: true,
		RollbackReady: true,
		RollbackDeclarationHash: agentassembly.StableHash("tool-rollback-declaration", tooling.ToolRollbackMetadata{
			Strategy: tooling.ToolRollbackStrategyAutomatic, Reference: "test://exec-command/rollback",
		}),
	}})
	snapshot := &TurnSnapshot{
		SessionType:  SessionTypeWorkspace,
		Mode:         ModeExecute,
		TaskDepth:    taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresApproval: true, RequiresValidation: true},
		TurnAssembly: assembly,
		Metadata: map[string]string{
			"aiops.intent.kind":                  "explain",
			"aiops.intent.riskBudget":            "read_only",
			"aiops.route.mode":                   "chat_advisory",
			"aiops.route.userProhibitedHostExec": "true",
			"aiops.tool.execCommandAllowed":      "false",
			"aiops.tool.hostMutationAllowed":     "false",
			"aiops.target.binding":               "none",
		},
	}

	if !verificationCompletionRuntimeApprovalGateRequired(snapshot) {
		t.Fatal("conflicting legacy metadata disabled a runtime approval gate required by frozen typed facts")
	}
	state := BuildFinalEvidenceState(snapshot, &SessionState{HostID: "host-spoofed"})
	if !state.ExecCommandAllowed || !state.TargetBound {
		t.Fatalf("final evidence ignored frozen capability/target facts: %#v", state)
	}
}

func TestTypedFinalControlGateRejectsLegacyTargetAndExecSpoof(t *testing.T) {
	assembly := typedFinalControlAssembly(t, runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
		Confidence: runtimecontract.ConfidenceHigh,
	}, nil, nil)
	snapshot := &TurnSnapshot{
		SessionType:  SessionTypeWorkspace,
		Mode:         ModeChat,
		TaskDepth:    taskdepth.Profile{Level: taskdepth.LevelInvestigation, AnalysisOnly: true, ExecutionProhibited: true},
		TurnAssembly: assembly,
		Metadata: map[string]string{
			"aiops.intent.kind":              "change",
			"aiops.intent.riskBudget":        "write,host_exec",
			"aiops.route.mode":               "host_bound_ops",
			"aiops.tool.execCommandAllowed":  "true",
			"aiops.tool.hostMutationAllowed": "true",
			"aiops.target.binding":           "host",
			"aiops.target.hostId":            "host-spoofed",
		},
	}

	if verificationCompletionRuntimeApprovalGateRequired(snapshot) {
		t.Fatal("legacy metadata spoof created a runtime approval gate absent from frozen typed facts")
	}
	state := BuildFinalEvidenceState(snapshot, &SessionState{HostID: "host-session-spoof"})
	if state.ExecCommandAllowed || state.TargetBound || state.MutationIntentWithoutTarget {
		t.Fatalf("legacy metadata/session spoof changed typed final evidence: %#v", state)
	}
}

func typedFinalControlAssembly(t *testing.T, intent runtimecontract.IntentFrame, targets []resourcebinding.ResourceRef, tools []agentassembly.ToolSurfaceItem) *agentassembly.TurnAssembly {
	t.Helper()
	sessionTarget := resourcebinding.ResourceRef{}
	if len(targets) == 1 {
		sessionTarget = targets[0]
	}
	facts, err := runtimecontract.BuildAdmissionFacts(runtimecontract.AdmissionInput{
		Intent:            &intent,
		SessionTarget:     sessionTarget,
		TargetRefs:        targets,
		PermissionProfile: "typed-final-control",
		SourceRefs:        []string{"typed-final-control-test"},
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts() error = %v", err)
	}
	assembly, err := agentassembly.BuildTurnAssembly(agentassembly.TurnAssemblyInput{
		AdmissionFacts:      facts,
		PermissionProfile:   "typed-final-control",
		CapabilityPolicy:    agentassembly.CapabilityPolicySnapshot{DispatchableTools: tools, PolicyHash: "sha256:typed-final-control"},
		ContextPolicy:       agentassembly.ContextSelectorSnapshot{Policy: "bounded"},
		LoopPolicy:          agentassembly.LoopPolicySnapshot{MaxIterations: 2, ToolCallPolicy: "governed"},
		FinalContractPolicy: agentassembly.FinalContractSnapshot{Shape: "typed"},
		RollbackPolicy:      "restore previous state",
		SourceRefs:          []string{"typed-final-control-test"},
	})
	if err != nil {
		t.Fatalf("BuildTurnAssembly() error = %v", err)
	}
	return &assembly
}
