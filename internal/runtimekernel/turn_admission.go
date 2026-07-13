package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func persistSessionTargetRequestState(session *SessionState, req TurnRequest) {
	if session == nil {
		return
	}
	if req.SessionTargetSnapshot != nil {
		session.SessionTargetSnapshot = req.SessionTargetSnapshot
		session.ResourceRoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), req.ResourceRoleBindings...)
		session.RoleBindingConflicts = append([]resourcebinding.RoleBindingConflict(nil), req.RoleBindingConflicts...)
	} else {
		if len(req.ResourceRoleBindings) > 0 {
			session.ResourceRoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), req.ResourceRoleBindings...)
		}
		if len(req.RoleBindingConflicts) > 0 {
			session.RoleBindingConflicts = append([]resourcebinding.RoleBindingConflict(nil), req.RoleBindingConflicts...)
		}
	}

	session.HostID = strings.TrimSpace(req.HostID)
	if snapshot := session.SessionTargetSnapshot; snapshot != nil && !snapshot.Expired() && !snapshot.RequiresConfirmation && snapshot.Confidence > 0 {
		hostIDs := resourcebinding.HostIDsFromSessionTarget(snapshot)
		if len(hostIDs) == 1 {
			session.HostID = hostIDs[0]
		} else {
			session.HostID = ""
		}
	}
}

type runtimeTurnAssemblyInput struct {
	TurnContext            RuntimeTurnContext
	CompileContext         promptcompiler.CompileContext
	ToolSurfacePolicy      tooling.ToolSurfacePolicySnapshot
	ToolSurfaceFingerprint string
	ResourceBindings       []resourcebinding.ResourceBindingSnapshot
	RollbackPolicy         string
	Mode                   Mode
	MaxIterations          int
}

func buildRuntimeTurnAssembly(input runtimeTurnAssemblyInput) (*agentassembly.TurnAssembly, error) {
	modelVisible := toolMetadataFromPromptTools(input.CompileContext.AssembledTools)
	capabilityPolicy := agentassembly.BuildToolSurfaceSnapshot(agentassembly.ToolSurfaceInput{
		ResourceBindings:  input.ResourceBindings,
		RegisteredTools:   modelVisible,
		ModelVisibleTools: modelVisible,
		DispatchableTools: modelVisible,
		HiddenTools:       hiddenToolInputsFromPolicy(input.ToolSurfacePolicy),
		PolicyHash:        input.ToolSurfacePolicy.Hash,
		Fingerprint:       input.ToolSurfaceFingerprint,
	})
	sourceRefs := []string{"runtimekernel:pre_prompt"}
	if hash := strings.TrimSpace(input.ToolSurfacePolicy.Hash); hash != "" {
		sourceRefs = append(sourceRefs, "tool-surface-policy:"+hash)
	}
	assembly, err := agentassembly.BuildTurnAssembly(agentassembly.TurnAssemblyInput{
		AdmissionFacts: input.TurnContext.AdmissionFacts,
		AdmissionError: input.TurnContext.AdmissionError,
		PermissionProfile: firstNonBlankRuntimeString(
			input.TurnContext.AdmissionFacts.PermissionProfile,
			input.ToolSurfacePolicy.PermissionHash,
			input.ToolSurfacePolicy.ApprovalPolicy,
		),
		CapabilityPolicy: capabilityPolicy,
		ContextPolicy: agentassembly.ContextSelectorSnapshot{
			Lifecycle: agentassembly.LifecycleRequestScope,
			Policy: firstNonBlankRuntimeString(
				input.CompileContext.EvidencePolicy,
				input.CompileContext.PlanningPolicy,
			),
			Budget: input.CompileContext.ToolBudget,
		},
		LoopPolicy: agentassembly.LoopPolicySnapshot{
			Lifecycle:      agentassembly.LifecycleRequestScope,
			MaxIterations:  input.MaxIterations,
			ToolCallPolicy: string(input.Mode),
		},
		FinalContractPolicy: agentassembly.FinalContractSnapshot{
			Lifecycle: agentassembly.LifecycleRequestScope,
			Shape:     firstNonBlankRuntimeString(input.CompileContext.AnswerStyle, "default"),
		},
		RollbackPolicy: input.RollbackPolicy,
		SourceRefs:     sourceRefs,
	})
	if err != nil {
		return nil, fmt.Errorf("build turn assembly: %w", err)
	}
	return &assembly, nil
}
