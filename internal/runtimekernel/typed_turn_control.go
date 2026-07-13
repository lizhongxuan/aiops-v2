package runtimekernel

import (
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

type frozenTurnControl struct {
	Admission      runtimecontract.AdmissionFacts
	Capability     agentassembly.CapabilityPolicySnapshot
	TargetBound    bool
	ExecAllowed    bool
	MutationTool   bool
	MutationIntent bool
}

func frozenTurnMutationRequired(snapshot *TurnSnapshot, control frozenTurnControl) bool {
	if control.MutationIntent {
		return true
	}
	return snapshot != nil && snapshot.TaskDepth.RequiresApproval
}

func frozenTurnControlFromSnapshot(snapshot *TurnSnapshot) (frozenTurnControl, bool) {
	if snapshot == nil || snapshot.TurnAssembly == nil || snapshot.TurnAssembly.Validate() != nil {
		return frozenTurnControl{}, false
	}
	assembly := snapshot.TurnAssembly
	control := frozenTurnControl{
		Admission:  assembly.AdmissionFacts,
		Capability: assembly.CapabilityPolicy,
	}
	control.TargetBound = len(control.Admission.TargetRefs) > 0 || !control.Admission.SessionTarget.IsZero()
	control.MutationIntent = typedIntentMutates(control.Admission.Intent)
	for _, tool := range control.Capability.DispatchableTools {
		name := strings.ToLower(strings.TrimSpace(tool.Name))
		capability := strings.ToLower(strings.TrimSpace(tool.Capability))
		if name == "exec_command" || capability == resourcebinding.CapabilityExec || capability == resourcebinding.CapabilityMutate {
			control.ExecAllowed = true
		}
		if capability == resourcebinding.CapabilityMutate || tool.RequiresApproval {
			control.MutationTool = true
		}
	}
	return control, true
}

func typedIntentMutates(intent runtimecontract.IntentFrame) bool {
	if intent.Kind == runtimecontract.IntentKindChange {
		return true
	}
	for _, risk := range intent.RiskBudget {
		switch risk {
		case runtimecontract.ActionRiskWrite, runtimecontract.ActionRiskHostExec, runtimecontract.ActionRiskDestruct:
			return true
		}
	}
	for _, capability := range intent.Capabilities {
		for _, risk := range capability.Risks {
			switch risk {
			case runtimecontract.ActionRiskWrite, runtimecontract.ActionRiskHostExec, runtimecontract.ActionRiskDestruct:
				return true
			}
		}
	}
	return false
}
