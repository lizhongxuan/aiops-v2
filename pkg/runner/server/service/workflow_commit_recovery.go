package service

import (
	"context"
	"fmt"

	"runner/workflow/edit"
	"runner/workflowstore"
)

func (s *WorkflowCommitStore) publishWorkflowCommitSnapshot(ctx context.Context, namespace *workflowstore.WorkflowNamespace, snapshot workflowstore.TransactionSnapshot) error {
	digest, err := workflowCommitSnapshotDigest(snapshot)
	if err != nil {
		return err
	}
	record := workflowCommitWALRecord{State: workflowCommitPrepared, SnapshotDigest: digest, Snapshot: snapshot}
	if err := s.inject(workflowCommitBeforePreparedWrite); err != nil {
		return err
	}
	if err := s.persistWorkflowCommitWAL(namespace, record, "write_prepared"); err != nil {
		return err
	}
	if err := s.inject(workflowCommitAfterPrepared); err != nil {
		return err
	}
	if err := s.inject(workflowCommitBeforeSnapshotWrite); err != nil {
		return err
	}
	if err := workflowstore.PublishTransactionSnapshot(namespace, snapshot); err != nil {
		return classifyWorkflowCommitFailure("publish_snapshot", err)
	}
	if err := s.inject(workflowCommitAfterCurrentPointer); err != nil {
		return err
	}
	record.State = workflowCommitSnapshotCommitted
	if err := s.persistWorkflowCommitWAL(namespace, record, "mark_snapshot_committed"); err != nil {
		return err
	}
	return s.completeWorkflowCommit(ctx, namespace, &record, snapshot)
}

func (s *WorkflowCommitStore) Recover(ctx context.Context, key edit.WorkflowKey) error {
	if err := validateWorkflowCommitRead(ctx, key); err != nil {
		return err
	}
	namespace, err := workflowstore.NewWorkflowNamespace(s.root, key)
	if err != nil {
		return err
	}
	return namespace.WithLock(ctx, func(locked *workflowstore.WorkflowNamespace) error {
		return s.recoverWorkflowCommitNamespace(ctx, locked, key)
	})
}
func (s *WorkflowCommitStore) recoverWorkflowCommitNamespace(ctx context.Context, namespace *workflowstore.WorkflowNamespace, key edit.WorkflowKey) error {
	records, err := loadWorkflowCommitWAL(namespace)
	if err != nil {
		return classifyWorkflowCommitFailure("read_wal", err)
	}
	current, found, err := workflowstore.LoadCurrentTransactionSnapshot(namespace)
	if err != nil {
		return err
	}
	currentDigest := ""
	if found {
		currentDigest, err = workflowCommitSnapshotDigest(current)
		if err != nil {
			return err
		}
	}
	for index := range records {
		record := &records[index]
		if record.Snapshot.Key != key {
			return fmt.Errorf("workflow commit WAL key binding mismatch")
		}
		if record.State == workflowCommitCommitted || record.State == workflowCommitDiscarded {
			continue
		}
		pointerSwitched := found && currentDigest == record.SnapshotDigest && current.TransactionID == record.Snapshot.TransactionID && current.ResourceVersion == record.Snapshot.ResourceVersion && current.Generation == record.Snapshot.Generation
		if !pointerSwitched {
			record.State = workflowCommitDiscarded
			if err := s.persistWorkflowCommitWAL(namespace, *record, "discard_prepared"); err != nil {
				return err
			}
			continue
		}
		if record.State == workflowCommitPrepared {
			record.State = workflowCommitSnapshotCommitted
			if err := s.persistWorkflowCommitWAL(namespace, *record, "mark_snapshot_committed"); err != nil {
				return err
			}
		}
		if err := s.completeWorkflowCommit(ctx, namespace, record, current); err != nil {
			return err
		}
	}
	return nil
}
func (s *WorkflowCommitStore) completeWorkflowCommit(ctx context.Context, namespace *workflowstore.WorkflowNamespace, record *workflowCommitWALRecord, snapshot workflowstore.TransactionSnapshot) error {
	steps := []struct {
		confirmed uint8
		callback  WorkflowCommitRecoveryCallback
		failpoint workflowCommitFailpoint
		label     string
	}{
		{workflowCommitReceiptConfirmed, s.recovery.ConfirmReceipt, workflowCommitAfterReceiptAck, "receipt"},
		{workflowCommitEventsConfirmed, s.recovery.ConfirmEvents, workflowCommitAfterEventAck, "events"},
		{workflowCommitOutboxConfirmed, s.recovery.ConfirmOutbox, workflowCommitAfterOutboxAck, "outbox"},
	}
	for _, step := range steps {
		if record.Confirmed&step.confirmed != 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if step.callback != nil {
			if err := step.callback(ctx, snapshot); err != nil {
				return fmt.Errorf("confirm workflow commit %s: %w", step.label, err)
			}
		}
		record.Confirmed |= step.confirmed
		if err := s.persistWorkflowCommitWAL(namespace, *record, "ack_"+step.label); err != nil {
			return err
		}
		if err := s.inject(step.failpoint); err != nil {
			return err
		}
	}
	if err := s.inject(workflowCommitBeforeCommittedWrite); err != nil {
		return err
	}
	record.State = workflowCommitCommitted
	return s.persistWorkflowCommitWAL(namespace, *record, "mark_committed")
}
func (s *WorkflowCommitStore) persistWorkflowCommitWAL(namespace *workflowstore.WorkflowNamespace, record workflowCommitWALRecord, operation string) error {
	if err := saveWorkflowCommitWAL(namespace, record); err != nil {
		return classifyWorkflowCommitFailure(operation, err)
	}
	return nil
}
