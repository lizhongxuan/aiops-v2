package workflowgen

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"aiops-v2/internal/opsmanual"
)

func TestResourcePlanBuilderCreatesPreflightExecuteVerifyRollbackStages(t *testing.T) {
	frame := opsmanual.BuildOperationFrame("帮我写一个workflow,让主机A和主机B的PG两个节点可以通过主机C的pg_mon形成PG集群", nil)
	frame.Roles = []opsmanual.OperationResourceRole{
		{ID: "host-a", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-a", UserLabel: "主机A"},
		{ID: "host-b", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-b", UserLabel: "主机B"},
		{ID: "host-c-monitor", Kind: opsmanual.ResourceRoleMonitor, ResourceRef: "host-c", UserLabel: "主机C", RuntimeName: "pg_mon"},
	}
	frame.ObservationPoints = []opsmanual.OperationObservationPoint{
		{Kind: opsmanual.ObservationPointMonitorComponent, ResourceRef: "host-c", Role: "pg_mon", Access: opsmanual.ObservationAccessUnknown},
	}
	frame.RiskPreference = opsmanual.OperationRiskPreference{DataLossAcceptable: true, StillRequiresApproval: true}

	builder := ResourcePlanBuilder{}
	plan, err := builder.BuildResourcePlan(context.Background(), BuildResourcePlanRequest{
		Requirement:    frame.RawText,
		OperationFrame: frame,
	})
	if err != nil {
		t.Fatalf("BuildResourcePlan() error = %v", err)
	}
	if plan.ReviewStatus != ReviewStatusPendingReview {
		t.Fatalf("ReviewStatus = %q, want %q", plan.ReviewStatus, ReviewStatusPendingReview)
	}
	if plan.ResourceKind != "postgresql" {
		t.Fatalf("ResourceKind = %q, want postgresql", plan.ResourceKind)
	}
	for _, stage := range []string{"preflight", "execute", "verify", "rollback"} {
		if !hasResourceStage(plan, stage) {
			t.Fatalf("missing stage %q in nodes: %#v", stage, plan.Nodes)
		}
	}
	if !usesSecretRefInput(plan) {
		t.Fatalf("resource workflow must use a secret_ref input: %#v", plan.Inputs)
	}
	if !hasRequiredSlot(plan, "target_resources") {
		t.Fatalf("resource workflow must ask for target_resources slot: %#v", plan.RequiredSlots)
	}
	if !hasRequiredSlot(plan, "secret_ref") {
		t.Fatalf("resource workflow must ask for secret_ref slot: %#v", plan.RequiredSlots)
	}
	if got := operationFrameString(plan, "target", "type"); got != "postgresql" {
		t.Fatalf("operation frame target type = %q, want postgresql; frame=%#v", got, plan.OperationFrame)
	}
	if got := operationFrameString(plan, "observation_points", "0", "role"); got != "pg_mon" {
		t.Fatalf("operation frame observation role = %q, want pg_mon; frame=%#v", got, plan.OperationFrame)
	}
}

func TestResourcePlanBuilderCreatesGenericNonPostgresResourcePlan(t *testing.T) {
	frame := opsmanual.BuildOperationFrame("主机A和主机B的Redis主从集群异常，sentinel监控部署在主机C，只需要集群恢复正常。", nil)
	frame.Roles = []opsmanual.OperationResourceRole{
		{ID: "host-a", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-a", UserLabel: "主机A"},
		{ID: "host-b", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-b", UserLabel: "主机B"},
		{ID: "host-c-monitor", Kind: opsmanual.ResourceRoleMonitor, ResourceRef: "host-c", UserLabel: "主机C", RuntimeName: "sentinel"},
	}
	frame.ObservationPoints = []opsmanual.OperationObservationPoint{
		{Kind: opsmanual.ObservationPointMonitorComponent, ResourceRef: "host-c", Role: "sentinel", Access: opsmanual.ObservationAccessUnknown},
	}

	plan, err := (ResourcePlanBuilder{}).BuildResourcePlan(context.Background(), BuildResourcePlanRequest{
		Requirement:    frame.RawText,
		OperationFrame: frame,
	})
	if err != nil {
		t.Fatalf("BuildResourcePlan() error = %v", err)
	}
	if plan.ResourceKind != "redis" {
		t.Fatalf("ResourceKind = %q, want redis", plan.ResourceKind)
	}
	if got := operationFrameString(plan, "observation_points", "0", "role"); got != "sentinel" {
		t.Fatalf("operation frame observation role = %q, want sentinel; frame=%#v", got, plan.OperationFrame)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if strings.Contains(string(raw), "pg_mon") {
		t.Fatalf("non-postgres plan contains pg_mon-specific template: %s", string(raw))
	}
}

func TestResourcePlanBuilderPreservesAssignedMonitorRoleFromOperationFrame(t *testing.T) {
	frame := opsmanual.BuildOperationFrame("帮我写一个workflow，让主机A=@pg-a和主机B=@pg-b的PG两个节点可以通过主机C=@pg-mon的pg_mon形成PG集群", nil)
	plan, err := (ResourcePlanBuilder{}).BuildResourcePlan(context.Background(), BuildResourcePlanRequest{
		Requirement:    frame.RawText,
		OperationFrame: frame,
	})
	if err != nil {
		t.Fatalf("BuildResourcePlan() error = %v", err)
	}
	if got := operationFrameString(plan, "observation_points", "0", "role"); got != "pg_mon" {
		t.Fatalf("operation frame observation role = %q, want pg_mon; frame=%#v", got, plan.OperationFrame)
	}
	if operationFrameHasRole(plan, "主机C", "data_node", "") {
		t.Fatalf("operation frame roles = %#v, want 主机C monitor-only host", plan.OperationFrame["roles"])
	}
	if !operationFrameHasRole(plan, "主机C", "monitor", "pg_mon") {
		t.Fatalf("operation frame roles = %#v, want monitor role for 主机C pg_mon", plan.OperationFrame["roles"])
	}
}

func hasResourceStage(plan *WorkflowGenerationPlan, stage string) bool {
	if plan == nil {
		return false
	}
	for _, node := range plan.Nodes {
		if node.Config == nil {
			continue
		}
		if got, _ := node.Config["stage"].(string); got == stage {
			return true
		}
	}
	return false
}

func usesSecretRefInput(plan *WorkflowGenerationPlan) bool {
	if plan == nil {
		return false
	}
	for _, input := range plan.Inputs {
		if input.ID == "secret_ref" && input.Type == "secret_ref" && input.Required {
			return true
		}
	}
	return false
}

func hasRequiredSlot(plan *WorkflowGenerationPlan, id string) bool {
	if plan == nil {
		return false
	}
	for _, slot := range plan.RequiredSlots {
		if slot.ID == id && slot.Required {
			return true
		}
	}
	return false
}

func operationFrameString(plan *WorkflowGenerationPlan, path ...string) string {
	if plan == nil || len(path) == 0 {
		return ""
	}
	var current any = plan.OperationFrame
	for _, part := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	value, _ := current.(string)
	return value
}

func operationFrameHasRole(plan *WorkflowGenerationPlan, resourceRef, kind, runtimeName string) bool {
	if plan == nil || plan.OperationFrame == nil {
		return false
	}
	roles, _ := plan.OperationFrame["roles"].([]any)
	for _, item := range roles {
		role, _ := item.(map[string]any)
		if role == nil {
			continue
		}
		if role["resource_ref"] == resourceRef && role["kind"] == kind && role["runtime_name"] == runtimeName {
			return true
		}
	}
	return false
}
