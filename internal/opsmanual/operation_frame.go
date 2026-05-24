package opsmanual

import (
	"fmt"
	"regexp"
	"strings"
)

var backupPathPattern = regexp.MustCompile(`(?i)(/data/[^\s，。；,;]+|/[^\s，。；,;]*backup[^\s，。；,;]*)`)
var labeledTargetPattern = regexp.MustCompile(`(?im)(?:目标实例/服务|目标实例|目标服务|目标对象|实例|服务)\s*[:：]\s*([^\n\r，。；,;]+)`)

func BuildOperationFrame(text string, metadata map[string]any) OperationFrame {
	return BuildOperationFrameWithCapabilityRegistry(text, metadata, DefaultOpsManualCapabilityRegistry())
}

func BuildOperationFrameWithCapabilityRegistry(text string, metadata map[string]any, registry *CapabilityRegistry) OperationFrame {
	if registry == nil {
		registry = DefaultOpsManualCapabilityRegistry()
	}
	lower := strings.ToLower(text)
	frame := OperationFrame{RawText: text, Metadata: cloneMap(metadata), RequiredParams: map[string]any{}}
	frame.Target.Type = registry.DetectObjectType(text)
	frame.ObjectType = frame.Target.Type
	frame.Target.Name = firstNonEmpty(
		metadataString(metadata, "target_name"),
		metadataString(metadata, "target_instance"),
		metadataString(metadata, "pod_name"),
	)
	if frame.Target.Name == "" {
		frame.Target.Name = extractLabeledTargetName(text, frame.Target.Type)
	}
	if frame.Target.Name != "" {
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, frame.Target.Name)
	}
	applyExplicitContextMetadata(&frame, metadata, registry)
	frame.Operation.TargetType = frame.Target.Type
	frame.Operation.Action = registry.DetectOperationType(text)
	if frame.Operation.Action == "" && frame.Target.Type != "" {
		frame.Operation.Action = "rca_or_repair"
	}
	frame.OperationType = frame.Operation.Action
	frame.Intent = frame.Operation.Action
	frame.Environment.Env = firstMatch(lower, map[string][]string{
		"prod": {"生产", "prod", "production"},
		"test": {"测试", "test", "staging"},
	})
	frame.Environment.OS = registry.MatchOS(lower)
	frame.Environment.Platform = registry.MatchPlatform(lower)
	frame.Environment.ExecutionSurface = registry.MatchExecutionSurface(lower)
	frame.Environment.OSVersion = metadataString(metadata, "os_version")
	if frame.Environment.OSVersion == "" {
		frame.Environment.OSVersion = extractOSVersion(lower)
	}
	frame.Environment.Runtime = registry.MatchRuntime(lower)
	frame.Environment.PackageManager = packageManagerForOS(frame.Environment.OS)
	frame.Evidence.Provided = registry.EvidenceFromText(lower)
	mergeMetadataEvidence(&frame, metadata)
	if backupPath := firstNonEmpty(metadataString(metadata, "backup_path"), extractBackupPath(text)); backupPath != "" {
		frame.Metadata = ensureMap(frame.Metadata)
		frame.Metadata["backup_path"] = backupPath
		frame.RequiredParams["backup_path"] = backupPath
	}
	applyCapabilityParameterHintsToFrame(&frame, registry)
	frame.Evidence.Missing = missingContext(frame, lower, registry)
	if frame.Operation.Stateful || registry.IsStatefulTargetType(frame.Target.Type) {
		frame.Operation.Stateful = true
		frame.Risk.Level = "medium"
		frame.Risk.Reason = "stateful middleware operation"
	}
	if frame.Risk.Level == "" && frame.Operation.Action == "rca_or_repair" && frame.Target.Type != "" {
		frame.Risk.Level = "medium"
		frame.Risk.Reason = "operations troubleshooting"
	}
	if strings.Contains(lower, "恢复") || strings.Contains(lower, "restore") || strings.Contains(lower, "delete") || strings.Contains(lower, "drop") {
		frame.Risk.DataMutation = true
		frame.Risk.Level = maxRiskLevel(frame.Risk.Level, "high")
		frame.Risk.Reason = firstNonEmpty(frame.Risk.Reason, "data mutation operation")
	}
	if hasPositiveRestartIntent(lower) {
		frame.Risk.ServiceRestart = true
		frame.Risk.Level = maxRiskLevel(frame.Risk.Level, "high")
		frame.Risk.Reason = firstNonEmpty(frame.Risk.Reason, "service restart operation")
	}
	if frame.Environment.Env == "prod" {
		frame.Risk.ProductionImpact = "possible"
	}
	if len(frame.RequiredParams) == 0 {
		frame.RequiredParams = nil
	}
	return frame
}

func firstMatch(text string, rules map[string][]string) string {
	for value, needles := range rules {
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return value
			}
		}
	}
	return ""
}

func maxRiskLevel(left, right string) string {
	if riskLevelRank(right) > riskLevelRank(left) {
		return right
	}
	return left
}

func extractLabeledTargetName(text, targetType string) string {
	if targetType == "" {
		return ""
	}
	match := labeledTargetPattern.FindStringSubmatch(text)
	if len(match) <= 1 {
		return ""
	}
	candidate := strings.TrimSpace(strings.Trim(match[1], " `\"'，。；,;"))
	if candidate == "" {
		return ""
	}
	fields := strings.Fields(candidate)
	if len(fields) > 0 {
		candidate = strings.Trim(fields[0], " `\"'，。；,;")
	}
	if candidate == "" {
		return ""
	}
	return candidate
}

func hasPositiveRestartIntent(lower string) bool {
	if strings.Contains(lower, "no restart") || strings.Contains(lower, "without restart") ||
		strings.Contains(lower, "do not restart") || strings.Contains(lower, "不重启") || strings.Contains(lower, "无需重启") {
		return false
	}
	if strings.Contains(lower, "频繁重启") || strings.Contains(lower, "反复重启") {
		return false
	}
	return strings.Contains(lower, "重启") || strings.Contains(lower, "restart") || strings.Contains(lower, "systemctl restart")
}

func missingContext(frame OperationFrame, lower string, registry *CapabilityRegistry) []string {
	var missing []string
	if frame.Target.Name == "" {
		missing = appendUnique(missing, "target_instance")
	}
	if frame.Environment.Env == "" && shouldRequireEnvironment(frame, registry) {
		missing = appendUnique(missing, "environment")
	}
	if frame.Environment.ExecutionSurface == "" {
		missing = appendUnique(missing, "execution_surface")
	}
	if frame.Operation.Action == "rca_or_repair" {
		if !hasAny(frame.Evidence.Provided, "symptom") {
			missing = appendUnique(missing, "symptom")
		}
		if !registry.MetricsExemptTargetType(frame.Target.Type) && !registry.HasMetricEvidence(frame.Evidence.Provided) {
			missing = appendUnique(missing, "metrics")
		}
	}
	if frame.Operation.Action == "backup" && metadataString(frame.Metadata, "backup_path") == "" && metadataString(frame.RequiredParams, "backup_path") == "" && !strings.Contains(lower, "backup_path") {
		missing = appendUnique(missing, "backup_path")
	}
	for name, value := range frame.RequiredParams {
		name = strings.TrimSpace(name)
		if name != "" && !valuePresent(value) {
			missing = appendUnique(missing, name)
		}
	}
	return missing
}

func applyCapabilityParameterHintsToFrame(frame *OperationFrame, registry *CapabilityRegistry) {
	if frame == nil || registry == nil {
		return
	}
	if frame.RequiredParams == nil {
		frame.RequiredParams = map[string]any{}
	}
	for _, hint := range registry.ParameterHintsFor(frame.Target.Type, frame.Operation.Action) {
		if !hint.Required {
			continue
		}
		if capabilityHintSatisfied(*frame, hint.ID) {
			continue
		}
		if _, exists := frame.RequiredParams[hint.ID]; !exists {
			frame.RequiredParams[hint.ID] = ""
		}
	}
}

func capabilityHintSatisfied(frame OperationFrame, id string) bool {
	switch strings.TrimSpace(id) {
	case "target_instance":
		return strings.TrimSpace(frame.Target.Name) != "" || len(frame.TargetScope.Hosts) > 0
	case "execution_surface":
		return strings.TrimSpace(frame.Environment.ExecutionSurface) != ""
	case "backup_path":
		return metadataString(frame.Metadata, "backup_path") != "" || valuePresent(frame.RequiredParams["backup_path"])
	default:
		return valuePresent(frame.RequiredParams[id]) || metadataString(frame.Metadata, id) != ""
	}
}

func applyExplicitContextMetadata(frame *OperationFrame, metadata map[string]any, registry *CapabilityRegistry) {
	if frame == nil {
		return
	}
	if namespace := metadataString(metadata, "namespace"); namespace != "" {
		frame.TargetScope.Namespace = namespace
		frame.RequiredParams["namespace"] = namespace
	}
	if podName := metadataString(metadata, "pod_name"); podName != "" {
		frame.RequiredParams["pod_name"] = podName
		if frame.Target.Name == "" {
			frame.Target.Name = podName
		}
	}
	if param := registry.TargetNameParamFor(frame.Target.Type); param != "" && frame.Target.Name != "" && !valuePresent(frame.RequiredParams[param]) {
		frame.RequiredParams[param] = frame.Target.Name
	}
	if frame.Target.Name != "" {
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, frame.Target.Name)
	}
}

func shouldRequireEnvironment(frame OperationFrame, registry *CapabilityRegistry) bool {
	if registry != nil && registry.IsNamespaceScopedTargetType(frame.Target.Type) && frame.TargetScope.Namespace != "" {
		return false
	}
	return true
}

func extractBackupPath(text string) string {
	return strings.TrimSpace(backupPathPattern.FindString(text))
}

func extractOSVersion(lower string) string {
	for _, os := range []string{"centos", "rhel", "rocky", "ubuntu", "debian"} {
		idx := strings.Index(lower, os)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(lower[idx+len(os):])
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return ""
		}
		candidate := strings.Trim(fields[0], " .，。；,;")
		if candidate != "" && candidate[0] >= '0' && candidate[0] <= '9' {
			return candidate
		}
	}
	return ""
}

func packageManagerForOS(osName string) string {
	switch strings.TrimSpace(strings.ToLower(osName)) {
	case "ubuntu", "debian":
		return "apt"
	case "centos", "rhel", "rocky":
		return "yum"
	default:
		return ""
	}
}

func mergeMetadataEvidence(frame *OperationFrame, metadata map[string]any) {
	if frame == nil || metadata == nil {
		return
	}
	for _, key := range []string{"evidence", "provided_evidence"} {
		switch value := metadata[key].(type) {
		case []string:
			for _, item := range value {
				frame.Evidence.Provided = appendUnique(frame.Evidence.Provided, normalizeEvidence(item))
			}
		case []any:
			for _, item := range value {
				frame.Evidence.Provided = appendUnique(frame.Evidence.Provided, normalizeEvidence(fmt.Sprint(item)))
			}
		case string:
			for _, item := range strings.Split(value, ",") {
				frame.Evidence.Provided = appendUnique(frame.Evidence.Provided, normalizeEvidence(item))
			}
		}
	}
}

func normalizeEvidence(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func ensureMap(in map[string]any) map[string]any {
	if in != nil {
		return in
	}
	return map[string]any{}
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := metadata[key]; ok && value != nil {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}
