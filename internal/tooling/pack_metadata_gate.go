package tooling

import (
	"strings"
	"unicode"
)

func ToolPackAllowedByMetadata(metadata map[string]string, pack string) bool {
	key := ToolPackAllowedMetadataKey(pack)
	if key == "" {
		return true
	}
	value, ok := metadata[key]
	if !ok {
		return true
	}
	return packMetadataBool(value)
}

func ToolAllowedByPackMetadata(metadata map[string]string, meta ToolMetadata) bool {
	pack := strings.TrimSpace(meta.Pack)
	if pack == "" {
		return true
	}
	return ToolPackAllowedByMetadata(metadata, pack)
}

func ToolPackAllowedMetadataKey(pack string) string {
	normalized := normalizeToolPackMetadataName(pack)
	if normalized == "" {
		return ""
	}
	return "aiops.toolPack." + normalized + ".allowed"
}

func FilterToolsByPackMetadata(tools []Tool, metadata map[string]string) []Tool {
	if len(tools) == 0 || len(metadata) == 0 {
		return tools
	}
	out := tools[:0]
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		pack := strings.TrimSpace(tool.Metadata().Pack)
		if pack != "" && !ToolPackAllowedByMetadata(metadata, pack) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func normalizeToolPackMetadataName(pack string) string {
	pack = strings.ToLower(strings.TrimSpace(pack))
	if pack == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range pack {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func packMetadataBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "allow", "allowed", "enabled":
		return true
	default:
		return false
	}
}
