package operatorruntime

import (
	"strings"
	"testing"
)

func TestValidatePGClusterRequiresPrimaryAndReplica(t *testing.T) {
	cluster := validPGCluster()
	cluster.Primary = PGInstance{}
	if err := ValidatePGCluster(cluster); err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("expected missing primary validation error, got %v", err)
	}

	cluster = validPGCluster()
	cluster.Replicas = nil
	if err := ValidatePGCluster(cluster); err == nil || !strings.Contains(err.Error(), "replica") {
		t.Fatalf("expected missing replica validation error, got %v", err)
	}
}

func TestValidateManagedResourceAcceptsGenericMiddlewareResource(t *testing.T) {
	resource := ManagedResource{
		ID:   "redis-cache-prod",
		Name: "redis-cache-prod",
		Kind: "redis",
		Endpoints: []ResourceEndpoint{
			{ID: "redis-cache-prod-a", Role: "leader", Host: "10.0.1.10", Port: 6379, ServiceName: "redis"},
			{ID: "redis-cache-prod-b", Role: "replica", Host: "10.0.1.11", Port: 6379, ServiceName: "redis"},
		},
		CredentialRefs: CredentialRefs{
			Monitor: "redis-monitor-ref",
			Repair:  "redis-repair-ref",
		},
	}

	if err := ValidateManagedResource(resource); err != nil {
		t.Fatalf("generic managed resource should be valid: %v", err)
	}
}

func TestValidateInspectionTemplateRejectsWriteSQL(t *testing.T) {
	template := validInspectionTemplate()
	template.ReplicaSQL = "select 1; drop table users;"
	if err := ValidateInspectionTemplate(template); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only validation error, got %v", err)
	}
}

func TestValidateInspectionTemplateAcceptsGenericChecks(t *testing.T) {
	template := InspectionTemplate{
		ID:              "redis.health.basic.v1",
		Name:            "Redis 基础健康巡检",
		ObjectKind:      "redis",
		IntervalSeconds: 30,
		Checks: []InspectionCheck{
			{ID: "ping", Kind: CheckKindCommand, TargetRole: "leader", Command: "redis-cli ping", TimeoutSeconds: 5},
		},
		OutputFields: []InspectionField{
			{Name: "redis.ping.ok", Type: FieldTypeBool},
			{Name: "redis.connected_clients", Type: FieldTypeNumber},
		},
	}

	if err := ValidateInspectionTemplate(template); err != nil {
		t.Fatalf("generic inspection template should be valid: %v", err)
	}
}

func TestValidateProblemTypeRejectsUnknownField(t *testing.T) {
	template := validInspectionTemplate()
	problem := validLagProblem()
	problem.Conditions = append(problem.Conditions, ProblemCondition{
		Field:    "replica.notExists",
		Operator: OperatorEqual,
		Value:    KnownBool(true),
	})
	if err := ValidateProblemType(problem, template, 60); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field validation error, got %v", err)
	}
}

func TestValidateGuardRuleEnableRequiresWorkflowBinding(t *testing.T) {
	catalog := ValidationCatalog{
		Clusters:         []PGCluster{validPGCluster()},
		Templates:        []InspectionTemplate{validInspectionTemplate()},
		ProblemTypes:     []ProblemType{validLagProblem(), validReceiverStoppedProblem()},
		Actions:          []ActionCatalogItem{validAction()},
		WorkflowBindings: nil,
	}
	err := ValidateGuardRule(validGuardRule(), catalog)
	if err == nil || !strings.Contains(err.Error(), "workflow binding") {
		t.Fatalf("expected workflow binding validation error, got %v", err)
	}
}

func TestValidateMediumRiskRestartRequiresApproval(t *testing.T) {
	action := validAction()
	action.ConfirmationRequiredSteps = nil
	action.Steps[2].RequiresApproval = false
	err := ValidateAction(action)
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("expected restart approval validation error, got %v", err)
	}
}

func TestValidateActionAcceptsGenericTargetKind(t *testing.T) {
	action := ActionCatalogItem{
		ID:          "redis.restart_replica.v1",
		DisplayName: "重启 Redis 副本",
		RiskLevel:   RiskMedium,
		TargetKind:  "redis_replica",
		Steps: []ActionStep{
			{ID: "restart", Kind: ActionStepRestartService, RequiresApproval: true},
		},
	}

	if err := ValidateAction(action); err != nil {
		t.Fatalf("generic action target kind should be valid: %v", err)
	}
}
