package opssemantic

type OpsSemanticTask struct {
	ID                   string
	SessionID            string
	TurnID               string
	UserGoal             string
	Intent               OpsIntent
	Targets              []OpsTarget
	HostScope            []OpsHostRef
	ActionType           OpsActionType
	RiskLevel            OpsRiskLevel
	MissingSlots         []MissingSlot
	EvidenceRequirements []EvidenceRequirement
	ExecutionPolicy      ExecutionPolicy
	PlanRequired         bool
}

type OpsIntent struct {
	Category string
	Goal     string
	Source   string
}

type OpsTarget struct {
	Kind   string
	Name   string
	Source string
}

type OpsHostRef struct {
	HostID      string
	Address     string
	DisplayName string
	Raw         string
	Source      string
}

type OpsActionType string

const (
	ActionUnknown  OpsActionType = "unknown"
	ActionReadOnly OpsActionType = "read_only"
	ActionWrite    OpsActionType = "write"
)

type OpsRiskLevel string

const (
	RiskReadOnly    OpsRiskLevel = "read_only"
	RiskLowWrite    OpsRiskLevel = "low_write"
	RiskMediumWrite OpsRiskLevel = "medium_write"
	RiskHighWrite   OpsRiskLevel = "high_write"
	RiskDestructive OpsRiskLevel = "destructive"
)

type MissingSlot struct {
	Name   string
	Reason string
}

const SlotTargetHost = "target_host"

type EvidenceRequirement struct {
	Kind        string
	Description string
	Required    bool
}

const EvidenceCommandOutput = "command_output"

type ExecutionPolicy struct {
	AllowParallel    bool
	RequiresApproval bool
}

const (
	SourceUserInput   = "user_input"
	SourceHostMention = "host_mention"
)
