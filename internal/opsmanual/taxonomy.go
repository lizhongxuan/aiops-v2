package opsmanual

import (
	"sort"
	"strings"
)

type taxonomyRule struct {
	Value   string
	Needles []string
}

type TaxonomyMetadata struct {
	CapabilityCandidates []TaxonomyCapabilityMetadata `json:"capability_candidates,omitempty"`
	ResourceKinds        []string                     `json:"resource_kinds,omitempty"`
	EvidenceKinds        []string                     `json:"evidence_kinds,omitempty"`
}

type TaxonomyCapabilityMetadata struct {
	Capability    string   `json:"capability,omitempty"`
	ResourceKind  string   `json:"resource_kind,omitempty"`
	Middleware    string   `json:"middleware,omitempty"`
	Platform      string   `json:"platform,omitempty"`
	Runtime       string   `json:"runtime,omitempty"`
	EvidenceKinds []string `json:"evidence_kinds,omitempty"`
	Source        string   `json:"source,omitempty"`
}

func normalizeText(text string) string {
	replacer := strings.NewReplacer("，", " ", "。", " ", "；", " ", ",", " ", ";", " ", "：", " ", ":", " ")
	normalized := " " + strings.ToLower(replacer.Replace(text)) + " "
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	return normalized
}

func detectObjectType(text string) string {
	return DefaultOpsManualCapabilityRegistry().DetectObjectType(text)
}

func detectOperationType(text string) string {
	return DefaultOpsManualCapabilityRegistry().DetectOperationType(text)
}

func looksLikeStatusCheck(normalized string) bool {
	if containsAnyTaxonomyNeedle(normalized, []string{"status check", "health check", "健康检查", "巡检", "状态检查", "检查状态", "运行状态", "健康状态"}) {
		return true
	}
	return strings.Contains(normalized, "检查") && strings.Contains(normalized, "状态")
}

func BuildTaxonomyMetadata(text string, registry *CapabilityRegistry) TaxonomyMetadata {
	if registry == nil {
		registry = DefaultOpsManualCapabilityRegistry()
	}
	resourceKind := registry.DetectObjectType(text)
	capability := registry.DetectOperationType(text)
	workflowResource, workflowCapability, workflowEvidence := registry.InferWorkflowOperation(text)
	if resourceKind == "" {
		resourceKind = workflowResource
	}
	if capability == "" {
		capability = workflowCapability
	}
	metadata := TaxonomyMetadata{}
	if resourceKind != "" {
		metadata.ResourceKinds = appendUnique(metadata.ResourceKinds, resourceKind)
	}
	evidenceKinds := registry.EvidenceFromText(text)
	for _, evidence := range evidenceKinds {
		metadata.EvidenceKinds = appendUnique(metadata.EvidenceKinds, evidence)
	}
	if workflowEvidence != "" {
		metadata.EvidenceKinds = appendUnique(metadata.EvidenceKinds, workflowEvidence)
	}
	if resourceKind == "" && capability == "" && len(metadata.EvidenceKinds) == 0 {
		return metadata
	}
	candidate := TaxonomyCapabilityMetadata{
		Capability:    capability,
		ResourceKind:  resourceKind,
		Middleware:    registry.DetectMiddlewareType(text),
		Platform:      registry.MatchPlatform(text),
		Runtime:       registry.MatchRuntime(text),
		EvidenceKinds: append([]string(nil), metadata.EvidenceKinds...),
		Source:        "opsmanual_taxonomy_metadata",
	}
	metadata.CapabilityCandidates = append(metadata.CapabilityCandidates, candidate)
	sort.Strings(metadata.ResourceKinds)
	sort.Strings(metadata.EvidenceKinds)
	sort.Slice(metadata.CapabilityCandidates, func(i, j int) bool {
		left := metadata.CapabilityCandidates[i]
		right := metadata.CapabilityCandidates[j]
		if left.ResourceKind != right.ResourceKind {
			return left.ResourceKind < right.ResourceKind
		}
		return left.Capability < right.Capability
	})
	return metadata
}

func looksLikeTroubleshooting(normalized string) bool {
	return containsAnyTaxonomyNeedle(normalized, []string{
		"排查", "故障", "诊断", "异常", "报错", "错误", "失败", "恢复",
		"rca", "triage", "troubleshoot", "troubleshooting", "diagnose", "diagnosis", "repair", "lag", "rebalance", "crashloopbackoff",
		"oomkilled", "频繁重启", "timeout", "latency", "慢查询",
	})
}

func containsAnyTaxonomyNeedle(normalized string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(normalized, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func operationAliases(value string) []string {
	return DefaultOpsManualCapabilityRegistry().OperationAliasesFor(value)
}

func objectAliases(value string) []string {
	return DefaultOpsManualCapabilityRegistry().ObjectAliasesFor(value)
}
