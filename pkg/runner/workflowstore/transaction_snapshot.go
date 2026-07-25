package workflowstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"runner/workflow/edit"
)

const currentTransactionManifestPath = "current-manifest.json"

// TypedEnvelope persists an artifact without coupling the commit boundary to
// event, audit, approval, or rollback implementations that evolve separately.
type TypedEnvelope struct {
	Kind          string          `json:"kind"`
	SchemaVersion string          `json:"schemaVersion"`
	Payload       json.RawMessage `json:"payload"`
}

// PersistedReceipt is indexed by a one-way idempotency-key digest.
type PersistedReceipt struct {
	IdempotencyKeyHash string                       `json:"idempotencyKeyHash"`
	RequestDigest      string                       `json:"requestDigest"`
	Receipt            edit.WorkflowMutationReceipt `json:"receipt"`
}

// TransactionSnapshot is the complete immutable state visible at one resource
// version. The current manifest atomically selects exactly one such snapshot.
type TransactionSnapshot struct {
	FormatVersion           int                          `json:"formatVersion"`
	SchemaVersion           string                       `json:"schemaVersion"`
	Key                     edit.WorkflowKey             `json:"workflowKey"`
	Generation              uint64                       `json:"generation"`
	ResourceVersion         string                       `json:"resourceVersion"`
	PreviousResourceVersion string                       `json:"previousResourceVersion,omitempty"`
	TransactionID           string                       `json:"transactionId"`
	GraphPayload            []byte                       `json:"graphPayload"`
	Meta                    TypedEnvelope                `json:"meta"`
	History                 TypedEnvelope                `json:"history"`
	ImportResult            TypedEnvelope                `json:"importResult"`
	RollbackResult          TypedEnvelope                `json:"rollbackResult"`
	Receipt                 edit.WorkflowMutationReceipt `json:"receipt"`
	ValidationBasis         edit.ValidationBasis         `json:"validationBasis"`
	Checkpoint              TypedEnvelope                `json:"checkpoint"`
	Inverse                 TypedEnvelope                `json:"inverse"`
	ApprovalConsume         TypedEnvelope                `json:"approvalConsume"`
	ApprovalConsumptions    []TypedEnvelope              `json:"approvalConsumptions,omitempty"`
	LifecycleArtifacts      []TypedEnvelope              `json:"lifecycleArtifacts,omitempty"`
	Audit                   TypedEnvelope                `json:"audit"`
	Events                  []TypedEnvelope              `json:"events"`
	Outbox                  []TypedEnvelope              `json:"outbox"`
	Receipts                []PersistedReceipt           `json:"receipts"`
	CommittedAt             time.Time                    `json:"committedAt"`
}

type transactionManifest struct {
	FormatVersion   int    `json:"formatVersion"`
	SchemaVersion   string `json:"schemaVersion"`
	Generation      uint64 `json:"generation"`
	ResourceVersion string `json:"resourceVersion"`
	TransactionID   string `json:"transactionId"`
	SnapshotPath    string `json:"snapshotPath"`
	SnapshotDigest  string `json:"snapshotDigest"`
}

// PublishTransactionSnapshot writes the immutable snapshot first and advances
// the current pointer last. The caller must hold the WorkflowNamespace lock.
func PublishTransactionSnapshot(namespace *WorkflowNamespace, snapshot TransactionSnapshot) error {
	if err := validateTransactionSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode workflow transaction snapshot: %w", err)
	}
	payloadDigest := sha256.Sum256(payload)
	digest := hex.EncodeToString(payloadDigest[:])
	identity := sha256.Sum256([]byte(snapshot.TransactionID + "\x00" + snapshot.ResourceVersion + "\x00" + digest))
	snapshotPath := fmt.Sprintf("transactions/%020d-%s/snapshot.json", snapshot.Generation, hex.EncodeToString(identity[:16]))
	absoluteSnapshotPath, err := namespace.Path(snapshotPath)
	if err != nil {
		return err
	}
	existing, readErr := os.ReadFile(absoluteSnapshotPath)
	switch {
	case readErr == nil && !bytes.Equal(existing, payload):
		return fmt.Errorf("immutable workflow transaction snapshot collision")
	case readErr == nil:
	case errors.Is(readErr, os.ErrNotExist):
		if err := namespace.AtomicWriteFile(snapshotPath, payload, 0o600); err != nil {
			return fmt.Errorf("write workflow transaction snapshot: %w", err)
		}
	default:
		return fmt.Errorf("inspect workflow transaction snapshot: %w", readErr)
	}

	manifest := transactionManifest{
		FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Generation: snapshot.Generation,
		ResourceVersion: snapshot.ResourceVersion, TransactionID: snapshot.TransactionID,
		SnapshotPath: snapshotPath, SnapshotDigest: digest,
	}
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workflow transaction manifest: %w", err)
	}
	if err := namespace.AtomicWriteFile(currentTransactionManifestPath, manifestPayload, 0o600); err != nil {
		return fmt.Errorf("publish workflow transaction manifest: %w", err)
	}
	return nil
}

// LoadCurrentTransactionSnapshot resolves the atomic pointer and verifies the
// referenced immutable content before returning it.
func LoadCurrentTransactionSnapshot(namespace *WorkflowNamespace) (TransactionSnapshot, bool, error) {
	manifestPath, err := namespace.Path(currentTransactionManifestPath)
	if err != nil {
		return TransactionSnapshot{}, false, err
	}
	manifestPayload, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return TransactionSnapshot{}, false, nil
	}
	if err != nil {
		return TransactionSnapshot{}, false, fmt.Errorf("read workflow transaction manifest: %w", err)
	}
	var manifest transactionManifest
	if err := edit.DecodeStrict(manifestPayload, &manifest); err != nil {
		return TransactionSnapshot{}, false, fmt.Errorf("decode workflow transaction manifest: %w", err)
	}
	if manifest.FormatVersion != 1 || manifest.SchemaVersion != edit.SchemaVersion || manifest.Generation == 0 || strings.TrimSpace(manifest.ResourceVersion) == "" || strings.TrimSpace(manifest.TransactionID) == "" || len(manifest.SnapshotDigest) != sha256.Size*2 {
		return TransactionSnapshot{}, false, fmt.Errorf("invalid workflow transaction manifest")
	}
	snapshotPath, err := namespace.Path(manifest.SnapshotPath)
	if err != nil {
		return TransactionSnapshot{}, false, fmt.Errorf("invalid workflow transaction manifest path: %w", err)
	}
	snapshotPayload, err := os.ReadFile(snapshotPath)
	if err != nil {
		return TransactionSnapshot{}, false, fmt.Errorf("read workflow transaction snapshot: %w", err)
	}
	digest := sha256.Sum256(snapshotPayload)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), manifest.SnapshotDigest) {
		return TransactionSnapshot{}, false, fmt.Errorf("workflow transaction snapshot digest mismatch")
	}
	var snapshot TransactionSnapshot
	if err := edit.DecodeStrict(snapshotPayload, &snapshot); err != nil {
		return TransactionSnapshot{}, false, fmt.Errorf("decode workflow transaction snapshot: %w", err)
	}
	if err := validateTransactionSnapshot(snapshot); err != nil {
		return TransactionSnapshot{}, false, err
	}
	if snapshot.Generation != manifest.Generation || snapshot.ResourceVersion != manifest.ResourceVersion || snapshot.TransactionID != manifest.TransactionID {
		return TransactionSnapshot{}, false, fmt.Errorf("workflow transaction manifest binding mismatch")
	}
	return snapshot, true, nil
}

func validateTransactionSnapshot(snapshot TransactionSnapshot) error {
	if snapshot.FormatVersion != 1 || snapshot.SchemaVersion != edit.SchemaVersion || snapshot.Generation == 0 || snapshot.CommittedAt.IsZero() {
		return fmt.Errorf("invalid workflow transaction snapshot header")
	}
	if err := snapshot.Key.Validate(); err != nil {
		return fmt.Errorf("invalid workflow transaction snapshot key: %w", err)
	}
	if strings.TrimSpace(snapshot.ResourceVersion) == "" || strings.TrimSpace(snapshot.TransactionID) == "" || len(snapshot.GraphPayload) == 0 {
		return fmt.Errorf("invalid workflow transaction snapshot identity or graph")
	}
	if err := snapshot.ValidationBasis.Validate(); err != nil {
		return fmt.Errorf("invalid workflow transaction validation basis: %w", err)
	}
	if err := validateSnapshotReceipt(snapshot.Receipt, snapshot.Key); err != nil || snapshot.Receipt.TransactionID != snapshot.TransactionID || snapshot.Receipt.RevisionAfter.ResourceVersion != snapshot.ResourceVersion {
		return fmt.Errorf("invalid workflow transaction current receipt")
	}
	envelopes := make([]TypedEnvelope, 0, 8+len(snapshot.ApprovalConsumptions)+len(snapshot.LifecycleArtifacts)+len(snapshot.Events)+len(snapshot.Outbox))
	envelopes = append(envelopes, snapshot.Meta, snapshot.History, snapshot.ImportResult, snapshot.RollbackResult, snapshot.Checkpoint, snapshot.Inverse, snapshot.ApprovalConsume, snapshot.Audit)
	envelopes = append(envelopes, snapshot.ApprovalConsumptions...)
	envelopes = append(envelopes, snapshot.LifecycleArtifacts...)
	envelopes = append(envelopes, snapshot.Events...)
	envelopes = append(envelopes, snapshot.Outbox...)
	for _, envelope := range envelopes {
		if strings.TrimSpace(envelope.Kind) == "" || strings.TrimSpace(envelope.SchemaVersion) == "" || len(envelope.Payload) == 0 || !json.Valid(envelope.Payload) {
			return fmt.Errorf("invalid workflow transaction typed envelope")
		}
	}
	seenReceipts := make(map[string]struct{}, len(snapshot.Receipts))
	for _, persisted := range snapshot.Receipts {
		if strings.TrimSpace(persisted.IdempotencyKeyHash) == "" || strings.TrimSpace(persisted.RequestDigest) == "" {
			return fmt.Errorf("invalid workflow transaction persisted receipt identity")
		}
		if _, exists := seenReceipts[persisted.IdempotencyKeyHash]; exists {
			return fmt.Errorf("duplicate workflow transaction persisted receipt")
		}
		seenReceipts[persisted.IdempotencyKeyHash] = struct{}{}
		if err := validateSnapshotReceipt(persisted.Receipt, snapshot.Key); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotReceipt(receipt edit.WorkflowMutationReceipt, key edit.WorkflowKey) error {
	if receipt.SchemaVersion != edit.SchemaVersion || receipt.Kind != "receipt" || receipt.Key != key || strings.TrimSpace(receipt.TransactionID) == "" || receipt.RevisionAfter == nil || strings.TrimSpace(receipt.RevisionAfter.ResourceVersion) == "" {
		return fmt.Errorf("invalid workflow transaction receipt")
	}
	return nil
}
