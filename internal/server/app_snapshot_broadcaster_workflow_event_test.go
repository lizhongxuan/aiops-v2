package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"runner/server/service"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEventGapAndSnapshotResyncUseMainAppSnapshotBroadcaster(t *testing.T) {
	state := workflowBroadcasterStateService{snapshot: appui.StateSnapshot{Kind: "state", Config: map[string]any{"preserved": true}}}
	broadcaster := NewAppSnapshotBroadcaster(state, nil)
	updates, cancel := broadcaster.Subscribe()
	defer cancel()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	broadcaster.OnWorkflowEvent(workflowProjectorEventForBroadcaster(key, 2))
	first := <-updates
	projection, ok := first.Config[appui.WorkflowWorkspaceSnapshotKey].(appui.WorkflowEventProjection)
	if !ok || projection.Gap == nil || !projection.SnapshotResyncRequired {
		t.Fatalf("gap app snapshot = %#v", first.Config[appui.WorkflowWorkspaceSnapshotKey])
	}
	broadcaster.OnWorkflowSnapshot(appui.WorkflowAuthoritativeSnapshot{WorkflowKey: key, Revision: edit.Revision{ResourceVersion: "rv-2", SemanticHash: "semantic-2", LayoutHash: "layout-2"}, Sequence: 2})
	second := <-updates
	projection, ok = second.Config[appui.WorkflowWorkspaceSnapshotKey].(appui.WorkflowEventProjection)
	if !ok || projection.Gap != nil || projection.SnapshotResyncRequired || projection.Committed == nil || projection.Committed.Revision.ResourceVersion != "rv-2" {
		t.Fatalf("resynced app snapshot = %#v", second.Config[appui.WorkflowWorkspaceSnapshotKey])
	}
}

func TestWorkflowEventReplayBindingResyncsCommittedSnapshotThroughBroadcaster(t *testing.T) {
	state := workflowBroadcasterStateService{snapshot: appui.StateSnapshot{Kind: "state", Config: map[string]any{}}}
	broadcaster := NewAppSnapshotBroadcaster(state, nil)
	updates, cancelUpdates := broadcaster.Subscribe()
	defer cancelUpdates()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-bound"}
	source := newWorkflowBroadcasterEventSource(key)
	stop, err := broadcaster.BindWorkflowEvents(context.Background(), key, source)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	<-updates // initial replay + authoritative snapshot

	source.setRevision("rv-2")
	source.events <- workflowProjectorEventForBroadcaster(key, 2)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case update := <-updates:
			projection := update.Config[appui.WorkflowWorkspaceSnapshotKey].(appui.WorkflowEventProjection)
			if projection.Committed != nil && projection.Committed.Revision.ResourceVersion == "rv-2" && projection.LastSequence == 2 {
				if !source.recovered {
					t.Fatal("binding skipped durable recovery")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for committed AppSnapshot resync")
		}
	}
}

type workflowBroadcasterStateService struct{ snapshot appui.StateSnapshot }

func (s workflowBroadcasterStateService) GetState(context.Context) (appui.StateSnapshot, error) {
	return s.snapshot, nil
}

func workflowProjectorEventForBroadcaster(key edit.WorkflowKey, sequence uint64) edit.WorkflowDomainEvent {
	return edit.WorkflowDomainEvent{SchemaVersion: edit.SchemaVersion, EventID: "event-2", Sequence: sequence, Type: "transaction.applied", Key: key, Revision: edit.Revision{ResourceVersion: "rv-2", SemanticHash: "semantic-2", LayoutHash: "layout-2"}, Actor: "system", TransactionID: "transaction-2", Payload: []byte(`{}`), OccurredAt: time.Now().UTC()}
}

type workflowBroadcasterEventSource struct {
	mu        sync.Mutex
	key       edit.WorkflowKey
	revision  edit.Revision
	events    chan edit.WorkflowDomainEvent
	recovered bool
}

func newWorkflowBroadcasterEventSource(key edit.WorkflowKey) *workflowBroadcasterEventSource {
	return &workflowBroadcasterEventSource{key: key, revision: edit.Revision{ResourceVersion: "rv-1", SemanticHash: "semantic-rv-1", LayoutHash: "layout-rv-1"}, events: make(chan edit.WorkflowDomainEvent, 2)}
}
func (s *workflowBroadcasterEventSource) Recover(context.Context, edit.WorkflowKey) error {
	s.recovered = true
	return nil
}
func (s *workflowBroadcasterEventSource) Replay(context.Context, edit.WorkflowKey, uint64, int) (service.WorkflowEventReplay, error) {
	return service.WorkflowEventReplay{LatestSequence: 1}, nil
}
func (s *workflowBroadcasterEventSource) Snapshot(context.Context, edit.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return service.WorkflowEditSnapshot{Key: s.key, Revision: s.revision, Graph: visual.Graph{}}, nil
}
func (s *workflowBroadcasterEventSource) Subscribe(context.Context, edit.WorkflowKey, uint64) (<-chan edit.WorkflowDomainEvent, func(), error) {
	return s.events, func() {}, nil
}
func (s *workflowBroadcasterEventSource) setRevision(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revision = edit.Revision{ResourceVersion: value, SemanticHash: "semantic-" + value, LayoutHash: "layout-" + value}
}
