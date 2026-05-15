package opsmanual

import "strings"

func cloneManual(in OpsManual) OpsManual {
	out := in
	out.Applicability.MiddlewareVersions = cloneStrings(in.Applicability.MiddlewareVersions)
	out.Applicability.OS = cloneStrings(in.Applicability.OS)
	out.Applicability.Platform = cloneStrings(in.Applicability.Platform)
	out.Applicability.ExecutionSurface = cloneStrings(in.Applicability.ExecutionSurface)
	out.Applicability.Topology = cloneStrings(in.Applicability.Topology)
	out.RequiredContext.RequiredInputs = cloneStrings(in.RequiredContext.RequiredInputs)
	out.RequiredContext.RequiredEvidence = cloneStrings(in.RequiredContext.RequiredEvidence)
	out.RequiredContext.OptionalEvidence = cloneStrings(in.RequiredContext.OptionalEvidence)
	out.ParameterRules = cloneParameterRules(in.ParameterRules)
	out.Preconditions = cloneStrings(in.Preconditions)
	out.Validation = cloneStrings(in.Validation)
	out.CannotUseWhen = cloneStrings(in.CannotUseWhen)
	out.RiskNotes = cloneStrings(in.RiskNotes)
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneCandidate(in ManualCandidate) ManualCandidate {
	out := in
	out.SourceRefs = cloneStrings(in.SourceRefs)
	out.ValidationReport = cloneStrings(in.ValidationReport)
	out.ProposedManual = cloneManual(in.ProposedManual)
	return out
}

func cloneRunRecord(in RunRecord) RunRecord {
	out := in
	out.OperationFrame.Metadata = cloneMap(in.OperationFrame.Metadata)
	out.OperationFrame.RequiredParams = cloneMap(in.OperationFrame.RequiredParams)
	out.OperationFrame.TargetScope.Hosts = cloneStrings(in.OperationFrame.TargetScope.Hosts)
	out.OperationFrame.Evidence.Provided = cloneStrings(in.OperationFrame.Evidence.Provided)
	out.OperationFrame.Evidence.Missing = cloneStrings(in.OperationFrame.Evidence.Missing)
	out.EnvironmentSnapshot = in.EnvironmentSnapshot
	out.RedactedParameters = cloneMap(in.RedactedParameters)
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneParameterRules(in map[string]ParameterRule) map[string]ParameterRule {
	if in == nil {
		return nil
	}
	out := make(map[string]ParameterRule, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func dedupe(values []string) []string {
	out := []string{}
	for _, value := range values {
		out = appendUnique(out, value)
	}
	return out
}

func hasAny(values []string, wants ...string) bool {
	for _, value := range values {
		for _, want := range wants {
			if value == want {
				return true
			}
		}
	}
	return false
}

func hasAnyFold(values []string, want string) bool {
	for _, value := range values {
		if equalFold(value, want) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
