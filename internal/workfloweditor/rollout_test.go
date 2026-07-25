package workfloweditor

import (
	"encoding/json"
	"strings"
	"testing"
)

func rolloutConfig(stage WorkflowAgentStage) WorkflowRolloutConfig {
	return WorkflowRolloutConfig{
		SchemaVersion:     WorkflowRolloutSchemaVersion,
		Stage:             stage,
		SaltVersion:       "workflow-rollout-2026-07-v1",
		RolloutPercentage: 100,
	}
}

func TestWorkflowRolloutStagesDeriveMonotonicCapabilities(t *testing.T) {
	stages := []WorkflowAgentStage{
		WorkflowAgentStageDisabled,
		WorkflowAgentStageReadOnly,
		WorkflowAgentStageInternalApply,
		WorkflowAgentStageCanary,
		WorkflowAgentStageGeneral,
	}
	var previous map[WorkflowCapability]bool
	for _, stage := range stages {
		config := rolloutConfig(stage)
		config.Features = WorkflowRolloutFeatures{
			DomainEvents: true, PlanIR: true, ELKLayout: true, EditGrants: true, ExternalMCP: true, MCPApps: true,
		}
		snapshot, err := EvaluateWorkflowRollout(config, "tenant-monotonic")
		if err != nil {
			t.Fatalf("EvaluateWorkflowRollout(%s): %v", stage, err)
		}
		current := snapshot.Capabilities.EnabledMap()
		for capability, enabled := range previous {
			if enabled && !current[capability] {
				t.Fatalf("stage %s removed capability %s", stage, capability)
			}
		}
		previous = current
	}

	readOnly, _ := EvaluateWorkflowRollout(rolloutConfig(WorkflowAgentStageReadOnly), "tenant-a")
	if !readOnly.Capabilities.WorkflowAgentUI || !readOnly.Capabilities.WorkflowMCPRead || !readOnly.Capabilities.WorkflowMCPPreview {
		t.Fatalf("read_only capabilities = %+v", readOnly.Capabilities)
	}
	if readOnly.Capabilities.WorkflowMCPMutation || readOnly.Capabilities.WorkflowDelete || readOnly.Capabilities.WorkflowPublish {
		t.Fatalf("read_only exposed mutation: %+v", readOnly.Capabilities)
	}
	internalApply, _ := EvaluateWorkflowRollout(rolloutConfig(WorkflowAgentStageInternalApply), "tenant-a")
	if !internalApply.Capabilities.WorkflowMCPMutation || internalApply.Capabilities.WorkflowDelete || internalApply.Capabilities.WorkflowPublish || internalApply.Capabilities.WorkflowExecute {
		t.Fatalf("internal_apply capabilities = %+v", internalApply.Capabilities)
	}
	general, _ := EvaluateWorkflowRollout(rolloutConfig(WorkflowAgentStageGeneral), "tenant-a")
	if !general.Capabilities.WorkflowDelete || !general.Capabilities.WorkflowPublish || !general.Capabilities.WorkflowExecute {
		t.Fatalf("general capabilities = %+v", general.Capabilities)
	}
}

func TestWorkflowRolloutDenyWinsAllowAndAllowWinsPercentage(t *testing.T) {
	config := rolloutConfig(WorkflowAgentStageCanary)
	config.RolloutPercentage = 0
	config.AllowTenants = []string{"tenant-allow", "tenant-both"}
	config.DenyTenants = []string{"tenant-deny", "tenant-both"}

	allowed, err := EvaluateWorkflowRollout(config, "tenant-allow")
	if err != nil || allowed.Stage != WorkflowAgentStageCanary || allowed.Decision != WorkflowRolloutDecisionAllowList {
		t.Fatalf("allow decision = %+v, %v", allowed, err)
	}
	denied, err := EvaluateWorkflowRollout(config, "tenant-both")
	if err != nil || denied.Stage != WorkflowAgentStageDisabled || denied.Decision != WorkflowRolloutDecisionDenyList || denied.Capabilities.Any() {
		t.Fatalf("deny decision = %+v, %v", denied, err)
	}
	excluded, err := EvaluateWorkflowRollout(config, "tenant-other")
	if err != nil || excluded.Stage != WorkflowAgentStageDisabled || excluded.Decision != WorkflowRolloutDecisionPercentageExcluded {
		t.Fatalf("percentage exclusion = %+v, %v", excluded, err)
	}
}

func TestWorkflowRolloutStableBucketAndPercentageBoundaries(t *testing.T) {
	first, err := WorkflowRolloutBucket("tenant-stable", "salt-v1")
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		next, nextErr := WorkflowRolloutBucket("tenant-stable", "salt-v1")
		if nextErr != nil || next != first {
			t.Fatalf("unstable bucket: %d, %d, %v", first, next, nextErr)
		}
	}
	changed, err := WorkflowRolloutBucket("tenant-stable", "salt-v2")
	if err != nil || changed == first {
		t.Fatalf("salt version did not change bucket: first=%d changed=%d err=%v", first, changed, err)
	}

	zero := rolloutConfig(WorkflowAgentStageGeneral)
	zero.RolloutPercentage = 0
	zeroSnapshot, zeroErr := EvaluateWorkflowRollout(zero, "tenant-stable")
	if zeroErr != nil || zeroSnapshot.Stage != WorkflowAgentStageDisabled {
		t.Fatalf("0%% rollout = %+v, %v", zeroSnapshot, zeroErr)
	}
	hundred := zero
	hundred.RolloutPercentage = 100
	hundredSnapshot, hundredErr := EvaluateWorkflowRollout(hundred, "tenant-stable")
	if hundredErr != nil || hundredSnapshot.Stage != WorkflowAgentStageGeneral {
		t.Fatalf("100%% rollout = %+v, %v", hundredSnapshot, hundredErr)
	}
	if hundredSnapshot.BucketBasisPoints < 0 || hundredSnapshot.BucketBasisPoints >= WorkflowRolloutBucketCount {
		t.Fatalf("bucket out of range: %+v", hundredSnapshot)
	}
}

func TestWorkflowRolloutOptionalCapabilitiesRequireFlagAndEligibleStage(t *testing.T) {
	features := WorkflowRolloutFeatures{
		DomainEvents: true, PlanIR: true, ELKLayout: true, EditGrants: true, ExternalMCP: true, MCPApps: true,
	}
	readOnlyConfig := rolloutConfig(WorkflowAgentStageReadOnly)
	readOnlyConfig.Features = features
	readOnly, _ := EvaluateWorkflowRollout(readOnlyConfig, "tenant-a")
	if !readOnly.Capabilities.WorkflowDomainEvents {
		t.Fatalf("read_only did not enable configured domain events: %+v", readOnly.Capabilities)
	}
	if readOnly.Capabilities.WorkflowPlanIR || readOnly.Capabilities.WorkflowELKLayout || readOnly.Capabilities.WorkflowEditGrants || readOnly.Capabilities.WorkflowExternalMCP || readOnly.Capabilities.WorkflowMCPApps {
		t.Fatalf("read_only bypassed optional stage gates: %+v", readOnly.Capabilities)
	}
	canaryConfig := rolloutConfig(WorkflowAgentStageCanary)
	canaryConfig.Features = features
	canary, _ := EvaluateWorkflowRollout(canaryConfig, "tenant-a")
	if !canary.Capabilities.WorkflowPlanIR || !canary.Capabilities.WorkflowELKLayout || !canary.Capabilities.WorkflowEditGrants || !canary.Capabilities.WorkflowExternalMCP {
		t.Fatalf("canary optional capabilities = %+v", canary.Capabilities)
	}
	if canary.Capabilities.WorkflowMCPApps {
		t.Fatalf("canary enabled general-only MCP Apps: %+v", canary.Capabilities)
	}
	generalConfig := rolloutConfig(WorkflowAgentStageGeneral)
	generalConfig.Features = features
	general, _ := EvaluateWorkflowRollout(generalConfig, "tenant-a")
	if !general.Capabilities.WorkflowMCPApps {
		t.Fatalf("general did not enable MCP Apps: %+v", general.Capabilities)
	}
}

func TestWorkflowRolloutParsingAndEvaluationFailClosed(t *testing.T) {
	valid := `{
		"schemaVersion":"aiops.workflow.rollout.v1",
		"stage":"read_only",
		"saltVersion":"salt-v1",
		"rolloutPercentage":25,
		"allowTenants":["tenant-a"],
		"denyTenants":["tenant-b"],
		"features":{"workflow_domain_events":true}
	}`
	parsed, err := ParseWorkflowRolloutConfig([]byte(valid))
	if err != nil || parsed.Stage != WorkflowAgentStageReadOnly || !parsed.Features.DomainEvents {
		t.Fatalf("parsed config = %+v, %v", parsed, err)
	}
	for name, raw := range map[string]string{
		"unknown root":   strings.Replace(valid, `"stage":"read_only"`, `"stage":"read_only","unknown":true`, 1),
		"unknown nested": strings.Replace(valid, `"workflow_domain_events":true`, `"workflow_domain_events":true,"unknown":true`, 1),
		"trailing":       valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, parseErr := ParseWorkflowRolloutConfig([]byte(raw)); parseErr == nil {
				t.Fatalf("accepted invalid config: %s", raw)
			}
		})
	}

	invalid := rolloutConfig(WorkflowAgentStage("future"))
	snapshot, evalErr := EvaluateWorkflowRollout(invalid, "tenant-a")
	if evalErr == nil || snapshot.Stage != WorkflowAgentStageDisabled || snapshot.Capabilities.Any() {
		t.Fatalf("invalid stage did not fail closed: %+v, %v", snapshot, evalErr)
	}
	invalid = rolloutConfig(WorkflowAgentStageCanary)
	invalid.RolloutPercentage = 101
	snapshot, evalErr = EvaluateWorkflowRollout(invalid, "tenant-a")
	if evalErr == nil || snapshot.Capabilities.Any() {
		t.Fatalf("invalid percentage did not fail closed: %+v, %v", snapshot, evalErr)
	}
	snapshot, evalErr = EvaluateWorkflowRollout(rolloutConfig(WorkflowAgentStageCanary), " tenant-a ")
	if evalErr == nil || snapshot.Capabilities.Any() {
		t.Fatalf("invalid tenant did not fail closed: %+v, %v", snapshot, evalErr)
	}

	encoded, err := json.Marshal(parsed)
	if err != nil || strings.Contains(string(encoded), "tenant-secret") {
		t.Fatalf("config encoding unexpectedly leaked data: %s, %v", encoded, err)
	}
}
