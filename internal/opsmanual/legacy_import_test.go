package opsmanual

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacyJSONFilesIfEmptyImportsManualsCandidatesAndRunRecords(t *testing.T) {
	dataDir := t.TempDir()
	manual := legacyImportPGManual()
	candidate := ManualCandidate{
		ID:             "candidate-pg-backup",
		SourceType:     "workflow",
		ProposedManual: manual,
		ReviewStatus:   "pending",
	}
	record := RunRecord{
		ID:              "run-pg-backup-1",
		ManualID:        manual.ID,
		WorkflowID:      manual.WorkflowRef.WorkflowID,
		ExecutionStatus: "passed",
	}
	writeLegacyJSON(t, filepath.Join(dataDir, "ops-manuals.json"), []OpsManual{manual})
	writeLegacyJSON(t, filepath.Join(dataDir, "ops-manual-candidates.json"), []ManualCandidate{candidate})
	writeLegacyJSON(t, filepath.Join(dataDir, "ops-manual-run-records.json"), []RunRecord{record})

	repo := NewMemoryStore()
	summary, err := ImportLegacyJSONFilesIfEmpty(repo, dataDir)
	if err != nil {
		t.Fatalf("ImportLegacyJSONFilesIfEmpty() error = %v", err)
	}
	if summary.ManualsImported != 1 || summary.CandidatesImported != 1 || summary.RunRecordsImported != 1 {
		t.Fatalf("summary = %#v, want one imported item for each legacy file", summary)
	}
	manuals, err := repo.ListManuals(ListManualsRequest{Status: ManualStatusVerified})
	if err != nil {
		t.Fatalf("ListManuals() error = %v", err)
	}
	if len(manuals) != 1 || manuals[0].ID != "manual-pg-backup-ubuntu" {
		t.Fatalf("manuals = %#v, want imported pg manual", manuals)
	}
	candidates, err := repo.ListCandidates()
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "candidate-pg-backup" {
		t.Fatalf("candidates = %#v, want imported candidate", candidates)
	}
	records, err := repo.ListRunRecords(ListRunRecordsRequest{ManualID: manual.ID})
	if err != nil {
		t.Fatalf("ListRunRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != "run-pg-backup-1" {
		t.Fatalf("records = %#v, want imported run record", records)
	}
}

func TestImportLegacyJSONFilesIfEmptyDoesNotOverwriteExistingManuals(t *testing.T) {
	dataDir := t.TempDir()
	legacy := legacyImportPGManual()
	writeLegacyJSON(t, filepath.Join(dataDir, "ops-manuals.json"), []OpsManual{legacy})

	repo := NewMemoryStore()
	existing := legacyImportPGManual()
	existing.ID = "manual-existing"
	existing.Title = "Existing manual"
	if err := repo.SaveManual(existing); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}

	summary, err := ImportLegacyJSONFilesIfEmpty(repo, dataDir)
	if err != nil {
		t.Fatalf("ImportLegacyJSONFilesIfEmpty() error = %v", err)
	}
	if !summary.Skipped || summary.ManualsImported != 0 {
		t.Fatalf("summary = %#v, want skipped with no imports", summary)
	}
	manuals, err := repo.ListManuals(ListManualsRequest{})
	if err != nil {
		t.Fatalf("ListManuals() error = %v", err)
	}
	if len(manuals) != 1 || manuals[0].ID != "manual-existing" {
		t.Fatalf("manuals = %#v, want existing manual untouched", manuals)
	}
}

func writeLegacyJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func legacyImportPGManual() OpsManual {
	return OpsManual{
		ID:     "manual-pg-backup-ubuntu",
		Title:  "PostgreSQL 备份 Ubuntu 运维手册",
		Status: ManualStatusVerified,
		WorkflowRef: WorkflowRef{
			WorkflowID:      "workflow-pg-backup-ubuntu",
			WorkflowVersion: "v3",
		},
		Operation: OperationProfile{TargetType: "postgresql", Action: "backup", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "postgresql",
			OS:               []string{"ubuntu"},
			Platform:         []string{"vm"},
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance", "backup_path"},
			RequiredEvidence: []string{"ssh_access", "pg_isready"},
		},
		Preconditions:    []string{"SSH access is available"},
		Validation:       []string{"backup file exists"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "用于通过 SSH 执行 PostgreSQL 备份。",
	}
}
