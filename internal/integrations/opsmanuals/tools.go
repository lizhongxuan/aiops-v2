package opsmanuals

import (
	"context"
	"encoding/json"
	"fmt"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

const searchOpsManualsToolName = "search_ops_manuals"

func newSearchOpsManualsTool(service *core.Service, cache *turnSearchContextCache) tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"chat", "inspect", "plan", "execute"},
	}
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           searchOpsManualsToolName,
			Aliases:        []string{"ops_manual.search"},
			Origin:         tooling.ToolOriginBuiltin,
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "ops_manual_flow",
			DeferByDefault: true,
			Description:    "Search verified ops manuals for an operations request and return an auditable decision: direct_execute, need_info, adapt, reference_only, or no_match. Preserve explicit user targets, negations, and clearly stated semantics in text or operation_frame.",
			SearchHint:     "ops manual runbook repair fix operation procedure search",
			Triggers:       []string{"ops manual", "runbook", "manual", "repair", "fix", "recover", "restore", "修复", "恢复", "手册", "预案", "操作步骤"},
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				DiscoveryGroup:    "runbook",
				DiscoveryTags:     []string{"manual", "runbook", "procedure", "preflight"},
				CapabilityKind:    "search",
				ResourceTypes:     []string{"manual", "runbook", "procedure"},
				OperationKinds:    []string{"search", "read"},
				RequiresSelect:    true,
				PermissionScope:   "read",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
			},
		},
		Visibility:      visibility,
		InputSchemaData: searchOpsManualsInputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return false
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := decodeSearchOpsManualsInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			req = enrichSearchRequestFromExecutionContext(ctx, req)
			result, err := service.SearchOpsManuals(req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if cache != nil {
				cache.remember(ctx, result)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			modelContent, err := json.Marshal(searchOpsManualsModelResult(result))
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(modelContent),
				Display: &tooling.ToolDisplayPayload{
					Type:  "ops_manual_search_result",
					Title: searchOpsManualsToolName,
					Data:  data,
				},
			}, nil
		},
	}
}

type searchOpsManualsModelManual struct {
	ID                string   `json:"id,omitempty"`
	Title             string   `json:"title,omitempty"`
	UsableMode        string   `json:"usable_mode,omitempty"`
	RecommendedAction string   `json:"recommended_action,omitempty"`
	BlockedReasons    []string `json:"blocked_reasons,omitempty"`
}

type searchOpsManualsModelPayload struct {
	Decision              string                        `json:"decision"`
	OpsManualFlowID       string                        `json:"ops_manual_flow_id,omitempty"`
	Summary               string                        `json:"summary"`
	Manuals               []searchOpsManualsModelManual `json:"manuals,omitempty"`
	SuppressedManuals     []string                      `json:"suppressed_manuals,omitempty"`
	SuppressionReason     string                        `json:"suppression_reason,omitempty"`
	NextQuestions         []string                      `json:"next_questions,omitempty"`
	RecommendedNextAction string                        `json:"recommended_next_action,omitempty"`
	Instructions          []string                      `json:"instructions,omitempty"`
}

func searchOpsManualsModelResult(result core.SearchOpsManualsResult) searchOpsManualsModelPayload {
	payload := searchOpsManualsModelPayload{
		Decision:              string(result.Decision),
		OpsManualFlowID:       result.OpsManualFlowID,
		Summary:               result.Summary,
		SuppressedManuals:     limitStrings(result.SuppressedManuals, 3),
		SuppressionReason:     result.SuppressionReason,
		NextQuestions:         limitStrings(result.NextQuestions, 2),
		RecommendedNextAction: result.RecommendedNextAction,
	}
	if len(result.Manuals) > 0 {
		hit := result.Manuals[0]
		payload.Manuals = []searchOpsManualsModelManual{{
			ID:                hit.Manual.ID,
			Title:             hit.Manual.Title,
			UsableMode:        string(hit.UsableMode),
			RecommendedAction: hit.RecommendedAction,
			BlockedReasons:    limitStrings(hit.BlockedReasons, 2),
		}}
	}
	switch result.Decision {
	case core.DecisionNeedInfo:
		if len(result.Manuals) > 0 {
			payload.Instructions = []string{
				"Do not execute the workflow.",
				"Call resolve_ops_manual_params before asking the user any manual parameters.",
				"Your immediate next action must be a resolve_ops_manual_params tool call with the matched manual_id; do not run host commands, ask prose questions, or fall back to normal investigation before it returns.",
				"Do not ask fixed target/location/execution/symptom fields yourself.",
				"Tell the user this is missing ops manual matching context, not a Workflow preflight failure.",
				"Do not repeat card details such as manual id, workflow id, decision, score, or all missing fields.",
				"Do not list internal missing_fields or request a full environment template.",
				"Do not ask the user whether Coroot evidence exists; use available Coroot read-only tools with the session-bound project/environment instead.",
			}
		} else {
			payload.Instructions = []string{
				"Do not execute the workflow.",
				"No matched manual_id is available yet; do not call resolve_ops_manual_params.",
				"If object or action is genuinely missing, ask only the smallest missing question instead of a fixed target/location/execution/symptom template.",
				"Do not call search_ops_manuals again just to fill missing fields.",
				"Do not mention operations manual search or no-match status unless the user explicitly asked about manuals.",
				"Continue ordinary safe read-only investigation when the user asked to investigate or diagnose.",
			}
		}
	case core.DecisionDirectExecute:
		payload.Instructions = []string{
			"Do not execute the workflow directly.",
			"Say the manual is matched and the next step is Workflow preflight; do not say it will execute now.",
			"Run run_ops_manual_preflight before any confirmation, approval, workflow execution, or mutation.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionAdapt:
		payload.Instructions = []string{
			"Say the manual is relevant but the Workflow needs adaptation before preflight.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionReference:
		payload.Instructions = []string{
			"Do not mention operations manual search or runnable Workflow status unless the user explicitly asked about manuals.",
			"Continue safe read-only investigation with available tools when the target, time window, permissions, and data sources are sufficient.",
			"If read-only automation cannot continue, state the concrete blocker: missing target cluster/service/consumer group, time window, Kafka tooling, metrics/log source, permission, or host/session availability.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionNoMatch:
		payload.Instructions = []string{
			"Do not mention operations manual search or no-match status unless the user explicitly asked about manuals.",
			"Do not mention or expose cross-object manuals as references unless the user explicitly asks for analogous patterns.",
			"Continue normal safe read-only evidence-driven investigation with available tools.",
			"If read-only automation cannot continue, state the concrete blocker: missing target cluster/service/consumer group, time window, Kafka tooling, metrics/log source, permission, or host/session availability.",
			"When required user-provided fields block progress, rely on the compact Agent-to-UI form when next_questions are present; do not duplicate the form as a multiline prose template.",
			"Keep the assistant text short and avoid listing internal search fields.",
		}
	}
	return payload
}

func limitStrings(values []string, limit int) []string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:limit]...)
}

func decodeSearchOpsManualsInput(input json.RawMessage) (core.SearchOpsManualsRequest, error) {
	var req core.SearchOpsManualsRequest
	if len(input) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return req, fmt.Errorf("invalid search_ops_manuals input: %w", err)
	}
	return req, nil
}

var searchOpsManualsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "text": {
      "type": "string",
      "description": "The user's original operations request, verbatim. Preserve target names, negations, read-only constraints, symptoms, metrics, and paths."
    },
    "metadata": {
      "type": "object",
      "additionalProperties": true
    },
    "operation_frame": {
      "type": "object",
      "description": "Structured semantics extracted by the model when the user clearly states them. Prefer this over backend text guessing. Common fields: object_type or target.type (redis/mysql/postgresql/kubernetes_pod/kafka/host/network), operation_type or operation.action (status_check/rca_or_repair/backup/restore/restart), target.name only when explicitly provided as a concrete instance/service/pod/container/host/resource name, target_scope.hosts for the selected/current host, target_scope.namespace/cluster/service, environment.execution_surface/platform/env, evidence.provided/missing, risk.level/data_mutation/service_restart, required_params such as backup_path.",
      "additionalProperties": true
    },
    "limit": {
      "type": "integer",
      "minimum": 1
    }
  },
  "additionalProperties": false
}`)
