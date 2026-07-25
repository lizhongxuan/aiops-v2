package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"runner/workflow/edit"
	"runner/workflowstore"
)

type workflowCommitWALState string
type workflowCommitFailpoint string
type WorkflowCommitFailureClass string

const (
	workflowCommitPrepared          workflowCommitWALState = "prepared"
	workflowCommitSnapshotCommitted workflowCommitWALState = "snapshot_committed"
	workflowCommitCommitted         workflowCommitWALState = "committed"
	workflowCommitDiscarded         workflowCommitWALState = "discarded"
	workflowCommitReceiptConfirmed  uint8                  = 1 << iota
	workflowCommitEventsConfirmed
	workflowCommitOutboxConfirmed
	workflowCommitAllConfirmed                                    = workflowCommitReceiptConfirmed | workflowCommitEventsConfirmed | workflowCommitOutboxConfirmed
	workflowCommitBeforePreparedWrite  workflowCommitFailpoint    = "before_prepared_write"
	workflowCommitAfterPrepared        workflowCommitFailpoint    = "after_prepared"
	workflowCommitBeforeSnapshotWrite  workflowCommitFailpoint    = "before_snapshot_write"
	workflowCommitAfterCurrentPointer  workflowCommitFailpoint    = "after_current_pointer"
	workflowCommitAfterReceiptAck      workflowCommitFailpoint    = "after_receipt_ack"
	workflowCommitAfterEventAck        workflowCommitFailpoint    = "after_event_ack"
	workflowCommitAfterOutboxAck       workflowCommitFailpoint    = "after_outbox_ack"
	workflowCommitBeforeCommittedWrite workflowCommitFailpoint    = "before_committed_write"
	WorkflowCommitFailureDiskFull      WorkflowCommitFailureClass = "disk_full"
	WorkflowCommitFailureRename        WorkflowCommitFailureClass = "rename"
	WorkflowCommitFailureFsync         WorkflowCommitFailureClass = "fsync"
)

type workflowCommitWALRecord struct {
	State          workflowCommitWALState            `json:"state"`
	SnapshotDigest string                            `json:"snapshotDigest"`
	Snapshot       workflowstore.TransactionSnapshot `json:"snapshot"`
	Confirmed      uint8                             `json:"confirmed"`
}
type WorkflowCommitRecoveryCallback func(context.Context, workflowstore.TransactionSnapshot) error
type WorkflowCommitRecoveryCallbacks struct {
	ConfirmReceipt, ConfirmEvents, ConfirmOutbox WorkflowCommitRecoveryCallback
}
type WorkflowCommitStoreOption func(*WorkflowCommitStore) error

func WithWorkflowCommitRecoveryCallbacks(callbacks WorkflowCommitRecoveryCallbacks) WorkflowCommitStoreOption {
	return func(store *WorkflowCommitStore) error { store.recovery = callbacks; return nil }
}

type workflowCommitFailpointHook func(workflowCommitFailpoint) error

func withWorkflowCommitFailpointForTest(hook workflowCommitFailpointHook) WorkflowCommitStoreOption {
	return func(store *WorkflowCommitStore) error {
		if os.Getenv("AIOPS_WORKFLOW_FAILPOINT_TEST") != "1" {
			return fmt.Errorf("workflow commit failpoints are test-only")
		}
		store.failpoint = hook
		return nil
	}
}

type WorkflowCommitFailure struct {
	Class WorkflowCommitFailureClass
	Err   error
}

func (e *WorkflowCommitFailure) Error() string { return string(e.Class) + ": " + e.Err.Error() }
func (e *WorkflowCommitFailure) Unwrap() error { return e.Err }
func (s *WorkflowCommitStore) inject(point workflowCommitFailpoint) error {
	if s.failpoint == nil {
		return nil
	}
	if err := s.failpoint(point); err != nil {
		return classifyWorkflowCommitFailure(string(point), err)
	}
	return nil
}
func classifyWorkflowCommitFailure(point string, err error) error {
	class, lower := WorkflowCommitFailureClass("injected"), strings.ToLower(err.Error())
	switch {
	case errors.Is(err, syscall.ENOSPC):
		class = WorkflowCommitFailureDiskFull
	case strings.Contains(lower, "rename"):
		class = WorkflowCommitFailureRename
	case strings.Contains(lower, "fsync"), strings.Contains(lower, "sync atomic"):
		class = WorkflowCommitFailureFsync
	}
	return &WorkflowCommitFailure{Class: class, Err: fmt.Errorf("%s: %w", point, err)}
}
func workflowCommitSnapshotDigest(snapshot workflowstore.TransactionSnapshot) (string, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
func workflowCommitWALPath(record workflowCommitWALRecord) string {
	digest := sha256.Sum256([]byte(record.Snapshot.TransactionID + "\x00" + record.SnapshotDigest))
	return filepath.ToSlash(filepath.Join("wal", hex.EncodeToString(digest[:])+".json"))
}
func loadWorkflowCommitWAL(namespace *workflowstore.WorkflowNamespace) ([]workflowCommitWALRecord, error) {
	directory, err := namespace.Path("wal")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]workflowCommitWALRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path, err := namespace.Path(filepath.ToSlash(filepath.Join("wal", entry.Name())))
		if err != nil {
			return nil, err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var record workflowCommitWALRecord
		if err := edit.DecodeStrict(payload, &record); err != nil || validateWorkflowCommitWAL(record) != nil {
			return nil, fmt.Errorf("invalid workflow commit WAL %q", entry.Name())
		}
		records = append(records, record)
	}
	return records, nil
}
func saveWorkflowCommitWAL(namespace *workflowstore.WorkflowNamespace, record workflowCommitWALRecord) error {
	if err := validateWorkflowCommitWAL(record); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return namespace.AtomicWriteFile(workflowCommitWALPath(record), payload, 0o600)
}
func validateWorkflowCommitWAL(record workflowCommitWALRecord) error {
	digest, err := workflowCommitSnapshotDigest(record.Snapshot)
	validState := record.State == workflowCommitPrepared || record.State == workflowCommitSnapshotCommitted || record.State == workflowCommitCommitted || record.State == workflowCommitDiscarded
	if err != nil || !validState || record.SnapshotDigest != digest || record.Confirmed&^workflowCommitAllConfirmed != 0 {
		return fmt.Errorf("invalid workflow commit WAL")
	}
	if record.State == workflowCommitCommitted && record.Confirmed != workflowCommitAllConfirmed {
		return fmt.Errorf("committed workflow WAL has incomplete acknowledgements")
	}
	return nil
}
