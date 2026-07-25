package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"runner/workflow/edit"
	"runner/workflowstore"
)

const (
	WorkflowEventCandidateCreated   = "candidate.created"
	WorkflowEventCandidateRevised   = "candidate.revised"
	WorkflowEventTransactionApplied = "transaction.applied"
)

type workflowEventSubscriber struct {
	id     uint64
	key    edit.WorkflowKey
	inbox  chan edit.WorkflowDomainEvent
	output chan edit.WorkflowDomainEvent
	cancel context.CancelFunc
}

// WorkflowDomainEventService binds the retained log, delivery outbox and the
// mutation CommitStore. It must be constructed before accepting writes so the
// current-pointer recovery callbacks can materialize committed events.
type WorkflowDomainEventService struct {
	application *WorkflowEditApplication
	log         *FileWorkflowEventLog
	outbox      *FileWorkflowOutbox
	dispatcher  *WorkflowOutboxDispatcher
	ctx         context.Context
	cancel      context.CancelFunc

	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]*workflowEventSubscriber
	wg          sync.WaitGroup
}

func NewWorkflowDomainEventService(application *WorkflowEditApplication) (*WorkflowDomainEventService, error) {
	if application == nil || application.approvals == nil || application.commits == nil || strings.TrimSpace(application.approvals.Root) == "" {
		return nil, ErrWorkflowEditConfiguration
	}
	log, err := NewFileWorkflowEventLog(application.approvals.Root)
	if err != nil {
		return nil, err
	}
	outbox, err := NewFileWorkflowOutbox(application.approvals.Root)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := &WorkflowDomainEventService{application: application, log: log, outbox: outbox, ctx: ctx, cancel: cancel, subscribers: map[uint64]*workflowEventSubscriber{}}
	service.dispatcher = NewWorkflowOutboxDispatcher(outbox, service.deliver, time.Second)
	previous := application.commits.recovery
	application.commits.recovery.ConfirmEvents = chainWorkflowCommitRecovery(previous.ConfirmEvents, service.confirmEvents)
	application.commits.recovery.ConfirmOutbox = chainWorkflowCommitRecovery(previous.ConfirmOutbox, service.confirmOutbox)
	return service, nil
}

func chainWorkflowCommitRecovery(first, second WorkflowCommitRecoveryCallback) WorkflowCommitRecoveryCallback {
	return func(ctx context.Context, snapshot workflowstore.TransactionSnapshot) error {
		if first != nil {
			if err := first(ctx, snapshot); err != nil {
				return err
			}
		}
		return second(ctx, snapshot)
	}
}

func (s *WorkflowDomainEventService) Replay(ctx context.Context, key edit.WorkflowKey, afterSequence uint64, limit int) (WorkflowEventReplay, error) {
	page, err := s.log.Replay(ctx, key, afterSequence, limit)
	if err == nil {
		for index := range page.Events {
			page.Events[index] = canonicalWorkflowEvent(page.Events[index])
		}
	}
	return page, err
}

// Recover projects an already-switched current snapshot into the event log and
// outbox. Prepared snapshots that never became current are invisible here.
func (s *WorkflowDomainEventService) Recover(ctx context.Context, key edit.WorkflowKey) (resultErr error) {
	startedAt := time.Now().UTC()
	defer func() {
		if s != nil && s.application != nil {
			s.application.observeWorkflowEdit(ctx, WorkflowEditObservation{Operation: WorkflowEditObserveRecovery, Key: key, StartedAt: startedAt, Duration: time.Since(startedAt)}, resultErr)
		}
	}()
	if s == nil || s.application == nil || s.application.commits == nil {
		return ErrWorkflowEditConfiguration
	}
	return s.application.commits.withWorkflowCommitLock(ctx, key, func(namespace *workflowstore.WorkflowNamespace) error {
		records, err := loadWorkflowCommitWAL(namespace)
		if err != nil {
			return err
		}
		sort.SliceStable(records, func(i, j int) bool { return records[i].Snapshot.Generation < records[j].Snapshot.Generation })
		for _, record := range records {
			if record.State == workflowCommitCommitted {
				if err := s.confirmOutbox(ctx, record.Snapshot); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *WorkflowDomainEventService) Snapshot(ctx context.Context, key edit.WorkflowKey) (WorkflowEditSnapshot, error) {
	if s == nil || s.application == nil {
		return WorkflowEditSnapshot{}, ErrWorkflowEditConfiguration
	}
	return s.application.workspaceProjectionSnapshot(ctx, key)
}

func (s *WorkflowDomainEventService) Dispatch(ctx context.Context, key edit.WorkflowKey, now time.Time, limit int) (WorkflowOutboxDispatchResult, error) {
	if err := s.reconcileOutbox(ctx, key); err != nil {
		return WorkflowOutboxDispatchResult{}, err
	}
	return s.dispatcher.Dispatch(ctx, key, now, limit)
}

func (s *WorkflowDomainEventService) Subscribe(ctx context.Context, key edit.WorkflowKey, afterSequence uint64) (<-chan edit.WorkflowDomainEvent, func(), error) {
	if s == nil || key.Validate() != nil || s.ctx.Err() != nil {
		return nil, nil, ErrWorkflowEditConfiguration
	}
	subCtx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	subscriber := &workflowEventSubscriber{id: id, key: key, inbox: make(chan edit.WorkflowDomainEvent, 32), output: make(chan edit.WorkflowDomainEvent, 8), cancel: cancel}
	s.subscribers[id] = subscriber
	s.mu.Unlock()
	s.wg.Add(1)
	go s.runSubscriber(subCtx, ctx, subscriber, afterSequence)
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); s.mu.Lock(); delete(s.subscribers, id); s.mu.Unlock() }) }
	return subscriber.output, stop, nil
}

func (s *WorkflowDomainEventService) runSubscriber(serviceCtx, callerCtx context.Context, subscriber *workflowEventSubscriber, cursor uint64) {
	defer s.wg.Done()
	defer close(subscriber.output)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	replay := func() bool {
		for {
			page, err := s.Replay(serviceCtx, subscriber.key, cursor, 256)
			if err != nil {
				s.application.observeWorkflowEdit(serviceCtx, WorkflowEditObservation{Operation: WorkflowEditObserveGap, Outcome: WorkflowEditOutcomeGapUnrecoverable, Key: subscriber.key, GapSize: 1, StartedAt: time.Now().UTC()}, err)
				return false
			}
			for _, event := range page.Events {
				select {
				case subscriber.output <- event:
					cursor = event.Sequence
				case <-serviceCtx.Done():
					return false
				case <-callerCtx.Done():
					return false
				}
			}
			if !page.HasMore {
				return true
			}
		}
	}
	if !replay() {
		return
	}
	for {
		select {
		case <-serviceCtx.Done():
			return
		case <-callerCtx.Done():
			return
		case event := <-subscriber.inbox:
			if event.Sequence > cursor && !replay() {
				return
			}
		case <-ticker.C:
			_, _ = s.Dispatch(serviceCtx, subscriber.key, time.Now().UTC(), 256)
			if !replay() {
				return
			}
		}
	}
}

func (s *WorkflowDomainEventService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *WorkflowDomainEventService) deliver(_ context.Context, event edit.WorkflowDomainEvent) error {
	event = canonicalWorkflowEvent(event)
	if s != nil && s.application != nil {
		s.application.observeWorkflowEdit(context.Background(), WorkflowEditObservation{Operation: WorkflowEditObserveEvent, Key: event.Key, TransactionID: event.TransactionID, EventID: event.EventID, EventLag: time.Since(event.OccurredAt), StartedAt: time.Now().UTC()}, nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, subscriber := range s.subscribers {
		if subscriber.key == event.Key {
			select {
			case subscriber.inbox <- event:
			default:
			}
		}
	}
	return nil
}

func (s *WorkflowDomainEventService) confirmEvents(ctx context.Context, snapshot workflowstore.TransactionSnapshot) error {
	_, err := s.materializeSnapshotEvents(ctx, snapshot)
	return err
}

func (s *WorkflowDomainEventService) confirmOutbox(ctx context.Context, snapshot workflowstore.TransactionSnapshot) error {
	events, err := s.materializeSnapshotEvents(ctx, snapshot)
	if err != nil {
		return err
	}
	for _, event := range events {
		if _, err := s.outbox.Enqueue(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *WorkflowDomainEventService) materializeSnapshotEvents(ctx context.Context, snapshot workflowstore.TransactionSnapshot) ([]edit.WorkflowDomainEvent, error) {
	byTransaction, after := map[string]edit.WorkflowDomainEvent{}, uint64(0)
	var candidate *edit.WorkflowDomainEvent
	for {
		page, err := s.log.Replay(ctx, snapshot.Key, after, 1000)
		if err != nil {
			return nil, err
		}
		for _, event := range page.Events {
			byTransaction[event.TransactionID], after = event, event.Sequence
			if event.ChangeSetID == snapshot.Receipt.ChangeSetID && event.CandidateRevision != 0 {
				value := event
				candidate = &value
			}
		}
		if !page.HasMore {
			break
		}
	}
	result := make([]edit.WorkflowDomainEvent, 0, len(snapshot.Events))
	for _, envelope := range snapshot.Events {
		if existing, ok := byTransaction[snapshot.TransactionID]; ok {
			result = append(result, existing)
			continue
		}
		event, eventErr := workflowDomainEventFromSnapshot(snapshot, envelope, candidate)
		if eventErr != nil {
			return nil, eventErr
		}
		stored, appendErr := s.log.Append(ctx, event)
		if appendErr != nil {
			return nil, appendErr
		}
		byTransaction[stored.TransactionID] = stored
		result = append(result, stored)
	}
	return result, nil
}

func (s *WorkflowDomainEventService) reconcileOutbox(ctx context.Context, key edit.WorkflowKey) error {
	after := uint64(0)
	for {
		page, err := s.log.Replay(ctx, key, after, 1000)
		if err != nil {
			return err
		}
		for _, event := range page.Events {
			if _, err := s.outbox.Enqueue(ctx, event); err != nil {
				return err
			}
			after = event.Sequence
		}
		if !page.HasMore {
			return nil
		}
	}
}

func workflowDomainEventFromSnapshot(snapshot workflowstore.TransactionSnapshot, envelope workflowstore.TypedEnvelope, candidate *edit.WorkflowDomainEvent) (edit.WorkflowDomainEvent, error) {
	var source struct {
		EventID, Type, TransactionID, ResourceVersion string
		OccurredAt                                    time.Time
	}
	if envelope.Kind != "event" || strings.TrimSpace(envelope.SchemaVersion) == "" || json.Unmarshal(envelope.Payload, &source) != nil {
		return edit.WorkflowDomainEvent{}, ErrWorkflowEventInvalid
	}
	if source.TransactionID == "" {
		source.TransactionID = snapshot.TransactionID
	}
	if source.TransactionID != snapshot.TransactionID || snapshot.Receipt.RevisionAfter == nil {
		return edit.WorkflowDomainEvent{}, ErrWorkflowEventInvalid
	}
	eventType := canonicalWorkflowEventType(source.Type)
	if eventType == "" {
		eventType = WorkflowEventTransactionApplied
	}
	if eventType == "graph_committed" {
		eventType = WorkflowEventTransactionApplied
	}
	if source.EventID == "" {
		digest := sha256.Sum256([]byte(snapshot.Key.TenantID + "\x00" + snapshot.Key.WorkflowID + "\x00" + snapshot.TransactionID + "\x00" + eventType))
		source.EventID = "workflow-event-" + hex.EncodeToString(digest[:16])
	}
	if source.OccurredAt.IsZero() {
		source.OccurredAt = snapshot.CommittedAt
	}
	actor, candidateRevision := edit.ActorKind("system"), uint64(0)
	if candidate != nil {
		actor, candidateRevision = candidate.Actor, candidate.CandidateRevision
	}
	payload, err := json.Marshal(map[string]any{"transactionId": snapshot.TransactionID, "changeSetId": snapshot.Receipt.ChangeSetID, "candidateRevision": candidateRevision, "resourceVersion": snapshot.ResourceVersion, "semanticHash": snapshot.Receipt.RevisionAfter.SemanticHash, "layoutHash": snapshot.Receipt.RevisionAfter.LayoutHash})
	if err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	return edit.WorkflowDomainEvent{SchemaVersion: edit.SchemaVersion, EventID: source.EventID, Type: eventType, Key: snapshot.Key, Revision: *snapshot.Receipt.RevisionAfter, Actor: actor, TransactionID: snapshot.TransactionID, ChangeSetID: snapshot.Receipt.ChangeSetID, CandidateRevision: candidateRevision, Payload: payload, OccurredAt: source.OccurredAt}, nil
}

func canonicalWorkflowEvent(event edit.WorkflowDomainEvent) edit.WorkflowDomainEvent {
	event.Type = canonicalWorkflowEventType(event.Type)
	return event
}

func canonicalWorkflowEventType(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "workflow.")
	if value == "graph_committed" {
		return WorkflowEventTransactionApplied
	}
	return value
}
