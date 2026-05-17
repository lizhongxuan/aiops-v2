package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.k8s/v1"

type k8sInput struct {
	Environment string `json:"environment,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Container   string `json:"container,omitempty"`
	Replicas    int    `json:"replicas,omitempty"`
	ActionToken string `json:"actionToken,omitempty"`
}

func tools(opts Options) []tooling.Tool {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	visibility := tooling.Visibility{SessionTypes: []string{"workspace", "host"}, Modes: []string{"inspect", "execute"}}
	return []tooling.Tool{
		newReadOnlyTool("k8s.get_workload", "Read a Kubernetes workload summary", visibility, func(in k8sInput) any {
			return map[string]any{"schemaVersion": schemaVersion, "tool": "k8s.get_workload", "status": "ok", "workload": Workload{Kind: firstNonEmpty(in.Kind, "deployment"), Namespace: firstNonEmpty(in.Namespace, "prod"), Name: firstNonEmpty(in.Name, "order-api"), Replicas: 3, Image: "registry.example/erp/order-api:2026.05.04", Status: "available"}}
		}),
		newReadOnlyTool("k8s.get_events", "Read Kubernetes events for a workload", visibility, func(in k8sInput) any {
			return map[string]any{"schemaVersion": schemaVersion, "tool": "k8s.get_events", "status": "ok", "events": []Event{{Type: "Warning", Reason: "Backoff", Message: "recent restart backoff observed", Timestamp: "2026-05-04T09:58:00Z"}}}
		}),
		newReadOnlyTool("k8s.get_logs", "Read recent Kubernetes logs for a workload", visibility, func(in k8sInput) any {
			return map[string]any{"schemaVersion": schemaVersion, "tool": "k8s.get_logs", "status": "ok", "logs": []LogLine{{Timestamp: "2026-05-04T09:59:00Z", Message: "waiting for database connection"}}}
		}),
		newReadOnlyTool("k8s.rollout_status", "Read Kubernetes rollout status", visibility, func(in k8sInput) any {
			return map[string]any{"schemaVersion": schemaVersion, "tool": "k8s.rollout_status", "status": "ok", "rollout": RolloutStatus{Namespace: firstNonEmpty(in.Namespace, "prod"), Name: firstNonEmpty(in.Name, "order-api"), Status: "progressing", Revision: "42"}}
		}),
		newMutatingTool("k8s.restart_workload", "Restart a Kubernetes workload after ActionToken and approval", visibility, opts),
		newMutatingTool("k8s.scale_workload", "Scale a Kubernetes workload after ActionToken and approval", visibility, opts),
		newMutatingTool("k8s.rollout_undo", "Undo a Kubernetes rollout after ActionToken and approval", visibility, opts),
	}
}

func newReadOnlyTool(name, description string, visibility tooling.Visibility, build func(k8sInput) any) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Origin:      tooling.ToolOriginBuiltin,
			Description: description,
			Domain:      "kubernetes",
			Mock:        true,
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility:       visibility,
		InputSchemaData:  k8sSchema(),
		OutputSchemaData: toolEnvelopeSchema(),
		ReadOnlyFunc:     func(json.RawMessage) bool { return true },
		DestructiveFunc:  func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			in, err := decodeInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			payload := ensureEnvelopeFields(build(in), name, "mock", true)
			data, _ := json.Marshal(payload)
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "k8s", Title: name, Data: data}}, nil
		},
	}
}

func newMutatingTool(name, description string, visibility tooling.Visibility, opts Options) tooling.Tool {
	signer := actionproposal.NewSigner(opts.ActionTokenSecret, opts.Now)
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             name,
			Origin:           tooling.ToolOriginBuiltin,
			Description:      description,
			Domain:           "kubernetes",
			Mock:             true,
			RiskLevel:        tooling.ToolRiskHigh,
			Mutating:         true,
			RequiresApproval: true,
		},
		Visibility:       visibility,
		InputSchemaData:  k8sSchema(),
		OutputSchemaData: toolEnvelopeSchema(),
		ReadOnlyFunc:     func(json.RawMessage) bool { return false },
		DestructiveFunc:  func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return false
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			in, err := decodeInput(input)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
			}
			return checkActionToken(ctx, name, input, in, signer)
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			in, err := decodeInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, _ := json.Marshal(map[string]any{
				"schemaVersion": schemaVersion,
				"tool":          name,
				"status":        "planned",
				"source":        "mock",
				"mock":          true,
				"evidenceRefs":  []string{},
				"workload":      Workload{Kind: firstNonEmpty(in.Kind, "deployment"), Namespace: firstNonEmpty(in.Namespace, "prod"), Name: firstNonEmpty(in.Name, "order-api"), Replicas: in.Replicas, Status: "pending_approval"},
			})
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "k8s", Title: name, Data: data}}, nil
		},
	}
}

func checkActionToken(ctx context.Context, toolName string, input json.RawMessage, in k8sInput, signer *actionproposal.Signer) tooling.PermissionDecision {
	token := strings.TrimSpace(in.ActionToken)
	if execCtx, ok := tooling.ToolExecutionContextFrom(ctx); ok && strings.TrimSpace(execCtx.ActionToken) != "" {
		token = strings.TrimSpace(execCtx.ActionToken)
	}
	if token == "" {
		return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: "mutating Kubernetes action requires a signed ActionToken"}
	}
	inputHash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: "unable to normalize Kubernetes action input"}
	}
	claims, err := signer.Verify(token, actionproposal.ActionTokenClaims{ToolName: toolName, InputHash: inputHash})
	if err != nil {
		return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: "invalid ActionToken: " + err.Error()}
	}
	if claims.Source != actionproposal.SourceRunbook && claims.Source != actionproposal.SourceFallback && claims.Source != actionproposal.SourceBreakGlass {
		return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "ActionToken source is not allowed for Kubernetes mutation"}
	}
	approval := &tooling.PermissionApprovalPayload{
		Command:        toolName + " " + firstNonEmpty(in.Namespace, "default") + "/" + firstNonEmpty(in.Name, "workload"),
		Reason:         firstNonEmpty(claims.Reason, "Kubernetes mutation requires approval"),
		Risk:           string(claims.Risk),
		Source:         string(claims.Source),
		RunbookID:      claims.RunbookID,
		RunbookStep:    claims.RunbookStepID,
		ExpectedEffect: claims.ExpectedEffect,
		Rollback:       claims.Rollback,
	}
	if claims.Risk == actionproposal.RiskLow {
		return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
	}
	return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: approval.Reason, Approval: approval}
}

func decodeInput(input json.RawMessage) (k8sInput, error) {
	var in k8sInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return k8sInput{}, fmt.Errorf("invalid Kubernetes input: %w", err)
		}
	}
	return in, nil
}

func k8sSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"environment":{"type":"string"},"namespace":{"type":"string"},"kind":{"type":"string"},"name":{"type":"string"},"container":{"type":"string"},"replicas":{"type":"integer"},"actionToken":{"type":"string"}}}`)
}

func toolEnvelopeSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"schemaVersion":{"type":"string"},
			"tool":{"type":"string"},
			"status":{"type":"string"},
			"source":{"type":"string"},
			"mock":{"type":"boolean"},
			"evidenceRefs":{"type":"array","items":{"type":"string"}}
		},
		"required":["schemaVersion","tool","status"]
	}`)
}

func ensureEnvelopeFields(payload any, toolName, source string, mock bool) map[string]any {
	out, ok := payload.(map[string]any)
	if !ok {
		out = map[string]any{"data": payload}
	}
	if out["schemaVersion"] == nil {
		out["schemaVersion"] = schemaVersion
	}
	if out["tool"] == nil {
		out["tool"] = toolName
	}
	if out["status"] == nil {
		out["status"] = "ok"
	}
	out["source"] = source
	out["mock"] = mock
	if _, ok := out["evidenceRefs"]; !ok {
		out["evidenceRefs"] = []string{}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
