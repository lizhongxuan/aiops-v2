package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/modelrouter"
)

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
	SessionID       string                        `json:"sessionId"`
	TurnID          string                        `json:"turnId"`
	ClientTurnID    string                        `json:"clientTurnId,omitempty"`
	ClientMessageID string                        `json:"clientMessageId,omitempty"`
	SessionType     SessionType                   `json:"sessionType"`
	Mode            Mode                          `json:"mode"`
	Route           RuntimeRouteSnapshot          `json:"route"`
	Profile         string                        `json:"profile,omitempty"`
	HostID          string                        `json:"hostId,omitempty"`
	Model           modelrouter.ModelCapabilities `json:"model"`
	Permission      RuntimePermissionSnapshot     `json:"permission"`
	ContextBudget   RuntimeContextBudgetSnapshot  `json:"contextBudget"`
	ToolPolicyHash  string                        `json:"toolPolicyHash,omitempty"`
	Lineage         RuntimeLineageSnapshot        `json:"lineage,omitempty"`
	Metadata        map[string]string             `json:"metadata,omitempty"`
}

type RuntimeTurnContextOptions struct {
	Model          modelrouter.ModelCapabilities
	ContextBudget  RuntimeContextBudgetSnapshot
	ToolPolicyHash string
	Lineage        RuntimeLineageSnapshot
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
	profile := firstMetadataValue(metadata, "profile", "toolProfile", "agentProfile")
	if profile == "" {
		profile = RuntimePromptProfileAdvisor
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" && session != nil {
		hostID = strings.TrimSpace(session.HostID)
	}
	route := strings.TrimSpace(metadata["runtimeRoute"])
	if route == "" {
		route = string(req.SessionType)
	}
	return RuntimeTurnContext{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Route:           RuntimeRouteSnapshot{Route: route, HostID: hostID, Profile: profile},
		Profile:         profile,
		HostID:          hostID,
		Model:           opts.Model,
		Permission: RuntimePermissionSnapshot{
			ApprovalPolicy: strings.TrimSpace(metadata["approvalPolicy"]),
			PermissionHash: strings.TrimSpace(metadata["permissionHash"]),
		},
		ContextBudget:  opts.ContextBudget,
		ToolPolicyHash: strings.TrimSpace(opts.ToolPolicyHash),
		Lineage:        opts.Lineage,
		Metadata:       metadata,
	}, nil
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
