package workflowstore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestTransactionSnapshotPublishesImmutableVersionAndAtomicCurrentManifest(t *testing.T) {
	t.Parallel()

	namespace, err := NewWorkflowNamespace(t.TempDir(), transactionSnapshotTestKey())
	if err != nil {
		t.Fatalf("NewWorkflowNamespace() error = %v", err)
	}
	first := transactionSnapshotFixture(1, "rv-1", "transaction-1")
	if err := namespace.WithLock(context.Background(), func(locked *WorkflowNamespace) error {
		return PublishTransactionSnapshot(locked, first)
	}); err != nil {
		t.Fatalf("PublishTransactionSnapshot(first) error = %v", err)
	}
	manifestBytes, err := os.ReadFile(mustNamespacePath(t, namespace, currentTransactionManifestPath))
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}
	var firstManifest transactionManifest
	if err := json.Unmarshal(manifestBytes, &firstManifest); err != nil {
		t.Fatalf("decode first manifest: %v", err)
	}
	firstSnapshotPath := mustNamespacePath(t, namespace, firstManifest.SnapshotPath)
	firstSnapshotBytes, err := os.ReadFile(firstSnapshotPath)
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}

	second := transactionSnapshotFixture(2, "rv-2", "transaction-2")
	second.PreviousResourceVersion = "rv-1"
	second.Receipts = append(second.Receipts, first.Receipts...)
	if err := namespace.WithLock(context.Background(), func(locked *WorkflowNamespace) error {
		return PublishTransactionSnapshot(locked, second)
	}); err != nil {
		t.Fatalf("PublishTransactionSnapshot(second) error = %v", err)
	}

	loaded, found, err := LoadCurrentTransactionSnapshot(namespace)
	if err != nil {
		t.Fatalf("LoadCurrentTransactionSnapshot() error = %v", err)
	}
	if !found || !reflect.DeepEqual(loaded, second) {
		t.Fatalf("loaded snapshot = %#v, found = %v; want %#v", loaded, found, second)
	}
	unchanged, err := os.ReadFile(firstSnapshotPath)
	if err != nil {
		t.Fatalf("read first snapshot after second publish: %v", err)
	}
	if !bytes.Equal(unchanged, firstSnapshotBytes) {
		t.Fatal("publishing the next version mutated the immutable previous snapshot")
	}
}

func TestTransactionSnapshotRejectsManifestTraversalAndDigestDrift(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *WorkflowNamespace)
	}{
		{
			name: "manifest traversal",
			mutate: func(t *testing.T, namespace *WorkflowNamespace) {
				manifest := transactionManifest{FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Generation: 1, ResourceVersion: "rv-1", TransactionID: "transaction-1", SnapshotPath: "../outside.json", SnapshotDigest: "digest"}
				payload, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := namespace.AtomicWriteFile(currentTransactionManifestPath, payload, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "snapshot digest drift",
			mutate: func(t *testing.T, namespace *WorkflowNamespace) {
				manifestRaw, err := os.ReadFile(mustNamespacePath(t, namespace, currentTransactionManifestPath))
				if err != nil {
					t.Fatal(err)
				}
				var manifest transactionManifest
				if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(mustNamespacePath(t, namespace, manifest.SnapshotPath), []byte(`{"corrupt":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			namespace, err := NewWorkflowNamespace(t.TempDir(), transactionSnapshotTestKey())
			if err != nil {
				t.Fatal(err)
			}
			if err := namespace.WithLock(context.Background(), func(locked *WorkflowNamespace) error {
				return PublishTransactionSnapshot(locked, transactionSnapshotFixture(1, "rv-1", "transaction-1"))
			}); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, namespace)
			if _, _, err := LoadCurrentTransactionSnapshot(namespace); err == nil {
				t.Fatal("LoadCurrentTransactionSnapshot() accepted corrupt transaction state")
			}
		})
	}
}

func transactionSnapshotFixture(generation uint64, resourceVersion, transactionID string) TransactionSnapshot {
	key := transactionSnapshotTestKey()
	basis := edit.ValidationBasis{ResourceVersion: "basis-rv", SemanticHash: "semantic", CatalogVersion: "catalog", ActionSchemaDigest: "actions", ReferenceSnapshotHash: "references", EnvironmentBindingVersion: "environment"}
	revisionAfter := edit.Revision{ResourceVersion: resourceVersion, SemanticHash: "semantic-after", LayoutHash: "layout-after"}
	receipt := edit.WorkflowMutationReceipt{SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: transactionID, ChangeSetID: "change-set", Key: key, Status: "committed", RevisionBefore: edit.Revision{}, RevisionAfter: &revisionAfter, Validation: edit.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}}, AuditID: "audit-1"}
	return TransactionSnapshot{
		FormatVersion: 1, SchemaVersion: edit.SchemaVersion, Key: key, Generation: generation,
		ResourceVersion: resourceVersion, TransactionID: transactionID, GraphPayload: []byte("name: example\n"),
		Meta: envelopeFixture("meta"), History: envelopeFixture("history"), ImportResult: envelopeFixture("import"), RollbackResult: envelopeFixture("rollback"),
		Receipt: receipt, ValidationBasis: basis, Checkpoint: envelopeFixture("checkpoint"), Inverse: envelopeFixture("inverse"),
		ApprovalConsume: envelopeFixture("approval-consume"), Audit: envelopeFixture("audit"),
		Events: []TypedEnvelope{envelopeFixture("event")}, Outbox: []TypedEnvelope{envelopeFixture("outbox")},
		Receipts:    []PersistedReceipt{{IdempotencyKeyHash: "idempotency-hash-" + transactionID, RequestDigest: "request-digest-" + transactionID, Receipt: receipt}},
		CommittedAt: time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC),
	}
}

func envelopeFixture(kind string) TypedEnvelope {
	return TypedEnvelope{Kind: kind, SchemaVersion: "test.v1", Payload: json.RawMessage(`{"value":"ok"}`)}
}

func transactionSnapshotTestKey() edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
}

func mustNamespacePath(t *testing.T, namespace *WorkflowNamespace, relative string) string {
	t.Helper()
	path, err := namespace.Path(relative)
	if err != nil {
		t.Fatalf("Path(%q) error = %v", relative, err)
	}
	return path
}
