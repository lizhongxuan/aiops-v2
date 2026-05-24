package runtimekernel

import (
	"encoding/json"
	"strings"
)

const (
	opsManualFlowPackName        = "ops_manual_flow"
	opsManualPreflightToolName   = "run_ops_manual_preflight"
	opsManualSearchResultType    = "ops_manual_search_result"
	opsManualParamResolutionType = "ops_manual_param_resolution"
)

func cloneTurnMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func updateOpsManualFlowTurnMetadata(metadata map[string]string, result ToolResult) map[string]string {
	if result.Display == nil {
		return metadata
	}
	switch result.Display.Type {
	case opsManualSearchResultType:
		if !opsManualSearchMatchedManual(result.Display.Data) {
			return metadata
		}
		metadata = ensureTurnMetadata(metadata)
		metadata["opsManualMatched"] = "true"
		metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], opsManualFlowPackName)
		if opsManualSearchDecision(result.Display.Data) == "direct_execute" {
			metadata["opsManualDirectExecute"] = "true"
			metadata["enableTool"] = appendMetadataListValue(metadata["enableTool"], opsManualPreflightToolName)
		}
	case opsManualParamResolutionType:
		if !opsManualParamsResolved(result.Display.Data) {
			return metadata
		}
		metadata = ensureTurnMetadata(metadata)
		metadata["opsManualParamsResolved"] = "true"
		metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], opsManualFlowPackName)
		metadata["enableTool"] = appendMetadataListValue(metadata["enableTool"], opsManualPreflightToolName)
	}
	return metadata
}

func ensureTurnMetadata(metadata map[string]string) map[string]string {
	if metadata != nil {
		return metadata
	}
	return map[string]string{}
}

func opsManualSearchMatchedManual(data json.RawMessage) bool {
	if len(data) == 0 {
		return false
	}
	var payload struct {
		Manuals []json.RawMessage `json:"manuals"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	return len(payload.Manuals) > 0
}

func opsManualSearchDecision(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var payload struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(payload.Decision))
}

func opsManualParamsResolved(data json.RawMessage) bool {
	if len(data) == 0 {
		return false
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(payload.Status), "resolved")
}

func appendMetadataListValue(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	values := strings.FieldsFunc(current, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	for _, value := range values {
		if strings.TrimSpace(value) == next {
			return strings.TrimSpace(current)
		}
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	return strings.TrimSpace(current) + "," + next
}
