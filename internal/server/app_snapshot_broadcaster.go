package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
)

// AppSnapshotBroadcaster fans out first-party state snapshots to the main /ws
// clients. It also implements projection.Subscriber so runtime lifecycle events
// can trigger fresh snapshot rebuilds through appui.StateService.
type AppSnapshotBroadcaster struct {
	state appui.StateService

	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan appui.StateSnapshot

	turnEventMu          sync.RWMutex
	nextTurnEventID      int
	nextTurnEventSeq     int64
	turnEventSubscribers map[int]chan appui.TurnEvent
}

func NewAppSnapshotBroadcaster(state appui.StateService) *AppSnapshotBroadcaster {
	return &AppSnapshotBroadcaster{
		state:                state,
		subscribers:          map[int]chan appui.StateSnapshot{},
		turnEventSubscribers: map[int]chan appui.TurnEvent{},
	}
}

func (b *AppSnapshotBroadcaster) Subscribe() (<-chan appui.StateSnapshot, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan appui.StateSnapshot, 1)
	b.subscribers[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
	}
}

func (b *AppSnapshotBroadcaster) Broadcast(snapshot appui.StateSnapshot) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- snapshot:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snapshot:
			default:
			}
		}
	}
}

func (b *AppSnapshotBroadcaster) SubscribeTurnEvents() (<-chan appui.TurnEvent, func()) {
	b.turnEventMu.Lock()
	defer b.turnEventMu.Unlock()
	id := b.nextTurnEventID
	b.nextTurnEventID++
	ch := make(chan appui.TurnEvent, 16)
	b.turnEventSubscribers[id] = ch
	return ch, func() {
		b.turnEventMu.Lock()
		defer b.turnEventMu.Unlock()
		if existing, ok := b.turnEventSubscribers[id]; ok {
			delete(b.turnEventSubscribers, id)
			close(existing)
		}
	}
}

func (b *AppSnapshotBroadcaster) BroadcastTurnEvent(event appui.TurnEvent) {
	b.turnEventMu.RLock()
	defer b.turnEventMu.RUnlock()
	for _, ch := range b.turnEventSubscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (b *AppSnapshotBroadcaster) nextTurnEvent(eventType appui.TurnEventType, sessionID, turnID string, payload map[string]any) appui.TurnEvent {
	b.turnEventMu.Lock()
	b.nextTurnEventSeq++
	seq := b.nextTurnEventSeq
	b.turnEventMu.Unlock()
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	return appui.TurnEvent{
		Type:      eventType,
		SessionID: sessionID,
		TurnID:    turnID,
		EventID:   fmt.Sprintf("%s:%s:%d", turnID, eventType, seq),
		Seq:       seq,
		CreatedAt: createdAt,
		Payload:   payload,
	}
}

func (b *AppSnapshotBroadcaster) BroadcastLatest(ctx context.Context) error {
	if b == nil || b.state == nil {
		return nil
	}
	snapshot, err := b.state.GetState(ctx)
	if err != nil {
		return err
	}
	b.Broadcast(snapshot)
	return nil
}

func (b *AppSnapshotBroadcaster) OnToolInvocation(inv projection.ToolInvocation) {
	eventType := appui.TurnEventToolStatusDelta
	status := string(inv.Status)
	switch inv.Status {
	case projection.ToolInvocationStarted:
		eventType = appui.TurnEventToolCallStart
		status = "running"
	case projection.ToolInvocationCompleted:
		eventType = appui.TurnEventToolResultDone
		status = "completed"
	case projection.ToolInvocationFailed:
		eventType = appui.TurnEventToolResultError
		status = "failed"
	case projection.ToolInvocationProgress:
		eventType = appui.TurnEventToolStatusDelta
		status = "running"
	}
	b.BroadcastTurnEvent(b.nextTurnEvent(eventType, inv.SessionID, inv.TurnID, map[string]any{
		"toolCallId":    inv.ID,
		"toolName":      inv.ToolName,
		"status":        status,
		"inputSummary":  string(inv.Args),
		"outputSummary": inv.Result,
		"error":         inv.Error,
	}))
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnActivity(projection.ActivityStats) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnCard(projection.Card) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnApproval(projection.Approval) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnEvidence(projection.Evidence) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnSnapshot(snapshot projection.Snapshot) {
	b.BroadcastTurnEvent(b.nextTurnEvent(appui.TurnEventDone, snapshot.SessionID, snapshot.TurnID, nil))
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnRuntimeLifecycleEvent(event runtimekernel.LifecycleEvent) {
	eventType, ok := mapRuntimeTurnEventType(event.Type)
	if !ok {
		return
	}
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	b.BroadcastTurnEvent(b.nextTurnEvent(eventType, event.SessionID, event.TurnID, payload))
}

func mapRuntimeTurnEventType(eventType runtimekernel.EventType) (appui.TurnEventType, bool) {
	switch eventType {
	case runtimekernel.EventTurnStarted:
		return appui.TurnEventStarted, true
	case runtimekernel.EventAssistantIntent:
		return appui.TurnEventAssistantIntentDelta, true
	case runtimekernel.EventAssistantFinalDelta:
		return appui.TurnEventAssistantFinalDelta, true
	case runtimekernel.EventPhaseEnd:
		return appui.TurnEventPhaseEnd, true
	case runtimekernel.EventProcessSummary:
		return appui.TurnEventProcessSummary, true
	case runtimekernel.EventTurnError:
		return appui.TurnEventError, true
	case runtimekernel.EventTurnAborted:
		return appui.TurnEventAborted, true
	default:
		return "", false
	}
}

func (s *HTTPServer) ProjectionSubscriber() projection.Subscriber {
	if s == nil || s.appSnapshots == nil {
		return nil
	}
	return s.appSnapshots
}
