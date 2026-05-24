package runtimekernel

// ContextBudgetPolicy describes the token budget used by the L1-L5 context
// governance helpers.
type ContextBudgetPolicy struct {
	MaxContextTokens              int `json:"maxContextTokens"`
	ModelMaxOutputTokens          int `json:"modelMaxOutputTokens"`
	ReservedOutputTokens          int `json:"reservedOutputTokens"`
	AutoCompactBufferTokens       int `json:"autoCompactBufferTokens"`
	WarningBufferTokens           int `json:"warningBufferTokens"`
	BlockingBufferTokens          int `json:"blockingBufferTokens"`
	MaxConsecutiveCompactFailures int `json:"maxConsecutiveCompactFailures"`
}

// ContextBudgetThresholds is the computed, model-facing budget envelope.
type ContextBudgetThresholds struct {
	MaxContextTokens       int  `json:"maxContextTokens"`
	ReservedOutputTokens   int  `json:"reservedOutputTokens"`
	EffectiveContextWindow int  `json:"effectiveContextWindow"`
	WarningThreshold       int  `json:"warningThreshold"`
	AutoCompactThreshold   int  `json:"autoCompactThreshold"`
	BlockingLimit          int  `json:"blockingLimit"`
	SmallContextMode       bool `json:"smallContextMode"`
}

// DefaultContextBudgetPolicy returns the conservative default policy for a
// model window. Windows at or below 32K automatically enter small-context mode.
func DefaultContextBudgetPolicy(maxContextTokens, modelMaxOutputTokens int) ContextBudgetPolicy {
	if maxContextTokens <= 0 {
		maxContextTokens = DefaultMaxTokens
	}
	if modelMaxOutputTokens <= 0 {
		modelMaxOutputTokens = 20000
	}
	policy := ContextBudgetPolicy{
		MaxContextTokens:              maxContextTokens,
		ModelMaxOutputTokens:          modelMaxOutputTokens,
		ReservedOutputTokens:          contextPolicyMinInt(modelMaxOutputTokens, 20000),
		AutoCompactBufferTokens:       13000,
		WarningBufferTokens:           20000,
		BlockingBufferTokens:          3000,
		MaxConsecutiveCompactFailures: 3,
	}
	if maxContextTokens <= 32000 {
		reserved := contextPolicyClampInt(maxContextTokens*15/100, 2000, 6000)
		effective := maxContextTokens - reserved
		policy.ReservedOutputTokens = reserved
		policy.AutoCompactBufferTokens = contextPolicyClampInt(effective*12/100, 1500, 4000)
		policy.WarningBufferTokens = contextPolicyClampInt(effective*10/100, 1000, 3000)
		policy.BlockingBufferTokens = contextPolicyClampInt(effective*6/100, 800, 2000)
	}
	return policy
}

// Thresholds computes the concrete context thresholds from the policy.
func (p ContextBudgetPolicy) Thresholds() ContextBudgetThresholds {
	if p.MaxContextTokens <= 0 || p.ReservedOutputTokens <= 0 {
		p = DefaultContextBudgetPolicy(p.MaxContextTokens, p.ModelMaxOutputTokens)
	}
	effective := p.MaxContextTokens - p.ReservedOutputTokens
	if effective < 1 {
		effective = 1
	}
	return ContextBudgetThresholds{
		MaxContextTokens:       p.MaxContextTokens,
		ReservedOutputTokens:   p.ReservedOutputTokens,
		EffectiveContextWindow: effective,
		WarningThreshold:       contextPolicyFloorAtZero(effective - p.AutoCompactBufferTokens - p.WarningBufferTokens),
		AutoCompactThreshold:   contextPolicyFloorAtZero(effective - p.AutoCompactBufferTokens),
		BlockingLimit:          contextPolicyFloorAtZero(effective - p.BlockingBufferTokens),
		SmallContextMode:       p.MaxContextTokens <= 32000,
	}
}

func contextPolicyClampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func contextPolicyMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contextPolicyFloorAtZero(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
