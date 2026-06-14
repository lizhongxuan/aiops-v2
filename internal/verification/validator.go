package verification

type ValidationDecision struct {
	Valid       bool                    `json:"valid"`
	Status      ReportStatus            `json:"status,omitempty"`
	Requirement VerificationRequirement `json:"requirement,omitempty"`
	Reasons     []string                `json:"reasons,omitempty"`
}

func ValidateReport(report VerificationReport) ValidationDecision {
	report = report.Normalize()
	decision := ValidationDecision{
		Valid:       true,
		Status:      report.Status,
		Requirement: report.Requirement,
	}

	addReason := func(reason string) {
		for _, existing := range decision.Reasons {
			if existing == reason {
				return
			}
		}
		decision.Valid = false
		decision.Reasons = append(decision.Reasons, reason)
	}

	if report.ID == "" {
		addReason("missing_id")
	}
	if !isKnownRequirement(report.Requirement) {
		addReason("unknown_requirement")
	}
	if !isKnownStatus(report.Status) {
		addReason("unknown_status")
	}
	if report.Subject == "" {
		addReason("missing_subject")
	}
	if report.Requirement != VerificationNone && len(report.Evidence) == 0 {
		addReason("missing_evidence")
	}

	validateEvidence(report.Evidence, addReason)
	validateProbes(report.Probes, addReason)
	validateContractChecks(report.ContractChecks, addReason)

	switch report.Status {
	case StatusPass:
		validatePassReport(report, addReason)
	case StatusPartial:
		validatePartialReport(report, addReason)
	case StatusFail:
		validateFailReport(report, addReason)
	}

	return decision
}

func validatePassReport(report VerificationReport, addReason func(string)) {
	if report.Requirement == VerificationExecutionRequired && !hasPassingExecutionEvidence(report.Evidence) {
		addReason("missing_execution_evidence")
	}
	for _, probe := range report.Probes {
		switch probe.Result {
		case EvidenceResultFail:
			addReason("pass_has_failed_probe")
		case EvidenceResultBlocked:
			addReason("pass_has_blocked_probe")
		}
	}
	for _, check := range report.ContractChecks {
		switch check.Result {
		case EvidenceResultFail:
			addReason("pass_has_failed_contract")
		case EvidenceResultBlocked:
			addReason("pass_has_blocked_contract")
		}
	}
	if len(report.Blockers) > 0 {
		addReason("pass_has_blocker")
	}
}

func validatePartialReport(report VerificationReport, addReason func(string)) {
	if len(report.Blockers) == 0 {
		addReason("partial_missing_blocker")
		return
	}
	for _, blocker := range report.Blockers {
		if blocker.Reason == "" {
			addReason("partial_blocker_missing_reason")
		}
		if !isAllowedBlockerSource(blocker.Source) {
			addReason("partial_invalid_blocker_source")
		}
		if blocker.BlockedScope == "" {
			addReason("partial_blocker_missing_scope")
		}
		if blocker.NextAction == "" {
			addReason("partial_blocker_missing_next_action")
		}
	}
}

func validateFailReport(report VerificationReport, addReason func(string)) {
	if report.Expected == "" || report.Actual == "" {
		addReason("fail_missing_expected_actual")
	}
	if !hasRawEvidenceRef(report) {
		addReason("fail_missing_raw_ref")
	}
	if !hasFailedEvidenceOrContract(report) {
		addReason("fail_missing_failed_evidence")
	}
	if !hasCheckedContract(report.ContractChecks) && !hasContractUnavailableBlocker(report.Blockers) {
		addReason("fail_missing_contract_check")
	}
}

func validateEvidence(evidence []EvidenceRecord, addReason func(string)) {
	for _, record := range evidence {
		if !isKnownEvidenceKind(record.Kind) {
			addReason("unknown_evidence_kind")
		}
		if !isKnownResult(record.Result) {
			addReason("invalid_evidence_result")
		}
	}
}

func validateProbes(probes []ProbeResult, addReason func(string)) {
	for _, probe := range probes {
		if !isKnownProbeType(probe.Type) {
			addReason("unknown_probe_type")
		}
		if !isKnownResult(probe.Result) {
			addReason("invalid_probe_result")
		}
	}
}

func validateContractChecks(checks []ContractCheck, addReason func(string)) {
	for _, check := range checks {
		if check.Name == "" {
			addReason("contract_missing_name")
		}
		if !isKnownResult(check.Result) {
			addReason("invalid_contract_result")
		}
	}
}

func hasPassingExecutionEvidence(evidence []EvidenceRecord) bool {
	for _, record := range evidence {
		if record.Result != EvidenceResultPass {
			continue
		}
		switch record.Kind {
		case EvidenceExecution, EvidenceStaticCheck, EvidenceAdversarial:
			return true
		}
	}
	return false
}

func hasFailedEvidenceOrContract(report VerificationReport) bool {
	for _, record := range report.Evidence {
		if record.Result == EvidenceResultFail {
			return true
		}
	}
	for _, check := range report.ContractChecks {
		if check.Result == EvidenceResultFail {
			return true
		}
	}
	return false
}

func hasRawEvidenceRef(report VerificationReport) bool {
	if len(report.RawRefs) > 0 {
		return true
	}
	for _, record := range report.Evidence {
		if record.RawRef != "" {
			return true
		}
	}
	for _, probe := range report.Probes {
		if probe.RawRef != "" {
			return true
		}
	}
	return false
}

func hasCheckedContract(checks []ContractCheck) bool {
	for _, check := range checks {
		if check.Checked {
			return true
		}
	}
	return false
}

func hasContractUnavailableBlocker(blockers []VerificationBlocker) bool {
	for _, blocker := range blockers {
		if blocker.Source == BlockerContract && blocker.Reason != "" && blocker.BlockedScope != "" && blocker.NextAction != "" {
			return true
		}
	}
	return false
}

func isKnownStatus(status ReportStatus) bool {
	switch status {
	case StatusPass, StatusPartial, StatusFail:
		return true
	default:
		return false
	}
}

func isKnownRequirement(requirement VerificationRequirement) bool {
	switch requirement {
	case VerificationNone, VerificationAnalysisAllowed, VerificationExecutionRequired:
		return true
	default:
		return false
	}
}

func isKnownEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceAnalysis, EvidenceStaticCheck, EvidenceExecution, EvidenceAdversarial, EvidenceUserBlocker:
		return true
	default:
		return false
	}
}

func isKnownProbeType(probe ProbeType) bool {
	switch probe {
	case ProbeBoundary, ProbeReverse, ProbeIdempotency, ProbeErrorPath:
		return true
	default:
		return false
	}
}

func isKnownResult(result string) bool {
	switch result {
	case EvidenceResultPass, EvidenceResultFail, EvidenceResultBlocked:
		return true
	default:
		return false
	}
}

func isAllowedBlockerSource(source string) bool {
	switch source {
	case BlockerEnvironment, BlockerPermission, BlockerToolUnavailable, BlockerUserInput, BlockerContract:
		return true
	default:
		return false
	}
}
