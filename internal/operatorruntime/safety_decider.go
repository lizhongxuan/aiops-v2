package operatorruntime

type SafetyDecision struct {
	Decision GuardRunDecision `json:"decision"`
	Reason   string           `json:"reason"`
}

func DecideSafety(rule GuardRule, action ActionCatalogItem, target ResourceEndpoint) SafetyDecision {
	if rule.Policy.Paused {
		return SafetyDecision{Decision: DecisionBlocked, Reason: "guard rule is paused"}
	}
	if target.Role == ResourceRolePrimary {
		return SafetyDecision{Decision: DecisionBlocked, Reason: "repair target must not be primary"}
	}
	if !riskAllowed(action.RiskLevel, rule.Policy.MaxAutoRisk) {
		return SafetyDecision{Decision: DecisionBlocked, Reason: "action risk exceeds auto policy"}
	}
	approvalKinds := map[ActionStepKind]bool{}
	for _, kind := range rule.Policy.RequireApprovalStepKinds {
		approvalKinds[kind] = true
	}
	for _, step := range action.Steps {
		if step.RequiresApproval || approvalKinds[step.Kind] {
			return SafetyDecision{Decision: DecisionRequiresApproval, Reason: "action contains approval-required step"}
		}
	}
	return SafetyDecision{Decision: DecisionAuto, Reason: "action is within auto policy"}
}

func DecideSafetyForCluster(rule GuardRule, action ActionCatalogItem, cluster PGCluster, target PGInstance) SafetyDecision {
	return DecideSafetyForResource(rule, action, cluster, target)
}

func DecideSafetyForResource(rule GuardRule, action ActionCatalogItem, resource ManagedResource, target ResourceEndpoint) SafetyDecision {
	if ResourceRepairCredentialRef(resource) == "" {
		return SafetyDecision{Decision: DecisionBlocked, Reason: "repair credential is missing"}
	}
	return DecideSafety(rule, action, target)
}

func riskAllowed(actual, max RiskLevel) bool {
	order := map[RiskLevel]int{RiskReadonly: 0, RiskLow: 1, RiskMedium: 2, RiskHigh: 3, RiskCritical: 4}
	return order[actual] <= order[max]
}
