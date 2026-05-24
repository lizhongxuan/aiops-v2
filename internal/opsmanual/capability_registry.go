package opsmanual

import (
	"fmt"
	"sort"
	"strings"
)

const (
	resourceProviderDocker     = "docker"
	resourceProviderKubernetes = "kubernetes"
	resourceProviderHost       = "host_process"
)

type OpsManualCapabilityPack struct {
	ID                          string
	BuiltIn                     bool
	Enabled                     bool
	Priority                    int
	ResourceProviders           []CapabilityResourceProvider
	ObjectAliases               []CapabilityAlias
	OperationAliases            []CapabilityAlias
	MiddlewareAliases           []CapabilityAlias
	PlatformAliases             []CapabilityAlias
	ExecutionSurfaceAliases     []CapabilityAlias
	OSAliases                   []CapabilityAlias
	RuntimeAliases              []CapabilityAlias
	EvidenceAliases             []CapabilityAlias
	StatefulTargetTypes         []string
	MetricEvidence              []string
	MetricEvidenceExemptTargets []string
	NamespaceScopedTargetTypes  []string
	TargetNameParams            []CapabilityTargetNameParam
	NegativeKeywordTargets      []string
	ParameterHints              []CapabilityParameterHint
	PreflightProbes             []CapabilityPreflightProbe
	WorkflowOperationRules      []CapabilityWorkflowOperationRule
}

type CapabilityResourceProvider struct {
	ID string
}

type CapabilityAlias struct {
	Value   string
	Needles []string
}

type CapabilityParameterHint struct {
	ID         string
	TargetType string
	Action     string
	Required   bool
	Source     string
}

type CapabilityTargetNameParam struct {
	TargetType string
	Param      string
}

type CapabilityPreflightProbe struct {
	ID         string
	TargetType string
	Action     string
	RiskLevel  string
}

type CapabilityWorkflowOperationRule struct {
	TargetType string
	Action     string
	AllNeedles []string
	AnyNeedles []string
	Evidence   string
}

type CapabilityRegistry struct {
	packs []registeredCapabilityPack
}

type registeredCapabilityPack struct {
	pack  OpsManualCapabilityPack
	order int
}

func NewCapabilityRegistry(packs ...OpsManualCapabilityPack) *CapabilityRegistry {
	registry := &CapabilityRegistry{}
	for _, pack := range packs {
		_ = registry.RegisterPack(pack)
	}
	return registry
}

func DefaultOpsManualCapabilityRegistry() *CapabilityRegistry {
	registry := NewCapabilityRegistry()
	for _, pack := range OpsManualCoreCapabilityPacks() {
		_ = registry.RegisterPack(pack)
	}
	return registry
}

func (r *CapabilityRegistry) RegisterPack(pack OpsManualCapabilityPack) error {
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return fmt.Errorf("opsmanual capability pack id is required")
	}
	r.packs = append(r.packs, registeredCapabilityPack{pack: cloneCapabilityPack(pack), order: len(r.packs)})
	return nil
}

func (r *CapabilityRegistry) ResourceDiscoveryProviders() []CapabilityResourceProvider {
	if r == nil {
		return nil
	}
	var out []CapabilityResourceProvider
	seen := map[string]bool{}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, provider := range registered.pack.ResourceProviders {
			id := strings.TrimSpace(provider.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, CapabilityResourceProvider{ID: id})
		}
	}
	return out
}

func (r *CapabilityRegistry) UnavailableMessage(providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = "resource discovery"
	}
	for _, registered := range r.sortedPacks() {
		for _, provider := range registered.pack.ResourceProviders {
			if strings.EqualFold(strings.TrimSpace(provider.ID), providerID) && !registered.pack.Enabled {
				return fmt.Sprintf("%s capability pack unavailable: pack is disabled", registered.pack.ID)
			}
		}
	}
	return fmt.Sprintf("%s capability unavailable: no enabled capability pack provides it", providerID)
}

func (r *CapabilityRegistry) DetectObjectType(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.ObjectAliases })
}

func (r *CapabilityRegistry) ObjectAliasesFor(value string) []string {
	return r.aliasNeedlesForValue(value, func(pack OpsManualCapabilityPack) []CapabilityAlias {
		return pack.ObjectAliases
	})
}

func (r *CapabilityRegistry) DetectOperationType(text string) string {
	normalized := normalizeText(text)
	if strings.Contains(normalized, "恢复") && strings.Contains(normalized, "数据") {
		return "restore"
	}
	if strings.Contains(normalized, "crashloopbackoff") || strings.Contains(normalized, "oomkilled") ||
		strings.Contains(normalized, "频繁重启") || strings.Contains(normalized, "反复重启") {
		return "rca_or_repair"
	}
	if looksLikeStatusCheck(normalized) && !looksLikeTroubleshooting(normalized) {
		return "status_check"
	}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, alias := range registered.pack.OperationAliases {
			if strings.TrimSpace(alias.Value) == "restart" && !hasPositiveRestartIntent(normalized) {
				continue
			}
			if containsAnyTaxonomyNeedle(normalized, alias.Needles) {
				return strings.TrimSpace(alias.Value)
			}
		}
	}
	return ""
}

func (r *CapabilityRegistry) OperationAliasesFor(value string) []string {
	return r.aliasNeedlesForValue(value, func(pack OpsManualCapabilityPack) []CapabilityAlias {
		return pack.OperationAliases
	})
}

func (r *CapabilityRegistry) DetectMiddlewareType(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.MiddlewareAliases })
}

func (r *CapabilityRegistry) MatchPlatform(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.PlatformAliases })
}

func (r *CapabilityRegistry) MatchExecutionSurface(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.ExecutionSurfaceAliases })
}

func (r *CapabilityRegistry) MatchOS(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.OSAliases })
}

func (r *CapabilityRegistry) MatchRuntime(text string) string {
	return r.detectAlias(text, func(pack OpsManualCapabilityPack) []CapabilityAlias { return pack.RuntimeAliases })
}

func (r *CapabilityRegistry) EvidenceFromText(text string) []string {
	if r == nil {
		return nil
	}
	normalized := strings.ToLower(text)
	var out []string
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, alias := range registered.pack.EvidenceAliases {
			value := strings.TrimSpace(alias.Value)
			if value != "" && containsAnyTaxonomyNeedle(normalized, alias.Needles) {
				out = appendUnique(out, value)
			}
		}
	}
	return out
}

func (r *CapabilityRegistry) IsStatefulTargetType(targetType string) bool {
	return r.stringListContains(targetType, func(pack OpsManualCapabilityPack) []string {
		return pack.StatefulTargetTypes
	})
}

func (r *CapabilityRegistry) IsNamespaceScopedTargetType(targetType string) bool {
	return r.stringListContains(targetType, func(pack OpsManualCapabilityPack) []string {
		return pack.NamespaceScopedTargetTypes
	})
}

func (r *CapabilityRegistry) TargetNameParamFor(targetType string) string {
	targetType = strings.TrimSpace(targetType)
	if r == nil || targetType == "" {
		return ""
	}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, hint := range registered.pack.TargetNameParams {
			if strings.EqualFold(strings.TrimSpace(hint.TargetType), targetType) {
				return strings.TrimSpace(hint.Param)
			}
		}
	}
	return ""
}

func (r *CapabilityRegistry) HasMetricEvidence(evidence []string) bool {
	if r == nil {
		return hasAny(evidence, "metrics")
	}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, item := range registered.pack.MetricEvidence {
			if hasAny(evidence, strings.TrimSpace(item)) {
				return true
			}
		}
	}
	return false
}

func (r *CapabilityRegistry) MetricsExemptTargetType(targetType string) bool {
	return r.stringListContains(targetType, func(pack OpsManualCapabilityPack) []string {
		return pack.MetricEvidenceExemptTargets
	})
}

func (r *CapabilityRegistry) WorkflowTargetTypes() []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, rule := range registered.pack.WorkflowOperationRules {
			out = appendUnique(out, strings.TrimSpace(rule.TargetType))
		}
		for _, target := range registered.pack.NegativeKeywordTargets {
			out = appendUnique(out, strings.TrimSpace(target))
		}
	}
	return out
}

func (r *CapabilityRegistry) DisplayObjectType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "redis":
		return "Redis"
	case "mysql":
		return "MySQL"
	case "postgresql", "postgres", "pg":
		return "PostgreSQL"
	case "kafka":
		return "Kafka"
	case "kubernetes_pod", "k8s_pod":
		return "Kubernetes Pod"
	default:
		return strings.TrimSpace(value)
	}
}

func (r *CapabilityRegistry) WorkflowValidationMarkers() []string {
	return []string{"pg_isready", "node ready", "kubelet_ready", "health endpoint", "status_code"}
}

func (r *CapabilityRegistry) StepTextLooksServiceRestart(text string) bool {
	return containsAny(text, "systemctl restart", "systemctl stop", "systemctl start", "service restart", "restart kubelet", "stop postgres", "start postgres")
}

func (r *CapabilityRegistry) ManualApplicabilityConstraintReason(rule string, platform string) string {
	if strings.Contains(rule, "Kubernetes") && platform != "kubernetes" {
		return "manual mentions Kubernetes applicability constraint"
	}
	return ""
}

func (r *CapabilityRegistry) ParameterHintsFor(targetType, action string) []CapabilityParameterHint {
	if r == nil {
		return nil
	}
	var out []CapabilityParameterHint
	seen := map[string]bool{}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, hint := range registered.pack.ParameterHints {
			if !capabilityScopeMatches(hint.TargetType, targetType) || !capabilityScopeMatches(hint.Action, action) {
				continue
			}
			id := strings.TrimSpace(hint.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			hint.ID = id
			hint.Source = firstNonEmpty(strings.TrimSpace(hint.Source), "capability_pack:"+registered.pack.ID)
			out = append(out, hint)
		}
	}
	return out
}

func (r *CapabilityRegistry) PreflightProbeFor(targetType, action string) (CapabilityPreflightProbe, bool) {
	if r == nil {
		return CapabilityPreflightProbe{}, false
	}
	merged := map[string]CapabilityPreflightProbe{}
	order := []string{}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, probe := range registered.pack.PreflightProbes {
			if !capabilityScopeMatches(probe.TargetType, targetType) || !capabilityScopeMatches(probe.Action, action) {
				continue
			}
			key := firstNonEmpty(strings.TrimSpace(probe.ID), strings.TrimSpace(probe.TargetType)+":"+strings.TrimSpace(probe.Action))
			if key == "" {
				continue
			}
			if current, ok := merged[key]; ok {
				probe.RiskLevel = maxRiskLevel(current.RiskLevel, probe.RiskLevel)
				if strings.TrimSpace(probe.TargetType) == "" {
					probe.TargetType = current.TargetType
				}
				if strings.TrimSpace(probe.Action) == "" {
					probe.Action = current.Action
				}
			} else {
				order = append(order, key)
			}
			merged[key] = probe
		}
	}
	if len(order) == 0 {
		return CapabilityPreflightProbe{}, false
	}
	return merged[order[0]], true
}

func (r *CapabilityRegistry) InferWorkflowOperation(text string) (targetType string, action string, evidence string) {
	lower := strings.ToLower(text)
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, rule := range registered.pack.WorkflowOperationRules {
			if !allNeedlesPresent(lower, rule.AllNeedles) || !anyNeedlePresent(lower, rule.AnyNeedles) {
				continue
			}
			return strings.TrimSpace(rule.TargetType), strings.TrimSpace(rule.Action), strings.TrimSpace(rule.Evidence)
		}
	}
	return "", "", ""
}

func (r *CapabilityRegistry) detectAlias(text string, aliases func(OpsManualCapabilityPack) []CapabilityAlias) string {
	if r == nil {
		return ""
	}
	normalized := normalizeText(text)
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, alias := range aliases(registered.pack) {
			if strings.TrimSpace(alias.Value) != "" && containsAnyTaxonomyNeedle(normalized, alias.Needles) {
				return strings.TrimSpace(alias.Value)
			}
		}
	}
	return ""
}

func (r *CapabilityRegistry) aliasNeedlesForValue(value string, aliases func(OpsManualCapabilityPack) []CapabilityAlias) []string {
	value = strings.TrimSpace(value)
	if r == nil || value == "" {
		return []string{value}
	}
	out := []string{value}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, alias := range aliases(registered.pack) {
			if strings.EqualFold(strings.TrimSpace(alias.Value), value) {
				for _, needle := range alias.Needles {
					out = appendUnique(out, needle)
				}
			}
		}
	}
	return out
}

func (r *CapabilityRegistry) stringListContains(value string, values func(OpsManualCapabilityPack) []string) bool {
	value = strings.TrimSpace(value)
	if r == nil || value == "" {
		return false
	}
	for _, registered := range r.sortedPacks() {
		if !registered.pack.Enabled {
			continue
		}
		for _, item := range values(registered.pack) {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return true
			}
		}
	}
	return false
}

func (r *CapabilityRegistry) sortedPacks() []registeredCapabilityPack {
	if r == nil {
		return nil
	}
	out := append([]registeredCapabilityPack(nil), r.packs...)
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].pack
		right := out[j].pack
		if left.BuiltIn != right.BuiltIn {
			return left.BuiltIn
		}
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		return out[i].order < out[j].order
	})
	return out
}

var coreObjectAliases = []CapabilityAlias{
	{Value: "kafka", Needles: []string{"kafka", "consumer group", "consumer_group", "broker", "partition", "rebalance", "lag"}},
	{Value: "postgresql", Needles: []string{"postgresql", "postgres", "pg_dump", "pg_basebackup", " pg ", " pg-", "pg-", "pg "}},
	{Value: "mysql", Needles: []string{"mysql", "mysqldump"}},
	{Value: "redis", Needles: []string{"redis", "used_memory_rss"}},
	{Value: "kubernetes_pod", Needles: []string{"crashloopbackoff", "oomkilled", " pod ", " pod-", "pod ", "容器组"}},
	{Value: "kubernetes_workload", Needles: []string{"deployment", "statefulset", "daemonset", "workload", "k8s", "kubernetes", "kubectl", "工作负载"}},
	{Value: "host", Needles: []string{"主机", "虚拟机", "vm", "systemd", "systemctl"}},
	{Value: "network", Needles: []string{"network", "网络", "latency", "timeout", "丢包", "dns"}},
	{Value: "tool", Needles: []string{"工具", "tool", "install package"}},
}

var coreOperationAliases = []CapabilityAlias{
	{Value: "backup", Needles: []string{"备份", "backup", "back up", "back-up", "dump"}},
	{Value: "restore", Needles: []string{"数据恢复", "restore", "rollback data"}},
	{Value: "restart", Needles: []string{"重启", "restart", "systemctl restart"}},
	{Value: "scale", Needles: []string{"扩容", "缩容", "scale"}},
	{Value: "deploy", Needles: []string{"部署", "主从", "install", "搭建"}},
	{Value: "migration", Needles: []string{"迁移", "migration", "migrate"}},
	{Value: "status_check", Needles: []string{"status check", "health check", "健康检查", "巡检", "状态检查", "检查状态", "运行状态", "健康状态"}},
	{Value: "rca_or_repair", Needles: []string{"排查", "故障", "诊断", "恢复", "rca", "triage", "troubleshoot", "troubleshooting", "diagnose", "diagnosis", "repair", "checkout", "lag", "rebalance", "broker", "partition", "consumer group", "crashloopbackoff", "oomkilled", "频繁重启"}},
}

func OpsManualCoreCapabilityPacks() []OpsManualCapabilityPack {
	return []OpsManualCapabilityPack{
		{
			ID:                "opsmanual-core.taxonomy",
			BuiltIn:           true,
			Enabled:           true,
			Priority:          1000,
			ObjectAliases:     cloneCapabilityAliases(coreObjectAliases),
			OperationAliases:  cloneCapabilityAliases(coreOperationAliases),
			MiddlewareAliases: []CapabilityAlias{{Value: "postgresql", Needles: []string{"postgresql", "postgres", "pg-"}}, {Value: "mysql", Needles: []string{"mysql"}}, {Value: "redis", Needles: []string{"redis"}}, {Value: "kubelet", Needles: []string{"kubelet"}}},
			PlatformAliases: []CapabilityAlias{
				{Value: "kubernetes", Needles: []string{"k8s", "kubernetes", "kubectl", "kube", "pod"}},
				{Value: "docker", Needles: []string{"docker"}},
				{Value: "vm", Needles: []string{"主机", "虚拟机", "vm", "ssh"}},
			},
			ExecutionSurfaceAliases: []CapabilityAlias{
				{Value: "kubectl", Needles: []string{"kubectl"}},
				{Value: "docker_exec", Needles: []string{"docker exec"}},
				{Value: "ssh", Needles: []string{"ssh", "systemctl", "journalctl", "shell", "script.shell"}},
				{Value: "runner", Needles: []string{"http.request", "builtin.", "probe"}},
			},
			OSAliases: []CapabilityAlias{
				{Value: "ubuntu", Needles: []string{"ubuntu"}},
				{Value: "centos", Needles: []string{"centos"}},
				{Value: "rocky", Needles: []string{"rocky"}},
				{Value: "rhel", Needles: []string{"rhel"}},
				{Value: "debian", Needles: []string{"debian"}},
			},
			RuntimeAliases: []CapabilityAlias{
				{Value: "systemd", Needles: []string{"systemd", "systemctl"}},
				{Value: "docker", Needles: []string{"docker"}},
			},
			EvidenceAliases: []CapabilityAlias{
				{Value: "ssh_access", Needles: []string{"ssh_access"}},
				{Value: "pg_isready", Needles: []string{"pg_isready"}},
				{Value: "used_memory_rss", Needles: []string{"used_memory_rss"}},
				{Value: "coroot", Needles: []string{"coroot"}},
				{Value: "p95", Needles: []string{"p95"}},
				{Value: "metrics", Needles: []string{"metrics", "指标"}},
				{Value: "pg_version", Needles: []string{"pg_version"}},
				{Value: "disk_free", Needles: []string{"disk_free"}},
				{Value: "connection_test", Needles: []string{"connection_test"}},
				{Value: "rbac_read_ok", Needles: []string{"rbac_read_ok"}},
				{Value: "kubectl_access", Needles: []string{"kubectl_access", "kubectl"}},
				{Value: "pod_exists", Needles: []string{"pod_exists"}},
				{Value: "version", Needles: []string{"version"}},
				{Value: "readonly", Needles: []string{"readonly", "只读", "不写入", "no write"}},
				{Value: "pod_restart", Needles: []string{"crashloopbackoff", "频繁重启", "反复重启"}},
				{Value: "oom", Needles: []string{"oomkilled", "内存打爆"}},
				{Value: "symptom", Needles: []string{"crashloopbackoff", "频繁重启", "反复重启", "oomkilled", "内存打爆", "lag", "rebalance", "broker", "partition", "症状", "持续上涨", "升高", "symptom", "rising", "increasing", "growth", "timeout", "latency"}},
			},
			StatefulTargetTypes:         []string{"redis", "postgresql", "mysql", "kafka"},
			MetricEvidence:              []string{"metrics", "used_memory_rss", "p95", "coroot"},
			MetricEvidenceExemptTargets: []string{"kubernetes_pod"},
			NamespaceScopedTargetTypes:  []string{"kubernetes_pod"},
			TargetNameParams:            []CapabilityTargetNameParam{{TargetType: "kubernetes_pod", Param: "pod_name"}},
			NegativeKeywordTargets:      []string{"postgresql", "mysql", "redis", "kubernetes", "kubelet", "network_service", "incident"},
			WorkflowOperationRules: []CapabilityWorkflowOperationRule{
				{TargetType: "postgresql", Action: "restore", AllNeedles: []string{"restore"}, AnyNeedles: []string{"postgresql", "postgres", "pg-"}, Evidence: "capability taxonomy matched postgresql restore"},
				{TargetType: "postgresql", Action: "backup", AllNeedles: []string{}, AnyNeedles: []string{"postgresql backup", "postgres backup", "pg- backup", "postgresql dump", "postgres dump"}, Evidence: "capability taxonomy matched postgresql backup"},
				{TargetType: "mysql", Action: "backup", AllNeedles: []string{}, AnyNeedles: []string{"mysql backup", "mysql dump"}, Evidence: "capability taxonomy matched mysql backup"},
				{TargetType: "kubelet", Action: "repair", AllNeedles: []string{"kubelet"}, AnyNeedles: nil, Evidence: "capability taxonomy matched kubelet repair"},
				{TargetType: "network_service", Action: "inspect", AllNeedles: []string{}, AnyNeedles: []string{"dns probe", "tcp probe", "tls probe", "ssl probe", "http probe", "dns inspect", "tcp inspect", "tls inspect", "ssl inspect", "http inspect", "dns check", "tcp check", "tls check", "ssl check", "http check"}, Evidence: "capability taxonomy matched network probes"},
				{TargetType: "incident", Action: "create_or_notify", AllNeedles: []string{}, AnyNeedles: []string{"incident", "itsm", "ticket", "chatops"}, Evidence: "capability taxonomy matched incident/chatops"},
				{TargetType: "redis", Action: "rca_or_repair", AllNeedles: []string{"redis"}, AnyNeedles: []string{"memory", "rca", "diagnos", "dry run", "策略"}, Evidence: "capability taxonomy matched redis memory operation"},
			},
		},
		{
			ID:       "opsmanual-core.docker",
			BuiltIn:  true,
			Enabled:  true,
			Priority: 900,
			ResourceProviders: []CapabilityResourceProvider{{
				ID: resourceProviderDocker,
			}},
		},
		{
			ID:       "opsmanual-core.host",
			BuiltIn:  true,
			Enabled:  true,
			Priority: 800,
			ResourceProviders: []CapabilityResourceProvider{{
				ID: resourceProviderHost,
			}},
		},
		{
			ID:       "opsmanual-core.kubernetes",
			BuiltIn:  true,
			Enabled:  true,
			Priority: 700,
			ResourceProviders: []CapabilityResourceProvider{{
				ID: resourceProviderKubernetes,
			}},
		},
		{
			ID:       "opsmanual-core.hints",
			BuiltIn:  true,
			Enabled:  true,
			Priority: 600,
			ParameterHints: []CapabilityParameterHint{
				{ID: "target_instance", TargetType: "*", Action: "*", Required: true},
				{ID: "execution_surface", TargetType: "*", Action: "*", Required: true},
				{ID: "backup_path", TargetType: "*", Action: "backup", Required: true},
			},
		},
		{
			ID:       "opsmanual-core.preflight",
			BuiltIn:  true,
			Enabled:  true,
			Priority: 500,
			PreflightProbes: []CapabilityPreflightProbe{
				{ID: "postgresql_restore_probe", TargetType: "postgresql", Action: "restore", RiskLevel: "high"},
				{ID: "kubelet_repair_probe", TargetType: "kubelet", Action: "repair", RiskLevel: "high"},
				{ID: "redis_readonly_probe", TargetType: "redis", Action: "rca_or_repair", RiskLevel: "read_only"},
			},
		},
	}
}

func capabilityAliasesFromTaxonomyRules(rules []taxonomyRule) []CapabilityAlias {
	out := make([]CapabilityAlias, 0, len(rules))
	for _, rule := range rules {
		out = append(out, CapabilityAlias{Value: rule.Value, Needles: cloneStrings(rule.Needles)})
	}
	return out
}

func cloneCapabilityPack(pack OpsManualCapabilityPack) OpsManualCapabilityPack {
	pack.ResourceProviders = append([]CapabilityResourceProvider(nil), pack.ResourceProviders...)
	pack.ObjectAliases = cloneCapabilityAliases(pack.ObjectAliases)
	pack.OperationAliases = cloneCapabilityAliases(pack.OperationAliases)
	pack.MiddlewareAliases = cloneCapabilityAliases(pack.MiddlewareAliases)
	pack.PlatformAliases = cloneCapabilityAliases(pack.PlatformAliases)
	pack.ExecutionSurfaceAliases = cloneCapabilityAliases(pack.ExecutionSurfaceAliases)
	pack.OSAliases = cloneCapabilityAliases(pack.OSAliases)
	pack.RuntimeAliases = cloneCapabilityAliases(pack.RuntimeAliases)
	pack.EvidenceAliases = cloneCapabilityAliases(pack.EvidenceAliases)
	pack.StatefulTargetTypes = cloneStrings(pack.StatefulTargetTypes)
	pack.MetricEvidence = cloneStrings(pack.MetricEvidence)
	pack.MetricEvidenceExemptTargets = cloneStrings(pack.MetricEvidenceExemptTargets)
	pack.NamespaceScopedTargetTypes = cloneStrings(pack.NamespaceScopedTargetTypes)
	pack.TargetNameParams = append([]CapabilityTargetNameParam(nil), pack.TargetNameParams...)
	pack.NegativeKeywordTargets = cloneStrings(pack.NegativeKeywordTargets)
	pack.ParameterHints = append([]CapabilityParameterHint(nil), pack.ParameterHints...)
	pack.PreflightProbes = append([]CapabilityPreflightProbe(nil), pack.PreflightProbes...)
	pack.WorkflowOperationRules = cloneCapabilityWorkflowOperationRules(pack.WorkflowOperationRules)
	return pack
}

func cloneCapabilityAliases(in []CapabilityAlias) []CapabilityAlias {
	out := make([]CapabilityAlias, len(in))
	for i, alias := range in {
		out[i] = CapabilityAlias{Value: alias.Value, Needles: cloneStrings(alias.Needles)}
	}
	return out
}

func cloneCapabilityWorkflowOperationRules(in []CapabilityWorkflowOperationRule) []CapabilityWorkflowOperationRule {
	out := make([]CapabilityWorkflowOperationRule, len(in))
	for i, rule := range in {
		out[i] = CapabilityWorkflowOperationRule{
			TargetType: rule.TargetType,
			Action:     rule.Action,
			AllNeedles: cloneStrings(rule.AllNeedles),
			AnyNeedles: cloneStrings(rule.AnyNeedles),
			Evidence:   rule.Evidence,
		}
	}
	return out
}

func capabilityScopeMatches(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	return pattern == "" || pattern == "*" || value == "" || strings.EqualFold(pattern, value)
}

func allNeedlesPresent(text string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, strings.ToLower(strings.TrimSpace(needle))) {
			return false
		}
	}
	return true
}

func anyNeedlePresent(text string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}
