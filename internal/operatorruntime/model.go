package operatorruntime

import "time"

const (
	ObjectKindPostgresReplication = "postgres_replication"

	FieldPrimaryReachable        = "primary.reachable"
	FieldReplicaReachable        = "replica.reachable"
	FieldReplicaInRecovery       = "replica.inRecovery"
	FieldReplicaReceiverRunning  = "replica.receiverRunning"
	FieldReplicaReplayLagSeconds = "replica.replayLagSeconds"
	FieldReplicaReplayLagBytes   = "replica.replayLagBytes"
	FieldReplicaReceiveLSN       = "replica.receiveLsn"
	FieldReplicaReplayLSN        = "replica.replayLsn"
)

type ResourceRole string

const (
	ResourceRolePrimary ResourceRole = "primary"
	ResourceRoleReplica ResourceRole = "replica"

	PGRolePrimary = ResourceRolePrimary
	PGRoleReplica = ResourceRoleReplica
)

type PGRole = ResourceRole

type RiskLevel string

const (
	RiskReadonly RiskLevel = "readonly"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ProblemSeverity string

const (
	SeverityInfo     ProblemSeverity = "info"
	SeverityWarning  ProblemSeverity = "warning"
	SeverityCritical ProblemSeverity = "critical"
)

type GuardRunState string

const (
	GuardRunPendingInspection GuardRunState = "pending_inspection"
	GuardRunInspected         GuardRunState = "inspected"
	GuardRunMatchedProblem    GuardRunState = "matched_problem"
	GuardRunEvidenceCollected GuardRunState = "evidence_collected"
	GuardRunActionSelected    GuardRunState = "action_selected"
	GuardRunWaitingApproval   GuardRunState = "waiting_approval"
	GuardRunRunningWorkflow   GuardRunState = "running_workflow"
	GuardRunVerifying         GuardRunState = "verifying"
	GuardRunSucceeded         GuardRunState = "succeeded"
	GuardRunFailed            GuardRunState = "failed"
	GuardRunBlocked           GuardRunState = "blocked"
	GuardRunDisabledByPolicy  GuardRunState = "disabled_by_policy"
)

type GuardRunDecision string

const (
	DecisionAuto             GuardRunDecision = "auto"
	DecisionRequiresApproval GuardRunDecision = "requires_approval"
	DecisionBlocked          GuardRunDecision = "blocked"
)

type CredentialRefs struct {
	Monitor string `json:"monitor,omitempty"`
	Repair  string `json:"repair,omitempty"`
}

type ManagedResource struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Kind                 string             `json:"kind"`
	Provider             string             `json:"provider,omitempty"`
	Environment          string             `json:"environment,omitempty"`
	Endpoints            []ResourceEndpoint `json:"endpoints,omitempty"`
	CredentialRefs       CredentialRefs     `json:"credentialRefs,omitempty"`
	Primary              ResourceEndpoint   `json:"primary,omitempty"`
	Replicas             []ResourceEndpoint `json:"replicas,omitempty"`
	MonitorCredentialRef string             `json:"monitorCredentialRef"`
	RepairCredentialRef  string             `json:"repairCredentialRef"`
	Tags                 []string           `json:"tags,omitempty"`
	Labels               map[string]string  `json:"labels,omitempty"`
	CreatedAt            time.Time          `json:"createdAt,omitempty"`
	UpdatedAt            time.Time          `json:"updatedAt,omitempty"`
}

type ResourceEndpoint struct {
	ID          string            `json:"id"`
	Role        ResourceRole      `json:"role"`
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	ServiceName string            `json:"serviceName,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type PGCluster = ManagedResource
type PGInstance = ResourceEndpoint

type InspectionTemplate struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	ObjectKind      string            `json:"objectKind"`
	IntervalSeconds int               `json:"intervalSeconds"`
	Checks          []InspectionCheck `json:"checks,omitempty"`
	PrimarySQL      string            `json:"primarySql"`
	ReplicaSQL      string            `json:"replicaSql"`
	OutputFields    []InspectionField `json:"outputFields"`
}

type InspectionCheck struct {
	ID             string    `json:"id"`
	Kind           CheckKind `json:"kind"`
	TargetRole     string    `json:"targetRole,omitempty"`
	Query          string    `json:"query,omitempty"`
	Command        string    `json:"command,omitempty"`
	URL            string    `json:"url,omitempty"`
	TimeoutSeconds int       `json:"timeoutSeconds,omitempty"`
}

type CheckKind string

const (
	CheckKindSQL     CheckKind = "sql"
	CheckKindCommand CheckKind = "command"
	CheckKindMetric  CheckKind = "metric"
	CheckKindHTTP    CheckKind = "http"
)

type InspectionField struct {
	Name string    `json:"name"`
	Type FieldType `json:"type"`
}

type FieldType string

const (
	FieldTypeBool   FieldType = "bool"
	FieldTypeNumber FieldType = "number"
	FieldTypeString FieldType = "string"
)

type FieldValue struct {
	Known  bool      `json:"known"`
	Type   FieldType `json:"type"`
	Bool   bool      `json:"bool,omitempty"`
	Number float64   `json:"number,omitempty"`
	String string    `json:"string,omitempty"`
}

func KnownBool(value bool) FieldValue {
	return FieldValue{Known: true, Type: FieldTypeBool, Bool: value}
}

func KnownNumber(value float64) FieldValue {
	return FieldValue{Known: true, Type: FieldTypeNumber, Number: value}
}

func KnownString(value string) FieldValue {
	return FieldValue{Known: true, Type: FieldTypeString, String: value}
}

func Unknown(kind FieldType) FieldValue {
	return FieldValue{Known: false, Type: kind}
}

type InspectionResult struct {
	SnapshotID string                `json:"snapshotId"`
	ResourceID string                `json:"resourceId"`
	TargetID   string                `json:"targetId"`
	Target     ResourceEndpoint      `json:"target"`
	ClusterID  string                `json:"clusterId"`
	ReplicaID  string                `json:"replicaId"`
	Replica    ResourceEndpoint      `json:"replica"`
	Fields     map[string]FieldValue `json:"fields"`
	Errors     []string              `json:"errors,omitempty"`
	ObservedAt time.Time             `json:"observedAt,omitempty"`
}

type ProblemOperator string

const (
	OperatorEqual              ProblemOperator = "=="
	OperatorNotEqual           ProblemOperator = "!="
	OperatorGreaterThan        ProblemOperator = ">"
	OperatorGreaterThanOrEqual ProblemOperator = ">="
	OperatorLessThan           ProblemOperator = "<"
	OperatorLessThanOrEqual    ProblemOperator = "<="
)

type ProblemType struct {
	ID                    string             `json:"id"`
	DisplayName           string             `json:"displayName"`
	Severity              ProblemSeverity    `json:"severity"`
	Conditions            []ProblemCondition `json:"conditions"`
	ForSeconds            int                `json:"forSeconds"`
	AutoRepairAllowed     bool               `json:"autoRepairAllowed"`
	RecommendedActionRefs []string           `json:"recommendedActionRefs,omitempty"`
}

type ProblemCondition struct {
	Field    string          `json:"field"`
	Operator ProblemOperator `json:"operator"`
	Value    FieldValue      `json:"value"`
}

type ActionCatalogItem struct {
	ID                        string            `json:"id"`
	DisplayName               string            `json:"displayName"`
	RiskLevel                 RiskLevel         `json:"riskLevel"`
	TargetKind                string            `json:"targetKind"`
	InputSchema               map[string]string `json:"inputSchema,omitempty"`
	Steps                     []ActionStep      `json:"steps,omitempty"`
	ConfirmationRequiredSteps []string          `json:"confirmationRequiredSteps,omitempty"`
}

const TargetKindPostgresReplica = "postgres_replica"

type ActionStepKind string

const (
	ActionStepCheckService   ActionStepKind = "check_service"
	ActionStepReloadConfig   ActionStepKind = "reload_config"
	ActionStepRestartService ActionStepKind = "restart_service"
)

type ActionStep struct {
	ID               string         `json:"id"`
	Kind             ActionStepKind `json:"kind"`
	RequiresApproval bool           `json:"requiresApproval,omitempty"`
}

type WorkflowBinding struct {
	ID              string            `json:"id"`
	ActionRef       string            `json:"actionRef"`
	WorkflowRef     string            `json:"workflowRef"`
	WorkflowVersion string            `json:"workflowVersion"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	InputMapping    map[string]string `json:"inputMapping,omitempty"`
	VerifyPolicy    VerifyPolicy      `json:"verifyPolicy"`
}

type VerifyPolicy struct {
	ReceiverRunningRequired bool               `json:"receiverRunningRequired"`
	MaxReplayLagSeconds     int                `json:"maxReplayLagSeconds"`
	TimeoutSeconds          int                `json:"timeoutSeconds"`
	IntervalSeconds         int                `json:"intervalSeconds"`
	SuccessConditions       []ProblemCondition `json:"successConditions,omitempty"`
}

type GuardRule struct {
	ID                              string      `json:"id"`
	Name                            string      `json:"name"`
	ResourceRef                     string      `json:"resourceRef"`
	ClusterRef                      string      `json:"clusterRef"`
	TemplateRef                     string      `json:"templateRef"`
	ProblemTypeRefs                 []string    `json:"problemTypeRefs"`
	ActionRefs                      []string    `json:"actionRefs"`
	WorkflowBindingRefs             []string    `json:"workflowBindingRefs"`
	ScheduleSeconds                 int         `json:"scheduleSeconds"`
	CooldownSeconds                 int         `json:"cooldownSeconds"`
	MaxConcurrency                  int         `json:"maxConcurrency"`
	DisableAfterConsecutiveFailures int         `json:"disableAfterConsecutiveFailures"`
	Enabled                         bool        `json:"enabled"`
	Policy                          GuardPolicy `json:"policy"`
}

type GuardPolicy struct {
	MaxAutoRisk              RiskLevel        `json:"maxAutoRisk"`
	RequireApprovalStepKinds []ActionStepKind `json:"requireApprovalStepKinds,omitempty"`
	Paused                   bool             `json:"paused,omitempty"`
}

type GuardRun struct {
	ID            string                `json:"id"`
	GuardRuleRef  string                `json:"guardRuleRef"`
	State         GuardRunState         `json:"state"`
	ProblemTypeID string                `json:"problemTypeId,omitempty"`
	ActionRef     string                `json:"actionRef,omitempty"`
	WorkflowRun   *WorkflowRun          `json:"workflowRun,omitempty"`
	Recovery      *RecoveryVerification `json:"recovery,omitempty"`
	Events        []GuardRunEvent       `json:"events,omitempty"`
	CreatedAt     time.Time             `json:"createdAt,omitempty"`
	UpdatedAt     time.Time             `json:"updatedAt,omitempty"`
}

type GuardRunEvent struct {
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type GuardFeedback struct {
	GuardRunID     string        `json:"guardRunId"`
	Result         GuardRunState `json:"result"`
	ProblemTypeID  string        `json:"problemTypeId"`
	ActionRef      string        `json:"actionRef"`
	WorkflowStatus string        `json:"workflowStatus,omitempty"`
	WorkflowError  string        `json:"workflowError,omitempty"`
	RecoveryReason string        `json:"recoveryReason,omitempty"`
	Recovered      bool          `json:"recovered"`
	CreatedAt      time.Time     `json:"createdAt"`
}

func NormalizeResource(resource ManagedResource) ManagedResource {
	if resource.Kind == "" && (resource.Primary.ID != "" || len(resource.Replicas) > 0) {
		resource.Kind = "postgresql"
	}
	if resource.CredentialRefs.Monitor == "" {
		resource.CredentialRefs.Monitor = resource.MonitorCredentialRef
	}
	if resource.CredentialRefs.Repair == "" {
		resource.CredentialRefs.Repair = resource.RepairCredentialRef
	}
	if resource.MonitorCredentialRef == "" {
		resource.MonitorCredentialRef = resource.CredentialRefs.Monitor
	}
	if resource.RepairCredentialRef == "" {
		resource.RepairCredentialRef = resource.CredentialRefs.Repair
	}
	if len(resource.Endpoints) == 0 {
		if resource.Primary.ID != "" {
			resource.Endpoints = append(resource.Endpoints, resource.Primary)
		}
		resource.Endpoints = append(resource.Endpoints, resource.Replicas...)
	}
	if resource.Primary.ID == "" {
		for _, endpoint := range resource.Endpoints {
			if endpoint.Role == ResourceRolePrimary {
				resource.Primary = endpoint
				break
			}
		}
	}
	if len(resource.Replicas) == 0 {
		for _, endpoint := range resource.Endpoints {
			if endpoint.Role == ResourceRoleReplica {
				resource.Replicas = append(resource.Replicas, endpoint)
			}
		}
	}
	return resource
}

func ResourceRef(rule GuardRule) string {
	if rule.ResourceRef != "" {
		return rule.ResourceRef
	}
	return rule.ClusterRef
}

func ResourceRepairCredentialRef(resource ManagedResource) string {
	resource = NormalizeResource(resource)
	return resource.CredentialRefs.Repair
}

func ResourceMonitorCredentialRef(resource ManagedResource) string {
	resource = NormalizeResource(resource)
	return resource.CredentialRefs.Monitor
}

func ResourceReplicaEndpoints(resource ManagedResource) []ResourceEndpoint {
	return NormalizeResource(resource).Replicas
}

func ResourceTarget(resource ManagedResource, result InspectionResult) ResourceEndpoint {
	normalized := NormalizeResource(resource)
	if result.Target.ID != "" {
		return result.Target
	}
	if result.Replica.ID != "" {
		return result.Replica
	}
	targetID := result.TargetID
	if targetID == "" {
		targetID = result.ReplicaID
	}
	for _, endpoint := range normalized.Endpoints {
		if endpoint.ID == targetID {
			return endpoint
		}
	}
	return ResourceEndpoint{ID: targetID}
}
