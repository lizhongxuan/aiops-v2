package workfloweditor

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	WorkflowRolloutSchemaVersion = "aiops.workflow.rollout.v1"
	WorkflowRolloutBucketCount   = 10_000
)

type WorkflowAgentStage string

const (
	WorkflowAgentStageDisabled      WorkflowAgentStage = "disabled"
	WorkflowAgentStageReadOnly      WorkflowAgentStage = "read_only"
	WorkflowAgentStageInternalApply WorkflowAgentStage = "internal_apply"
	WorkflowAgentStageCanary        WorkflowAgentStage = "canary"
	WorkflowAgentStageGeneral       WorkflowAgentStage = "general"
)

type WorkflowRolloutDecision string

const (
	WorkflowRolloutDecisionInvalid            WorkflowRolloutDecision = "invalid"
	WorkflowRolloutDecisionDenyList           WorkflowRolloutDecision = "deny_list"
	WorkflowRolloutDecisionAllowList          WorkflowRolloutDecision = "allow_list"
	WorkflowRolloutDecisionPercentage         WorkflowRolloutDecision = "percentage"
	WorkflowRolloutDecisionPercentageExcluded WorkflowRolloutDecision = "percentage_excluded"
)

type WorkflowCapability string

const (
	WorkflowCapabilityAgentUI      WorkflowCapability = "workflow_agent_ui"
	WorkflowCapabilityMCPRead      WorkflowCapability = "workflow_mcp_read"
	WorkflowCapabilityMCPPreview   WorkflowCapability = "workflow_mcp_preview"
	WorkflowCapabilityMCPMutation  WorkflowCapability = "workflow_mcp_mutation"
	WorkflowCapabilityDelete       WorkflowCapability = "workflow_delete"
	WorkflowCapabilityPublish      WorkflowCapability = "workflow_publish"
	WorkflowCapabilityExecute      WorkflowCapability = "workflow_execute"
	WorkflowCapabilityDomainEvents WorkflowCapability = "workflow_domain_events"
	WorkflowCapabilityPlanIR       WorkflowCapability = "workflow_plan_ir"
	WorkflowCapabilityELKLayout    WorkflowCapability = "workflow_elk_layout"
	WorkflowCapabilityEditGrants   WorkflowCapability = "workflow_edit_grants"
	WorkflowCapabilityExternalMCP  WorkflowCapability = "workflow_external_mcp"
	WorkflowCapabilityMCPApps      WorkflowCapability = "workflow_mcp_apps"
)

type WorkflowRolloutFeatures struct {
	DomainEvents bool `json:"workflow_domain_events"`
	PlanIR       bool `json:"workflow_plan_ir"`
	ELKLayout    bool `json:"workflow_elk_layout"`
	EditGrants   bool `json:"workflow_edit_grants"`
	ExternalMCP  bool `json:"workflow_external_mcp"`
	MCPApps      bool `json:"workflow_mcp_apps"`
}

type WorkflowRolloutConfig struct {
	SchemaVersion     string                  `json:"schemaVersion"`
	Stage             WorkflowAgentStage      `json:"stage"`
	SaltVersion       string                  `json:"saltVersion"`
	RolloutPercentage int                     `json:"rolloutPercentage"`
	AllowTenants      []string                `json:"allowTenants,omitempty"`
	DenyTenants       []string                `json:"denyTenants,omitempty"`
	Features          WorkflowRolloutFeatures `json:"features"`
}

type WorkflowCapabilities struct {
	WorkflowAgentUI      bool `json:"workflow_agent_ui"`
	WorkflowMCPRead      bool `json:"workflow_mcp_read"`
	WorkflowMCPPreview   bool `json:"workflow_mcp_preview"`
	WorkflowMCPMutation  bool `json:"workflow_mcp_mutation"`
	WorkflowDelete       bool `json:"workflow_delete"`
	WorkflowPublish      bool `json:"workflow_publish"`
	WorkflowExecute      bool `json:"workflow_execute"`
	WorkflowDomainEvents bool `json:"workflow_domain_events"`
	WorkflowPlanIR       bool `json:"workflow_plan_ir"`
	WorkflowELKLayout    bool `json:"workflow_elk_layout"`
	WorkflowEditGrants   bool `json:"workflow_edit_grants"`
	WorkflowExternalMCP  bool `json:"workflow_external_mcp"`
	WorkflowMCPApps      bool `json:"workflow_mcp_apps"`
}

type WorkflowCapabilitySnapshot struct {
	SchemaVersion     string                  `json:"schemaVersion"`
	Stage             WorkflowAgentStage      `json:"stage"`
	SaltVersion       string                  `json:"saltVersion"`
	RolloutPercentage int                     `json:"rolloutPercentage"`
	BucketBasisPoints int                     `json:"bucketBasisPoints"`
	Decision          WorkflowRolloutDecision `json:"decision"`
	Capabilities      WorkflowCapabilities    `json:"capabilities"`
}

func ParseWorkflowRolloutConfig(data []byte) (WorkflowRolloutConfig, error) {
	var config WorkflowRolloutConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return WorkflowRolloutConfig{}, fmt.Errorf("decode workflow rollout config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return WorkflowRolloutConfig{}, errors.New("decode workflow rollout config: trailing JSON data")
	}
	if err := config.Validate(); err != nil {
		return WorkflowRolloutConfig{}, err
	}
	return config, nil
}

func (c WorkflowRolloutConfig) Validate() error {
	if c.SchemaVersion != WorkflowRolloutSchemaVersion {
		return fmt.Errorf("unsupported workflow rollout schema %q", c.SchemaVersion)
	}
	if workflowStageRank(c.Stage) < 0 {
		return fmt.Errorf("unsupported workflow agent stage %q", c.Stage)
	}
	if !canonicalRolloutSaltVersion(c.SaltVersion) {
		return errors.New("workflow rollout saltVersion is required and must be canonical")
	}
	if c.RolloutPercentage < 0 || c.RolloutPercentage > 100 {
		return errors.New("workflow rollout percentage must be between 0 and 100")
	}
	if err := validateTenantList(c.AllowTenants, "allowTenants"); err != nil {
		return err
	}
	if err := validateTenantList(c.DenyTenants, "denyTenants"); err != nil {
		return err
	}
	return nil
}

func EvaluateWorkflowRollout(config WorkflowRolloutConfig, tenantID string) (WorkflowCapabilitySnapshot, error) {
	failClosed := WorkflowCapabilitySnapshot{
		SchemaVersion: WorkflowRolloutSchemaVersion,
		Stage:         WorkflowAgentStageDisabled,
		Decision:      WorkflowRolloutDecisionInvalid,
	}
	if err := config.Validate(); err != nil {
		return failClosed, err
	}
	failClosed.SaltVersion = config.SaltVersion
	failClosed.RolloutPercentage = config.RolloutPercentage
	if !canonicalTenantID(tenantID) {
		return failClosed, errors.New("workflow rollout tenant id is required and must be canonical")
	}
	bucket, err := WorkflowRolloutBucket(tenantID, config.SaltVersion)
	if err != nil {
		return failClosed, err
	}
	failClosed.BucketBasisPoints = bucket

	stage := WorkflowAgentStageDisabled
	decision := WorkflowRolloutDecisionPercentageExcluded
	switch {
	case containsTenant(config.DenyTenants, tenantID):
		decision = WorkflowRolloutDecisionDenyList
	case containsTenant(config.AllowTenants, tenantID):
		stage, decision = config.Stage, WorkflowRolloutDecisionAllowList
	case bucket < config.RolloutPercentage*100:
		stage, decision = config.Stage, WorkflowRolloutDecisionPercentage
	}
	return WorkflowCapabilitySnapshot{
		SchemaVersion:     WorkflowRolloutSchemaVersion,
		Stage:             stage,
		SaltVersion:       config.SaltVersion,
		RolloutPercentage: config.RolloutPercentage,
		BucketBasisPoints: bucket,
		Decision:          decision,
		Capabilities:      DeriveWorkflowCapabilities(stage, config.Features),
	}, nil
}

func WorkflowRolloutBucket(tenantID, saltVersion string) (int, error) {
	if !canonicalTenantID(tenantID) {
		return 0, errors.New("workflow rollout tenant id is required and must be canonical")
	}
	if !canonicalRolloutSaltVersion(saltVersion) {
		return 0, errors.New("workflow rollout saltVersion is required and must be canonical")
	}
	digest := sha256.Sum256([]byte("aiops.workflow.rollout\x00" + saltVersion + "\x00" + tenantID))
	return int(binary.BigEndian.Uint64(digest[:8]) % WorkflowRolloutBucketCount), nil
}

func DeriveWorkflowCapabilities(stage WorkflowAgentStage, features WorkflowRolloutFeatures) WorkflowCapabilities {
	rank := workflowStageRank(stage)
	if rank < workflowStageRank(WorkflowAgentStageReadOnly) {
		return WorkflowCapabilities{}
	}
	capabilities := WorkflowCapabilities{
		WorkflowAgentUI:      true,
		WorkflowMCPRead:      true,
		WorkflowMCPPreview:   true,
		WorkflowDomainEvents: features.DomainEvents,
	}
	if rank >= workflowStageRank(WorkflowAgentStageInternalApply) {
		capabilities.WorkflowMCPMutation = true
	}
	if rank >= workflowStageRank(WorkflowAgentStageCanary) {
		capabilities.WorkflowDelete = true
		capabilities.WorkflowPlanIR = features.PlanIR
		capabilities.WorkflowELKLayout = features.ELKLayout
		capabilities.WorkflowEditGrants = features.EditGrants
		capabilities.WorkflowExternalMCP = features.ExternalMCP
	}
	if rank >= workflowStageRank(WorkflowAgentStageGeneral) {
		capabilities.WorkflowPublish = true
		capabilities.WorkflowExecute = true
		capabilities.WorkflowMCPApps = features.MCPApps
	}
	return capabilities
}

func (c WorkflowCapabilities) Any() bool {
	for _, enabled := range c.EnabledMap() {
		if enabled {
			return true
		}
	}
	return false
}

func (c WorkflowCapabilities) EnabledMap() map[WorkflowCapability]bool {
	return map[WorkflowCapability]bool{
		WorkflowCapabilityAgentUI: c.WorkflowAgentUI, WorkflowCapabilityMCPRead: c.WorkflowMCPRead,
		WorkflowCapabilityMCPPreview: c.WorkflowMCPPreview, WorkflowCapabilityMCPMutation: c.WorkflowMCPMutation,
		WorkflowCapabilityDelete: c.WorkflowDelete, WorkflowCapabilityPublish: c.WorkflowPublish,
		WorkflowCapabilityExecute: c.WorkflowExecute, WorkflowCapabilityDomainEvents: c.WorkflowDomainEvents,
		WorkflowCapabilityPlanIR: c.WorkflowPlanIR, WorkflowCapabilityELKLayout: c.WorkflowELKLayout,
		WorkflowCapabilityEditGrants: c.WorkflowEditGrants, WorkflowCapabilityExternalMCP: c.WorkflowExternalMCP,
		WorkflowCapabilityMCPApps: c.WorkflowMCPApps,
	}
}

func workflowStageRank(stage WorkflowAgentStage) int {
	switch stage {
	case WorkflowAgentStageDisabled:
		return 0
	case WorkflowAgentStageReadOnly:
		return 1
	case WorkflowAgentStageInternalApply:
		return 2
	case WorkflowAgentStageCanary:
		return 3
	case WorkflowAgentStageGeneral:
		return 4
	default:
		return -1
	}
}

func validateTenantList(values []string, field string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !canonicalTenantID(value) {
			return fmt.Errorf("workflow rollout %s contains a non-canonical tenant id", field)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("workflow rollout %s contains duplicate tenant %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsTenant(values []string, tenantID string) bool {
	for _, value := range values {
		if value == tenantID {
			return true
		}
	}
	return false
}

func canonicalTenantID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." && value != ".." && len(value) <= 256 && !strings.ContainsAny(value, "\\/\x00")
}

func canonicalRolloutSaltVersion(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 64 {
		return false
	}
	for _, current := range value {
		if !((current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') || (current >= '0' && current <= '9') || current == '.' || current == '_' || current == '-') {
			return false
		}
	}
	return true
}
