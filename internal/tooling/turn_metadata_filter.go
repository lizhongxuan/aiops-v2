package tooling

import "strings"

func AssembleOptionsForTurnMetadata(metadata map[string]string) AssembleOptions {
	return ApplyTurnMetadataToAssembleOptions(AssembleOptions{}, metadata)
}

func ApplyTurnMetadataToAssembleOptions(opts AssembleOptions, metadata map[string]string) AssembleOptions {
	opts.TenantID = firstMetadataString(metadata, "tenantId", "tenantID", "tenant_id")
	opts.UserID = firstMetadataString(metadata, "userId", "userID", "user_id")
	if opts.Profile == "" {
		opts.Profile = firstMetadataString(metadata, "profile", "toolProfile", "mcpProfile")
	}
	opts.EnabledPacks = appendUniqueStrings(opts.EnabledPacks, metadataListValue(metadata, "enableToolPack")...)
	opts.EnabledTools = appendUniqueStrings(opts.EnabledTools, metadataListValue(metadata, "enableTool")...)
	opts.RuntimeCapabilities = appendUniqueStrings(opts.RuntimeCapabilities, metadataListValue(metadata, "runtimeCapability")...)
	opts.RuntimeCapabilities = appendUniqueStrings(opts.RuntimeCapabilities, metadataListValue(metadata, "runtimeCapabilities")...)
	opts.ContextArtifactAvailable = opts.ContextArtifactAvailable ||
		metadataBool(metadata, "contextArtifactAvailable") ||
		metadataBool(metadata, "hasContextArtifact") ||
		metadataBool(metadata, "contextArtifactEnabled")
	opts.MCPHealthSnapshot = mergeMCPHealthSnapshot(opts.MCPHealthSnapshot, metadata)
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

func mergeMCPHealthSnapshot(existing map[string]string, metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return existing
	}
	out := existing
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		const prefix = "mcpHealth."
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		serverID := strings.TrimSpace(strings.TrimPrefix(key, prefix))
		status := strings.TrimSpace(value)
		if serverID == "" || status == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[serverID] = status
	}
	return out
}

func firstMetadataString(metadata map[string]string, keys ...string) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
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
	case opsManualReferenceOnly(metadata) && meta.Name == "run_ops_manual_preflight":
		return false
	case meta.Name == "resolve_ops_manual_params":
		return metadataBool(metadata, "opsManualMatched") || opsManualParamFormSubmitted(metadata)
	case meta.Name == "run_ops_manual_preflight":
		return (metadataBool(metadata, "opsManualParamsResolved") && metadataListContains(metadata, "enableTool", "run_ops_manual_preflight")) ||
			metadataBool(metadata, "opsManualDirectExecute")
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

func opsManualReferenceOnly(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "reference_ops_manual")
}

func opsManualParamFormSubmitted(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "submit_ops_manual_param_form")
}
