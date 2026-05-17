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
			Name:        searchOpsManualsToolName,
			Aliases:     []string{"ops_manual.search"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Search verified ops manuals before complex or high-risk operations such as service restart, configuration change, database backup/restore/migration, cluster change, or any named middleware/infrastructure troubleshooting request. Do not call this tool when the user opted out of operations manuals, skipped a manual, or metadata opsManualAction=skip_ops_manual / opsManualSkipped=true is present for the current continuation. Even short requests like 排查 Redis, 检查 pg 状态, MySQL 备份, Pod CrashLoopBackOff, or Kafka lag must call this tool before prose follow-up questions unless the user opted out. The model is responsible for semantic interpretation: pass the user's original task text verbatim and, when the user clearly states object/action/target/environment, also pass an explicit operation_frame instead of relying on backend natural-language guesses. If the user names a concrete instance/service/pod/container/host/resource, set operation_frame.target.name to that exact user-provided name; keep the current host in target_scope.hosts instead of using it as target.name. Include object_type/target.type, operation_type/operation.action, target_scope, environment, evidence, risk, and required_params when those semantics are present. Do not summarize away target_instance, execution_surface, symptoms, metrics, risk, explicit target names, or negations such as no restart/no write. Return an auditable decision: direct_execute, need_info, adapt, reference_only, or no_match; follow that decision and never treat need_info, adapt, or reference_only as directly executable.",
			RiskLevel:   tooling.ToolRiskLow,
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
	Summary               string                        `json:"summary"`
	Manuals               []searchOpsManualsModelManual `json:"manuals,omitempty"`
	NextQuestions         []string                      `json:"next_questions,omitempty"`
	RecommendedNextAction string                        `json:"recommended_next_action,omitempty"`
	Instructions          []string                      `json:"instructions,omitempty"`
}

func searchOpsManualsModelResult(result core.SearchOpsManualsResult) searchOpsManualsModelPayload {
	payload := searchOpsManualsModelPayload{
		Decision:              string(result.Decision),
		Summary:               result.Summary,
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
				"If the user request already states object, action, target, environment, evidence, or risk, call search_ops_manuals again with an explicit operation_frame that preserves those semantics.",
				"If object or action is genuinely missing, ask only the smallest missing question instead of a fixed target/location/execution/symptom template.",
				"Do not run host commands, Coroot probes, ordinary shell checks, or normal investigation until the ops manual search has either matched a manual or returned no_match/reference_only.",
				"Tell the user this is missing ops manual matching context, not a Workflow preflight failure.",
			}
		}
	case core.DecisionDirectExecute:
		payload.Instructions = []string{
			"Do not execute the workflow directly.",
			"Say the manual is matched and the next step is Workflow preflight; do not say it will execute now.",
			"Run run_ops_manual_preflight before Dry Run or mutation.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionAdapt:
		payload.Instructions = []string{
			"Say the manual is relevant but the Workflow needs adaptation before preflight.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionReference:
		payload.Instructions = []string{
			"Say there is no directly runnable Workflow for this request, so no bound Workflow should run.",
			"Continue safe read-only investigation with available tools when the target, time window, permissions, and data sources are sufficient.",
			"If read-only automation cannot continue, state the concrete blocker: missing target cluster/service/consumer group, time window, Kafka tooling, metrics/log source, permission, or host/session availability.",
			"Keep the assistant text short and do not repeat the Agent-to-UI card details.",
		}
	case core.DecisionNoMatch:
		payload.Instructions = []string{
			"Say no usable operations manual or bound Workflow was found for this request.",
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
