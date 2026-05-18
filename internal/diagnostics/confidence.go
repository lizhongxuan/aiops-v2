package diagnostics

type ConfidenceInput struct {
	ScopeConfirmed        bool
	HasDirectSupport      bool
	CriticalRefuteChecked bool
	HasCriticalMissing    bool
	HasToolFailure        bool
	HasStaleContext       bool
	RequiresApproval      bool
}

func CalibrateConfidence(input ConfidenceInput) ConfidenceLevel {
	if !input.ScopeConfirmed || input.HasStaleContext {
		return ConfidenceLow
	}
	if !input.HasDirectSupport || !input.CriticalRefuteChecked {
		return ConfidenceLow
	}
	if input.HasToolFailure || input.HasCriticalMissing || input.RequiresApproval {
		return ConfidenceMedium
	}
	return ConfidenceHigh
}
