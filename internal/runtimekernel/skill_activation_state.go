package runtimekernel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxSkillActivationHistory = 20

type SkillReadRange struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

type LoadedSkillRef struct {
	Name         string         `json:"name"`
	Source       string         `json:"source,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Range        SkillReadRange `json:"range,omitempty"`
	Hash         string         `json:"hash,omitempty"`
	RiskCeiling  string         `json:"riskCeiling,omitempty"`
	AllowedTools []string       `json:"allowedTools,omitempty"`
	DeniedTools  []string       `json:"deniedTools,omitempty"`
	LoadedAt     time.Time      `json:"loadedAt,omitempty"`
}

type SkillSearchMatchSnapshot struct {
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	WhenToUse        string    `json:"whenToUse,omitempty"`
	ResourceTypes    []string  `json:"resourceTypes,omitempty"`
	TaskIntents      []string  `json:"taskIntents,omitempty"`
	Risk             string    `json:"risk,omitempty"`
	RequiresRead     bool      `json:"requiresRead,omitempty"`
	RequiredForMatch bool      `json:"requiredForMatch,omitempty"`
	SeenAt           time.Time `json:"seenAt,omitempty"`
}

type RejectedSkillActivation struct {
	SkillName      string    `json:"skillName,omitempty"`
	Reason         string    `json:"reason"`
	RequiredAction string    `json:"requiredAction,omitempty"`
	TurnID         string    `json:"turnId,omitempty"`
	RejectedAt     time.Time `json:"rejectedAt,omitempty"`
}

type SkillReadDelta struct {
	LoadedSkills []LoadedSkillRef `json:"loadedSkills,omitempty"`
	Reason       string           `json:"reason,omitempty"`
}

type SkillActivationSessionState struct {
	LoadedSkills        map[string]LoadedSkillRef  `json:"loadedSkills,omitempty"`
	LastSearchResults   []SkillSearchMatchSnapshot `json:"lastSearchResults,omitempty"`
	RejectedActivations []RejectedSkillActivation  `json:"rejectedActivations,omitempty"`
	SkillIndexHash      string                     `json:"skillIndexHash,omitempty"`
	UpdatedAt           time.Time                  `json:"updatedAt,omitempty"`
}

func (s *SkillActivationSessionState) ApplySearch(matches []SkillSearchMatchSnapshot, indexHash string, now time.Time) {
	if s == nil {
		return
	}
	cloned := make([]SkillSearchMatchSnapshot, 0, len(matches))
	for _, match := range matches {
		match.Name = strings.TrimSpace(match.Name)
		if match.Name == "" {
			continue
		}
		match.ResourceTypes = cloneSortedStrings(match.ResourceTypes)
		match.TaskIntents = cloneSortedStrings(match.TaskIntents)
		if match.SeenAt.IsZero() {
			match.SeenAt = now
		}
		cloned = append(cloned, match)
	}
	if len(cloned) > maxSkillActivationHistory {
		cloned = cloned[:maxSkillActivationHistory]
	}
	s.LastSearchResults = cloned
	s.SkillIndexHash = strings.TrimSpace(indexHash)
	s.UpdatedAt = now
}

func (s *SkillActivationSessionState) ApplyRead(delta SkillReadDelta, now time.Time) {
	if s == nil {
		return
	}
	if s.LoadedSkills == nil && len(delta.LoadedSkills) > 0 {
		s.LoadedSkills = make(map[string]LoadedSkillRef, len(delta.LoadedSkills))
	}
	for _, ref := range delta.LoadedSkills {
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			ref.Source = "skill_read"
		}
		if ref.Reason == "" {
			ref.Reason = delta.Reason
		}
		if ref.LoadedAt.IsZero() {
			ref.LoadedAt = now
		}
		ref.AllowedTools = cloneSortedStrings(ref.AllowedTools)
		ref.DeniedTools = cloneSortedStrings(ref.DeniedTools)
		s.LoadedSkills[ref.Name] = ref
	}
	if len(delta.LoadedSkills) > 0 {
		s.UpdatedAt = now
	}
}

func (s *SkillActivationSessionState) AddRejectedActivation(rejected RejectedSkillActivation, now time.Time) {
	if s == nil {
		return
	}
	rejected.SkillName = strings.TrimSpace(rejected.SkillName)
	rejected.Reason = strings.TrimSpace(rejected.Reason)
	if rejected.Reason == "" {
		return
	}
	if rejected.RejectedAt.IsZero() {
		rejected.RejectedAt = now
	}
	s.RejectedActivations = append([]RejectedSkillActivation{rejected}, s.RejectedActivations...)
	if len(s.RejectedActivations) > maxSkillActivationHistory {
		s.RejectedActivations = s.RejectedActivations[:maxSkillActivationHistory]
	}
	s.UpdatedAt = now
}

func (s SkillActivationSessionState) EnabledSkills() []string {
	out := make([]string, 0, len(s.LoadedSkills))
	for name := range s.LoadedSkills {
		if strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (s SkillActivationSessionState) Validate() error {
	for key, ref := range s.LoadedSkills {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("loaded skill %q missing name", key)
		}
	}
	for i, rejected := range s.RejectedActivations {
		if strings.TrimSpace(rejected.Reason) == "" {
			return fmt.Errorf("rejected skill activation[%d] missing reason", i)
		}
	}
	return nil
}

func applySkillDiscoveryState(session *SessionState, toolName string, result ToolResult, turnID string) {
	if session == nil || !isSkillDiscoveryToolResult(toolName, result) {
		return
	}
	data := []byte(result.Content)
	if result.Display != nil && len(result.Display.Data) > 0 {
		data = result.Display.Data
	}
	now := time.Now()
	switch skillDiscoveryResultType(toolName, result) {
	case "skill_search":
		var payload struct {
			SkillIndexHash string                     `json:"skillIndexHash"`
			Matches        []SkillSearchMatchSnapshot `json:"matches"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		session.SkillActivation.ApplySearch(payload.Matches, payload.SkillIndexHash, now)
	case "skill_read":
		var payload struct {
			LoadedSkills []LoadedSkillRef `json:"loadedSkills"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		session.SkillActivation.ApplyRead(SkillReadDelta{LoadedSkills: payload.LoadedSkills}, now)
	}
	_ = turnID
}

func isSkillDiscoveryToolResult(toolName string, result ToolResult) bool {
	switch strings.TrimSpace(toolName) {
	case "skill_search", "skill_read":
		return true
	}
	return result.Display != nil && (result.Display.Type == "skill_search" || result.Display.Type == "skill_read")
}

func skillDiscoveryResultType(toolName string, result ToolResult) string {
	if result.Display != nil && strings.TrimSpace(result.Display.Type) != "" {
		return strings.TrimSpace(result.Display.Type)
	}
	return strings.TrimSpace(toolName)
}
