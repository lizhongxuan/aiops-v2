package operatorruntime

import (
	"fmt"
	"strings"
)

type ValidationCatalog struct {
	Resources        []ManagedResource
	Clusters         []PGCluster
	Templates        []InspectionTemplate
	ProblemTypes     []ProblemType
	Actions          []ActionCatalogItem
	WorkflowBindings []WorkflowBinding
}

func ValidateManagedResource(resource ManagedResource) error {
	resource = NormalizeResource(resource)
	if strings.TrimSpace(resource.ID) == "" {
		return NewValidationError("id", "resource id is required")
	}
	if strings.TrimSpace(resource.Kind) == "" {
		return NewValidationError("kind", "resource kind is required")
	}
	if len(resource.Endpoints) == 0 {
		return NewValidationError("endpoints", "at least one endpoint is required")
	}
	if strings.TrimSpace(ResourceMonitorCredentialRef(resource)) == "" {
		return NewValidationError("credentialRefs.monitor", "monitor credential is required")
	}
	if strings.TrimSpace(ResourceRepairCredentialRef(resource)) == "" {
		return NewValidationError("credentialRefs.repair", "repair credential is required")
	}
	for _, endpoint := range resource.Endpoints {
		if strings.TrimSpace(endpoint.ID) == "" {
			return NewValidationError("endpoints", "endpoint id is required")
		}
		if strings.TrimSpace(endpoint.Host) == "" {
			return NewValidationError("endpoints", "endpoint host is required")
		}
	}
	return nil
}

func ValidatePGCluster(cluster PGCluster) error {
	if err := ValidateManagedResource(cluster); err != nil {
		return err
	}
	cluster = NormalizeResource(cluster)
	if strings.TrimSpace(cluster.ID) == "" {
		return NewValidationError("id", "cluster id is required")
	}
	if cluster.Primary.ID == "" || cluster.Primary.Role != PGRolePrimary {
		return NewValidationError("primary", "primary instance is required")
	}
	if len(cluster.Replicas) == 0 {
		return NewValidationError("replicas", "at least one replica is required")
	}
	if strings.TrimSpace(cluster.MonitorCredentialRef) == "" {
		return NewValidationError("monitorCredentialRef", "monitor credential is required")
	}
	if strings.TrimSpace(cluster.RepairCredentialRef) == "" {
		return NewValidationError("repairCredentialRef", "repair credential is required")
	}
	for _, replica := range cluster.Replicas {
		if replica.ID == "" || replica.Role != PGRoleReplica {
			return NewValidationError("replicas", "replica instance must have id and replica role")
		}
	}
	return nil
}

func ValidateInspectionTemplate(template InspectionTemplate) error {
	if strings.TrimSpace(template.ID) == "" {
		return NewValidationError("id", "inspection template id is required")
	}
	if strings.TrimSpace(template.ObjectKind) == "" {
		return NewValidationError("objectKind", "inspection template object kind is required")
	}
	if template.IntervalSeconds <= 0 {
		return NewValidationError("intervalSeconds", "interval seconds must be positive")
	}
	if len(template.Checks) > 0 {
		if err := validateInspectionChecks(template.Checks); err != nil {
			return err
		}
	} else if template.ObjectKind == ObjectKindPostgresReplication {
		if err := ensureReadOnlySQL("primarySql", template.PrimarySQL); err != nil {
			return err
		}
		if err := ensureReadOnlySQL("replicaSql", template.ReplicaSQL); err != nil {
			return err
		}
	} else {
		return NewValidationError("checks", "generic inspection template requires at least one check")
	}
	fields := outputFieldSet(template)
	if len(template.OutputFields) == 0 {
		return NewValidationError("outputFields", "inspection template must declare output fields")
	}
	if template.ObjectKind == ObjectKindPostgresReplication {
		if !fields[FieldReplicaReceiverRunning] {
			return NewValidationError("outputFields", fmt.Sprintf("inspection template must output %s", FieldReplicaReceiverRunning))
		}
		if !fields[FieldReplicaReplayLagSeconds] {
			return NewValidationError("outputFields", fmt.Sprintf("inspection template must output %s", FieldReplicaReplayLagSeconds))
		}
	}
	return nil
}

func validateInspectionChecks(checks []InspectionCheck) error {
	for _, check := range checks {
		if strings.TrimSpace(check.ID) == "" {
			return NewValidationError("checks", "inspection check id is required")
		}
		switch check.Kind {
		case CheckKindSQL:
			if err := ensureReadOnlySQL("checks", check.Query); err != nil {
				return err
			}
		case CheckKindCommand:
			if strings.TrimSpace(check.Command) == "" {
				return NewValidationError("checks", "command check requires command")
			}
		case CheckKindMetric:
			if strings.TrimSpace(check.Query) == "" {
				return NewValidationError("checks", "metric check requires query")
			}
		case CheckKindHTTP:
			if strings.TrimSpace(check.URL) == "" {
				return NewValidationError("checks", "http check requires url")
			}
		default:
			return NewValidationError("checks", fmt.Sprintf("unsupported inspection check kind %q", check.Kind))
		}
	}
	return nil
}

func ensureReadOnlySQL(field, sql string) error {
	normalized := strings.ToLower(strings.TrimSpace(sql))
	if normalized == "" {
		return NewValidationError(field, "read-only sql is required")
	}
	for _, forbidden := range []string{"insert", "update", "delete", "alter", "drop", "truncate", "create"} {
		if strings.Contains(normalized, forbidden+" ") || strings.Contains(normalized, forbidden+"\n") || strings.HasPrefix(normalized, forbidden) {
			return NewValidationError(field, "inspection sql must be read-only")
		}
	}
	return nil
}

func ValidateProblemType(problem ProblemType, template InspectionTemplate, scheduleSeconds int) error {
	if strings.TrimSpace(problem.ID) == "" {
		return NewValidationError("id", "problem type id is required")
	}
	if problem.ForSeconds < scheduleSeconds {
		return NewValidationError("forSeconds", "problem forSeconds must be greater than or equal to scheduleSeconds")
	}
	fields := outputFieldSet(template)
	for _, condition := range problem.Conditions {
		if !fields[condition.Field] {
			return NewValidationError("conditions", fmt.Sprintf("unknown field %s in problem condition", condition.Field))
		}
	}
	return nil
}

func ValidateAction(action ActionCatalogItem) error {
	if strings.TrimSpace(action.ID) == "" {
		return NewValidationError("id", "action id is required")
	}
	if strings.TrimSpace(action.TargetKind) == "" {
		return NewValidationError("targetKind", "target kind is required")
	}
	if action.RiskLevel == "" {
		return NewValidationError("riskLevel", "risk level is required")
	}
	approvalSteps := map[string]bool{}
	for _, id := range action.ConfirmationRequiredSteps {
		approvalSteps[id] = true
	}
	for _, step := range action.Steps {
		if step.Kind == ActionStepRestartService && !step.RequiresApproval && !approvalSteps[step.ID] {
			return NewValidationError("steps", "restart service step requires approval")
		}
	}
	return nil
}

func ValidateWorkflowBinding(binding WorkflowBinding) error {
	if strings.TrimSpace(binding.ID) == "" {
		return NewValidationError("id", "workflow binding id is required")
	}
	if strings.TrimSpace(binding.ActionRef) == "" || strings.TrimSpace(binding.WorkflowRef) == "" {
		return NewValidationError("actionRef", "workflow binding must reference action and workflow")
	}
	required := map[string]bool{"preflight": false, "act": false, "verify": false}
	for _, capability := range binding.Capabilities {
		if _, ok := required[capability]; ok {
			required[capability] = true
		}
	}
	for capability, ok := range required {
		if !ok {
			return NewValidationError("capabilities", fmt.Sprintf("workflow binding must include %s capability", capability))
		}
	}
	if len(binding.VerifyPolicy.SuccessConditions) == 0 &&
		(!binding.VerifyPolicy.ReceiverRunningRequired || binding.VerifyPolicy.MaxReplayLagSeconds <= 0) {
		return NewValidationError("verifyPolicy", "workflow binding requires recovery verify policy")
	}
	return nil
}

func ValidateGuardRule(rule GuardRule, catalog ValidationCatalog) error {
	if strings.TrimSpace(rule.ID) == "" {
		return NewValidationError("id", "guard rule id is required")
	}
	if rule.ScheduleSeconds <= 0 {
		return NewValidationError("scheduleSeconds", "schedule seconds must be positive")
	}
	resources := catalog.Resources
	if len(resources) == 0 {
		resources = catalog.Clusters
	}
	resourceMap := mapByID(resources, func(item ManagedResource) string { return item.ID })
	templates := mapByID(catalog.Templates, func(item InspectionTemplate) string { return item.ID })
	problems := mapByID(catalog.ProblemTypes, func(item ProblemType) string { return item.ID })
	actions := mapByID(catalog.Actions, func(item ActionCatalogItem) string { return item.ID })
	bindings := mapByID(catalog.WorkflowBindings, func(item WorkflowBinding) string { return item.ID })
	if _, ok := resourceMap[ResourceRef(rule)]; !ok {
		return NewValidationError("resourceRef", "guard rule resource ref not found")
	}
	if _, ok := templates[rule.TemplateRef]; !ok {
		return NewValidationError("templateRef", "guard rule template ref not found")
	}
	for _, ref := range rule.ProblemTypeRefs {
		if _, ok := problems[ref]; !ok {
			return NewValidationError("problemTypeRefs", fmt.Sprintf("guard rule problem type ref not found: %s", ref))
		}
	}
	for _, ref := range rule.ActionRefs {
		if _, ok := actions[ref]; !ok {
			return NewValidationError("actionRefs", fmt.Sprintf("guard rule action ref not found: %s", ref))
		}
	}
	if rule.Enabled && len(rule.WorkflowBindingRefs) == 0 {
		return NewValidationError("workflowBindingRefs", "enabled guard rule requires workflow binding")
	}
	for _, ref := range rule.WorkflowBindingRefs {
		if _, ok := bindings[ref]; !ok {
			return NewValidationError("workflowBindingRefs", fmt.Sprintf("guard rule workflow binding ref not found: %s", ref))
		}
	}
	return nil
}

func outputFieldSet(template InspectionTemplate) map[string]bool {
	out := map[string]bool{
		FieldPrimaryReachable:        true,
		FieldReplicaReachable:        true,
		FieldReplicaInRecovery:       true,
		FieldReplicaReceiverRunning:  false,
		FieldReplicaReplayLagSeconds: false,
	}
	for _, field := range template.OutputFields {
		out[field.Name] = true
	}
	return out
}

func mapByID[T any](items []T, id func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	for _, item := range items {
		out[id(item)] = item
	}
	return out
}
