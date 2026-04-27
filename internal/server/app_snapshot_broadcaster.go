package server

import (
	"context"
	"strings"
	"sync"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
)

// AppSnapshotBroadcaster fans out first-party state snapshots to the main /ws
// clients. It also implements projection.Subscriber so runtime lifecycle events
// can trigger fresh snapshot rebuilds through appui.StateService.
type AppSnapshotBroadcaster struct {
	state      appui.StateService
	agentEvent appui.AgentEventService

	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan appui.StateSnapshot
}

func NewAppSnapshotBroadcaster(state appui.StateService, agentEvent appui.AgentEventService) *AppSnapshotBroadcaster {
	return &AppSnapshotBroadcaster{
		state:       state,
		agentEvent:  agentEvent,
		subscribers: map[int]chan appui.StateSnapshot{},
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

func (b *AppSnapshotBroadcaster) BroadcastLatest(ctx context.Context) error {
	if b == nil || b.state == nil {
		return nil
	}
	snapshot, err := b.state.GetState(ctx)
	if err != nil {
		return err
	}
	b.Broadcast(b.withAgentEventProjection(ctx, snapshot))
	return nil
}

func (b *AppSnapshotBroadcaster) withAgentEventProjection(ctx context.Context, snapshot appui.StateSnapshot) appui.StateSnapshot {
	if b == nil || b.agentEvent == nil || strings.TrimSpace(snapshot.SessionID) == "" {
		return snapshot
	}
	projection, err := b.agentEvent.Projection(ctx, snapshot.SessionID)
	if err != nil {
		return snapshot
	}
	snapshot.AgentEventProjection = &projection
	return snapshot
}

func (b *AppSnapshotBroadcaster) OnToolInvocation(inv projection.ToolInvocation) {
	events, err := appui.NormalizeToolInvocation(inv)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnActivity(projection.ActivityStats) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnCard(projection.Card) {
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnApproval(approval projection.Approval) {
	events, err := appui.NormalizeApproval(approval)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnEvidence(evidence projection.Evidence) {
	events, err := appui.NormalizeEvidence(evidence)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnSnapshot(snapshot projection.Snapshot) {
	events, err := appui.NormalizeSnapshot(snapshot)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnRuntimeLifecycleEvent(event runtimekernel.LifecycleEvent) {
	events, err := appui.NormalizeRuntimeLifecycleEvent(event)
	b.appendAgentEvents(context.Background(), events, err)
}

func (b *AppSnapshotBroadcaster) appendAgentEvents(ctx context.Context, events []appui.AgentEvent, err error) {
	if b == nil || b.agentEvent == nil {
		return
	}
	if err != nil {
		return
	}
	for _, event := range events {
		_, _ = b.agentEvent.Append(ctx, event)
	}
}

func (s *HTTPServer) ProjectionSubscriber() projection.Subscriber {
	if s == nil || s.appSnapshots == nil {
		return nil
	}
	return s.appSnapshots
}
