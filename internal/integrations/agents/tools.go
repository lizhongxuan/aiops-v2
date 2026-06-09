package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentmgr"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.agents/v1"

type SpawnRequest struct {
	AgentType    string                   `json:"agentType"`
	Task         string                   `json:"task"`
	EvidenceGoal string                   `json:"evidenceGoal,omitempty"`
	IncidentID   string                   `json:"incidentId,omitempty"`
	SessionID    string                   `json:"sessionId,omitempty"`
	HostID       string                   `json:"hostId,omitempty"`
	Assignment   agentmgr.AgentAssignment `json:"assignment,omitempty"`
}

type SpawnResult struct {
	AgentID   string `json:"agentId"`
	AgentType string `json:"agentType"`
	Status    string `json:"status"`
}

type Manager interface {
	SpawnInvestigationAgent(ctx context.Context, req SpawnRequest) (SpawnResult, error)
	WaitEvidenceReports(ctx context.Context, agentIDs []string) ([]agentmgr.EvidenceReport, error)
}

func NewSpawnAgentTool(manager Manager) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "spawn_agent",
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Spawn a parallel operations investigation agent for evidence collection only. Allowed agent types are metrics_investigator, logs_investigator, k8s_investigator, change_investigator, and topology_investigator. Do not use for coding or mutating actions.",
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{"workspace"}, Modes: []string{"inspect", "plan", "execute"}},
		InputSchemaData:     spawnSchema,
		OutputSchemaData:    spawnOutputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			if manager == nil {
				return tooling.ToolResult{}, fmt.Errorf("spawn_agent: manager is required")
			}
			var req SpawnRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			req.AgentType = strings.TrimSpace(req.AgentType)
			req.Task = strings.TrimSpace(req.Task)
			req.Assignment = normalizeSpawnAssignment(req)
			if req.Task == "" {
				req.Task = strings.TrimSpace(req.Assignment.Objective)
			}
			if !isAllowedAgentType(req.AgentType) {
				return tooling.ToolResult{}, fmt.Errorf("spawn_agent: agentType %q is not an allowed operations investigator", req.AgentType)
			}
			if req.Task == "" {
				return tooling.ToolResult{}, fmt.Errorf("spawn_agent: task is required")
			}
			if looksLikeCodingTask(req.Task) {
				return tooling.ToolResult{}, fmt.Errorf("spawn_agent: coding tasks are not allowed")
			}
			if lint := agentmgr.ValidateAgentAssignment(req.Assignment); lint.Status != agentmgr.AssignmentLintPass {
				return tooling.ToolResult{}, assignmentLintError(lint)
			}
			result, err := manager.SpawnInvestigationAgent(ctx, req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			payload := envelope("spawn_agent", map[string]any{
				"agentId":   result.AgentID,
				"agentType": result.AgentType,
				"status":    firstNonEmpty(result.Status, "running"),
			})
			return jsonToolResult("agents", "spawn_agent", payload)
		},
	}
}

func NewWaitAgentTool(manager Manager) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "wait_agent",
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Wait for spawned operations investigation agents and return standardized EvidenceReport objects with evidence refs.",
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{"workspace"}, Modes: []string{"inspect", "plan", "execute"}},
		InputSchemaData:     waitSchema,
		OutputSchemaData:    waitOutputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			if manager == nil {
				return tooling.ToolResult{}, fmt.Errorf("wait_agent: manager is required")
			}
			var req struct {
				AgentIDs []string `json:"agentIds"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			ids := normalizeAgentIDs(req.AgentIDs)
			if len(ids) == 0 {
				return tooling.ToolResult{}, fmt.Errorf("wait_agent: agentIds is required")
			}
			reports, err := manager.WaitEvidenceReports(ctx, ids)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			normalized := make([]agentmgr.EvidenceReport, 0, len(reports))
			notifications := make([]agentmgr.AgentNotification, 0, len(reports))
			for _, report := range reports {
				report = report.Normalize()
				if err := report.Validate(); err != nil {
					return tooling.ToolResult{}, err
				}
				normalized = append(normalized, report)
				notifications = append(notifications, notificationFromEvidenceReport(report))
			}
			return jsonToolResult("agents", "wait_agent", envelope("wait_agent", map[string]any{"reports": normalized, "notifications": notifications}))
		},
	}
}

func normalizeSpawnAssignment(req SpawnRequest) agentmgr.AgentAssignment {
	if hasExplicitAssignment(req.Assignment) {
		return req.Assignment
	}
	resourceRefs := make([]string, 0, 4)
	for _, value := range []string{req.HostID, req.IncidentID, req.SessionID, req.AgentType} {
		if strings.TrimSpace(value) != "" {
			resourceRefs = append(resourceRefs, strings.TrimSpace(value))
		}
	}
	if len(resourceRefs) == 0 {
		resourceRefs = []string{"synthetic.unspecified_resource"}
	}
	knownFacts := []string{"legacy spawn_agent request supplied task and evidence goal"}
	if strings.TrimSpace(req.EvidenceGoal) != "" {
		knownFacts = append(knownFacts, strings.TrimSpace(req.EvidenceGoal))
	}
	return agentmgr.AgentAssignment{
		Objective:      req.Task,
		Background:     "Legacy spawn_agent input normalized into a self-contained assignment.",
		KnownFacts:     knownFacts,
		Scope:          agentmgr.AgentScope{ResourceRefs: resourceRefs},
		ExpectedOutput: "Return a bounded EvidenceReport with summary, evidenceRefs, confidence, nextQuestions, and errors.",
		EvidenceRequirement: agentmgr.EvidenceRequirement{
			MinEvidenceRefs: 1,
			RequiredKinds:   []string{"evidence"},
		},
		StopCondition: "Stop after collecting the required evidence refs or reporting a blocker.",
	}
}

func hasExplicitAssignment(assignment agentmgr.AgentAssignment) bool {
	return strings.TrimSpace(assignment.Objective) != "" ||
		strings.TrimSpace(assignment.Background) != "" ||
		len(assignment.KnownFacts) > 0 ||
		!assignment.Scope.IsZero() ||
		strings.TrimSpace(assignment.ExpectedOutput) != "" ||
		!assignment.EvidenceRequirement.IsZero() ||
		strings.TrimSpace(assignment.StopCondition) != "" ||
		len(assignment.Constraints) > 0
}

func assignmentLintError(lint agentmgr.AssignmentLintResult) error {
	payload := map[string]any{
		"code":           "agent_assignment_lint_failed",
		"missingFields":  lint.MissingFields,
		"reasons":        lint.Reasons,
		"requiredAction": "provide_self_contained_assignment",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("spawn_agent: agent_assignment_lint_failed")
	}
	return fmt.Errorf("spawn_agent: %s", string(data))
}

func notificationFromEvidenceReport(report agentmgr.EvidenceReport) agentmgr.AgentNotification {
	status := string(agentmgr.AgentStatusCompleted)
	if len(report.Errors) > 0 && len(report.EvidenceRefs) == 0 {
		status = string(agentmgr.AgentStatusFailed)
	}
	return agentmgr.AgentNotification{
		AgentID:    report.AgentID,
		Status:     status,
		Summary:    report.Summary,
		ResultRefs: append([]string(nil), report.EvidenceRefs...),
		Error:      strings.Join(report.Errors, "; "),
	}
}

func isAllowedAgentType(agentType string) bool {
	switch agentType {
	case "metrics_investigator", "logs_investigator", "k8s_investigator", "change_investigator", "topology_investigator":
		return true
	default:
		return false
	}
}

func looksLikeCodingTask(task string) bool {
	lower := strings.ToLower(task)
	for _, marker := range []string{"edit code", "write code", "apply_patch", "refactor", "implement feature", "fix test", "commit"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func normalizeAgentIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func jsonToolResult(displayType, title string, payload any) (tooling.ToolResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(data),
		Display: &tooling.ToolDisplayPayload{
			Type:  displayType,
			Title: title,
			Data:  data,
		},
	}, nil
}

func envelope(tool string, data map[string]any) map[string]any {
	return map[string]any{
		"schemaVersion": schemaVersion,
		"tool":          tool,
		"status":        "ok",
		"data":          data,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var spawnSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"agentType":{"type":"string","enum":["metrics_investigator","logs_investigator","k8s_investigator","change_investigator","topology_investigator"]},
		"task":{"type":"string"},
		"evidenceGoal":{"type":"string"},
		"incidentId":{"type":"string"},
		"sessionId":{"type":"string"},
		"hostId":{"type":"string"},
		"assignment":{
			"type":"object",
			"properties":{
				"objective":{"type":"string"},
				"background":{"type":"string"},
				"knownFacts":{"type":"array","items":{"type":"string"}},
				"scope":{
					"type":"object",
					"properties":{
						"resourceRefs":{"type":"array","items":{"type":"string"}},
						"timeRange":{"type":"string"},
						"exclusions":{"type":"array","items":{"type":"string"}}
					}
				},
				"expectedOutput":{"type":"string"},
				"evidenceRequirement":{
					"type":"object",
					"properties":{
						"minEvidenceRefs":{"type":"integer"},
						"requiredKinds":{"type":"array","items":{"type":"string"}}
					}
				},
				"stopCondition":{"type":"string"},
				"constraints":{"type":"array","items":{"type":"string"}}
			}
		}
	},
	"required":["agentType"]
}`)

var waitSchema = json.RawMessage(`{
	"type":"object",
	"properties":{"agentIds":{"type":"array","items":{"type":"string"}}},
	"required":["agentIds"]
}`)

var spawnOutputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"data":{"type":"object"}
	},
	"required":["schemaVersion","tool","status","data"]
}`)

var waitOutputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"data":{
			"type":"object",
			"properties":{
				"reports":{
					"type":"array",
					"items":{
						"type":"object",
						"properties":{
							"agentId":{"type":"string"},
							"summary":{"type":"string"},
							"evidenceRefs":{"type":"array","items":{"type":"string"}},
							"confidence":{"type":"string"},
							"nextQuestions":{"type":"array","items":{"type":"string"}},
							"errors":{"type":"array","items":{"type":"string"}}
						},
						"required":["agentId","summary","evidenceRefs","confidence","nextQuestions","errors"]
					}
				},
				"notifications":{"type":"array","items":{"type":"object"}}
			}
		}
	},
	"required":["schemaVersion","tool","status","data"]
}`)
