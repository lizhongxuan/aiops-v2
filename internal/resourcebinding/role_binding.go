package resourcebinding

import (
	"sort"
	"strings"
)

type RoleCandidateResource struct {
	Ref              ResourceRef
	Labels           []string
	BindingTraceHash string
}

type RoleBindingExtraction struct {
	Bindings        []ResourceRoleBinding `json:"bindings,omitempty"`
	RejectedReasons []string              `json:"rejectedReasons,omitempty"`
	Conflicts       []RoleBindingConflict `json:"conflicts,omitempty"`
}

type RoleBindingConflict struct {
	ResourceID string   `json:"resourceId,omitempty"`
	Role       string   `json:"role,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
	TraceHash  string   `json:"traceHash,omitempty"`
}

type RoleBindingResolutionStatus string

const (
	RoleBindingResolutionResolved  RoleBindingResolutionStatus = "resolved"
	RoleBindingResolutionNotFound  RoleBindingResolutionStatus = "not_found"
	RoleBindingResolutionAmbiguous RoleBindingResolutionStatus = "ambiguous"
	RoleBindingResolutionConflict  RoleBindingResolutionStatus = "conflict"
)

type RoleBindingResolution struct {
	Status        RoleBindingResolutionStatus `json:"status"`
	Role          string                      `json:"role,omitempty"`
	ResourceRef   ResourceRef                 `json:"resourceRef,omitempty"`
	Binding       ResourceRoleBinding         `json:"binding,omitempty"`
	CandidateRefs []ResourceRef               `json:"candidateRefs,omitempty"`
	Reason        string                      `json:"reason,omitempty"`
	TraceHash     string                      `json:"traceHash,omitempty"`
}

func ExtractRoleBindings(input string, resources []RoleCandidateResource, sourceTurnID string) RoleBindingExtraction {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" || len(resources) == 0 {
		return RoleBindingExtraction{}
	}
	var extraction RoleBindingExtraction
	for _, candidate := range resources {
		ref := NormalizeRef(candidate.Ref)
		if ref.Type == "" || ref.ID == "" {
			continue
		}
		labels := candidateLabels(candidate)
		if !anyLabelMentioned(text, labels) {
			continue
		}
		role := roleNearLabels(text, labels)
		if role == "" {
			extraction.RejectedReasons = append(extraction.RejectedReasons, "role_not_found:"+ref.ID)
			continue
		}
		extraction.Bindings = append(extraction.Bindings, NewRoleBinding(RoleBindingInput{
			ResourceRef:    ref,
			Role:           role,
			RoleAlias:      RoleAliases(role),
			SourceTurnID:   sourceTurnID,
			Confidence:     0.9,
			ConflictPolicy: "fail_closed",
		}))
	}
	extraction.Conflicts = DetectRoleBindingConflicts(extraction.Bindings)
	return extraction
}

func ResolveUniqueRoleBinding(bindings []ResourceRoleBinding, conflicts []RoleBindingConflict, role string) RoleBindingResolution {
	normalizedRole := NormalizeRole(role)
	if normalizedRole == "" {
		return newRoleBindingResolution(RoleBindingResolutionNotFound, normalizedRole, nil, "role_not_found")
	}
	for _, conflict := range conflicts {
		if NormalizeRole(conflict.Role) == normalizedRole {
			return newRoleBindingResolution(RoleBindingResolutionConflict, normalizedRole, nil, "role_conflict")
		}
	}
	matchesByRef := map[string]ResourceRoleBinding{}
	for _, binding := range bindings {
		if NormalizeRole(binding.Role) != normalizedRole {
			continue
		}
		ref := NormalizeRef(binding.ResourceRef)
		if ref.Type == "" || ref.ID == "" {
			continue
		}
		matchesByRef[ref.IdentityHash()] = binding
	}
	if len(matchesByRef) == 0 {
		return newRoleBindingResolution(RoleBindingResolutionNotFound, normalizedRole, nil, "role_binding_not_found")
	}
	matches := make([]ResourceRoleBinding, 0, len(matchesByRef))
	for _, binding := range matchesByRef {
		matches = append(matches, binding)
	}
	sort.Slice(matches, func(i, j int) bool {
		left := NormalizeRef(matches[i].ResourceRef)
		right := NormalizeRef(matches[j].ResourceRef)
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		return left.ID < right.ID
	})
	if len(matches) > 1 {
		return newRoleBindingResolution(RoleBindingResolutionAmbiguous, normalizedRole, matches, "role_binding_ambiguous")
	}
	resolution := newRoleBindingResolution(RoleBindingResolutionResolved, normalizedRole, matches, "")
	resolution.Binding = matches[0]
	resolution.ResourceRef = NormalizeRef(matches[0].ResourceRef)
	return resolution
}

func RoleCandidatesFromBindings(bindings []ResourceBindingSnapshot) []RoleCandidateResource {
	var out []RoleCandidateResource
	for _, binding := range bindings {
		if !binding.Verified() {
			continue
		}
		out = append(out, RoleCandidateResource{
			Ref:              binding.Ref,
			Labels:           []string{binding.Ref.ID, binding.Ref.DisplayName},
			BindingTraceHash: binding.TraceHash,
		})
	}
	return out
}

func DetectRoleBindingConflicts(bindings []ResourceRoleBinding) []RoleBindingConflict {
	var conflicts []RoleBindingConflict
	byResource := map[string][]ResourceRoleBinding{}
	byRole := map[string][]ResourceRoleBinding{}
	for _, binding := range bindings {
		ref := NormalizeRef(binding.ResourceRef)
		if ref.ID == "" {
			continue
		}
		role := NormalizeRole(binding.Role)
		byResource[ref.IdentityHash()] = append(byResource[ref.IdentityHash()], binding)
		byRole[role] = append(byRole[role], binding)
	}
	for _, items := range byResource {
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				if RolesConflict(items[i].Role, items[j].Role) {
					conflicts = append(conflicts, newRoleBindingConflict(items[i].ResourceRef.ID, NormalizeRole(items[i].Role), "mutually_exclusive_roles"))
				}
			}
		}
	}
	for role, items := range byRole {
		if !RoleUniqueInTargetSet(role) || len(items) <= 1 {
			continue
		}
		conflicts = append(conflicts, newRoleBindingConflict("", role, "unique_role_bound_to_multiple_resources"))
	}
	return conflicts
}

func newRoleBindingConflict(resourceID, role, reason string) RoleBindingConflict {
	conflict := RoleBindingConflict{
		ResourceID: strings.TrimSpace(resourceID),
		Role:       NormalizeRole(role),
		Reasons:    []string{strings.TrimSpace(reason)},
	}
	conflict.TraceHash = StableTraceHash("resource-role-binding.conflict", conflict)
	return conflict
}

func candidateLabels(candidate RoleCandidateResource) []string {
	labels := append([]string(nil), candidate.Labels...)
	labels = append(labels, candidate.Ref.ID, candidate.Ref.DisplayName)
	for i, label := range labels {
		labels[i] = strings.Trim(strings.ToLower(strings.TrimSpace(label)), "@")
	}
	return uniqueSorted(labels)
}

func anyLabelMentioned(text string, labels []string) bool {
	for _, label := range labels {
		if label != "" && strings.Contains(text, strings.ToLower(label)) {
			return true
		}
	}
	return false
}

func roleNearLabels(text string, labels []string) string {
	bestIdx := len(text) + 1
	bestRole := ""
	for _, label := range labels {
		labelIdx := strings.Index(text, strings.ToLower(label))
		if labelIdx < 0 {
			continue
		}
		segment := roleSegmentAfterLabel(text, labelIdx)
		for alias, role := range roleAliases {
			idx := strings.Index(segment, alias)
			if idx >= 0 && idx < bestIdx {
				bestIdx = idx
				bestRole = role
			}
		}
	}
	if bestRole != "" {
		return bestRole
	}
	for alias, role := range roleAliases {
		idx := strings.Index(text, alias)
		if idx < 0 {
			continue
		}
		for _, label := range labels {
			labelIdx := strings.Index(text, strings.ToLower(label))
			if labelIdx < 0 {
				continue
			}
			delta := idx - labelIdx
			if delta < 0 {
				delta = -delta
			}
			if delta < bestIdx {
				bestIdx = delta
				bestRole = role
			}
		}
	}
	return bestRole
}

func roleSegmentAfterLabel(text string, labelIdx int) string {
	if labelIdx < 0 || labelIdx >= len(text) {
		return ""
	}
	end := len(text)
	for _, sep := range []string{"，", ",", "。", ";", "；", "\n"} {
		if idx := strings.Index(text[labelIdx:], sep); idx >= 0 && labelIdx+idx < end {
			end = labelIdx + idx
		}
	}
	return text[labelIdx:end]
}

func newRoleBindingResolution(status RoleBindingResolutionStatus, role string, matches []ResourceRoleBinding, reason string) RoleBindingResolution {
	resolution := RoleBindingResolution{
		Status: status,
		Role:   NormalizeRole(role),
		Reason: strings.TrimSpace(reason),
	}
	for _, match := range matches {
		ref := NormalizeRef(match.ResourceRef)
		if ref.Type == "" || ref.ID == "" {
			continue
		}
		resolution.CandidateRefs = append(resolution.CandidateRefs, ref)
	}
	resolution.TraceHash = StableTraceHash("resource-role-binding.resolution", resolution.tracePayload())
	return resolution
}

func (r RoleBindingResolution) tracePayload() map[string]any {
	return map[string]any{
		"status":        r.Status,
		"role":          r.Role,
		"resourceRef":   NormalizeRef(r.ResourceRef),
		"candidateRefs": r.CandidateRefs,
		"reason":        r.Reason,
	}
}
