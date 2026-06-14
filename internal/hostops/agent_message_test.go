package hostops

import (
	"context"
	"testing"
)

func TestAgentMessageStoreAppendsAndListsByMission(t *testing.T) {
	store := NewInMemoryAgentMessageStore()
	msg, err := store.Append(context.Background(), AgentMessage{
		MissionID:     "mission-msg",
		FromAgentID:   "manager",
		ToAgentID:     "child-a",
		Type:          AgentMessageHostSubTaskAssigned,
		CorrelationID: "corr-1",
		Payload: HostSubTaskAssignedPayload{
			SubTaskID:         "subtask-a",
			RuntimeContextRef: "ctx-ref-a",
		},
	})
	if err != nil {
		t.Fatalf("Append error = %v", err)
	}
	if msg.ID == "" || msg.CreatedAt.IsZero() || msg.PayloadDigest == "" {
		t.Fatalf("message = %#v, want id/time/digest", msg)
	}
	got, err := store.ListByMission(context.Background(), "mission-msg")
	if err != nil {
		t.Fatalf("ListByMission error = %v", err)
	}
	if len(got) != 1 || got[0].ID != msg.ID || got[0].Type != AgentMessageHostSubTaskAssigned {
		t.Fatalf("messages = %#v, want appended message", got)
	}
}

func TestHostSubTaskAssignedCarriesRuntimeContextRef(t *testing.T) {
	payload := HostSubTaskAssignedPayload{
		SubTaskID:              "subtask-a",
		RuntimeContextRef:      "host-context:mission:host-a:step-a",
		ContextDecisionTraceID: "context-trace-a",
		SourcePlanStepID:       "step-a",
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestHostReportMessageCarriesValidatedReport(t *testing.T) {
	report := HostTaskReport{MissionID: "mission-msg", PlanStepID: "step-a", HostAgentID: "child-a", HostID: "host-a", Status: string(HostTaskReportStatusCompleted)}
	payload := HostTaskReportMessagePayload{ReportRef: "report-ref-a", Report: report, ValidationState: "validated"}
	if err := payload.Validate(); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestAgentMessagesAreReplayableInOrder(t *testing.T) {
	store := NewInMemoryAgentMessageStore()
	for _, typ := range []AgentMessageType{
		AgentMessageHostSubTaskAssigned,
		AgentMessageHostReportProgress,
		AgentMessageHostReportCompleted,
	} {
		if _, err := store.Append(context.Background(), AgentMessage{
			MissionID:     "mission-msg",
			FromAgentID:   "agent-a",
			ToAgentID:     "agent-b",
			Type:          typ,
			CorrelationID: "corr-1",
			Payload:       map[string]string{"type": string(typ)},
		}); err != nil {
			t.Fatalf("Append %s error = %v", typ, err)
		}
	}
	got, err := store.Replay(context.Background(), "mission-msg")
	if err != nil {
		t.Fatalf("Replay error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(replay) = %d, want 3", len(got))
	}
	for i, want := range []AgentMessageType{AgentMessageHostSubTaskAssigned, AgentMessageHostReportProgress, AgentMessageHostReportCompleted} {
		if got[i].Type != want {
			t.Fatalf("replay[%d].Type = %s, want %s", i, got[i].Type, want)
		}
	}
	got[0].MissionID = "mutated"
	again, err := store.Replay(context.Background(), "mission-msg")
	if err != nil {
		t.Fatalf("Replay again error = %v", err)
	}
	if again[0].MissionID != "mission-msg" {
		t.Fatalf("store returned mutable history: %#v", again[0])
	}
}
