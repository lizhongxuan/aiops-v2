package opsmanual

import "strings"

func cloneManual(in OpsManual) OpsManual {
	out := in
	out.Tags = cloneStrings(in.Tags)
	out.Applicability.MiddlewareVersions = cloneStrings(in.Applicability.MiddlewareVersions)
	out.Applicability.OS = cloneStrings(in.Applicability.OS)
	out.Applicability.Platform = cloneStrings(in.Applicability.Platform)
	out.Applicability.ExecutionSurface = cloneStrings(in.Applicability.ExecutionSurface)
	out.Applicability.Topology = cloneStrings(in.Applicability.Topology)
	out.RequiredContext.RequiredInputs = cloneStrings(in.RequiredContext.RequiredInputs)
	out.RequiredContext.RequiredEvidence = cloneStrings(in.RequiredContext.RequiredEvidence)
	out.RequiredContext.OptionalEvidence = cloneStrings(in.RequiredContext.OptionalEvidence)
	out.RetrievalProfile = cloneRetrievalProfile(in.RetrievalProfile)
	out.RunnableConditions.RequiredParams = cloneStrings(in.RunnableConditions.RequiredParams)
	out.RunnableConditions.AllowedEnvironments = cloneStrings(in.RunnableConditions.AllowedEnvironments)
	out.PreflightProbe.RequiredOutputs = cloneStrings(in.PreflightProbe.RequiredOutputs)
	out.Diagnosis = cloneDiagnosisProfile(in.Diagnosis)
	out.RiskPolicy.ApprovalRequiredWhen = cloneStrings(in.RiskPolicy.ApprovalRequiredWhen)
	out.FallbackGuide.Steps = cloneStrings(in.FallbackGuide.Steps)
	out.ParameterRules = cloneParameterRules(in.ParameterRules)
	out.Preconditions = cloneStrings(in.Preconditions)
	out.Validation = cloneStrings(in.Validation)
	out.CannotUseWhen = cloneStrings(in.CannotUseWhen)
	out.RiskNotes = cloneStrings(in.RiskNotes)
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneParamRequirements(in []ParamRequirement) []ParamRequirement {
	if in == nil {
		return nil
	}
	out := make([]ParamRequirement, len(in))
	for i, requirement := range in {
		out[i] = requirement
		out[i].DependsOn = cloneStrings(requirement.DependsOn)
		out[i].ResolverHints = cloneStrings(requirement.ResolverHints)
		out[i].AskUserWhen = cloneStrings(requirement.AskUserWhen)
	}
	return out
}

func cloneParamCandidates(in []ParamCandidate) []ParamCandidate {
	if in == nil {
		return nil
	}
	out := make([]ParamCandidate, len(in))
	copy(out, in)
	for i := range out {
		out[i].Metadata = cloneMap(in[i].Metadata)
	}
	return out
}

func cloneResolvedParams(in []ResolvedParam) []ResolvedParam {
	if in == nil {
		return nil
	}
	out := make([]ResolvedParam, len(in))
	for i, param := range in {
		out[i] = param
		out[i].Metadata = cloneMap(param.Metadata)
	}
	return out
}

func cloneMissingParams(in []MissingParam) []MissingParam {
	if in == nil {
		return nil
	}
	out := make([]MissingParam, len(in))
	for i, param := range in {
		out[i] = param
		out[i].DependsOn = cloneStrings(param.DependsOn)
		out[i].ResolverHints = cloneStrings(param.ResolverHints)
		out[i].AskUserWhen = cloneStrings(param.AskUserWhen)
		out[i].Candidates = cloneParamCandidates(param.Candidates)
	}
	return out
}

func cloneAmbiguousParams(in []AmbiguousParam) []AmbiguousParam {
	if in == nil {
		return nil
	}
	out := make([]AmbiguousParam, len(in))
	for i, param := range in {
		out[i] = param
		out[i].DependsOn = cloneStrings(param.DependsOn)
		out[i].ResolverHints = cloneStrings(param.ResolverHints)
		out[i].AskUserWhen = cloneStrings(param.AskUserWhen)
		out[i].Candidates = cloneParamCandidates(param.Candidates)
	}
	return out
}

func cloneParamResolutionResult(in ParamResolutionResult) ParamResolutionResult {
	out := in
	out.OperationFrame = cloneOperationFrameValue(in.OperationFrame)
	out.Graph = cloneParamResolutionGraph(in.Graph)
	out.ResolvedParams = cloneResolvedParams(in.ResolvedParams)
	out.MissingParams = cloneMissingParams(in.MissingParams)
	out.AmbiguousParams = cloneAmbiguousParams(in.AmbiguousParams)
	out.Fields = cloneParamResolutionFormFields(in.Fields)
	return out
}

func cloneParamResolutionGraph(in ParamResolutionGraph) ParamResolutionGraph {
	out := in
	if in.Nodes != nil {
		out.Nodes = make([]ParamResolutionNode, len(in.Nodes))
		for i, node := range in.Nodes {
			out.Nodes[i] = node
			out.Nodes[i].Requirement = cloneParamRequirements([]ParamRequirement{node.Requirement})[0]
			if node.Resolved != nil {
				resolved := cloneResolvedParams([]ResolvedParam{*node.Resolved})[0]
				out.Nodes[i].Resolved = &resolved
			}
			if node.Missing != nil {
				missing := cloneMissingParams([]MissingParam{*node.Missing})[0]
				out.Nodes[i].Missing = &missing
			}
			if node.Ambiguous != nil {
				ambiguous := cloneAmbiguousParams([]AmbiguousParam{*node.Ambiguous})[0]
				out.Nodes[i].Ambiguous = &ambiguous
			}
			out.Nodes[i].Dependencies = cloneStrings(node.Dependencies)
			out.Nodes[i].ResolverLog = cloneParamResolverLogs(node.ResolverLog)
		}
	}
	if in.Edges != nil {
		out.Edges = make([]ParamResolutionEdge, len(in.Edges))
		copy(out.Edges, in.Edges)
	}
	return out
}

func cloneParamResolverLogs(in []ParamResolverLog) []ParamResolverLog {
	if in == nil {
		return nil
	}
	out := make([]ParamResolverLog, len(in))
	copy(out, in)
	return out
}

func cloneParamResolutionFormFields(in []ParamResolutionFormField) []ParamResolutionFormField {
	if in == nil {
		return nil
	}
	out := make([]ParamResolutionFormField, len(in))
	for i, field := range in {
		out[i] = field
		out[i].Candidates = cloneParamCandidates(field.Candidates)
	}
	return out
}

func cloneRetrievalProfile(in RetrievalProfile) RetrievalProfile {
	out := in
	out.Aliases = cloneStringSliceMap(in.Aliases)
	out.Keywords = cloneStrings(in.Keywords)
	out.NegativeKeywords = cloneStrings(in.NegativeKeywords)
	return out
}

func cloneDiagnosisProfile(in DiagnosisProfile) DiagnosisProfile {
	out := in
	out.ApplicableSymptoms = cloneStrings(in.ApplicableSymptoms)
	out.NotApplicableWhen = cloneStrings(in.NotApplicableWhen)
	out.AllowedEvidenceSources = cloneStrings(in.AllowedEvidenceSources)
	out.RecommendedEvidenceOrder = cloneStrings(in.RecommendedEvidenceOrder)
	out.KeyJudgmentRules = cloneStrings(in.KeyJudgmentRules)
	out.CommonMisdiagnoses = cloneStrings(in.CommonMisdiagnoses)
	out.ConfidenceCriteria = cloneStrings(in.ConfidenceCriteria)
	out.ConservativeWording = cloneStrings(in.ConservativeWording)
	out.ApprovalRequiredActions = cloneStrings(in.ApprovalRequiredActions)
	out.MinimumRiskNextSteps = cloneStrings(in.MinimumRiskNextSteps)
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

func cloneOperationFrameValue(in OperationFrame) OperationFrame {
	out := in
	out.TargetScope.Hosts = cloneStrings(in.TargetScope.Hosts)
	out.Evidence.Provided = cloneStrings(in.Evidence.Provided)
	out.Evidence.Missing = cloneStrings(in.Evidence.Missing)
	out.RequiredParams = cloneMap(in.RequiredParams)
	out.Metadata = cloneMap(in.Metadata)
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

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = cloneStrings(value)
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
