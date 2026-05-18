package opsmanual

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type LegacyImportSummary struct {
	ManualsImported    int  `json:"manuals_imported"`
	CandidatesImported int  `json:"candidates_imported,omitempty"`
	RunRecordsImported int  `json:"run_records_imported,omitempty"`
	Skipped            bool `json:"skipped,omitempty"`
}

// ImportLegacyJSONFilesIfEmpty imports the legacy JSONFileStore ops manual
// files into a new repository, primarily for PostgreSQL migrations. Existing
// repositories are never overwritten.
func ImportLegacyJSONFilesIfEmpty(repo ManualRepository, dataDir string) (LegacyImportSummary, error) {
	if repo == nil {
		return LegacyImportSummary{}, fmt.Errorf("manual repository is nil")
	}
	if dataDir == "" {
		dataDir = ".data"
	}
	existing, err := repo.ListManuals(ListManualsRequest{})
	if err != nil {
		return LegacyImportSummary{}, err
	}
	if len(existing) > 0 {
		return LegacyImportSummary{Skipped: true}, nil
	}

	var summary LegacyImportSummary
	manuals, err := loadLegacyJSONArray[OpsManual](filepath.Join(dataDir, "ops-manuals.json"))
	if err != nil {
		return LegacyImportSummary{}, err
	}
	for _, manual := range manuals {
		if manual.ID == "" {
			continue
		}
		if err := repo.SaveManual(cloneManual(manual)); err != nil {
			return LegacyImportSummary{}, err
		}
		summary.ManualsImported++
	}

	if candidateRepo, ok := repo.(CandidateRepository); ok {
		candidates, err := loadLegacyJSONArray[ManualCandidate](filepath.Join(dataDir, "ops-manual-candidates.json"))
		if err != nil {
			return LegacyImportSummary{}, err
		}
		for _, candidate := range candidates {
			if candidate.ID == "" {
				continue
			}
			if err := candidateRepo.SaveCandidate(cloneCandidate(candidate)); err != nil {
				return LegacyImportSummary{}, err
			}
			summary.CandidatesImported++
		}
	}

	if runRepo, ok := repo.(RunRecordRepository); ok {
		records, err := loadLegacyJSONArray[RunRecord](filepath.Join(dataDir, "ops-manual-run-records.json"))
		if err != nil {
			return LegacyImportSummary{}, err
		}
		for _, record := range records {
			if record.ID == "" {
				continue
			}
			if err := runRepo.SaveRunRecord(cloneRunRecord(record)); err != nil {
				return LegacyImportSummary{}, err
			}
			summary.RunRecordsImported++
		}
	}
	if summary.ManualsImported == 0 && summary.CandidatesImported == 0 && summary.RunRecordsImported == 0 {
		summary.Skipped = true
	}
	return summary, nil
}

func loadLegacyJSONArray[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("read legacy ops manual file %s: %w", path, err)
	}
	return items, nil
}
