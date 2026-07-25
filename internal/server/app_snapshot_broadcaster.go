package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
	"runner/server/service"
	"runner/workflow/edit"
)

// AppSnapshotBroadcaster fans out first-party state snapshots to the main /ws
// clients. It also implements projection.Subscriber so runtime lifecycle events
// can trigger fresh snapshot rebuilds through appui.StateService.
type AppSnapshotBroadcaster struct {
	state      appui.StateService
	agentEvent appui.AgentEventService
	workflow   *appui.WorkflowEventProjector

	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan appui.StateSnapshot
}

func NewAppSnapshotBroadcaster(state appui.StateService, agentEvent appui.AgentEventService) *AppSnapshotBroadcaster {
	return &AppSnapshotBroadcaster{
		state:       state,
		agentEvent:  agentEvent,
		workflow:    appui.NewWorkflowEventProjector(),
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
	snapshot, err := b.BuildLatest(ctx)
	if err != nil {
		return err
	}
	b.Broadcast(snapshot)
	return nil
}

// BuildLatest rebuilds the canonical AppSnapshot and then overlays every
// process-lifetime projection owned by the broadcaster. Callers that join
// after an earlier broadcast therefore receive the same Workflow workspace as
// already-connected clients instead of depending on channel replay timing.
func (b *AppSnapshotBroadcaster) BuildLatest(ctx context.Context) (appui.StateSnapshot, error) {
	if b == nil || b.state == nil {
		return appui.StateSnapshot{}, nil
	}
	snapshot, err := b.state.GetState(ctx)
	if err != nil {
		return appui.StateSnapshot{}, err
	}
	return b.projectSnapshot(ctx, snapshot), nil
}

func (b *AppSnapshotBroadcaster) projectSnapshot(ctx context.Context, snapshot appui.StateSnapshot) appui.StateSnapshot {
	if b == nil {
		return snapshot
	}
	snapshot = b.withAgentEventProjection(ctx, snapshot)
	if b.workflow != nil {
		snapshot = b.workflow.ProjectAppSnapshot(snapshot)
	}
	return snapshot
}

// OnWorkflowEvent folds Runner events into the canonical AppSnapshot stream;
// pages never receive a feature-local socket or polling source.
func (b *AppSnapshotBroadcaster) OnWorkflowEvent(event edit.WorkflowDomainEvent) {
	if b == nil || b.workflow == nil {
		return
	}
	b.workflow.Apply(event)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnWorkflowSnapshot(snapshot appui.WorkflowAuthoritativeSnapshot) {
	if b == nil || b.workflow == nil {
		return
	}
	b.workflow.Resync(snapshot)
	_ = b.BroadcastLatest(context.Background())
}

type WorkflowEventSource interface {
	Recover(context.Context, edit.WorkflowKey) error
	Replay(context.Context, edit.WorkflowKey, uint64, int) (service.WorkflowEventReplay, error)
	Snapshot(context.Context, edit.WorkflowKey) (service.WorkflowEditSnapshot, error)
	Subscribe(context.Context, edit.WorkflowKey, uint64) (<-chan edit.WorkflowDomainEvent, func(), error)
}

func (s *HTTPServer) BindWorkflowEvents(ctx context.Context, key edit.WorkflowKey, source WorkflowEventSource) (func(), error) {
	if s == nil || s.appSnapshots == nil {
		return nil, service.ErrWorkflowEditConfiguration
	}
	return s.appSnapshots.BindWorkflowEvents(ctx, key, source)
}

// BindWorkflowEvents performs reconnect recovery before following live
// outbox-backed delivery. It is called by the root application when a Workflow
// workspace becomes active; the page still consumes only AppSnapshot state.
func (b *AppSnapshotBroadcaster) BindWorkflowEvents(ctx context.Context, key edit.WorkflowKey, source WorkflowEventSource) (func(), error) {
	if b == nil || source == nil {
		return nil, service.ErrWorkflowEditConfiguration
	}
	if err := source.Recover(ctx, key); err != nil {
		return nil, err
	}
	replay, err := source.Replay(ctx, key, 0, 1)
	if err != nil {
		return nil, err
	}
	snapshot, err := source.Snapshot(ctx, key)
	if err != nil {
		return nil, err
	}
	b.OnWorkflowSnapshot(workflowAuthoritativeSnapshot(snapshot, replay.LatestSequence))
	bindCtx, cancel := context.WithCancel(ctx)
	events, unsubscribe, err := source.Subscribe(bindCtx, key, replay.LatestSequence)
	if err != nil {
		cancel()
		return nil, err
	}
	go func() {
		defer unsubscribe()
		for {
			select {
			case <-bindCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				b.OnWorkflowEvent(event)
				if strings.TrimPrefix(event.Type, "workflow.") == service.WorkflowEventTransactionApplied {
					for {
						current, readErr := source.Snapshot(bindCtx, key)
						if readErr == nil {
							b.OnWorkflowSnapshot(workflowAuthoritativeSnapshot(current, event.Sequence))
							break
						}
						select {
						case <-bindCtx.Done():
							return
						case <-time.After(100 * time.Millisecond):
						}
					}
				}
			}
		}
	}()
	return cancel, nil
}

func workflowAuthoritativeSnapshot(snapshot service.WorkflowEditSnapshot, sequence uint64) appui.WorkflowAuthoritativeSnapshot {
	return appui.WorkflowAuthoritativeSnapshot{
		WorkflowKey: snapshot.Key, Revision: snapshot.Revision, Graph: snapshot.Graph,
		Sequence: sequence, LoadedAt: time.Now().UTC(), RunGateFacts: snapshot.RunGateFacts,
		Receipts: snapshot.Receipts, ValidationArtifacts: snapshot.ValidationArtifacts,
		LifecycleArtifacts: snapshot.LifecycleArtifacts, DurableHistory: snapshot.DurableHistory,
	}
}

func (b *AppSnapshotBroadcaster) withAgentEventProjection(ctx context.Context, snapshot appui.StateSnapshot) appui.StateSnapshot {
	if b == nil || b.agentEvent == nil || strings.TrimSpace(snapshot.SessionID) == "" {
		return snapshot
	}
	projection, err := b.agentEvent.Projection(ctx, snapshot.SessionID)
	if err != nil {
		return snapshot
	}
	projection = appui.SanitizeAgentEventProjectionForSnapshot(projection, snapshot)
	snapshot.AgentEventProjection = &projection
	return snapshot
}

func (b *AppSnapshotBroadcaster) OnToolInvocation(inv projection.ToolInvocation) {
	events, err := appui.NormalizeToolInvocation(inv)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnActivity(activity projection.ActivityStats) {
	events, err := appui.NormalizeActivity(activity)
	b.appendAgentEvents(context.Background(), events, err)
	_ = b.BroadcastLatest(context.Background())
}

func (b *AppSnapshotBroadcaster) OnCard(card projection.Card) {
	events, err := appui.NormalizeCard(card)
	b.appendAgentEvents(context.Background(), events, err)
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
	switch event.Type {
	case runtimekernel.EventTurnError, runtimekernel.EventTurnAborted:
		_ = b.BroadcastLatest(context.Background())
	}
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
