package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWorkflowServiceCommitStoreIsSoleWritePathAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	svc := NewWorkflowService(root)
	if err := svc.Create(context.Background(), &WorkflowRecord{Name: "restart", RawYAML: []byte(testWorkflowYAML("restart", "echo one")), Labels: map[string]string{"revision": "one"}, SaveNote: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Update(context.Background(), "restart", &WorkflowRecord{Name: "restart", RawYAML: []byte(testWorkflowYAML("restart", "echo two")), Labels: map[string]string{"revision": "two"}, SaveNote: "two"}); err != nil {
		t.Fatal(err)
	}
	for _, legacyPath := range []string{filepath.Join(root, "restart.yaml"), filepath.Join(root, ".meta.json"), filepath.Join(root, ".history")} {
		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Fatalf("legacy write path exists at %s: %v", legacyPath, err)
		}
	}

	restarted := NewWorkflowService(root)
	record, err := restarted.Get(context.Background(), "restart")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(record.RawYAML), "echo two") || record.Labels["revision"] != "two" || record.SaveNote != "two" {
		t.Fatalf("restarted snapshot mixed graph and metadata: %+v\n%s", record, record.RawYAML)
	}
	versions, err := restarted.ListVersions(context.Background(), "restart")
	if err != nil || len(versions) != 2 {
		t.Fatalf("restarted history = %+v, %v", versions, err)
	}
}

func TestWorkflowServiceReadersNeverObserveMixedGraphAndMetadata(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{Name: "atomic-read", RawYAML: []byte(testWorkflowYAML("atomic-read", "echo r0")), Labels: map[string]string{"revision": "r0"}, SaveNote: "r0"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 8)
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for ctx.Err() == nil {
				record, err := svc.Get(ctx, "atomic-read")
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					errCh <- err
					return
				}
				revision := record.Labels["revision"]
				if record.SaveNote != revision || !strings.Contains(string(record.RawYAML), "echo "+revision) {
					errCh <- errors.New("reader observed mixed transaction fields")
					return
				}
			}
		}()
	}
	for i := 1; i <= 8; i++ {
		revision := "r" + string(rune('0'+i))
		if err := svc.Update(context.Background(), "atomic-read", &WorkflowRecord{Name: "atomic-read", RawYAML: []byte(testWorkflowYAML("atomic-read", "echo "+revision)), Labels: map[string]string{"revision": revision}, SaveNote: revision}); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	readers.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestWorkflowImportAndRollbackArtifactsShareCurrentSnapshot(t *testing.T) {
	source := NewWorkflowService(t.TempDir())
	if err := source.Create(context.Background(), &WorkflowRecord{Name: "artifacts", RawYAML: []byte(testWorkflowYAML("artifacts", "echo initial"))}); err != nil {
		t.Fatal(err)
	}
	if err := source.Update(context.Background(), "artifacts", &WorkflowRecord{Name: "artifacts", RawYAML: []byte(testWorkflowYAML("artifacts", "echo updated"))}); err != nil {
		t.Fatal(err)
	}
	bundle, err := source.ExportBundle(context.Background(), "artifacts")
	if err != nil {
		t.Fatal(err)
	}
	target := NewWorkflowService(t.TempDir())
	if _, err := target.ImportBundle(context.Background(), bundle, WorkflowImportOptions{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := target.commitStore.Snapshot(context.Background(), target.workflowKey("artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	var imported struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(snapshot.ImportResult.Payload, &imported); err != nil || !imported.Applied {
		t.Fatalf("import artifact = %s, %v", snapshot.ImportResult.Payload, err)
	}
	versions, err := target.ListVersions(context.Background(), "artifacts")
	if err != nil || len(versions) < 2 {
		t.Fatalf("imported versions = %+v, %v", versions, err)
	}
	if _, err := target.Rollback(context.Background(), "artifacts", versions[len(versions)-1].ID, WorkflowRollbackOptions{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = target.commitStore.Snapshot(context.Background(), target.workflowKey("artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	var rolledBack struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(snapshot.RollbackResult.Payload, &rolledBack); err != nil || !rolledBack.Applied {
		t.Fatalf("rollback artifact = %s, %v", snapshot.RollbackResult.Payload, err)
	}
}
