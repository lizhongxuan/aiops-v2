package edit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const WorkflowPlanIRSchemaVersion = "aiops.workflow.plan.v1"

type WorkflowPlanItemKind string
type WorkflowPlanOutcome string

const (
	PlanItemAction      WorkflowPlanItemKind = "action"
	PlanItemBranch      WorkflowPlanItemKind = "branch"
	PlanItemParallel    WorkflowPlanItemKind = "parallel"
	PlanItemJoin        WorkflowPlanItemKind = "join"
	PlanItemLoop        WorkflowPlanItemKind = "loop"
	PlanItemApproval    WorkflowPlanItemKind = "approval"
	PlanItemSubflow     WorkflowPlanItemKind = "subflow"
	PlanItemAggregation WorkflowPlanItemKind = "aggregation"
	PlanOutcomeSuccess  WorkflowPlanOutcome  = "success"
	PlanOutcomeFailure  WorkflowPlanOutcome  = "failure"
	PlanOutcomeAlways   WorkflowPlanOutcome  = "always"
)

type WorkflowPlanIR struct {
	SchemaVersion string                      `json:"schemaVersion"`
	Goal          WorkflowPlanGoal            `json:"goal"`
	Items         []WorkflowPlanItem          `json:"items"`
	Variables     []WorkflowPlanVariable      `json:"variables,omitempty"`
	Environment   []WorkflowPlanEnvironment   `json:"environment,omitempty"`
	Unresolved    []WorkflowPlanUnresolved    `json:"unresolved,omitempty"`
	Constraints   WorkflowPlanConstraints     `json:"constraints"`
	Risk          WorkflowPlanRiskExpectation `json:"risk"`
}

type WorkflowPlanGoal struct {
	Summary         string   `json:"summary"`
	SuccessCriteria []string `json:"successCriteria"`
}

type WorkflowPlanItem struct {
	ClientRef   string                   `json:"clientRef"`
	Kind        WorkflowPlanItemKind     `json:"kind"`
	Title       string                   `json:"title"`
	DependsOn   []WorkflowPlanDependency `json:"dependsOn,omitempty"`
	Action      *WorkflowPlanAction      `json:"action,omitempty"`
	Branch      *WorkflowPlanBranch      `json:"branch,omitempty"`
	Parallel    *WorkflowPlanParallel    `json:"parallel,omitempty"`
	Join        *WorkflowPlanJoin        `json:"join,omitempty"`
	Loop        *WorkflowPlanLoop        `json:"loop,omitempty"`
	Approval    *WorkflowPlanApproval    `json:"approval,omitempty"`
	Subflow     *WorkflowPlanSubflow     `json:"subflow,omitempty"`
	Aggregation *WorkflowPlanAggregation `json:"aggregation,omitempty"`
}

type WorkflowPlanDependency struct {
	ClientRef  string              `json:"clientRef"`
	Outcome    WorkflowPlanOutcome `json:"outcome"`
	SourcePort string              `json:"sourcePort,omitempty"`
	TargetPort string              `json:"targetPort,omitempty"`
}

type WorkflowPlanAction struct {
	CatalogID      string                 `json:"catalogId"`
	CatalogVersion string                 `json:"catalogVersion"`
	Arguments      map[string]any         `json:"arguments"`
	InputPorts     []WorkflowPlanPortBind `json:"inputPorts,omitempty"`
	OutputPorts    []WorkflowPlanPortBind `json:"outputPorts,omitempty"`
}

type WorkflowPlanPortBind struct {
	CatalogPort string `json:"catalogPort"`
	PlanPort    string `json:"planPort"`
}

type WorkflowPlanBranch struct {
	Cases      []WorkflowPlanBranchCase `json:"cases"`
	DefaultRef string                   `json:"defaultRef,omitempty"`
}

type WorkflowPlanBranchCase struct {
	Expression string `json:"expression"`
	TargetRef  string `json:"targetRef"`
}

type WorkflowPlanParallel struct {
	BranchRefs []string `json:"branchRefs"`
}

type WorkflowPlanJoin struct {
	WaitForRefs []string `json:"waitForRefs"`
	Strategy    string   `json:"strategy"`
}

type WorkflowPlanLoop struct {
	CollectionPath string   `json:"collectionPath"`
	BodyRefs       []string `json:"bodyRefs"`
	MaxIterations  int      `json:"maxIterations"`
}

type WorkflowPlanApproval struct {
	Prompt       string   `json:"prompt"`
	ApproverRefs []string `json:"approverRefs,omitempty"`
	Timeout      string   `json:"timeout,omitempty"`
}

type WorkflowPlanSubflow struct {
	WorkflowRef string         `json:"workflowRef"`
	Inputs      map[string]any `json:"inputs,omitempty"`
}

type WorkflowPlanAggregation struct {
	InputRefs []string `json:"inputRefs"`
	Strategy  string   `json:"strategy"`
	OutputVar string   `json:"outputVar"`
}

type WorkflowPlanVariable struct {
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Default   json.RawMessage `json:"default,omitempty"`
	SecretRef string          `json:"secretRef,omitempty"`
}

type WorkflowPlanEnvironment struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type WorkflowPlanUnresolved struct {
	ClientRef      string `json:"clientRef"`
	Path           string `json:"path"`
	Requirement    string `json:"requirement"`
	Reason         string `json:"reason"`
	SecretRefScope string `json:"secretRefScope,omitempty"`
}

type WorkflowPlanConstraints struct {
	RequiredPaths         []string                    `json:"requiredPaths"`
	ForbiddenPlaceholders []string                    `json:"forbiddenPlaceholders"`
	Invariants            []WorkflowSemanticInvariant `json:"invariants,omitempty"`
}

type WorkflowSemanticInvariant struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	ClientRefs  []string `json:"clientRefs,omitempty"`
}

type WorkflowPlanRiskExpectation struct {
	Level            string   `json:"level"`
	Reasons          []string `json:"reasons,omitempty"`
	RequiresApproval bool     `json:"requiresApproval"`
	ApprovalRefs     []string `json:"approvalRefs,omitempty"`
}

var workflowPlanClientRefPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[-_][a-z0-9]+)*$`)

func (plan WorkflowPlanIR) Validate() error {
	if plan.SchemaVersion != WorkflowPlanIRSchemaVersion {
		return fmt.Errorf("schemaVersion must be %s", WorkflowPlanIRSchemaVersion)
	}
	if strings.TrimSpace(plan.Goal.Summary) == "" || len(plan.Goal.SuccessCriteria) == 0 || len(plan.Items) == 0 {
		return fmt.Errorf("goal summary, success criteria, and items are required")
	}
	if len(plan.Constraints.RequiredPaths) == 0 || len(plan.Constraints.ForbiddenPlaceholders) == 0 {
		return fmt.Errorf("required paths and forbidden placeholders are required")
	}
	refs := make(map[string]WorkflowPlanItem, len(plan.Items))
	for _, item := range plan.Items {
		if !workflowPlanClientRefPattern.MatchString(item.ClientRef) || strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("invalid clientRef or title for plan item %q", item.ClientRef)
		}
		if _, exists := refs[item.ClientRef]; exists {
			return fmt.Errorf("duplicate clientRef %q", item.ClientRef)
		}
		if err := validateWorkflowPlanItem(item); err != nil {
			return fmt.Errorf("item %s: %w", item.ClientRef, err)
		}
		refs[item.ClientRef] = item
	}
	if err := validateWorkflowPlanReferences(plan, refs); err != nil {
		return err
	}
	if err := validateWorkflowPlanVariables(plan.Variables); err != nil {
		return err
	}
	if err := validateWorkflowPlanRequirements(plan); err != nil {
		return err
	}
	return validateWorkflowPlanSensitiveValues(plan)
}

func validateWorkflowPlanItem(item WorkflowPlanItem) error {
	payloads := []bool{item.Action != nil, item.Branch != nil, item.Parallel != nil, item.Join != nil, item.Loop != nil, item.Approval != nil, item.Subflow != nil, item.Aggregation != nil}
	count := 0
	for _, present := range payloads {
		if present {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("exactly one typed payload is required")
	}
	switch item.Kind {
	case PlanItemAction:
		if item.Action == nil || strings.TrimSpace(item.Action.CatalogID) == "" || !strings.Contains(item.Action.CatalogID, ".") || strings.TrimSpace(item.Action.CatalogVersion) == "" || item.Action.Arguments == nil {
			return fmt.Errorf("action requires a versioned Catalog ID and arguments")
		}
		for _, port := range append(append([]WorkflowPlanPortBind(nil), item.Action.InputPorts...), item.Action.OutputPorts...) {
			if strings.TrimSpace(port.CatalogPort) == "" || strings.TrimSpace(port.PlanPort) == "" {
				return fmt.Errorf("action port bindings must be explicit")
			}
		}
	case PlanItemBranch:
		if item.Branch == nil || len(item.Branch.Cases) == 0 {
			return fmt.Errorf("branch cases are required")
		}
		for _, branch := range item.Branch.Cases {
			if strings.TrimSpace(branch.Expression) == "" || strings.TrimSpace(branch.TargetRef) == "" {
				return fmt.Errorf("branch expression and target are required")
			}
		}
	case PlanItemParallel:
		if item.Parallel == nil || len(item.Parallel.BranchRefs) < 2 {
			return fmt.Errorf("parallel requires at least two branches")
		}
	case PlanItemJoin:
		if item.Join == nil || len(item.Join.WaitForRefs) < 2 || strings.TrimSpace(item.Join.Strategy) == "" {
			return fmt.Errorf("join wait refs and strategy are required")
		}
	case PlanItemLoop:
		if item.Loop == nil || strings.TrimSpace(item.Loop.CollectionPath) == "" || len(item.Loop.BodyRefs) == 0 || item.Loop.MaxIterations <= 0 {
			return fmt.Errorf("bounded loop contract is required")
		}
	case PlanItemApproval:
		if item.Approval == nil || strings.TrimSpace(item.Approval.Prompt) == "" {
			return fmt.Errorf("approval prompt is required")
		}
	case PlanItemSubflow:
		if item.Subflow == nil || strings.TrimSpace(item.Subflow.WorkflowRef) == "" {
			return fmt.Errorf("subflow reference is required")
		}
	case PlanItemAggregation:
		if item.Aggregation == nil || len(item.Aggregation.InputRefs) == 0 || strings.TrimSpace(item.Aggregation.Strategy) == "" || strings.TrimSpace(item.Aggregation.OutputVar) == "" {
			return fmt.Errorf("aggregation inputs, strategy, and output are required")
		}
	default:
		return fmt.Errorf("unsupported plan item kind %q", item.Kind)
	}
	for _, dependency := range item.DependsOn {
		if dependency.Outcome != PlanOutcomeSuccess && dependency.Outcome != PlanOutcomeFailure && dependency.Outcome != PlanOutcomeAlways {
			return fmt.Errorf("dependency outcome must be success, failure, or always")
		}
	}
	return nil
}

func validateWorkflowPlanReferences(plan WorkflowPlanIR, refs map[string]WorkflowPlanItem) error {
	check := func(owner, ref string) error {
		if ref == "" {
			return nil
		}
		if _, exists := refs[ref]; !exists || owner == ref {
			return fmt.Errorf("item %s has invalid reference %q", owner, ref)
		}
		return nil
	}
	for _, item := range plan.Items {
		for _, dependency := range item.DependsOn {
			if err := check(item.ClientRef, dependency.ClientRef); err != nil {
				return err
			}
		}
		var linked []string
		switch item.Kind {
		case PlanItemBranch:
			for _, branch := range item.Branch.Cases {
				linked = append(linked, branch.TargetRef)
			}
			linked = append(linked, item.Branch.DefaultRef)
		case PlanItemParallel:
			linked = item.Parallel.BranchRefs
		case PlanItemJoin:
			linked = item.Join.WaitForRefs
		case PlanItemLoop:
			linked = item.Loop.BodyRefs
		case PlanItemAggregation:
			linked = item.Aggregation.InputRefs
		}
		for _, ref := range linked {
			if err := check(item.ClientRef, ref); err != nil {
				return err
			}
		}
	}
	for _, invariant := range plan.Constraints.Invariants {
		if strings.TrimSpace(invariant.Kind) == "" || strings.TrimSpace(invariant.Description) == "" {
			return fmt.Errorf("semantic invariant kind and description are required")
		}
		for _, ref := range invariant.ClientRefs {
			if _, exists := refs[ref]; !exists {
				return fmt.Errorf("invariant references unknown item %q", ref)
			}
		}
	}
	for _, unresolved := range plan.Unresolved {
		if _, exists := refs[unresolved.ClientRef]; !exists {
			return fmt.Errorf("unresolved input references unknown item %q", unresolved.ClientRef)
		}
	}
	for _, ref := range plan.Risk.ApprovalRefs {
		item, exists := refs[ref]
		if !exists || item.Kind != PlanItemApproval {
			return fmt.Errorf("risk approval references non-approval item %q", ref)
		}
	}
	return nil
}

func validateWorkflowPlanVariables(variables []WorkflowPlanVariable) error {
	seen := map[string]bool{}
	for _, variable := range variables {
		if strings.TrimSpace(variable.Name) == "" || strings.TrimSpace(variable.Type) == "" || seen[variable.Name] {
			return fmt.Errorf("variable name/type must be unique and non-empty")
		}
		seen[variable.Name] = true
		if len(variable.Default) > 0 && !json.Valid(variable.Default) {
			return fmt.Errorf("variable %s has invalid JSON default", variable.Name)
		}
		if variable.SecretRef != "" && len(variable.Default) > 0 {
			return fmt.Errorf("secret variable %s cannot have an inline default", variable.Name)
		}
	}
	return nil
}

func validateWorkflowPlanRequirements(plan WorkflowPlanIR) error {
	for _, environment := range plan.Environment {
		if strings.TrimSpace(environment.Name) == "" || strings.TrimSpace(environment.Version) == "" {
			return fmt.Errorf("environment name and version are required")
		}
	}
	for _, unresolved := range plan.Unresolved {
		if !workflowPlanClientRefPattern.MatchString(unresolved.ClientRef) || !strings.HasPrefix(unresolved.Path, "/") || strings.TrimSpace(unresolved.Requirement) == "" || strings.TrimSpace(unresolved.Reason) == "" {
			return fmt.Errorf("unresolved input requires clientRef, JSON pointer, requirement, and reason")
		}
	}
	if plan.Risk.Level != "low" && plan.Risk.Level != "medium" && plan.Risk.Level != "high" && plan.Risk.Level != "critical" {
		return fmt.Errorf("risk level must be low, medium, high, or critical")
	}
	if plan.Risk.RequiresApproval && len(plan.Risk.ApprovalRefs) == 0 {
		return fmt.Errorf("approval refs are required for approval-gated risk")
	}
	return nil
}

func validateWorkflowPlanSensitiveValues(plan WorkflowPlanIR) error {
	for _, item := range plan.Items {
		var values []any
		if item.Action != nil {
			values = append(values, item.Action.Arguments)
		}
		if item.Subflow != nil {
			values = append(values, item.Subflow.Inputs)
		}
		for _, value := range values {
			if err := walkWorkflowPlanValue(value, plan.Constraints.ForbiddenPlaceholders); err != nil {
				return fmt.Errorf("item %s: %w", item.ClientRef, err)
			}
		}
	}
	return nil
}

func walkWorkflowPlanValue(value any, forbidden []string) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if normalized != "secretref" && isWorkflowPlanSecretField(normalized) {
				return fmt.Errorf("inline credential field %q is forbidden; use secretRef", key)
			}
			if err := walkWorkflowPlanValue(typed[key], forbidden); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := walkWorkflowPlanValue(item, forbidden); err != nil {
				return err
			}
		}
	case string:
		candidate := strings.ToLower(strings.TrimSpace(typed))
		for _, placeholder := range forbidden {
			if token := strings.ToLower(strings.TrimSpace(placeholder)); token != "" && strings.Contains(candidate, token) {
				return fmt.Errorf("forbidden placeholder %q", placeholder)
			}
		}
	}
	return nil
}

func isWorkflowPlanSecretField(field string) bool {
	switch field {
	case "password", "passwd", "secret", "token", "apikey", "accesskey", "privatekey", "clientsecret", "authorization", "bearertoken", "refreshtoken", "accesstoken", "credential", "credentials":
		return true
	default:
		return false
	}
}
