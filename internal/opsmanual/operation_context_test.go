package opsmanual

import (
	"encoding/json"
	"testing"
)

func TestOperationFrameV2JSONRoundTripPreservesRolesAndRiskPreference(t *testing.T) {
	frame := OperationFrame{
		Target: OperationTarget{Type: "postgresql", Name: "pg-cluster"},
		Roles: []OperationResourceRole{
			{ID: "host-a", Kind: ResourceRoleDataNode, ResourceRef: "host-a", UserLabel: "主机A", InferredFrom: "user_input"},
			{ID: "host-c-monitor", Kind: ResourceRoleMonitor, ResourceRef: "host-c", UserLabel: "主机C", RuntimeName: "pg_mon", InferredFrom: "user_input"},
		},
		Relationships: []OperationResourceRelationship{
			{From: "host-c", To: "pg-cluster", Type: RelationshipMonitors},
		},
		ExecutionSurfaceV2: OperationExecutionSurface{Kind: ExecutionSurfaceHostAgent, Resources: []string{"host-a", "host-b"}},
		ObservationPoints: []OperationObservationPoint{
			{Kind: ObservationPointMonitorComponent, ResourceRef: "host-c", Role: "pg_mon", Access: ObservationAccessUnknown},
		},
		RiskPreference:       OperationRiskPreference{DataLossAcceptable: true, StillRequiresApproval: true},
		EvidenceRequirements: []string{"cluster_role", "member_health", "observer_health"},
	}
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	var decoded OperationFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Roles[1].Kind != ResourceRoleMonitor || decoded.ObservationPoints[0].Role != "pg_mon" {
		t.Fatalf("decoded monitor role = %#v", decoded)
	}
	if !decoded.RiskPreference.DataLossAcceptable || !decoded.RiskPreference.StillRequiresApproval {
		t.Fatalf("risk preference lost: %#v", decoded.RiskPreference)
	}
}
