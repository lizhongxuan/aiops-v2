package reportreader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RunSummary struct {
	RunDir  string          `json:"runDir"`
	Reports []ReportSummary `json:"reports"`
}

type ReportSummary struct {
	Name        string       `json:"name"`
	Total       int          `json:"total"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	AvgScore    float64      `json:"avgScore"`
	Worse       int          `json:"worse,omitempty"`
	FailedCases []FailedCase `json:"failedCases,omitempty"`
	ReportPath  string       `json:"reportPath,omitempty"`
	DiagPath    string       `json:"diagnosisPath,omitempty"`
}

type FailedCase struct {
	CaseID          string   `json:"caseId"`
	Score           float64  `json:"score"`
	Movement        string   `json:"movement,omitempty"`
	LikelyRootCause string   `json:"likelyRootCause,omitempty"`
	FailedChecks    []string `json:"failedChecks,omitempty"`
}

func ReadLatest(root string) (RunSummary, error) {
	raw, err := os.ReadFile(filepath.Join(root, "latest_run.txt"))
	if err != nil {
		return RunSummary{}, err
	}
	runDir := strings.TrimSpace(string(raw))
	if runDir == "" {
		return RunSummary{}, fmt.Errorf("latest_run.txt is empty")
	}
	return ReadRun(runDir)
}

func ReadRun(runDir string) (RunSummary, error) {
	pattern := filepath.Join(runDir, "prompt-regression-*", "eval", "report.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return RunSummary{}, err
	}
	if len(paths) == 0 {
		return RunSummary{}, fmt.Errorf("no prompt regression reports found under %s", runDir)
	}
	summary := RunSummary{RunDir: runDir}
	for _, path := range paths {
		report, err := readReport(path)
		if err != nil {
			return RunSummary{}, err
		}
		name := filepath.Base(filepath.Dir(filepath.Dir(path)))
		report.Name = name
		report.ReportPath = path
		diagPath := filepath.Join(filepath.Dir(filepath.Dir(path)), "diagnosis.json")
		if diag, err := readDiagnosis(diagPath); err == nil {
			report.DiagPath = diagPath
			if diag.Worse > report.Worse {
				report.Worse = diag.Worse
			}
			mergeDiagnosis(&report, diag)
		}
		summary.Reports = append(summary.Reports, report)
	}
	return summary, nil
}

func readReport(path string) (ReportSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReportSummary{}, err
	}
	var payload struct {
		Summary struct {
			Total    int     `json:"total"`
			Passed   int     `json:"passed"`
			Failed   int     `json:"failed"`
			AvgScore float64 `json:"avgScore"`
		} `json:"summary"`
		Cases []struct {
			CaseID string  `json:"caseId"`
			Passed bool    `json:"passed"`
			Score  float64 `json:"score"`
			Checks []struct {
				Name    string   `json:"name"`
				Passed  bool     `json:"passed"`
				Missing []string `json:"missing"`
			} `json:"checks"`
		} `json:"cases"`
		BaselineComparison struct {
			Summary struct {
				Worse int `json:"worse"`
			} `json:"summary"`
			Cases []struct {
				CaseID          string   `json:"caseId"`
				CurrentScore    float64  `json:"currentScore"`
				Status          string   `json:"status"`
				RegressedChecks []string `json:"regressedChecks"`
			} `json:"cases"`
		} `json:"baselineComparison"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ReportSummary{}, err
	}
	out := ReportSummary{
		Total:    payload.Summary.Total,
		Passed:   payload.Summary.Passed,
		Failed:   payload.Summary.Failed,
		AvgScore: payload.Summary.AvgScore,
		Worse:    payload.BaselineComparison.Summary.Worse,
	}
	caseScores := map[string]float64{}
	for _, c := range payload.Cases {
		caseScores[c.CaseID] = c.Score
		if c.Passed {
			continue
		}
		failed := FailedCase{CaseID: c.CaseID, Score: c.Score}
		for _, check := range c.Checks {
			if !check.Passed {
				failed.FailedChecks = append(failed.FailedChecks, check.Name)
			}
		}
		out.FailedCases = append(out.FailedCases, failed)
	}
	for _, c := range payload.BaselineComparison.Cases {
		if c.Status != "worse" {
			continue
		}
		score := c.CurrentScore
		if score == 0 {
			score = caseScores[c.CaseID]
		}
		upsertCase(&out, FailedCase{
			CaseID:       c.CaseID,
			Score:        score,
			Movement:     c.Status,
			FailedChecks: c.RegressedChecks,
		})
	}
	return out, nil
}

type diagnosisSummary struct {
	Worse int
	Cases []FailedCase
}

func readDiagnosis(path string) (diagnosisSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return diagnosisSummary{}, err
	}
	var payload struct {
		Summary struct {
			Worse int `json:"worse"`
		} `json:"summary"`
		Cases []struct {
			CaseID          string   `json:"caseId"`
			Passed          bool     `json:"passed"`
			Score           float64  `json:"score"`
			Movement        string   `json:"movement"`
			LikelyRootCause string   `json:"likelyRootCause"`
			FailedChecks    []string `json:"failedChecks"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return diagnosisSummary{}, err
	}
	out := diagnosisSummary{Worse: payload.Summary.Worse}
	for _, c := range payload.Cases {
		if c.Passed && c.Movement != "worse" {
			continue
		}
		out.Cases = append(out.Cases, FailedCase{
			CaseID:          c.CaseID,
			Score:           c.Score,
			Movement:        c.Movement,
			LikelyRootCause: c.LikelyRootCause,
			FailedChecks:    c.FailedChecks,
		})
	}
	return out, nil
}

func mergeDiagnosis(report *ReportSummary, diag diagnosisSummary) {
	byCase := map[string]int{}
	for i, c := range report.FailedCases {
		byCase[c.CaseID] = i
	}
	for _, c := range diag.Cases {
		if i, ok := byCase[c.CaseID]; ok {
			if c.Movement != "" {
				report.FailedCases[i].Movement = c.Movement
			}
			if c.Score != 0 {
				report.FailedCases[i].Score = c.Score
			}
			if c.LikelyRootCause != "" {
				report.FailedCases[i].LikelyRootCause = c.LikelyRootCause
			}
			if len(c.FailedChecks) > 0 {
				report.FailedCases[i].FailedChecks = c.FailedChecks
			}
			continue
		}
		report.FailedCases = append(report.FailedCases, c)
	}
}

func upsertCase(report *ReportSummary, c FailedCase) {
	for i, existing := range report.FailedCases {
		if existing.CaseID != c.CaseID {
			continue
		}
		if c.Score != 0 {
			report.FailedCases[i].Score = c.Score
		}
		if c.Movement != "" {
			report.FailedCases[i].Movement = c.Movement
		}
		if len(c.FailedChecks) > 0 {
			report.FailedCases[i].FailedChecks = c.FailedChecks
		}
		if c.LikelyRootCause != "" {
			report.FailedCases[i].LikelyRootCause = c.LikelyRootCause
		}
		return
	}
	report.FailedCases = append(report.FailedCases, c)
}
