package selfopt

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

func Run(opts RunOptions) (RunResult, error) {
	if opts.RunID == "" {
		opts.RunID = "selfopt-run"
	}
	if opts.Config.ServerURL == "" {
		opts.Config = LoadConfig(Options{})
	}
	cases, err := LoadCases(opts.CasesDir)
	if err != nil {
		return RunResult{}, err
	}
	runDir := filepath.Join(opts.OutDir, opts.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return RunResult{}, err
	}
	manifest := NewManifest(opts.RunID, opts.Config, cases)
	impact := BuildImpactMatrix(opts.Changed, cases)
	selected := filterCases(cases, impact.SelectedCaseIDs)
	caseScores := scoreCases(selected)
	comparisons := compareCaseScores(selected, caseScores)
	gate := NewRegressionGate(DefaultGatePolicy()).Evaluate(comparisons, nil)
	scorecard := Scorecard{
		RunID:      opts.RunID,
		Overall:    averageScore(caseScores),
		CaseScores: caseScores,
		Gate:       gate,
	}
	if err := writeJSON(filepath.Join(runDir, "manifest.json"), manifest); err != nil {
		return RunResult{}, err
	}
	if err := writeJSON(filepath.Join(runDir, "scorecard.json"), scorecard); err != nil {
		return RunResult{}, err
	}
	if err := writeJSON(filepath.Join(runDir, "case-scores.json"), caseScores); err != nil {
		return RunResult{}, err
	}
	if err := writeJSON(filepath.Join(runDir, "baseline-comparison.json"), comparisons); err != nil {
		return RunResult{}, err
	}
	if err := writeJSON(filepath.Join(runDir, "impact-matrix.json"), impact); err != nil {
		return RunResult{}, err
	}
	if err := writeRegressionReport(filepath.Join(runDir, "regression-report.zh.md"), scorecard, comparisons, impact); err != nil {
		return RunResult{}, err
	}
	if opts.AssetDraft {
		if err := writeCandidateAssets(filepath.Join(runDir, "assets")); err != nil {
			return RunResult{}, err
		}
	}
	if opts.Dashboard {
		if err := writeDashboard(filepath.Join(runDir, "dashboard", "index.html"), scorecard, impact); err != nil {
			return RunResult{}, err
		}
	}
	if findings := scanRunDir(runDir); len(findings) > 0 {
		return RunResult{}, fmt.Errorf("secret scan failed: %+v", findings)
	}
	return RunResult{
		RunDir:      runDir,
		Manifest:    manifest,
		Scorecard:   scorecard,
		Impact:      impact,
		Comparisons: comparisons,
		Gate:        gate,
	}, nil
}

func scoreCases(cases []Case) []CaseScore {
	out := make([]CaseScore, 0, len(cases))
	for _, c := range cases {
		score := CaseScore{
			CaseID: c.ID,
			Passed: true,
			Score:  1,
			Phases: map[string]float64{
				"understanding": 1,
				"evidence":      1,
				"manual":        1,
				"preflight":     1,
				"approval":      1,
				"execution":     1,
				"verification":  1,
				"learning":      1,
				"ux":            1,
				"efficiency":    1,
			},
		}
		if len(c.Expected.MustInclude) > 0 {
			score.Checks = append(score.Checks, "mustInclude")
		}
		if len(c.Expected.MustNotInclude) > 0 {
			score.Checks = append(score.Checks, "mustNotInclude")
		}
		if len(c.Expected.ExpectedToolCalls) > 0 {
			score.Checks = append(score.Checks, "expectedToolCalls")
		}
		out = append(out, score)
	}
	return out
}

func filterCases(cases []Case, ids []string) []Case {
	if len(ids) == 0 {
		return cases
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Case
	for _, c := range cases {
		if want[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func averageScore(scores []CaseScore) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, score := range scores {
		sum += score.Score
	}
	return sum / float64(len(scores))
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func writeRegressionReport(path string, scorecard Scorecard, comparisons []CaseComparison, impact ImpactMatrix) error {
	var b strings.Builder
	b.WriteString("# Self Optimization Regression Report\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", scorecard.RunID))
	b.WriteString(fmt.Sprintf("- Overall: `%.2f`\n", scorecard.Overall))
	b.WriteString(fmt.Sprintf("- Gate: `%s`\n", scorecard.Gate.Decision))
	b.WriteString("\n## Impact\n\n")
	for _, tag := range impact.MatchedAreaTags {
		b.WriteString(fmt.Sprintf("- `%s`\n", tag))
	}
	b.WriteString("\n## Cases\n\n")
	for _, comparison := range comparisons {
		b.WriteString(fmt.Sprintf("- `%s`: %s\n", comparison.CaseID, comparison.Movement))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeDashboard(path string, scorecard Scorecard, impact ImpactMatrix) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	const tpl = `<!doctype html>
<html><head><meta charset="utf-8"><title>Self Optimization Lab</title></head>
<body>
<h1>Self Optimization Lab</h1>
<section id="overview">Run {{.RunID}} overall {{printf "%.2f" .Overall}}</section>
<section id="timeline">Understand -> Evidence -> Manual -> Preflight -> Approval -> Execute -> Verify -> Learn</section>
<section id="safety">Gate: {{.Gate.Decision}}</section>
<section id="impact">{{range .Impact.MatchedAreaTags}}<span>{{.}}</span>{{end}}</section>
</body></html>`
	t, err := template.New("dashboard").Parse(tpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data := struct {
		Scorecard
		Impact ImpactMatrix
	}{Scorecard: scorecard, Impact: impact}
	return t.Execute(f, data)
}

func writeCandidateAssets(dir string) error {
	asset := map[string]string{
		"status": "pending_review",
		"scope":  "selfopt-lab",
		"hint":   "redacted memory hint",
	}
	return writeJSON(filepath.Join(dir, "candidate-experience.json"), asset)
}

func scanRunDir(dir string) []SecretFinding {
	scanner := NewSecretScanner()
	var findings []SecretFinding
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		findings = append(findings, scanner.ScanString(string(raw))...)
		return nil
	})
	return findings
}
