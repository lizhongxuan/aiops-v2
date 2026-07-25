package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"runner/workflow/edit"
	"runner/workflowstore"
)

var (
	ErrWorkflowEventInvalid  = errors.New("invalid workflow event")
	ErrWorkflowEventConflict = errors.New("workflow event identity conflict")
	ErrWorkflowEventSequence = errors.New("workflow event sequence conflict")
	ErrWorkflowEventSecret   = errors.New("workflow event contains inline credential")
)

// WorkflowEventGap describes an inclusive missing sequence range.
type WorkflowEventGap struct {
	FromSequence uint64 `json:"fromSequence"`
	ToSequence   uint64 `json:"toSequence"`
}

type WorkflowEventReplay struct {
	Events         []edit.WorkflowDomainEvent `json:"events"`
	LatestSequence uint64                     `json:"latestSequence"`
	NextSequence   uint64                     `json:"nextSequence"`
	HasMore        bool                       `json:"hasMore"`
}

type workflowEventFile struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Events        []edit.WorkflowDomainEvent `json:"events"`
}

// FileWorkflowEventLog is the retained replay source. It never removes events
// when a delivery is acknowledged by the separately persisted outbox.
type FileWorkflowEventLog struct{ root string }

type WorkflowKeyEventSequencer struct {
	log *FileWorkflowEventLog
	key edit.WorkflowKey
}

func NewFileWorkflowEventLog(root string) (*FileWorkflowEventLog, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrWorkflowEventInvalid)
	}
	if err := os.MkdirAll(filepath.Join(root, "events"), 0o700); err != nil {
		return nil, fmt.Errorf("create workflow event log: %w", err)
	}
	return &FileWorkflowEventLog{root: root}, nil
}

func (s *FileWorkflowEventLog) Sequencer(key edit.WorkflowKey) (*WorkflowKeyEventSequencer, error) {
	if s == nil || key.Validate() != nil {
		return nil, fmt.Errorf("%w: invalid workflow key", ErrWorkflowEventInvalid)
	}
	return &WorkflowKeyEventSequencer{log: s, key: key}, nil
}

func (s *WorkflowKeyEventSequencer) Append(ctx context.Context, event edit.WorkflowDomainEvent) (edit.WorkflowDomainEvent, error) {
	if event.Key != s.key {
		return edit.WorkflowDomainEvent{}, ErrWorkflowEventInvalid
	}
	return s.log.Append(ctx, event)
}

func (s *FileWorkflowEventLog) Append(ctx context.Context, event edit.WorkflowDomainEvent) (edit.WorkflowDomainEvent, error) {
	if err := validateWorkflowDomainEvent(event); err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	event.Payload = compactWorkflowJSON(event.Payload)
	if err := ctx.Err(); err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	lock, err := workflowEventFileLock(ctx, s.root, event.Key)
	if err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	defer lock.Close()

	doc, err := s.loadProjected(event.Key)
	if err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	for _, stored := range doc.Events {
		if stored.EventID == event.EventID {
			event.Sequence = stored.Sequence
			if sameWorkflowEvent(stored, event) {
				return stored, nil
			}
			return edit.WorkflowDomainEvent{}, ErrWorkflowEventConflict
		}
		if stored.TransactionID == event.TransactionID {
			return edit.WorkflowDomainEvent{}, ErrWorkflowEventConflict
		}
	}
	expected := uint64(len(doc.Events) + 1)
	if event.Sequence != 0 && event.Sequence != expected {
		return edit.WorkflowDomainEvent{}, ErrWorkflowEventSequence
	}
	event.Sequence = expected
	doc.Events = append(doc.Events, event)
	if err := writeWorkflowJSON(workflowStatePath(s.root, "events", event.Key), doc); err != nil {
		return edit.WorkflowDomainEvent{}, fmt.Errorf("append workflow event: %w", err)
	}
	return event, nil
}

// Replay returns events strictly after afterSequence in monotonic order.
func (s *FileWorkflowEventLog) Replay(ctx context.Context, key edit.WorkflowKey, afterSequence uint64, limit int) (WorkflowEventReplay, error) {
	if err := key.Validate(); err != nil {
		return WorkflowEventReplay{}, fmt.Errorf("%w: invalid workflow key", ErrWorkflowEventInvalid)
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	if err := ctx.Err(); err != nil {
		return WorkflowEventReplay{}, err
	}
	lock, err := workflowEventFileLock(ctx, s.root, key)
	if err != nil {
		return WorkflowEventReplay{}, err
	}
	defer lock.Close()
	doc, err := s.loadProjected(key)
	if err != nil {
		return WorkflowEventReplay{}, err
	}
	result := WorkflowEventReplay{NextSequence: afterSequence + 1}
	if len(doc.Events) != 0 {
		result.LatestSequence = doc.Events[len(doc.Events)-1].Sequence
	}
	for _, event := range doc.Events {
		if event.Sequence <= afterSequence {
			continue
		}
		if len(result.Events) == limit {
			result.HasMore = true
			break
		}
		result.Events = append(result.Events, event)
		result.NextSequence = event.Sequence + 1
	}
	return result, nil
}

// QueryGaps checks an inclusive range against the retained replay source.
func (s *FileWorkflowEventLog) QueryGaps(ctx context.Context, key edit.WorkflowKey, fromSequence, toSequence uint64) ([]WorkflowEventGap, error) {
	if err := key.Validate(); err != nil || fromSequence == 0 || toSequence < fromSequence {
		return nil, fmt.Errorf("%w: invalid gap range", ErrWorkflowEventInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := workflowEventFileLock(ctx, s.root, key)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	doc, err := s.loadProjected(key)
	if err != nil {
		return nil, err
	}
	cursor := fromSequence
	var gaps []WorkflowEventGap
	for _, event := range doc.Events {
		if event.Sequence < cursor || event.Sequence > toSequence {
			continue
		}
		if event.Sequence > cursor {
			gaps = append(gaps, WorkflowEventGap{FromSequence: cursor, ToSequence: event.Sequence - 1})
		}
		cursor = event.Sequence + 1
	}
	if cursor <= toSequence {
		gaps = append(gaps, WorkflowEventGap{FromSequence: cursor, ToSequence: toSequence})
	}
	return gaps, nil
}

func (s *FileWorkflowEventLog) persistCandidateTransaction(ctx context.Context, path string, transaction workflowCandidateTransaction) (edit.WorkflowDomainEvent, error) {
	if err := ctx.Err(); err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	lock, err := workflowEventFileLock(ctx, s.root, transaction.Revision.Key)
	if err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	defer lock.Close()
	if existing, err := readWorkflowCandidateTransaction(path); err == nil {
		if !sameWorkflowCandidateRevision(existing.Revision, transaction.Revision) {
			return edit.WorkflowDomainEvent{}, workflowCandidateCASConflict(transaction.Revision.Key, transaction.Revision.ParentRevision, transaction.Revision.CandidateRevision)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return edit.WorkflowDomainEvent{}, err
	} else if err := writeWorkflowJSON(path, transaction); err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	doc, err := s.loadProjected(transaction.Revision.Key)
	if err != nil {
		return edit.WorkflowDomainEvent{}, err
	}
	for _, event := range doc.Events {
		if event.EventID == transaction.Event.EventID {
			return event, nil
		}
	}
	return edit.WorkflowDomainEvent{}, ErrWorkflowEventConflict
}

func (s *FileWorkflowEventLog) loadProjected(key edit.WorkflowKey) (workflowEventFile, error) {
	doc, err := s.load(key)
	if err != nil {
		return workflowEventFile{}, err
	}
	transactions, err := listWorkflowCandidateTransactions(s.root, key)
	if err != nil {
		return workflowEventFile{}, err
	}
	changed := false
	for _, transaction := range transactions {
		event, found := transaction.Event, false
		for _, stored := range doc.Events {
			if stored.EventID == event.EventID {
				event.Sequence = stored.Sequence
				if !sameWorkflowEvent(stored, event) {
					return workflowEventFile{}, ErrWorkflowEventConflict
				}
				found = true
				break
			}
			if stored.TransactionID == event.TransactionID {
				return workflowEventFile{}, ErrWorkflowEventConflict
			}
		}
		if !found {
			event.Sequence = uint64(len(doc.Events) + 1)
			doc.Events = append(doc.Events, event)
			changed = true
		}
	}
	if changed {
		if err := writeWorkflowJSON(workflowStatePath(s.root, "events", key), doc); err != nil {
			return workflowEventFile{}, err
		}
	}
	return doc, nil
}

func workflowEventFileLock(ctx context.Context, root string, key edit.WorkflowKey) (*workflowstore.FileLock, error) {
	namespace, err := workflowstore.NewWorkflowNamespace(filepath.Join(root, ".event-sequencers"), key)
	if err != nil {
		return nil, err
	}
	return namespace.Lock(ctx)
}

func (s *FileWorkflowEventLog) load(key edit.WorkflowKey) (workflowEventFile, error) {
	doc := workflowEventFile{SchemaVersion: edit.SchemaVersion}
	data, err := os.ReadFile(workflowStatePath(s.root, "events", key))
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, fmt.Errorf("read workflow event log: %w", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.SchemaVersion != edit.SchemaVersion {
		return workflowEventFile{}, fmt.Errorf("%w: corrupt workflow event log", ErrWorkflowEventInvalid)
	}
	eventIDs, transactionIDs := make(map[string]struct{}, len(doc.Events)), make(map[string]struct{}, len(doc.Events))
	for i, event := range doc.Events {
		_, duplicateEvent := eventIDs[event.EventID]
		_, duplicateTransaction := transactionIDs[event.TransactionID]
		if validateWorkflowDomainEvent(event) != nil || event.Key != key || event.Sequence != uint64(i+1) || duplicateEvent || duplicateTransaction {
			return workflowEventFile{}, fmt.Errorf("%w: corrupt workflow event sequence", ErrWorkflowEventSequence)
		}
		eventIDs[event.EventID], transactionIDs[event.TransactionID] = struct{}{}, struct{}{}
	}
	return doc, nil
}

func validateWorkflowDomainEvent(event edit.WorkflowDomainEvent) error {
	if event.SchemaVersion != edit.SchemaVersion || event.Key.Validate() != nil || event.OccurredAt.IsZero() || len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return ErrWorkflowEventInvalid
	}
	for _, value := range []string{event.EventID, event.TransactionID, event.Type, string(event.Actor)} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
			return ErrWorkflowEventInvalid
		}
	}
	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return ErrWorkflowEventInvalid
	}
	if err := walkWorkflowCandidateSecrets(payload); err != nil {
		return ErrWorkflowEventSecret
	}
	return nil
}

func sameWorkflowEvent(left, right edit.WorkflowDomainEvent) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func compactWorkflowJSON(raw json.RawMessage) json.RawMessage {
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return raw
	}
	return append(json.RawMessage(nil), compact.Bytes()...)
}

func workflowStatePath(root, kind string, key edit.WorkflowKey) string {
	return filepath.Join(root, kind, key.TenantID, key.WorkflowID+".json")
}

func writeWorkflowJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".workflow-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
