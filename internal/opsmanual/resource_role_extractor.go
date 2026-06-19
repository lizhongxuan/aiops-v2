package opsmanual

import (
	"regexp"
	"strings"
)

var (
	hostLabelPattern                  = regexp.MustCompile(`(?i)(主机[A-Za-z0-9_-]+|host[-_A-Za-z0-9]+|node[-_A-Za-z0-9]+)`)
	monitorDeploymentPattern          = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]+)(?:监控|monitor)?部署在(主机[A-Za-z0-9_-]+|host[-_A-Za-z0-9]+|node[-_A-Za-z0-9]+)`)
	hostOwnedObserverComponentPattern = regexp.MustCompile(`(?i)(?:通过|由|使用|经由)?(主机[A-Za-z0-9_-]+|host[-_A-Za-z0-9]+|node[-_A-Za-z0-9]+)(?:\s*[=:：]\s*@?[A-Za-z0-9_.-]+)?(?:上?的|上的)([A-Za-z0-9_.-]+)`)
)

type monitorDeployment struct {
	Component string
	Host      string
}

func applyResourceRoleContext(frame *OperationFrame, text string, metadata map[string]any, registry *CapabilityRegistry) {
	if frame == nil {
		return
	}
	monitors := extractMonitorDeployments(text)
	monitorHosts := map[string]bool{}
	for _, monitor := range monitors {
		monitorHosts[monitor.Host] = true
	}
	for _, host := range extractHostLabels(text) {
		if monitorHosts[host] {
			continue
		}
		frame.Roles = appendUniqueRole(frame.Roles, OperationResourceRole{
			ID:           roleID(host),
			Kind:         ResourceRoleDataNode,
			ResourceRef:  host,
			UserLabel:    host,
			InferredFrom: "user_input",
		})
		frame.TargetScope.Hosts = appendUnique(frame.TargetScope.Hosts, host)
	}
	for _, monitor := range monitors {
		frame.Roles = appendUniqueRole(frame.Roles, OperationResourceRole{
			ID:           roleID(monitor.Component + "-" + monitor.Host),
			Kind:         ResourceRoleMonitor,
			ResourceRef:  monitor.Host,
			UserLabel:    monitor.Host,
			RuntimeName:  monitor.Component,
			InferredFrom: "user_input",
		})
		frame.ObservationPoints = append(frame.ObservationPoints, OperationObservationPoint{
			Kind:        ObservationPointMonitorComponent,
			ResourceRef: monitor.Host,
			Role:        monitor.Component,
			Access:      ObservationAccessUnknown,
		})
		frame.Relationships = append(frame.Relationships, OperationResourceRelationship{
			From: monitor.Host,
			To:   firstNonEmpty(frame.Target.Name, frame.Target.Type, "target"),
			Type: RelationshipMonitors,
		})
	}
	if len(frame.Roles) > 0 && frame.ExecutionSurfaceV2.Kind == "" {
		frame.ExecutionSurfaceV2 = OperationExecutionSurface{Kind: executionSurfaceKind(frame.Environment.ExecutionSurface)}
		for _, role := range frame.Roles {
			if role.Kind == ResourceRoleDataNode || role.Kind == ResourceRoleExecutor {
				frame.ExecutionSurfaceV2.Resources = appendUnique(frame.ExecutionSurfaceV2.Resources, role.ResourceRef)
			}
		}
	}
	if dataLossAccepted(text) {
		frame.RiskPreference.DataLossAcceptable = true
		frame.RiskPreference.StillRequiresApproval = true
	}
	frame.EvidenceRequirements = inferEvidenceRequirements(*frame, registry)
}

func extractHostLabels(text string) []string {
	matches := hostLabelPattern.FindAllString(text, -1)
	hosts := make([]string, 0, len(matches))
	for _, match := range matches {
		host := strings.TrimSpace(match)
		if host != "" {
			hosts = appendUnique(hosts, host)
		}
	}
	return hosts
}

func extractMonitorDeployments(text string) []monitorDeployment {
	matches := monitorDeploymentPattern.FindAllStringSubmatch(text, -1)
	out := make([]monitorDeployment, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		component := strings.TrimSpace(match[1])
		host := strings.TrimSpace(match[2])
		if component == "" || host == "" {
			continue
		}
		out = append(out, monitorDeployment{Component: component, Host: host})
	}
	for _, match := range hostOwnedObserverComponentPattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 3 {
			continue
		}
		host := strings.TrimSpace(match[1])
		component := strings.TrimSpace(match[2])
		if host == "" || component == "" || !looksLikeObserverComponent(component) {
			continue
		}
		out = append(out, monitorDeployment{Component: component, Host: host})
	}
	return out
}

func looksLikeObserverComponent(component string) bool {
	normalized := strings.ToLower(strings.TrimSpace(component))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"monitor", "observer", "sentinel", "exporter", "watcher", "_mon", "-mon", ".mon", "agent"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func appendUniqueRole(roles []OperationResourceRole, role OperationResourceRole) []OperationResourceRole {
	key := role.Kind + "\x00" + role.ResourceRef + "\x00" + role.RuntimeName
	for _, existing := range roles {
		if existing.Kind+"\x00"+existing.ResourceRef+"\x00"+existing.RuntimeName == key {
			return roles
		}
	}
	return append(roles, role)
}

func dataLossAccepted(text string) bool {
	normalized := strings.ToLower(text)
	return strings.Contains(normalized, "数据可以不要") ||
		strings.Contains(normalized, "数据不要") ||
		strings.Contains(normalized, "允许数据丢失") ||
		strings.Contains(normalized, "data loss acceptable")
}

func executionSurfaceKind(surface string) string {
	switch strings.TrimSpace(surface) {
	case "ssh", "docker_exec", "kubectl":
		return ExecutionSurfaceHostAgent
	case "runner":
		return ExecutionSurfaceRunner
	case "":
		return ExecutionSurfaceUnknown
	default:
		return strings.TrimSpace(surface)
	}
}

func inferEvidenceRequirements(frame OperationFrame, registry *CapabilityRegistry) []string {
	var requirements []string
	if len(frame.Roles) > 0 {
		requirements = appendUnique(requirements, "resource_roles")
	}
	if len(frame.ObservationPoints) > 0 {
		requirements = appendUnique(requirements, "observer_health")
	}
	if frame.Operation.Stateful || (registry != nil && registry.IsStatefulTargetType(frame.Target.Type)) {
		requirements = appendUnique(requirements, "member_health")
		requirements = appendUnique(requirements, "storage_health")
		requirements = appendUnique(requirements, "sync_status")
	}
	if frame.Operation.Action == "rca_or_repair" || frame.Operation.Action == "restore" || frame.Risk.DataMutation {
		requirements = appendUnique(requirements, "recent_changes")
		requirements = appendUnique(requirements, "rollback_constraints")
	}
	return requirements
}

func roleID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "主机", "host-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}
