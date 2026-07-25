package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"runner/workflow/edit"
)

var (
	ErrWorkflowOutboxConflict = errors.New("workflow outbox identity conflict")
	ErrWorkflowOutboxNotFound = errors.New("workflow outbox event not found")
	ErrWorkflowOutboxState    = errors.New("invalid workflow outbox state")
)

type WorkflowOutboxState string

const (
	WorkflowOutboxPending WorkflowOutboxState = "pending"
	WorkflowOutboxAcked   WorkflowOutboxState = "acked"
)

type WorkflowOutboxRecord struct {
	Event          edit.WorkflowDomainEvent `json:"event"`
	State          WorkflowOutboxState      `json:"state"`
	Attempts       uint32                   `json:"attempts"`
	AvailableAt    time.Time                `json:"availableAt"`
	LastAttemptAt  *time.Time               `json:"lastAttemptAt,omitempty"`
	LastError      string                   `json:"lastError,omitempty"`
	AcknowledgedAt *time.Time               `json:"acknowledgedAt,omitempty"`
}

type workflowOutboxFile struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Records       []WorkflowOutboxRecord `json:"records"`
}

// FileWorkflowOutbox persists delivery state only. Replay always belongs to
// FileWorkflowEventLog, whose lifecycle is independent from ACK processing.
type FileWorkflowOutbox struct {
	root string
	mu   sync.Mutex
}

func NewFileWorkflowOutbox(root string) (*FileWorkflowOutbox, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrWorkflowEventInvalid)
	}
	if err := os.MkdirAll(filepath.Join(root, "outbox"), 0o700); err != nil {
		return nil, fmt.Errorf("create workflow outbox: %w", err)
	}
	return &FileWorkflowOutbox{root: root}, nil
}

func (s *FileWorkflowOutbox) Enqueue(ctx context.Context, event edit.WorkflowDomainEvent) (WorkflowOutboxRecord, error) {
	if err := validateWorkflowDomainEvent(event); err != nil {
		return WorkflowOutboxRecord{}, err
	}
	if event.Sequence == 0 {
		return WorkflowOutboxRecord{}, ErrWorkflowEventSequence
	}
	event.Payload = compactWorkflowJSON(event.Payload)
	if err := ctx.Err(); err != nil {
		return WorkflowOutboxRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(event.Key)
	if err != nil {
		return WorkflowOutboxRecord{}, err
	}
	for _, stored := range doc.Records {
		if stored.Event.EventID == event.EventID {
			if sameWorkflowEvent(stored.Event, event) {
				return stored, nil
			}
			return WorkflowOutboxRecord{}, ErrWorkflowOutboxConflict
		}
		if stored.Event.TransactionID == event.TransactionID {
			return WorkflowOutboxRecord{}, ErrWorkflowOutboxConflict
		}
	}
	record := WorkflowOutboxRecord{Event: event, State: WorkflowOutboxPending, AvailableAt: event.OccurredAt}
	doc.Records = append(doc.Records, record)
	if err := s.save(event.Key, doc); err != nil {
		return WorkflowOutboxRecord{}, fmt.Errorf("enqueue workflow event: %w", err)
	}
	return record, nil
}

func (s *FileWorkflowOutbox) Pending(ctx context.Context, key edit.WorkflowKey, now time.Time, limit int) ([]WorkflowOutboxRecord, error) {
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid workflow key", ErrWorkflowEventInvalid)
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(key)
	if err != nil {
		return nil, err
	}
	records := append([]WorkflowOutboxRecord(nil), doc.Records...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].Event.Sequence < records[j].Event.Sequence })
	result := make([]WorkflowOutboxRecord, 0, min(limit, len(records)))
	for _, record := range records {
		if record.State == WorkflowOutboxPending && !record.AvailableAt.After(now) {
			result = append(result, record)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *FileWorkflowOutbox) Retry(ctx context.Context, key edit.WorkflowKey, eventID string, deliveryErr error, retryAt time.Time) error {
	if err := key.Validate(); err != nil || strings.TrimSpace(eventID) == "" || retryAt.IsZero() {
		return ErrWorkflowEventInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(key)
	if err != nil {
		return err
	}
	for index := range doc.Records {
		record := &doc.Records[index]
		if record.Event.EventID != eventID {
			continue
		}
		if record.State != WorkflowOutboxPending {
			return ErrWorkflowOutboxState
		}
		now := time.Now().UTC()
		record.Attempts++
		record.LastAttemptAt = &now
		record.AvailableAt = retryAt
		if deliveryErr != nil {
			record.LastError = "delivery failed"
		}
		return s.save(key, doc)
	}
	return ErrWorkflowOutboxNotFound
}

func (s *FileWorkflowOutbox) Ack(ctx context.Context, key edit.WorkflowKey, eventID string, acknowledgedAt time.Time) error {
	if err := key.Validate(); err != nil || strings.TrimSpace(eventID) == "" || acknowledgedAt.IsZero() {
		return ErrWorkflowEventInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(key)
	if err != nil {
		return err
	}
	for index := range doc.Records {
		record := &doc.Records[index]
		if record.Event.EventID != eventID {
			continue
		}
		if record.State == WorkflowOutboxAcked {
			return nil
		}
		record.State = WorkflowOutboxAcked
		record.AcknowledgedAt = &acknowledgedAt
		return s.save(key, doc)
	}
	return ErrWorkflowOutboxNotFound
}

func (s *FileWorkflowOutbox) load(key edit.WorkflowKey) (workflowOutboxFile, error) {
	doc := workflowOutboxFile{SchemaVersion: edit.SchemaVersion}
	data, err := os.ReadFile(workflowStatePath(s.root, "outbox", key))
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, fmt.Errorf("read workflow outbox: %w", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.SchemaVersion != edit.SchemaVersion {
		return workflowOutboxFile{}, fmt.Errorf("%w: corrupt workflow outbox", ErrWorkflowOutboxState)
	}
	eventIDs, transactionIDs := make(map[string]struct{}, len(doc.Records)), make(map[string]struct{}, len(doc.Records))
	for _, record := range doc.Records {
		_, duplicateEvent := eventIDs[record.Event.EventID]
		_, duplicateTransaction := transactionIDs[record.Event.TransactionID]
		if validateWorkflowDomainEvent(record.Event) != nil || record.Event.Sequence == 0 || record.Event.Key != key || duplicateEvent || duplicateTransaction ||
			(record.State != WorkflowOutboxPending && record.State != WorkflowOutboxAcked) {
			return workflowOutboxFile{}, fmt.Errorf("%w: corrupt workflow outbox", ErrWorkflowOutboxState)
		}
		eventIDs[record.Event.EventID], transactionIDs[record.Event.TransactionID] = struct{}{}, struct{}{}
	}
	return doc, nil
}

func (s *FileWorkflowOutbox) save(key edit.WorkflowKey, doc workflowOutboxFile) error {
	return writeWorkflowJSON(workflowStatePath(s.root, "outbox", key), doc)
}
