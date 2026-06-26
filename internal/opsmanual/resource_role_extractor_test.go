package opsmanual

import "testing"

func TestBuildOperationFrameAssignsDataNodesAndMonitorRole(t *testing.T) {
	frame := BuildOperationFrame("主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	if got := roleKindByResource(frame, "主机B"); got != ResourceRoleDataNode {
		t.Fatalf("主机B role = %q, want data_node; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "pg_mon")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("pg_mon monitor role = %#v, frame=%#v", monitor, frame)
	}
	if !frame.RiskPreference.DataLossAcceptable || !frame.RiskPreference.StillRequiresApproval {
		t.Fatalf("risk preference = %#v", frame.RiskPreference)
	}
}

func TestBuildOperationFrameUsesGenericMonitorRoleForRedisVariant(t *testing.T) {
	frame := BuildOperationFrame("主机A和主机B的Redis主从集群异常，sentinel监控部署在主机C，只需要集群恢复正常。", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "sentinel")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("sentinel monitor role = %#v, frame=%#v", monitor, frame)
	}
	if frame.Target.Type == "postgresql" {
		t.Fatalf("redis variant was polluted by PG target type: %#v", frame.Target)
	}
}

func TestBuildOperationFrameUsesGenericHostOwnedObserverComponentAsMonitor(t *testing.T) {
	frame := BuildOperationFrame("帮我写一个workflow,让主机A和主机B的PG两个节点可以通过主机C的pg_mon形成PG集群", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	if got := roleKindByResource(frame, "主机B"); got != ResourceRoleDataNode {
		t.Fatalf("主机B role = %q, want data_node; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "pg_mon")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("host-owned observer component role = %#v, frame=%#v", monitor, frame)
	}
}

func TestBuildOperationFrameUsesAssignedHostOwnedObserverComponentAsMonitor(t *testing.T) {
	frame := BuildOperationFrame("帮我写一个workflow，让主机A=@db-a和主机B=@db-b通过主机C=@monitor-1的sentinel形成Redis集群", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	if got := roleKindByResource(frame, "主机B"); got != ResourceRoleDataNode {
		t.Fatalf("主机B role = %q, want data_node; frame=%#v", got, frame)
	}
	if got := roleKindByResource(frame, "主机C"); got == ResourceRoleDataNode {
		t.Fatalf("主机C role = %q, want monitor-only host; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "sentinel")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("assigned host-owned observer component role = %#v, frame=%#v", monitor, frame)
	}
}

func TestBuildOperationFrameDoesNotPromoteReferenceHostsToExecutionResources(t *testing.T) {
	frame := BuildOperationFrame(`请分析主机A当前的恢复问题，不要执行变更。

参考资料：
| hostname | role |
| --- | --- |
| host82 | old standby |

示例日志：
`+"```text\nnode_16 failed to join cluster\n```\n", nil)

	if got := roleByResource(frame, "主机A"); got.Kind != ResourceRoleDataNode || got.SourceKind != ResourceSourceUserRequest || got.Confidence != ResourceConfidenceHigh {
		t.Fatalf("主机A role = %#v, want high-confidence user_request data_node; frame=%#v", got, frame)
	}
	for _, forbidden := range []string{"hostname", "host82", "node_16"} {
		if role := roleByResource(frame, forbidden); role.ResourceRef != "" {
			t.Fatalf("reference resource %q was promoted into roles: %#v frame=%#v", forbidden, role, frame)
		}
		for _, resource := range frame.ExecutionSurfaceV2.Resources {
			if resource == forbidden {
				t.Fatalf("reference resource %q was promoted into execution resources: %#v", forbidden, frame.ExecutionSurfaceV2.Resources)
			}
		}
	}
}

func roleKindByResource(frame OperationFrame, resource string) string {
	for _, role := range frame.Roles {
		if role.ResourceRef == resource || role.UserLabel == resource {
			return role.Kind
		}
	}
	return ""
}

func roleByRuntimeName(frame OperationFrame, runtimeName string) OperationResourceRole {
	for _, role := range frame.Roles {
		if role.RuntimeName == runtimeName {
			return role
		}
	}
	return OperationResourceRole{}
}

func roleByResource(frame OperationFrame, resource string) OperationResourceRole {
	for _, role := range frame.Roles {
		if role.ResourceRef == resource || role.UserLabel == resource {
			return role
		}
	}
	return OperationResourceRole{}
}
