package diagnostics

func BuildTrace(input TraceBuildInput) DiagnosticTrace {
	if isEmptyInput(input) {
		return DiagnosticTrace{}
	}

	scopeHash := input.CurrentScope.Hash
	trace := DiagnosticTrace{
		TurnID:           input.TurnID,
		ScopeHash:        scopeHash,
		ScopeSummary:     input.CurrentScope.Summary,
		ManualBindingID:  input.ManualBindingID,
		RequiresApproval: input.RequiresApproval,
	}

	for _, hypothesis := range input.Hypotheses {
		if belongsToScope(hypothesis.ScopeHash, scopeHash) && hypothesis.Text != "" {
			trace.Hypotheses = append(trace.Hypotheses, hypothesis.Text)
		}
	}

	confidenceInput := ConfidenceInput{
		ScopeConfirmed:   input.CurrentScope.Confirmed,
		RequiresApproval: input.RequiresApproval,
	}

	for _, fact := range input.Facts {
		if !belongsToScope(fact.ScopeHash, scopeHash) || fact.Summary == "" {
			continue
		}
		switch fact.Status {
		case "", EvidenceStatusActive:
			trace.ObservedEvidence = append(trace.ObservedEvidence, fact.Summary)
			if fact.DirectSupport {
				confidenceInput.HasDirectSupport = true
			}
		case EvidenceStatusStale:
			trace.MissingEvidence = append(trace.MissingEvidence, fact.Summary)
			confidenceInput.HasStaleContext = true
			if fact.Critical {
				confidenceInput.HasCriticalMissing = true
			}
		case EvidenceStatusBlocked, EvidenceStatusMissing:
			trace.MissingEvidence = append(trace.MissingEvidence, fact.Summary)
			if fact.Critical {
				confidenceInput.HasCriticalMissing = true
			}
		default:
			trace.MissingEvidence = append(trace.MissingEvidence, fact.Summary)
		}
	}

	for _, evidence := range input.RefutingEvidence {
		if belongsToScope(evidence.ScopeHash, scopeHash) && evidence.Text != "" {
			trace.RefutingEvidence = append(trace.RefutingEvidence, evidence.Text)
			confidenceInput.CriticalRefuteChecked = true
		}
	}

	for _, failure := range input.ToolFailures {
		if !belongsToScope(failure.ScopeHash, scopeHash) {
			continue
		}
		trace.ToolFailures = append(trace.ToolFailures, failure)
		if isCriticalToolFailure(failure) {
			confidenceInput.HasToolFailure = true
		}
	}

	if input.ManualBindingIsStale {
		confidenceInput.HasStaleContext = true
		if input.ManualBindingID == "" {
			trace.MissingEvidence = append(trace.MissingEvidence, "manual binding is stale")
		} else {
			trace.MissingEvidence = append(trace.MissingEvidence, "manual binding "+input.ManualBindingID+" is stale")
		}
	}

	trace.Confidence = CalibrateConfidence(confidenceInput)
	trace.ConfidenceReason = confidenceReason(confidenceInput)

	return RedactTrace(trace)
}

func isEmptyInput(input TraceBuildInput) bool {
	return input.TurnID == "" &&
		input.CurrentScope == (DiagnosticScope{}) &&
		len(input.Facts) == 0 &&
		len(input.Hypotheses) == 0 &&
		len(input.RefutingEvidence) == 0 &&
		len(input.ToolFailures) == 0 &&
		input.ManualBindingID == "" &&
		!input.ManualBindingIsStale &&
		!input.RequiresApproval
}

func belongsToScope(itemScopeHash, currentScopeHash string) bool {
	return itemScopeHash == "" || currentScopeHash == "" || itemScopeHash == currentScopeHash
}

func isCriticalToolFailure(failure ToolFailure) bool {
	if !failure.Critical {
		return false
	}
	switch failure.Semantic {
	case ToolFailurePolicyBlocked, ToolFailureCommandNotAllowed, ToolFailurePermissionDenied, ToolFailureTimeout:
		return true
	default:
		return false
	}
}

func confidenceReason(input ConfidenceInput) string {
	switch {
	case !input.ScopeConfirmed:
		return "scope is not confirmed"
	case input.HasStaleContext:
		return "context or manual binding is stale"
	case !input.HasDirectSupport:
		return "direct supporting evidence is missing"
	case !input.CriticalRefuteChecked:
		return "critical refuting evidence is not checked"
	case input.HasToolFailure:
		return "critical tool failure blocks high confidence"
	case input.HasCriticalMissing:
		return "critical evidence is missing"
	case input.RequiresApproval:
		return "validation requires approval"
	default:
		return "scope confirmed with direct support and critical refute checks"
	}
}
