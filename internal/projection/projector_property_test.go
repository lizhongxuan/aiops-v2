package projection

// Feature: aiops-codex-eino-rewrite, Property 19: Lifecycle 事件投影分类
// Feature: aiops-codex-eino-rewrite, Property 20: Activity 计数聚合正确性
// Feature: aiops-codex-eino-rewrite, Property 21: 审批/证据/工作台状态与正文隔离
// Feature: aiops-codex-eino-rewrite, Property 22: Projection 纯函数性
// Feature: aiops-codex-eino-rewrite, Property 26: 审计记录完整性
// Feature: aiops-codex-eino-rewrite, Property 32: Tool Lifecycle 作为状态真源

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"

	"pgregory.net/rapid"
)

// **Validates: Requirements 7.1, 7.3, 7.4, 7.5, 7.6, 7.7, 8.6, 12.2, 12.3, 12.4, 12.6**

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genSessionID generates a non-empty session identifier.
func genSessionID() *rapid.Generator[string] {
	return rapid.StringMatching(`sess-[a-z0-9]{4,12}`)
}

// genTurnID generates a non-empty turn identifier.
func genTurnID() *rapid.Generator[string] {
	return rapid.StringMatching(`turn-[a-z0-9]{4,12}`)
}

// genToolID generates a non-empty tool invocation identifier.
func genToolID() *rapid.Generator[string] {
	return rapid.StringMatching(`tool-[a-z0-9]{4,12}`)
}

// genToolName generates a tool name.
func genToolName() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_.]{2,20}`)
}

// genTimestamp generates a valid timestamp.
func genTimestamp() *rapid.Generator[time.Time] {
	return rapid.Custom[time.Time](func(t *rapid.T) time.Time {
		sec := rapid.Int64Range(1600000000, 1900000000).Draw(t, "sec")
		return time.Unix(sec, 0).UTC()
	})
}

// toolEventTypes are the event types that route to tool invocation projection.
var toolEventTypes = []runtimekernel.EventType{
	runtimekernel.EventToolStarted,
	runtimekernel.EventToolProgress,
	runtimekernel.EventToolCompleted,
	runtimekernel.EventToolFailed,
}

// genToolEventType generates one of the tool lifecycle event types.
func genToolEventType() *rapid.Generator[runtimekernel.EventType] {
	return rapid.SampledFrom(toolEventTypes)
}

// genEventType generates any valid event type.
func genEventType() *rapid.Generator[runtimekernel.EventType] {
	return rapid.SampledFrom(runtimekernel.AllEventTypes())
}

// genToolPayload generates a valid tool invocation payload.
func genToolPayload() *rapid.Generator[json.RawMessage] {
	return rapid.Custom[json.RawMessage](func(t *rapid.T) json.RawMessage {
		payload := map[string]interface{}{
			"id":       genToolID().Draw(t, "toolID"),
			"toolName": genToolName().Draw(t, "toolName"),
		}
		data, _ := json.Marshal(payload)
		return data
	})
}

// genActivityPayload generates a valid activity update payload.
func genActivityPayload() *rapid.Generator[json.RawMessage] {
	return rapid.Custom[json.RawMessage](func(t *rapid.T) json.RawMessage {
		payload := map[string]interface{}{
			"searchCount":    rapid.IntRange(0, 100).Draw(t, "search"),
			"browseCount":    rapid.IntRange(0, 100).Draw(t, "browse"),
			"commandCount":   rapid.IntRange(0, 100).Draw(t, "command"),
			"fileReadCount":  rapid.IntRange(0, 100).Draw(t, "fileRead"),
			"fileWriteCount": rapid.IntRange(0, 100).Draw(t, "fileWrite"),
		}
		data, _ := json.Marshal(payload)
		return data
	})
}

// genApprovalPayload generates a valid approval payload.
func genApprovalPayload() *rapid.Generator[json.RawMessage] {
	return rapid.Custom[json.RawMessage](func(t *rapid.T) json.RawMessage {
		payload := map[string]interface{}{
			"id":       rapid.StringMatching(`appr-[a-z0-9]{4,8}`).Draw(t, "id"),
			"toolName": genToolName().Draw(t, "toolName"),
			"command":  rapid.StringMatching(`[a-z]{3,20}`).Draw(t, "command"),
			"hostId":   rapid.StringMatching(`host-[a-z0-9]{2,6}`).Draw(t, "hostId"),
			"operator": rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "operator"),
			"decision": rapid.SampledFrom([]string{"approved", "denied"}).Draw(t, "decision"),
		}
		data, _ := json.Marshal(payload)
		return data
	})
}

// genEvidencePayload generates a valid evidence payload.
func genEvidencePayload() *rapid.Generator[json.RawMessage] {
	return rapid.Custom[json.RawMessage](func(t *rapid.T) json.RawMessage {
		payload := map[string]interface{}{
			"id":      rapid.StringMatching(`ev-[a-z0-9]{4,8}`).Draw(t, "id"),
			"type":    rapid.SampledFrom([]string{"log_analysis", "metric", "trace"}).Draw(t, "type"),
			"summary": rapid.StringMatching(`[A-Za-z ]{5,40}`).Draw(t, "summary"),
		}
		data, _ := json.Marshal(payload)
		return data
	})
}

// genLifecycleEvent generates a valid LifecycleEvent with appropriate payload for its type.
func genLifecycleEvent() *rapid.Generator[runtimekernel.LifecycleEvent] {
	return rapid.Custom[runtimekernel.LifecycleEvent](func(t *rapid.T) runtimekernel.LifecycleEvent {
		eventType := genEventType().Draw(t, "eventType")
		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		var payload json.RawMessage
		switch eventType {
		case runtimekernel.EventToolStarted, runtimekernel.EventToolProgress,
			runtimekernel.EventToolCompleted, runtimekernel.EventToolFailed:
			payload = genToolPayload().Draw(t, "payload")
		case runtimekernel.EventReasoningSummaryDelta:
			payload, _ = json.Marshal(map[string]interface{}{
				"itemId":       rapid.StringMatching(`reasoning-[a-z0-9]{4}`).Draw(t, "reasoningID"),
				"summaryIndex": 0,
				"delta":        "我会先查看项目结构。",
				"summary":      "我会先查看项目结构。",
			})
		case runtimekernel.EventReasoningSummaryCompleted:
			payload, _ = json.Marshal(map[string]interface{}{
				"itemId":       rapid.StringMatching(`reasoning-[a-z0-9]{4}`).Draw(t, "reasoningID"),
				"summaryIndex": 0,
				"summary":      "已确认需要检查项目结构和事件流实现。",
				"foldable":     true,
				"autoCollapse": true,
			})
		case runtimekernel.EventActivityUpdate:
			payload = genActivityPayload().Draw(t, "payload")
		case runtimekernel.EventCardGenerated:
			cardPayload := map[string]interface{}{
				"id":    rapid.StringMatching(`card-[a-z0-9]{4}`).Draw(t, "cardID"),
				"type":  "metric",
				"title": "Test Card",
			}
			payload, _ = json.Marshal(cardPayload)
		case runtimekernel.EventApprovalNeeded, runtimekernel.EventApprovalDecided:
			payload = genApprovalPayload().Draw(t, "payload")
		case runtimekernel.EventEvidenceCollected:
			payload = genEvidencePayload().Draw(t, "payload")
		case runtimekernel.EventTurnComplete:
			statePayload := map[string]interface{}{"status": "completed"}
			payload, _ = json.Marshal(statePayload)
		}

		return runtimekernel.LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}
	})
}

// ---------------------------------------------------------------------------
// trackingSubscriber records which callback was invoked for each event.
// ---------------------------------------------------------------------------

type callbackKind int

const (
	cbToolInvocation callbackKind = iota
	cbActivity
	cbCard
	cbApproval
	cbEvidence
	cbSnapshot
	cbRuntimeLifecycle
)

type trackingSubscriber struct {
	calls           []callbackKind
	toolInvocations []ToolInvocation
	activities      []ActivityStats
	cards           []Card
	approvals       []Approval
	evidences       []Evidence
	snapshots       []Snapshot
	lifecycleEvents []runtimekernel.LifecycleEvent
}

func (s *trackingSubscriber) OnToolInvocation(inv ToolInvocation) {
	s.calls = append(s.calls, cbToolInvocation)
	s.toolInvocations = append(s.toolInvocations, inv)
}
func (s *trackingSubscriber) OnActivity(activity ActivityStats) {
	s.calls = append(s.calls, cbActivity)
	s.activities = append(s.activities, activity)
}
func (s *trackingSubscriber) OnCard(card Card) {
	s.calls = append(s.calls, cbCard)
	s.cards = append(s.cards, card)
}
func (s *trackingSubscriber) OnApproval(approval Approval) {
	s.calls = append(s.calls, cbApproval)
	s.approvals = append(s.approvals, approval)
}
func (s *trackingSubscriber) OnEvidence(evidence Evidence) {
	s.calls = append(s.calls, cbEvidence)
	s.evidences = append(s.evidences, evidence)
}
func (s *trackingSubscriber) OnSnapshot(snapshot Snapshot) {
	s.calls = append(s.calls, cbSnapshot)
	s.snapshots = append(s.snapshots, snapshot)
}
func (s *trackingSubscriber) OnRuntimeLifecycleEvent(event runtimekernel.LifecycleEvent) {
	s.calls = append(s.calls, cbRuntimeLifecycle)
	s.lifecycleEvents = append(s.lifecycleEvents, event)
}

// ---------------------------------------------------------------------------
// Property 19: Lifecycle 事件投影分类
// For any RuntimeKernel lifecycle event, Projection routes it to the correct
// category (toolInvocations / runtime.activity / cards / approvals / evidence / snapshot).
// **Validates: Requirements 7.1**
// ---------------------------------------------------------------------------

func TestProperty19_LifecycleEventProjectionClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		event := genLifecycleEvent().Draw(t, "event")
		p.Emit(event)

		// Exactly one callback should have been invoked
		if len(sub.calls) != 1 {
			t.Fatalf("expected exactly 1 callback invocation, got %d", len(sub.calls))
		}

		// Verify the event was routed to the correct category
		got := sub.calls[0]
		var expected callbackKind
		switch event.Type {
		case runtimekernel.EventToolStarted, runtimekernel.EventToolProgress,
			runtimekernel.EventToolCompleted, runtimekernel.EventToolFailed:
			expected = cbToolInvocation
		case runtimekernel.EventActivityUpdate:
			expected = cbActivity
		case runtimekernel.EventCardGenerated:
			expected = cbCard
		case runtimekernel.EventApprovalNeeded, runtimekernel.EventApprovalDecided:
			expected = cbApproval
		case runtimekernel.EventEvidenceCollected:
			expected = cbEvidence
		case runtimekernel.EventTurnComplete:
			expected = cbSnapshot
		case runtimekernel.EventTurnStarted, runtimekernel.EventAssistantIntent, runtimekernel.EventAssistantFinalDelta,
			runtimekernel.EventReasoningSummaryDelta, runtimekernel.EventReasoningSummaryCompleted,
			runtimekernel.EventPhaseEnd, runtimekernel.EventProcessSummary, runtimekernel.EventTurnError, runtimekernel.EventTurnAborted:
			expected = cbRuntimeLifecycle
		default:
			t.Fatalf("unexpected event type %q", event.Type)
		}

		if got != expected {
			t.Fatalf("event type %q routed to callback %d, expected %d", event.Type, got, expected)
		}
	})
}

// TestProperty19_AllEventTypesAreClassified verifies that every canonical event type
// is handled by the projector (no unhandled types).
func TestProperty19_AllEventTypesAreClassified(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		eventType := genEventType().Draw(t, "eventType")
		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		// Create minimal payload
		payload, _ := json.Marshal(map[string]string{"id": "test-id"})

		event := runtimekernel.LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		// Every valid event type should trigger exactly one callback
		if len(sub.calls) != 1 {
			t.Fatalf("event type %q did not trigger exactly 1 callback (got %d)", eventType, len(sub.calls))
		}
	})
}

// ---------------------------------------------------------------------------
// Property 20: Activity 计数聚合正确性
// For any event sequence, runtime.activity.searchCount only increments on search
// events; browse/open_page events do NOT count as search.
// **Validates: Requirements 7.3, 12.2**
// ---------------------------------------------------------------------------

func TestProperty20_ActivitySearchCountOnlyFromSearchEvents(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		searchCount := rapid.IntRange(0, 50).Draw(t, "searchCount")
		browseCount := rapid.IntRange(0, 50).Draw(t, "browseCount")

		payload, _ := json.Marshal(map[string]interface{}{
			"searchCount":    searchCount,
			"browseCount":    browseCount,
			"commandCount":   0,
			"fileReadCount":  0,
			"fileWriteCount": 0,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventActivityUpdate,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		if len(sub.activities) != 1 {
			t.Fatalf("expected 1 activity projection, got %d", len(sub.activities))
		}

		act := sub.activities[0]

		// searchCount should reflect only the search count from the event, not browse
		if act.SearchCount != searchCount {
			t.Fatalf("searchCount should be %d (from search events only), got %d", searchCount, act.SearchCount)
		}

		// browseCount should be separate and not affect searchCount
		if act.BrowseCount != browseCount {
			t.Fatalf("browseCount should be %d, got %d", browseCount, act.BrowseCount)
		}

		// Verify session/turn are preserved
		if act.SessionID != sessionID {
			t.Fatalf("sessionID not preserved: expected %q, got %q", sessionID, act.SessionID)
		}
		if act.TurnID != turnID {
			t.Fatalf("turnID not preserved: expected %q, got %q", turnID, act.TurnID)
		}
	})
}

func TestProperty20_ActivityCountersIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		searchCount := rapid.IntRange(0, 100).Draw(t, "searchCount")
		browseCount := rapid.IntRange(0, 100).Draw(t, "browseCount")
		commandCount := rapid.IntRange(0, 100).Draw(t, "commandCount")
		fileReadCount := rapid.IntRange(0, 100).Draw(t, "fileReadCount")
		fileWriteCount := rapid.IntRange(0, 100).Draw(t, "fileWriteCount")

		payload, _ := json.Marshal(map[string]interface{}{
			"searchCount":    searchCount,
			"browseCount":    browseCount,
			"commandCount":   commandCount,
			"fileReadCount":  fileReadCount,
			"fileWriteCount": fileWriteCount,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventActivityUpdate,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		act := sub.activities[0]

		// Each counter should independently reflect its own value
		if act.SearchCount != searchCount {
			t.Fatalf("searchCount: expected %d, got %d", searchCount, act.SearchCount)
		}
		if act.BrowseCount != browseCount {
			t.Fatalf("browseCount: expected %d, got %d", browseCount, act.BrowseCount)
		}
		if act.CommandCount != commandCount {
			t.Fatalf("commandCount: expected %d, got %d", commandCount, act.CommandCount)
		}
		if act.FileReadCount != fileReadCount {
			t.Fatalf("fileReadCount: expected %d, got %d", fileReadCount, act.FileReadCount)
		}
		if act.FileWriteCount != fileWriteCount {
			t.Fatalf("fileWriteCount: expected %d, got %d", fileWriteCount, act.FileWriteCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 21: 审批/证据/工作台状态与正文隔离
// For any approval, evidence, or workspace task state change event, the projection
// result is an independent data structure, not mixed into assistant text message flow.
// **Validates: Requirements 7.4, 7.5, 7.6, 12.3, 12.4**
// ---------------------------------------------------------------------------

func TestProperty21_ApprovalProjectionIsIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")
		approvalPayload := genApprovalPayload().Draw(t, "payload")

		eventType := rapid.SampledFrom([]runtimekernel.EventType{
			runtimekernel.EventApprovalNeeded,
			runtimekernel.EventApprovalDecided,
		}).Draw(t, "eventType")

		event := runtimekernel.LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   approvalPayload,
		}

		p.Emit(event)

		// Approval should be projected as independent data structure
		if len(sub.approvals) != 1 {
			t.Fatalf("expected 1 approval projection, got %d", len(sub.approvals))
		}

		// No other callbacks should have been triggered (isolation from text)
		if len(sub.toolInvocations) != 0 {
			t.Fatal("approval event should not trigger tool invocation callback")
		}
		if len(sub.activities) != 0 {
			t.Fatal("approval event should not trigger activity callback")
		}
		if len(sub.cards) != 0 {
			t.Fatal("approval event should not trigger card callback")
		}
		if len(sub.evidences) != 0 {
			t.Fatal("approval event should not trigger evidence callback")
		}
		if len(sub.snapshots) != 0 {
			t.Fatal("approval event should not trigger snapshot callback")
		}

		// Verify the approval is a self-contained data structure with its own fields
		appr := sub.approvals[0]
		if appr.SessionID != sessionID {
			t.Fatalf("approval sessionID not preserved: expected %q, got %q", sessionID, appr.SessionID)
		}
		if appr.TurnID != turnID {
			t.Fatalf("approval turnID not preserved: expected %q, got %q", turnID, appr.TurnID)
		}
	})
}

func TestProperty21_EvidenceProjectionIsIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")
		evidencePayload := genEvidencePayload().Draw(t, "payload")

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventEvidenceCollected,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   evidencePayload,
		}

		p.Emit(event)

		// Evidence should be projected as independent data structure
		if len(sub.evidences) != 1 {
			t.Fatalf("expected 1 evidence projection, got %d", len(sub.evidences))
		}

		// No other callbacks should have been triggered (isolation from text)
		if len(sub.toolInvocations) != 0 {
			t.Fatal("evidence event should not trigger tool invocation callback")
		}
		if len(sub.activities) != 0 {
			t.Fatal("evidence event should not trigger activity callback")
		}
		if len(sub.cards) != 0 {
			t.Fatal("evidence event should not trigger card callback")
		}
		if len(sub.approvals) != 0 {
			t.Fatal("evidence event should not trigger approval callback")
		}
		if len(sub.snapshots) != 0 {
			t.Fatal("evidence event should not trigger snapshot callback")
		}

		// Verify the evidence is a self-contained data structure
		ev := sub.evidences[0]
		if ev.SessionID != sessionID {
			t.Fatalf("evidence sessionID not preserved: expected %q, got %q", sessionID, ev.SessionID)
		}
		if ev.TurnID != turnID {
			t.Fatalf("evidence turnID not preserved: expected %q, got %q", turnID, ev.TurnID)
		}
	})
}

func TestProperty21_SnapshotProjectionIsIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		statePayload, _ := json.Marshal(map[string]interface{}{
			"mode":   "execute",
			"status": "completed",
			"agents": []string{"worker-1", "worker-2"},
		})

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventTurnComplete,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   statePayload,
		}

		p.Emit(event)

		// Snapshot should be projected as independent data structure
		if len(sub.snapshots) != 1 {
			t.Fatalf("expected 1 snapshot projection, got %d", len(sub.snapshots))
		}

		// No other callbacks should have been triggered
		if len(sub.toolInvocations) != 0 {
			t.Fatal("turn complete event should not trigger tool invocation callback")
		}
		if len(sub.approvals) != 0 {
			t.Fatal("turn complete event should not trigger approval callback")
		}
		if len(sub.evidences) != 0 {
			t.Fatal("turn complete event should not trigger evidence callback")
		}

		snap := sub.snapshots[0]
		if snap.SessionID != sessionID {
			t.Fatalf("snapshot sessionID not preserved: expected %q, got %q", sessionID, snap.SessionID)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 22: Projection 纯函数性
// For any lifecycle event input, Projection does not modify the input state,
// does not trigger business reasoning or non-projection side effects.
// **Validates: Requirements 7.7**
// ---------------------------------------------------------------------------

func TestProperty22_ProjectionDoesNotModifyInput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		event := genLifecycleEvent().Draw(t, "event")

		// Deep copy the event fields before projection
		originalType := event.Type
		originalSessionID := event.SessionID
		originalTurnID := event.TurnID
		originalTimestamp := event.Timestamp
		var originalPayload []byte
		if event.Payload != nil {
			originalPayload = make([]byte, len(event.Payload))
			copy(originalPayload, event.Payload)
		}

		// Execute projection
		p.Emit(event)

		// Verify the input event was NOT modified
		if event.Type != originalType {
			t.Fatalf("Emit modified event.Type: was %q, now %q", originalType, event.Type)
		}
		if event.SessionID != originalSessionID {
			t.Fatalf("Emit modified event.SessionID: was %q, now %q", originalSessionID, event.SessionID)
		}
		if event.TurnID != originalTurnID {
			t.Fatalf("Emit modified event.TurnID: was %q, now %q", originalTurnID, event.TurnID)
		}
		if !event.Timestamp.Equal(originalTimestamp) {
			t.Fatalf("Emit modified event.Timestamp: was %v, now %v", originalTimestamp, event.Timestamp)
		}
		if originalPayload != nil {
			if string(event.Payload) != string(originalPayload) {
				t.Fatalf("Emit modified event.Payload:\n  was: %s\n  now: %s", string(originalPayload), string(event.Payload))
			}
		}
	})
}

func TestProperty22_ProjectionIsDeterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := genLifecycleEvent().Draw(t, "event")

		// Run projection twice with the same input
		sub1 := &trackingSubscriber{}
		p1 := NewProjector(sub1)
		p1.Emit(event)

		sub2 := &trackingSubscriber{}
		p2 := NewProjector(sub2)
		p2.Emit(event)

		// Both should produce the same callback type
		if len(sub1.calls) != len(sub2.calls) {
			t.Fatalf("non-deterministic: first run %d calls, second run %d calls",
				len(sub1.calls), len(sub2.calls))
		}
		if len(sub1.calls) > 0 && sub1.calls[0] != sub2.calls[0] {
			t.Fatalf("non-deterministic: first run callback %d, second run callback %d",
				sub1.calls[0], sub2.calls[0])
		}

		// Verify projected data is identical
		if len(sub1.toolInvocations) > 0 && len(sub2.toolInvocations) > 0 {
			inv1 := sub1.toolInvocations[0]
			inv2 := sub2.toolInvocations[0]
			if inv1.ID != inv2.ID || inv1.ToolName != inv2.ToolName || inv1.Status != inv2.Status {
				t.Fatal("non-deterministic tool invocation projection")
			}
		}
		if len(sub1.activities) > 0 && len(sub2.activities) > 0 {
			act1 := sub1.activities[0]
			act2 := sub2.activities[0]
			if act1.SearchCount != act2.SearchCount || act1.BrowseCount != act2.BrowseCount {
				t.Fatal("non-deterministic activity projection")
			}
		}
		if len(sub1.approvals) > 0 && len(sub2.approvals) > 0 {
			a1 := sub1.approvals[0]
			a2 := sub2.approvals[0]
			if a1.ID != a2.ID || a1.Status != a2.Status {
				t.Fatal("non-deterministic approval projection")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 26: 审计记录完整性
// For any approval decision, a structured audit record is generated containing
// all necessary fields: time, host, operator, decision, tool name, command.
// **Validates: Requirements 8.6**
// ---------------------------------------------------------------------------

func TestProperty26_AuditRecordCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		// Generate a complete approval decision payload
		approvalID := rapid.StringMatching(`appr-[a-z0-9]{4,8}`).Draw(t, "approvalID")
		toolName := genToolName().Draw(t, "toolName")
		command := rapid.StringMatching(`[a-z]{3,30}`).Draw(t, "command")
		hostID := rapid.StringMatching(`host-[a-z0-9]{2,6}`).Draw(t, "hostID")
		operator := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "operator")
		decision := rapid.SampledFrom([]string{"approved", "denied"}).Draw(t, "decision")

		payload, _ := json.Marshal(map[string]interface{}{
			"id":       approvalID,
			"toolName": toolName,
			"command":  command,
			"hostId":   hostID,
			"operator": operator,
			"decision": decision,
			"status":   decision,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventApprovalDecided,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		if len(sub.approvals) != 1 {
			t.Fatalf("expected 1 approval record, got %d", len(sub.approvals))
		}

		record := sub.approvals[0]

		// Verify all required audit fields are present
		if record.ID == "" {
			t.Fatal("audit record missing ID")
		}
		if record.ID != approvalID {
			t.Fatalf("audit record ID mismatch: expected %q, got %q", approvalID, record.ID)
		}

		// Time (CreatedAt from event timestamp)
		if record.CreatedAt.IsZero() {
			t.Fatal("audit record missing timestamp (CreatedAt)")
		}
		if !record.CreatedAt.Equal(ts) {
			t.Fatalf("audit record timestamp mismatch: expected %v, got %v", ts, record.CreatedAt)
		}

		// Host
		if record.HostID != hostID {
			t.Fatalf("audit record hostId mismatch: expected %q, got %q", hostID, record.HostID)
		}

		// Operator
		if record.Operator != operator {
			t.Fatalf("audit record operator mismatch: expected %q, got %q", operator, record.Operator)
		}

		// Decision
		if record.Decision != decision {
			t.Fatalf("audit record decision mismatch: expected %q, got %q", decision, record.Decision)
		}

		// Tool name
		if record.ToolName != toolName {
			t.Fatalf("audit record toolName mismatch: expected %q, got %q", toolName, record.ToolName)
		}

		// Command
		if record.Command != command {
			t.Fatalf("audit record command mismatch: expected %q, got %q", command, record.Command)
		}

		// Session context
		if record.SessionID != sessionID {
			t.Fatalf("audit record sessionID mismatch: expected %q, got %q", sessionID, record.SessionID)
		}
		if record.TurnID != turnID {
			t.Fatalf("audit record turnID mismatch: expected %q, got %q", turnID, record.TurnID)
		}
	})
}

func TestProperty26_AuditRecordForApprovalNeeded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")

		approvalID := rapid.StringMatching(`appr-[a-z0-9]{4,8}`).Draw(t, "approvalID")
		toolName := genToolName().Draw(t, "toolName")
		command := rapid.StringMatching(`[a-z]{3,30}`).Draw(t, "command")
		hostID := rapid.StringMatching(`host-[a-z0-9]{2,6}`).Draw(t, "hostID")

		payload, _ := json.Marshal(map[string]interface{}{
			"id":       approvalID,
			"toolName": toolName,
			"command":  command,
			"hostId":   hostID,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      runtimekernel.EventApprovalNeeded,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		if len(sub.approvals) != 1 {
			t.Fatalf("expected 1 approval record, got %d", len(sub.approvals))
		}

		record := sub.approvals[0]

		// For approval.needed, status should be pending
		if record.Status != ApprovalPending {
			t.Fatalf("approval.needed should produce pending status, got %q", record.Status)
		}

		// Required fields for audit trail
		if record.ID != approvalID {
			t.Fatalf("audit record ID mismatch: expected %q, got %q", approvalID, record.ID)
		}
		if record.ToolName != toolName {
			t.Fatalf("audit record toolName mismatch: expected %q, got %q", toolName, record.ToolName)
		}
		if record.Command != command {
			t.Fatalf("audit record command mismatch: expected %q, got %q", command, record.Command)
		}
		if record.HostID != hostID {
			t.Fatalf("audit record hostId mismatch: expected %q, got %q", hostID, record.HostID)
		}
		if record.CreatedAt.IsZero() {
			t.Fatal("audit record missing timestamp")
		}
	})
}

// ---------------------------------------------------------------------------
// Property 32: Tool Lifecycle 作为状态真源
// For any tool execution process, the projected tool state comes entirely from
// lifecycle events (started/progress/completed/failed), not from assistant text
// content parsing.
// **Validates: Requirements 12.6**
// ---------------------------------------------------------------------------

func TestProperty32_ToolStateFromLifecycleEventsOnly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")
		toolID := genToolID().Draw(t, "toolID")
		toolName := genToolName().Draw(t, "toolName")
		eventType := genToolEventType().Draw(t, "eventType")

		payload, _ := json.Marshal(map[string]interface{}{
			"id":       toolID,
			"toolName": toolName,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		if len(sub.toolInvocations) != 1 {
			t.Fatalf("expected 1 tool invocation, got %d", len(sub.toolInvocations))
		}

		inv := sub.toolInvocations[0]

		// The tool state (status) must be derived from the event type, not text
		var expectedStatus ToolInvocationStatus
		switch eventType {
		case runtimekernel.EventToolStarted:
			expectedStatus = ToolInvocationStarted
		case runtimekernel.EventToolProgress:
			expectedStatus = ToolInvocationProgress
		case runtimekernel.EventToolCompleted:
			expectedStatus = ToolInvocationCompleted
		case runtimekernel.EventToolFailed:
			expectedStatus = ToolInvocationFailed
		}

		if inv.Status != expectedStatus {
			t.Fatalf("tool status should be derived from lifecycle event type %q → %q, got %q",
				eventType, expectedStatus, inv.Status)
		}

		// Tool identity comes from the event payload, not text parsing
		if inv.ID != toolID {
			t.Fatalf("tool ID should come from event payload, expected %q, got %q", toolID, inv.ID)
		}
		if inv.ToolName != toolName {
			t.Fatalf("tool name should come from event payload, expected %q, got %q", toolName, inv.ToolName)
		}

		// Session context comes from the event envelope
		if inv.SessionID != sessionID {
			t.Fatalf("sessionID should come from event, expected %q, got %q", sessionID, inv.SessionID)
		}
		if inv.TurnID != turnID {
			t.Fatalf("turnID should come from event, expected %q, got %q", turnID, inv.TurnID)
		}
	})
}

func TestProperty32_ToolLifecycleEndedAtOnlyOnTerminalStates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		ts := genTimestamp().Draw(t, "timestamp")
		toolID := genToolID().Draw(t, "toolID")
		toolName := genToolName().Draw(t, "toolName")
		eventType := genToolEventType().Draw(t, "eventType")

		payload, _ := json.Marshal(map[string]interface{}{
			"id":       toolID,
			"toolName": toolName,
		})

		event := runtimekernel.LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: ts,
			Payload:   payload,
		}

		p.Emit(event)

		inv := sub.toolInvocations[0]

		// EndedAt should only be set for terminal states (completed/failed)
		isTerminal := eventType == runtimekernel.EventToolCompleted || eventType == runtimekernel.EventToolFailed
		if isTerminal && inv.EndedAt == nil {
			t.Fatalf("terminal event type %q should set EndedAt", eventType)
		}
		if !isTerminal && inv.EndedAt != nil {
			t.Fatalf("non-terminal event type %q should NOT set EndedAt", eventType)
		}
	})
}

func TestProperty32_ToolLifecycleSequenceProducesCorrectStates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sub := &trackingSubscriber{}
		p := NewProjector(sub)

		sessionID := genSessionID().Draw(t, "sessionID")
		turnID := genTurnID().Draw(t, "turnID")
		toolID := genToolID().Draw(t, "toolID")
		toolName := genToolName().Draw(t, "toolName")

		// Simulate a full lifecycle: started → progress (0-3 times) → completed/failed
		baseTime := genTimestamp().Draw(t, "baseTime")
		numProgress := rapid.IntRange(0, 3).Draw(t, "numProgress")
		endWithFailure := rapid.Bool().Draw(t, "endWithFailure")

		// Emit started
		startPayload, _ := json.Marshal(map[string]interface{}{
			"id": toolID, "toolName": toolName,
		})
		p.Emit(runtimekernel.LifecycleEvent{
			Type: runtimekernel.EventToolStarted, SessionID: sessionID,
			TurnID: turnID, Timestamp: baseTime, Payload: startPayload,
		})

		// Emit progress events
		for i := 0; i < numProgress; i++ {
			progressTime := baseTime.Add(time.Duration(i+1) * time.Second)
			p.Emit(runtimekernel.LifecycleEvent{
				Type: runtimekernel.EventToolProgress, SessionID: sessionID,
				TurnID: turnID, Timestamp: progressTime, Payload: startPayload,
			})
		}

		// Emit terminal event
		endTime := baseTime.Add(time.Duration(numProgress+2) * time.Second)
		var endType runtimekernel.EventType
		if endWithFailure {
			endType = runtimekernel.EventToolFailed
		} else {
			endType = runtimekernel.EventToolCompleted
		}
		p.Emit(runtimekernel.LifecycleEvent{
			Type: endType, SessionID: sessionID,
			TurnID: turnID, Timestamp: endTime, Payload: startPayload,
		})

		// Total events: 1 started + numProgress + 1 terminal
		expectedCount := 1 + numProgress + 1
		if len(sub.toolInvocations) != expectedCount {
			t.Fatalf("expected %d tool invocations, got %d", expectedCount, len(sub.toolInvocations))
		}

		// First should be started
		if sub.toolInvocations[0].Status != ToolInvocationStarted {
			t.Fatalf("first event should produce 'started' status, got %q", sub.toolInvocations[0].Status)
		}

		// Middle should be progress
		for i := 1; i <= numProgress; i++ {
			if sub.toolInvocations[i].Status != ToolInvocationProgress {
				t.Fatalf("event %d should produce 'progress' status, got %q", i, sub.toolInvocations[i].Status)
			}
		}

		// Last should be terminal
		lastInv := sub.toolInvocations[expectedCount-1]
		if endWithFailure {
			if lastInv.Status != ToolInvocationFailed {
				t.Fatalf("last event should produce 'failed' status, got %q", lastInv.Status)
			}
		} else {
			if lastInv.Status != ToolInvocationCompleted {
				t.Fatalf("last event should produce 'completed' status, got %q", lastInv.Status)
			}
		}

		// All invocations should reference the same tool (state from lifecycle, not text)
		for i, inv := range sub.toolInvocations {
			if inv.ID != toolID {
				t.Fatalf("invocation %d: tool ID should be %q (from lifecycle), got %q", i, toolID, inv.ID)
			}
			if inv.ToolName != toolName {
				t.Fatalf("invocation %d: tool name should be %q (from lifecycle), got %q", i, toolName, inv.ToolName)
			}
		}
	})
}
