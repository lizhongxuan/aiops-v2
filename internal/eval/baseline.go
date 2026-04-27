package eval

import "sort"

const (
	ComparisonBetter  = "better"
	ComparisonWorse   = "worse"
	ComparisonSame    = "same"
	ComparisonNew     = "new"
	ComparisonMissing = "missing"
)

// CompareReports compares current case scores against a saved baseline report.
func CompareReports(baseline, current Report) ComparisonReport {
	baselineByID := make(map[string]CaseScore, len(baseline.Cases))
	currentByID := make(map[string]CaseScore, len(current.Cases))
	ids := make(map[string]bool, len(baseline.Cases)+len(current.Cases))
	for _, c := range baseline.Cases {
		baselineByID[c.CaseID] = c
		ids[c.CaseID] = true
	}
	for _, c := range current.Cases {
		currentByID[c.CaseID] = c
		ids[c.CaseID] = true
	}

	allIDs := make([]string, 0, len(ids))
	for id := range ids {
		allIDs = append(allIDs, id)
	}
	sort.Strings(allIDs)

	report := ComparisonReport{Cases: make([]ComparisonCase, 0, len(allIDs))}
	for _, id := range allIDs {
		base, hasBase := baselineByID[id]
		cur, hasCur := currentByID[id]
		item := ComparisonCase{CaseID: id}
		switch {
		case !hasBase && hasCur:
			item.CurrentScore = cur.Score
			item.CurrentPassed = cur.Passed
			item.Status = ComparisonNew
			report.Summary.New++
		case hasBase && !hasCur:
			item.BaselineScore = base.Score
			item.BaselinePassed = base.Passed
			item.Status = ComparisonMissing
			report.Summary.Missing++
		default:
			item.BaselineScore = base.Score
			item.CurrentScore = cur.Score
			item.Delta = cur.Score - base.Score
			item.BaselinePassed = base.Passed
			item.CurrentPassed = cur.Passed
			switch {
			case item.Delta > 0:
				item.Status = ComparisonBetter
				report.Summary.Better++
			case item.Delta < 0:
				item.Status = ComparisonWorse
				report.Summary.Worse++
			default:
				item.Status = ComparisonSame
				report.Summary.Same++
			}
		}
		report.Cases = append(report.Cases, item)
	}
	return report
}
