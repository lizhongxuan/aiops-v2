package runtimekernel

import (
	"errors"
	"fmt"
	"strings"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

const runtimeAdmissionErrorTargetRequired = "admission_target_required"

type RuntimeRouteSnapshot struct {
	Route   string `json:"route,omitempty"`
	HostID  string `json:"hostId,omitempty"`
	Profile string `json:"profile,omitempty"`
}

type RuntimePermissionSnapshot struct {
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	PermissionHash string `json:"permissionHash,omitempty"`
}

type RuntimeContextBudgetSnapshot struct {
	MaxTokens    int `json:"maxTokens,omitempty"`
	TargetTokens int `json:"targetTokens,omitempty"`
}

type RuntimeLineageSnapshot struct {
	ParentSessionID string `json:"parentSessionId,omitempty"`
	ParentTurnID    string `json:"parentTurnId,omitempty"`
	AgentKind       string `json:"agentKind,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
}

type RuntimeTurnContext struct {
	SessionID        string                         `json:"sessionId"`
	TurnID           string                         `json:"turnId"`
	ClientTurnID     string                         `json:"clientTurnId,omitempty"`
	ClientMessageID  string                         `json:"clientMessageId,omitempty"`
	SessionType      SessionType                    `json:"sessionType"`
	Mode             Mode                           `json:"mode"`
	Route            RuntimeRouteSnapshot           `json:"route"`
	Profile          string                         `json:"profile,omitempty"`
	HostID           string                         `json:"hostId,omitempty"`
	Model            modelrouter.ModelCapabilities  `json:"model"`
	Permission       RuntimePermissionSnapshot      `json:"permission"`
	ContextBudget    RuntimeContextBudgetSnapshot   `json:"contextBudget"`
	ToolPolicyHash   string                         `json:"toolPolicyHash,omitempty"`
	Lineage          RuntimeLineageSnapshot         `json:"lineage,omitempty"`
	Metadata         map[string]string              `json:"metadata,omitempty"`
	AdmissionFacts   runtimecontract.AdmissionFacts `json:"admissionFacts"`
	AdmissionError   string                         `json:"admissionError,omitempty"`
	TurnAssemblyHash string                         `json:"turnAssemblyHash,omitempty"`
}

type RuntimeTurnContextOptions struct {
	Model            modelrouter.ModelCapabilities
	ContextBudget    RuntimeContextBudgetSnapshot
	ToolPolicyHash   string
	Lineage          RuntimeLineageSnapshot
	TurnAssemblyHash string
}

func BuildRuntimeTurnContext(req TurnRequest, session *SessionState, opts RuntimeTurnContextOptions) (RuntimeTurnContext, error) {
	if err := req.Validate(); err != nil {
		return RuntimeTurnContext{}, err
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return RuntimeTurnContext{}, fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(req.TurnID) == "" {
		return RuntimeTurnContext{}, fmt.Errorf("turn id is required")
	}
	metadata := copyRuntimeMetadata(req.Metadata)
	targetRefs := runtimeTurnTargetRefs(req, session)
	roleBindings, roleConflicts := runtimeTurnRoleFacts(req, session, targetRefs)
	target := resourcebinding.ResourceRef{}
	if len(targetRefs) == 1 {
		target = targetRefs[0]
	}
	admission, admissionErr := runtimecontract.BuildAdmissionFacts(runtimecontract.AdmissionInput{
		Intent:            req.IntentFrame,
		SessionTarget:     target,
		TargetRefs:        targetRefs,
		ResourceBindings:  req.ResourceBindings,
		RoleBindings:      roleBindings,
		RoleConflicts:     roleConflicts,
		AgentKind:         opts.Lineage.AgentKind,
		DefaultProfile:    RuntimePromptProfileAdvisor,
		PermissionProfile: strings.TrimSpace(req.PermissionProfile),
		SourceRefs:        []string{"runtimekernel:turn_request"},
		Metadata:          metadata,
	})
	if runtimecontract.IsAdmissionControlConflict(admissionErr) {
		return RuntimeTurnContext{}, fmt.Errorf("runtime admission control conflict")
	}
	profile := firstNonBlankRuntimeString(admission.Profile, RuntimePromptProfileAdvisor)
	hostID := runtimeTurnHostID(admission.TargetRefs)
	route := deriveRuntimeRouteSnapshot(admission, req.SessionType, hostID)
	return RuntimeTurnContext{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Route:           route,
		Profile:         profile,
		HostID:          hostID,
		Model:           opts.Model,
		Permission: RuntimePermissionSnapshot{
			ApprovalPolicy: strings.TrimSpace(metadata[runtimecontract.MetadataApprovalPolicy]),
			PermissionHash: strings.TrimSpace(metadata[runtimecontract.MetadataPermissionHash]),
		},
		ContextBudget:    opts.ContextBudget,
		ToolPolicyHash:   strings.TrimSpace(opts.ToolPolicyHash),
		Lineage:          opts.Lineage,
		Metadata:         metadata,
		AdmissionFacts:   admission,
		AdmissionError:   admissionErrorText(admissionErr),
		TurnAssemblyHash: strings.TrimSpace(opts.TurnAssemblyHash),
	}, nil
}

func runtimeTurnRoleFacts(req TurnRequest, session *SessionState, targetRefs []resourcebinding.ResourceRef) ([]resourcebinding.ResourceRoleBinding, []resourcebinding.RoleBindingConflict) {
	bindings := append([]resourcebinding.ResourceRoleBinding(nil), req.ResourceRoleBindings...)
	conflicts := append([]resourcebinding.RoleBindingConflict(nil), req.RoleBindingConflicts...)
	if req.SessionTargetSnapshot != nil || len(bindings) > 0 || len(conflicts) > 0 || session == nil || session.SessionTargetSnapshot == nil {
		return bindings, conflicts
	}
	snapshot := session.SessionTargetSnapshot
	if snapshot.Expired() || snapshot.RequiresConfirmation || snapshot.Confidence <= 0 || len(targetRefs) == 0 {
		return bindings, conflicts
	}
	allowed := make(map[string]struct{}, len(targetRefs))
	for _, ref := range targetRefs {
		if hash := resourcebinding.NormalizeRef(ref).IdentityHash(); hash != "" {
			allowed[hash] = struct{}{}
		}
	}
	for _, binding := range session.ResourceRoleBindings {
		if _, ok := allowed[resourcebinding.NormalizeRef(binding.ResourceRef).IdentityHash()]; !ok {
			return nil, nil
		}
	}
	return append([]resourcebinding.ResourceRoleBinding(nil), session.ResourceRoleBindings...), append([]resourcebinding.RoleBindingConflict(nil), session.RoleBindingConflicts...)
}

func runtimeTurnTargetRefs(req TurnRequest, session *SessionState) []resourcebinding.ResourceRef {
	var snapshot *resourcebinding.SessionTargetSnapshot
	if req.SessionTargetSnapshot != nil {
		snapshot = req.SessionTargetSnapshot
	} else if session != nil {
		snapshot = session.SessionTargetSnapshot
	}
	var refs []resourcebinding.ResourceRef
	if snapshot != nil && !snapshot.Expired() && !snapshot.RequiresConfirmation && snapshot.Confidence > 0 {
		for _, hostID := range resourcebinding.HostIDsFromSessionTarget(snapshot) {
			refs = append(refs, resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: hostID})
		}
	} else if req.SessionTargetSnapshot == nil {
		if hostID := strings.TrimSpace(req.HostID); hostID != "" {
			refs = append(refs, resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: hostID})
		} else {
			for _, binding := range req.ResourceBindings {
				if binding.Verified() {
					refs = append(refs, binding.Ref)
				}
			}
		}
	}
	return normalizeRuntimeTargetRefs(refs)
}

func normalizeRuntimeTargetRefs(values []resourcebinding.ResourceRef) []resourcebinding.ResourceRef {
	seen := map[string]bool{}
	out := make([]resourcebinding.ResourceRef, 0, len(values))
	for _, ref := range values {
		ref = resourcebinding.NormalizeRef(ref)
		hash := ref.IdentityHash()
		if hash == "" || seen[hash] {
			continue
		}
		seen[hash] = true
		out = append(out, ref)
	}
	return out
}

func runtimeTurnHostID(refs []resourcebinding.ResourceRef) string {
	if len(refs) != 1 || refs[0].Type != resourcebinding.ResourceTypeHost {
		return ""
	}
	return strings.TrimSpace(refs[0].ID)
}

func deriveRuntimeRouteSnapshot(facts runtimecontract.AdmissionFacts, sessionType SessionType, hostID string) RuntimeRouteSnapshot {
	route := string(sessionType)
	if len(facts.TargetRefs) > 1 {
		route = "multi_host_manager"
	} else if len(facts.TargetRefs) == 1 {
		route = "host_bound_ops"
	} else {
		switch facts.Intent.Kind {
		case runtimecontract.IntentKindResearch:
			route = "research"
		case runtimecontract.IntentKindDiagnose, runtimecontract.IntentKindVerify:
			route = "evidence_rca"
		}
	}
	return RuntimeRouteSnapshot{Route: route, HostID: strings.TrimSpace(hostID), Profile: facts.Profile}
}

func admissionErrorText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, runtimecontract.ErrAdmissionTargetRequired) {
		return runtimeAdmissionErrorTargetRequired
	}
	return "admission_facts_invalid"
}

func copyRuntimeMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
