package tooling

import (
	"sort"
	"strings"
)

// EffectiveDiscovery returns normalized generic discovery metadata.
func (m ToolMetadata) EffectiveDiscovery() ToolDiscoveryMetadata {
	d := m.Discovery
	d.DiscoveryGroup = strings.TrimSpace(d.DiscoveryGroup)
	d.CapabilityKind = normalizeDiscoveryToken(d.CapabilityKind)
	d.DiscoveryTags = normalizeDiscoveryList(d.DiscoveryTags)
	d.ResourceTypes = normalizeDiscoveryList(d.ResourceTypes)
	d.OperationKinds = normalizeDiscoveryList(d.OperationKinds)
	d.SupersedesShellHints = normalizeDiscoveryList(d.SupersedesShellHints)

	if d.CapabilityKind == "" {
		switch {
		case m.Mutating:
			d.CapabilityKind = "write"
		case m.RiskLevel.Normalize().RequiresApproval():
			d.CapabilityKind = "execute"
		default:
			for _, op := range d.OperationKinds {
				switch op {
				case "read", "list", "search", "inspect", "query", "summarize":
					d.CapabilityKind = "read"
				case "write", "delete", "modify", "create", "update":
					d.CapabilityKind = "write"
				case "run", "execute":
					d.CapabilityKind = "execute"
				}
				if d.CapabilityKind != "" {
					break
				}
			}
		}
	}
	if d.CapabilityKind == "" {
		d.CapabilityKind = "read"
	}
	if m.Layer == ToolLayerInternal {
		d.HiddenFromDiscovery = true
		d.HiddenFromPrompt = true
	}
	return d
}

// ToolHiddenFromDiscovery reports whether a tool should be omitted from
// discovery listings such as tool_search.
func ToolHiddenFromDiscovery(meta ToolMetadata) bool {
	return meta.EffectiveDiscovery().HiddenFromDiscovery
}

// ToolHiddenFromPrompt reports whether a tool should be omitted from the
// prompt-visible tool index.
func ToolHiddenFromPrompt(meta ToolMetadata) bool {
	return meta.EffectiveDiscovery().HiddenFromPrompt
}

// ToolRequiresSelect reports whether the tool must be explicitly selected
// before full schema/call use is advertised to the model.
func ToolRequiresSelect(meta ToolMetadata) bool {
	d := meta.EffectiveDiscovery()
	if d.RequiresSelect {
		return true
	}
	if meta.Layer == ToolLayerDeferred || meta.DeferByDefault || meta.Pack != "" {
		return !meta.AlwaysLoad
	}
	return false
}

// ToolDiscoverySearchText returns a normalized metadata-first search corpus.
func ToolDiscoverySearchText(meta ToolMetadata) string {
	d := meta.EffectiveDiscovery()
	parts := []string{
		meta.Name,
		strings.Join(meta.Aliases, " "),
		meta.Description,
		meta.Domain,
		meta.SearchHint,
		d.DiscoveryGroup,
		d.CapabilityKind,
		strings.Join(d.DiscoveryTags, " "),
		strings.Join(d.ResourceTypes, " "),
		strings.Join(d.OperationKinds, " "),
		strings.Join(meta.Triggers, " "),
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func normalizeDiscoveryList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeDiscoveryToken(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeDiscoveryToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
