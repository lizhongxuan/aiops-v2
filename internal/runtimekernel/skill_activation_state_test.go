package runtimekernel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSkillActivationStateApplySearchAndRead(t *testing.T) {
	var state SkillActivationSessionState
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	state.ApplySearch([]SkillSearchMatchSnapshot{{
		Name:          "synthetic.triage",
		ResourceTypes: []string{"log"},
		TaskIntents:   []string{"diagnose"},
	}}, "sha256:index", now)
	state.ApplyRead(SkillReadDelta{LoadedSkills: []LoadedSkillRef{{
		Name:   "synthetic.triage",
		Source: "skill_read",
		Reason: "Need relevant checklist",
		Range:  SkillReadRange{Offset: 0, Limit: 128},
		Hash:   "sha256:body",
	}}}, now)

	if state.SkillIndexHash != "sha256:index" {
		t.Fatalf("SkillIndexHash = %q", state.SkillIndexHash)
	}
	if len(state.LastSearchResults) != 1 || state.LastSearchResults[0].Name != "synthetic.triage" {
		t.Fatalf("LastSearchResults = %+v", state.LastSearchResults)
	}
	if got := state.EnabledSkills(); len(got) != 1 || got[0] != "synthetic.triage" {
		t.Fatalf("EnabledSkills = %v", got)
	}
	if state.LoadedSkills["synthetic.triage"].LoadedAt.IsZero() {
		t.Fatalf("LoadedSkills missing LoadedAt: %+v", state.LoadedSkills)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplySkillDiscoveryStateFromToolResults(t *testing.T) {
	session := &SessionState{ID: "session-1", Type: SessionTypeHost, Mode: ModeChat}
	searchPayload := json.RawMessage(`{
		"schemaVersion":"aiops.skill_discovery/v1",
		"mode":"search",
		"skillIndexHash":"sha256:index",
		"matches":[{"name":"synthetic.triage","resourceTypes":["log"],"taskIntents":["diagnose"]}]
	}`)
	applySkillDiscoveryState(session, "skill_search", ToolResult{
		Content: string(searchPayload),
		Display: &ToolDisplayPayload{Type: "skill_search", Data: searchPayload},
	}, "turn-1")

	readPayload := json.RawMessage(`{
		"schemaVersion":"aiops.skill_discovery/v1",
		"skill":"synthetic.triage",
		"loadedSkills":[{"name":"synthetic.triage","source":"skill_read","reason":"Need relevant checklist","range":{"offset":0,"limit":128},"hash":"sha256:body"}]
	}`)
	applySkillDiscoveryState(session, "skill_read", ToolResult{
		Content: string(readPayload),
		Display: &ToolDisplayPayload{Type: "skill_read", Data: readPayload},
	}, "turn-1")

	if session.SkillActivation.SkillIndexHash != "sha256:index" {
		t.Fatalf("SkillIndexHash = %q", session.SkillActivation.SkillIndexHash)
	}
	if got := session.SkillActivation.EnabledSkills(); len(got) != 1 || got[0] != "synthetic.triage" {
		t.Fatalf("EnabledSkills = %v", got)
	}
}

func TestActiveSkillToolPoliciesFromLoadedSkills(t *testing.T) {
	session := &SessionState{SkillActivation: SkillActivationSessionState{LoadedSkills: map[string]LoadedSkillRef{
		"synthetic.triage": {
			Name:         "synthetic.triage",
			AllowedTools: []string{"synthetic.read"},
			DeniedTools:  []string{"synthetic.write"},
			RiskCeiling:  "low",
		},
	}}}

	policies := activeSkillToolPolicies(session)
	if len(policies) != 1 {
		t.Fatalf("policies = %+v", policies)
	}
	if policies[0].AllowedTools[0] != "synthetic.read" || policies[0].DeniedTools[0] != "synthetic.write" || policies[0].RiskCeiling != "low" {
		t.Fatalf("policy = %+v", policies[0])
	}
}
