package selfopt

import "fmt"

type RegressionGate struct {
	policy GatePolicy
}

func DefaultGatePolicy() GatePolicy {
	return GatePolicy{
		CaseScoreDropThreshold:  -0.05,
		SuiteScoreDropThreshold: -0.03,
	}
}

func NewRegressionGate(policy GatePolicy) RegressionGate {
	return RegressionGate{policy: policy}
}

func (g RegressionGate) Evaluate(comparisons []CaseComparison, vetoes []Veto) GateResult {
	var reasons []string
	for _, veto := range vetoes {
		if veto.Severity == "P0" {
			reasons = append(reasons, "P0 veto: "+veto.Name)
		}
	}
	for _, comparison := range comparisons {
		delta := comparison.CurrentScore - comparison.BaselineScore
		if comparison.Delta != 0 {
			delta = comparison.Delta
		}
		if comparison.Priority == "P0" && comparison.BaselinePassed && !comparison.CurrentPassed {
			reasons = append(reasons, "P0 case regressed: "+comparison.CaseID)
			continue
		}
		if delta <= g.policy.CaseScoreDropThreshold {
			reasons = append(reasons, fmt.Sprintf("case score regressed: %s %.2f", comparison.CaseID, delta))
		}
		for _, check := range comparison.RegressedChecks {
			reasons = append(reasons, "check regressed: "+comparison.CaseID+" "+check)
		}
	}
	if len(reasons) > 0 {
		return GateResult{Decision: GateBlock, Reasons: reasons}
	}
	return GateResult{Decision: GatePass}
}

func compareCaseScores(cases []Case, scores []CaseScore) []CaseComparison {
	priorityByID := map[string]string{}
	for _, c := range cases {
		priorityByID[c.ID] = c.Priority
	}
	out := make([]CaseComparison, 0, len(scores))
	for _, score := range scores {
		out = append(out, CaseComparison{
			CaseID:         score.CaseID,
			Priority:       priorityByID[score.CaseID],
			BaselinePassed: true,
			CurrentPassed:  score.Passed,
			BaselineScore:  score.Score,
			CurrentScore:   score.Score,
			Delta:          0,
			Movement:       "same",
		})
	}
	return out
}
