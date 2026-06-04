package appui

import "testing"

func TestNewAiopsTransportStateInitializesHostOpsMaps(t *testing.T) {
	state := NewAiopsTransportState("sess-1", "thread-1")
	if state.HostMissions == nil {
		t.Fatalf("HostMissions is nil")
	}
	if state.ChildAgents == nil {
		t.Fatalf("ChildAgents is nil")
	}
}

func TestAiopsTransportStateSerializesHostMission(t *testing.T) {
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.HostMissions["mission-1"] = AiopsTransportHostMission{
		ID:           "mission-1",
		TurnID:       "turn-1",
		Status:       "planning",
		PlanRequired: true,
		MentionedHosts: []AiopsTransportHostMention{
			{
				TokenID:     "hm-1",
				Raw:         "@1.1.1.1",
				HostID:      "host-a",
				Address:     "1.1.1.1",
				DisplayName: "1.1.1.1",
				Source:      "inventory",
				Resolved:    true,
			},
		},
	}
	if state.HostMissions["mission-1"].MentionedHosts[0].HostID != "host-a" {
		t.Fatalf("state.HostMissions = %#v, want host-a", state.HostMissions)
	}
}
