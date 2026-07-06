package agentassembly

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

type ToolSurfaceSnapshot struct {
	Lifecycle         Lifecycle         `json:"lifecycle,omitempty"`
	RegisteredTools   []ToolSurfaceItem `json:"registeredTools,omitempty"`
	ModelVisibleTools []ToolSurfaceItem `json:"modelVisibleTools,omitempty"`
	DispatchableTools []ToolSurfaceItem `json:"dispatchableTools,omitempty"`
	HiddenTools       []ToolSurfaceItem `json:"hiddenTools,omitempty"`
	PolicyHash        string            `json:"policyHash,omitempty"`
	Fingerprint       string            `json:"fingerprint,omitempty"`
	Hash              string            `json:"hash,omitempty"`
}

type ToolSurfaceItem struct {
	Name                string `json:"name,omitempty"`
	Namespace           string `json:"namespace,omitempty"`
	DescriptionHash     string `json:"descriptionHash,omitempty"`
	ResourceBindingHash string `json:"resourceBindingHash,omitempty"`
	Capability          string `json:"capability,omitempty"`
	RequiresApproval    bool   `json:"requiresApproval,omitempty"`
	PolicyHash          string `json:"policyHash,omitempty"`
	HiddenReason        string `json:"hiddenReason,omitempty"`
}

type HiddenToolInput struct {
	Name   string
	Reason string
}

type ToolSurfaceInput struct {
	ResourceBindings  []resourcebinding.ResourceBindingSnapshot
	RegisteredTools   []tooling.ToolMetadata
	ModelVisibleTools []tooling.ToolMetadata
	DispatchableTools []tooling.ToolMetadata
	HiddenTools       []HiddenToolInput
	PolicyHash        string
	Fingerprint       string
}

func BuildToolSurfaceSnapshot(input ToolSurfaceInput) ToolSurfaceSnapshot {
	snapshot := ToolSurfaceSnapshot{
		Lifecycle:         LifecycleDispatchScope,
		RegisteredTools:   toolSurfaceItemsFromMetadata(input.RegisteredTools, input.ResourceBindings, input.PolicyHash, ""),
		ModelVisibleTools: toolSurfaceItemsFromMetadata(input.ModelVisibleTools, input.ResourceBindings, input.PolicyHash, ""),
		DispatchableTools: toolSurfaceItemsFromMetadata(input.DispatchableTools, input.ResourceBindings, input.PolicyHash, ""),
		HiddenTools:       hiddenToolSurfaceItems(input.HiddenTools),
		PolicyHash:        strings.TrimSpace(input.PolicyHash),
		Fingerprint:       strings.TrimSpace(input.Fingerprint),
	}
	if len(snapshot.RegisteredTools) == 0 {
		snapshot.RegisteredTools = append([]ToolSurfaceItem(nil), snapshot.DispatchableTools...)
	}
	if len(snapshot.DispatchableTools) == 0 {
		snapshot.DispatchableTools = append([]ToolSurfaceItem(nil), snapshot.ModelVisibleTools...)
	}
	if snapshot.Fingerprint == "" {
		snapshot.Fingerprint = StableHash("tool-surface.fingerprint", map[string]any{
			"visible":      snapshot.ModelVisibleTools,
			"dispatchable": snapshot.DispatchableTools,
			"hidden":       snapshot.HiddenTools,
			"policyHash":   snapshot.PolicyHash,
		})
	}
	snapshot.Hash = StableHash("tool-surface.snapshot", map[string]any{
		"registered":   snapshot.RegisteredTools,
		"visible":      snapshot.ModelVisibleTools,
		"dispatchable": snapshot.DispatchableTools,
		"hidden":       snapshot.HiddenTools,
		"policyHash":   snapshot.PolicyHash,
		"fingerprint":  snapshot.Fingerprint,
	})
	return snapshot
}

func (s ToolSurfaceSnapshot) Validate() error {
	dispatchable := map[string]struct{}{}
	for _, item := range s.DispatchableTools {
		if key := toolIdentityKey(item); key != "" {
			dispatchable[key] = struct{}{}
		}
	}
	for _, item := range s.ModelVisibleTools {
		key := toolIdentityKey(item)
		if key == "" {
			continue
		}
		if _, ok := dispatchable[key]; !ok {
			return fmt.Errorf("model visible tool %q is not dispatchable in snapshot", key)
		}
	}
	for _, item := range s.HiddenTools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		reason := strings.TrimSpace(item.HiddenReason)
		if reason == "" {
			return fmt.Errorf("hidden tool %q missing hidden reason", name)
		}
		if !tooling.IsKnownToolHiddenReason(reason) {
			return fmt.Errorf("hidden tool %q has unknown hidden reason %q", name, reason)
		}
	}
	return nil
}

func toolSurfaceItemsFromMetadata(metas []tooling.ToolMetadata, bindings []resourcebinding.ResourceBindingSnapshot, policyHash, hiddenReason string) []ToolSurfaceItem {
	if len(metas) == 0 {
		return nil
	}
	out := make([]ToolSurfaceItem, 0, len(metas))
	for _, meta := range metas {
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			continue
		}
		input, _ := resourcebinding.ToolCapabilityInputFromMetadata(meta, policyHash)
		item := ToolSurfaceItem{
			Name:             name,
			Namespace:        toolNamespace(meta),
			DescriptionHash:  StableHash("tool-description", strings.TrimSpace(meta.Description)),
			Capability:       input.Capability,
			RequiresApproval: input.RequiresApproval,
			PolicyHash:       strings.TrimSpace(policyHash),
			HiddenReason:     strings.TrimSpace(hiddenReason),
		}
		if item.Capability == "" && meta.Mutating {
			item.Capability = resourcebinding.CapabilityMutate
			item.RequiresApproval = true
		}
		item.ResourceBindingHash = resourceBindingHashForTool(meta, bindings)
		out = append(out, item)
	}
	sortToolSurfaceItems(out)
	return out
}

func hiddenToolSurfaceItems(inputs []HiddenToolInput) []ToolSurfaceItem {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]ToolSurfaceItem, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		out = append(out, ToolSurfaceItem{
			Name:         name,
			HiddenReason: strings.TrimSpace(input.Reason),
		})
	}
	sortToolSurfaceItems(out)
	return out
}

func resourceBindingHashForTool(meta tooling.ToolMetadata, bindings []resourcebinding.ResourceBindingSnapshot) string {
	if len(bindings) == 0 {
		return ""
	}
	resourceTypes := meta.EffectiveDiscovery().ResourceTypes
	for _, binding := range bindings {
		if !binding.Verified() {
			continue
		}
		if len(resourceTypes) == 0 || containsString(resourceTypes, binding.Ref.Type) {
			return strings.TrimSpace(binding.TraceHash)
		}
	}
	return ""
}

func toolNamespace(meta tooling.ToolMetadata) string {
	for _, value := range []string{meta.Pack, meta.Domain, string(meta.Layer), string(meta.Origin)} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "default"
}

func sortToolSurfaceItems(items []ToolSurfaceItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
}

func toolIdentityKey(item ToolSurfaceItem) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(item.Namespace) + "/" + name
}

func containsString(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
