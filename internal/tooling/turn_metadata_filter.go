package tooling

import "strings"

func turnMetadataToolFilter(metadata map[string]string) func(Tool, ToolContext, ToolMetadata) bool {
	if !opsManualsOptedOut(metadata) {
		return nil
	}
	return func(_ Tool, _ ToolContext, meta ToolMetadata) bool {
		return IsToolVisibleForTurnMetadata(meta, metadata)
	}
}

func IsToolVisibleForTurnMetadata(meta ToolMetadata, metadata map[string]string) bool {
	if !opsManualsOptedOut(metadata) {
		return true
	}
	switch meta.Name {
	case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
		return false
	default:
		return true
	}
}

func opsManualsOptedOut(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "skip_ops_manual") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualSkipped"]), "true")
}
