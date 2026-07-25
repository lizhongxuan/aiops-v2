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
	"sort"
	"strconv"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

// WorkflowEditHistoryRequest reads the durable transaction ledger for one
// authenticated workflow scope.
type WorkflowEditHistoryRequest struct {
	SchemaVersion string           `json:"schemaVersion"`
	WorkflowKey   edit.WorkflowKey `json:"workflowKey"`
}

type WorkflowHistoryActionKind string

const (
	WorkflowHistoryActionNormal WorkflowHistoryActionKind = "normal"
	WorkflowHistoryActionUndo   WorkflowHistoryActionKind = "undo"
	WorkflowHistoryActionRedo   WorkflowHistoryActionKind = "redo"
)

type WorkflowEditHistoryEntry struct {
	SchemaVersion        string                    `json:"schemaVersion"`
	WorkflowKey          edit.WorkflowKey          `json:"workflowKey"`
	Generation           uint64                    `json:"generation"`
	TransactionID        string                    `json:"transactionId"`
	ChangeSetID          string                    `json:"changeSetId"`
	CheckpointID         string                    `json:"checkpointId,omitempty"`
	InverseTransactionID string                    `json:"inverseTransactionId,omitempty"`
	AuditID              string                    `json:"auditId"`
	RevisionBefore       edit.Revision             `json:"revisionBefore"`
	RevisionAfter        edit.Revision             `json:"revisionAfter"`
	TouchedPaths         []string                  `json:"touchedPaths"`
	Actor                edit.ActorKind            `json:"actor,omitempty"`
	ActorID              string                    `json:"actorId,omitempty"`
	AgentSessionID       string                    `json:"agentSessionId,omitempty"`
	TurnID               string                    `json:"turnId,omitempty"`
	ToolCallID           string                    `json:"toolCallId,omitempty"`
	ActionKind           WorkflowHistoryActionKind `json:"actionKind,omitempty"`
	TargetTransactionIDs []string                  `json:"targetTransactionIds,omitempty"`
	Status               string                    `json:"status"`
	CommittedAt          time.Time                 `json:"committedAt"`
	Reversible           bool                      `json:"reversible"`
}

type WorkflowEditHistoryResult struct {
	Kind          string                     `json:"kind"`
	SchemaVersion string                     `json:"schemaVersion"`
	WorkflowKey   edit.WorkflowKey           `json:"workflowKey"`
	Entries       []WorkflowEditHistoryEntry `json:"entries"`
}

// WorkflowEditHistoryActionRequest selects either one transaction or all
// transactions from an AI session. Undo/Redo only create a new candidate.
type WorkflowEditHistoryActionRequest struct {
	SchemaVersion        string           `json:"schemaVersion"`
	WorkflowKey          edit.WorkflowKey `json:"workflowKey"`
	TargetTransactionID  string           `json:"targetTransactionId,omitempty"`
	TargetAgentSessionID string           `json:"targetAgentSessionId,omitempty"`
	CandidateID          string           `json:"candidateId"`
	Expected             edit.Revision    `json:"expected"`
	Actor                edit.ActorKind   `json:"actor"`
	ActorID              string           `json:"actorId"`
	CanvasSessionID      string           `json:"canvasSessionId"`
	AgentSessionID       string           `json:"agentSessionId,omitempty"`
	TurnID               string           `json:"turnId,omitempty"`
	ToolCallID           string           `json:"toolCallId,omitempty"`
}

type workflowEditHistoryRecord struct {
	entry         WorkflowEditHistoryEntry
	snapshot      workflowstore.TransactionSnapshot
	inverse       edit.InversePlan
	operations    []edit.WorkflowOperation
	actionPresent bool
	actionValid   bool
}

type workflowEditHistoryArtifact struct {
	TransactionID        string                    `json:"transactionId"`
	ChangeSetID          string                    `json:"changeSetId"`
	TouchedPaths         []string                  `json:"touchedPaths"`
	ActionKind           WorkflowHistoryActionKind `json:"actionKind,omitempty"`
	TargetTransactionIDs []string                  `json:"targetTransactionIds,omitempty"`
}

type workflowEditCheckpointArtifact struct {
	ID              string `json:"id"`
	ResourceVersion string `json:"resourceVersion"`
	Graph           string `json:"graph"`
}

type workflowEditAuditArtifact struct {
	AuditID       string `json:"auditId"`
	TransactionID string `json:"transactionId"`
}

func (a *WorkflowEditApplication) History(ctx context.Context, request WorkflowEditHistoryRequest) (WorkflowEditHistoryResult, error) {
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil {
		return WorkflowEditHistoryResult{}, fmt.Errorf("%w: invalid workflow history request", ErrInvalid)
	}
	records, err := a.workflowEditHistoryRecords(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditHistoryResult{}, err
	}
	entries := make([]WorkflowEditHistoryEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, record.entry)
	}
	return WorkflowEditHistoryResult{Kind: "history", SchemaVersion: edit.SchemaVersion, WorkflowKey: request.WorkflowKey, Entries: entries}, nil
}

func (a *WorkflowEditApplication) Undo(ctx context.Context, request WorkflowEditHistoryActionRequest) (WorkflowEditPreviewResult, error) {
	return a.workflowEditHistoryPreview(ctx, request, WorkflowHistoryActionUndo)
}

func (a *WorkflowEditApplication) Redo(ctx context.Context, request WorkflowEditHistoryActionRequest) (WorkflowEditPreviewResult, error) {
	return a.workflowEditHistoryPreview(ctx, request, WorkflowHistoryActionRedo)
}

func (a *WorkflowEditApplication) workflowEditHistoryPreview(ctx context.Context, request WorkflowEditHistoryActionRequest, action WorkflowHistoryActionKind) (WorkflowEditPreviewResult, error) {
	if err := validateWorkflowEditHistoryActionRequest(request); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	if a == nil || a.backend == nil || a.candidates == nil || a.commits == nil {
		return WorkflowEditPreviewResult{}, ErrWorkflowEditConfiguration
	}
	current, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	committed, err := a.commits.Snapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	if committed.ResourceVersion != current.Revision.ResourceVersion || request.Expected != current.Revision {
		return WorkflowEditPreviewResult{}, workflowPreviewConflict(request.WorkflowKey, request.Expected, current.Revision, []string{"/resourceVersion"}, nil)
	}
	records, err := a.workflowEditHistoryRecords(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	targets := selectWorkflowEditHistoryTargets(records, request)
	if len(targets) == 0 {
		return WorkflowEditPreviewResult{}, fmt.Errorf("%w: reversible workflow transaction", ErrNotFound)
	}
	if err := validateWorkflowEditHistoryActionTargets(request.WorkflowKey, action, targets); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	working := current.Graph
	operations := make([]edit.WorkflowOperation, 0)
	identities := make([]string, 0, len(targets))
	for _, target := range targets {
		if !target.entry.Reversible {
			return WorkflowEditPreviewResult{}, workflowPreviewDomainError(edit.ErrorInvalidPatch, request.WorkflowKey, current.Revision.ResourceVersion, target.entry.RevisionAfter.ResourceVersion, "workflow transaction does not have a complete inverse checkpoint", "choose a reversible workflow transaction")
		}
		after, parseErr := parseCommittedVisualGraph(target.snapshot.GraphPayload, target.snapshot.ResourceVersion)
		if parseErr != nil {
			return WorkflowEditPreviewResult{}, fmt.Errorf("decode workflow history graph: %w", parseErr)
		}
		if conflicts := workflowEditHistoryConflicts(after, working, target.entry.TouchedPaths); len(conflicts) > 0 {
			return WorkflowEditPreviewResult{}, workflowPreviewConflict(request.WorkflowKey, target.entry.RevisionAfter, current.Revision, conflicts, nil)
		}
		applied, applyErr := applyWorkflowPreviewOperations(working, target.inverse.Operations)
		if applyErr != nil {
			return WorkflowEditPreviewResult{}, applyErr
		}
		working = applied.Graph
		operations = append(operations, cloneWorkflowOperations(target.inverse.Operations)...)
		identities = append(identities, target.entry.TransactionID)
	}
	intent := string(action) + " workflow transaction " + strings.Join(identities, ",")
	preview, err := a.Preview(ctx, WorkflowEditPreviewRequest{
		PreviewPatchRequest: edit.PreviewPatchRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: request.WorkflowKey, Expected: request.Expected, Intent: intent, Operations: operations},
		CandidateID:         request.CandidateID, Actor: request.Actor, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID,
		AgentSessionID: request.AgentSessionID, TurnID: request.TurnID, ToolCallID: request.ToolCallID,
	})
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	metadata := newWorkflowHistoryActionMetadata(preview, action, identities)
	if err := a.persistWorkflowHistoryActionMetadata(ctx, metadata); err != nil {
		at := time.Now().UTC()
		if a.now != nil {
			at = a.now().UTC()
		}
		_, _ = a.candidates.Abandon(ctx, request.WorkflowKey, preview.Candidate.ID, preview.CandidateRevision, at)
		return WorkflowEditPreviewResult{}, err
	}
	return preview, nil
}

func validateWorkflowEditHistoryActionTargets(key edit.WorkflowKey, action WorkflowHistoryActionKind, targets []workflowEditHistoryRecord) error {
	for _, target := range targets {
		switch action {
		case WorkflowHistoryActionUndo:
			if target.actionValid && target.entry.ActionKind == WorkflowHistoryActionUndo {
				return workflowPreviewDomainError(edit.ErrorInvalidPatch, key, target.entry.RevisionAfter.ResourceVersion, "", "an undo transaction must be replayed with redo", "choose redo for the durable undo transaction")
			}
		case WorkflowHistoryActionRedo:
			if !target.actionValid || target.entry.ActionKind != WorkflowHistoryActionUndo {
				return workflowPreviewDomainError(edit.ErrorInvalidPatch, key, target.entry.RevisionAfter.ResourceVersion, "", "redo requires a durable undo transaction", "choose a transaction with authoritative undo lineage")
			}
		default:
			return fmt.Errorf("%w: invalid workflow history action", ErrInvalid)
		}
	}
	return nil
}

func validateWorkflowEditHistoryActionRequest(request WorkflowEditHistoryActionRequest) error {
	hasTransaction := strings.TrimSpace(request.TargetTransactionID) != ""
	hasSession := strings.TrimSpace(request.TargetAgentSessionID) != ""
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || hasTransaction == hasSession || strings.TrimSpace(request.CandidateID) == "" || strings.TrimSpace(string(request.Actor)) == "" || strings.TrimSpace(request.ActorID) == "" || strings.TrimSpace(request.CanvasSessionID) == "" {
		return fmt.Errorf("%w: invalid workflow history action request", ErrInvalid)
	}
	for _, value := range []string{request.TargetTransactionID, request.TargetAgentSessionID, request.CandidateID, request.ActorID, request.CanvasSessionID, request.AgentSessionID, request.TurnID, request.ToolCallID} {
		if value != strings.TrimSpace(value) {
			return fmt.Errorf("%w: invalid workflow history action identity", ErrInvalid)
		}
	}
	return nil
}

func selectWorkflowEditHistoryTargets(records []workflowEditHistoryRecord, request WorkflowEditHistoryActionRequest) []workflowEditHistoryRecord {
	targets := make([]workflowEditHistoryRecord, 0)
	for _, record := range records { // records are newest first: required inverse order.
		if request.TargetTransactionID != "" && record.entry.TransactionID == request.TargetTransactionID {
			return []workflowEditHistoryRecord{record}
		}
		if request.TargetAgentSessionID != "" && record.entry.AgentSessionID == request.TargetAgentSessionID {
			targets = append(targets, record)
		}
	}
	return targets
}

func (a *WorkflowEditApplication) workflowEditHistoryRecords(ctx context.Context, key edit.WorkflowKey) ([]workflowEditHistoryRecord, error) {
	if a == nil || a.commits == nil || a.candidates == nil {
		return nil, ErrWorkflowEditConfiguration
	}
	var snapshots []workflowstore.TransactionSnapshot
	err := a.commits.withWorkflowCommitLock(ctx, key, func(namespace *workflowstore.WorkflowNamespace) error {
		current, found, err := workflowstore.LoadCurrentTransactionSnapshot(namespace)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: workflow commit snapshot", ErrNotFound)
		}
		transactionsPath, err := namespace.Path("transactions")
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(transactionsPath)
		if err != nil {
			return err
		}
		latest := make(map[string]workflowstore.TransactionSnapshot)
		for _, entry := range entries {
			generation, ok := workflowEditHistoryGeneration(entry.Name())
			if !ok || generation > current.Generation {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return fmt.Errorf("invalid workflow transaction history entry %q", entry.Name())
			}
			path := filepath.Join(transactionsPath, entry.Name(), "snapshot.json")
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("invalid workflow transaction history snapshot %q", entry.Name())
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var snapshot workflowstore.TransactionSnapshot
			if err := edit.DecodeStrict(raw, &snapshot); err != nil {
				return fmt.Errorf("decode workflow transaction history: %w", err)
			}
			if !workflowEditHistorySnapshotIdentity(entry.Name(), raw, snapshot) {
				return fmt.Errorf("workflow transaction history snapshot identity mismatch")
			}
			if err := validateWorkflowEditHistorySnapshot(snapshot, key, generation); err != nil {
				return err
			}
			if previous, exists := latest[snapshot.TransactionID]; !exists || previous.Generation < snapshot.Generation {
				latest[snapshot.TransactionID] = snapshot
			}
		}
		for _, snapshot := range latest {
			snapshots = append(snapshots, snapshot)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Generation > snapshots[j].Generation })
	records := make([]workflowEditHistoryRecord, 0, len(snapshots))
	for _, snapshot := range snapshots {
		record, err := a.workflowEditHistoryRecord(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	validateWorkflowEditHistoryActionRecords(records)
	return records, nil
}

func workflowEditHistorySnapshotIdentity(name string, raw []byte, snapshot workflowstore.TransactionSnapshot) bool {
	payloadDigest := sha256.Sum256(raw)
	digest := hex.EncodeToString(payloadDigest[:])
	identity := sha256.Sum256([]byte(snapshot.TransactionID + "\x00" + snapshot.ResourceVersion + "\x00" + digest))
	want := fmt.Sprintf("%020d-%s", snapshot.Generation, hex.EncodeToString(identity[:16]))
	return name == want
}

func workflowEditHistoryGeneration(name string) (uint64, bool) {
	prefix, _, found := strings.Cut(name, "-")
	if !found {
		return 0, false
	}
	generation, err := strconv.ParseUint(prefix, 10, 64)
	return generation, err == nil && generation > 0
}

func validateWorkflowEditHistorySnapshot(snapshot workflowstore.TransactionSnapshot, key edit.WorkflowKey, generation uint64) error {
	if snapshot.FormatVersion != 1 || snapshot.SchemaVersion != edit.SchemaVersion || snapshot.Key != key || snapshot.Generation != generation || snapshot.Receipt.Key != key || snapshot.Receipt.TransactionID != snapshot.TransactionID || snapshot.Receipt.RevisionAfter == nil || snapshot.Receipt.RevisionAfter.ResourceVersion != snapshot.ResourceVersion || len(snapshot.GraphPayload) == 0 || snapshot.ValidationBasis.Validate() != nil {
		return fmt.Errorf("invalid workflow transaction history snapshot")
	}
	return nil
}

func (a *WorkflowEditApplication) workflowEditHistoryRecord(ctx context.Context, snapshot workflowstore.TransactionSnapshot) (workflowEditHistoryRecord, error) {
	entry := WorkflowEditHistoryEntry{SchemaVersion: edit.SchemaVersion, WorkflowKey: snapshot.Key, Generation: snapshot.Generation, TransactionID: snapshot.TransactionID, ChangeSetID: snapshot.Receipt.ChangeSetID, CheckpointID: snapshot.Receipt.CheckpointID, InverseTransactionID: snapshot.Receipt.InverseTransactionID, AuditID: snapshot.Receipt.AuditID, RevisionBefore: snapshot.Receipt.RevisionBefore, RevisionAfter: *snapshot.Receipt.RevisionAfter, Status: snapshot.Receipt.Status, CommittedAt: snapshot.CommittedAt}
	var history workflowEditHistoryArtifact
	historyDecoded := edit.DecodeStrict(snapshot.History.Payload, &history) == nil
	actionPresent := history.ActionKind != "" || len(history.TargetTransactionIDs) > 0
	historyOK := historyDecoded && history.TransactionID == entry.TransactionID && history.ChangeSetID == entry.ChangeSetID && len(history.TouchedPaths) > 0 && validWorkflowHistoryArtifactAction(history.ActionKind, history.TargetTransactionIDs)
	if historyOK {
		entry.TouchedPaths = append([]string(nil), history.TouchedPaths...)
		sort.Strings(entry.TouchedPaths)
		entry.TouchedPaths = compactWorkflowEditHistoryPaths(entry.TouchedPaths)
		entry.ActionKind = history.ActionKind
		entry.TargetTransactionIDs = append([]string(nil), history.TargetTransactionIDs...)
	}
	var checkpoint workflowEditCheckpointArtifact
	checkpointOK := edit.DecodeStrict(snapshot.Checkpoint.Payload, &checkpoint) == nil && checkpoint.ID == entry.CheckpointID && checkpoint.ResourceVersion == entry.RevisionBefore.ResourceVersion && checkpoint.Graph != ""
	if checkpointOK {
		graph, err := parseCommittedVisualGraph([]byte(checkpoint.Graph), checkpoint.ResourceVersion)
		hashes, hashErr := VisualWorkflowGraphHashes(graph)
		checkpointOK = err == nil && hashErr == nil && hashes.SemanticHash == entry.RevisionBefore.SemanticHash && hashes.LayoutHash == entry.RevisionBefore.LayoutHash
	}
	var inverse edit.InversePlan
	inverseOK := edit.DecodeStrict(snapshot.Inverse.Payload, &inverse) == nil && len(inverse.Operations) > 0
	var audit workflowEditAuditArtifact
	auditOK := json.Unmarshal(snapshot.Audit.Payload, &audit) == nil && audit.AuditID == entry.AuditID && audit.TransactionID == entry.TransactionID
	var operations []edit.WorkflowOperation
	if candidate, err := a.candidates.ResolveChangeSet(ctx, snapshot.Key, entry.ChangeSetID); err == nil {
		entry.Actor, entry.ActorID = candidate.Actor, candidate.ActorID
		entry.AgentSessionID, entry.TurnID, entry.ToolCallID = candidate.AgentSessionID, candidate.TurnID, candidate.ToolCallID
		operations = cloneWorkflowOperations(candidate.NormalizedOperations)
	} else if !errors.Is(err, ErrNotFound) {
		return workflowEditHistoryRecord{}, err
	}
	entry.Reversible = historyOK && checkpointOK && inverseOK && auditOK
	return workflowEditHistoryRecord{entry: entry, snapshot: snapshot, inverse: inverse, operations: operations, actionPresent: actionPresent}, nil
}

func validWorkflowHistoryArtifactAction(action WorkflowHistoryActionKind, targets []string) bool {
	switch action {
	case "":
		return len(targets) == 0 // Legacy transaction: compatible, never redoable.
	case WorkflowHistoryActionNormal:
		return len(targets) == 0
	case WorkflowHistoryActionUndo, WorkflowHistoryActionRedo:
		if len(targets) == 0 {
			return false
		}
		seen := make(map[string]struct{}, len(targets))
		for _, target := range targets {
			if !validWorkflowApprovalValue(target, true) {
				return false
			}
			if _, exists := seen[target]; exists {
				return false
			}
			seen[target] = struct{}{}
		}
		return true
	default:
		return false
	}
}

func validateWorkflowEditHistoryActionRecords(records []workflowEditHistoryRecord) {
	byTransaction := make(map[string]int, len(records))
	for index := range records {
		byTransaction[records[index].entry.TransactionID] = index
	}
	// Targets must be older, so validate from oldest to newest.
	for index := len(records) - 1; index >= 0; index-- {
		record := &records[index]
		if !record.entry.Reversible || !record.actionPresent {
			continue
		}
		switch record.entry.ActionKind {
		case WorkflowHistoryActionNormal:
			record.actionValid = len(record.entry.TargetTransactionIDs) == 0
			continue
		case WorkflowHistoryActionUndo, WorkflowHistoryActionRedo:
		default:
			continue
		}
		expected := make([]edit.WorkflowOperation, 0)
		valid := len(record.operations) > 0
		for _, transactionID := range record.entry.TargetTransactionIDs {
			targetIndex, exists := byTransaction[transactionID]
			if !exists {
				valid = false
				break
			}
			target := &records[targetIndex]
			if target.entry.Generation >= record.entry.Generation || !target.entry.Reversible {
				valid = false
				break
			}
			if record.entry.ActionKind == WorkflowHistoryActionUndo && target.actionValid && target.entry.ActionKind == WorkflowHistoryActionUndo {
				valid = false
				break
			}
			if record.entry.ActionKind == WorkflowHistoryActionRedo && (!target.actionValid || target.entry.ActionKind != WorkflowHistoryActionUndo) {
				valid = false
				break
			}
			expected = append(expected, cloneWorkflowOperations(target.inverse.Operations)...)
		}
		if !valid {
			continue
		}
		normalized, err := normalizeWorkflowPreviewOperations(expected)
		if err != nil || !workflowHistoryOperationsEqual(normalized, record.operations) {
			continue
		}
		record.actionValid = true
	}
}

func workflowHistoryOperationsEqual(left, right []edit.WorkflowOperation) bool {
	leftDigest, leftErr := workflowQueryDigest([]any{left})
	rightDigest, rightErr := workflowQueryDigest([]any{right})
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func workflowEditHistoryConflicts(expected, actual visual.Graph, paths []string) []string {
	conflicts := make([]string, 0)
	for _, path := range paths {
		expectedValue, expectedFound := workflowPreviewPathValue(expected, path)
		actualValue, actualFound := workflowPreviewPathValue(actual, path)
		if expectedFound != actualFound || !workflowPreviewJSONEqual(expectedValue, actualValue) {
			conflicts = append(conflicts, path)
		}
	}
	sort.Strings(conflicts)
	return compactWorkflowEditHistoryPaths(conflicts)
}

func compactWorkflowEditHistoryPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{}
	}
	result := paths[:1]
	for _, path := range paths[1:] {
		if path != result[len(result)-1] {
			result = append(result, path)
		}
	}
	return result
}
