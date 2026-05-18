package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	core "aiops-v2/internal/evidence"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.evidence/v1"

func NewRecordTool(service *core.Service) tooling.Tool {
	return newTool("evidence.record", "Record an operational observation as reusable evidence", recordSchema, func(ctx context.Context, input json.RawMessage) (any, error) {
		var req core.RecordRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		rec, err := service.Record(ctx, req)
		if err != nil {
			return nil, err
		}
		return envelope("evidence.record", map[string]any{
			"evidenceRef": rec.Ref,
			"record":      rec,
		}), nil
	})
}

func NewGetTool(service *core.Service) tooling.Tool {
	return newTool("evidence.get", "Read an evidence record by evidence ref", getSchema, func(ctx context.Context, input json.RawMessage) (any, error) {
		var req struct {
			EvidenceRef string `json:"evidenceRef"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		ref := strings.TrimSpace(req.EvidenceRef)
		if ref == "" {
			return nil, fmt.Errorf("evidenceRef is required")
		}
		rec, ok := service.Get(ctx, ref)
		if !ok {
			return nil, fmt.Errorf("evidence ref %q not found", ref)
		}
		return envelope("evidence.get", map[string]any{"record": rec, "evidenceRefs": []string{rec.Ref}}), nil
	})
}

func NewLinkIncidentTool(service *core.Service) tooling.Tool {
	return newTool("evidence.link_incident", "Link evidence refs to an incident with a relation", linkSchema, func(ctx context.Context, input json.RawMessage) (any, error) {
		var req struct {
			IncidentID   string        `json:"incidentId"`
			EvidenceRefs []string      `json:"evidenceRefs"`
			Relation     core.Relation `json:"relation"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		if err := service.LinkIncident(ctx, req.IncidentID, req.EvidenceRefs, req.Relation); err != nil {
			return nil, err
		}
		return envelope("evidence.link_incident", map[string]any{
			"incidentId":   strings.TrimSpace(req.IncidentID),
			"evidenceRefs": req.EvidenceRefs,
			"relation":     firstNonEmpty(string(req.Relation), string(core.RelationContext)),
		}), nil
	})
}

func newTool(name, description string, schema json.RawMessage, execute func(context.Context, json.RawMessage) (any, error)) tooling.Tool {
	meta := evidenceToolMetadata(name, description)
	return &tooling.StaticTool{
		Meta:                meta,
		Visibility:          tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"inspect", "plan", "execute"}},
		InputSchemaData:     schema,
		OutputSchemaData:    outputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			payload, err := execute(ctx, input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{
					Type:  "evidence",
					Title: name,
					Data:  data,
				},
			}, nil
		},
	}
}

func evidenceToolMetadata(name, description string) tooling.ToolMetadata {
	meta := tooling.ToolMetadata{
		Name:        name,
		Origin:      tooling.ToolOriginBuiltin,
		Description: description,
		RiskLevel:   tooling.ToolRiskLow,
	}
	switch name {
	case "evidence.record", "evidence.link_incident":
		meta.Layer = tooling.ToolLayerInternal
	case "evidence.get":
		meta.Layer = tooling.ToolLayerDeferred
		meta.Pack = "evidence_read"
		meta.DeferByDefault = true
	}
	return meta
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

var recordSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"incidentId":{"type":"string"},
		"sourceTool":{"type":"string"},
		"source":{"type":"string"},
		"kind":{"type":"string"},
		"service":{"type":"string"},
		"environment":{"type":"string"},
		"timeRange":{"type":"string"},
		"summary":{"type":"string"},
		"data":{"type":"object"},
		"sessionId":{"type":"string"},
		"turnId":{"type":"string"},
		"toolCallId":{"type":"string"}
	},
	"required":["summary"]
}`)

var getSchema = json.RawMessage(`{"type":"object","properties":{"evidenceRef":{"type":"string"}},"required":["evidenceRef"]}`)

var linkSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"incidentId":{"type":"string"},
		"evidenceRefs":{"type":"array","items":{"type":"string"}},
		"relation":{"type":"string"}
	},
	"required":["incidentId","evidenceRefs"]
}`)

var outputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"data":{"type":"object"}
	},
	"required":["schemaVersion","tool","status"]
}`)
