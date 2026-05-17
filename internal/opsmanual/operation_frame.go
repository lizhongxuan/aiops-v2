package opsmanual

import (
	"fmt"
	"regexp"
	"strings"
)

var backupPathPattern = regexp.MustCompile(`(?i)(/data/[^\s，。；,;]+|/[^\s，。；,;]*backup[^\s，。；,;]*)`)
var labeledTargetPattern = regexp.MustCompile(`(?im)(?:目标实例/服务|目标实例|目标服务|目标对象|实例|服务)\s*[:：]\s*([^\n\r，。；,;]+)`)

func BuildOperationFrame(text string, metadata map[string]any) OperationFrame {
	lower := strings.ToLower(text)
	frame := OperationFrame{RawText: text, Metadata: cloneMap(metadata), RequiredParams: map[string]any{}}
	frame.Target.Type = detectObjectType(text)
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
	applyExplicitContextMetadata(&frame, metadata)
	frame.Operation.TargetType = frame.Target.Type
	frame.Operation.Action = detectOperationType(text)
	if frame.Operation.Action == "" && frame.Target.Type != "" {
		frame.Operation.Action = "rca_or_repair"
	}
	frame.OperationType = frame.Operation.Action
	frame.Intent = frame.Operation.Action
	frame.Environment.Env = firstMatch(lower, map[string][]string{
		"prod": {"生产", "prod", "production"},
		"test": {"测试", "test", "staging"},
	})
	frame.Environment.OS = firstMatch(lower, map[string][]string{
		"ubuntu": {"ubuntu"},
		"centos": {"centos"},
		"rocky":  {"rocky"},
		"rhel":   {"rhel"},
		"debian": {"debian"},
	})
	frame.Environment.Platform = firstMatch(lower, map[string][]string{
		"kubernetes": {"k8s", "kubernetes", "kubectl"},
		"docker":     {"docker"},
		"vm":         {"主机", "虚拟机", "vm", "ssh"},
	})
	frame.Environment.ExecutionSurface = firstMatch(lower, map[string][]string{
		"kubectl":     {"kubectl"},
		"docker_exec": {"docker exec"},
		"ssh":         {"ssh", "systemctl"},
	})
	frame.Environment.OSVersion = metadataString(metadata, "os_version")
	if frame.Environment.OSVersion == "" {
		frame.Environment.OSVersion = extractOSVersion(lower)
	}
	frame.Environment.Runtime = firstMatch(lower, map[string][]string{
		"systemd": {"systemd", "systemctl"},
		"docker":  {"docker"},
	})
	frame.Environment.PackageManager = packageManagerForOS(frame.Environment.OS)
	frame.Evidence.Provided = evidenceFromText(lower)
	mergeMetadataEvidence(&frame, metadata)
	if backupPath := firstNonEmpty(metadataString(metadata, "backup_path"), extractBackupPath(text)); backupPath != "" {
		frame.Metadata = ensureMap(frame.Metadata)
		frame.Metadata["backup_path"] = backupPath
		frame.RequiredParams["backup_path"] = backupPath
	}
	frame.Evidence.Missing = missingContext(frame, lower)
	if frame.Operation.Stateful || frame.Target.Type == "redis" || frame.Target.Type == "postgresql" || frame.Target.Type == "mysql" || frame.Target.Type == "kafka" {
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

func evidenceFromText(text string) []string {
	var out []string
	for _, item := range []string{"ssh_access", "pg_isready", "used_memory_rss", "coroot", "p95", "metrics", "pg_version", "disk_free", "connection_test", "rbac_read_ok", "kubectl_access", "pod_exists", "version"} {
		if strings.Contains(text, item) {
			out = appendUnique(out, item)
		}
	}
	if strings.Contains(text, "readonly") || strings.Contains(text, "只读") || strings.Contains(text, "不写入") || strings.Contains(text, "no write") {
		out = appendUnique(out, "readonly")
	}
	if strings.Contains(text, "kubectl") {
		out = appendUnique(out, "kubectl_access")
	}
	if strings.Contains(text, "crashloopbackoff") || strings.Contains(text, "频繁重启") || strings.Contains(text, "反复重启") {
		out = appendUnique(out, "pod_restart")
		out = appendUnique(out, "symptom")
	}
	if strings.Contains(text, "oomkilled") || strings.Contains(text, "内存打爆") {
		out = appendUnique(out, "oom")
		out = appendUnique(out, "symptom")
	}
	if strings.Contains(text, "指标") {
		out = appendUnique(out, "metrics")
	}
	if strings.Contains(text, "lag") || strings.Contains(text, "rebalance") || strings.Contains(text, "broker") || strings.Contains(text, "partition") {
		out = appendUnique(out, "symptom")
		out = appendUnique(out, "metrics")
	}
	if strings.Contains(text, "症状") || strings.Contains(text, "持续上涨") || strings.Contains(text, "升高") ||
		strings.Contains(text, "symptom") || strings.Contains(text, "rising") || strings.Contains(text, "increasing") ||
		strings.Contains(text, "growth") || strings.Contains(text, "timeout") || strings.Contains(text, "latency") {
		out = appendUnique(out, "symptom")
	}
	return out
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

func missingContext(frame OperationFrame, lower string) []string {
	var missing []string
	if frame.Target.Name == "" {
		missing = appendUnique(missing, "target_instance")
	}
	if frame.Environment.Env == "" && shouldRequireEnvironment(frame) {
		missing = appendUnique(missing, "environment")
	}
	if frame.Environment.ExecutionSurface == "" {
		missing = appendUnique(missing, "execution_surface")
	}
	if frame.Operation.Action == "rca_or_repair" {
		if !hasAny(frame.Evidence.Provided, "symptom") {
			missing = appendUnique(missing, "symptom")
		}
		if frame.Target.Type != "kubernetes_pod" && !hasAny(frame.Evidence.Provided, "metrics", "used_memory_rss", "p95", "coroot") {
			missing = appendUnique(missing, "metrics")
		}
	}
	if frame.Operation.Action == "backup" && metadataString(frame.Metadata, "backup_path") == "" && metadataString(frame.RequiredParams, "backup_path") == "" && !strings.Contains(lower, "backup_path") {
		missing = appendUnique(missing, "backup_path")
	}
	return missing
}

func applyExplicitContextMetadata(frame *OperationFrame, metadata map[string]any) {
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
	if frame.Target.Type == "kubernetes_pod" && frame.Target.Name != "" && !valuePresent(frame.RequiredParams["pod_name"]) {
		frame.RequiredParams["pod_name"] = frame.Target.Name
	}
	if frame.Target.Name != "" {
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, frame.Target.Name)
	}
}

func shouldRequireEnvironment(frame OperationFrame) bool {
	if frame.Target.Type == "kubernetes_pod" && frame.TargetScope.Namespace != "" {
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
