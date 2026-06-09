package runtimekernel

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxToolDiscoveryHistory = 20

type LoadedToolRef struct {
	Name        string    `json:"name"`
	Pack        string    `json:"pack,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Source      string    `json:"source,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	LoadedAt    time.Time `json:"loadedAt,omitempty"`
}

type LoadedPackRef struct {
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Source      string    `json:"source,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	LoadedAt    time.Time `json:"loadedAt,omitempty"`
}

type ToolSearchMatchSnapshot struct {
	Kind             string    `json:"kind,omitempty"`
	Name             string    `json:"name"`
	Pack             string    `json:"pack,omitempty"`
	Tools            []string  `json:"tools,omitempty"`
	CapabilityKind   string    `json:"capabilityKind,omitempty"`
	ResourceTypes    []string  `json:"resourceTypes,omitempty"`
	OperationKinds   []string  `json:"operationKinds,omitempty"`
	RiskLevel        string    `json:"riskLevel,omitempty"`
	Mutating         bool      `json:"mutating,omitempty"`
	RequiresApproval bool      `json:"requiresApproval,omitempty"`
	RequiresSelect   bool      `json:"requiresSelect,omitempty"`
	SeenAt           time.Time `json:"seenAt,omitempty"`
}

type DeferredToolRejectedCall struct {
	ToolName             string    `json:"toolName"`
	ErrorType            string    `json:"errorType"`
	Reason               string    `json:"reason"`
	RequiredAction       string    `json:"requiredAction,omitempty"`
	SuggestedSearchQuery string    `json:"suggestedSearchQuery,omitempty"`
	TurnID               string    `json:"turnId,omitempty"`
	ToolCallID           string    `json:"toolCallId,omitempty"`
	RejectedAt           time.Time `json:"rejectedAt,omitempty"`
}

type ToolSelectionDelta struct {
	LoadedTools []LoadedToolRef `json:"loadedTools,omitempty"`
	LoadedPacks []LoadedPackRef `json:"loadedPacks,omitempty"`
	NotLoaded   []string        `json:"notLoaded,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

type ToolCatalogSnapshot struct {
	Tools map[string]string `json:"tools,omitempty"`
	Packs map[string]string `json:"packs,omitempty"`
}

type ToolDiscoveryInvalidation struct {
	InvalidatedTools []string `json:"invalidatedTools,omitempty"`
	InvalidatedPacks []string `json:"invalidatedPacks,omitempty"`
}

type ToolDiscoverySessionState struct {
	LoadedTools         map[string]LoadedToolRef   `json:"loadedTools,omitempty"`
	LoadedPacks         map[string]LoadedPackRef   `json:"loadedPacks,omitempty"`
	LastSearchResults   []ToolSearchMatchSnapshot  `json:"lastSearchResults,omitempty"`
	RejectedCalls       []DeferredToolRejectedCall `json:"rejectedCalls,omitempty"`
	ToolSurfaceRevision string                     `json:"toolSurfaceRevision,omitempty"`
	PolicySnapshotHash  string                     `json:"policySnapshotHash,omitempty"`
	UpdatedAt           time.Time                  `json:"updatedAt,omitempty"`
}

func (s *ToolDiscoverySessionState) ApplySelection(delta ToolSelectionDelta, now time.Time) {
	if s == nil {
		return
	}
	if s.LoadedTools == nil && len(delta.LoadedTools) > 0 {
		s.LoadedTools = make(map[string]LoadedToolRef, len(delta.LoadedTools))
	}
	if s.LoadedPacks == nil && len(delta.LoadedPacks) > 0 {
		s.LoadedPacks = make(map[string]LoadedPackRef, len(delta.LoadedPacks))
	}
	for _, ref := range delta.LoadedTools {
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			ref.Source = "tool_search.select"
		}
		if ref.Reason == "" {
			ref.Reason = delta.Reason
		}
		if ref.LoadedAt.IsZero() {
			ref.LoadedAt = now
		}
		s.LoadedTools[ref.Name] = ref
	}
	for _, ref := range delta.LoadedPacks {
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			ref.Source = "tool_search.select"
		}
		if ref.Reason == "" {
			ref.Reason = delta.Reason
		}
		if ref.LoadedAt.IsZero() {
			ref.LoadedAt = now
		}
		s.LoadedPacks[ref.Name] = ref
	}
	if len(delta.LoadedTools) > 0 || len(delta.LoadedPacks) > 0 {
		s.UpdatedAt = now
	}
}

func (s *ToolDiscoverySessionState) ApplySearch(matches []ToolSearchMatchSnapshot, now time.Time) {
	if s == nil {
		return
	}
	cloned := make([]ToolSearchMatchSnapshot, 0, len(matches))
	for _, match := range matches {
		match.Name = strings.TrimSpace(match.Name)
		if match.Name == "" {
			continue
		}
		if match.SeenAt.IsZero() {
			match.SeenAt = now
		}
		match.Tools = cloneSortedStrings(match.Tools)
		match.ResourceTypes = cloneSortedStrings(match.ResourceTypes)
		match.OperationKinds = cloneSortedStrings(match.OperationKinds)
		cloned = append(cloned, match)
	}
	if len(cloned) > maxToolDiscoveryHistory {
		cloned = cloned[:maxToolDiscoveryHistory]
	}
	s.LastSearchResults = cloned
	s.UpdatedAt = now
}

func (s *ToolDiscoverySessionState) AddRejectedCall(call DeferredToolRejectedCall, now time.Time) {
	if s == nil {
		return
	}
	call.ToolName = strings.TrimSpace(call.ToolName)
	call.ErrorType = strings.TrimSpace(call.ErrorType)
	call.Reason = strings.TrimSpace(call.Reason)
	if call.ToolName == "" || call.ErrorType == "" || call.Reason == "" {
		return
	}
	if call.RejectedAt.IsZero() {
		call.RejectedAt = now
	}
	s.RejectedCalls = append([]DeferredToolRejectedCall{call}, s.RejectedCalls...)
	if len(s.RejectedCalls) > maxToolDiscoveryHistory {
		s.RejectedCalls = s.RejectedCalls[:maxToolDiscoveryHistory]
	}
	s.UpdatedAt = now
}

func (s *ToolDiscoverySessionState) InvalidateMissing(current ToolCatalogSnapshot, now time.Time) ToolDiscoveryInvalidation {
	var report ToolDiscoveryInvalidation
	if s == nil {
		return report
	}
	for name, ref := range s.LoadedTools {
		currentFP, ok := current.Tools[name]
		if !ok || (ref.Fingerprint != "" && currentFP != "" && ref.Fingerprint != currentFP) {
			report.InvalidatedTools = append(report.InvalidatedTools, name)
			delete(s.LoadedTools, name)
		}
	}
	for name, ref := range s.LoadedPacks {
		currentFP, ok := current.Packs[name]
		if !ok || (ref.Fingerprint != "" && currentFP != "" && ref.Fingerprint != currentFP) {
			report.InvalidatedPacks = append(report.InvalidatedPacks, name)
			delete(s.LoadedPacks, name)
		}
	}
	sort.Strings(report.InvalidatedTools)
	sort.Strings(report.InvalidatedPacks)
	if len(report.InvalidatedTools) > 0 || len(report.InvalidatedPacks) > 0 {
		s.UpdatedAt = now
	}
	return report
}

func (s ToolDiscoverySessionState) EnabledPacks() []string {
	out := make([]string, 0, len(s.LoadedPacks))
	for name := range s.LoadedPacks {
		if strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (s ToolDiscoverySessionState) EnabledTools() []string {
	out := make([]string, 0, len(s.LoadedTools))
	for name := range s.LoadedTools {
		if strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (s ToolDiscoverySessionState) Validate() error {
	for key, ref := range s.LoadedTools {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("loaded tool %q missing name", key)
		}
	}
	for key, ref := range s.LoadedPacks {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("loaded pack %q missing name", key)
		}
	}
	for i, call := range s.RejectedCalls {
		if strings.TrimSpace(call.ToolName) == "" || strings.TrimSpace(call.ErrorType) == "" || strings.TrimSpace(call.Reason) == "" {
			return fmt.Errorf("rejected call[%d] incomplete", i)
		}
	}
	return nil
}

func cloneSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
