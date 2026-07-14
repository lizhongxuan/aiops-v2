package specialinputmemory

import (
	"sort"
	"strings"
	"time"
)

type RoleBindingInput struct {
	ResourceKind   string
	ResourceID     string
	Display        string
	RoleKey        string
	RuntimeName    string
	EnvironmentKey string
	ClusterKey     string
	Source         string
	SourceTurnID   string
	Confidence     float64
	Now            time.Time
}

func NewMentionRoleBinding(input RoleBindingInput) MentionRoleBinding {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	binding := MentionRoleBinding{
		RoleKey:         compactToken(input.RoleKey),
		RuntimeName:     strings.TrimSpace(input.RuntimeName),
		ResourceID:      strings.TrimSpace(input.ResourceID),
		ResourceKind:    firstNonEmpty(compactToken(input.ResourceKind), ResourceKindHost),
		Display:         strings.TrimSpace(input.Display),
		EnvironmentKey:  compactToken(input.EnvironmentKey),
		ClusterKey:      compactToken(input.ClusterKey),
		Source:          firstNonEmpty(normalizedSource(input.Source), SourceStructuredSelection),
		Status:          RoleBindingStatusActive,
		FirstSeenTurnID: strings.TrimSpace(input.SourceTurnID),
		LastSeenTurnID:  strings.TrimSpace(input.SourceTurnID),
		Confidence:      input.Confidence,
		ExpiresAt:       now.Add(roleBindingTTL),
	}
	if binding.Confidence <= 0 {
		binding.Confidence = 1
	}
	binding.BindingHash = RoleBindingHash(binding)
	binding.ID = stableHash("special-input-memory.role-binding.id", map[string]any{
		"hash": binding.BindingHash,
		"turn": binding.FirstSeenTurnID,
	})
	return binding
}

func RoleBindingHash(binding MentionRoleBinding) string {
	return stableHash("special-input-memory.role-binding", map[string]any{
		"schemaVersion":  SchemaVersion,
		"environmentKey": compactToken(binding.EnvironmentKey),
		"clusterKey":     compactToken(binding.ClusterKey),
		"roleKey":        compactToken(binding.RoleKey),
		"resourceKind":   compactToken(binding.ResourceKind),
		"resourceID":     strings.TrimSpace(binding.ResourceID),
		"runtimeName":    strings.TrimSpace(binding.RuntimeName),
	})
}

func DetectRoleBindingConflicts(bindings []MentionRoleBinding) []MemoryConflict {
	type key struct {
		env     string
		cluster string
		role    string
	}
	grouped := map[key]map[string]struct{}{}
	for _, binding := range bindings {
		if binding.Status != "" && binding.Status != RoleBindingStatusActive {
			continue
		}
		if compactToken(binding.RoleKey) == "" || strings.TrimSpace(binding.ResourceID) == "" {
			continue
		}
		k := key{env: compactToken(binding.EnvironmentKey), cluster: compactToken(binding.ClusterKey), role: compactToken(binding.RoleKey)}
		if grouped[k] == nil {
			grouped[k] = map[string]struct{}{}
		}
		grouped[k][strings.TrimSpace(binding.ResourceID)] = struct{}{}
	}
	var conflicts []MemoryConflict
	for k, resources := range grouped {
		if len(resources) <= 1 {
			continue
		}
		resourceIDs := make([]string, 0, len(resources))
		for resourceID := range resources {
			resourceIDs = append(resourceIDs, resourceID)
		}
		sort.Strings(resourceIDs)
		conflict := MemoryConflict{
			Kind:           "role_binding",
			RoleKey:        k.role,
			EnvironmentKey: k.env,
			ClusterKey:     k.cluster,
			ResourceIDs:    resourceIDs,
			Reasons:        []string{"unique_role_bound_to_multiple_resources"},
		}
		conflict.TraceHash = stableHash("special-input-memory.role-conflict", conflict)
		conflict.ID = conflict.TraceHash
		conflicts = append(conflicts, conflict)
	}
	sort.SliceStable(conflicts, func(i, j int) bool {
		return conflicts[i].ID < conflicts[j].ID
	})
	return conflicts
}

func upsertRoleBinding(bindings []MentionRoleBinding, next MentionRoleBinding) []MentionRoleBinding {
	for i := range bindings {
		if bindings[i].BindingHash == next.BindingHash {
			if bindings[i].FirstSeenTurnID != "" {
				next.FirstSeenTurnID = bindings[i].FirstSeenTurnID
			}
			bindings[i] = next
			return bindings
		}
	}
	return append(bindings, next)
}
