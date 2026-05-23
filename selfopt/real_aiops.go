package selfopt

import "aiops-v2/selfopt/reportreader"

func LoadAIOpsTestSummary(runDir string) (AIOpsTestSummary, error) {
	read, err := reportreader.ReadRun(runDir)
	if err != nil {
		return AIOpsTestSummary{}, err
	}
	summary := AIOpsTestSummary{RunDir: read.RunDir}
	for _, report := range read.Reports {
		converted := AIOpsTestReport{
			Name:       report.Name,
			Total:      report.Total,
			Passed:     report.Passed,
			Failed:     report.Failed,
			AvgScore:   report.AvgScore,
			Worse:      report.Worse,
			ReportPath: report.ReportPath,
			DiagPath:   report.DiagPath,
		}
		for _, failed := range report.FailedCases {
			converted.FailedCases = append(converted.FailedCases, AIOpsFailedCase{
				CaseID:          failed.CaseID,
				Score:           failed.Score,
				Movement:        failed.Movement,
				LikelyRootCause: failed.LikelyRootCause,
				FailedChecks:    append([]string(nil), failed.FailedChecks...),
			})
		}
		summary.Reports = append(summary.Reports, converted)
		summary.Total += report.Total
		summary.Passed += report.Passed
		summary.Failed += report.Failed
		summary.Worse += report.Worse
	}
	return summary, nil
}

func applyAIOpsTestGate(scorecard *Scorecard, summary AIOpsTestSummary) {
	if summary.Failed == 0 && summary.Worse == 0 {
		return
	}
	scorecard.Vetoes = append(scorecard.Vetoes, Veto{
		Name:     "real_aiops_tests",
		Severity: "P0",
		Detail:   "real aiops prompt regression reported failed or worse cases",
	})
	scorecard.Gate.Decision = GateBlock
	scorecard.Gate.Reasons = append(scorecard.Gate.Reasons, "real aiops tests failed or regressed")
}
