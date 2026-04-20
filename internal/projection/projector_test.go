package projection

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

// mockSubscriber records all projection callbacks for verification.
type mockSubscriber struct {
	toolInvocations []ToolInvocation
	activities      []ActivityStats
	cards           []Card
	approvals       []Approval
	evidences       []Evidence
	snapshots       []Snapshot
}

func (m *mockSubscriber) OnToolInvocation(inv ToolInvocation) {
	m.toolInvocations = append(m.toolInvocations, inv)
}
func (m *mockSubscriber) OnActivity(activity ActivityStats) {
	m.activities = append(m.activities, activity)
}
func (m *mockSubscriber) OnCard(card Card) {
	m.cards = append(m.cards, card)
}
func (m *mockSubscriber) OnApproval(approval Approval) {
	m.approvals = append(m.approvals, approval)
}
func (m *mockSubscriber) OnEvidence(evidence Evidence) {
	m.evidences = append(m.evidences, evidence)
}
func (m *mockSubscriber) OnSnapshot(snapshot Snapshot) {
	m.snapshots = append(m.snapshots, snapshot)
}

func TestNewProjector(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)
	if p == nil {
		t.Fatal("NewProjector returned nil")
	}
	if len(p.subscribers) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(p.subscribers))
	}
}

func TestAddSubscriber(t *testing.T) {
	p := NewProjector()
	sub := &mockSubscriber{}
	p.AddSubscriber(sub)
	if len(p.subscribers) != 1 {
		t.Fatalf("expected 1 subscriber after AddSubscriber, got %d", len(p.subscribers))
	}
}

func TestEmit_ToolStarted(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]string{
		"id":       "tool-1",
		"toolName": "host.disk_usage",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolStarted,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.toolInvocations) != 1 {
		t.Fatalf("expected 1 tool invocation, got %d", len(sub.toolInvocations))
	}
	inv := sub.toolInvocations[0]
	if inv.Status != ToolInvocationStarted {
		t.Errorf("expected status %q, got %q", ToolInvocationStarted, inv.Status)
	}
	if inv.ToolName != "host.disk_usage" {
		t.Errorf("expected toolName %q, got %q", "host.disk_usage", inv.ToolName)
	}
	if inv.ID != "tool-1" {
		t.Errorf("expected id %q, got %q", "tool-1", inv.ID)
	}
	if inv.SessionID != "sess-1" {
		t.Errorf("expected sessionId %q, got %q", "sess-1", inv.SessionID)
	}
	if inv.EndedAt != nil {
		t.Error("expected EndedAt to be nil for started status")
	}
}

func TestEmit_ToolCompleted(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":       "tool-2",
		"toolName": "host.file_read",
		"result":   "file content here",
	})
	now := time.Now()
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolCompleted,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: now,
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.toolInvocations) != 1 {
		t.Fatalf("expected 1 tool invocation, got %d", len(sub.toolInvocations))
	}
	inv := sub.toolInvocations[0]
	if inv.Status != ToolInvocationCompleted {
		t.Errorf("expected status %q, got %q", ToolInvocationCompleted, inv.Status)
	}
	if inv.Result != "file content here" {
		t.Errorf("expected result %q, got %q", "file content here", inv.Result)
	}
	if inv.EndedAt == nil {
		t.Error("expected EndedAt to be set for completed status")
	}
}

func TestEmit_ToolFailed(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":       "tool-3",
		"toolName": "host.exec",
		"error":    "permission denied",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolFailed,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	inv := sub.toolInvocations[0]
	if inv.Status != ToolInvocationFailed {
		t.Errorf("expected status %q, got %q", ToolInvocationFailed, inv.Status)
	}
	if inv.Error != "permission denied" {
		t.Errorf("expected error %q, got %q", "permission denied", inv.Error)
	}
	if inv.EndedAt == nil {
		t.Error("expected EndedAt to be set for failed status")
	}
}

func TestEmit_ToolProgress(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":       "tool-4",
		"toolName": "host.log_tail",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolProgress,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	inv := sub.toolInvocations[0]
	if inv.Status != ToolInvocationProgress {
		t.Errorf("expected status %q, got %q", ToolInvocationProgress, inv.Status)
	}
}

func TestEmit_ActivityUpdate(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"searchCount":    3,
		"browseCount":    1,
		"commandCount":   5,
		"fileReadCount":  2,
		"fileWriteCount": 0,
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventActivityUpdate,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(sub.activities))
	}
	act := sub.activities[0]
	if act.SearchCount != 3 {
		t.Errorf("expected searchCount 3, got %d", act.SearchCount)
	}
	if act.BrowseCount != 1 {
		t.Errorf("expected browseCount 1, got %d", act.BrowseCount)
	}
	if act.CommandCount != 5 {
		t.Errorf("expected commandCount 5, got %d", act.CommandCount)
	}
	if act.SessionID != "sess-1" {
		t.Errorf("expected sessionId %q, got %q", "sess-1", act.SessionID)
	}
}

func TestEmit_CardGenerated(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":    "card-1",
		"type":  "metric",
		"title": "CPU Usage",
		"data":  map[string]interface{}{"value": 85.5},
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventCardGenerated,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(sub.cards))
	}
	card := sub.cards[0]
	if card.ID != "card-1" {
		t.Errorf("expected id %q, got %q", "card-1", card.ID)
	}
	if card.Type != "metric" {
		t.Errorf("expected type %q, got %q", "metric", card.Type)
	}
	if card.Title != "CPU Usage" {
		t.Errorf("expected title %q, got %q", "CPU Usage", card.Title)
	}
}

func TestEmit_ApprovalNeeded(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":       "appr-1",
		"toolName": "host.exec",
		"command":  "systemctl restart nginx",
		"hostId":   "host-a",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventApprovalNeeded,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(sub.approvals))
	}
	appr := sub.approvals[0]
	if appr.ID != "appr-1" {
		t.Errorf("expected id %q, got %q", "appr-1", appr.ID)
	}
	if appr.Status != ApprovalPending {
		t.Errorf("expected status %q, got %q", ApprovalPending, appr.Status)
	}
	if appr.ToolName != "host.exec" {
		t.Errorf("expected toolName %q, got %q", "host.exec", appr.ToolName)
	}
	if appr.Command != "systemctl restart nginx" {
		t.Errorf("expected command %q, got %q", "systemctl restart nginx", appr.Command)
	}
}

func TestEmit_ApprovalDecided(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":       "appr-2",
		"toolName": "host.exec",
		"status":   "approved",
		"operator": "admin",
		"decision": "approved",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventApprovalDecided,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	appr := sub.approvals[0]
	if appr.Status != ApprovalApproved {
		t.Errorf("expected status %q, got %q", ApprovalApproved, appr.Status)
	}
	if appr.Operator != "admin" {
		t.Errorf("expected operator %q, got %q", "admin", appr.Operator)
	}
}

func TestEmit_EvidenceCollected(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":      "ev-1",
		"type":    "log_analysis",
		"summary": "Found 3 OOM events in last hour",
		"data":    map[string]interface{}{"count": 3},
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventEvidenceCollected,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub.evidences) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(sub.evidences))
	}
	ev := sub.evidences[0]
	if ev.ID != "ev-1" {
		t.Errorf("expected id %q, got %q", "ev-1", ev.ID)
	}
	if ev.Type != "log_analysis" {
		t.Errorf("expected type %q, got %q", "log_analysis", ev.Type)
	}
	if ev.Summary != "Found 3 OOM events in last hour" {
		t.Errorf("expected summary %q, got %q", "Found 3 OOM events in last hour", ev.Summary)
	}
}

func TestEmit_TurnComplete(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	statePayload, _ := json.Marshal(map[string]interface{}{
		"mode":   "inspect",
		"status": "completed",
	})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventTurnComplete,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   statePayload,
	}

	p.Emit(event)

	if len(sub.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sub.snapshots))
	}
	snap := sub.snapshots[0]
	if snap.SessionID != "sess-1" {
		t.Errorf("expected sessionId %q, got %q", "sess-1", snap.SessionID)
	}
	if snap.TurnID != "turn-1" {
		t.Errorf("expected turnId %q, got %q", "turn-1", snap.TurnID)
	}
	if snap.State == nil {
		t.Error("expected state to be non-nil")
	}
}

func TestEmit_MultipleSubscribers(t *testing.T) {
	sub1 := &mockSubscriber{}
	sub2 := &mockSubscriber{}
	p := NewProjector(sub1, sub2)

	payload, _ := json.Marshal(map[string]string{"id": "tool-x", "toolName": "test"})
	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolStarted,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   payload,
	}

	p.Emit(event)

	if len(sub1.toolInvocations) != 1 {
		t.Errorf("sub1: expected 1 tool invocation, got %d", len(sub1.toolInvocations))
	}
	if len(sub2.toolInvocations) != 1 {
		t.Errorf("sub2: expected 1 tool invocation, got %d", len(sub2.toolInvocations))
	}
}

func TestEmit_NilPayload(t *testing.T) {
	sub := &mockSubscriber{}
	p := NewProjector(sub)

	event := runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventToolStarted,
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Timestamp: time.Now(),
		Payload:   nil,
	}

	// Should not panic with nil payload
	p.Emit(event)

	if len(sub.toolInvocations) != 1 {
		t.Fatalf("expected 1 tool invocation, got %d", len(sub.toolInvocations))
	}
	inv := sub.toolInvocations[0]
	if inv.Status != ToolInvocationStarted {
		t.Errorf("expected status %q, got %q", ToolInvocationStarted, inv.Status)
	}
}

func TestToolInvocationStatus_Values(t *testing.T) {
	statuses := []ToolInvocationStatus{
		ToolInvocationStarted,
		ToolInvocationProgress,
		ToolInvocationCompleted,
		ToolInvocationFailed,
	}
	expected := []string{"started", "progress", "completed", "failed"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("expected %q, got %q", expected[i], s)
		}
	}
}

func TestApprovalStatus_Values(t *testing.T) {
	statuses := []ApprovalStatus{
		ApprovalPending,
		ApprovalApproved,
		ApprovalDenied,
	}
	expected := []string{"pending", "approved", "denied"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("expected %q, got %q", expected[i], s)
		}
	}
}
