package tooling

import "strings"

func AssembleOptionsForTurnMetadata(metadata map[string]string) AssembleOptions {
	return ApplyTurnMetadataToAssembleOptions(AssembleOptions{}, metadata)
}

func ApplyTurnMetadataToAssembleOptions(opts AssembleOptions, metadata map[string]string) AssembleOptions {
	opts.EnabledPacks = appendUniqueStrings(opts.EnabledPacks, metadataListValue(metadata, "enableToolPack")...)
	metadataFilter := turnMetadataToolFilter(metadata)
	if metadataFilter == nil {
		return opts
	}
	if opts.Filter == nil {
		opts.Filter = metadataFilter
		return opts
	}
	existingFilter := opts.Filter
	opts.Filter = func(t Tool, ctx ToolContext, meta ToolMetadata) bool {
		return existingFilter(t, ctx, meta) && metadataFilter(t, ctx, meta)
	}
	return opts
}

func turnMetadataToolFilter(metadata map[string]string) func(Tool, ToolContext, ToolMetadata) bool {
	if len(metadata) == 0 {
		return nil
	}
	return func(_ Tool, _ ToolContext, meta ToolMetadata) bool {
		return IsToolVisibleForTurnMetadata(meta, metadata)
	}
}

func IsToolVisibleForTurnMetadata(meta ToolMetadata, metadata map[string]string) bool {
	switch {
	case opsManualsOptedOut(metadata):
		switch meta.Name {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			return false
		default:
			return true
		}
	case meta.Name == "resolve_ops_manual_params":
		return metadataBool(metadata, "opsManualMatched")
	case meta.Name == "run_ops_manual_preflight":
		return metadataBool(metadata, "opsManualParamsResolved") && metadataListContains(metadata, "enableTool", "run_ops_manual_preflight")
	default:
		return true
	}
}

func appendUniqueStrings(existing []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, current := range existing {
			if current == value {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, value)
		}
	}
	return existing
}

func metadataBool(metadata map[string]string, key string) bool {
	if len(metadata) == 0 {
		return false
	}
	value := strings.TrimSpace(metadata[key])
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

func metadataListContains(metadata map[string]string, key, want string) bool {
	for _, value := range metadataListValue(metadata, key) {
		if value == want {
			return true
		}
	}
	return false
}

func metadataListValue(metadata map[string]string, key string) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw := strings.TrimSpace(metadata[key])
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func isOpsManualToolName(name string) bool {
	switch name {
	case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
		return true
	default:
		return false
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
